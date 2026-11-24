// Package website coordinates node route changes in an installer-managed website.
package website

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"proxy-panel/internal/model"
)

type Manager struct{ manifestPath, configPath, databasePath, kernelPath, deploymentLock string }

func New(manifestPath, configPath, databasePath, kernelPath string) *Manager {
	return &Manager{manifestPath: manifestPath, configPath: configPath, databasePath: databasePath, kernelPath: kernelPath,
		deploymentLock: filepath.Join(filepath.Dir(configPath), ".update.lock")}
}

type manifest struct {
	Mode            string           `json:"mode"`
	NodeID          int64            `json:"node_id"`
	Database        string           `json:"database"`
	KernelPath      string           `json:"kernel_path"`
	Path            string           `json:"path"`
	NodeHost        string           `json:"node_host"`
	Protocol        string           `json:"protocol"`
	Host            string           `json:"host"`
	SNI             string           `json:"sni"`
	Cert            string           `json:"cert"`
	Key             string           `json:"key"`
	Domains         []string         `json:"domains"`
	CA              string           `json:"ca"`
	Listens         []netip.AddrPort `json:"listens"`
	RealityFrontend *realityFrontend `json:"reality_frontend"`
}

// Change must be coordinated with the node database and Xray under the caller's lock.
// Rollback restores Nginx after the caller has restored the old Xray configuration.
type Change struct {
	manager                                        *Manager
	state                                          manifest
	oldManifest, newManifest, oldConfig, newConfig []byte
	nodeID                                         int64
	newPath                                        string
	newReality                                     *realityProbe
	changed                                        bool
}

var routePath = regexp.MustCompile(`^/[A-Za-z0-9/_~.\-]*$`)

// Prepare is read-only except for a temporary nginx -t configuration, removed before return.
// Unmanaged sites and unrelated nodes do not participate in the synchronous update.
func (m *Manager) Prepare(old, candidate model.Node) (*Change, error) {
	return m.PrepareUpdate(old, &candidate)
}

// PrepareUpdate also resolves the installer's loopback binding when a Reality
// route changes. Persist and apply the returned candidate under the same lock.
func (m *Manager) PrepareUpdate(old model.Node, candidate *model.Node) (*Change, error) {
	// Display metadata does not affect the managed route and must remain editable
	// even when the independently managed website manifest needs repair.
	if sameWebsiteRoute(old, *candidate) {
		return nil, nil
	}
	state, data, err := m.readManifest()
	if state == nil || err != nil {
		return nil, err
	}
	if state.Mode != "standalone" && state.NodeID != old.ID && (state.RealityFrontend != nil && state.RealityFrontend.NodeID == old.ID || wantsRealityShare(*state, old, *candidate)) {
		return m.prepareReality(*state, data, old, candidate)
	}
	if err := state.checkListener(*candidate); err != nil {
		return nil, err
	}
	if state.Mode == "standalone" || state.NodeID != old.ID {
		return nil, nil
	}
	var before, after map[string]any
	if json.Unmarshal([]byte(old.Settings), &before) != nil || json.Unmarshal([]byte(candidate.Settings), &after) != nil || before == nil || after == nil {
		return nil, errors.New("托管节点 settings 必须是 JSON 对象")
	}
	if candidate.ID != old.ID || candidate.Host != old.Host || candidate.Port != old.Port || candidate.Protocol != old.Protocol || candidate.Transport != old.Transport || candidate.KernelType != old.KernelType || candidate.Enable != old.Enable {
		return nil, fmt.Errorf("%w：托管网站依赖此节点的地址、端口、协议和启用状态；请先调整网站部署，面板当前仅自动同步路径", ErrManagedConflict)
	}
	for _, key := range []string{"security", "listen"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return nil, fmt.Errorf("%w：托管网站依赖节点的 %s 设置，请先调整网站部署；面板当前仅自动同步路径", ErrManagedConflict, key)
		}
	}
	if !reflect.DeepEqual(routeALPN(before), routeALPN(after)) {
		return nil, fmt.Errorf("%w：托管网站依赖节点的 ALPN 设置，请先调整网站部署", ErrManagedConflict)
	}
	if text(before, "host") != text(after, "host") {
		return nil, fmt.Errorf("%w：托管网站依赖节点的 Host 设置，请先调整网站部署；面板当前仅自动同步路径", ErrManagedConflict)
	}
	// The panel normalizes legacy aliases when saving the complete settings form.
	// Compare their effective values, not the spelling of the stored JSON keys.
	if firstText(before, "sni", "serverName", old.Host) != firstText(after, "sni", "serverName", candidate.Host) ||
		!sameFilePath(firstText(before, "cert_path", "certPath", ""), firstText(after, "cert_path", "certPath", "")) ||
		!sameFilePath(firstText(before, "key_path", "keyPath", ""), firstText(after, "key_path", "keyPath", "")) {
		return nil, fmt.Errorf("%w：托管网站依赖节点的 SNI 和证书设置，请先调整网站部署；面板当前仅自动同步路径", ErrManagedConflict)
	}
	oldPath, _ := before["path"].(string)
	if oldPath == "" {
		oldPath = "/"
	}
	newPath, ok := after["path"].(string)
	if !ok {
		newPath = "/"
	}
	if newPath == oldPath {
		return nil, nil
	}
	if !routePath.MatchString(newPath) || newPath == "/" || path.Clean(newPath) != strings.TrimSuffix(newPath, "/") {
		return nil, errors.New("托管 HTTPUpgrade 路径须以 / 开头，仅含字母、数字、/、_、~、.、-，且不能是根路径、重复斜杠或 . / .. 路径段")
	}
	if (state.Mode != "" && state.Mode != "proxy") || !sameFilePath(state.Database, m.databasePath) || !sameFilePath(state.KernelPath, m.kernelPath) || state.Path != oldPath || state.NodeHost != old.Host || state.Protocol != old.Protocol {
		return nil, errors.New("托管网站清单与当前节点不一致，请先执行 proxy-panel website setup 修复部署")
	}
	if !old.Enable || old.Port != 443 || old.KernelType != "xray" || (old.Protocol != "vless" && old.Protocol != "vmess") || old.Transport != "httpupgrade" || before["security"] != "tls" || before["listen"] != "127.0.0.1" {
		return nil, errors.New("托管网站需要启用的 Xray HTTPUpgrade + TLS 节点监听 127.0.0.1:443")
	}
	if text(before, "host") != state.Host || firstText(before, "sni", "serverName", old.Host) != state.SNI || !sameFilePath(firstText(before, "cert_path", "certPath", ""), state.Cert) || !sameFilePath(firstText(before, "key_path", "keyPath", ""), state.Key) {
		return nil, errors.New("托管网站的 Host、SNI 或证书与节点不一致，请先执行 proxy-panel website setup 修复部署")
	}
	if len(state.Domains) == 0 || len(state.Listens) == 0 {
		return nil, errors.New("托管网站清单缺少域名或监听地址")
	}
	config, err := m.managedConfig(oldPath)
	if err != nil {
		return nil, err
	}
	location := "location = \"" + oldPath + "\" {"
	nextConfig := bytes.Replace(config, []byte(location), []byte("location = \""+newPath+"\" {"), 1)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["path"], _ = json.Marshal(newPath)
	nextManifest, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	change := &Change{manager: m, state: *state, nodeID: old.ID, oldManifest: data, newManifest: nextManifest, oldConfig: config, newConfig: nextConfig, newPath: newPath}
	return change.preflight()
}

func (m *Manager) managedConfig(oldPath string) ([]byte, error) {
	for _, filename := range []string{m.manifestPath, m.configPath} {
		if err := regularFile(filename); err != nil {
			return nil, err
		}
	}
	config, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取托管 Nginx 配置失败: %w", err)
	}
	location := "location = \"" + oldPath + "\" {"
	if !routePath.MatchString(oldPath) || bytes.Count(config, []byte(location)) != 1 || bytes.Count(config, []byte("error_page 418 = @proxy_panel_node;")) != 1 || !bytes.Contains(config, []byte("location @proxy_panel_node {")) || !bytes.Contains(config, []byte("proxy_pass https://127.0.0.1:443;")) || bytes.Count(config, []byte("pid /run/proxy-panel-web.pid;")) != 1 {
		return nil, errors.New("托管 Nginx 配置已变化，无法定位唯一的原路径；请先执行 proxy-panel website setup")
	}
	return config, nil
}

func (c *Change) preflight() (*Change, error) {
	// nginx -t opens its PID file even without starting workers. Keep the candidate
	// PID in the writable managed directory, rather than requiring writable /run.
	stage, err := os.MkdirTemp(filepath.Dir(c.manager.configPath), ".path-check-")
	if err != nil {
		return nil, fmt.Errorf("无法写入托管网站目录，请升级管理脚本并执行 proxy-panel update 以更新服务权限: %w", err)
	}
	defer os.RemoveAll(stage)
	check := bytes.Replace(c.newConfig, []byte("pid /run/proxy-panel-web.pid;"), []byte("pid \""+filepath.Join(stage, "nginx.pid")+"\";"), 1)
	filename := filepath.Join(stage, "nginx.conf")
	if err := os.WriteFile(filename, check, 0600); err != nil {
		return nil, err
	}
	if err := command(context.Background(), "nginx", "-t", "-e", "stderr", "-c", filename); err != nil {
		return nil, fmt.Errorf("Nginx 配置预检失败: %w", err)
	}
	return c, nil
}

func sameWebsiteRoute(a, b model.Node) bool {
	if a.ID != b.ID || a.Host != b.Host || a.Port != b.Port ||
		a.Protocol != b.Protocol || a.Transport != b.Transport ||
		a.KernelType != b.KernelType || a.Enable != b.Enable {
		return false
	}
	if a.Settings == b.Settings {
		return true
	}
	var before, after map[string]any
	if json.Unmarshal([]byte(a.Settings), &before) != nil || json.Unmarshal([]byte(b.Settings), &after) != nil || before == nil || after == nil {
		return false
	}
	// A full form save normalizes aliases and adds client-only defaults. Compare
	// the values used by the website route rather than the JSON representation.
	security := func(settings map[string]any) string {
		if value := text(settings, "security"); value != "" {
			return value
		}
		if settings["tls"] == true {
			return "tls"
		}
		return "none"
	}
	if security(before) == "reality" && !strings.EqualFold(realitySNI(before), realitySNI(after)) {
		return false
	}
	if security(before) != security(after) || !reflect.DeepEqual(routeALPN(before), routeALPN(after)) ||
		firstText(before, "listen", "listen", "0.0.0.0") != firstText(after, "listen", "listen", "0.0.0.0") ||
		firstText(before, "path", "path", "/") != firstText(after, "path", "path", "/") ||
		text(before, "host") != text(after, "host") ||
		firstText(before, "sni", "serverName", a.Host) != firstText(after, "sni", "serverName", b.Host) {
		return false
	}
	for _, aliases := range [][2]string{{"cert_path", "certPath"}, {"key_path", "keyPath"}} {
		oldPath := firstText(before, aliases[0], aliases[1], "")
		newPath := firstText(after, aliases[0], aliases[1], "")
		if oldPath != newPath && !sameFilePath(oldPath, newPath) {
			return false
		}
	}
	return true
}

func routeALPN(settings map[string]any) []string {
	var result []string
	switch value := settings["alpn"].(type) {
	case []any:
		for _, part := range value {
			if text, ok := part.(string); ok {
				result = append(result, text)
			}
		}
	case string:
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func text(settings map[string]any, key string) string {
	result, _ := settings[key].(string)
	return result
}
func firstText(settings map[string]any, a, b, fallback string) string {
	if value := text(settings, a); value != "" {
		return value
	}
	if value := text(settings, b); value != "" {
		return value
	}
	return fallback
}
func sameFilePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	return err == nil && aa == bb
}

func (c *Change) CheckOld(ctx context.Context) error {
	return c.health(ctx, c.state.Path, nil)
}

func (c *Change) Apply(ctx context.Context) error {
	if err := c.CheckSnapshot(); err != nil {
		return err
	}
	c.changed = true
	if err := atomicWrite(c.manager.configPath, c.newConfig); err != nil {
		return err
	}
	if err := atomicWrite(c.manager.manifestPath, c.newManifest); err != nil {
		return err
	}
	if err := command(ctx, "systemctl", "reload", "proxy-panel-web.service"); err != nil {
		return err
	}
	return c.health(ctx, c.newPath, c.newReality)
}

func (c *Change) Rollback(ctx context.Context) error {
	if !c.changed {
		return nil
	}
	if err := errors.Join(atomicWrite(c.manager.configPath, c.oldConfig), atomicWrite(c.manager.manifestPath, c.oldManifest)); err != nil {
		return fmt.Errorf("恢复托管网站文件失败: %w", err)
	}
	if err := command(ctx, "systemctl", "reload", "proxy-panel-web.service"); err != nil {
		return fmt.Errorf("恢复托管 Nginx 失败: %w", err)
	}
	if err := c.health(ctx, c.state.Path, nil); err != nil {
		return fmt.Errorf("旧 HTTPUpgrade 路径恢复验证失败: %w", err)
	}
	c.changed = false
	return nil
}

func regularFile(filename string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("托管网站清单和 Nginx 配置必须是普通文件，不能覆盖符号链接")
	}
	return nil
}

func atomicWrite(filename string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(filename), ".path-sync-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filename); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func command(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		return fmt.Errorf("%s 失败: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (c *Change) health(ctx context.Context, route string, reality *realityProbe) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cert, err := os.ReadFile(c.state.CA)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert) {
		return errors.New("托管网站 CA 文件无效")
	}
	addresses := c.state.Listens[:1]
	if c.nodeID != c.state.NodeID {
		addresses = c.state.Listens
	}
	var realityRoots *x509.CertPool
	if reality != nil {
		// The website may use a private CA bundle while the Reality target
		// uses a public issuer. Accept both for the Reality handshake.
		realityRoots, err = x509.SystemCertPool()
		if err != nil {
			return err
		}
		realityRoots.AppendCertsFromPEM(cert)
	}
	var last error
	consecutive := 0
	for {
		last = nil
		for _, addr := range addresses {
			last = c.checkWebsite(ctx, addr, route, pool)
			if last != nil {
				break
			}
		}
		if last == nil && reality != nil {
			last = reality.check(ctx, realityRoots)
		}
		if last == nil {
			// A reload briefly overlaps worker generations. A single healthy
			// connection can hit a draining worker before the new route settles.
			consecutive++
			if consecutive >= 3 {
				return nil
			}
		} else {
			consecutive = 0
		}
		select {
		case <-ctx.Done():
			if last == nil {
				return fmt.Errorf("托管网站稳定性验证超时: %w", ctx.Err())
			}
			return fmt.Errorf("托管网站路径验证失败: %w", last)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (c *Change) checkWebsite(ctx context.Context, addr netip.AddrPort, route string, pool *x509.CertPool) error {
	for _, domain := range c.state.Domains {
		if err := probe(ctx, addr.String(), domain, "/", 200, pool); err != nil {
			return err
		}
		if err := probe(ctx, addr.String(), domain, route, 101, pool); err != nil {
			return err
		}
	}
	return nil
}

func probe(ctx context.Context, address, domain, route string, status int, pool *x509.CertPool) error {
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: time.Second}, Config: &tls.Config{ServerName: domain, RootCAs: pool, NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	req, err := http.NewRequest("GET", "https://"+domain+route, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Connection", "close")
	if status == 101 {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
	}
	if err := req.Write(conn); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return fmt.Errorf("%s %s 应返回 %d，实际 %d", domain, route, status, response.StatusCode)
	}
	return nil
}

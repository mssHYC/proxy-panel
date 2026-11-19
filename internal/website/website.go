// Package website updates only HTTPUpgrade paths in an installer-managed website.
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

type Manager struct{ manifestPath, configPath, databasePath, kernelPath string }

func New(manifestPath, configPath, databasePath, kernelPath string) *Manager {
	return &Manager{manifestPath, configPath, databasePath, kernelPath}
}

type manifest struct {
	Mode       string   `json:"mode"`
	NodeID     int64    `json:"node_id"`
	Database   string   `json:"database"`
	KernelPath string   `json:"kernel_path"`
	Path       string   `json:"path"`
	NodeHost   string   `json:"node_host"`
	Protocol   string   `json:"protocol"`
	Host       string   `json:"host"`
	SNI        string   `json:"sni"`
	Cert       string   `json:"cert"`
	Key        string   `json:"key"`
	Domains    []string `json:"domains"`
	CA         string   `json:"ca"`
	Listens    []string `json:"listens"`
}

// Change must be coordinated with the node database and Xray under the caller's lock.
// Rollback restores Nginx after the caller has restored the old Xray configuration.
type Change struct {
	manager                                        *Manager
	state                                          manifest
	oldManifest, newManifest, oldConfig, newConfig []byte
	newPath                                        string
	changed                                        bool
}

var routePath = regexp.MustCompile(`^/[A-Za-z0-9/_~.\-]*$`)

// Prepare is read-only except for a temporary nginx -t configuration, removed before return.
// Unmanaged sites and unrelated nodes do not participate in the synchronous update.
func (m *Manager) Prepare(old, candidate model.Node) (*Change, error) {
	data, err := os.ReadFile(m.manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取托管网站失败: %w", err)
	}
	var state manifest
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("读取托管网站清单失败: %w", err)
	}
	if state.Mode == "standalone" || state.NodeID != old.ID {
		return nil, nil
	}
	var before, after map[string]any
	if json.Unmarshal([]byte(old.Settings), &before) != nil || json.Unmarshal([]byte(candidate.Settings), &after) != nil || before == nil || after == nil {
		return nil, errors.New("托管节点 settings 必须是 JSON 对象")
	}
	if candidate.ID != old.ID || candidate.Host != old.Host || candidate.Port != old.Port || candidate.Protocol != old.Protocol || candidate.Transport != old.Transport || candidate.KernelType != old.KernelType || candidate.Enable != old.Enable {
		return nil, errors.New("托管网站依赖此节点的地址、端口、协议和启用状态；请先调整网站部署，面板当前仅自动同步路径")
	}
	for _, key := range []string{"security", "listen", "alpn"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return nil, fmt.Errorf("托管网站依赖节点的 %s 设置，请先调整网站部署；面板当前仅自动同步路径", key)
		}
	}
	if text(before, "host") != text(after, "host") {
		return nil, errors.New("托管网站依赖节点的 Host 设置，请先调整网站部署；面板当前仅自动同步路径")
	}
	// The panel normalizes legacy aliases when saving the complete settings form.
	// Compare their effective values, not the spelling of the stored JSON keys.
	if firstText(before, "sni", "serverName", old.Host) != firstText(after, "sni", "serverName", candidate.Host) ||
		!sameFilePath(firstText(before, "cert_path", "certPath", ""), firstText(after, "cert_path", "certPath", "")) ||
		!sameFilePath(firstText(before, "key_path", "keyPath", ""), firstText(after, "key_path", "keyPath", "")) {
		return nil, errors.New("托管网站依赖节点的 SNI 和证书设置，请先调整网站部署；面板当前仅自动同步路径")
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
	for _, address := range state.Listens {
		host, port, err := net.SplitHostPort(address)
		ip := net.ParseIP(host)
		if err != nil || port == "" || ip == nil || ip.IsUnspecified() {
			return nil, errors.New("托管网站监听地址无效")
		}
	}
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
	// nginx -t opens its PID file even without starting workers. Keep the candidate
	// PID in the writable managed directory, rather than requiring writable /run.
	stage, err := os.MkdirTemp(filepath.Dir(m.configPath), ".path-check-")
	if err != nil {
		return nil, fmt.Errorf("无法写入托管网站目录，请升级管理脚本并执行 proxy-panel update 以更新服务权限: %w", err)
	}
	defer os.RemoveAll(stage)
	check := bytes.Replace(nextConfig, []byte("pid /run/proxy-panel-web.pid;"), []byte("pid \""+filepath.Join(stage, "nginx.pid")+"\";"), 1)
	filename := filepath.Join(stage, "nginx.conf")
	if err := os.WriteFile(filename, check, 0600); err != nil {
		return nil, err
	}
	if err := command(context.Background(), "nginx", "-t", "-e", "stderr", "-c", filename); err != nil {
		return nil, fmt.Errorf("Nginx 路径预检失败: %w", err)
	}
	return &Change{manager: m, state: state, oldManifest: data, newManifest: nextManifest, oldConfig: config, newConfig: nextConfig, newPath: newPath}, nil
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

func (c *Change) CheckOld(ctx context.Context) error { return c.health(ctx, c.state.Path) }

func (c *Change) Apply(ctx context.Context) error {
	for filename, expected := range map[string][]byte{c.manager.manifestPath: c.oldManifest, c.manager.configPath: c.oldConfig} {
		if err := regularFile(filename); err != nil {
			return err
		}
		actual, err := os.ReadFile(filename)
		if err != nil || !bytes.Equal(actual, expected) {
			return errors.New("托管网站配置在预检后发生变化，请重试")
		}
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
	return c.health(ctx, c.newPath)
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
	if err := c.health(ctx, c.state.Path); err != nil {
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
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filename)
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

func (c *Change) health(ctx context.Context, route string) error {
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
	var last error
	for {
		last = nil
		for _, domain := range c.state.Domains {
			for _, check := range []struct {
				path   string
				status int
			}{{"/", 200}, {route, 101}} {
				if err := probe(ctx, c.state.Listens[0], domain, check.path, check.status, pool); err != nil {
					last = err
					break
				}
			}
			if last != nil {
				break
			}
		}
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("托管网站路径验证失败: %w", last)
		case <-time.After(100 * time.Millisecond):
		}
	}
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

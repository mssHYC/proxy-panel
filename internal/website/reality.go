package website

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
)

// These are the dynamic module locations supported by the installer. No module
// directive is needed on distributions which build stream into Nginx.
var streamModules = []string{"/usr/lib/nginx/modules/ngx_stream_module.so", "/usr/lib64/nginx/modules/ngx_stream_module.so"}

func canonicalSNI(s string) string { return strings.TrimSuffix(strings.ToLower(s), ".") }

func realityNode(n model.Node) bool {
	return n.Enable && n.KernelType == "xray" && n.Protocol == "vless" && n.Transport == "tcp"
}

func wantsRealityShare(s manifest, old, candidate model.Node) bool {
	if s.RealityFrontend != nil || old.Port == 443 || candidate.Port != 443 || !realityNode(old) || !realityNode(candidate) {
		return false
	}
	var settings map[string]any
	if json.Unmarshal([]byte(candidate.Settings), &settings) != nil || text(settings, "security") != "reality" {
		return false
	}
	listen := text(settings, "listen")
	if listen == "" {
		return true
	}
	addr, err := netip.ParseAddr(listen)
	return err == nil && (addr.Unmap().IsUnspecified() || slices.ContainsFunc(s.Listens, func(a netip.AddrPort) bool { return a.Addr().Unmap() == addr.Unmap() }))
}

func (m *Manager) prepareReality(state manifest, data []byte, old model.Node, candidate *model.Node) (*Change, error) {
	if old.ID != candidate.ID {
		return nil, fmt.Errorf("%w：共享入口节点 ID 不匹配", ErrManagedConflict)
	}
	var before, after map[string]any
	if json.Unmarshal([]byte(old.Settings), &before) != nil || json.Unmarshal([]byte(candidate.Settings), &after) != nil || before == nil || after == nil {
		return nil, fmt.Errorf("%w：共享节点的 settings 必须是 JSON 对象", ErrManagedConflict)
	}
	if len(state.Domains) == 0 || state.CA == "" || !sameFilePath(state.Database, m.databasePath) || !sameFilePath(state.KernelPath, m.kernelPath) {
		return nil, fmt.Errorf("%w：共享入口清单与当前网站部署不一致", ErrManagedConflict)
	}
	front := state.RealityFrontend
	if _, err := realityEndpoint(state, old, before); err != nil {
		return nil, err
	}
	next := state
	listen := text(after, "listen")
	if candidate.Port == 443 {
		sni := canonicalSNI(realitySNI(after))
		// Some single-label names are directives when used as Nginx map keys.
		if sni == "" || slices.Contains([]string{"default", "hostnames", "include", "volatile"}, sni) || slices.ContainsFunc(state.Domains, func(d string) bool { return canonicalSNI(d) == sni }) {
			return nil, fmt.Errorf("%w：Reality SNI 必须是有效的独立域名，且不能与网站域名重叠", ErrManagedConflict)
		}
		if front != nil {
			if listen != "" && listen != front.Listen.Addr().String() {
				return nil, fmt.Errorf("%w：共享 443 时 Reality 必须监听 %s", ErrManagedConflict, front.Listen.Addr())
			}
			updated := *front
			updated.SNI = sni
			next.RealityFrontend = &updated
		} else {
			_, names, err := kernel.RealityTarget(after)
			if err != nil || len(names) != 1 {
				return nil, fmt.Errorf("%w：新建共享入口需要唯一的 Reality SNI", ErrManagedConflict)
			}
			f := &realityFrontend{NodeID: old.ID, SNI: sni, Listen: netip.MustParseAddrPort("127.0.0.2:443"), WebListen: netip.MustParseAddrPort("127.0.0.1:9443"), PreviousPort: old.Port}
			if prev, ok := before["listen"].(string); ok {
				f.PreviousListen = &prev
			}
			for _, file := range streamModules {
				if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
					f.StreamModule = file
					break
				}
			}
			next.RealityFrontend = f
		}
		listen = next.RealityFrontend.Listen.Addr().String()
	} else {
		if listen == front.Listen.Addr().String() {
			listen = ""
			if front.PreviousListen != nil {
				listen = *front.PreviousListen
			} else if front.PreviousPort == 0 {
				// Older manifests never recorded the independent bind. Keep the
				// current public interface unless the form explicitly requests all.
				if len(state.Listens) != 1 {
					return nil, fmt.Errorf("%w：旧部署有多个网站监听地址，请填写独立节点的监听 IP，或清空以监听所有接口", ErrManagedConflict)
				}
				listen = state.Listens[0].Addr().String()
			}
		}
		next.RealityFrontend = nil
	}
	if listen != text(after, "listen") {
		// The port-only API also uses this path. Preserve opaque settings,
		// including large integers that cannot be round-tripped via float64.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(candidate.Settings), &raw); err != nil {
			return nil, err
		}
		if listen == "" {
			delete(raw, "listen")
		} else {
			raw["listen"], _ = json.Marshal(listen)
		}
		after["listen"] = listen
		settings, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		candidate.Settings = string(settings)
	}
	// Xray takes its new bind before Nginx reloads, so the old Nginx backend
	// remains occupied while detaching. Verify both configurations.
	if front != nil && next.RealityFrontend == nil {
		if err := state.checkListener(*candidate); err != nil {
			return nil, err
		}
	}
	if err := next.checkListener(*candidate); err != nil {
		return nil, err
	}
	newProbe, err := realityEndpoint(next, *candidate, after)
	if err != nil {
		return nil, err
	}
	if front != nil && next.RealityFrontend != nil && front.SNI == next.RealityFrontend.SNI {
		return nil, nil
	}
	config, err := m.managedConfig(state.Path)
	if err != nil {
		return nil, err
	}
	nextConfig, err := changeFrontendConfig(config, state.Listens, front, next.RealityFrontend)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	// Early installers omitted mode from proxy manifests. A shared frontend
	// uses the current installer schema even when upgrading such a site.
	fields["mode"] = json.RawMessage(`"proxy"`)
	switch {
	case next.RealityFrontend == nil:
		delete(fields, "reality_frontend")
	case front == nil:
		fields["reality_frontend"], err = json.Marshal(next.RealityFrontend)
	default:
		// Keep installer extensions and rollback information byte-for-value.
		var f map[string]json.RawMessage
		if err := json.Unmarshal(fields["reality_frontend"], &f); err != nil {
			return nil, err
		}
		f["sni"], _ = json.Marshal(next.RealityFrontend.SNI)
		fields["reality_frontend"], err = json.Marshal(f)
	}
	if err != nil {
		return nil, err
	}
	nextData, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	change := &Change{manager: m, state: state, nodeID: old.ID, oldManifest: data, newManifest: nextData, oldConfig: config, newConfig: nextConfig, newPath: state.Path, newReality: newProbe}
	return change.preflight()
}

type realityProbe struct {
	addresses []netip.AddrPort
	sni       string
}

// A normal, certificate-verified TLS handshake exercises the same SNI branch
// as the installer's health check, including the new independent bind.
func realityEndpoint(state manifest, node model.Node, settings map[string]any) (*realityProbe, error) {
	sni := canonicalSNI(realitySNI(settings))
	if !realityNode(node) || text(settings, "security") != "reality" || sni == "" || node.Port < 1 || node.Port > 65535 {
		return nil, fmt.Errorf("%w：Reality 节点协议、端口或 SNI 无效", ErrManagedConflict)
	}
	if f := state.RealityFrontend; f != nil {
		if f.NodeID != node.ID || f.Listen != netip.MustParseAddrPort("127.0.0.2:443") || f.WebListen != netip.MustParseAddrPort("127.0.0.1:9443") || f.SNI != sni || text(settings, "listen") != f.Listen.Addr().String() || node.Port != 443 || len(state.Listens) == 0 {
			return nil, fmt.Errorf("%w：Reality 节点与网站清单不一致", ErrManagedConflict)
		}
		return &realityProbe{state.Listens, sni}, nil
	}
	bind, err := netip.ParseAddr(firstText(settings, "listen", "listen", "0.0.0.0"))
	if err != nil || node.Port == 443 {
		return nil, fmt.Errorf("%w：独立 Reality 的端口或监听地址无效", ErrManagedConflict)
	}
	bind = bind.Unmap()
	if bind.IsUnspecified() {
		if bind.Is4() {
			bind = netip.MustParseAddr("127.0.0.1")
		} else {
			bind = netip.IPv6Loopback()
		}
	}
	return &realityProbe{[]netip.AddrPort{netip.AddrPortFrom(bind, uint16(node.Port))}, sni}, nil
}

func (p *realityProbe) check(ctx context.Context, pool *x509.CertPool) error {
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 2 * time.Second}, Config: &tls.Config{ServerName: p.sni, RootCAs: pool, MinVersion: tls.VersionTLS12}}
	for _, addr := range p.addresses {
		conn, err := dialer.DialContext(ctx, "tcp", addr.String())
		if err != nil {
			return fmt.Errorf("Reality TLS %s (%s): %w", p.sni, addr, err)
		}
		conn.Close()
	}
	return nil
}

func listenLines(addrs []netip.AddrPort, options string) string {
	var lines strings.Builder
	for _, addr := range addrs {
		fmt.Fprintf(&lines, "        listen %s%s;\n", addr, options)
	}
	return lines.String()
}

// Match the installer's complete stream block and listener lists before editing
// so hand-written maps or extra routes cannot be silently discarded.
func changeFrontendConfig(data []byte, public []netip.AddrPort, old, next *realityFrontend) ([]byte, error) {
	type layout struct {
		module, stream string
		http           []netip.AddrPort
	}
	from := func(f *realityFrontend) layout {
		if f == nil {
			return layout{http: public}
		}
		section := layout{http: []netip.AddrPort{f.WebListen}}
		if f.StreamModule != "" {
			section.module = fmt.Sprintf("load_module \"%s\";\n", f.StreamModule)
		}
		section.stream = fmt.Sprintf(`stream {
    map $ssl_preread_server_name $proxy_panel_backend {
        %s %s;
        default %s;
    }
    server {
%s        ssl_preread on;
        proxy_pass $proxy_panel_backend;
        proxy_timeout 3600s;
    }
}
`, f.SNI, f.Listen, f.WebListen, listenLines(public, ""))
		return section
	}
	before, after := from(old), from(next)
	config := string(data)
	const pid = "pid /run/proxy-panel-web.pid;\n"
	if !strings.HasPrefix(config, before.module+pid) || strings.Count(config, "\nstream {\n") != strings.Count(before.stream, "stream {") || strings.Count(config, "\nhttp {\n") != 1 {
		return nil, fmt.Errorf("%w：Nginx 共享入口已变化，无法定位原配置", ErrManagedConflict)
	}
	config = after.module + strings.TrimPrefix(config, before.module)
	for _, parts := range [][2]string{
		{"\n" + before.stream + "http {\n", "\n" + after.stream + "http {\n"},
		{"\n" + listenLines(before.http, " ssl default_server"), "\n" + listenLines(after.http, " ssl default_server")},
		{"\n" + listenLines(before.http, " ssl"), "\n" + listenLines(after.http, " ssl")},
	} {
		if strings.Count(config, parts[0]) != 1 {
			return nil, fmt.Errorf("%w：Nginx 共享入口或网站监听配置与清单不符", ErrManagedConflict)
		}
		config = strings.Replace(config, parts[0], parts[1], 1)
	}
	return []byte(config), nil
}

package website

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"

	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
)

// ErrManagedConflict means the requested node change would break an installed website.
var ErrManagedConflict = errors.New("节点与托管网站冲突")

type realityFrontend struct {
	NodeID    int64          `json:"node_id"`
	SNI       string         `json:"sni"`
	Listen    netip.AddrPort `json:"listen"`
	WebListen netip.AddrPort `json:"web_listen"`
}

func (m *Manager) readManifest() (*manifest, []byte, error) {
	data, err := os.ReadFile(m.manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取托管网站失败: %w", err)
	}
	var state manifest
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil, fmt.Errorf("读取托管网站清单失败: %w", err)
	}
	switch state.Mode {
	case "standalone":
	case "", "proxy": // The first installer manifests omitted mode.
		if state.NodeID <= 0 {
			return nil, nil, errors.New("托管网站清单缺少绑定节点")
		}
	default:
		return nil, nil, errors.New("未知托管网站模式")
	}
	if len(state.Listens) == 0 {
		return nil, nil, errors.New("托管网站清单缺少监听地址")
	}
	for _, addr := range state.Listens {
		if !addr.IsValid() || addr.Port() == 0 || addr.Addr().Unmap().IsUnspecified() {
			return nil, nil, errors.New("托管网站监听地址无效")
		}
	}
	if f := state.RealityFrontend; f != nil && (state.Mode != "proxy" || f.NodeID <= 0 || f.NodeID == state.NodeID || f.SNI == "" || f.Listen != netip.MustParseAddrPort("127.0.0.2:443") || f.WebListen != netip.MustParseAddrPort("127.0.0.1:9443")) {
		return nil, nil, errors.New("托管网站共享 Reality 入口无效")
	}
	return &state, data, nil
}

// UsesPublicPort reports whether the firewall must keep a port open for Nginx.
func (m *Manager) UsesPublicPort(port int) (bool, error) {
	state, _, err := m.readManifest()
	if state == nil || err != nil {
		return false, err
	}
	for _, addr := range state.Listens {
		if int(addr.Port()) == port {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) CheckCreate(node model.Node) error {
	state, _, err := m.readManifest()
	if state == nil || err != nil {
		return err
	}
	return state.checkListener(node)
}

func (m *Manager) CheckDelete(id int64) error {
	state, _, err := m.readManifest()
	if state == nil || err != nil {
		return err
	}
	if state.Mode != "standalone" && (id == state.NodeID || (state.RealityFrontend != nil && id == state.RealityFrontend.NodeID)) {
		return fmt.Errorf("%w：节点 %d 仍是网站入口，请先迁移或解除网站绑定", ErrManagedConflict, id)
	}
	return nil
}

func realitySNI(settings map[string]any) string {
	_, names, err := kernel.RealityTarget(settings)
	if err != nil {
		return ""
	}
	return firstText(settings, "sni", "serverName", names[0])
}

func (s *manifest) checkListener(n model.Node) error {
	front := s.RealityFrontend
	ownsReality := front != nil && n.ID == front.NodeID
	udpOnly := n.Protocol == "hysteria2" || n.Protocol == "hy2" || n.Protocol == "tuic" ||
		(n.KernelType == "xray" && (n.Transport == "kcp" || n.Transport == "quic"))
	if !ownsReality && (!n.Enable || udpOnly) {
		return nil
	}
	reserved := append([]netip.AddrPort(nil), s.Listens...)
	if s.Mode != "standalone" && n.ID != s.NodeID {
		reserved = append(reserved, netip.MustParseAddrPort("127.0.0.1:443"))
	}
	if front != nil {
		reserved = append(reserved, front.WebListen)
		if !ownsReality {
			reserved = append(reserved, front.Listen)
		}
	}
	if !ownsReality && !slices.ContainsFunc(reserved, func(a netip.AddrPort) bool { return int(a.Port()) == n.Port }) {
		return nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(n.Settings), &settings); err != nil {
		return fmt.Errorf("读取节点 settings 失败: %w", err)
	}
	// Both engines bind to a wildcard when listen is empty; an IPv6 wildcard
	// can also claim IPv4 listeners on a dual-stack host.
	bind := text(settings, "listen")
	ip := netip.IPv4Unspecified()
	if bind != "" {
		var err error
		ip, err = netip.ParseAddr(bind)
		if err != nil {
			return fmt.Errorf("节点监听地址无效: %w", err)
		}
	}
	if ownsReality {
		if !n.Enable || n.Protocol != "vless" || n.Transport != "tcp" || n.KernelType != "xray" || text(settings, "security") != "reality" ||
			ip != front.Listen.Addr() || n.Port != int(front.Listen.Port()) || !strings.EqualFold(realitySNI(settings), front.SNI) {
			return fmt.Errorf("%w：网站依赖此 Reality 节点的协议、启用状态、监听地址、端口和 SNI；请先调整网站部署", ErrManagedConflict)
		}
	}
	for _, addr := range reserved {
		if int(addr.Port()) == n.Port && (ip.Unmap().IsUnspecified() || addr.Addr().Unmap() == ip.Unmap()) {
			return fmt.Errorf("%w：监听地址 %s 与网站使用的 %s 重叠，请更换节点地址或端口", ErrManagedConflict, ip, addr)
		}
	}
	return nil
}

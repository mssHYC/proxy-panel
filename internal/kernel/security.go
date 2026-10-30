package kernel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Block proxy users from reaching management services, cloud metadata and the
// server's own addresses. A local website fallback bypasses outbound routing.
func blockedDestinations() []string {
	cidrs := []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4", "::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8"}
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		cidrs = append(cidrs, ip.String()+"/"+strconv.Itoa(bits))
	}
	return cidrs
}

func nodeSecurity(s map[string]interface{}) string {
	security := getSettingStr(s, "security", "")
	if security == "" && getSettingBool(s, "tls", false) {
		return "tls"
	}
	if security == "" {
		return "none"
	}
	return security
}

func tlsALPN(s map[string]interface{}) []string {
	if alpn := getSettingSlice(s, "alpn"); len(alpn) > 0 {
		return alpn
	}
	if value := getSettingStr(s, "alpn", ""); value != "" {
		var alpn []string
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				alpn = append(alpn, part)
			}
		}
		if len(alpn) > 0 {
			return alpn
		}
	}
	if getSettingInt(s, "fallback_port", 0) > 0 {
		return []string{"http/1.1"}
	}
	return nil
}

// ValidateNodeSecurity rejects combinations that would silently lose protection
// or expose a different listener from the one selected in the panel.
func ValidateNodeSecurity(node NodeConfig, engine string) error {
	if v, ok := node.Settings["listen"]; ok {
		address, ok := v.(string)
		if !ok || (address != "" && net.ParseIP(address) == nil) {
			return fmt.Errorf("listen 必须是有效的 IP 地址")
		}
	}
	security := nodeSecurity(node.Settings)
	if value, ok := node.Settings["public_port"]; ok {
		port := getSettingInt(node.Settings, "public_port", -1)
		if n, ok := value.(float64); ok && n != float64(port) {
			return fmt.Errorf("订阅对外端口必须是整数")
		}
		if port != 443 || engine != "xray" || node.Protocol != "vmess" ||
			(node.Transport != "ws" && node.Transport != "httpupgrade") || security != "tls" ||
			getSettingStr(node.Settings, "listen", "") != "127.0.0.1" {
			return fmt.Errorf("订阅对外端口仅支持托管到 443 的 Xray VMess WS/HTTPUpgrade + TLS，后端须监听 127.0.0.1")
		}
	}
	if security == "reality" {
		if _, _, err := RealityTarget(node.Settings); err != nil {
			return err
		}
	}
	tcp := node.Transport == "" || node.Transport == "tcp" || node.Transport == "raw"
	if flow := getSettingStr(node.Settings, "flow", ""); flow != "" {
		if flow != "xtls-rprx-vision" || node.Protocol != "vless" || !tcp || (security != "tls" && security != "reality") {
			return fmt.Errorf("Vision 仅适用于 VLESS TCP + TLS/Reality")
		}
	}
	if value, ok := node.Settings["fallback_port"]; ok {
		port := getSettingInt(node.Settings, "fallback_port", -1)
		// JSON numbers must be integral; do not silently truncate a destination.
		if n, ok := value.(float64); ok && n != float64(port) {
			return fmt.Errorf("网站回落端口必须是整数")
		}
		if port < 1 || port > 65535 || port == node.Port {
			return fmt.Errorf("网站回落端口必须为 1–65535 且不能与节点端口相同")
		}
		if engine != "xray" || !tcp || security != "tls" || (node.Protocol != "vless" && node.Protocol != "trojan") {
			return fmt.Errorf("网站回落仅支持 Xray VLESS/Trojan TCP + TLS")
		}
		for _, alpn := range tlsALPN(node.Settings) {
			if alpn != "http/1.1" {
				return fmt.Errorf("本机静态网站回落仅支持 ALPN http/1.1")
			}
		}
	}
	return nil
}

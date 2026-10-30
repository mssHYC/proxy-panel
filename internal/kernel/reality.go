package kernel

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseRealityTarget accepts a host with an optional port, including IPv6.
// URLs, Unix sockets and private destinations are not public camouflage sites.
func ParseRealityTarget(dest string) (string, int, error) {
	host, port := strings.TrimSpace(dest), "443"
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.Trim(host, "[]")
	} else if net.ParseIP(host) == nil && strings.Contains(host, ":") {
		var err error
		host, port, err = net.SplitHostPort(host)
		if err != nil {
			return "", 0, fmt.Errorf("Reality 目标应为域名或 IP:端口")
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || !validTargetHost(host) {
		return "", 0, fmt.Errorf("Reality 目标地址或端口无效")
	}
	if ip := net.ParseIP(host); ip != nil && !PublicRealityIP(ip) {
		return "", 0, fmt.Errorf("Reality 目标必须是公网地址，不能指向本机或内网")
	}
	return host, n, nil
}

func validTargetHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// PublicRealityIP also excludes the server's own public interfaces.
func PublicRealityIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, cidr := range append(blockedDestinations(), "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32") {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return false
		}
	}
	return true
}

func RealityTarget(s map[string]interface{}) (string, []string, error) {
	host, port, err := ParseRealityTarget(getSettingStr(s, "dest", ""))
	if err != nil {
		return "", nil, err
	}
	names := getSettingSliceAny(s, "server_names", "serverNames")
	if len(names) == 0 {
		names = []string{getSettingStrAny(s, "", "sni", "serverName")}
	}
	if len(names) > 8 {
		return "", nil, fmt.Errorf("Reality 最多允许 8 个 SNI")
	}
	for _, name := range names {
		if !validTargetHost(name) || net.ParseIP(name) != nil {
			return "", nil, fmt.Errorf("Reality SNI 必须是有效域名，不支持通配符或 IP")
		}
	}
	if sni := getSettingStrAny(s, "", "sni", "serverName"); sni != "" {
		found := false
		for _, name := range names {
			if strings.EqualFold(sni, name) {
				found = true
			}
		}
		if !found {
			return "", nil, fmt.Errorf("Reality 客户端 SNI 必须包含在 Server Names 中")
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), names, nil
}

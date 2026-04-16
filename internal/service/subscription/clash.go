package subscription

import (
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

// ClashGenerator Clash/Mihomo YAML 格式订阅生成器
type ClashGenerator struct{}

func (g *ClashGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var b strings.Builder

	// 全局配置
	b.WriteString("port: 7890\n")
	b.WriteString("socks-port: 7891\n")
	b.WriteString("allow-lan: false\n")
	b.WriteString("mode: rule\n")
	b.WriteString("log-level: info\n\n")

	// DNS
	b.WriteString("dns:\n")
	b.WriteString("  enable: true\n")
	b.WriteString("  nameserver:\n")
	b.WriteString("    - 223.5.5.5\n")
	b.WriteString("    - 119.29.29.29\n\n")

	// proxies
	b.WriteString("proxies:\n")
	var proxyNames []string
	for _, node := range nodes {
		proxy := g.buildProxy(node, user)
		if proxy != "" {
			b.WriteString(proxy)
			proxyNames = append(proxyNames, node.Name)
		}
	}
	b.WriteString("\n")

	// proxy-groups
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: Proxy\n")
	b.WriteString("    type: select\n")
	b.WriteString("    proxies:\n")
	for _, name := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", name))
	}
	b.WriteString("      - DIRECT\n")

	b.WriteString("  - name: Auto\n")
	b.WriteString("    type: url-test\n")
	b.WriteString("    url: http://www.gstatic.com/generate_204\n")
	b.WriteString("    interval: 300\n")
	b.WriteString("    proxies:\n")
	for _, name := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", name))
	}
	b.WriteString("\n")

	// rules
	b.WriteString("rules:\n")
	b.WriteString("  - GEOIP,CN,DIRECT\n")
	b.WriteString("  - MATCH,Proxy\n")

	return b.String(), "text/yaml; charset=utf-8", nil
}

func (g *ClashGenerator) buildProxy(node model.Node, user *model.User) string {
	s := parseSettings(node)
	var b strings.Builder

	switch node.Protocol {
	case "vless":
		b.WriteString(fmt.Sprintf("  - name: %s\n", node.Name))
		b.WriteString("    type: vless\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    uuid: %s\n", user.UUID))
		if s.Flow != "" {
			b.WriteString(fmt.Sprintf("    flow: %s\n", s.Flow))
		}
		if node.Transport != "" && node.Transport != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", node.Transport))
		}
		// Reality 配置
		if s.Security == "reality" {
			b.WriteString("    tls: true\n")
			b.WriteString(fmt.Sprintf("    servername: %s\n", s.SNI))
			b.WriteString("    reality-opts:\n")
			b.WriteString(fmt.Sprintf("      public-key: %s\n", s.PublicKey))
			if s.ShortID != "" {
				b.WriteString(fmt.Sprintf("      short-id: %s\n", s.ShortID))
			}
			if s.Fingerprint != "" {
				b.WriteString(fmt.Sprintf("    client-fingerprint: %s\n", s.Fingerprint))
			}
		} else if s.TLS {
			b.WriteString("    tls: true\n")
			if s.SNI != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", s.SNI))
			}
		}
		g.writeTransportOpts(&b, node, s)

	case "vmess":
		b.WriteString(fmt.Sprintf("  - name: %s\n", node.Name))
		b.WriteString("    type: vmess\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    uuid: %s\n", user.UUID))
		b.WriteString("    alterId: 0\n")
		b.WriteString("    cipher: auto\n")
		if node.Transport != "" && node.Transport != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", node.Transport))
		}
		if s.TLS {
			b.WriteString("    tls: true\n")
			if s.SNI != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", s.SNI))
			}
		}
		g.writeTransportOpts(&b, node, s)

	case "trojan":
		b.WriteString(fmt.Sprintf("  - name: %s\n", node.Name))
		b.WriteString("    type: trojan\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    password: %s\n", user.UUID))
		if s.SNI != "" {
			b.WriteString(fmt.Sprintf("    sni: %s\n", s.SNI))
		}
		if s.AllowInsecure {
			b.WriteString("    skip-cert-verify: true\n")
		}
		if node.Transport != "" && node.Transport != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", node.Transport))
		}
		g.writeTransportOpts(&b, node, s)

	case "ss":
		method := s.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		b.WriteString(fmt.Sprintf("  - name: %s\n", node.Name))
		b.WriteString("    type: ss\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    cipher: %s\n", method))
		b.WriteString(fmt.Sprintf("    password: %s\n", password))

	case "hysteria2":
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		b.WriteString(fmt.Sprintf("  - name: %s\n", node.Name))
		b.WriteString("    type: hysteria2\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    password: %s\n", password))
		if s.SNI != "" {
			b.WriteString(fmt.Sprintf("    sni: %s\n", s.SNI))
		}
		if s.AllowInsecure {
			b.WriteString("    skip-cert-verify: true\n")
		}
		if s.Obfs != "" {
			b.WriteString(fmt.Sprintf("    obfs: %s\n", s.Obfs))
			if s.ObfsPassword != "" {
				b.WriteString(fmt.Sprintf("    obfs-password: %s\n", s.ObfsPassword))
			}
		}

	default:
		return ""
	}

	return b.String()
}

// writeTransportOpts 写入传输层配置 (ws/grpc)
func (g *ClashGenerator) writeTransportOpts(b *strings.Builder, node model.Node, s nodeSettings) {
	switch node.Transport {
	case "ws":
		b.WriteString("    ws-opts:\n")
		if s.Path != "" {
			b.WriteString(fmt.Sprintf("      path: %s\n", s.Path))
		}
		if s.Host != "" {
			b.WriteString("      headers:\n")
			b.WriteString(fmt.Sprintf("        Host: %s\n", s.Host))
		}
	case "grpc":
		if s.ServiceName != "" {
			b.WriteString("    grpc-opts:\n")
			b.WriteString(fmt.Sprintf("      grpc-service-name: %s\n", s.ServiceName))
		}
	}
}

package subscription

import (
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

// SurgeGenerator Surge 格式订阅生成器
type SurgeGenerator struct{}

func (g *SurgeGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var b strings.Builder

	// 托管配置头
	b.WriteString(fmt.Sprintf("#!MANAGED-CONFIG %s/api/sub/%s?format=surge interval=86400 strict=false\n\n", baseURL, user.UUID))

	// [General]
	b.WriteString("[General]\n")
	b.WriteString("loglevel = notify\n")
	b.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local\n")
	b.WriteString("dns-server = 223.5.5.5, 119.29.29.29, system\n\n")

	// [Proxy]
	b.WriteString("[Proxy]\n")
	b.WriteString("DIRECT = direct\n")

	var proxyNames []string
	for _, node := range nodes {
		line := g.buildProxyLine(node, user)
		if line != "" {
			b.WriteString(line + "\n")
			proxyNames = append(proxyNames, node.Name)
		}
	}
	b.WriteString("\n")

	// [Proxy Group]
	b.WriteString("[Proxy Group]\n")
	allNames := strings.Join(proxyNames, ", ")
	b.WriteString(fmt.Sprintf("Proxy = select, %s, DIRECT\n", allNames))
	if len(proxyNames) > 0 {
		b.WriteString(fmt.Sprintf("Auto = url-test, %s, url=http://www.gstatic.com/generate_204, interval=300\n", allNames))
	}
	b.WriteString("\n")

	// [Rule]
	b.WriteString("[Rule]\n")
	b.WriteString("GEOIP,CN,DIRECT\n")
	b.WriteString("FINAL,Proxy\n")

	return b.String(), "text/plain; charset=utf-8", nil
}

func (g *SurgeGenerator) buildProxyLine(node model.Node, user *model.User) string {
	s := parseSettings(node)
	port := fmt.Sprintf("%d", node.Port)

	switch node.Protocol {
	case "vmess":
		parts := []string{
			fmt.Sprintf("%s = vmess, %s, %s, username=%s", node.Name, node.Host, port, user.UUID),
		}
		if s.TLS {
			parts = append(parts, "tls=true")
			if s.SNI != "" {
				parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
			}
		}
		if node.Transport == "ws" {
			parts = append(parts, "ws=true")
			if s.Path != "" {
				parts = append(parts, fmt.Sprintf("ws-path=%s", s.Path))
			}
			if s.Host != "" {
				parts = append(parts, fmt.Sprintf("ws-headers=Host:%s", s.Host))
			}
		}
		return strings.Join(parts, ", ")

	case "trojan":
		parts := []string{
			fmt.Sprintf("%s = trojan, %s, %s, password=%s", node.Name, node.Host, port, user.UUID),
		}
		if s.SNI != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
		}
		return strings.Join(parts, ", ")

	case "ss":
		method := s.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		return fmt.Sprintf("%s = ss, %s, %s, encrypt-method=%s, password=%s", node.Name, node.Host, port, method, password)

	case "hysteria2":
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		parts := []string{
			fmt.Sprintf("%s = hysteria2, %s, %s, password=%s", node.Name, node.Host, port, password),
		}
		if s.SNI != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
		}
		return strings.Join(parts, ", ")

	case "vless":
		return fmt.Sprintf("# %s = VLESS (Surge 不原生支持)", node.Name)

	default:
		return ""
	}
}

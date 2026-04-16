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

	// 收集节点名
	var proxyNames []string
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Name)
	}

	// === 全局配置 ===
	b.WriteString(`log-level: info
mode: rule
ipv6: true
mixed-port: 7890
allow-lan: true
bind-address: "*"
find-process-mode: strict
external-controller: 0.0.0.0:9090
global-client-fingerprint: chrome

geox-url:
  geoip: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat"
  geosite: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
  mmdb: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb"
geo-auto-update: true
geo-update-interval: 24

profile:
  store-selected: true
  store-fake-ip: true

sniffer:
  enable: true
  override-destination: false
  sniff:
    QUIC:
      ports: [443]
    TLS:
      ports: [443]
    HTTP:
      ports: [80]

dns:
  enable: true
  prefer-h3: false
  listen: 0.0.0.0:1053
  ipv6: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - "*.lan"
    - "*.local"
    - "dns.google"
    - "localhost.ptlogin2.qq.com"
  use-hosts: true
  nameserver:
    - https://1.1.1.1/dns-query
    - https://8.8.8.8/dns-query
  proxy-server-nameserver:
    - https://223.5.5.5/dns-query
    - https://1.12.12.12/dns-query
  nameserver-policy:
    "geosite:cn,private":
      - https://doh.pub/dns-query
      - https://dns.alidns.com/dns-query

`)

	// === proxies ===
	b.WriteString("proxies:\n")
	for _, node := range nodes {
		proxy := g.buildProxy(node, user)
		if proxy != "" {
			b.WriteString(proxy)
		}
	}
	b.WriteString("\n")

	// === proxy-groups ===
	b.WriteString("proxy-groups:\n")

	// 手动切换
	b.WriteString("  - name: 手动切换\n    type: select\n    proxies:\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 自动选择
	b.WriteString("  - name: 自动选择\n    type: url-test\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n    tolerance: 50\n    proxies:\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 全球代理
	b.WriteString("  - name: 全球代理\n    type: select\n    proxies:\n      - 手动切换\n      - 自动选择\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 流媒体
	b.WriteString("  - name: 流媒体\n    type: select\n    proxies:\n      - 手动切换\n      - 自动选择\n      - DIRECT\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// DNS_Proxy
	b.WriteString("  - name: DNS_Proxy\n    type: select\n    proxies:\n      - 自动选择\n      - 手动切换\n      - DIRECT\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 各服务组 (引用手动切换+自动选择)
	for _, svc := range []string{"Telegram", "Google", "YouTube", "Bing", "OpenAI", "ClaudeAI", "GitHub"} {
		b.WriteString(fmt.Sprintf("  - name: %s\n    type: select\n    proxies:\n      - 手动切换\n      - 自动选择\n", svc))
		for _, n := range proxyNames {
			b.WriteString(fmt.Sprintf("      - %s\n", n))
		}
		if svc == "Google" || svc == "GitHub" {
			b.WriteString("      - DIRECT\n")
		}
	}

	// 流媒体子组
	for _, svc := range []string{"Netflix", "HBO", "Disney"} {
		b.WriteString(fmt.Sprintf("  - name: %s\n    type: select\n    proxies:\n      - 流媒体\n      - 手动切换\n      - 自动选择\n", svc))
		for _, n := range proxyNames {
			b.WriteString(fmt.Sprintf("      - %s\n", n))
		}
	}

	// Spotify (含 DIRECT)
	b.WriteString("  - name: Spotify\n    type: select\n    proxies:\n      - 流媒体\n      - 手动切换\n      - 自动选择\n      - DIRECT\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 国内媒体
	b.WriteString("  - name: 国内媒体\n    type: select\n    proxies:\n      - DIRECT\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 本地直连
	b.WriteString("  - name: 本地直连\n    type: select\n    proxies:\n      - DIRECT\n      - 自动选择\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}

	// 漏网之鱼
	b.WriteString("  - name: 漏网之鱼\n    type: select\n    proxies:\n      - DIRECT\n      - 手动切换\n      - 自动选择\n")
	for _, n := range proxyNames {
		b.WriteString(fmt.Sprintf("      - %s\n", n))
	}
	b.WriteString("\n")

	// === rule-providers (override 模式下跳过) ===
	if !IsOverrideMode() {
	b.WriteString(`rule-providers:
  lan:
    type: http
    behavior: classical
    interval: 86400
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Lan/Lan.yaml
    path: ./Rules/lan.yaml
  reject:
    type: http
    behavior: domain
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/reject.txt
    path: ./ruleset/reject.yaml
    interval: 86400
  proxy:
    type: http
    behavior: domain
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/proxy.txt
    path: ./ruleset/proxy.yaml
    interval: 86400
  direct:
    type: http
    behavior: domain
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/direct.txt
    path: ./ruleset/direct.yaml
    interval: 86400
  private:
    type: http
    behavior: domain
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/private.txt
    path: ./ruleset/private.yaml
    interval: 86400
  gfw:
    type: http
    behavior: domain
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/gfw.txt
    path: ./ruleset/gfw.yaml
    interval: 86400
  telegramcidr:
    type: http
    behavior: ipcidr
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/telegramcidr.txt
    path: ./ruleset/telegramcidr.yaml
    interval: 86400
  applications:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/applications.txt
    path: ./ruleset/applications.yaml
    interval: 86400
  Disney:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Disney/Disney.yaml
    path: ./ruleset/disney.yaml
    interval: 86400
  Netflix:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Netflix/Netflix.yaml
    path: ./ruleset/netflix.yaml
    interval: 86400
  YouTube:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/YouTube/YouTube.yaml
    path: ./ruleset/youtube.yaml
    interval: 86400
  HBO:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/HBO/HBO.yaml
    path: ./ruleset/hbo.yaml
    interval: 86400
  OpenAI:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/OpenAI/OpenAI.yaml
    path: ./ruleset/openai.yaml
    interval: 86400
  ClaudeAI:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Claude/Claude.yaml
    path: ./ruleset/claudeai.yaml
    interval: 86400
  Bing:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Bing/Bing.yaml
    path: ./ruleset/bing.yaml
    interval: 86400
  Google:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml
    path: ./ruleset/google.yaml
    interval: 86400
  GitHub:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/GitHub/GitHub.yaml
    path: ./ruleset/github.yaml
    interval: 86400
  Spotify:
    type: http
    behavior: classical
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Spotify/Spotify.yaml
    path: ./ruleset/spotify.yaml
    interval: 86400
  ChinaMaxDomain:
    type: http
    behavior: domain
    interval: 86400
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Domain.yaml
    path: ./Rules/ChinaMaxDomain.yaml
  ChinaMaxIPNoIPv6:
    type: http
    behavior: ipcidr
    interval: 86400
    url: https://gh-proxy.com/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_IP_No_IPv6.yaml
    path: ./Rules/ChinaMaxIPNoIPv6.yaml

`)
	} // end if !IsOverrideMode() for rule-providers

	// === rules ===
	b.WriteString("rules:\n")
	if IsOverrideMode() {
		// 完全使用自定义规则
		for _, rule := range customRules {
			rule = strings.TrimSpace(rule)
			if rule != "" && !strings.HasPrefix(rule, "#") {
				b.WriteString(fmt.Sprintf("  - %s\n", rule))
			}
		}
	} else {
		// 自定义规则优先，然后是默认规则
		for _, rule := range customRules {
			rule = strings.TrimSpace(rule)
			if rule != "" && !strings.HasPrefix(rule, "#") {
				b.WriteString(fmt.Sprintf("  - %s\n", rule))
			}
		}
		b.WriteString(`  - RULE-SET,YouTube,YouTube,no-resolve
  - RULE-SET,Google,Google,no-resolve
  - RULE-SET,GitHub,GitHub
  - RULE-SET,telegramcidr,Telegram,no-resolve
  - RULE-SET,Spotify,Spotify,no-resolve
  - RULE-SET,Netflix,Netflix
  - RULE-SET,HBO,HBO
  - RULE-SET,Bing,Bing
  - RULE-SET,OpenAI,OpenAI
  - RULE-SET,ClaudeAI,ClaudeAI
  - RULE-SET,Disney,Disney
  - RULE-SET,proxy,全球代理
  - RULE-SET,gfw,全球代理
  - RULE-SET,applications,本地直连
  - RULE-SET,ChinaMaxDomain,本地直连
  - RULE-SET,ChinaMaxIPNoIPv6,本地直连,no-resolve
  - RULE-SET,lan,本地直连,no-resolve
  - GEOIP,CN,本地直连
  - MATCH,漏网之鱼
`)
	}

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

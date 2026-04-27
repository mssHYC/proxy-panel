package subscription

import (
	"fmt"
	"sort"
	"strings"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// ClashGenerator Clash/Mihomo YAML 格式订阅生成器
type ClashGenerator struct{}

// Generate 旧入口不再使用；需要通过 GenerateWithPlan 消费 routing.Plan。
func (g *ClashGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	return "", "", fmt.Errorf("clash generator requires routing plan; use GenerateWithPlan")
}

// GenerateWithPlan 基于预构建的 routing.Plan 渲染 Clash 订阅。
func (g *ClashGenerator) GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL, token string) (string, string, error) {
	var b strings.Builder

	// 收集节点名
	var proxyNames []string
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Name)
	}

	// === 全局配置 ===
	b.WriteString(clashGlobalPreamble)

	// === proxies ===
	b.WriteString("proxies:\n")
	for _, node := range nodes {
		proxy := g.buildProxy(node, user)
		if proxy != "" {
			b.WriteString(proxy)
		}
	}
	b.WriteString("\n")

	providers, groups, rules := renderClashRoutingFromPlan(plan, proxyNames)

	// === rule-providers ===
	if len(providers) > 0 {
		b.WriteString("rule-providers:\n")
		// 稳定输出
		keys := make([]string, 0, len(providers))
		for k := range providers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := providers[k].(map[string]any)
			b.WriteString(fmt.Sprintf("  %q:\n", k))
			b.WriteString(fmt.Sprintf("    type: %s\n", p["type"]))
			b.WriteString(fmt.Sprintf("    behavior: %s\n", p["behavior"]))
			b.WriteString(fmt.Sprintf("    format: %s\n", p["format"]))
			b.WriteString(fmt.Sprintf("    url: %q\n", p["url"]))
			b.WriteString(fmt.Sprintf("    interval: %d\n", p["interval"]))
			b.WriteString(fmt.Sprintf("    path: %q\n", p["path"]))
		}
		b.WriteString("\n")
	}

	// === proxy-groups ===
	b.WriteString("proxy-groups:\n")
	for _, group := range groups {
		b.WriteString(fmt.Sprintf("  - name: %q\n", group["name"]))
		b.WriteString(fmt.Sprintf("    type: %s\n", clashGroupType(group["type"])))
		if url, ok := group["url"].(string); ok {
			b.WriteString(fmt.Sprintf("    url: %q\n", url))
		}
		if interval, ok := group["interval"].(int); ok {
			b.WriteString(fmt.Sprintf("    interval: %d\n", interval))
		}
		b.WriteString("    proxies:\n")
		for _, m := range group["proxies"].([]string) {
			b.WriteString(fmt.Sprintf("      - %q\n", m))
		}
	}
	b.WriteString("\n")

	// === rules ===
	b.WriteString("rules:\n")
	for _, r := range rules {
		b.WriteString(fmt.Sprintf("  - %q\n", r))
	}

	return b.String(), "text/plain; charset=utf-8", nil
}

// renderClashRoutingFromPlan 基于 Plan + 节点全名列表生成 3 个 YAML 子结构。
func renderClashRoutingFromPlan(plan *routing.Plan, allNodeNames []string) (
	providers map[string]any,
	groups []map[string]any,
	rules []string,
) {
	providers = map[string]any{}
	for tag, urls := range plan.Providers.Site {
		providers[tag] = map[string]any{
			"type": "http", "behavior": "domain", "format": "mrs",
			"url": urls.Clash, "interval": 86400,
			"path": "./rule_provider/site/" + tag + ".mrs",
		}
	}
	for tag, urls := range plan.Providers.IP {
		providers[tag+"-ip"] = map[string]any{
			"type": "http", "behavior": "ipcidr", "format": "mrs",
			"url": urls.Clash, "interval": 86400,
			"path": "./rule_provider/ip/" + tag + ".mrs",
		}
	}

	codeToName := map[string]string{}
	for _, g := range plan.Groups {
		codeToName[g.Code] = g.DisplayName
	}

	for _, g := range plan.Groups {
		members := []string{}
		for _, m := range g.Members {
			switch {
			case m == "<ALL>":
				members = append(members, allNodeNames...)
			case routing.IsLiteralOutbound(m):
				members = append(members, m)
			default:
				if n, ok := codeToName[m]; ok {
					members = append(members, n)
				} else {
					members = append(members, m)
				}
			}
		}
		group := map[string]any{
			"name":    g.DisplayName,
			"type":    g.Type,
			"proxies": members,
		}
		if g.Type == "urltest" {
			group["url"] = "http://www.gstatic.com/generate_204"
			group["interval"] = 300
		}
		groups = append(groups, group)
	}

	for _, r := range plan.Rules {
		out := clashOutboundName(r.Outbound, codeToName)
		for _, t := range r.SiteTags {
			rules = append(rules, fmt.Sprintf("RULE-SET,%s,%s", t, out))
		}
		for _, t := range r.IPTags {
			rules = append(rules, fmt.Sprintf("RULE-SET,%s-ip,%s,no-resolve", t, out))
		}
		for _, v := range r.DomainSuffix {
			rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", v, out))
		}
		for _, v := range r.DomainKeyword {
			rules = append(rules, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", v, out))
		}
		for _, v := range r.IPCIDR {
			rules = append(rules, fmt.Sprintf("IP-CIDR,%s,%s,no-resolve", v, out))
		}
		for _, v := range r.SrcIPCIDR {
			rules = append(rules, fmt.Sprintf("SRC-IP-CIDR,%s,%s", v, out))
		}
	}
	rules = append(rules, fmt.Sprintf("MATCH,%s", clashOutboundName(plan.Final, codeToName)))
	return
}

// clashGroupType 把 IR 中的 group type（对齐 sing-box 词汇）映射到 Clash/Mihomo 的命名。
// IR: selector / urltest   →   Clash: select / url-test
func clashGroupType(t any) string {
	s, _ := t.(string)
	switch s {
	case "selector":
		return "select"
	case "urltest":
		return "url-test"
	}
	return s
}

func clashOutboundName(codeOrLiteral string, codeToName map[string]string) string {
	if routing.IsLiteralOutbound(codeOrLiteral) {
		return codeOrLiteral
	}
	if n, ok := codeToName[codeOrLiteral]; ok {
		return n
	}
	return codeOrLiteral
}

const clashGlobalPreamble = `log-level: info
mode: rule
ipv6: true
mixed-port: 7890
allow-lan: true
bind-address: "*"
find-process-mode: strict
external-controller: 0.0.0.0:9090

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
    - "+.lan"
    - "+.local"
    - "+.market.xiaomi.com"
    - "dns.google"
    - "localhost.ptlogin2.qq.com"
    - "+.msftncsi.com"
    - "+.msftconnecttest.com"
    - "+.windowsupdate.com"
  use-hosts: true
  # default-nameserver 用纯 IP，仅用来引导 DoH 主机名的初次解析
  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29
    - 1.0.0.1
  # 主 nameserver：国内 DoH（用来解析国内域名 + 跑 fallback-filter 判定）
  nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  # fallback：境外 DoH，与 nameserver 并发查询；fallback-filter 决定何时用 fallback 结果
  fallback:
    - https://1.1.1.1/dns-query
    - https://8.8.8.8/dns-query
    - tls://8.8.4.4:853
  # fallback-filter：当 nameserver 返回值不是 CN IP 或命中污染 IP 时，
  # 改用 fallback（境外 DoH）的结果。这样国内域名走国内、国外域名走国外。
  fallback-filter:
    geoip: true
    geoip-code: CN
    geosite:
      - gfw
    ipcidr:
      - 240.0.0.0/4
      - 0.0.0.0/32
    domain:
      - "+.google.com"
      - "+.facebook.com"
      - "+.youtube.com"
      - "+.twitter.com"
      - "+.instagram.com"
      - "+.githubusercontent.com"
  # 解析代理服务器自身主机名一律走国内 DNS（避免循环依赖）
  proxy-server-nameserver:
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query

`

func (g *ClashGenerator) buildProxy(node model.Node, user *model.User) string {
	s := parseSettings(node)
	var b strings.Builder

	switch node.Protocol {
	case "vless":
		b.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
		b.WriteString("    type: vless\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    uuid: %s\n", user.UUID))
		if s.Flow != "" {
			b.WriteString(fmt.Sprintf("    flow: %s\n", s.Flow))
		}
		if n := clashNetworkName(node.Transport); n != "" && n != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", n))
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
		b.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
		b.WriteString("    type: vmess\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    uuid: %s\n", user.UUID))
		b.WriteString("    alterId: 0\n")
		b.WriteString("    cipher: auto\n")
		if n := clashNetworkName(node.Transport); n != "" && n != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", n))
		}
		if s.TLS {
			b.WriteString("    tls: true\n")
			if s.SNI != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", s.SNI))
			}
		}
		g.writeTransportOpts(&b, node, s)

	case "trojan":
		b.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
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
		if n := clashNetworkName(node.Transport); n != "" && n != "tcp" {
			b.WriteString(fmt.Sprintf("    network: %s\n", n))
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
		b.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
		b.WriteString("    type: ss\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    cipher: %s\n", method))
		b.WriteString(fmt.Sprintf("    password: %s\n", password))

	case "hysteria2":
		// sing-box 服务端固定以 user.UUID 作为 hy2 password，订阅必须与之一致
		b.WriteString(fmt.Sprintf("  - name: %q\n", node.Name))
		b.WriteString("    type: hysteria2\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", node.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", node.Port))
		b.WriteString(fmt.Sprintf("    password: %s\n", user.UUID))
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

// clashNetworkName 将内部 transport 映射为 Clash YAML 的 network 字段值。
// 原生 `network: httpupgrade` 较新，Clash Verge 等客户端支持不完整，
// 统一输出为 `network: ws` + `ws-opts.v2ray-http-upgrade: true` 的兼容写法。
func clashNetworkName(transport string) string {
	if transport == "httpupgrade" {
		return "ws"
	}
	return transport
}

// writeTransportOpts 写入传输层配置 (ws/grpc/httpupgrade)
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
	case "httpupgrade":
		// 兼容写法：ws + v2ray-http-upgrade: true。mihomo/Clash Verge 的 ws-opts
		// 同一结构承载 path/Host，再加 v2ray-http-upgrade 字段切换为 HTTPUpgrade 语义。
		b.WriteString("    ws-opts:\n")
		path := s.Path
		if path == "" {
			path = "/"
		}
		b.WriteString(fmt.Sprintf("      path: %s\n", path))
		b.WriteString("      v2ray-http-upgrade: true\n")
		host := s.Host
		if host == "" {
			host = node.Host
		}
		b.WriteString("      headers:\n")
		b.WriteString(fmt.Sprintf("        Host: %s\n", host))
	}
}

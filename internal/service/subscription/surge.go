package subscription

import (
	"fmt"
	"strings"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// SurgeGenerator Surge 格式订阅生成器
type SurgeGenerator struct{}

// Generate 旧入口不再使用；需要通过 GenerateWithPlan 消费 routing.Plan。
func (g *SurgeGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	return "", "", fmt.Errorf("surge generator requires routing plan; use GenerateWithPlan")
}

// GenerateWithPlan 基于预构建的 routing.Plan 渲染 Surge 订阅。
func (g *SurgeGenerator) GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL, token string) (string, string, error) {
	var b strings.Builder

	// 托管配置头
	b.WriteString(fmt.Sprintf("#!MANAGED-CONFIG %s/api/sub/t/%s?format=surge interval=86400 strict=false\n\n", baseURL, token))

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
		if line == "" {
			continue
		}
		b.WriteString(line + "\n")
		// 注释行（如 VLESS 不被 Surge 原生支持）不算可用节点，避免 Proxy Group 引用不存在的 Proxy
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			proxyNames = append(proxyNames, node.Name)
		}
	}
	b.WriteString("\n")

	allNodeNames := proxyNames
	if len(allNodeNames) == 0 {
		allNodeNames = []string{"DIRECT"}
	}

	proxyGroups, rules := renderSurgeRoutingFromPlan(plan, allNodeNames)

	// [Proxy Group]
	b.WriteString("[Proxy Group]\n")
	for _, line := range proxyGroups {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")

	// [Rule]
	b.WriteString("[Rule]\n")
	for _, r := range rules {
		b.WriteString(r + "\n")
	}

	return b.String(), "text/plain; charset=utf-8", nil
}

// renderSurgeRoutingFromPlan produces Surge [Proxy Group] lines and [Rule] lines.
// allNodeNames is the list of node names for <ALL> macro expansion.
func renderSurgeRoutingFromPlan(plan *routing.Plan, allNodeNames []string) (proxyGroups []string, rules []string) {
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
			case m == "DIRECT" || m == "REJECT":
				members = append(members, m)
			default:
				if n, ok := codeToName[m]; ok {
					members = append(members, n)
				} else {
					members = append(members, m)
				}
			}
		}
		proxyGroups = append(proxyGroups,
			fmt.Sprintf("%s = %s, %s", g.DisplayName, g.Type, strings.Join(members, ", ")))
	}

	for _, r := range plan.Rules {
		out := surgeOutbound(r.Outbound, codeToName)
		for _, t := range r.SiteTags {
			if plan.SurgeSiteBase != "" {
				rules = append(rules, fmt.Sprintf("DOMAIN-SET,%s%s.list,%s", plan.SurgeSiteBase, t, out))
			} else {
				rules = append(rules, fmt.Sprintf("GEOSITE,%s,%s", t, out))
			}
		}
		for _, t := range r.IPTags {
			rules = append(rules, fmt.Sprintf("GEOIP,%s,%s", t, out))
		}
		for _, v := range r.DomainSuffix {
			rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", v, out))
		}
		for _, v := range r.DomainKeyword {
			rules = append(rules, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", v, out))
		}
		for _, v := range r.IPCIDR {
			rules = append(rules, fmt.Sprintf("IP-CIDR,%s,%s", v, out))
		}
		for _, v := range r.SrcIPCIDR {
			rules = append(rules, fmt.Sprintf("SRC-IP-CIDR,%s,%s", v, out))
		}
	}
	rules = append(rules, fmt.Sprintf("FINAL,%s", surgeOutbound(plan.Final, codeToName)))
	return
}

func surgeOutbound(codeOrLit string, codeToName map[string]string) string {
	if codeOrLit == "DIRECT" || codeOrLit == "REJECT" {
		return codeOrLit
	}
	if n, ok := codeToName[codeOrLit]; ok {
		return n
	}
	return codeOrLit
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
		// sing-box 服务端固定以 user.UUID 作为 hy2 password，订阅必须与之一致
		parts := []string{
			fmt.Sprintf("%s = hysteria2, %s, %s, password=%s", node.Name, node.Host, port, user.UUID),
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

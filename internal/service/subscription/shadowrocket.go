package subscription

import (
	"encoding/base64"
	"fmt"
	"strings"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// ShadowrocketGenerator Shadowrocket 格式订阅生成器
// 输出 base64-URI 节点列表 + Surge 风格规则段（Shadowrocket 同时识别）
type ShadowrocketGenerator struct{}

// Generate 旧入口不再使用；需要通过 GenerateWithPlan 消费 routing.Plan。
func (g *ShadowrocketGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	return "", "", fmt.Errorf("shadowrocket generator requires routing plan; use GenerateWithPlan")
}

// GenerateWithPlan 基于预构建的 routing.Plan 渲染 Shadowrocket 订阅。
func (g *ShadowrocketGenerator) GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	uris := buildV2RayURIs(nodes, user)

	var allNodeNames []string
	for _, n := range nodes {
		allNodeNames = append(allNodeNames, n.Name)
	}
	if len(allNodeNames) == 0 {
		allNodeNames = []string{"DIRECT"}
	}

	proxyGroups, rules := renderShadowrocketRoutingFromPlan(plan, allNodeNames)

	var b strings.Builder
	b.WriteString(strings.Join(uris, "\n"))
	b.WriteString("\n\n[Proxy Group]\n")
	for _, line := range proxyGroups {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Rule]\n")
	for _, r := range rules {
		b.WriteString(r + "\n")
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(b.String()))
	return encoded, "text/plain; charset=utf-8", nil
}

func renderShadowrocketRoutingFromPlan(plan *routing.Plan, allNodeNames []string) (proxyGroups []string, rules []string) {
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
		out := shadowrocketOutbound(r.Outbound, codeToName)
		for _, t := range r.SiteTags {
			if plan.SurgeSiteBase != "" {
				rules = append(rules, fmt.Sprintf("RULE-SET,%s%s.list,%s", plan.SurgeSiteBase, t, out))
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
	}
	rules = append(rules, fmt.Sprintf("FINAL,%s", shadowrocketOutbound(plan.Final, codeToName)))
	return
}

func shadowrocketOutbound(codeOrLit string, codeToName map[string]string) string {
	if codeOrLit == "DIRECT" || codeOrLit == "REJECT" {
		return codeOrLit
	}
	if n, ok := codeToName[codeOrLit]; ok {
		return n
	}
	return codeOrLit
}

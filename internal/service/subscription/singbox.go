package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// SingboxGenerator Sing-box JSON 格式订阅生成器
type SingboxGenerator struct{}

// Generate 旧入口不再使用；需要通过 GenerateWithPlan 消费 routing.Plan。
func (g *SingboxGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	return "", "", fmt.Errorf("singbox generator requires routing plan; use GenerateWithPlan")
}

// GenerateWithPlan 基于预构建的 routing.Plan 渲染 Sing-box 订阅。
func (g *SingboxGenerator) GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL, token string) (string, string, error) {
	var nodeOutbounds []map[string]interface{}
	var nodeTags []string

	// 生成每个节点的 outbound
	for _, node := range nodes {
		ob := g.buildOutbound(node, user)
		if ob != nil {
			nodeOutbounds = append(nodeOutbounds, ob)
			nodeTags = append(nodeTags, node.Name)
		}
	}

	// 基于 Plan 渲染路由部分
	ruleSets, groupOutbounds, rules, final := renderSingboxRoutingFromPlan(plan, nodeTags)

	// 系统 outbounds（不再注入 dns-out，让客户端使用系统默认 DNS）
	directOb := map[string]interface{}{"type": "direct", "tag": "direct"}
	blockOb := map[string]interface{}{"type": "block", "tag": "block"}

	// 组装完整 outbounds：代理组 + 节点 + 系统
	allOutbounds := make([]map[string]interface{}, 0, len(groupOutbounds)+len(nodeOutbounds)+2)
	allOutbounds = append(allOutbounds, groupOutbounds...)
	allOutbounds = append(allOutbounds, nodeOutbounds...)
	allOutbounds = append(allOutbounds, directOb, blockOb)

	route := map[string]interface{}{
		"rules":                 rules,
		"final":                 final,
		"auto_detect_interface": true,
	}
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}

	config := map[string]interface{}{
		"outbounds": allOutbounds,
		"route":     route,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}

	return string(data), "text/plain; charset=utf-8", nil
}

// renderSingboxRoutingFromPlan 返回 route.rule_set 条目、代理组 outbounds、route.rules 以及 final 出站 tag。
// 节点 tag 通过 allNodeTags 传入。
func renderSingboxRoutingFromPlan(plan *routing.Plan, allNodeTags []string) (
	ruleSets []map[string]any,
	groupOutbounds []map[string]any,
	rules []map[string]any,
	final string,
) {
	for tag, urls := range plan.Providers.Site {
		ruleSets = append(ruleSets, map[string]any{
			"tag": tag, "type": "remote", "format": "binary",
			"url": urls.Singbox, "download_detour": "direct",
		})
	}
	for tag, urls := range plan.Providers.IP {
		ruleSets = append(ruleSets, map[string]any{
			"tag": tag + "-ip", "type": "remote", "format": "binary",
			"url": urls.Singbox, "download_detour": "direct",
		})
	}

	// group code -> displayed tag (singbox "tag" 字段)。
	codeToTag := map[string]string{}
	for _, g := range plan.Groups {
		codeToTag[g.Code] = g.DisplayName
	}

	for _, g := range plan.Groups {
		members := []string{}
		for _, m := range g.Members {
			switch {
			case m == "<ALL>":
				members = append(members, allNodeTags...)
			case m == "DIRECT":
				members = append(members, "direct")
			case m == "REJECT":
				members = append(members, "block")
			default:
				if t, ok := codeToTag[m]; ok {
					members = append(members, t)
				} else {
					members = append(members, m)
				}
			}
		}
		groupOutbounds = append(groupOutbounds, map[string]any{
			"tag":       g.DisplayName,
			"type":      g.Type,
			"outbounds": members,
		})
	}

	for _, r := range plan.Rules {
		out := singboxOutboundName(r.Outbound, codeToTag)
		rule := map[string]any{"outbound": out}
		if len(r.SiteTags) > 0 {
			merged := append([]string{}, r.SiteTags...)
			for _, t := range r.IPTags {
				merged = append(merged, t+"-ip")
			}
			rule["rule_set"] = merged
		} else if len(r.IPTags) > 0 {
			merged := []string{}
			for _, t := range r.IPTags {
				merged = append(merged, t+"-ip")
			}
			rule["rule_set"] = merged
		}
		if len(r.DomainSuffix) > 0 {
			rule["domain_suffix"] = r.DomainSuffix
		}
		if len(r.DomainKeyword) > 0 {
			rule["domain_keyword"] = r.DomainKeyword
		}
		if len(r.IPCIDR) > 0 {
			rule["ip_cidr"] = r.IPCIDR
		}
		if len(r.SrcIPCIDR) > 0 {
			rule["source_ip_cidr"] = r.SrcIPCIDR
		}
		rules = append(rules, rule)
	}

	final = singboxOutboundName(plan.Final, codeToTag)
	return
}

func singboxOutboundName(codeOrLit string, codeToTag map[string]string) string {
	switch codeOrLit {
	case "DIRECT":
		return "direct"
	case "REJECT":
		return "block"
	}
	if t, ok := codeToTag[codeOrLit]; ok {
		return t
	}
	return codeOrLit
}

func (g *SingboxGenerator) buildOutbound(node model.Node, user *model.User) map[string]interface{} {
	s := parseSettings(node)

	switch node.Protocol {
	case "vless":
		return g.buildVLESS(node, user, s)
	case "vmess":
		return g.buildVMess(node, user, s)
	case "trojan":
		return g.buildTrojan(node, user, s)
	case "ss":
		return g.buildShadowsocks(node, user, s)
	case "hysteria2":
		return g.buildHysteria2(node, user, s)
	default:
		return nil
	}
}

func (g *SingboxGenerator) buildVLESS(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	ob := map[string]interface{}{
		"type":        "vless",
		"tag":         node.Name,
		"server":      node.Host,
		"server_port": node.Port,
		"uuid":        user.UUID,
	}
	if s.Flow != "" {
		ob["flow"] = s.Flow
	}

	// TLS 配置
	if s.Security == "reality" {
		ob["tls"] = map[string]interface{}{
			"enabled":     true,
			"server_name": s.SNI,
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": s.PublicKey,
				"short_id":   s.ShortID,
			},
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": g.getFingerprint(s),
			},
		}
	} else if s.TLS {
		tls := map[string]interface{}{
			"enabled": true,
		}
		if s.SNI != "" {
			tls["server_name"] = s.SNI
		}
		if s.AllowInsecure {
			tls["insecure"] = true
		}
		if s.ALPN != "" {
			tls["alpn"] = splitALPN(s.ALPN)
		}
		ob["tls"] = tls
	}

	g.setTransport(ob, node, s)
	return ob
}

func (g *SingboxGenerator) buildVMess(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	ob := map[string]interface{}{
		"type":        "vmess",
		"tag":         node.Name,
		"server":      node.Host,
		"server_port": node.Port,
		"uuid":        user.UUID,
		"alter_id":    0,
		"security":    "auto",
	}

	if s.TLS {
		tls := map[string]interface{}{
			"enabled": true,
		}
		if s.SNI != "" {
			tls["server_name"] = s.SNI
		}
		if s.AllowInsecure {
			tls["insecure"] = true
		}
		if s.ALPN != "" {
			tls["alpn"] = splitALPN(s.ALPN)
		}
		ob["tls"] = tls
	}

	g.setTransport(ob, node, s)
	return ob
}

func (g *SingboxGenerator) buildTrojan(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	ob := map[string]interface{}{
		"type":        "trojan",
		"tag":         node.Name,
		"server":      node.Host,
		"server_port": node.Port,
		"password":    user.UUID,
	}

	tls := map[string]interface{}{
		"enabled": true,
	}
	if s.SNI != "" {
		tls["server_name"] = s.SNI
	}
	if s.AllowInsecure {
		tls["insecure"] = true
	}
	if s.ALPN != "" {
		tls["alpn"] = splitALPN(s.ALPN)
	}
	ob["tls"] = tls

	g.setTransport(ob, node, s)
	return ob
}

func (g *SingboxGenerator) buildShadowsocks(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	method := s.Method
	if method == "" {
		method = "aes-256-gcm"
	}
	password := s.Password
	if password == "" {
		password = user.UUID
	}
	return map[string]interface{}{
		"type":        "shadowsocks",
		"tag":         node.Name,
		"server":      node.Host,
		"server_port": node.Port,
		"method":      method,
		"password":    password,
	}
}

func (g *SingboxGenerator) buildHysteria2(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	// sing-box 服务端固定以 user.UUID 作为 hy2 password，订阅必须与之一致
	ob := map[string]interface{}{
		"type":        "hysteria2",
		"tag":         node.Name,
		"server":      node.Host,
		"server_port": node.Port,
		"password":    user.UUID,
	}

	tls := map[string]interface{}{
		"enabled": true,
	}
	if s.SNI != "" {
		tls["server_name"] = s.SNI
	}
	if s.AllowInsecure {
		tls["insecure"] = true
	}
	if s.ALPN != "" {
		tls["alpn"] = splitALPN(s.ALPN)
	}
	ob["tls"] = tls

	if s.Obfs != "" {
		ob["obfs"] = map[string]interface{}{
			"type":     s.Obfs,
			"password": s.ObfsPassword,
		}
	}
	// 客户端带宽上报（Hysteria2 拥塞算法依据），0 视为未配置
	if s.UpMbps > 0 {
		ob["up_mbps"] = s.UpMbps
	}
	if s.DownMbps > 0 {
		ob["down_mbps"] = s.DownMbps
	}
	return ob
}

// splitALPN 把逗号分隔的 ALPN 字符串拆成数组，兼容前端既可存字符串也可存数组的情况
func splitALPN(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// setTransport 设置传输层配置
func (g *SingboxGenerator) setTransport(ob map[string]interface{}, node model.Node, s nodeSettings) {
	switch node.Transport {
	case "ws":
		transport := map[string]interface{}{
			"type": "ws",
		}
		if s.Path != "" {
			transport["path"] = s.Path
		}
		if s.Host != "" {
			transport["headers"] = map[string]interface{}{
				"Host": s.Host,
			}
		}
		ob["transport"] = transport
	case "grpc":
		transport := map[string]interface{}{
			"type": "grpc",
		}
		if s.ServiceName != "" {
			transport["service_name"] = s.ServiceName
		}
		ob["transport"] = transport
	case "httpupgrade":
		transport := map[string]interface{}{
			"type": "httpupgrade",
		}
		if s.Path != "" {
			transport["path"] = s.Path
		}
		if s.Host != "" {
			transport["host"] = s.Host
		}
		ob["transport"] = transport
	}
}

func (g *SingboxGenerator) getFingerprint(s nodeSettings) string {
	if s.Fingerprint != "" {
		return s.Fingerprint
	}
	fp := strings.ToLower(s.Fingerprint)
	if fp == "" {
		return "chrome"
	}
	return fp
}

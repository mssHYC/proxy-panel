package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

// SingboxGenerator Sing-box JSON 格式订阅生成器
type SingboxGenerator struct{}

func (g *SingboxGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var nodeOutbounds []map[string]interface{}
	var nodeNames []string

	// 生成每个节点的 outbound
	for _, node := range nodes {
		ob := g.buildOutbound(node, user)
		if ob != nil {
			nodeOutbounds = append(nodeOutbounds, ob)
			nodeNames = append(nodeNames, node.Name)
		}
	}

	// 构建代理组 outbounds
	groupOutbounds := g.buildProxyGroups(nodeNames)

	// 系统 outbounds
	directOb := map[string]interface{}{"type": "direct", "tag": "direct"}
	blockOb := map[string]interface{}{"type": "block", "tag": "block"}
	dnsOb := map[string]interface{}{"type": "dns", "tag": "dns-out"}

	// 组装完整 outbounds：代理组 + 节点 + 系统
	allOutbounds := make([]map[string]interface{}, 0, len(groupOutbounds)+len(nodeOutbounds)+3)
	allOutbounds = append(allOutbounds, groupOutbounds...)
	allOutbounds = append(allOutbounds, nodeOutbounds...)
	allOutbounds = append(allOutbounds, directOb, blockOb, dnsOb)

	// DNS 配置
	dns := map[string]interface{}{
		"servers": []map[string]interface{}{
			{"tag": "dns-remote", "address": "https://1.1.1.1/dns-query", "detour": "全球代理"},
			{"tag": "dns-direct", "address": "https://223.5.5.5/dns-query", "detour": "direct"},
			{"tag": "dns-block", "address": "rcode://success"},
		},
		"rules": []map[string]interface{}{
			{"geosite": []string{"cn"}, "server": "dns-direct"},
			{"geosite": []string{"category-ads-all"}, "server": "dns-block", "disable_cache": true},
		},
	}

	// 路由规则
	routeRules := g.buildRouteRules()

	route := map[string]interface{}{
		"rules":                routeRules,
		"final":                "漏网之鱼",
		"auto_detect_interface": true,
	}

	config := map[string]interface{}{
		"dns":       dns,
		"outbounds": allOutbounds,
		"route":     route,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}

	return string(data), "text/plain; charset=utf-8", nil
}

// buildProxyGroups 构建所有代理组
func (g *SingboxGenerator) buildProxyGroups(nodeNames []string) []map[string]interface{} {
	groups := []map[string]interface{}{
		// 手动切换
		{
			"type":      "selector",
			"tag":       "手动切换",
			"outbounds": copyNames(nodeNames),
		},
		// 自动选择
		{
			"type":      "urltest",
			"tag":       "自动选择",
			"outbounds": copyNames(nodeNames),
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  "5m",
		},
		// 全球代理
		{
			"type":      "selector",
			"tag":       "全球代理",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// 流媒体
		{
			"type":      "selector",
			"tag":       "流媒体",
			"outbounds": concat([]string{"手动切换", "自动选择", "direct"}, nodeNames),
		},
		// Telegram
		{
			"type":      "selector",
			"tag":       "Telegram",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// Google
		{
			"type":      "selector",
			"tag":       "Google",
			"outbounds": concat3([]string{"手动切换", "自动选择"}, nodeNames, []string{"direct"}),
		},
		// YouTube
		{
			"type":      "selector",
			"tag":       "YouTube",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// Netflix
		{
			"type":      "selector",
			"tag":       "Netflix",
			"outbounds": concat([]string{"流媒体", "手动切换", "自动选择"}, nodeNames),
		},
		// Spotify
		{
			"type":      "selector",
			"tag":       "Spotify",
			"outbounds": concat3([]string{"流媒体", "手动切换", "自动选择"}, []string{"direct"}, nodeNames),
		},
		// HBO
		{
			"type":      "selector",
			"tag":       "HBO",
			"outbounds": concat([]string{"流媒体", "手动切换", "自动选择"}, nodeNames),
		},
		// Disney
		{
			"type":      "selector",
			"tag":       "Disney",
			"outbounds": concat([]string{"流媒体", "手动切换", "自动选择"}, nodeNames),
		},
		// Bing
		{
			"type":      "selector",
			"tag":       "Bing",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// OpenAI
		{
			"type":      "selector",
			"tag":       "OpenAI",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// ClaudeAI
		{
			"type":      "selector",
			"tag":       "ClaudeAI",
			"outbounds": concat([]string{"手动切换", "自动选择"}, nodeNames),
		},
		// GitHub
		{
			"type":      "selector",
			"tag":       "GitHub",
			"outbounds": concat3([]string{"手动切换", "自动选择"}, nodeNames, []string{"direct"}),
		},
		// 国内媒体
		{
			"type":      "selector",
			"tag":       "国内媒体",
			"outbounds": concat([]string{"direct"}, nodeNames),
		},
		// 本地直连
		{
			"type":      "selector",
			"tag":       "本地直连",
			"outbounds": concat([]string{"direct", "自动选择"}, nodeNames),
		},
		// 漏网之鱼
		{
			"type":      "selector",
			"tag":       "漏网之鱼",
			"outbounds": concat([]string{"direct", "手动切换", "自动选择"}, nodeNames),
		},
	}
	return groups
}

// buildRouteRules 构建路由规则
func (g *SingboxGenerator) buildRouteRules() []map[string]interface{} {
	var rules []map[string]interface{}

	// 自定义规则
	customRules := GetCustomRules()
	for _, rule := range customRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		// 尝试解析自定义规则为 sing-box 格式
		r := parseSingboxCustomRule(rule)
		if r != nil {
			rules = append(rules, r)
		}
	}

	// DNS 劫持
	rules = append(rules, map[string]interface{}{
		"protocol": "dns",
		"outbound": "dns-out",
	})

	// override 模式下跳过默认规则
	if !IsOverrideMode() {
		defaultRules := []struct {
			geosite  []string
			geoip    []string
			outbound string
		}{
			{geosite: []string{"youtube"}, outbound: "YouTube"},
			{geosite: []string{"google"}, outbound: "Google"},
			{geosite: []string{"github"}, outbound: "GitHub"},
			{geoip: []string{"telegram"}, outbound: "Telegram"},
			{geosite: []string{"telegram"}, outbound: "Telegram"},
			{geosite: []string{"spotify"}, outbound: "Spotify"},
			{geosite: []string{"netflix"}, outbound: "Netflix"},
			{geosite: []string{"hbo"}, outbound: "HBO"},
			{geosite: []string{"bing"}, outbound: "Bing"},
			{geosite: []string{"openai"}, outbound: "OpenAI"},
			{geosite: []string{"disney"}, outbound: "Disney"},
			{geosite: []string{"geolocation-!cn"}, outbound: "全球代理"},
			{geoip: []string{"cn"}, outbound: "本地直连"},
			{geosite: []string{"cn"}, outbound: "本地直连"},
		}

		for _, dr := range defaultRules {
			r := map[string]interface{}{
				"outbound": dr.outbound,
			}
			if len(dr.geosite) > 0 {
				r["geosite"] = dr.geosite
			}
			if len(dr.geoip) > 0 {
				r["geoip"] = dr.geoip
			}
			rules = append(rules, r)
		}
	}

	return rules
}

// parseSingboxCustomRule 尝试将自定义规则转换为 sing-box 路由规则
// 支持简单的 Clash/Surge 格式规则，如 DOMAIN,example.com,Proxy
func parseSingboxCustomRule(rule string) map[string]interface{} {
	parts := strings.SplitN(rule, ",", 3)
	if len(parts) < 3 {
		return nil
	}
	ruleType := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	outbound := strings.TrimSpace(parts[2])

	switch strings.ToUpper(ruleType) {
	case "DOMAIN":
		return map[string]interface{}{"domain": []string{value}, "outbound": outbound}
	case "DOMAIN-SUFFIX":
		return map[string]interface{}{"domain_suffix": []string{value}, "outbound": outbound}
	case "DOMAIN-KEYWORD":
		return map[string]interface{}{"domain_keyword": []string{value}, "outbound": outbound}
	case "IP-CIDR", "IP-CIDR6":
		return map[string]interface{}{"ip_cidr": []string{value}, "outbound": outbound}
	case "GEOIP":
		return map[string]interface{}{"geoip": []string{strings.ToLower(value)}, "outbound": outbound}
	case "GEOSITE":
		return map[string]interface{}{"geosite": []string{strings.ToLower(value)}, "outbound": outbound}
	default:
		return nil
	}
}

// copyNames 返回 names 的副本
func copyNames(names []string) []string {
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// concat 合并 prefix 和 names
func concat(prefix []string, names []string) []string {
	result := make([]string, 0, len(prefix)+len(names))
	result = append(result, prefix...)
	result = append(result, names...)
	return result
}

// concat3 合并三组字符串
func concat3(a, b, c []string) []string {
	result := make([]string, 0, len(a)+len(b)+len(c))
	result = append(result, a...)
	result = append(result, b...)
	result = append(result, c...)
	return result
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

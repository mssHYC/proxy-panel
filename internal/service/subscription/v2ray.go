package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// V2RayGenerator V2Ray JSON 格式订阅生成器（消费 routing.Plan）
type V2RayGenerator struct{}

// Generate 旧入口不再使用；需要通过 GenerateWithPlan 消费 routing.Plan。
func (g *V2RayGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	return "", "", fmt.Errorf("v2ray generator requires routing plan; use GenerateWithPlan")
}

// GenerateWithPlan 基于预构建的 routing.Plan 渲染 V2Ray JSON 订阅。
func (g *V2RayGenerator) GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	outbounds := []map[string]any{}
	for _, node := range nodes {
		ob := buildV2RayOutbound(node, user)
		if ob != nil {
			outbounds = append(outbounds, ob)
		}
	}
	// 系统 outbounds
	outbounds = append(outbounds,
		map[string]any{"protocol": "freedom", "tag": "direct"},
		map[string]any{"protocol": "blackhole", "tag": "block"},
	)

	cfg := map[string]any{
		"outbounds": outbounds,
		"routing":   renderV2RayRoutingFromPlan(plan),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", "", err
	}
	return string(data), "application/json; charset=utf-8", nil
}

// buildV2RayOutbound 为单个节点构造 V2Ray JSON outbound（tag = node.Name）。
// 仅覆盖核心协议字段，保持最小可用。
func buildV2RayOutbound(node model.Node, user *model.User) map[string]any {
	s := parseSettings(node)
	switch node.Protocol {
	case "vmess":
		return map[string]any{
			"tag":      node.Name,
			"protocol": "vmess",
			"settings": map[string]any{
				"vnext": []map[string]any{{
					"address": node.Host,
					"port":    node.Port,
					"users": []map[string]any{{
						"id":       user.UUID,
						"alterId":  0,
						"security": "auto",
					}},
				}},
			},
			"streamSettings": v2rayStreamSettings(node, s),
		}
	case "vless":
		return map[string]any{
			"tag":      node.Name,
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []map[string]any{{
					"address": node.Host,
					"port":    node.Port,
					"users": []map[string]any{{
						"id":         user.UUID,
						"encryption": "none",
						"flow":       s.Flow,
					}},
				}},
			},
			"streamSettings": v2rayStreamSettings(node, s),
		}
	case "trojan":
		return map[string]any{
			"tag":      node.Name,
			"protocol": "trojan",
			"settings": map[string]any{
				"servers": []map[string]any{{
					"address":  node.Host,
					"port":     node.Port,
					"password": user.UUID,
				}},
			},
			"streamSettings": v2rayStreamSettings(node, s),
		}
	case "ss":
		method := s.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		return map[string]any{
			"tag":      node.Name,
			"protocol": "shadowsocks",
			"settings": map[string]any{
				"servers": []map[string]any{{
					"address":  node.Host,
					"port":     node.Port,
					"method":   method,
					"password": password,
				}},
			},
		}
	default:
		// hysteria2 等 V2Ray 核心不支持的协议跳过
		return nil
	}
}

func v2rayStreamSettings(node model.Node, s nodeSettings) map[string]any {
	net := node.Transport
	if net == "" {
		net = "tcp"
	}
	stream := map[string]any{"network": net}

	security := s.Security
	if security == "" && s.TLS {
		security = "tls"
	}
	if security != "" && security != "none" {
		stream["security"] = security
		if security == "tls" {
			tls := map[string]any{}
			if s.SNI != "" {
				tls["serverName"] = s.SNI
			}
			stream["tlsSettings"] = tls
		} else if security == "reality" {
			reality := map[string]any{}
			if s.SNI != "" {
				reality["serverName"] = s.SNI
			}
			if s.PublicKey != "" {
				reality["publicKey"] = s.PublicKey
			}
			if s.ShortID != "" {
				reality["shortId"] = s.ShortID
			}
			if s.Fingerprint != "" {
				reality["fingerprint"] = s.Fingerprint
			}
			stream["realitySettings"] = reality
		}
	}

	if net == "ws" || net == "httpupgrade" {
		ws := map[string]any{}
		if s.Path != "" {
			ws["path"] = s.Path
		}
		host := s.Host
		if host == "" {
			host = node.Host
		}
		ws["headers"] = map[string]any{"Host": host}
		stream["wsSettings"] = ws
	}
	if net == "grpc" && s.ServiceName != "" {
		stream["grpcSettings"] = map[string]any{"serviceName": s.ServiceName}
	}
	return stream
}

// renderV2RayRoutingFromPlan returns the V2Ray "routing" config section.
func renderV2RayRoutingFromPlan(plan *routing.Plan) map[string]any {
	rules := []map[string]any{}
	for _, r := range plan.Rules {
		out := v2rayOutbound(r.Outbound, plan.Groups)
		rule := map[string]any{
			"type":        "field",
			"outboundTag": out,
		}
		domains := []string{}
		for _, t := range r.SiteTags {
			domains = append(domains, "geosite:"+t)
		}
		for _, v := range r.DomainSuffix {
			domains = append(domains, "domain:"+v)
		}
		domains = append(domains, r.DomainKeyword...)
		if len(domains) > 0 {
			rule["domain"] = domains
		}
		ips := []string{}
		for _, t := range r.IPTags {
			ips = append(ips, "geoip:"+t)
		}
		ips = append(ips, r.IPCIDR...)
		if len(ips) > 0 {
			rule["ip"] = ips
		}
		if len(r.SrcIPCIDR) > 0 {
			log.Printf("[routing/v2ray] 忽略 src_ip_cidr (不支持): %v", r.SrcIPCIDR)
		}
		rules = append(rules, rule)
	}
	rules = append(rules, map[string]any{
		"type": "field", "port": "0-65535", "outboundTag": v2rayOutbound(plan.Final, plan.Groups),
	})
	return map[string]any{"domainStrategy": "IPIfNonMatch", "rules": rules}
}

func v2rayOutbound(codeOrLit string, groups []routing.OutboundGroup) string {
	if codeOrLit == "DIRECT" {
		return "direct"
	}
	if codeOrLit == "REJECT" {
		return "block"
	}
	for _, g := range groups {
		if g.Code == codeOrLit {
			return g.DisplayName
		}
	}
	return codeOrLit
}

// buildV2RayURIs 构建所有节点的 URI 列表（供 Shadowrocket 复用）
func buildV2RayURIs(nodes []model.Node, user *model.User) []string {
	var uris []string
	for _, node := range nodes {
		uri := buildNodeURI(node, user)
		if uri != "" {
			uris = append(uris, uri)
		}
	}
	return uris
}

func buildNodeURI(node model.Node, user *model.User) string {
	s := parseSettings(node)

	switch node.Protocol {
	case "vless":
		return buildVLESSURI(node, user, s)
	case "vmess":
		return buildVMessURI(node, user, s)
	case "trojan":
		return buildTrojanURI(node, user, s)
	case "ss":
		return buildSSURI(node, user, s)
	case "hysteria2":
		return buildHysteria2URI(node, user, s)
	default:
		return ""
	}
}

func buildVLESSURI(node model.Node, user *model.User, s nodeSettings) string {
	params := url.Values{}
	if node.Transport != "" {
		params.Set("type", node.Transport)
	} else {
		params.Set("type", "tcp")
	}

	security := s.Security
	if security == "" {
		if s.TLS {
			security = "tls"
		} else {
			security = "none"
		}
	}
	params.Set("security", security)

	if security == "reality" {
		if s.PublicKey != "" {
			params.Set("pbk", s.PublicKey)
		}
		if s.ShortID != "" {
			params.Set("sid", s.ShortID)
		}
		if s.Fingerprint != "" {
			params.Set("fp", s.Fingerprint)
		}
	}

	if s.SNI != "" {
		params.Set("sni", s.SNI)
	}
	if s.Flow != "" {
		params.Set("flow", s.Flow)
	}
	if node.Transport == "ws" || node.Transport == "httpupgrade" {
		path := s.Path
		if path == "" {
			path = "/"
		}
		params.Set("path", path)
		host := s.Host
		if host == "" {
			host = node.Host
		}
		params.Set("host", host)
	}
	if node.Transport == "grpc" && s.ServiceName != "" {
		params.Set("serviceName", s.ServiceName)
	}

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, node.Host, node.Port, params.Encode(), url.PathEscape(node.Name))
}

func buildVMessURI(node model.Node, user *model.User, s nodeSettings) string {
	vmessObj := map[string]interface{}{
		"v":    "2",
		"ps":   node.Name,
		"add":  node.Host,
		"port": node.Port,
		"id":   user.UUID,
		"aid":  0,
		"net":  node.Transport,
		"type": "none",
	}
	if node.Transport == "" {
		vmessObj["net"] = "tcp"
	}

	if s.TLS {
		vmessObj["tls"] = "tls"
		if s.SNI != "" {
			vmessObj["sni"] = s.SNI
		}
	} else {
		vmessObj["tls"] = ""
	}

	if node.Transport == "ws" || node.Transport == "httpupgrade" {
		path := s.Path
		if path == "" {
			path = "/"
		}
		vmessObj["path"] = path
		host := s.Host
		if host == "" {
			host = node.Host
		}
		vmessObj["host"] = host
	}

	data, _ := json.Marshal(vmessObj)
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}

func buildTrojanURI(node model.Node, user *model.User, s nodeSettings) string {
	params := url.Values{}
	if s.SNI != "" {
		params.Set("sni", s.SNI)
	}
	if node.Transport != "" && node.Transport != "tcp" {
		params.Set("type", node.Transport)
	}
	if node.Transport == "ws" {
		if s.Path != "" {
			params.Set("path", s.Path)
		}
		if s.Host != "" {
			params.Set("host", s.Host)
		}
	}

	query := ""
	if encoded := params.Encode(); encoded != "" {
		query = "?" + encoded
	}
	return fmt.Sprintf("trojan://%s@%s:%d%s#%s",
		user.UUID, node.Host, node.Port, query, url.PathEscape(node.Name))
}

func buildSSURI(node model.Node, user *model.User, s nodeSettings) string {
	method := s.Method
	if method == "" {
		method = "aes-256-gcm"
	}
	password := s.Password
	if password == "" {
		password = user.UUID
	}
	userInfo := base64.URLEncoding.EncodeToString([]byte(method + ":" + password))
	return fmt.Sprintf("ss://%s@%s:%d#%s",
		userInfo, node.Host, node.Port, url.PathEscape(node.Name))
}

func buildHysteria2URI(node model.Node, user *model.User, s nodeSettings) string {
	// sing-box 服务端固定以 user.UUID 作为 hy2 password，订阅必须与之一致
	password := user.UUID
	params := url.Values{}
	if s.SNI != "" {
		params.Set("sni", s.SNI)
	}
	if s.Obfs != "" {
		params.Set("obfs", s.Obfs)
		if s.ObfsPassword != "" {
			params.Set("obfs-password", s.ObfsPassword)
		}
	}
	if s.AllowInsecure {
		params.Set("insecure", "1")
	}

	query := ""
	if encoded := params.Encode(); encoded != "" {
		query = "?" + encoded
	}
	return fmt.Sprintf("hysteria2://%s@%s:%d%s#%s",
		password, node.Host, node.Port, query, url.PathEscape(node.Name))
}

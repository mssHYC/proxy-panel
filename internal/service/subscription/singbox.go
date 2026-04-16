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
	var outbounds []map[string]interface{}
	var tags []string

	// 生成每个节点的 outbound
	for _, node := range nodes {
		ob := g.buildOutbound(node, user)
		if ob != nil {
			outbounds = append(outbounds, ob)
			tags = append(tags, node.Name)
		}
	}

	// selector outbound
	selectorOutbounds := make([]string, 0, len(tags)+1)
	selectorOutbounds = append(selectorOutbounds, tags...)
	selectorOutbounds = append(selectorOutbounds, "direct")
	selector := map[string]interface{}{
		"type":      "selector",
		"tag":       "proxy",
		"outbounds": selectorOutbounds,
	}

	// urltest outbound
	urltest := map[string]interface{}{
		"type":      "urltest",
		"tag":       "auto",
		"outbounds": tags,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  "5m",
	}

	// 系统 outbound
	directOb := map[string]interface{}{"type": "direct", "tag": "direct"}
	blockOb := map[string]interface{}{"type": "block", "tag": "block"}
	dnsOb := map[string]interface{}{"type": "dns", "tag": "dns-out"}

	// 组装完整 outbounds：selector + urltest + 节点 + 系统
	allOutbounds := []map[string]interface{}{selector, urltest}
	allOutbounds = append(allOutbounds, outbounds...)
	allOutbounds = append(allOutbounds, directOb, blockOb, dnsOb)

	// 路由规则
	route := map[string]interface{}{
		"rules": []map[string]interface{}{
			{"protocol": "dns", "outbound": "dns-out"},
			{"geoip": []string{"cn"}, "outbound": "direct"},
			{"geosite": []string{"cn"}, "outbound": "direct"},
		},
		"final": "proxy",
	}

	config := map[string]interface{}{
		"outbounds": allOutbounds,
		"route":     route,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}

	return string(data), "application/json; charset=utf-8", nil
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
		"type":       "vless",
		"tag":        node.Name,
		"server":     node.Host,
		"server_port": node.Port,
		"uuid":       user.UUID,
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
		ob["tls"] = tls
	}

	g.setTransport(ob, node, s)
	return ob
}

func (g *SingboxGenerator) buildVMess(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	ob := map[string]interface{}{
		"type":       "vmess",
		"tag":        node.Name,
		"server":     node.Host,
		"server_port": node.Port,
		"uuid":       user.UUID,
		"alter_id":   0,
		"security":   "auto",
	}

	if s.TLS {
		tls := map[string]interface{}{
			"enabled": true,
		}
		if s.SNI != "" {
			tls["server_name"] = s.SNI
		}
		ob["tls"] = tls
	}

	g.setTransport(ob, node, s)
	return ob
}

func (g *SingboxGenerator) buildTrojan(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	ob := map[string]interface{}{
		"type":       "trojan",
		"tag":        node.Name,
		"server":     node.Host,
		"server_port": node.Port,
		"password":   user.UUID,
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
		"type":       "shadowsocks",
		"tag":        node.Name,
		"server":     node.Host,
		"server_port": node.Port,
		"method":     method,
		"password":   password,
	}
}

func (g *SingboxGenerator) buildHysteria2(node model.Node, user *model.User, s nodeSettings) map[string]interface{} {
	password := s.Password
	if password == "" {
		password = user.UUID
	}
	ob := map[string]interface{}{
		"type":       "hysteria2",
		"tag":        node.Name,
		"server":     node.Host,
		"server_port": node.Port,
		"password":   password,
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
	ob["tls"] = tls

	if s.Obfs != "" {
		ob["obfs"] = map[string]interface{}{
			"type":     s.Obfs,
			"password": s.ObfsPassword,
		}
	}
	return ob
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

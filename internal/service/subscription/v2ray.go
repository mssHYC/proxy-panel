package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"proxy-panel/internal/model"
)

// V2RayGenerator V2Ray base64 URI 格式订阅生成器
type V2RayGenerator struct{}

func (g *V2RayGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	uris := buildV2RayURIs(nodes, user)
	raw := strings.Join(uris, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	return encoded, "text/plain; charset=utf-8", nil
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
	if node.Transport == "ws" {
		if s.Path != "" {
			params.Set("path", s.Path)
		}
		if s.Host != "" {
			params.Set("host", s.Host)
		}
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

	if node.Transport == "ws" {
		if s.Path != "" {
			vmessObj["path"] = s.Path
		}
		if s.Host != "" {
			vmessObj["host"] = s.Host
		}
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

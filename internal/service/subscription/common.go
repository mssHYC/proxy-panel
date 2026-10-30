package subscription

import (
	"encoding/json"
	"strings"

	"proxy-panel/internal/model"
)

// nodeSettings 节点配置的 JSON 结构
type nodeSettings struct {
	SNI           string   `json:"sni"`
	Host          string   `json:"host"`
	Path          string   `json:"path"`
	TLS           bool     `json:"tls"`
	Method        string   `json:"method"`
	Password      string   `json:"password"`
	Flow          string   `json:"flow"`
	Fingerprint   string   `json:"fingerprint"`
	PublicKey     string   `json:"pbk"`
	ShortID       string   `json:"sid"`
	Security      string   `json:"security"`
	ALPN          string   `json:"-"`
	UpMbps        int      `json:"up_mbps"`
	DownMbps      int      `json:"down_mbps"`
	Obfs          string   `json:"obfs"`
	ObfsPassword  string   `json:"obfs_password"`
	AllowInsecure bool     `json:"allow_insecure"`
	ServiceName   string   `json:"service_name"`
	Dest          string   `json:"dest"`
	ServerNames   []string `json:"server_names"`
	ShortIDs      []string `json:"short_ids"`
}

// parseSettings 解析节点 Settings JSON 字段，兼容前端多种 key 名
func parseSettings(node model.Node) nodeSettings {
	var s nodeSettings
	if node.Settings == "" {
		return s
	}
	_ = json.Unmarshal([]byte(node.Settings), &s)

	// 兼容前端使用 public_key / short_id / private_key 等 key 名
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(node.Settings), &raw) == nil {
		if s.SNI == "" {
			_ = json.Unmarshal(raw["serverName"], &s.SNI)
		}
		if len(s.ServerNames) == 0 {
			_ = json.Unmarshal(raw["serverNames"], &s.ServerNames)
		}
		if s.SNI == "" && len(s.ServerNames) > 0 {
			s.SNI = s.ServerNames[0]
		}
		if v := raw["alpn"]; len(v) > 0 {
			var values []string
			if json.Unmarshal(v, &values) == nil {
				s.ALPN = strings.Join(values, ",")
			} else {
				_ = json.Unmarshal(v, &s.ALPN)
			}
		}
		if s.ALPN == "" {
			var port int
			if json.Unmarshal(raw["fallback_port"], &port) == nil && port > 0 {
				s.ALPN = "http/1.1"
			}
		}

		if s.PublicKey == "" {
			if v, ok := raw["public_key"]; ok {
				var val string
				if json.Unmarshal(v, &val) == nil {
					s.PublicKey = val
				}
			}
		}
		if s.ShortID == "" {
			if v, ok := raw["short_id"]; ok {
				var val string
				if json.Unmarshal(v, &val) == nil {
					s.ShortID = val
				}
			}
		}
	}

	// 从 short_ids 数组回退取第一个 short_id
	if s.ShortID == "" && len(s.ShortIDs) > 0 {
		s.ShortID = s.ShortIDs[0]
	}

	// ShortID 可能是逗号分隔的多个值，客户端只取第一个
	if strings.Contains(s.ShortID, ",") {
		s.ShortID = strings.TrimSpace(strings.Split(s.ShortID, ",")[0])
	}

	// SNI: 去除可能带的端口号 (如 www.tesla.com:443 → www.tesla.com)
	if strings.Contains(s.SNI, ":") {
		s.SNI = strings.Split(s.SNI, ":")[0]
	}

	if s.Security == "tls" {
		s.TLS = true
	}
	return s
}

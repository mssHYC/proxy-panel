package subscription

import (
	"encoding/json"
	"proxy-panel/internal/model"
)

// nodeSettings 节点配置的 JSON 结构
type nodeSettings struct {
	SNI            string `json:"sni"`
	Host           string `json:"host"`
	Path           string `json:"path"`
	TLS            bool   `json:"tls"`
	Method         string `json:"method"`
	Password       string `json:"password"`
	Flow           string `json:"flow"`
	Fingerprint    string `json:"fingerprint"`
	PublicKey      string `json:"pbk"`
	ShortID        string `json:"sid"`
	Security       string `json:"security"`
	ALPN           string `json:"alpn"`
	UpMbps         int    `json:"up_mbps"`
	DownMbps       int    `json:"down_mbps"`
	Obfs           string `json:"obfs"`
	ObfsPassword   string `json:"obfs_password"`
	AllowInsecure  bool   `json:"allow_insecure"`
	ServiceName    string `json:"service_name"`
}

// parseSettings 解析节点 Settings JSON 字段
func parseSettings(node model.Node) nodeSettings {
	var s nodeSettings
	if node.Settings != "" {
		_ = json.Unmarshal([]byte(node.Settings), &s)
	}
	return s
}

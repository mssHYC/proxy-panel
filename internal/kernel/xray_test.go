package kernel

import (
	"testing"
)

// 回归：TLS 分支历史上只识别 camelCase key (serverName / certPath / keyPath)，
// 但前端实际存储的是 snake_case (sni / cert_path / key_path)，导致生成的
// xray config 里 serverName 为空、且完全没有 certificates 字段，TLS inbound
// 根本起不来，客户端握手直接超时。修复后两种命名必须都能解析。
func TestXrayBuildStreamSettings_TLSWithSnakeCaseKeys(t *testing.T) {
	e := &XrayEngine{}
	node := NodeConfig{
		Transport: "httpupgrade",
		Settings: map[string]interface{}{
			"security":  "tls",
			"sni":       "cdn.example.com",
			"cert_path": "/etc/ssl/example.crt",
			"key_path":  "/etc/ssl/example.key",
			"path":      "/a3f9c2",
			"host":      "cdn.example.com",
		},
	}
	stream := e.buildStreamSettings(node)

	tls, ok := stream["tlsSettings"].(map[string]interface{})
	if !ok {
		t.Fatalf("tlsSettings missing or wrong type: %#v", stream["tlsSettings"])
	}
	if got := tls["serverName"]; got != "cdn.example.com" {
		t.Errorf("serverName want %q, got %v", "cdn.example.com", got)
	}
	certs, ok := tls["certificates"].([]interface{})
	if !ok || len(certs) != 1 {
		t.Fatalf("certificates missing/wrong length: %#v", tls["certificates"])
	}
	c := certs[0].(map[string]interface{})
	if c["certificateFile"] != "/etc/ssl/example.crt" {
		t.Errorf("certificateFile want %q, got %v", "/etc/ssl/example.crt", c["certificateFile"])
	}
	if c["keyFile"] != "/etc/ssl/example.key" {
		t.Errorf("keyFile want %q, got %v", "/etc/ssl/example.key", c["keyFile"])
	}
}

// 保留 camelCase 兼容：老配置/手动导入的 camelCase 字段也要继续识别。
func TestXrayBuildStreamSettings_TLSWithCamelCaseKeys(t *testing.T) {
	e := &XrayEngine{}
	node := NodeConfig{
		Transport: "tcp",
		Settings: map[string]interface{}{
			"security":   "tls",
			"serverName": "foo.example.com",
			"certPath":   "/a.crt",
			"keyPath":    "/a.key",
		},
	}
	stream := e.buildStreamSettings(node)
	tls := stream["tlsSettings"].(map[string]interface{})
	if tls["serverName"] != "foo.example.com" {
		t.Errorf("camelCase serverName not read: %v", tls["serverName"])
	}
	certs, ok := tls["certificates"].([]interface{})
	if !ok || len(certs) != 1 {
		t.Fatalf("camelCase certificates missing: %#v", tls["certificates"])
	}
}

func TestGetSettingInt(t *testing.T) {
	cases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		def      int
		expected int
	}{
		{"nil map returns default", nil, "x", 7, 7},
		{"missing key returns default", map[string]interface{}{}, "x", 7, 7},
		{"int value", map[string]interface{}{"x": 5}, "x", 7, 5},
		{"float64 from json", map[string]interface{}{"x": 5.0}, "x", 7, 5},
		{"int64 value", map[string]interface{}{"x": int64(5)}, "x", 7, 5},
		{"numeric string", map[string]interface{}{"x": "12"}, "x", 7, 12},
		{"invalid string falls back", map[string]interface{}{"x": "abc"}, "x", 7, 7},
		{"empty string falls back", map[string]interface{}{"x": ""}, "x", 7, 7},
		{"wrong type falls back", map[string]interface{}{"x": []int{1}}, "x", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getSettingInt(c.m, c.key, c.def); got != c.expected {
				t.Errorf("%s: want %d, got %d", c.name, c.expected, got)
			}
		})
	}
}

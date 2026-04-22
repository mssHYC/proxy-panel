package kernel

import (
	"encoding/json"
	"testing"
)

func TestHy2BuildInbound_NodeLevelLimit(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(8),
			"max_down_mbps": float64(15),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2"},
		{UUID: "u2", Email: "bob", Protocol: "hysteria2"},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 8 {
		t.Errorf("up_mbps: want 8, got %v", ib["up_mbps"])
	}
	if ib["down_mbps"] != 15 {
		t.Errorf("down_mbps: want 15, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_NoLimitsOmitted(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2"},
	}
	ib := e.buildInbound(node, users)
	if _, ok := ib["up_mbps"]; ok {
		t.Errorf("expected no up_mbps key, got %v", ib["up_mbps"])
	}
	if _, ok := ib["down_mbps"]; ok {
		t.Errorf("expected no down_mbps key")
	}
}

func TestHy2GenerateConfigSerializable(t *testing.T) {
	e := NewSingboxEngine("", "", 9090)
	nodes := []NodeConfig{{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{"max_up_mbps": float64(7)},
	}}
	users := []UserConfig{{UUID: "u1", Email: "a", Protocol: "hysteria2"}}
	data, err := e.GenerateConfig(nodes, users)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestHy2BuildInbound_SingleUserUseMinimum(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(20),
			"max_down_mbps": float64(20),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 5},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 5 {
		t.Errorf("single-user: up_mbps want 5, got %v", ib["up_mbps"])
	}
	if ib["down_mbps"] != 5 {
		t.Errorf("single-user: down_mbps want 5, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_SingleUserNodeMaxWhenUserHigher(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_down_mbps": float64(10),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 100},
	}
	ib := e.buildInbound(node, users)
	if ib["down_mbps"] != 10 {
		t.Errorf("user 100 > node 10: down_mbps want 10, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_SingleUserNoNodeMax(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 3},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 3 || ib["down_mbps"] != 3 {
		t.Errorf("node unset, user=3: want both 3, got up=%v down=%v", ib["up_mbps"], ib["down_mbps"])
	}
}

func TestHy2BuildInbound_MultiUserIgnoresUserLimits(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(10),
			"max_down_mbps": float64(10),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 3},
		{UUID: "u2", Email: "bob", Protocol: "hysteria2", SpeedLimit: 5},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 10 || ib["down_mbps"] != 10 {
		t.Errorf("multi-user should fallback to node max; got up=%v down=%v", ib["up_mbps"], ib["down_mbps"])
	}
}

func TestSingboxBuildInbound_Vless(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 1, Tag: "vless-1", Port: 443, Protocol: "vless", Transport: "ws",
		Settings: map[string]interface{}{
			"flow":     "xtls-rprx-vision",
			"path":     "/vl",
			"host":     "example.com",
			"security": "tls",
			"sni":      "example.com",
		},
	}
	users := []UserConfig{{UUID: "uuid-a", Email: "alice", Protocol: "vless", NodeIDs: []int64{1}}}
	ib := e.buildInbound(node, users)
	if ib["type"] != "vless" {
		t.Fatalf("type: got %v", ib["type"])
	}
	us := ib["users"].([]map[string]interface{})
	if len(us) != 1 || us[0]["uuid"] != "uuid-a" || us[0]["flow"] != "xtls-rprx-vision" {
		t.Fatalf("users wrong: %+v", us)
	}
	tr, ok := ib["transport"].(map[string]interface{})
	if !ok || tr["type"] != "ws" || tr["path"] != "/vl" {
		t.Fatalf("transport wrong: %+v", ib["transport"])
	}
	tls, ok := ib["tls"].(map[string]interface{})
	if !ok || tls["enabled"] != true || tls["server_name"] != "example.com" {
		t.Fatalf("tls wrong: %+v", ib["tls"])
	}
	if _, err := json.Marshal(ib); err != nil {
		t.Fatalf("not json serializable: %v", err)
	}
}

func TestSingboxBuildInbound_Vmess(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{ID: 2, Tag: "vm-1", Port: 10086, Protocol: "vmess", Transport: "tcp", Settings: map[string]interface{}{}}
	users := []UserConfig{{UUID: "uuid-b", Email: "bob", Protocol: "vmess", NodeIDs: []int64{2}}}
	ib := e.buildInbound(node, users)
	us := ib["users"].([]map[string]interface{})
	if us[0]["uuid"] != "uuid-b" || us[0]["alterId"] != 0 {
		t.Fatalf("users wrong: %+v", us)
	}
	if _, ok := ib["transport"]; ok {
		t.Fatalf("tcp should have no transport")
	}
	if _, ok := ib["tls"]; ok {
		t.Fatalf("security=none should have no tls")
	}
}

func TestHy2BuildInbound_IgnoreClientBandwidth(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 1, Tag: "hy2-1", Port: 443, Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"ignore_client_bandwidth": true,
		},
	}
	users := []UserConfig{{UUID: "u1", Email: "alice", Protocol: "hysteria2", NodeIDs: []int64{1}}}
	ib := e.buildInbound(node, users)
	if ib["ignore_client_bandwidth"] != true {
		t.Errorf("ignore_client_bandwidth: want true, got %v", ib["ignore_client_bandwidth"])
	}
}

// Hy2 inbound 不再透传 masquerade：sing-box 1.10+ 的同端口 HTTP/3 反代实现会
// 干扰鉴权后流传输（handshake success 紧跟 stream canceled），整体不通。
// 即使老 settings JSON 里残留该字段，内核层也应忽略。
func TestHy2BuildInbound_MasqueradeAlwaysStripped(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 1, Tag: "hy2-1", Port: 443, Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"masquerade": "https://example.com",
		},
	}
	users := []UserConfig{{UUID: "u1", Email: "alice", Protocol: "hysteria2", NodeIDs: []int64{1}}}
	ib := e.buildInbound(node, users)
	if _, ok := ib["masquerade"]; ok {
		t.Errorf("masquerade 应被过滤掉, got %v", ib["masquerade"])
	}
}

// Hy2 inbound 不写 server_name / alpn：避免 sing-box 做强匹配导致握手失败
func TestHy2BuildInbound_TLSOmitsSNIAndALPN(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 1, Tag: "hy2-1", Port: 443, Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"cert_path": "/a.crt",
			"key_path":  "/a.key",
			"sni":       "example.com",
			"alpn":      []interface{}{"h3"},
		},
	}
	users := []UserConfig{{UUID: "u1", Email: "alice", Protocol: "hysteria2", NodeIDs: []int64{1}}}
	ib := e.buildInbound(node, users)
	tls, ok := ib["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls missing: %+v", ib["tls"])
	}
	if _, ok := tls["server_name"]; ok {
		t.Errorf("hy2 inbound 不应写入 server_name: %v", tls["server_name"])
	}
	if _, ok := tls["alpn"]; ok {
		t.Errorf("hy2 inbound 不应写入 alpn: %v", tls["alpn"])
	}
}

func TestSingboxBuildInbound_Shadowsocks_SingleUser(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 9, Tag: "ss-1", Port: 8388, Protocol: "shadowsocks",
		Settings: map[string]interface{}{"method": "aes-256-gcm"},
	}
	users := []UserConfig{{UUID: "pw-a", Email: "alice", Protocol: "ss", NodeIDs: []int64{9}}}
	ib := e.buildInbound(node, users)
	if ib["type"] != "shadowsocks" || ib["method"] != "aes-256-gcm" {
		t.Fatalf("ss inbound wrong: %+v", ib)
	}
	// 单用户 + 非 2022 加密走顶层 password
	if ib["password"] != "pw-a" {
		t.Errorf("expected top-level password for single-user legacy SS, got %+v", ib)
	}
	if _, ok := ib["users"]; ok {
		t.Errorf("legacy SS single-user should not use users array: %+v", ib["users"])
	}
}

func TestSingboxBuildInbound_Shadowsocks2022_UsesUsersArray(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 10, Tag: "ss-2", Port: 8389, Protocol: "ss",
		Settings: map[string]interface{}{"method": "2022-blake3-aes-256-gcm"},
	}
	users := []UserConfig{
		{UUID: "pw-a", Email: "alice", Protocol: "ss", NodeIDs: []int64{10}},
		{UUID: "pw-b", Email: "bob", Protocol: "ss", NodeIDs: []int64{10}},
	}
	ib := e.buildInbound(node, users)
	us, ok := ib["users"].([]map[string]interface{})
	if !ok || len(us) != 2 {
		t.Fatalf("ss2022 should use users array with 2 entries, got %+v", ib["users"])
	}
	if _, ok := ib["password"]; ok {
		t.Errorf("ss2022 multi-user should not have top-level password")
	}
}

func TestSingboxTLS_ALPN(t *testing.T) {
	tls := buildSingboxTLS(map[string]interface{}{
		"security": "tls",
		"sni":      "example.com",
		"alpn":     []interface{}{"h2", "http/1.1"},
	})
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Errorf("alpn: want [h2, http/1.1], got %v", tls["alpn"])
	}
}

func TestGetSettingBool(t *testing.T) {
	cases := []struct {
		in   interface{}
		want bool
	}{
		{true, true},
		{false, false},
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"", false},
		{float64(1), true},
		{float64(0), false},
	}
	for _, c := range cases {
		got := getSettingBool(map[string]interface{}{"k": c.in}, "k", false)
		if got != c.want {
			t.Errorf("in=%v: want %v, got %v", c.in, c.want, got)
		}
	}
	if got := getSettingBool(nil, "k", true); got != true {
		t.Errorf("nil map should return default")
	}
	if got := getSettingBool(map[string]interface{}{}, "missing", true); got != true {
		t.Errorf("missing key should return default")
	}
}

func TestIsSS2022Method(t *testing.T) {
	if !isSS2022Method("2022-blake3-aes-256-gcm") {
		t.Error("should detect 2022-blake3 as SS2022")
	}
	if isSS2022Method("aes-256-gcm") {
		t.Error("legacy method should not be SS2022")
	}
	if isSS2022Method("") {
		t.Error("empty should not be SS2022")
	}
}

func TestSingboxBuildInbound_Trojan(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID: 3, Tag: "tj-1", Port: 443, Protocol: "trojan", Transport: "grpc",
		Settings: map[string]interface{}{
			"serviceName": "tjgrpc",
			"security":    "reality",
			"dest":        "www.apple.com:443",
			"server_names": []interface{}{"www.apple.com"},
			"private_key": "priv",
			"short_ids":   []interface{}{"abcd"},
		},
	}
	users := []UserConfig{{UUID: "pw", Email: "charlie", Protocol: "trojan", NodeIDs: []int64{3}}}
	ib := e.buildInbound(node, users)
	us := ib["users"].([]map[string]interface{})
	if us[0]["password"] != "pw" {
		t.Fatalf("trojan password wrong: %+v", us)
	}
	tr := ib["transport"].(map[string]interface{})
	if tr["type"] != "grpc" || tr["service_name"] != "tjgrpc" {
		t.Fatalf("grpc transport wrong: %+v", tr)
	}
	tls := ib["tls"].(map[string]interface{})
	reality := tls["reality"].(map[string]interface{})
	if reality["enabled"] != true || reality["private_key"] != "priv" {
		t.Fatalf("reality wrong: %+v", reality)
	}
	hs := reality["handshake"].(map[string]interface{})
	if hs["server"] != "www.apple.com" || hs["server_port"] != 443 {
		t.Fatalf("handshake wrong: %+v", hs)
	}
	if _, err := json.Marshal(ib); err != nil {
		t.Fatalf("not json serializable: %v", err)
	}
}

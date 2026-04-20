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

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

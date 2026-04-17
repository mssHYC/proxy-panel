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

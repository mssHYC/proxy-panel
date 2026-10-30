package service

import (
	"errors"
	"testing"
)

// TestNodeCreate_InsertColumnsMatchValues 防回归：Create 的 INSERT 语句
// 列数必须与 VALUES 个数一致，否则 SQLite 报 "N values for M columns"，
// 导致所有节点创建失败。
func TestNodeCreate_InsertColumnsMatchValues(t *testing.T) {
	db := openSyncTestDB(t)
	s := NewNodeService(db, nil) // Allow 有 nil 接收者保护，测试传 nil 安全

	node, err := s.Create(&CreateNodeReq{
		Name:       "vpn",
		Host:       "vpn.example.com",
		Port:       1388,
		Protocol:   "vless",
		Transport:  "tcp",
		KernelType: "xray",
		Settings:   `{"security":"reality"}`,
		SortOrder:  0,
	})
	if err != nil {
		t.Fatalf("Create 节点失败: %v", err)
	}
	if node == nil || node.ID == 0 {
		t.Fatalf("期望返回带 ID 的节点，得到 %+v", node)
	}
}

func TestNodeSecurityValidationBeforePersistence(t *testing.T) {
	db := openSyncTestDB(t)
	s := NewNodeService(db, nil)
	req := &CreateNodeReq{Name: "site", Host: "example.com", Port: 443, Protocol: "vless", Transport: "tcp", KernelType: "xray", Settings: `{"security":"tls","fallback_port":443}`}
	if _, err := s.Create(req); !errors.Is(err, ErrInvalidNodeSettings) {
		t.Fatalf("expected validation error, got %v", err)
	}
	nodes, err := s.List()
	if err != nil || len(nodes) != 0 {
		t.Fatalf("invalid create changed DB: %v %v", nodes, err)
	}
	req.Settings = `{"security":"tls","fallback_port":8081}`
	node, err := s.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	ws := "ws"
	if _, err := s.Update(node.ID, &UpdateNodeReq{Transport: &ws}); !errors.Is(err, ErrInvalidNodeSettings) {
		t.Fatalf("merged settings not validated: %v", err)
	}
	saved, err := s.GetByID(node.ID)
	if err != nil || saved.Transport != "tcp" {
		t.Fatal("invalid update persisted")
	}
}

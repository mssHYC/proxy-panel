package service

import "testing"

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

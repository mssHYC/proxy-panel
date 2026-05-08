package service

import (
	"testing"
)

// TestPlanVisibility_UnionUserNodesAndPlanGroups 验证：
// loadUserNodeMap 与 ListByUserID 都把 user_nodes 直接关联与 plan→group→node 关联做并集。
func TestPlanVisibility_UnionUserNodesAndPlanGroups(t *testing.T) {
	db := openSyncTestDB(t)

	// 三个节点：n1 直接关联，n2 通过套餐授权，n3 与该用户无关
	n1 := seedNode(t, db, "xray", "vless")
	n2 := seedNode(t, db, "xray", "trojan")
	n3 := seedNode(t, db, "xray", "vmess")

	// 用户
	res, err := db.Exec(`INSERT INTO users (uuid, username, protocol) VALUES ('u', 'alice', 'vless')`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid, _ := res.LastInsertId()

	// user_nodes: alice → n1
	if _, err := db.Exec(`INSERT INTO user_nodes (user_id, node_id) VALUES (?, ?)`, uid, n1); err != nil {
		t.Fatalf("insert user_nodes: %v", err)
	}

	// node_group g1 包含 n2；plan p1 关联 g1；alice.plan_id = p1
	res, err = db.Exec(`INSERT INTO node_groups (name) VALUES ('g1')`)
	if err != nil {
		t.Fatalf("insert node_groups: %v", err)
	}
	gid, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO node_group_members (node_group_id, node_id) VALUES (?, ?)`, gid, n2); err != nil {
		t.Fatalf("insert node_group_members: %v", err)
	}
	res, err = db.Exec(`INSERT INTO plans (name, enabled) VALUES ('p1', 1)`)
	if err != nil {
		t.Fatalf("insert plans: %v", err)
	}
	pid, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO plan_node_groups (plan_id, node_group_id) VALUES (?, ?)`, pid, gid); err != nil {
		t.Fatalf("insert plan_node_groups: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET plan_id = ? WHERE id = ?`, pid, uid); err != nil {
		t.Fatalf("update users.plan_id: %v", err)
	}

	// loadUserNodeMap 应该返回 {uid: [n1, n2]}（顺序无保证），不含 n3
	sync := NewKernelSyncService(db, nil)
	m, err := sync.loadUserNodeMap()
	if err != nil {
		t.Fatalf("loadUserNodeMap: %v", err)
	}
	got := map[int64]bool{}
	for _, nid := range m[uid] {
		got[nid] = true
	}
	if !got[n1] || !got[n2] || got[n3] {
		t.Errorf("可见性错误，期望 [n1,n2]，得到 %v", m[uid])
	}

	// 关停套餐后，n2 不再可见
	if _, err := db.Exec(`UPDATE plans SET enabled = 0 WHERE id = ?`, pid); err != nil {
		t.Fatalf("disable plan: %v", err)
	}
	m2, _ := sync.loadUserNodeMap()
	got2 := map[int64]bool{}
	for _, nid := range m2[uid] {
		got2[nid] = true
	}
	if !got2[n1] || got2[n2] {
		t.Errorf("套餐 disabled 后应只剩 n1，得到 %v", m2[uid])
	}

	// ListByUserID 验证 enable=0 套餐场景
	nodeSvc := NewNodeService(db, nil)
	nodes, err := nodeSvc.ListByUserID(uid)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != n1 {
		t.Errorf("ListByUserID 期望仅 n1，得到 %+v", nodes)
	}
}

// TestPlanService_AssignAndUnassign 套餐分配会重置流量并设置过期时间；解绑只清 plan_id。
func TestPlanService_AssignAndUnassign(t *testing.T) {
	db := openSyncTestDB(t)
	res, err := db.Exec(`INSERT INTO users (uuid, username, protocol, traffic_used, traffic_up, traffic_down, warn_sent)
		VALUES ('u', 'alice', 'vless', 999, 9, 9, 1)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO plans (name, traffic_limit, duration_days, enabled) VALUES ('p1', 1024, 7, 1)`)
	if err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	pid, _ := res.LastInsertId()

	svc := NewPlanService(db)
	if err := svc.AssignToUser(uid, &AssignPlanReq{PlanID: &pid}); err != nil {
		t.Fatalf("AssignToUser: %v", err)
	}
	var planID, used, up, down, limit int64
	var warn int
	if err := db.QueryRow(`SELECT plan_id, traffic_used, traffic_up, traffic_down, traffic_limit, warn_sent FROM users WHERE id = ?`, uid).
		Scan(&planID, &used, &up, &down, &limit, &warn); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if planID != pid || used != 0 || up != 0 || down != 0 || limit != 1024 || warn != 0 {
		t.Errorf("分配套餐字段不正确: plan=%d used=%d up=%d down=%d limit=%d warn=%d", planID, used, up, down, limit, warn)
	}

	// 解绑：plan_id 清空，traffic 不再变
	if err := svc.AssignToUser(uid, &AssignPlanReq{PlanID: nil}); err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	var pidAfter *int64
	if err := db.QueryRow(`SELECT plan_id FROM users WHERE id = ?`, uid).Scan(&pidAfter); err != nil {
		t.Fatalf("query plan_id: %v", err)
	}
	if pidAfter != nil {
		t.Errorf("解绑后 plan_id 应为 NULL，得到 %v", *pidAfter)
	}
}

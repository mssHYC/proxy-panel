package service

import (
	"testing"

	"proxy-panel/internal/kernel"
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

// TestKernelSync_RestrictedFlagOnPlanUser 已分配套餐的用户即便授权解析为空也应被
// 标记 Restricted=true，避免 inbound 侧把 NodeIDs 空误解释为"全部节点"。
func TestKernelSync_RestrictedFlagOnPlanUser(t *testing.T) {
	db := openSyncTestDB(t)

	// 用户 A：通过 PlanService.AssignToUser 分配一个空套餐 → NodeIDs 空但 Restricted=true
	resA, _ := db.Exec(`INSERT INTO users (uuid, username, protocol, enable) VALUES ('a', 'alice', 'vless', 1)`)
	uidA, _ := resA.LastInsertId()
	resP, _ := db.Exec(`INSERT INTO plans (name, enabled) VALUES ('p_empty', 1)`)
	pid, _ := resP.LastInsertId()
	if err := NewPlanService(db).AssignToUser(uidA, &AssignPlanReq{PlanID: &pid}); err != nil {
		t.Fatalf("AssignToUser: %v", err)
	}

	// 用户 B：从未配置授权 → Restricted=false（保留老兼容）
	resB, _ := db.Exec(`INSERT INTO users (uuid, username, protocol, enable) VALUES ('b', 'bob', 'vless', 1)`)
	_, _ = resB.LastInsertId()

	sync := NewKernelSyncService(db, nil)
	users, err := sync.loadUsers()
	if err != nil {
		t.Fatalf("loadUsers: %v", err)
	}
	got := map[string]kernel.UserConfig{}
	for _, u := range users {
		got[u.UUID] = u
	}
	if !got["a"].Restricted {
		t.Errorf("已分配套餐的用户应 Restricted=true，得 %+v", got["a"])
	}
	if got["b"].Restricted {
		t.Errorf("无任何授权来源的用户应 Restricted=false，得 %+v", got["b"])
	}
}

// TestPlanService_DeleteCascadesPlanID 删除套餐应同步把 users.plan_id 置 NULL。
func TestPlanService_DeleteCascadesPlanID(t *testing.T) {
	db := openSyncTestDB(t)
	resU, _ := db.Exec(`INSERT INTO users (uuid, username, protocol) VALUES ('u', 'alice', 'vless')`)
	uid, _ := resU.LastInsertId()
	resP, _ := db.Exec(`INSERT INTO plans (name, enabled) VALUES ('p1', 1)`)
	pid, _ := resP.LastInsertId()
	if _, err := db.Exec(`UPDATE users SET plan_id = ? WHERE id = ?`, pid, uid); err != nil {
		t.Fatalf("set plan_id: %v", err)
	}

	if err := NewPlanService(db).Delete(pid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var planID *int64
	if err := db.QueryRow(`SELECT plan_id FROM users WHERE id = ?`, uid).Scan(&planID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if planID != nil {
		t.Errorf("删除套餐后 users.plan_id 必须为 NULL，得 %d", *planID)
	}
}

// TestPlanService_DeleteDoesNotGrantAllNodes 回归：仅通过套餐授权的用户，
// 套餐被删除后 plan_id 被 NULL 化，但 users.restricted 必须保留为 1，
// 避免订阅 / kernel sync 把"plan_id NULL && user_nodes 空"再次解释为
// 老兼容"全部节点"兜底，重新打开权限扩大通道。
func TestPlanService_DeleteDoesNotGrantAllNodes(t *testing.T) {
	db := openSyncTestDB(t)
	resU, _ := db.Exec(`INSERT INTO users (uuid, username, protocol, enable) VALUES ('u', 'alice', 'vless', 1)`)
	uid, _ := resU.LastInsertId()
	resP, _ := db.Exec(`INSERT INTO plans (name, traffic_limit, duration_days, enabled) VALUES ('p1', 0, 0, 1)`)
	pid, _ := resP.LastInsertId()

	svc := NewPlanService(db)
	if err := svc.AssignToUser(uid, &AssignPlanReq{PlanID: &pid}); err != nil {
		t.Fatalf("AssignToUser: %v", err)
	}
	if err := svc.Delete(pid); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 删套餐后用户应：plan_id NULL、user_nodes 仍为空、但 restricted=1。
	usrSvc := NewUserService(db)
	hasAuth, err := usrSvc.HasExplicitNodeAuth(uid)
	if err != nil {
		t.Fatalf("HasExplicitNodeAuth: %v", err)
	}
	if !hasAuth {
		t.Fatalf("删套餐后用户的 restricted 标记必须保留，否则会触发 fallback 全部节点")
	}

	// 同时 kernel sync 也必须把该用户视为 Restricted，NodeIDs 空 → 不再注入任何 inbound。
	users, err := NewKernelSyncService(db, nil).loadUsers()
	if err != nil {
		t.Fatalf("loadUsers: %v", err)
	}
	var found bool
	for _, u := range users {
		if u.UUID == "u" {
			found = true
			if !u.Restricted {
				t.Errorf("kernel sync 视角下，被删套餐的用户必须保留 Restricted=true，得 %+v", u)
			}
			if len(u.NodeIDs) != 0 {
				t.Errorf("用户应无可用节点，得 %v", u.NodeIDs)
			}
		}
	}
	if !found {
		t.Fatalf("loadUsers 没返回用户 u")
	}
}

// TestPlanService_AssignDurationZero duration_days=0 视为不限期：
// 清空 expires_at，并把已禁用用户重新启用。
func TestPlanService_AssignDurationZero(t *testing.T) {
	db := openSyncTestDB(t)
	resU, _ := db.Exec(`INSERT INTO users (uuid, username, protocol, enable, expires_at)
		VALUES ('u', 'alice', 'vless', 0, '2020-01-01 00:00:00')`)
	uid, _ := resU.LastInsertId()

	resP, _ := db.Exec(`INSERT INTO plans (name, traffic_limit, duration_days, enabled)
		VALUES ('forever', 0, 0, 1)`)
	pid, _ := resP.LastInsertId()

	if err := NewPlanService(db).AssignToUser(uid, &AssignPlanReq{PlanID: &pid}); err != nil {
		t.Fatalf("AssignToUser: %v", err)
	}
	var enable int
	var exp *string
	if err := db.QueryRow(`SELECT enable, expires_at FROM users WHERE id = ?`, uid).Scan(&enable, &exp); err != nil {
		t.Fatalf("query: %v", err)
	}
	if enable != 1 {
		t.Errorf("不限期套餐应重新启用用户，enable=%d", enable)
	}
	if exp != nil {
		t.Errorf("不限期套餐应清空 expires_at，得 %v", *exp)
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

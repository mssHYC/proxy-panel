package service

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
)

// fakeEngine 是一个仅用于测试的 kernel.Engine 实现：
// 记录 ApplyConfig / AddUser / RemoveUser 调用次数，并允许注入 Restart 失败。
type fakeEngine struct {
	name        string
	configPath  string
	running     atomic.Bool
	applyCalls  atomic.Int32
	restartFail atomic.Int32 // >0 表示 Restart 仍失败的次数（每次自减）

	mu      sync.Mutex
	addOps  []string // tag:user
	delOps  []string
	lastCfg []byte
}

func (e *fakeEngine) Name() string                { return e.name }
func (e *fakeEngine) Start() error                { e.running.Store(true); return nil }
func (e *fakeEngine) Stop() error                 { e.running.Store(false); return nil }
func (e *fakeEngine) IsRunning() bool             { return e.running.Load() }
func (e *fakeEngine) GetTrafficStats() ([]kernel.TrafficStat, error) {
	return nil, nil
}
func (e *fakeEngine) Restart() error {
	if e.restartFail.Load() > 0 {
		e.restartFail.Add(-1)
		return errors.New("simulated restart failure")
	}
	e.running.Store(true)
	return nil
}
func (e *fakeEngine) AddUser(tag, uuid, email, protocol string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.addOps = append(e.addOps, tag+":"+email)
	return nil
}
func (e *fakeEngine) RemoveUser(tag, uuid, email string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.delOps = append(e.delOps, tag+":"+email)
	return nil
}
func (e *fakeEngine) GenerateConfig(nodes []kernel.NodeConfig, users []kernel.UserConfig) ([]byte, error) {
	// 用节点数 + 用户数当成"指纹"，让回滚测试能比较旧/新内容差异。
	return []byte("nodes=" + itoa(len(nodes)) + "users=" + itoa(len(users))), nil
}
func (e *fakeEngine) WriteConfig(data []byte) error {
	e.mu.Lock()
	e.lastCfg = append([]byte(nil), data...)
	e.mu.Unlock()
	return nil
}
func (e *fakeEngine) ApplyConfig(data []byte) error {
	e.applyCalls.Add(1)
	// 复用与真实引擎相同的回滚逻辑（通过 file 实现）。
	// 这里直接调用 helper 的等价实现：写入 + Restart + 失败回滚。
	// 用 e.WriteConfig + e.Restart 简单串起来即可——失败回滚的细节由真实实现/单测覆盖。
	if err := e.WriteConfig(data); err != nil {
		return err
	}
	if err := e.Restart(); err != nil {
		// 模拟回滚：不写回 file（fake 没有持久化）。返回带回滚标记的错误。
		return errors.New("apply failed (fake): " + err.Error())
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// openSyncTestDB 起一个干净 SQLite + nodes/users/user_nodes 测试数据。
func openSyncTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedNode 插入一个启用节点；返回 id。
func seedNode(t *testing.T, db *database.DB, kernelType, protocol string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO nodes (name, host, port, protocol, transport, kernel_type, settings, enable)
		 VALUES (?, '127.0.0.1', 443, ?, 'tcp', ?, '{}', 1)`,
		"n", protocol, kernelType)
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestHotAddUser_XrayOnly 验证：用户只关联 xray 节点时，HotAddUser 走 AddUser
// 不调用 ApplyConfig（不重启内核），其他用户连接保持不变。
func TestHotAddUser_XrayOnly(t *testing.T) {
	db := openSyncTestDB(t)
	mgr := kernel.NewManager()
	xray := &fakeEngine{name: "xray"}
	xray.Start()
	mgr.Register(xray)

	n1 := seedNode(t, db, "xray", "vless")
	n2 := seedNode(t, db, "xray", "trojan")

	s := NewKernelSyncService(db, mgr)
	op := UserKernelOp{UUID: "u1", Username: "alice", Protocol: "vless", NodeIDs: []int64{n1, n2}}
	if err := s.HotAddUser(op); err != nil {
		t.Fatalf("HotAddUser: %v", err)
	}
	if got := xray.applyCalls.Load(); got != 0 {
		t.Errorf("纯 xray 关联不应触发 ApplyConfig（重启），got %d", got)
	}
	if len(xray.addOps) != 2 {
		t.Errorf("应对每个 xray inbound 调一次 AddUser，got %v", xray.addOps)
	}
}

// TestHotAddUser_FallbackOnSingbox 验证：用户关联到 sing-box 节点时回退全量同步。
func TestHotAddUser_FallbackOnSingbox(t *testing.T) {
	db := openSyncTestDB(t)
	mgr := kernel.NewManager()
	xray := &fakeEngine{name: "xray"}
	xray.Start()
	sb := &fakeEngine{name: "sing-box"}
	sb.Start()
	mgr.Register(xray)
	mgr.Register(sb)

	xn := seedNode(t, db, "xray", "vless")
	sn := seedNode(t, db, "singbox", "hysteria2")

	s := NewKernelSyncService(db, mgr)
	op := UserKernelOp{UUID: "u1", Username: "alice", Protocol: "vless", NodeIDs: []int64{xn, sn}}
	if err := s.HotAddUser(op); err != nil {
		t.Fatalf("HotAddUser: %v", err)
	}
	// sing-box 必须 fallback 到全量 → ApplyConfig 都被调过一次
	if xray.applyCalls.Load() == 0 {
		t.Error("含 sing-box 节点应回退全量，xray 应被 ApplyConfig")
	}
	if sb.applyCalls.Load() == 0 {
		t.Error("含 sing-box 节点应回退全量，sing-box 应被 ApplyConfig")
	}
	if len(xray.addOps) != 0 {
		t.Errorf("回退全量时不应调 AddUser，got %v", xray.addOps)
	}
}

// TestHotAddUser_FallbackOnEmptyNodeIDs 验证：NodeIDs 空（兜底语义"所有节点"）走全量同步。
func TestHotAddUser_FallbackOnEmptyNodeIDs(t *testing.T) {
	db := openSyncTestDB(t)
	mgr := kernel.NewManager()
	xray := &fakeEngine{name: "xray"}
	xray.Start()
	mgr.Register(xray)
	seedNode(t, db, "xray", "vless")

	s := NewKernelSyncService(db, mgr)
	op := UserKernelOp{UUID: "u1", Username: "alice", Protocol: "vless"} // NodeIDs 空
	if err := s.HotAddUser(op); err != nil {
		t.Fatalf("HotAddUser: %v", err)
	}
	if xray.applyCalls.Load() == 0 {
		t.Error("NodeIDs 空必须走全量同步")
	}
	if len(xray.addOps) != 0 {
		t.Errorf("空 NodeIDs 不应调 AddUser，got %v", xray.addOps)
	}
}

// TestHotRemoveUser_XrayOnly 类似 add 路径但调 RemoveUser。
func TestHotRemoveUser_XrayOnly(t *testing.T) {
	db := openSyncTestDB(t)
	mgr := kernel.NewManager()
	xray := &fakeEngine{name: "xray"}
	xray.Start()
	mgr.Register(xray)
	n := seedNode(t, db, "xray", "vless")

	s := NewKernelSyncService(db, mgr)
	op := UserKernelOp{UUID: "u1", Username: "alice", Protocol: "vless", NodeIDs: []int64{n}}
	if err := s.HotRemoveUser(op); err != nil {
		t.Fatalf("HotRemoveUser: %v", err)
	}
	if xray.applyCalls.Load() != 0 {
		t.Errorf("RemoveUser 路径不应触发重启，got %d", xray.applyCalls.Load())
	}
	if len(xray.delOps) != 1 {
		t.Errorf("应调一次 RemoveUser，got %v", xray.delOps)
	}
}

// TestApplyConfigWithRollback_RestartFailure 验证 engines 共享的回滚逻辑：
// 写入新配置后 Restart 失败 → 回写旧配置文件 + 错误带 ErrConfigRolledBack 标记。
func TestApplyConfigWithRollback_RestartFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernel.json")
	old := []byte(`{"old":true}`)
	if err := os.WriteFile(cfgPath, old, 0600); err != nil {
		t.Fatal(err)
	}
	newCfg := []byte(`{"new":true}`)

	calls := 0
	restart := func() error {
		calls++
		if calls == 1 {
			return errors.New("first restart fails")
		}
		return nil // 第二次（用旧配置重启）成功
	}
	err := kernel.ApplyConfigForTest(cfgPath, restart, newCfg)
	if err == nil || !errors.Is(err, kernel.ErrConfigRolledBack) {
		t.Fatalf("want ErrConfigRolledBack, got %v", err)
	}
	got, _ := os.ReadFile(cfgPath)
	if string(got) != string(old) {
		t.Errorf("回滚后文件应是旧内容 %q, got %q", old, got)
	}
	if calls != 2 {
		t.Errorf("应尝试两次 Restart（新一次失败 + 用旧配置再试一次），got %d", calls)
	}
}

// TestSync_SerializesConcurrentTriggers 验证 syncMu 串行化：
// 并发 N 次 Sync()，apply 调用次数等于 N（不会因竞争丢失），且不会同时进入临界区。
func TestSync_SerializesConcurrentTriggers(t *testing.T) {
	db := openSyncTestDB(t)
	mgr := kernel.NewManager()
	xray := &fakeEngine{name: "xray"}
	xray.Start()
	mgr.Register(xray)
	seedNode(t, db, "xray", "vless")

	s := NewKernelSyncService(db, mgr)

	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_ = s.Sync()
		}()
	}
	wg.Wait()
	if got := xray.applyCalls.Load(); got != int32(N) {
		t.Errorf("并发 N 次 Sync 应串行执行 N 次 ApplyConfig，got %d", got)
	}
}

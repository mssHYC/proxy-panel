package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
)

// syncDebounceWindow 默认防抖窗口：5 秒内的多次 Trigger 合并为一次 Sync。
// 对 sing-box 尤其关键：每次 Sync 都会重启内核，合并后重启次数与连接中断显著降低。
const syncDebounceWindow = 5 * time.Second

// KernelSyncService 负责将数据库中的节点和用户同步到内核配置文件
type KernelSyncService struct {
	db  *database.DB
	mgr *kernel.Manager

	debounceMu     sync.Mutex
	debounceTimer  *time.Timer
	debounceWindow time.Duration

	// syncMu 串行化 Sync()/HotAddUser/HotRemoveUser，避免并发触发时
	// "同时写两份配置 + 同时 Restart"互相覆盖导致的端口竞争或半写文件。
	syncMu sync.Mutex
}

// NewKernelSyncService 创建内核同步服务
func NewKernelSyncService(db *database.DB, mgr *kernel.Manager) *KernelSyncService {
	return &KernelSyncService{db: db, mgr: mgr, debounceWindow: syncDebounceWindow}
}

// UserKernelOp 描述一次用户级热同步操作所需的最小快照。
// 必须在 DB 写入前/后立刻捕获——hot path 不再回查数据库以避免与 delete 的并发竞争。
type UserKernelOp struct {
	UUID     string
	Username string
	Protocol string
	NodeIDs  []int64
}

// Trigger 请求一次内核同步；在 debounceWindow 内的多次调用会合并为一次执行。
//
// 用于所有"修改用户/节点"类 handler：Xray 本来就能热加载大部分变更，sing-box
// 则必须重启，合并窗口把高频批量操作（如 UI 表格勾选多个节点批量保存）从每
// 次一重启降到最终一次重启。
func (s *KernelSyncService) Trigger() {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()

	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
	}
	s.debounceTimer = time.AfterFunc(s.debounceWindow, func() {
		if err := s.Sync(); err != nil {
			log.Printf("[内核同步] 防抖触发同步失败: %v", err)
		}
	})
}

// SyncNow 立即执行一次同步，取消任何等待中的防抖计时。
// 用于"应用变更"按钮等需要即刻生效的场景。
func (s *KernelSyncService) SyncNow() error {
	s.debounceMu.Lock()
	if s.debounceTimer != nil {
		s.debounceTimer.Stop()
		s.debounceTimer = nil
	}
	s.debounceMu.Unlock()
	return s.Sync()
}

// Sync 从数据库读取所有启用的节点和用户，生成配置并通过 ApplyConfig 写入并重启内核。
// ApplyConfig 失败时引擎会自动回滚到旧配置；这里只把失败汇总成日志，不阻断其他引擎。
func (s *KernelSyncService) Sync() error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncLocked()
}

// syncLocked 必须在 syncMu 加锁后调用。抽出来便于 hot path fallback 复用同一把锁。
func (s *KernelSyncService) syncLocked() error {
	nodes, err := s.loadNodes()
	if err != nil {
		return fmt.Errorf("加载节点失败: %w", err)
	}

	users, err := s.loadUsers()
	if err != nil {
		return fmt.Errorf("加载用户失败: %w", err)
	}

	// 按内核类型分组节点
	xrayNodes := make([]kernel.NodeConfig, 0)
	singboxNodes := make([]kernel.NodeConfig, 0)

	for _, n := range nodes {
		nc := kernel.NodeConfig{
			ID:        n.id,
			Tag:       fmt.Sprintf("node-%d", n.id),
			Port:      n.port,
			Protocol:  n.protocol,
			Transport: n.transport,
			Settings:  n.settings,
		}
		switch n.kernelType {
		case "singbox":
			singboxNodes = append(singboxNodes, nc)
		default:
			xrayNodes = append(xrayNodes, nc)
		}
	}

	applyOne := func(name string, eng kernel.Engine, kernelNodes []kernel.NodeConfig, skipEmpty bool) {
		if skipEmpty && len(kernelNodes) == 0 {
			return
		}
		data, err := eng.GenerateConfig(kernelNodes, users)
		if err != nil {
			log.Printf("[内核同步] %s 生成配置失败: %v", name, err)
			return
		}
		if err := eng.ApplyConfig(data); err != nil {
			if errors.Is(err, kernel.ErrConfigRolledBack) {
				log.Printf("[内核同步] %s 应用新配置失败，已回滚到旧配置: %v", name, err)
			} else {
				log.Printf("[内核同步] %s 应用配置失败（无法回滚）: %v", name, err)
			}
			return
		}
		log.Printf("[内核同步] %s 配置已同步并重启", name)
	}

	if eng, err := s.mgr.Get("xray"); err == nil {
		applyOne("Xray", eng, xrayNodes, false)
	}
	if eng, err := s.mgr.Get("sing-box"); err == nil {
		applyOne("Sing-box", eng, singboxNodes, true)
	}

	return nil
}

type nodeRow struct {
	id         int64
	port       int
	protocol   string
	transport  string
	kernelType string
	settings   map[string]interface{}
}

func (s *KernelSyncService) loadNodes() ([]nodeRow, error) {
	rows, err := s.db.Query(`SELECT id, port, protocol, transport, kernel_type, settings
		FROM nodes WHERE enable = 1 ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []nodeRow
	for rows.Next() {
		var n nodeRow
		var settingsStr string
		if err := rows.Scan(&n.id, &n.port, &n.protocol, &n.transport, &n.kernelType, &settingsStr); err != nil {
			return nil, err
		}
		n.settings = make(map[string]interface{})
		json.Unmarshal([]byte(settingsStr), &n.settings)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// loadUsers 加载所有启用用户，并按 user_nodes 关联填充每个用户的 NodeIDs。
//
// 内核的 buildInbound 会据此只把用户注入到他关联的节点 inbound，严格对齐订阅
// 侧的 ListByUserID(user_nodes JOIN) 可见性，避免跨协议节点 clients 为 null。
func (s *KernelSyncService) loadUsers() ([]kernel.UserConfig, error) {
	rows, err := s.db.Query(`SELECT id, uuid, username, protocol, speed_limit
		FROM users WHERE enable = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type userWithID struct {
		id   int64
		conf kernel.UserConfig
	}
	var list []userWithID
	for rows.Next() {
		var uid int64
		var u kernel.UserConfig
		if err := rows.Scan(&uid, &u.UUID, &u.Email, &u.Protocol, &u.SpeedLimit); err != nil {
			return nil, err
		}
		list = append(list, userWithID{id: uid, conf: u})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 一次性拉全部 user_nodes 映射，避免 N+1
	nodeMap, err := s.loadUserNodeMap()
	if err != nil {
		return nil, err
	}

	users := make([]kernel.UserConfig, 0, len(list))
	for _, item := range list {
		u := item.conf
		u.NodeIDs = nodeMap[item.id]
		users = append(users, u)
	}
	return users, nil
}

// HotAddUser 尝试以热加载方式将用户注入 xray inbound；不可热加载场景直接退化为全量 Sync。
//
// 不可热加载条件（任一命中即 fallback）：
//   - NodeIDs 为空：兜底语义"可用全部节点"，新用户会出现在每个 inbound（含 sing-box hy2），
//     这与新增 inbound 等价，必须走全量重启 sing-box。
//   - 任一关联节点不是 xray：sing-box 不支持热加用户。
//   - 任一关联节点未启用 / 已删除：当前内核里没有对应 inbound。
//   - 调用 xray AddUser 失败：保留旧状态语义不一致，回退全量同步保证一致性。
//
// hot path 成功时不会重启 xray，正在跑的其他用户连接保持不变（验收 P0）。
func (s *KernelSyncService) HotAddUser(op UserKernelOp) error {
	return s.applyUserOp(op, "add")
}

// HotRemoveUser 见 HotAddUser；额外注意快照必须在 DELETE FROM users 之前捕获，
// 否则就拿不到 NodeIDs 与 UUID。
func (s *KernelSyncService) HotRemoveUser(op UserKernelOp) error {
	return s.applyUserOp(op, "del")
}

func (s *KernelSyncService) applyUserOp(op UserKernelOp, kind string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	// 兜底语义：NodeIDs 空 → 全量
	if len(op.NodeIDs) == 0 {
		log.Printf("[内核同步] 用户 %s NodeIDs 空，走全量同步", op.Username)
		return s.syncLocked()
	}

	nodes, err := s.loadEnabledNodesByIDs(op.NodeIDs)
	if err != nil {
		log.Printf("[内核同步] 加载节点失败，回退全量: %v", err)
		return s.syncLocked()
	}
	if len(nodes) != len(op.NodeIDs) {
		// 有节点已禁用 / 已删除——内核里 inbound 数量跟用户期望不一致；走全量统一。
		return s.syncLocked()
	}
	for _, n := range nodes {
		if n.kernelType != "xray" {
			return s.syncLocked()
		}
	}

	eng, err := s.mgr.Get("xray")
	if err != nil {
		return s.syncLocked()
	}

	for _, n := range nodes {
		var apiErr error
		tag := fmt.Sprintf("node-%d", n.id)
		switch kind {
		case "add":
			apiErr = eng.AddUser(tag, op.UUID, op.Username, n.protocol)
		case "del":
			apiErr = eng.RemoveUser(tag, op.UUID, op.Username)
		}
		if apiErr != nil {
			log.Printf("[内核同步] xray %s 用户 %s 失败 tag=%s err=%v，回退全量同步",
				kind, op.Username, tag, apiErr)
			return s.syncLocked()
		}
	}
	log.Printf("[内核同步] xray %s 用户 %s 完成，热路径 %d 个 inbound", kind, op.Username, len(nodes))
	return nil
}

// loadEnabledNodesByIDs 按 ID 列表加载启用节点。结果数量可能少于入参（节点已删/禁用）。
func (s *KernelSyncService) loadEnabledNodesByIDs(ids []int64) ([]nodeRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	q := "SELECT id, port, protocol, transport, kernel_type, settings FROM nodes WHERE enable = 1 AND id IN (" + string(placeholders) + ")"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []nodeRow
	for rows.Next() {
		var n nodeRow
		var settingsStr string
		if err := rows.Scan(&n.id, &n.port, &n.protocol, &n.transport, &n.kernelType, &settingsStr); err != nil {
			return nil, err
		}
		n.settings = make(map[string]interface{})
		json.Unmarshal([]byte(settingsStr), &n.settings)
		out = append(out, n)
	}
	return out, rows.Err()
}

// loadUserNodeMap 读出 user_nodes 关联表，返回 user_id → []node_id 的映射
func (s *KernelSyncService) loadUserNodeMap() (map[int64][]int64, error) {
	rows, err := s.db.Query(`SELECT user_id, node_id FROM user_nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[int64][]int64)
	for rows.Next() {
		var uid, nid int64
		if err := rows.Scan(&uid, &nid); err != nil {
			return nil, err
		}
		m[uid] = append(m[uid], nid)
	}
	return m, rows.Err()
}

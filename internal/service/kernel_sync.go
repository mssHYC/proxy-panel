package service

import (
	"encoding/json"
	"fmt"
	"log"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
)

// KernelSyncService 负责将数据库中的节点和用户同步到内核配置文件
type KernelSyncService struct {
	db  *database.DB
	mgr *kernel.Manager
}

// NewKernelSyncService 创建内核同步服务
func NewKernelSyncService(db *database.DB, mgr *kernel.Manager) *KernelSyncService {
	return &KernelSyncService{db: db, mgr: mgr}
}

// Sync 从数据库读取所有启用的节点和用户，生成配置并写入文件，重启内核
func (s *KernelSyncService) Sync() error {
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

	// 同步 Xray
	if eng, err := s.mgr.Get("xray"); err == nil {
		data, err := eng.GenerateConfig(xrayNodes, users)
		if err != nil {
			return fmt.Errorf("生成 Xray 配置失败: %w", err)
		}
		if err := eng.WriteConfig(data); err != nil {
			return fmt.Errorf("写入 Xray 配置失败: %w", err)
		}
		if err := eng.Restart(); err != nil {
			log.Printf("[内核同步] Xray 重启失败: %v", err)
		} else {
			log.Println("[内核同步] Xray 配置已同步并重启")
		}
	}

	// 同步 Sing-box
	if eng, err := s.mgr.Get("sing-box"); err == nil {
		if len(singboxNodes) > 0 {
			data, err := eng.GenerateConfig(singboxNodes, users)
			if err != nil {
				return fmt.Errorf("生成 Sing-box 配置失败: %w", err)
			}
			if err := eng.WriteConfig(data); err != nil {
				return fmt.Errorf("写入 Sing-box 配置失败: %w", err)
			}
			if err := eng.Restart(); err != nil {
				log.Printf("[内核同步] Sing-box 重启失败: %v", err)
			} else {
				log.Println("[内核同步] Sing-box 配置已同步并重启")
			}
		}
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

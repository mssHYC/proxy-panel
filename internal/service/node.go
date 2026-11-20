package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
	"proxy-panel/internal/service/firewall"
	"proxy-panel/internal/website"
)

var ErrInvalidNodeSettings = errors.New("节点设置无效")

func validateNodeSettings(protocol, transport, engine string, port int, settings string) error {
	var values map[string]interface{}
	if settings == "" {
		settings = "{}"
	}
	if err := json.Unmarshal([]byte(settings), &values); err != nil || values == nil {
		return fmt.Errorf("%w: settings 必须是 JSON 对象", ErrInvalidNodeSettings)
	}
	if err := kernel.ValidateNodeSecurity(kernel.NodeConfig{Protocol: protocol, Transport: transport, Port: port, Settings: values}, engine); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidNodeSettings, err)
	}
	return nil
}

// CreateNodeReq 创建节点请求
type CreateNodeReq struct {
	Name       string `json:"name" binding:"required"`
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	Transport  string `json:"transport"`
	KernelType string `json:"kernel_type"`
	Settings   string `json:"settings"`
	SortOrder  int    `json:"sort_order"`
}

// UpdateNodeReq 更新节点请求（指针字段实现部分更新）
type UpdateNodeReq struct {
	Name       *string `json:"name"`
	Host       *string `json:"host"`
	Port       *int    `json:"port"`
	Protocol   *string `json:"protocol"`
	Transport  *string `json:"transport"`
	KernelType *string `json:"kernel_type"`
	Settings   *string `json:"settings"`
	Enable     *bool   `json:"enable"`
	SortOrder  *int    `json:"sort_order"`
}

// NodeService 节点业务逻辑
type NodeService struct {
	db           *database.DB
	fw           *firewall.Service
	realityCheck func(context.Context, map[string]interface{}) (*RealityCheck, error)
	mutationMu   sync.Mutex
	syncSvc      *KernelSyncService
	website      *website.Manager
	xrayConfig   string
}

// NewNodeService 创建节点服务
func NewNodeService(db *database.DB, fw *firewall.Service) *NodeService {
	return &NodeService{db: db, fw: fw}
}

// List 获取所有节点
func (s *NodeService) List() ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at,
		last_check_at, last_check_ok, last_check_err, fail_count
		FROM nodes ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询节点列表失败: %w", err)
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		if err := scanNode(rows, &n); err != nil {
			return nil, fmt.Errorf("扫描节点数据失败: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// ListEnabled 获取已启用的节点（用于订阅）
func (s *NodeService) ListEnabled() ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at,
		last_check_at, last_check_ok, last_check_err, fail_count
		FROM nodes WHERE enable = 1 ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询启用节点失败: %w", err)
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		if err := scanNode(rows, &n); err != nil {
			return nil, fmt.Errorf("扫描节点数据失败: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// GetByID 根据 ID 获取节点
func (s *NodeService) GetByID(id int64) (*model.Node, error) {
	row := s.db.QueryRow(`SELECT id, name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at,
		last_check_at, last_check_ok, last_check_err, fail_count
		FROM nodes WHERE id = ?`, id)

	var n model.Node
	if err := scanNodeRow(row, &n); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询节点失败: %w", err)
	}
	return &n, nil
}

// Create 创建节点
func (s *NodeService) Create(req *CreateNodeReq) (*model.Node, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.syncSvc != nil {
		s.syncSvc.syncMu.Lock()
		defer s.syncSvc.syncMu.Unlock()
	}
	if err := s.recoverManagedWebsiteLocked(); err != nil {
		return nil, err
	}
	now := time.Now()

	// 设置默认值
	transport := req.Transport
	if transport == "" {
		transport = "tcp"
	}
	kernelType := req.KernelType
	if kernelType == "" {
		kernelType = "xray"
	}
	settings := req.Settings
	if settings == "" {
		settings = "{}"
	}

	if err := validateNodeSettings(req.Protocol, transport, kernelType, req.Port, settings); err != nil {
		return nil, err
	}

	if s.website != nil {
		candidate := model.Node{Host: req.Host, Port: req.Port, Protocol: req.Protocol, Transport: transport, KernelType: kernelType, Settings: settings, Enable: true}
		if err := s.website.CheckCreate(candidate); err != nil {
			return nil, err
		}
	}
	if err := s.validateRealityChange("", settings, true); err != nil {
		return nil, err
	}

	result, err := s.db.Exec(`INSERT INTO nodes (name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		req.Name, req.Host, req.Port, req.Protocol, transport,
		kernelType, settings, req.SortOrder, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建节点失败: %w", err)
	}

	id, _ := result.LastInsertId()
	node, err := s.GetByID(id)
	if err != nil || node == nil {
		return node, err
	}
	go s.applyFirewallPort(node.Port, allowPort)
	return node, nil
}

// Update 更新节点（部分更新）
func (s *NodeService) Update(id int64, req *UpdateNodeReq) (*model.Node, error) {
	node, _, err := s.UpdateAndSync(id, req)
	return node, err
}

func (s *NodeService) updateStored(old *model.Node, candidate model.Node, req *UpdateNodeReq) (*model.Node, error) {
	sets := []string{}
	args := []interface{}{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Host != nil {
		sets = append(sets, "host = ?")
		args = append(args, *req.Host)
	}
	if req.Port != nil {
		sets = append(sets, "port = ?")
		args = append(args, *req.Port)
	}
	if req.Protocol != nil {
		sets = append(sets, "protocol = ?")
		args = append(args, *req.Protocol)
	}
	if req.Transport != nil {
		sets = append(sets, "transport = ?")
		args = append(args, *req.Transport)
	}
	if req.KernelType != nil {
		sets = append(sets, "kernel_type = ?")
		args = append(args, *req.KernelType)
	}
	if req.Settings != nil {
		sets = append(sets, "settings = ?")
		args = append(args, *req.Settings)
	}
	if req.Enable != nil {
		sets = append(sets, "enable = ?")
		args = append(args, *req.Enable)
	}
	if req.SortOrder != nil {
		sets = append(sets, "sort_order = ?")
		args = append(args, *req.SortOrder)
	}

	if len(sets) == 0 {
		return old, nil
	}

	sets = append(sets, "updated_at = ?")
	candidate.UpdatedAt = time.Now()
	args = append(args, candidate.UpdatedAt)
	args = append(args, old.ID)

	query := "UPDATE nodes SET "
	for i, part := range sets {
		if i > 0 {
			query += ", "
		}
		query += part
	}
	query += " WHERE id = ?"

	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("更新节点失败: %w", err)
	}

	if old.Enable && (!candidate.Enable || old.Port != candidate.Port) {
		go s.applyFirewallPort(old.Port, revokePort)
	}
	if candidate.Enable && (!old.Enable || old.Port != candidate.Port) {
		go s.applyFirewallPort(candidate.Port, allowPort)
	}

	return &candidate, nil
}

// Delete 删除节点
func (s *NodeService) Delete(id int64) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.syncSvc != nil {
		s.syncSvc.syncMu.Lock()
		defer s.syncSvc.syncMu.Unlock()
	}
	if err := s.recoverManagedWebsiteLocked(); err != nil {
		return err
	}
	old, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("节点不存在")
	}
	if s.website != nil {
		if err := s.website.CheckDelete(id); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id); err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	if old.Enable {
		go s.applyFirewallPort(old.Port, revokePort)
	}
	return nil
}

// Count 统计节点数量
func (s *NodeService) Count() (total int, enabled int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("统计节点总数失败: %w", err)
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE enable = 1").Scan(&enabled)
	if err != nil {
		return 0, 0, fmt.Errorf("统计启用节点数失败: %w", err)
	}
	return
}

// ListByUserID 获取用户关联的已启用节点（用于订阅生成）。
// 可见性 = user_nodes 直接关联 ∪ 套餐授权（user.plan_id → plan_node_groups → node_group_members）。
func (s *NodeService) ListByUserID(userID int64) ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT DISTINCT n.id, n.name, n.host, n.port, n.protocol, n.transport,
		n.kernel_type, n.settings, n.enable, n.sort_order, n.created_at, n.updated_at,
		n.last_check_at, n.last_check_ok, n.last_check_err, n.fail_count
		FROM nodes n
		WHERE n.enable = 1 AND (
			n.id IN (SELECT node_id FROM user_nodes WHERE user_id = ?)
			OR n.id IN (
				SELECT ngm.node_id
				FROM users u
				JOIN plans p ON p.id = u.plan_id AND p.enabled = 1
				JOIN plan_node_groups png ON png.plan_id = p.id
				JOIN node_group_members ngm ON ngm.node_group_id = png.node_group_id
				WHERE u.id = ?
			)
		)
		ORDER BY n.sort_order ASC, n.id ASC`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("查询用户节点失败: %w", err)
	}
	defer rows.Close()
	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		if err := scanNode(rows, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func scanNode(rows *sql.Rows, n *model.Node) error {
	var lastAt sql.NullTime
	var lastErr sql.NullString
	if err := rows.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
		&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt,
		&lastAt, &n.LastCheckOK, &lastErr, &n.FailCount); err != nil {
		return err
	}
	if lastAt.Valid {
		t := lastAt.Time
		n.LastCheckAt = &t
	}
	if lastErr.Valid {
		n.LastCheckErr = lastErr.String
	}
	return nil
}

func scanNodeRow(row *sql.Row, n *model.Node) error {
	var lastAt sql.NullTime
	var lastErr sql.NullString
	if err := row.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
		&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt,
		&lastAt, &n.LastCheckOK, &lastErr, &n.FailCount); err != nil {
		return err
	}
	if lastAt.Valid {
		t := lastAt.Time
		n.LastCheckAt = &t
	}
	if lastErr.Valid {
		n.LastCheckErr = lastErr.String
	}
	return nil
}

// FirewallEnabled 供 handler 判断是否需要返回 firewall_warning
func (s *NodeService) FirewallEnabled() bool {
	return s.fw != nil && s.fw.Enabled()
}

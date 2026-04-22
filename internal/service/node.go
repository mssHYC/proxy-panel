package service

import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
	"proxy-panel/internal/service/firewall"
)

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
	db *database.DB
	fw *firewall.Service
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

	result, err := s.db.Exec(`INSERT INTO nodes (name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at,
		last_check_at, last_check_ok, last_check_err, fail_count)
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
	go s.fw.Allow(node.Port)
	return node, nil
}

// Update 更新节点（部分更新）
func (s *NodeService) Update(id int64, req *UpdateNodeReq) (*model.Node, error) {
	old, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, nil
	}

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
	args = append(args, time.Now())
	args = append(args, id)

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

	s.syncFirewallOnUpdate(old, req)

	return s.GetByID(id)
}

// syncFirewallOnUpdate 按新旧状态差异触发防火墙操作；所有调用都是异步的
// 规则：
//  1. enable 由 true 变 false：撤销旧端口
//  2. enable 由 false 变 true：放行当前端口（可能已被 port 变更）
//  3. enable 未变 且 enable=true：端口变化则撤旧+放新；否则 no-op
//  4. enable 未变 且 enable=false：不操作（节点本就不在防火墙中）
func (s *NodeService) syncFirewallOnUpdate(old *model.Node, req *UpdateNodeReq) {
	newEnable := old.Enable
	if req.Enable != nil {
		newEnable = *req.Enable
	}
	newPort := old.Port
	if req.Port != nil {
		newPort = *req.Port
	}

	switch {
	case old.Enable && !newEnable:
		// 关闭节点：撤销旧端口
		go s.fw.Revoke(old.Port)
	case !old.Enable && newEnable:
		// 重新启用：放行当前端口
		go s.fw.Allow(newPort)
	case old.Enable && newEnable && old.Port != newPort:
		// 仅改端口
		go func(oldPort, port int) {
			s.fw.Revoke(oldPort)
			s.fw.Allow(port)
		}(old.Port, newPort)
	}
}

// Delete 删除节点
func (s *NodeService) Delete(id int64) error {
	old, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("节点不存在")
	}
	if _, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id); err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	go s.fw.Revoke(old.Port)
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

// ListByUserID 获取用户关联的已启用节点（用于订阅生成）
func (s *NodeService) ListByUserID(userID int64) ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT n.id, n.name, n.host, n.port, n.protocol, n.transport,
		n.kernel_type, n.settings, n.enable, n.sort_order, n.created_at, n.updated_at,
		n.last_check_at, n.last_check_ok, n.last_check_err, n.fail_count
		FROM nodes n
		INNER JOIN user_nodes un ON un.node_id = n.id
		WHERE un.user_id = ? AND n.enable = 1
		ORDER BY n.sort_order ASC, n.id ASC`, userID)
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

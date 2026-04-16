package service

import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
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
}

// NewNodeService 创建节点服务
func NewNodeService(db *database.DB) *NodeService {
	return &NodeService{db: db}
}

// List 获取所有节点
func (s *NodeService) List() ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, protocol, transport,
		kernel_type, settings, enable, sort_order, created_at, updated_at
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
		kernel_type, settings, enable, sort_order, created_at, updated_at
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
		kernel_type, settings, enable, sort_order, created_at, updated_at
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
		kernel_type, settings, enable, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		req.Name, req.Host, req.Port, req.Protocol, transport,
		kernelType, settings, req.SortOrder, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建节点失败: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetByID(id)
}

// Update 更新节点（部分更新）
func (s *NodeService) Update(id int64, req *UpdateNodeReq) (*model.Node, error) {
	node, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if node == nil {
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
		return node, nil
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, id)

	query := "UPDATE nodes SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = ?"

	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("更新节点失败: %w", err)
	}

	return s.GetByID(id)
}

// Delete 删除节点
func (s *NodeService) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("节点不存在")
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

// ListByUserID 获取用户关联的已启用节点（用于订阅生成）
func (s *NodeService) ListByUserID(userID int64) ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT n.id, n.name, n.host, n.port, n.protocol, n.transport,
		n.kernel_type, n.settings, n.enable, n.sort_order, n.created_at, n.updated_at
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
	return rows.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
		&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
}

func scanNodeRow(row *sql.Row, n *model.Node) error {
	return row.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
		&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
}

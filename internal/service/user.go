package service

import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"

	"github.com/google/uuid"
)

// CreateUserReq 创建用户请求
type CreateUserReq struct {
	Username     string  `json:"username" binding:"required"`
	Email        string  `json:"email"`
	Protocol     string  `json:"protocol"`
	NodeIDs      []int64 `json:"node_ids"`
	TrafficLimit int64   `json:"traffic_limit"`
	SpeedLimit   int64   `json:"speed_limit"`
	ResetDay     int     `json:"reset_day"`
	ResetCron    string  `json:"reset_cron"`
	ExpiresAt    string  `json:"expires_at"`
}

// UpdateUserReq 更新用户请求（指针字段实现部分更新）
type UpdateUserReq struct {
	Username     *string  `json:"username"`
	Email        *string  `json:"email"`
	Protocol     *string  `json:"protocol"`
	NodeIDs      *[]int64 `json:"node_ids"`
	TrafficLimit *int64   `json:"traffic_limit"`
	SpeedLimit   *int64   `json:"speed_limit"`
	ResetDay     *int     `json:"reset_day"`
	ResetCron    *string  `json:"reset_cron"`
	Enable       *bool    `json:"enable"`
	ExpiresAt    *string  `json:"expires_at"`
}

// UserService 用户业务逻辑
type UserService struct {
	db *database.DB
}

// NewUserService 创建用户服务
func NewUserService(db *database.DB) *UserService {
	return &UserService{db: db}
}

// getNodeIDs 获取用户关联的节点 ID 列表
func (s *UserService) getNodeIDs(userID int64) ([]int64, error) {
	rows, err := s.db.Query("SELECT node_id FROM user_nodes WHERE user_id = ?", userID)
	if err != nil {
		return []int64{}, err
	}
	defer rows.Close()
	ids := make([]int64, 0) // 空切片序列化为 [] 而非 null
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// setNodeIDs 设置用户关联的节点（先删后插）
func (s *UserService) setNodeIDs(userID int64, nodeIDs []int64) error {
	if _, err := s.db.Exec("DELETE FROM user_nodes WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, nid := range nodeIDs {
		if _, err := s.db.Exec("INSERT INTO user_nodes (user_id, node_id) VALUES (?, ?)", userID, nid); err != nil {
			return err
		}
	}
	return nil
}

// fillNodeIDs 为用户列表填充节点 ID
func (s *UserService) fillNodeIDs(users []model.User) error {
	for i := range users {
		ids, err := s.getNodeIDs(users[i].ID)
		if err != nil {
			return err
		}
		users[i].NodeIDs = ids
	}
	return nil
}

// List 获取所有用户
func (s *UserService) List() ([]model.User, error) {
	rows, err := s.db.Query(`SELECT id, uuid, username, password, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询用户列表失败: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := scanUser(rows, &u); err != nil {
			return nil, fmt.Errorf("扫描用户数据失败: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillNodeIDs(users); err != nil {
		return nil, fmt.Errorf("获取用户节点关联失败: %w", err)
	}
	return users, nil
}

// GetByID 根据 ID 获取用户
func (s *UserService) GetByID(id int64) (*model.User, error) {
	row := s.db.QueryRow(`SELECT id, uuid, username, password, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users WHERE id = ?`, id)

	var u model.User
	if err := scanUserRow(row, &u); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	ids, _ := s.getNodeIDs(u.ID)
	u.NodeIDs = ids
	return &u, nil
}

// GetByUUID 根据 UUID 获取用户（用于订阅查询）
func (s *UserService) GetByUUID(uid string) (*model.User, error) {
	row := s.db.QueryRow(`SELECT id, uuid, username, password, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users WHERE uuid = ?`, uid)

	var u model.User
	if err := scanUserRow(row, &u); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	ids, _ := s.getNodeIDs(u.ID)
	u.NodeIDs = ids
	return &u, nil
}

// Create 创建用户
func (s *UserService) Create(req *CreateUserReq) (*model.User, error) {
	uid := uuid.New().String()
	now := time.Now()

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := parseTime(req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("过期时间格式错误: %w", err)
		}
		expiresAt = &t
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = "vless"
	}

	result, err := s.db.Exec(`INSERT INTO users (uuid, username, email, protocol,
		traffic_limit, speed_limit, reset_day, reset_cron, enable, expires_at,
		created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		uid, req.Username, req.Email, protocol,
		req.TrafficLimit, req.SpeedLimit, req.ResetDay, req.ResetCron,
		expiresAt, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}

	id, _ := result.LastInsertId()

	// 保存节点关联
	if len(req.NodeIDs) > 0 {
		if err := s.setNodeIDs(id, req.NodeIDs); err != nil {
			return nil, fmt.Errorf("保存节点关联失败: %w", err)
		}
	}

	// 为新用户注入 default token：token = uuid，ip_bind_enabled=0
	// 与启动时一次性迁移脚本等价，保证 /api/sub/:uuid 旧端点始终可用
	if _, err := s.db.Exec(
		`INSERT INTO subscription_tokens (user_id, name, token, enabled, ip_bind_enabled, created_at)
		 VALUES (?, 'default', ?, 1, 0, ?)`, id, uid, now); err != nil {
		return nil, fmt.Errorf("初始化订阅 token 失败: %w", err)
	}

	return s.GetByID(id)
}

// Update 更新用户（部分更新）
func (s *UserService) Update(id int64, req *UpdateUserReq) (*model.User, error) {
	// 先检查用户是否存在
	user, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}

	// 构建动态更新语句
	sets := []string{}
	args := []interface{}{}

	if req.Username != nil {
		sets = append(sets, "username = ?")
		args = append(args, *req.Username)
	}
	if req.Email != nil {
		sets = append(sets, "email = ?")
		args = append(args, *req.Email)
	}
	if req.Protocol != nil {
		sets = append(sets, "protocol = ?")
		args = append(args, *req.Protocol)
	}
	if req.TrafficLimit != nil {
		sets = append(sets, "traffic_limit = ?")
		args = append(args, *req.TrafficLimit)
	}
	if req.SpeedLimit != nil {
		sets = append(sets, "speed_limit = ?")
		args = append(args, *req.SpeedLimit)
	}
	if req.ResetDay != nil {
		sets = append(sets, "reset_day = ?")
		args = append(args, *req.ResetDay)
	}
	if req.ResetCron != nil {
		sets = append(sets, "reset_cron = ?")
		args = append(args, *req.ResetCron)
	}
	if req.Enable != nil {
		sets = append(sets, "enable = ?")
		args = append(args, *req.Enable)
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			sets = append(sets, "expires_at = NULL")
		} else {
			t, err := parseTime(*req.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("过期时间格式错误: %w", err)
			}
			sets = append(sets, "expires_at = ?")
			args = append(args, t)
		}
	}

	if len(sets) == 0 {
		return user, nil
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, id)

	query := "UPDATE users SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = ?"

	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("更新用户失败: %w", err)
	}

	// 更新节点关联
	if req.NodeIDs != nil {
		if err := s.setNodeIDs(id, *req.NodeIDs); err != nil {
			return nil, fmt.Errorf("更新节点关联失败: %w", err)
		}
	}

	return s.GetByID(id)
}

// Delete 删除用户
func (s *UserService) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// ResetTraffic 重置用户流量
func (s *UserService) ResetTraffic(id int64) error {
	result, err := s.db.Exec(`UPDATE users SET traffic_used = 0, traffic_up = 0,
		traffic_down = 0, warn_sent = 0, updated_at = ? WHERE id = ?`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("重置流量失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// ResetUUID 重新生成用户 UUID
func (s *UserService) ResetUUID(id int64) (string, error) {
	cur, err := s.GetByID(id)
	if err != nil {
		return "", err
	}
	if cur == nil {
		return "", fmt.Errorf("用户不存在")
	}
	newUUID := uuid.New().String()
	result, err := s.db.Exec("UPDATE users SET uuid = ?, updated_at = ? WHERE id = ?",
		newUUID, time.Now(), id)
	if err != nil {
		return "", fmt.Errorf("重置 UUID 失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return "", fmt.Errorf("用户不存在")
	}
	// 同步更新 default token（token = 旧 uuid），让旧订阅链接立即失效
	_, _ = s.db.Exec(`UPDATE subscription_tokens SET token = ? WHERE user_id = ? AND token = ?`,
		newUUID, id, cur.UUID)
	return newUUID, nil
}

// Count 统计用户数量
func (s *UserService) Count() (total int, enabled int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("统计用户总数失败: %w", err)
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE enable = 1").Scan(&enabled)
	if err != nil {
		return 0, 0, fmt.Errorf("统计启用用户数失败: %w", err)
	}
	return
}

// parseTime 解析时间字符串，支持两种格式
func parseTime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("不支持的时间格式: %s，请使用 2006-01-02 15:04:05 或 2006-01-02", s)
}

// scanner 接口用于统一 *sql.Row 和 *sql.Rows 的扫描
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanUserFromScanner(s scanner, u *model.User) error {
	return s.Scan(&u.ID, &u.UUID, &u.Username, &u.Password, &u.Email, &u.Protocol,
		&u.TrafficLimit, &u.TrafficUsed, &u.TrafficUp, &u.TrafficDown, &u.SpeedLimit,
		&u.ResetDay, &u.ResetCron, &u.Enable, &u.ExpiresAt, &u.WarnSent,
		&u.CreatedAt, &u.UpdatedAt)
}

func scanUser(rows *sql.Rows, u *model.User) error {
	return scanUserFromScanner(rows, u)
}

func scanUserRow(row *sql.Row, u *model.User) error {
	return scanUserFromScanner(row, u)
}

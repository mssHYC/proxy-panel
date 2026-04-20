package service

import (
	"fmt"
	"time"

	"proxy-panel/internal/database"
)

// AuditLog 审计日志条目
type AuditLog struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	IP         string    `json:"ip"`
	Detail     string    `json:"detail"`
}

// AuditFilter 查询过滤器
type AuditFilter struct {
	Actor  string
	Action string
	From   *time.Time
	To     *time.Time
	Offset int
	Limit  int
}

// AuditService 审计日志服务
type AuditService struct {
	db *database.DB
}

// NewAuditService 创建审计服务
func NewAuditService(db *database.DB) *AuditService {
	return &AuditService{db: db}
}

// Log 写入一条审计日志；失败仅告警，不阻断主流程
func (s *AuditService) Log(actor, action, targetType, targetID, ip, detail string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs (created_at, actor, action, target_type, target_id, ip, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now(), actor, action, targetType, targetID, ip, detail)
	if err != nil {
		return fmt.Errorf("写审计日志失败: %w", err)
	}
	return nil
}

// List 分页查询审计日志
func (s *AuditService) List(f AuditFilter) ([]AuditLog, int, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	if f.Actor != "" {
		where += " AND actor = ?"
		args = append(args, f.Actor)
	}
	if f.Action != "" {
		where += " AND action = ?"
		args = append(args, f.Action)
	}
	if f.From != nil {
		where += " AND created_at >= ?"
		args = append(args, *f.From)
	}
	if f.To != nil {
		where += " AND created_at < ?"
		args = append(args, *f.To)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计审计日志失败: %w", err)
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	qArgs := append(args, limit, offset)
	rows, err := s.db.Query(`SELECT id, created_at, actor, action, target_type, target_id, ip, detail
		FROM audit_logs `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, qArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询审计日志失败: %w", err)
	}
	defer rows.Close()

	out := []AuditLog{}
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.Actor, &a.Action, &a.TargetType, &a.TargetID, &a.IP, &a.Detail); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

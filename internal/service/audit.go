package service

import (
	"fmt"
	"strings"
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
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	From       *time.Time
	To         *time.Time
	Offset     int
	Limit      int
}

// AuditService 审计日志服务
type AuditService struct {
	db *database.DB
}

// NewAuditService 创建审计服务
func NewAuditService(db *database.DB) *AuditService {
	return &AuditService{db: db}
}

// timeColumnFormat 与 traffic.go 保持一致：modernc.org/sqlite 直接写入 time.Time
// 会带上 Go monotonic 尾巴（"... m=+..."），导致 SQLite 时间函数与字符串比较不可靠。
// 写入与查询统一格式化为标准 UTC 字符串。
const timeColumnFormat = "2006-01-02 15:04:05"

func formatTimeForDB(t time.Time) string {
	return t.UTC().Format(timeColumnFormat)
}

// Log 写入一条审计日志；失败仅告警，不阻断主流程
func (s *AuditService) Log(actor, action, targetType, targetID, ip, detail string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs (created_at, actor, action, target_type, target_id, ip, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		formatTimeForDB(time.Now()), actor, action, targetType, targetID, ip, detail)
	if err != nil {
		return fmt.Errorf("写审计日志失败: %w", err)
	}
	return nil
}

// buildWhere 根据过滤器构造 WHERE 子句与参数，由 List/Export 共用
func (f AuditFilter) buildWhere() (string, []interface{}) {
	var conds []string
	var args []interface{}
	if f.Actor != "" {
		conds = append(conds, "actor = ?")
		args = append(args, f.Actor)
	}
	if f.Action != "" {
		conds = append(conds, "action LIKE ?")
		args = append(args, "%"+f.Action+"%")
	}
	if f.TargetType != "" {
		conds = append(conds, "target_type = ?")
		args = append(args, f.TargetType)
	}
	if f.TargetID != "" {
		conds = append(conds, "target_id = ?")
		args = append(args, f.TargetID)
	}
	if f.From != nil {
		conds = append(conds, "created_at >= ?")
		args = append(args, formatTimeForDB(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "created_at < ?")
		args = append(args, formatTimeForDB(*f.To))
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// List 分页查询审计日志
func (s *AuditService) List(f AuditFilter) ([]AuditLog, int, error) {
	where, args := f.buildWhere()

	var total int
	countSQL := "SELECT COUNT(*) FROM audit_logs"
	if where != "" {
		countSQL += " " + where
	}
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计审计日志失败: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	qArgs := append(args, limit, offset)
	querySQL := "SELECT id, created_at, actor, action, target_type, target_id, ip, detail FROM audit_logs"
	if where != "" {
		querySQL += " " + where
	}
	querySQL += " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := s.db.Query(querySQL, qArgs...)
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

// ExportMaxRows 单次 CSV 导出最多返回的行数，防止过大查询拖垮 SQLite/连接
const ExportMaxRows = 50000

// Export 不分页查询审计日志（用于 CSV 导出），最多返回 ExportMaxRows 行；
// 返回的 truncated=true 表示命中上限，调用方应提示用户结果可能被截断
func (s *AuditService) Export(f AuditFilter) ([]AuditLog, bool, error) {
	where, args := f.buildWhere()
	args = append(args, ExportMaxRows)
	querySQL := "SELECT id, created_at, actor, action, target_type, target_id, ip, detail FROM audit_logs"
	if where != "" {
		querySQL += " " + where
	}
	querySQL += " ORDER BY id DESC LIMIT ?"
	rows, err := s.db.Query(querySQL, args...)
	if err != nil {
		return nil, false, fmt.Errorf("导出审计日志失败: %w", err)
	}
	defer rows.Close()
	out := []AuditLog{}
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.Actor, &a.Action, &a.TargetType, &a.TargetID, &a.IP, &a.Detail); err != nil {
			return nil, false, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, len(out) >= ExportMaxRows, nil
}

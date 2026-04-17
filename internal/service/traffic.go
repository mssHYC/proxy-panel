package service

import (
	"fmt"
	"log"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
)

// TrafficService 流量管理服务
type TrafficService struct {
	db  *database.DB
	mgr *kernel.Manager
}

// NewTrafficService 创建流量服务
func NewTrafficService(db *database.DB, mgr *kernel.Manager) *TrafficService {
	return &TrafficService{db: db, mgr: mgr}
}

// Collect 采集所有引擎的流量数据，更新用户流量并记录日志
func (s *TrafficService) Collect() error {
	stats, err := s.mgr.GetTrafficStats()
	if err != nil {
		return fmt.Errorf("获取流量统计失败: %w", err)
	}

	now := time.Now()
	for email, traffic := range stats {
		if traffic.Upload == 0 && traffic.Download == 0 {
			continue
		}

		// 内核配置里 client.email 使用 username，这里按 username 反查用户
		var userID int64
		err := s.db.QueryRow("SELECT id FROM users WHERE username = ?", email).Scan(&userID)
		if err != nil {
			log.Printf("查找用户失败 username=%s: %v", email, err)
			continue
		}

		// 更新用户流量
		_, err = s.db.Exec(`UPDATE users SET
			traffic_used = traffic_used + ?,
			traffic_up = traffic_up + ?,
			traffic_down = traffic_down + ?,
			updated_at = ?
			WHERE id = ?`,
			traffic.Upload+traffic.Download, traffic.Upload, traffic.Download, now, userID)
		if err != nil {
			log.Printf("更新用户流量失败 id=%d: %v", userID, err)
			continue
		}

		// 插入流量日志
		_, err = s.db.Exec(`INSERT INTO traffic_logs (user_id, node_id, upload, download, timestamp)
			VALUES (?, 0, ?, ?, ?)`, userID, traffic.Upload, traffic.Download, now)
		if err != nil {
			log.Printf("插入流量日志失败 id=%d: %v", userID, err)
		}

		// 更新服务器全局流量
		_, err = s.db.Exec(`UPDATE server_traffic SET
			total_up = total_up + ?,
			total_down = total_down + ?`,
			traffic.Upload, traffic.Download)
		if err != nil {
			log.Printf("更新服务器流量失败: %v", err)
		}
	}

	return nil
}

// CheckUserThresholds 检查用户流量阈值，返回告警用户和已耗尽用户
func (s *TrafficService) CheckUserThresholds(warnPercent int) (warns []model.User, exhausted []model.User, err error) {
	rows, err := s.db.Query(`SELECT id, uuid, username, password, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users WHERE traffic_limit > 0 AND enable = 1`)
	if err != nil {
		return nil, nil, fmt.Errorf("查询用户流量失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.UUID, &u.Username, &u.Password, &u.Email, &u.Protocol,
			&u.TrafficLimit, &u.TrafficUsed, &u.TrafficUp, &u.TrafficDown, &u.SpeedLimit,
			&u.ResetDay, &u.ResetCron, &u.Enable, &u.ExpiresAt, &u.WarnSent,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, nil, fmt.Errorf("扫描用户数据失败: %w", err)
		}

		percent := int(u.TrafficUsed * 100 / u.TrafficLimit)

		// 流量已耗尽，自动禁用
		if percent >= 100 {
			s.db.Exec("UPDATE users SET enable = 0, updated_at = ? WHERE id = ?", time.Now(), u.ID)
			exhausted = append(exhausted, u)
			continue
		}

		// 达到告警阈值且未发送过告警
		if percent >= warnPercent && !u.WarnSent {
			s.db.Exec("UPDATE users SET warn_sent = 1, updated_at = ? WHERE id = ?", time.Now(), u.ID)
			warns = append(warns, u)
		}
	}

	return warns, exhausted, rows.Err()
}

// CheckServerThreshold 检查服务器全局流量阈值
func (s *TrafficService) CheckServerThreshold(warnPercent int) (warnNeeded bool, limitReached bool, st *model.ServerTraffic, err error) {
	st, err = s.GetServerTraffic()
	if err != nil {
		return false, false, nil, err
	}
	if st == nil || st.LimitBytes <= 0 {
		return false, false, st, nil
	}

	totalUsed := st.TotalUp + st.TotalDown
	percent := int(totalUsed * 100 / st.LimitBytes)

	// 达到限制
	if percent >= 100 && !st.LimitSent {
		s.db.Exec("UPDATE server_traffic SET limit_sent = 1")
		limitReached = true
	}

	// 达到告警阈值
	if percent >= warnPercent && !st.WarnSent {
		s.db.Exec("UPDATE server_traffic SET warn_sent = 1")
		warnNeeded = true
	}

	return
}

// ResetByDay 重置指定日期的用户流量
func (s *TrafficService) ResetByDay(day int) (int64, error) {
	result, err := s.db.Exec(`UPDATE users SET
		traffic_used = 0, traffic_up = 0, traffic_down = 0,
		warn_sent = 0, updated_at = ?
		WHERE reset_day = ? AND reset_cron = ''`, time.Now(), day)
	if err != nil {
		return 0, fmt.Errorf("重置用户流量失败: %w", err)
	}
	return result.RowsAffected()
}

// ResetServerTraffic 重置服务器全局流量
func (s *TrafficService) ResetServerTraffic() error {
	_, err := s.db.Exec(`UPDATE server_traffic SET
		total_up = 0, total_down = 0,
		warn_sent = 0, limit_sent = 0,
		reset_at = ?`, time.Now())
	if err != nil {
		return fmt.Errorf("重置服务器流量失败: %w", err)
	}
	return nil
}

// GetServerTraffic 获取服务器全局流量记录
func (s *TrafficService) GetServerTraffic() (*model.ServerTraffic, error) {
	var st model.ServerTraffic
	err := s.db.QueryRow(`SELECT id, total_up, total_down, limit_bytes, warn_sent, limit_sent, reset_at
		FROM server_traffic LIMIT 1`).Scan(
		&st.ID, &st.TotalUp, &st.TotalDown, &st.LimitBytes,
		&st.WarnSent, &st.LimitSent, &st.ResetAt)
	if err != nil {
		return nil, fmt.Errorf("查询服务器流量失败: %w", err)
	}
	return &st, nil
}

// SetServerLimit 设置服务器流量限制（单位 GB）
func (s *TrafficService) SetServerLimit(limitGB int64) error {
	limitBytes := limitGB * 1024 * 1024 * 1024
	_, err := s.db.Exec("UPDATE server_traffic SET limit_bytes = ?", limitBytes)
	if err != nil {
		return fmt.Errorf("设置服务器流量限制失败: %w", err)
	}
	return nil
}

// GetHistory 获取最近 N 天的流量历史（按天聚合）
func (s *TrafficService) GetHistory(days int) ([]map[string]interface{}, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT
		DATE(timestamp) as date,
		SUM(upload) as upload,
		SUM(download) as download
		FROM traffic_logs
		WHERE DATE(timestamp) >= ?
		GROUP BY DATE(timestamp)
		ORDER BY date ASC`, since)
	if err != nil {
		return nil, fmt.Errorf("查询流量历史失败: %w", err)
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var date string
		var upload, download int64
		if err := rows.Scan(&date, &upload, &download); err != nil {
			return nil, fmt.Errorf("扫描流量历史失败: %w", err)
		}
		result = append(result, map[string]interface{}{
			"date":     date,
			"upload":   upload,
			"download": download,
			"total":    formatBytes(upload + download),
		})
	}
	return result, rows.Err()
}

// CleanupLogs 清理过期流量日志 (7天内保留原始, 7-90天按日聚合, 90天以上删除)
func (s *TrafficService) CleanupLogs() error {
	// 删除 90 天以上的日志
	_, err := s.db.Exec("DELETE FROM traffic_logs WHERE timestamp < datetime('now', '-90 days')")
	if err != nil {
		return fmt.Errorf("清理90天以上日志失败: %w", err)
	}
	// 删除 7-90 天内的非聚合日志（保留每天第一条作为聚合代表）
	_, err = s.db.Exec(`DELETE FROM traffic_logs WHERE timestamp < datetime('now', '-7 days')
		AND timestamp >= datetime('now', '-90 days')
		AND id NOT IN (
			SELECT MIN(id) FROM traffic_logs
			WHERE timestamp < datetime('now', '-7 days')
			AND timestamp >= datetime('now', '-90 days')
			GROUP BY user_id, DATE(timestamp)
		)`)
	if err != nil {
		return fmt.Errorf("聚合7-90天日志失败: %w", err)
	}
	return nil
}

// formatBytes 格式化字节数为可读字符串
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

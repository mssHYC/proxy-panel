package service

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
)

// parseNodeIDFromTag 从 "node-<id>" 形式的 inbound tag 中解析节点 ID。
// 不匹配（含空 tag）时返回 0，调用方按 node_id=0 记录，保持可观测但不参与节点维度统计。
func parseNodeIDFromTag(tag string) int64 {
	if !strings.HasPrefix(tag, "node-") {
		return 0
	}
	id, err := strconv.ParseInt(tag[len("node-"):], 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

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
	// modernc.org/sqlite 直接写入 time.Time 时会带上 Go 的 monotonic 尾巴
	// （形如 "2026-04-17 02:04:08 +0000 UTC m=+59.590863369"），SQLite 的
	// DATE()/strftime() 无法解析，趋势图会拿不到数据。统一格式化成标准字符串。
	tsStr := now.UTC().Format("2006-01-02 15:04:05")

	// 按 username 缓存 user_id，避免同一用户多节点重复查 users 表
	userIDCache := make(map[string]int64)
	for _, st := range stats {
		if st.Upload == 0 && st.Download == 0 {
			continue
		}

		userID, ok := userIDCache[st.Username]
		if !ok {
			err := s.db.QueryRow("SELECT id FROM users WHERE username = ?", st.Username).Scan(&userID)
			if err != nil {
				log.Printf("查找用户失败 username=%s: %v", st.Username, err)
				userIDCache[st.Username] = 0 // 负缓存，避免重复查询
				continue
			}
			userIDCache[st.Username] = userID
		}
		if userID == 0 {
			continue
		}

		nodeID := parseNodeIDFromTag(st.NodeTag)

		// 更新用户流量（聚合到用户维度，node 维度只进 traffic_logs）
		_, err = s.db.Exec(`UPDATE users SET
			traffic_used = traffic_used + ?,
			traffic_up = traffic_up + ?,
			traffic_down = traffic_down + ?,
			updated_at = ?
			WHERE id = ?`,
			st.Upload+st.Download, st.Upload, st.Download, tsStr, userID)
		if err != nil {
			log.Printf("更新用户流量失败 id=%d: %v", userID, err)
			continue
		}

		// 插入流量日志（带真实 node_id；解析失败时 node_id=0 仍可观测）
		_, err = s.db.Exec(`INSERT INTO traffic_logs (user_id, node_id, upload, download, timestamp)
			VALUES (?, ?, ?, ?, ?)`, userID, nodeID, st.Upload, st.Download, tsStr)
		if err != nil {
			log.Printf("插入流量日志失败 user_id=%d node_id=%d: %v", userID, nodeID, err)
		}

		// 更新服务器全局流量
		_, err = s.db.Exec(`UPDATE server_traffic SET
			total_up = total_up + ?,
			total_down = total_down + ?`,
			st.Upload, st.Download)
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

// GetNodeDistribution 返回最近 N 天按节点聚合的流量分布。
// 用于"按节点分布"卡片：node_id=0 的归到"未归属"分类，便于发现采集异常。
func (s *TrafficService) GetNodeDistribution(days int) ([]map[string]interface{}, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT
		t.node_id,
		COALESCE(n.name, '') AS node_name,
		SUM(t.upload) AS upload,
		SUM(t.download) AS download
		FROM traffic_logs t
		LEFT JOIN nodes n ON n.id = t.node_id
		WHERE DATE(t.timestamp) >= ?
		GROUP BY t.node_id
		ORDER BY (SUM(t.upload) + SUM(t.download)) DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("查询节点流量分布失败: %w", err)
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var nodeID int64
		var nodeName string
		var upload, download int64
		if err := rows.Scan(&nodeID, &nodeName, &upload, &download); err != nil {
			return nil, fmt.Errorf("扫描节点流量分布失败: %w", err)
		}
		label := nodeName
		if nodeID == 0 {
			label = "未归属"
		} else if label == "" {
			label = fmt.Sprintf("节点 #%d (已删除)", nodeID)
		}
		result = append(result, map[string]interface{}{
			"node_id":   nodeID,
			"node_name": label,
			"upload":    upload,
			"download":  download,
			"total":     formatBytes(upload + download),
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
			GROUP BY user_id, node_id, DATE(timestamp)
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

package service

import (
	"fmt"
	"log"
	"time"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	notify "proxy-panel/internal/service/notify"

	"github.com/robfig/cron/v3"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	cron       *cron.Cron
	cfg        *config.Config
	trafficSvc *TrafficService
	notifySvc  *notify.NotifyService
	db         *database.DB
}

// NewScheduler 创建调度器
func NewScheduler(cfg *config.Config, trafficSvc *TrafficService, notifySvc *notify.NotifyService, db *database.DB) *Scheduler {
	return &Scheduler{
		cron:       cron.New(),
		cfg:        cfg,
		trafficSvc: trafficSvc,
		notifySvc:  notifySvc,
		db:         db,
	}
}

// Start 启动所有定时任务
func (s *Scheduler) Start() {
	// 1. 流量采集（每 N 秒）
	interval := s.cfg.Traffic.CollectInterval
	if interval <= 0 {
		interval = 60
	}
	s.cron.AddFunc(fmt.Sprintf("@every %ds", interval), func() {
		if err := s.trafficSvc.Collect(); err != nil {
			log.Printf("[调度器] 流量采集失败: %v", err)
			return
		}

		// 检查用户流量阈值
		warnPercent := s.cfg.Traffic.WarnPercent
		if warnPercent <= 0 {
			warnPercent = 80
		}
		warns, exhausted, err := s.trafficSvc.CheckUserThresholds(warnPercent)
		if err != nil {
			log.Printf("[调度器] 检查用户流量阈值失败: %v", err)
		}
		for _, u := range warns {
			msg := fmt.Sprintf("⚠️ 用户 %s 流量已达 %d%% 阈值，已使用 %s / %s",
				u.Username, warnPercent,
				formatBytes(u.TrafficUsed), formatBytes(u.TrafficLimit))
			s.notifySvc.SendAll(msg)
		}
		for _, u := range exhausted {
			msg := fmt.Sprintf("🚫 用户 %s 流量已耗尽，已自动禁用。已使用 %s / %s",
				u.Username,
				formatBytes(u.TrafficUsed), formatBytes(u.TrafficLimit))
			s.notifySvc.SendAll(msg)
		}

		// 检查服务器流量阈值
		warnNeeded, limitReached, st, err := s.trafficSvc.CheckServerThreshold(warnPercent)
		if err != nil {
			log.Printf("[调度器] 检查服务器流量阈值失败: %v", err)
		}
		if warnNeeded && st != nil {
			msg := fmt.Sprintf("⚠️ 服务器流量已达 %d%% 阈值，已使用 %s / %s",
				warnPercent,
				formatBytes(st.TotalUp+st.TotalDown), formatBytes(st.LimitBytes))
			s.notifySvc.SendAll(msg)
		}
		if limitReached && st != nil {
			msg := fmt.Sprintf("🚫 服务器流量已达上限！已使用 %s / %s",
				formatBytes(st.TotalUp+st.TotalDown), formatBytes(st.LimitBytes))
			s.notifySvc.SendAll(msg)
		}
	})

	// 2. 用户流量按天重置（每天 00:00）
	s.cron.AddFunc("0 0 * * *", func() {
		day := time.Now().Day()
		count, err := s.trafficSvc.ResetByDay(day)
		if err != nil {
			log.Printf("[调度器] 重置用户流量失败: %v", err)
			return
		}
		if count > 0 {
			log.Printf("[调度器] 已重置 %d 个用户的流量（重置日=%d）", count, day)
		}
	})

	// 3. 服务器流量重置（自定义 cron 表达式）
	if s.cfg.Traffic.ResetCron != "" {
		s.cron.AddFunc(s.cfg.Traffic.ResetCron, func() {
			if err := s.trafficSvc.ResetServerTraffic(); err != nil {
				log.Printf("[调度器] 重置服务器流量失败: %v", err)
				return
			}
			log.Printf("[调度器] 服务器流量已重置")
			s.notifySvc.SendAll("🔄 服务器流量已重置")
		})
	}

	// 4. 流量日志清理（每天 03:00）
	s.cron.AddFunc("0 3 * * *", func() {
		if err := s.trafficSvc.CleanupLogs(); err != nil {
			log.Printf("[调度器] 流量日志清理失败: %v", err)
		} else {
			log.Println("[调度器] 流量日志清理完成")
		}
	})

	// 5. 用户过期检查（每天 08:00）
	s.cron.AddFunc("0 8 * * *", func() {
		now := time.Now()

		// 禁用已过期用户
		result, err := s.db.Exec(`UPDATE users SET enable = 0, updated_at = ?
			WHERE enable = 1 AND expires_at IS NOT NULL AND expires_at <= ?`, now, now)
		if err != nil {
			log.Printf("[调度器] 禁用过期用户失败: %v", err)
		} else {
			count, _ := result.RowsAffected()
			if count > 0 {
				msg := fmt.Sprintf("🚫 已自动禁用 %d 个过期用户", count)
				log.Printf("[调度器] %s", msg)
				s.notifySvc.SendAll(msg)
			}
		}

		// 通知即将过期（3天内）的用户
		threeDaysLater := now.AddDate(0, 0, 3)
		rows, err := s.db.Query(`SELECT username, expires_at FROM users
			WHERE enable = 1 AND expires_at IS NOT NULL
			AND expires_at > ? AND expires_at <= ?`, now, threeDaysLater)
		if err != nil {
			log.Printf("[调度器] 查询即将过期用户失败: %v", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var username string
			var expiresAt time.Time
			if err := rows.Scan(&username, &expiresAt); err != nil {
				continue
			}
			msg := fmt.Sprintf("⏰ 用户 %s 将于 %s 过期，请及时续期",
				username, expiresAt.Format("2006-01-02"))
			s.notifySvc.SendAll(msg)
		}
	})

	s.cron.Start()
	log.Printf("[调度器] 已启动，流量采集间隔 %d 秒", interval)
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Printf("[调度器] 已停止")
}

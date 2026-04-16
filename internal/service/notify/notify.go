package notify

import (
	"fmt"
	"log"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
)

// Channel 定义通知渠道接口
type Channel interface {
	Name() string
	Send(message string) error
}

// NotifyService 通知服务，管理多个通知渠道
type NotifyService struct {
	channels []Channel
	db       *database.DB
}

// NewNotifyService 根据配置创建通知服务实例
func NewNotifyService(cfg *config.Config, db *database.DB) *NotifyService {
	var channels []Channel
	if cfg.Notify.Telegram.Enable {
		channels = append(channels, NewTelegram(cfg.Notify.Telegram.BotToken, cfg.Notify.Telegram.ChatID))
	}
	if cfg.Notify.Wechat.Enable {
		channels = append(channels, NewWechat(cfg.Notify.Wechat.WebhookURL))
	}
	return &NotifyService{channels: channels, db: db}
}

// SendAll 向所有已启用的通知渠道发送消息
func (s *NotifyService) SendAll(message string) {
	for _, ch := range s.channels {
		if err := ch.Send(message); err != nil {
			log.Printf("通知发送失败 [%s]: %v", ch.Name(), err)
			s.recordAlert(ch.Name(), message, "failed")
		} else {
			s.recordAlert(ch.Name(), message, "sent")
		}
	}
}

// Test 发送测试消息到指定渠道，channel 为空时发送到第一个可用渠道
func (s *NotifyService) Test(channel string) error {
	for _, ch := range s.channels {
		if ch.Name() == channel || channel == "" {
			return ch.Send("🔔 ProxyPanel 测试消息 - 通知通道正常")
		}
	}
	return fmt.Errorf("通道 %s 未配置或未启用", channel)
}

// recordAlert 记录通知发送结果到数据库
func (s *NotifyService) recordAlert(channel, message, status string) {
	s.db.Exec(`INSERT INTO alert_records (type, message, channel, status) VALUES ('notify', ?, ?, ?)`,
		message, channel, status)
}

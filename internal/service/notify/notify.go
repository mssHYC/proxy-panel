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

// NotifyService 通知服务，按调用动态从 settings 表解析配置，
// 这样后台 UI 修改 tg_bot_token / tg_chat_id / wechat_webhook 后立即生效，
// 无需重启或回写 config.yaml。
type NotifyService struct {
	cfg *config.Config
	db  *database.DB
}

// NewNotifyService 创建通知服务实例
func NewNotifyService(cfg *config.Config, db *database.DB) *NotifyService {
	return &NotifyService{cfg: cfg, db: db}
}

// resolveChannels 读取 DB settings（fallback 到 config.yaml）构建当前可用渠道
func (s *NotifyService) resolveChannels() []Channel {
	settings := s.loadSettings()

	tgToken := pick(settings["tg_bot_token"], s.cfg.Notify.Telegram.BotToken)
	tgChat := pick(settings["tg_chat_id"], s.cfg.Notify.Telegram.ChatID)
	wechatHook := pick(settings["wechat_webhook"], s.cfg.Notify.Wechat.WebhookURL)

	var channels []Channel
	if tgToken != "" && tgChat != "" {
		channels = append(channels, NewTelegram(tgToken, tgChat))
	}
	if wechatHook != "" {
		channels = append(channels, NewWechat(wechatHook))
	}
	return channels
}

func (s *NotifyService) loadSettings() map[string]string {
	out := map[string]string{}
	if s.db == nil {
		return out
	}
	rows, err := s.db.Query(`SELECT key, value FROM settings WHERE key IN ('tg_bot_token','tg_chat_id','wechat_webhook')`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = v
		}
	}
	return out
}

func pick(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

// SendAll 向所有已启用的通知渠道发送消息
func (s *NotifyService) SendAll(message string) {
	for _, ch := range s.resolveChannels() {
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
	channels := s.resolveChannels()
	for _, ch := range channels {
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

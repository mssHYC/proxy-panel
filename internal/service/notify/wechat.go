package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Wechat 实现企业微信群机器人 Webhook 通知渠道
type Wechat struct {
	webhookURL string
	client     *http.Client
}

// NewWechat 创建企业微信通知渠道实例
func NewWechat(webhookURL string) *Wechat {
	return &Wechat{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *Wechat) Name() string {
	return "wechat"
}

// Send 通过企业微信 Webhook 发送文本消息
func (w *Wechat) Send(message string) error {
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求体失败: %w", err)
	}

	resp, err := w.client.Post(w.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("企业微信请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信返回异常状态码: %d", resp.StatusCode)
	}

	return nil
}

package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// 企业微信 Webhook 官方域名白名单。
// 仅允许向该域名发请求，防御 post-auth SSRF（例如攻击者填入 169.254.169.254 云元数据接口）。
var wechatAllowedHosts = map[string]struct{}{
	"qyapi.weixin.qq.com": {},
}

// Wechat 实现企业微信群机器人 Webhook 通知渠道
type Wechat struct {
	webhookURL string
	client     *http.Client
}

// NewWechat 创建企业微信通知渠道实例
func NewWechat(webhookURL string) *Wechat {
	// 自定义 Transport：
	// 1. DialContext 在建立连接时再做一次私网 IP 过滤，
	//    防御 DNS rebinding（白名单域名被解析到内网 IP）。
	// 2. 禁用重定向跟随，避免被 301 到任意域名。
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isPrivateOrLoopback(ip) {
					return nil, fmt.Errorf("拒绝连接到私网地址: %s", ip)
				}
			}
			// 直接用第一个公网 IP 建立连接，避免 Dial 时再次解析被 rebinding
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &Wechat{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (w *Wechat) Name() string {
	return "wechat"
}

// Send 通过企业微信 Webhook 发送文本消息
func (w *Wechat) Send(message string) error {
	if err := validateWechatURL(w.webhookURL); err != nil {
		return err
	}

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

// validateWechatURL 校验 webhook URL：scheme 必须是 https，host 必须在白名单内
func validateWechatURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook URL 解析失败: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook URL 必须使用 https，当前: %s", u.Scheme)
	}
	host := u.Hostname()
	if _, ok := wechatAllowedHosts[host]; !ok {
		return fmt.Errorf("webhook URL 域名不在白名单内: %s", host)
	}
	return nil
}

// isPrivateOrLoopback 判断 IP 是否属于需要拦截的非公网范围
// 覆盖：loopback / link-local / multicast / unspecified / 私网 / 保留地址
func isPrivateOrLoopback(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	// 显式拦截云元数据常见 IP（169.254.169.254 已被 LinkLocalUnicast 覆盖，这里保留兜底）
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
	}
	return false
}


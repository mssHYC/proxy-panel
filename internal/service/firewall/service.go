package firewall

import (
	"context"
	"fmt"
	"log"
	"time"

	"proxy-panel/internal/config"
)

var backendTimeout = 5 * time.Second

// Notifier 防火墙失败告警回调，由 internal/service/notify.NotifyService 适配
type Notifier interface {
	SendAll(message string)
}

// Service 节点侧调用入口；enabled=false 时所有方法 no-op
type Service struct {
	backend Backend
	enabled bool
	notify  Notifier
}

// NewService 根据配置构造 Service；配置禁用或 backend 无效时返回 enabled=false 实例
func NewService(cfg config.FirewallConfig, n Notifier) (*Service, error) {
	if !cfg.Enable {
		return &Service{enabled: false, notify: n}, nil
	}
	b, err := selectBackend(cfg.Backend)
	if err != nil {
		return &Service{enabled: false, notify: n}, err
	}
	return &Service{backend: b, enabled: true, notify: n}, nil
}

// Enabled 暴露给调用方判断是否需要提示 firewall_warning
func (s *Service) Enabled() bool { return s != nil && s.enabled }

// Allow 放行端口（tcp+udp）；禁用时 no-op
func (s *Service) Allow(port int) error {
	if !s.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	if err := s.backend.Allow(ctx, port); err != nil {
		log.Printf("[防火墙] 放行端口 %d 失败: %v", port, err)
		s.notify.SendAll(firewallFailMsg("放行", port, err))
		return err
	}
	return nil
}

// Revoke 关闭端口（tcp+udp）；禁用时 no-op
func (s *Service) Revoke(port int) error {
	if !s.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	if err := s.backend.Revoke(ctx, port); err != nil {
		log.Printf("[防火墙] 关闭端口 %d 失败: %v", port, err)
		s.notify.SendAll(firewallFailMsg("关闭", port, err))
		return err
	}
	return nil
}

// EnsureAll 启动时对存量端口做单向 ensure，不清理其它规则
func (s *Service) EnsureAll(ctx context.Context, ports []int) {
	if !s.Enabled() {
		return
	}
	for _, p := range ports {
		c, cancel := context.WithTimeout(ctx, backendTimeout)
		if err := s.backend.Allow(c, p); err != nil {
			log.Printf("[防火墙] 启动对齐端口 %d 失败: %v", p, err)
		}
		cancel()
	}
}

func firewallFailMsg(action string, port int, err error) string {
	return fmt.Sprintf("[ProxyPanel] 防火墙%s端口失败: port=%d, err=%v", action, port, err)
}

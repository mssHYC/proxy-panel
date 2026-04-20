package firewall

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"proxy-panel/internal/config"
)

var backendTimeout = 5 * time.Second

// Notifier receives firewall failure messages. Implementations must be safe for
// concurrent calls — Service invokes SendAll from any goroutine (including the
// goroutines spawned by NodeService for async port sync).
type Notifier interface {
	SendAll(message string)
}

// Service 节点侧调用入口；enabled=false 时所有方法 no-op
// 支持运行时通过 Swap 热替换 backend/enabled，读写受 mu 保护
type Service struct {
	mu      sync.RWMutex
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
func (s *Service) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// CurrentBackend 返回当前生效的 backend 名；禁用或未配置时返回空串
func (s *Service) CurrentBackend() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.enabled || s.backend == nil {
		return ""
	}
	return s.backend.Name()
}

// Allow 放行端口（tcp+udp）；禁用时 no-op
func (s *Service) Allow(port int) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	enabled := s.enabled
	backend := s.backend
	notifier := s.notify
	s.mu.RUnlock()
	if !enabled || backend == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	if err := backend.Allow(ctx, port); err != nil {
		log.Printf("[防火墙] 放行端口 %d 失败: %v", port, err)
		if notifier != nil {
			notifier.SendAll(firewallFailMsg("放行", port, err))
		}
		return err
	}
	return nil
}

// Revoke 关闭端口（tcp+udp）；禁用时 no-op
func (s *Service) Revoke(port int) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	enabled := s.enabled
	backend := s.backend
	notifier := s.notify
	s.mu.RUnlock()
	if !enabled || backend == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), backendTimeout)
	defer cancel()
	if err := backend.Revoke(ctx, port); err != nil {
		log.Printf("[防火墙] 关闭端口 %d 失败: %v", port, err)
		if notifier != nil {
			notifier.SendAll(firewallFailMsg("关闭", port, err))
		}
		return err
	}
	return nil
}

// EnsureAll 启动时对存量端口做单向 ensure，不清理其它规则
func (s *Service) EnsureAll(ctx context.Context, ports []int) {
	if s == nil {
		return
	}
	s.mu.RLock()
	enabled := s.enabled
	backend := s.backend
	s.mu.RUnlock()
	if !enabled || backend == nil {
		return
	}
	for _, p := range ports {
		c, cancel := context.WithTimeout(ctx, backendTimeout)
		if err := backend.Allow(c, p); err != nil {
			log.Printf("[防火墙] 启动对齐端口 %d 失败: %v", p, err)
		}
		cancel()
	}
}

// Swap 运行时热替换 backend/enabled；失败时回退为 enabled=false 并返回 error
// 调用方（firewall HTTP handler）负责处理返回错误并回显给用户
func (s *Service) Swap(cfg config.FirewallConfig) error {
	if s == nil {
		return fmt.Errorf("firewall service 未初始化")
	}
	if !cfg.Enable {
		s.mu.Lock()
		s.enabled = false
		s.backend = nil
		s.mu.Unlock()
		return nil
	}
	b, err := selectBackend(cfg.Backend)
	if err != nil {
		s.mu.Lock()
		s.enabled = false
		s.backend = nil
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.enabled = true
	s.backend = b
	s.mu.Unlock()
	return nil
}

// Probe 只检测给定 backend 是否可用，不改变 Service 状态
func (s *Service) Probe(ctx context.Context, backendName string) error {
	b, err := selectBackend(backendName)
	if err != nil {
		return err
	}
	return b.Available(ctx)
}

func firewallFailMsg(action string, port int, err error) string {
	return fmt.Sprintf("[ProxyPanel] 防火墙%s端口失败: port=%d, err=%v", action, port, err)
}

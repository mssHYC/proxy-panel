package firewall

import (
	"context"
	"fmt"
)

// Backend 抽象具体防火墙工具（ufw / firewalld 等）
type Backend interface {
	Name() string
	Available(ctx context.Context) error
	Allow(ctx context.Context, port int) error  // 同时放行 tcp+udp，必须幂等
	Revoke(ctx context.Context, port int) error // 同时关闭 tcp+udp，必须幂等
}

// selectBackend 根据配置 backend 字段返回对应实现
// Task 2: 临时 stub，Task 3 会替换为真实 switch
func selectBackend(name string) (Backend, error) {
	return nil, fmt.Errorf("backend %q 暂未实现", name)
}

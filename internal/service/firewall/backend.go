// Package firewall provides an abstraction for firewall backends (ufw / firewalld)
// and a Service that orchestrates port allow/revoke operations with graceful
// degradation when the backend command fails.
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

// selectBackend returns the Backend implementation for the given name.
// This is currently a stub that returns an error for every name; real ufw /
// firewalld cases are wired in by follow-up changes.
func selectBackend(name string) (Backend, error) {
	return nil, fmt.Errorf("backend %q 暂未实现", name)
}

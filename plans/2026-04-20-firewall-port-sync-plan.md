# 防火墙端口自动同步 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 install.sh 首次安装时交互开启"节点端口自动同步"；面板运行时，节点 CRUD（含改端口、enable 切换）自动放行/关闭 tcp+udp 端口；服务启动时对存量 enable 节点做一次幂等 ensure。防火墙失败降级为告警，不阻塞业务。

**Architecture:** 新增 `internal/service/firewall` 包，封装 Backend 接口（ufw / firewalld 两实现，通过 exec.CommandContext + runner 注入实现可测），Service 聚合 backend + Notifier 做失败告警。`NodeService` 注入 fw，Create/Update/Delete 在 DB 写入完成后异步调用 Allow/Revoke。install.sh 扩展 `setup_firewall()` 加交互询问，把选择写入 `config.yaml` 的 `firewall:` 段。

**Tech Stack:** Go 1.22、ufw / firewalld CLI、bash、gopkg.in/yaml.v3

**Spec:** [specs/2026-04-20-firewall-port-sync-design.md](../specs/2026-04-20-firewall-port-sync-design.md)

---

## 文件结构

创建：
- `internal/service/firewall/service.go` — `Service` / `Notifier` / 构造器 / Allow / Revoke / EnsureAll / Enabled
- `internal/service/firewall/backend.go` — `Backend` 接口 + `selectBackend(name)` 工厂
- `internal/service/firewall/backend_ufw.go` — ufw 实现（注入 runner）
- `internal/service/firewall/backend_firewalld.go` — firewalld 实现（注入 runner）
- `internal/service/firewall/service_test.go` — Service 行为测试（fake backend + fake notifier）
- `internal/service/firewall/backend_ufw_test.go` — ufw 命令生成 & 幂等处理测试
- `internal/service/firewall/backend_firewalld_test.go` — firewalld 命令生成 & 幂等处理测试

修改：
- `internal/config/config.go` — 新增 `FirewallConfig` 字段
- `internal/service/node.go` — 注入 `*firewall.Service`，Create/Update/Delete 增加生命周期钩子
- `internal/handler/node.go` — 响应体增加 `firewall_warning` 字段
- `cmd/server/main.go` — 实例化 firewall.Service 并注入，启动时异步 EnsureAll
- `config.example.yaml` — 新增 `firewall:` 段（默认关闭）
- `scripts/install.sh` — 新增 `confirm_default_yes()`，扩展 `setup_firewall()` 与 `generate_config()` 模板

---

### Task 1: 扩展 Config 结构加载 `firewall:` 段

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: 新增 FirewallConfig 类型并挂到 Config**

在 `internal/config/config.go` 的 `Config` 结构体末尾添加字段，并在文件底部（`KernelConfig` 之后）加类型定义：

```go
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Traffic  TrafficConfig  `yaml:"traffic"`
	Notify   NotifyConfig   `yaml:"notify"`
	Kernel   KernelConfig   `yaml:"kernel"`
	Firewall FirewallConfig `yaml:"firewall"`
}
```

```go
type FirewallConfig struct {
	Enable  bool   `yaml:"enable"`
	Backend string `yaml:"backend"`
}
```

- [ ] **Step 2: 编译并验证**

Run: `go build ./...`
Expected: 无错误输出

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): 新增 firewall 段用于节点端口自动同步"
```

---

### Task 2: firewall 包 — Backend 接口 + Service 骨架 + 禁用态 no-op 测试

**Files:**
- Create: `internal/service/firewall/backend.go`
- Create: `internal/service/firewall/service.go`
- Create: `internal/service/firewall/service_test.go`

- [ ] **Step 1: 写失败测试 —— 禁用时方法全部 no-op**

Create `internal/service/firewall/service_test.go`:

```go
package firewall

import (
	"context"
	"testing"
)

// fakeBackend 记录调用轨迹，便于 Service 行为断言
type fakeBackend struct {
	name      string
	allows    []int
	revokes   []int
	allowErr  error
	revokeErr error
}

func (f *fakeBackend) Name() string                              { return f.name }
func (f *fakeBackend) Available(ctx context.Context) error       { return nil }
func (f *fakeBackend) Allow(ctx context.Context, port int) error {
	f.allows = append(f.allows, port)
	return f.allowErr
}
func (f *fakeBackend) Revoke(ctx context.Context, port int) error {
	f.revokes = append(f.revokes, port)
	return f.revokeErr
}

// fakeNotifier 记录 SendAll 调用
type fakeNotifier struct {
	messages []string
}

func (n *fakeNotifier) SendAll(msg string) { n.messages = append(n.messages, msg) }

func TestService_Disabled_AllMethodsNoop(t *testing.T) {
	b := &fakeBackend{name: "fake"}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: false, notify: n}

	if s.Enabled() {
		t.Fatalf("expected Enabled=false")
	}
	if err := s.Allow(1234); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if err := s.Revoke(1234); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	s.EnsureAll(context.Background(), []int{1, 2, 3})

	if len(b.allows)+len(b.revokes) > 0 {
		t.Fatalf("backend called while disabled: allows=%v revokes=%v", b.allows, b.revokes)
	}
	if len(n.messages) > 0 {
		t.Fatalf("notifier called while disabled: %v", n.messages)
	}
}
```

- [ ] **Step 2: 写 Backend 接口**

Create `internal/service/firewall/backend.go`:

```go
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
func selectBackend(name string) (Backend, error) {
	switch name {
	case "ufw":
		return newUFWBackend(defaultRunner), nil
	case "firewalld":
		return newFirewalldBackend(defaultRunner), nil
	default:
		return nil, fmt.Errorf("不支持的 firewall backend: %q", name)
	}
}
```

- [ ] **Step 3: 写 Service 骨架 + Notifier 接口**

Create `internal/service/firewall/service.go`:

```go
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
```

`defaultRunner` 会在 Task 3 `backend_runner.go` 中定义；本 Task 的 `backend.go` 不能引用它，因此 `selectBackend` 暂时写成占位桩（Task 3 再改回真实实现）：

Modify `internal/service/firewall/backend.go` 里 `selectBackend` 为：

```go
func selectBackend(name string) (Backend, error) {
	return nil, fmt.Errorf("backend %q 暂未实现", name)
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/service/firewall/ -run TestService_Disabled -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/firewall/
git commit -m "feat(firewall): 包骨架 + Service 禁用态 no-op 行为"
```

---

### Task 3: ufw backend 实现 + 测试（runner 注入）

**Files:**
- Create: `internal/service/firewall/backend_runner.go`
- Create: `internal/service/firewall/backend_ufw.go`
- Create: `internal/service/firewall/backend_ufw_test.go`
- Modify: `internal/service/firewall/backend.go`（替换 selectBackend 临时桩）

- [ ] **Step 1: 写 runner 抽象**

Create `internal/service/firewall/backend_runner.go`:

```go
package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// runner 抽象 exec 调用，便于测试注入
type runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// defaultRunner 使用 exec.CommandContext，stdout 和 stderr 分别收集
var defaultRunner runner = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return out.Bytes(), errBuf.Bytes(), fmt.Errorf("%s %v: %w (stderr=%s)",
			name, args, err, errBuf.String())
	}
	return out.Bytes(), errBuf.Bytes(), nil
}
```

- [ ] **Step 2: 写 ufw backend 失败测试**

Create `internal/service/firewall/backend_ufw_test.go`:

```go
package firewall

import (
	"context"
	"strings"
	"testing"
)

// fakeRun 记录每次调用的完整命令，并按预设返回结果
type fakeRun struct {
	calls    [][]string
	stdouts  [][]byte
	stderrs  [][]byte
	errs     []error
	callIdx  int
}

func (f *fakeRun) run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	i := f.callIdx
	f.callIdx++
	var out, errb []byte
	var err error
	if i < len(f.stdouts) {
		out = f.stdouts[i]
	}
	if i < len(f.stderrs) {
		errb = f.stderrs[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return out, errb, err
}

func TestUFWAllow_EmitsTCPAndUDP(t *testing.T) {
	fr := &fakeRun{}
	b := newUFWBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(fr.calls), fr.calls)
	}
	if got := strings.Join(fr.calls[0], " "); got != "ufw allow 4443/tcp" {
		t.Errorf("call 0: want 'ufw allow 4443/tcp', got %q", got)
	}
	if got := strings.Join(fr.calls[1], " "); got != "ufw allow 4443/udp" {
		t.Errorf("call 1: want 'ufw allow 4443/udp', got %q", got)
	}
}

func TestUFWRevoke_EmitsTCPAndUDP(t *testing.T) {
	fr := &fakeRun{}
	b := newUFWBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fr.calls))
	}
	if got := strings.Join(fr.calls[0], " "); got != "ufw delete allow 4443/tcp" {
		t.Errorf("call 0: want 'ufw delete allow 4443/tcp', got %q", got)
	}
	if got := strings.Join(fr.calls[1], " "); got != "ufw delete allow 4443/udp" {
		t.Errorf("call 1: want 'ufw delete allow 4443/udp', got %q", got)
	}
}

// ufw 对 delete 不存在的规则返回非零 + stderr "Could not delete non-existent rule"
// 该场景视为成功
func TestUFWRevoke_IgnoresNonExistentRule(t *testing.T) {
	fr := &fakeRun{
		stderrs: [][]byte{
			[]byte("Could not delete non-existent rule\n"),
			[]byte("Could not delete non-existent rule (v6)\n"),
		},
		errs: []error{errFake("exit status 1"), errFake("exit status 1")},
	}
	b := newUFWBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke should tolerate non-existent rule, got: %v", err)
	}
}

// 其他错误必须原样透出
func TestUFWAllow_PropagatesRealError(t *testing.T) {
	fr := &fakeRun{
		stderrs: [][]byte{[]byte("ERROR: cannot bind to management socket\n")},
		errs:    []error{errFake("exit status 2")},
	}
	b := newUFWBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// errFake 避免引 errors 包只为一次 New
type errFake string

func (e errFake) Error() string { return string(e) }
```

- [ ] **Step 3: 运行测试验证失败**

Run: `go test ./internal/service/firewall/ -run TestUFW -v`
Expected: FAIL with `newUFWBackend undefined`

- [ ] **Step 4: 写 ufw 实现**

Create `internal/service/firewall/backend_ufw.go`:

```go
package firewall

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

type ufwBackend struct {
	run runner
}

func newUFWBackend(r runner) Backend { return &ufwBackend{run: r} }

func (u *ufwBackend) Name() string { return "ufw" }

func (u *ufwBackend) Available(ctx context.Context) error {
	stdout, _, err := u.run(ctx, "ufw", "status")
	if err != nil {
		return err
	}
	if !bytes.Contains(stdout, []byte("Status: active")) {
		return &backendUnavailable{backend: "ufw", reason: "status 不是 active"}
	}
	return nil
}

func (u *ufwBackend) Allow(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		_, _, err := u.run(ctx, "ufw", "allow", strconv.Itoa(port)+"/"+proto)
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *ufwBackend) Revoke(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		_, stderr, err := u.run(ctx, "ufw", "delete", "allow", strconv.Itoa(port)+"/"+proto)
		if err != nil {
			if isUFWNonExistent(stderr) {
				continue
			}
			return err
		}
	}
	return nil
}

func isUFWNonExistent(stderr []byte) bool {
	return strings.Contains(string(stderr), "Could not delete non-existent rule")
}

type backendUnavailable struct {
	backend string
	reason  string
}

func (e *backendUnavailable) Error() string { return e.backend + " 不可用: " + e.reason }
```

- [ ] **Step 5: 把 selectBackend 的占位替换为真实分支**

Modify `internal/service/firewall/backend.go`：

```go
func selectBackend(name string) (Backend, error) {
	switch name {
	case "ufw":
		return newUFWBackend(defaultRunner), nil
	default:
		return nil, fmt.Errorf("不支持的 firewall backend: %q", name)
	}
}
```

（firewalld 下个 Task 再加）

- [ ] **Step 6: 运行测试验证通过**

Run: `go test ./internal/service/firewall/ -v`
Expected: 全部 PASS

- [ ] **Step 7: Commit**

```bash
git add internal/service/firewall/
git commit -m "feat(firewall): ufw backend 实现 + runner 注入测试"
```

---

### Task 4: firewalld backend 实现 + 测试

**Files:**
- Create: `internal/service/firewall/backend_firewalld.go`
- Create: `internal/service/firewall/backend_firewalld_test.go`
- Modify: `internal/service/firewall/backend.go`（加 firewalld 分支）

- [ ] **Step 1: 写失败测试**

Create `internal/service/firewall/backend_firewalld_test.go`:

```go
package firewall

import (
	"context"
	"strings"
	"testing"
)

func TestFirewalldAllow_AddThenReload(t *testing.T) {
	fr := &fakeRun{}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected 3 calls (tcp add, udp add, reload), got %d: %v",
			len(fr.calls), fr.calls)
	}
	expect := []string{
		"firewall-cmd --permanent --add-port=4443/tcp",
		"firewall-cmd --permanent --add-port=4443/udp",
		"firewall-cmd --reload",
	}
	for i, want := range expect {
		if got := strings.Join(fr.calls[i], " "); got != want {
			t.Errorf("call %d: want %q got %q", i, want, got)
		}
	}
}

func TestFirewalldRevoke_RemoveThenReload(t *testing.T) {
	fr := &fakeRun{}
	b := newFirewalldBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fr.calls))
	}
	expect := []string{
		"firewall-cmd --permanent --remove-port=4443/tcp",
		"firewall-cmd --permanent --remove-port=4443/udp",
		"firewall-cmd --reload",
	}
	for i, want := range expect {
		if got := strings.Join(fr.calls[i], " "); got != want {
			t.Errorf("call %d: want %q got %q", i, want, got)
		}
	}
}

// firewalld 对已存在规则返回 stdout "ALREADY_ENABLED" + 非零退出；视为成功
func TestFirewalldAllow_IgnoresAlreadyEnabled(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{
			[]byte("Warning: ALREADY_ENABLED: 4443:tcp\nsuccess\n"),
			[]byte("Warning: ALREADY_ENABLED: 4443:udp\nsuccess\n"),
			[]byte("success\n"),
		},
		errs: []error{errFake("exit 12"), errFake("exit 12"), nil},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err != nil {
		t.Fatalf("should tolerate ALREADY_ENABLED, got: %v", err)
	}
}

// 不存在规则的 remove 返回 NOT_ENABLED + 非零；视为成功
func TestFirewalldRevoke_IgnoresNotEnabled(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{
			[]byte("Warning: NOT_ENABLED: 4443:tcp\n"),
			[]byte("Warning: NOT_ENABLED: 4443:udp\n"),
			[]byte("success\n"),
		},
		errs: []error{errFake("exit 12"), errFake("exit 12"), nil},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Revoke(context.Background(), 4443); err != nil {
		t.Fatalf("should tolerate NOT_ENABLED, got: %v", err)
	}
}

// 真实错误（如未启动）必须返回
func TestFirewalldAllow_PropagatesRealError(t *testing.T) {
	fr := &fakeRun{
		stdouts: [][]byte{[]byte("FirewallD is not running\n")},
		errs:    []error{errFake("exit status 252")},
	}
	b := newFirewalldBackend(fr.run)
	if err := b.Allow(context.Background(), 4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/service/firewall/ -run TestFirewalld -v`
Expected: FAIL with `newFirewalldBackend undefined`

- [ ] **Step 3: 写 firewalld 实现**

Create `internal/service/firewall/backend_firewalld.go`:

```go
package firewall

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

type firewalldBackend struct {
	run runner
}

func newFirewalldBackend(r runner) Backend { return &firewalldBackend{run: r} }

func (f *firewalldBackend) Name() string { return "firewalld" }

func (f *firewalldBackend) Available(ctx context.Context) error {
	stdout, _, err := f.run(ctx, "firewall-cmd", "--state")
	if err != nil {
		return err
	}
	if !bytes.Contains(stdout, []byte("running")) {
		return &backendUnavailable{backend: "firewalld", reason: "状态不是 running"}
	}
	return nil
}

func (f *firewalldBackend) Allow(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		stdout, _, err := f.run(ctx, "firewall-cmd", "--permanent",
			"--add-port="+strconv.Itoa(port)+"/"+proto)
		if err != nil && !isFirewalldAlreadyEnabled(stdout) {
			return err
		}
	}
	return f.reload(ctx)
}

func (f *firewalldBackend) Revoke(ctx context.Context, port int) error {
	for _, proto := range []string{"tcp", "udp"} {
		stdout, _, err := f.run(ctx, "firewall-cmd", "--permanent",
			"--remove-port="+strconv.Itoa(port)+"/"+proto)
		if err != nil && !isFirewalldNotEnabled(stdout) {
			return err
		}
	}
	return f.reload(ctx)
}

func (f *firewalldBackend) reload(ctx context.Context) error {
	_, _, err := f.run(ctx, "firewall-cmd", "--reload")
	return err
}

func isFirewalldAlreadyEnabled(stdout []byte) bool {
	return strings.Contains(string(stdout), "ALREADY_ENABLED")
}

func isFirewalldNotEnabled(stdout []byte) bool {
	return strings.Contains(string(stdout), "NOT_ENABLED")
}
```

- [ ] **Step 4: 把 firewalld 分支加入 selectBackend**

Modify `internal/service/firewall/backend.go` 的 switch：

```go
func selectBackend(name string) (Backend, error) {
	switch name {
	case "ufw":
		return newUFWBackend(defaultRunner), nil
	case "firewalld":
		return newFirewalldBackend(defaultRunner), nil
	default:
		return nil, fmt.Errorf("不支持的 firewall backend: %q", name)
	}
}
```

- [ ] **Step 5: 运行全部测试**

Run: `go test ./internal/service/firewall/ -v`
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/firewall/
git commit -m "feat(firewall): firewalld backend 实现 + 测试"
```

---

### Task 5: Service 成功/失败路径测试 + notify 告警

**Files:**
- Modify: `internal/service/firewall/service_test.go`（追加用例）

- [ ] **Step 1: 追加 Service 启用时的行为测试**

在 `internal/service/firewall/service_test.go` 文件末尾追加：

```go
func TestService_Allow_Success(t *testing.T) {
	b := &fakeBackend{name: "fake"}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	if err := s.Allow(4443); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if len(b.allows) != 1 || b.allows[0] != 4443 {
		t.Fatalf("want allow 4443, got %v", b.allows)
	}
	if len(n.messages) != 0 {
		t.Fatalf("notify should not be called on success: %v", n.messages)
	}
}

func TestService_Allow_BackendFailure_TriggersNotify(t *testing.T) {
	b := &fakeBackend{name: "fake", allowErr: errFake("ufw boom")}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	err := s.Allow(4443)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if len(n.messages) != 1 {
		t.Fatalf("expected 1 notify, got %d: %v", len(n.messages), n.messages)
	}
	if !strings.Contains(n.messages[0], "放行") || !strings.Contains(n.messages[0], "4443") {
		t.Errorf("notify text lacks expected context: %q", n.messages[0])
	}
}

func TestService_Revoke_BackendFailure_TriggersNotify(t *testing.T) {
	b := &fakeBackend{name: "fake", revokeErr: errFake("firewalld down")}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	if err := s.Revoke(4443); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if len(n.messages) != 1 || !strings.Contains(n.messages[0], "关闭") {
		t.Errorf("notify text unexpected: %v", n.messages)
	}
}

func TestService_EnsureAll_ContinuesOnFailure(t *testing.T) {
	b := &fakeBackend{
		name:     "fake",
		allowErr: nil,
	}
	n := &fakeNotifier{}
	s := &Service{backend: b, enabled: true, notify: n}

	s.EnsureAll(context.Background(), []int{10, 20, 30})

	if len(b.allows) != 3 {
		t.Fatalf("expected 3 allows, got %v", b.allows)
	}
}
```

在 test 文件顶部 import 追加 `"strings"`：

```go
import (
	"context"
	"strings"
	"testing"
)
```

`errFake` 由 Task 3 在 `backend_ufw_test.go` 中声明，同 package 可直接复用。

- [ ] **Step 2: 运行全部 firewall 测试**

Run: `go test ./internal/service/firewall/ -v`
Expected: 全部 PASS，新增 4 条用例

- [ ] **Step 3: Commit**

```bash
git add internal/service/firewall/service_test.go
git commit -m "test(firewall): Service 成功/失败/EnsureAll 用例"
```

---

### Task 6: NodeService 注入 firewall + Create / Delete 钩子

**Files:**
- Modify: `internal/service/node.go`

- [ ] **Step 1: 修改 NodeService 构造签名与字段**

在 `internal/service/node.go` 顶部 import 增加：

```go
import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
	"proxy-panel/internal/service/firewall"  // 新增
)
```

修改结构体与构造器：

```go
// NodeService 节点业务逻辑
type NodeService struct {
	db *database.DB
	fw *firewall.Service
}

// NewNodeService 创建节点服务
func NewNodeService(db *database.DB, fw *firewall.Service) *NodeService {
	return &NodeService{db: db, fw: fw}
}
```

- [ ] **Step 2: Create 钩子 —— DB 成功后异步放行**

替换 `Create` 函数末尾的 return 为：

```go
	id, _ := result.LastInsertId()
	node, err := s.GetByID(id)
	if err != nil || node == nil {
		return node, err
	}
	go s.fw.Allow(node.Port)
	return node, nil
}
```

- [ ] **Step 3: Delete 钩子 —— DB 删除前取旧端口，删除后异步撤销**

完整替换 `Delete`：

```go
// Delete 删除节点
func (s *NodeService) Delete(id int64) error {
	old, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if old == nil {
		return fmt.Errorf("节点不存在")
	}
	if _, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id); err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	go s.fw.Revoke(old.Port)
	return nil
}
```

（原先的 RowsAffected 校验被前置的 GetByID 代替，逻辑更一致）

- [ ] **Step 4: 更新调用方构造参数**

Modify `cmd/server/main.go` 第 41 行：

```go
	nodeSvc := service.NewNodeService(db)
```

改为（firewall.Service 的实例化放到 Task 9，本 Task 先传 nil）：

```go
	nodeSvc := service.NewNodeService(db, nil) // firewall 在 Task 9 注入
```

`firewall.Service.Enabled()` 已实现为 `s != nil && s.enabled`；`Allow` / `Revoke` / `EnsureAll` 的首行都是 `if !s.Enabled() { return nil }` —— 因此 nil receiver 调用会被首行直接短路返回，不会解引用字段，nil 安全。

- [ ] **Step 5: 编译验证**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 6: Commit**

```bash
git add internal/service/node.go cmd/server/main.go
git commit -m "feat(node): 注入 firewall.Service，Create/Delete 同步端口"
```

---

### Task 7: NodeService Update 钩子（端口变更 + enable 切换）

**Files:**
- Modify: `internal/service/node.go`

- [ ] **Step 1: 改造 Update 函数前置读取 old，末尾按差异触发钩子**

完整替换 `Update`：

```go
// Update 更新节点（部分更新）
func (s *NodeService) Update(id int64, req *UpdateNodeReq) (*model.Node, error) {
	old, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, nil
	}

	sets := []string{}
	args := []interface{}{}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Host != nil {
		sets = append(sets, "host = ?")
		args = append(args, *req.Host)
	}
	if req.Port != nil {
		sets = append(sets, "port = ?")
		args = append(args, *req.Port)
	}
	if req.Protocol != nil {
		sets = append(sets, "protocol = ?")
		args = append(args, *req.Protocol)
	}
	if req.Transport != nil {
		sets = append(sets, "transport = ?")
		args = append(args, *req.Transport)
	}
	if req.KernelType != nil {
		sets = append(sets, "kernel_type = ?")
		args = append(args, *req.KernelType)
	}
	if req.Settings != nil {
		sets = append(sets, "settings = ?")
		args = append(args, *req.Settings)
	}
	if req.Enable != nil {
		sets = append(sets, "enable = ?")
		args = append(args, *req.Enable)
	}
	if req.SortOrder != nil {
		sets = append(sets, "sort_order = ?")
		args = append(args, *req.SortOrder)
	}

	if len(sets) == 0 {
		return old, nil
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, id)

	query := "UPDATE nodes SET "
	for i, part := range sets {
		if i > 0 {
			query += ", "
		}
		query += part
	}
	query += " WHERE id = ?"

	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("更新节点失败: %w", err)
	}

	s.syncFirewallOnUpdate(old, req)

	return s.GetByID(id)
}

// syncFirewallOnUpdate 按新旧状态差异触发防火墙操作；所有调用都是异步的
// 规则：
//  1. enable 由 true 变 false：撤销旧端口
//  2. enable 由 false 变 true：放行当前端口（可能已被 port 变更）
//  3. enable 未变 且 enable=true：端口变化则撤旧+放新；否则 no-op
//  4. enable 未变 且 enable=false：不操作（节点本就不在防火墙中）
func (s *NodeService) syncFirewallOnUpdate(old *model.Node, req *UpdateNodeReq) {
	newEnable := old.Enable
	if req.Enable != nil {
		newEnable = *req.Enable
	}
	newPort := old.Port
	if req.Port != nil {
		newPort = *req.Port
	}

	switch {
	case old.Enable && !newEnable:
		// 关闭节点：撤销旧端口
		go s.fw.Revoke(old.Port)
	case !old.Enable && newEnable:
		// 重新启用：放行当前端口
		go s.fw.Allow(newPort)
	case old.Enable && newEnable && old.Port != newPort:
		// 仅改端口
		go func(oldPort, port int) {
			s.fw.Revoke(oldPort)
			s.fw.Allow(port)
		}(old.Port, newPort)
	}
}
```

- [ ] **Step 2: 编译**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 3: Commit**

```bash
git add internal/service/node.go
git commit -m "feat(node): Update 按差异同步端口（改端口/切 enable）"
```

---

### Task 8: NodeHandler 响应带 firewall_warning

**Files:**
- Modify: `internal/handler/node.go`

- [ ] **Step 1: 在 Create / Update / Delete 响应体加 firewall_warning**

三处要改。

`Create` 当前末尾：
```go
	// 同步内核配置
	go h.syncSvc.Sync()

	c.JSON(http.StatusCreated, node)
```

改为：

```go
	go h.syncSvc.Sync()

	resp := gin.H{
		"id":          node.ID,
		"name":        node.Name,
		"host":        node.Host,
		"port":        node.Port,
		"protocol":    node.Protocol,
		"transport":   node.Transport,
		"kernel_type": node.KernelType,
		"settings":    node.Settings,
		"enable":      node.Enable,
		"sort_order":  node.SortOrder,
		"created_at":  node.CreatedAt,
		"updated_at":  node.UpdatedAt,
	}
	if w := firewallWarning(h); w != "" {
		resp["firewall_warning"] = w
	}
	c.JSON(http.StatusCreated, resp)
```

`Update` 的 `c.JSON(http.StatusOK, node)` 同样替换为 resp + 可选 firewall_warning。

`Delete` 的 `c.JSON(http.StatusOK, gin.H{"message": "删除成功"})` 改为：

```go
	resp := gin.H{"message": "删除成功"}
	if w := firewallWarning(h); w != "" {
		resp["firewall_warning"] = w
	}
	c.JSON(http.StatusOK, resp)
```

- [ ] **Step 2: 定义 firewallWarning helper**

在文件末尾追加：

```go
// firewallWarning 如果防火墙同步已启用，返回一条异步提示文案；未启用时返回空串
func firewallWarning(h *NodeHandler) string {
	if h.svc == nil || !h.svc.FirewallEnabled() {
		return ""
	}
	return "防火墙同步已异步触发，如需核对请查看系统日志或 ufw/firewall-cmd 当前规则"
}
```

- [ ] **Step 3: 在 NodeService 暴露 FirewallEnabled**

在 `internal/service/node.go` 增加：

```go
// FirewallEnabled 供 handler 判断是否需要返回 firewall_warning
func (s *NodeService) FirewallEnabled() bool {
	return s.fw != nil && s.fw.Enabled()
}
```

- [ ] **Step 4: 编译**

Run: `go build ./...`
Expected: 无错误

- [ ] **Step 5: Commit**

```bash
git add internal/handler/node.go internal/service/node.go
git commit -m "feat(handler): 节点 CRUD 响应附带 firewall_warning"
```

---

### Task 9: main.go 实例化 firewall.Service + 启动对齐

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 导入 firewall 包 + 构造 Service**

在 `cmd/server/main.go` 顶部 import 区加：

```go
	"context"
	"proxy-panel/internal/service/firewall"
```

在 `notifySvc := notify.NewNotifyService(cfg, db)` 之后、`nodeSvc := ...` 之前插入：

```go
	fwSvc, err := firewall.NewService(cfg.Firewall, notifySvc)
	if err != nil {
		log.Printf("防火墙服务初始化失败，已降级为关闭状态: %v", err)
	}
```

- [ ] **Step 2: 把 nil 替换为 fwSvc**

把 Task 6 留下的：

```go
	nodeSvc := service.NewNodeService(db, nil) // firewall 在 Task 9 注入
```

替换为：

```go
	nodeSvc := service.NewNodeService(db, fwSvc)
```

- [ ] **Step 3: 启动时异步 EnsureAll**

在 `scheduler.Start()` 之前插入：

```go
	// 启动时对存量 enable 节点做一次单向 ensure（幂等）
	if fwSvc.Enabled() {
		go func() {
			nodes, err := nodeSvc.ListEnabled()
			if err != nil {
				log.Printf("[防火墙] 启动对齐：读取节点失败: %v", err)
				return
			}
			ports := make([]int, 0, len(nodes))
			for _, n := range nodes {
				ports = append(ports, n.Port)
			}
			fwSvc.EnsureAll(context.Background(), ports)
			log.Printf("[防火墙] 启动对齐完成，处理 %d 个节点端口", len(ports))
		}()
	}
```

- [ ] **Step 4: 编译 + 运行服务冒烟**

Run:
```bash
go build -o proxy-panel ./cmd/server/
# 不启动服务，只验证编译产物存在
ls -l proxy-panel
```
Expected: 二进制已生成

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(main): 实例化 firewall.Service 并启动时 EnsureAll"
```

---

### Task 10: config.example.yaml 增加 firewall 段

**Files:**
- Modify: `config.example.yaml`

- [ ] **Step 1: 在 kernel 段之后追加**

Edit `config.example.yaml` 末尾：

```yaml
kernel:
  xray_path: /usr/local/bin/xray
  xray_config: /opt/proxy-panel/kernel/xray.json
  xray_api_port: 10085
  singbox_path: /usr/local/bin/sing-box
  singbox_config: /opt/proxy-panel/kernel/singbox.json
  singbox_api_port: 9090

firewall:
  enable: false
  backend: ""   # "ufw" | "firewalld"，仅在 enable=true 时生效
```

- [ ] **Step 2: Commit**

```bash
git add config.example.yaml
git commit -m "docs(config): 新增 firewall 段示例"
```

---

### Task 11: install.sh 新增 confirm_default_yes + 扩展 setup_firewall

**Files:**
- Modify: `scripts/install.sh`

- [ ] **Step 1: 在 confirm() 定义下方加 confirm_default_yes()**

定位到 `scripts/install.sh:74` 附近的 confirm 函数：

```bash
confirm() {
    local msg="${1:-确认继续?}"
    read -p "${msg} [y/N]: " answer
    [[ "$answer" =~ ^[Yy]$ ]]
}
```

在其后紧接追加：

```bash
confirm_default_yes() {
    local msg="${1:-确认继续?}"
    read -p "${msg} [Y/n]: " answer
    [[ -z "$answer" || "$answer" =~ ^[Yy]$ ]]
}
```

- [ ] **Step 2: 重写 setup_firewall()**

完整替换 `setup_firewall()`（当前在 scripts/install.sh:872 附近）：

```bash
setup_firewall() {
    step "配置防火墙..."

    local detected=""
    if command -v ufw &>/dev/null; then
        detected="ufw"
    elif command -v firewall-cmd &>/dev/null; then
        detected="firewalld"
    fi

    if [[ -z "$detected" ]]; then
        warn "未检测到防火墙工具，请手动放行端口 ${PANEL_PORT}"
        FIREWALL_ENABLE="false"
        FIREWALL_BACKEND=""
        return
    fi

    # 放行面板端口（保持原有逻辑）
    if [[ "$detected" == "ufw" ]]; then
        ufw allow "${PANEL_PORT}/tcp" >/dev/null 2>&1
        info "ufw 已放行端口 ${PANEL_PORT}"
    else
        firewall-cmd --permanent --add-port="${PANEL_PORT}/tcp" >/dev/null 2>&1
        firewall-cmd --reload >/dev/null 2>&1
        info "firewalld 已放行端口 ${PANEL_PORT}"
    fi

    # 询问是否启用节点端口自动同步
    if confirm_default_yes "是否启用节点端口自动放行（新增/删除节点时自动同步防火墙）?"; then
        FIREWALL_ENABLE="true"
        FIREWALL_BACKEND="$detected"
        info "已启用节点端口自动同步 (backend=$detected)"
    else
        FIREWALL_ENABLE="false"
        FIREWALL_BACKEND=""
        info "已跳过节点端口自动同步（可日后在 config.yaml 的 firewall 段手动开启）"
    fi
}
```

- [ ] **Step 3: 在 generate_config 的 heredoc 末尾加 firewall 段**

定位到 `scripts/install.sh:664` 的 generate_config()，在 `CFGEOF` 结束前、`kernel:` 段之后追加：

```bash
kernel:
  xray_path: /usr/local/bin/xray
  xray_config: ${INSTALL_DIR}/kernel/xray.json
  xray_api_port: 10085
  singbox_path: /usr/local/bin/sing-box
  singbox_config: ${INSTALL_DIR}/kernel/singbox.json
  singbox_api_port: 9090

firewall:
  enable: ${FIREWALL_ENABLE:-false}
  backend: "${FIREWALL_BACKEND:-}"
CFGEOF
```

- [ ] **Step 4: Shell 语法自检**

Run: `bash -n scripts/install.sh`
Expected: 无输出（语法正确）

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh
git commit -m "feat(install): 交互启用节点端口自动同步并写入 firewall 段"
```

---

### Task 12: 全量构建 + 测试 + 手工验证记录

**Files:** 无代码修改；仅验证 + 更新 spec 附录

- [ ] **Step 1: 跑单元测试**

Run: `go test ./... -count=1`
Expected: 全部 PASS

- [ ] **Step 2: 构建二进制**

Run: `go build -o proxy-panel ./cmd/server/`
Expected: 生成 proxy-panel

- [ ] **Step 3: 本地 config 冒烟（firewall 禁用态）**

- 把 `config.example.yaml` 复制为 `config.yaml`（firewall.enable 保持 false）
- Run: `./proxy-panel -config config.yaml`（只跑到启动日志即可 Ctrl+C）
- Expected: 启动日志不出现 `[防火墙]` 相关对齐信息

- [ ] **Step 4: 本地 config 冒烟（firewall 启用 + 无效 backend）**

- 修改 `config.yaml` 为：
  ```yaml
  firewall:
    enable: true
    backend: "bogus"
  ```
- Run: `./proxy-panel -config config.yaml`
- Expected: 启动日志出现 `防火墙服务初始化失败，已降级为关闭状态: 不支持的 firewall backend: "bogus"`，服务继续对外提供 HTTP

- [ ] **Step 5: 手工部署验证清单（写入 spec 附录）**

在 `specs/2026-04-20-firewall-port-sync-design.md` 末尾追加章节：

```markdown
## 手工验证记录

- [ ] Ubuntu 22.04 (ufw)：install → 同意启用 → 新建节点 → `ufw status | grep <port>` 含 tcp/udp 两行
- [ ] Ubuntu 22.04 (ufw)：改端口 → 旧端口消失、新端口出现
- [ ] Ubuntu 22.04 (ufw)：enable → false → 端口消失；enable → true → 端口恢复
- [ ] Ubuntu 22.04 (ufw)：删除节点 → 端口消失
- [ ] CentOS Stream (firewalld)：全套同上，`firewall-cmd --list-ports` 核对
- [ ] 防火墙故障路径：`systemctl stop ufw` 后新建节点 → HTTP 响应 200 带 firewall_warning；Telegram 收到告警
- [ ] 启动对齐：改完 config 开启 firewall.enable 后首次启动 → 日志出现 `启动对齐完成，处理 N 个节点端口`
```

- [ ] **Step 6: Commit 手工验证清单**

```bash
git add specs/2026-04-20-firewall-port-sync-design.md
git commit -m "docs(spec): 防火墙端口同步手工验证清单"
```

---

## 完成标准

- `go test ./... -count=1` 全部通过
- `go build ./...` 无错误
- `bash -n scripts/install.sh` 无错误
- 配置 `firewall.enable=false` 时服务行为与实施前一致
- 配置 `firewall.enable=true` 且 backend 为 ufw/firewalld 时：节点 CRUD 触发对应防火墙命令；命令失败通过 notify 告警且不阻塞 HTTP 响应
- install.sh 首次安装时会询问是否启用，并把选择写入 config.yaml

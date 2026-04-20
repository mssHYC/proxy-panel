# 防火墙端口自动同步 设计文档

- 创建日期：2026-04-20
- 状态：待实现
- 相关代码：`scripts/install.sh`, `cmd/server/main.go`, `internal/service/node.go`, `internal/handler/node.go`, `internal/config`

## 背景

ProxyPanel 的一键安装脚本 `scripts/install.sh` 已经在安装末尾调用 `setup_firewall()` 放行面板端口（`ufw` 或 `firewall-cmd`）。但节点本身的端口是在面板运行后通过 Web UI 增删的，当前这些端口的防火墙规则完全依赖管理员手动维护：

- 新建节点后不放行端口 → 客户端连不上
- 删除节点后不关闭端口 → 留下无用开放端口，扩大攻击面
- 修改节点端口 → 旧端口继续开放、新端口未开放
- 从旧版本升级或安装脚本未启用防火墙时 → 历史节点全部无对应规则

## 目标

1. 在 `install.sh` 中把"节点端口自动同步"作为一项可选能力，首次安装时让管理员选择启用
2. 面板运行过程中，节点 CRUD（含改端口、enable 开关）自动同步到防火墙规则
3. 服务启动时对齐一次：把所有 `enable=true` 节点的端口补齐到防火墙，不清理其它规则
4. 防火墙操作失败不应阻塞业务：降级为告警 + 日志，管理员可观测

## 非目标（YAGNI）

- 不支持 iptables/nftables 原生后端（install.sh 本来就只支持 ufw / firewalld）
- 不做双向 reconcile（不清理"DB 没有但防火墙存在"的规则），因此不引入规则归属标签
- 节点模型不扩展多端口字段
- IPv4/IPv6 不分开管理（ufw / firewalld 默认同步处理）
- 前端暂不新增防火墙状态页，仅在节点 CRUD 响应中返回 `firewall_warning` 字段

## 架构概览

```
install.sh (首次安装)
    ├─ 检测 ufw / firewall-cmd
    ├─ 交互询问 "是否启用节点端口自动放行?" 默认 y
    └─ 写入 config.yaml: firewall.enable / firewall.backend

cmd/server/main.go (每次启动)
    ├─ 加载 config → 实例化 FirewallService
    ├─ 注入 NodeService
    └─ 异步 EnsureAll(enabledNodes)   ← startup reconcile

NodeService.Create/Update/Delete (运行时)
    └─ 调用 FirewallService.Allow / Revoke
         └─ Backend(ufw|firewalld) → exec.CommandContext
```

## 组件设计

### 1. 配置扩展

`config.yaml` 新增段：

```yaml
firewall:
  enable: false
  backend: ""   # "ufw" | "firewalld"
```

对应 Go 结构（`internal/config/config.go`）：

```go
type FirewallConfig struct {
    Enable  bool   `yaml:"enable"`
    Backend string `yaml:"backend"`
}
```

`config.example.yaml` 同步加入该段，值保持默认（关闭）。

### 2. `internal/service/firewall` 包

新建包，暴露：

```go
// Backend 抽象具体防火墙工具
type Backend interface {
    Name() string
    Available(ctx context.Context) error
    Allow(ctx context.Context, port int) error
    Revoke(ctx context.Context, port int) error
}

// Service 节点侧调用入口
type Service struct {
    backend Backend
    enabled bool
    notify  Notifier   // 抽象接口，实现由 internal/service/notify.NotifyService 提供
    log     *log.Logger
}

// Notifier 防火墙包内定义的最小接口，避免和 notify 包循环依赖
type Notifier interface {
    SendAll(message string)
}

func NewService(cfg config.FirewallConfig, notify Notifier) (*Service, error)

func (s *Service) Enabled() bool
func (s *Service) Allow(port int) error                  // enable=false 时 no-op
func (s *Service) Revoke(port int) error
func (s *Service) EnsureAll(ctx context.Context, ports []int)  // startup reconcile
```

关键约束：
- `enabled=false` 所有方法 no-op 且返回 nil
- `Allow` / `Revoke` 内部同时处理 tcp+udp（两条规则）
- 任一 backend 命令失败：warn 日志 + `notify.SendAll(告警消息)`，返回 error 但不 panic
- 所有 exec 调用使用 `exec.CommandContext` + 5s 超时

### 3. Backend 实现

`backend_ufw.go`：

- `Allow(port)`：`ufw allow <port>/tcp` && `ufw allow <port>/udp`
- `Revoke(port)`：`ufw delete allow <port>/tcp` && `ufw delete allow <port>/udp`
- `Available()`：`ufw status` 返回值包含 `Status: active`
- ufw 对已存在规则的 `allow` 返回 `Skipping adding existing rule`（退出码 0），天然幂等
- ufw 对不存在规则的 `delete` 返回非 0 —— 捕获 stderr 若包含 `Could not delete non-existent rule` 视为成功

`backend_firewalld.go`：

- `Allow(port)`：`firewall-cmd --permanent --add-port=<port>/tcp` && `...=udp` && `firewall-cmd --reload`
- `Revoke(port)`：`firewall-cmd --permanent --remove-port=<port>/tcp` && `...=udp` && `firewall-cmd --reload`
- `Available()`：`firewall-cmd --state` 返回 `running`
- firewalld 的 add/remove 对已存在/不存在规则返回 `ALREADY_ENABLED` / `NOT_ENABLED`，作为成功处理

Backend 选择逻辑：`NewService` 根据 `cfg.Backend` 字段 switch；未知值返回错误，`main.go` 捕获后 warn 并把 `enabled` 置 false（不阻止服务启动）。

### 4. NodeService 生命周期钩子

`NodeService` 构造函数新增 `fw *firewall.Service` 依赖：

```go
func NewNodeService(db *database.DB, fw *firewall.Service) *NodeService
```

- `Create(req)`：DB 插入成功后 `go fw.Allow(req.Port)`
- `Delete(id)`：
  1. `GetByID(id)` 拿旧 port（已存在逻辑）
  2. `db.Exec("DELETE ...")`
  3. `go fw.Revoke(oldPort)`
- `Update(id, req)`：
  1. `GetByID(id)` 拿旧 node（已存在逻辑）
  2. UPDATE DB
  3. 按差异触发：
     - `req.Port != nil && *req.Port != old.Port`：`go func() { fw.Revoke(old.Port); fw.Allow(*req.Port) }()`
     - `req.Enable != nil && old.Enable && !*req.Enable`：`go fw.Revoke(old.Port)`
     - `req.Enable != nil && !old.Enable && *req.Enable`：`go fw.Allow(old.Port)`
     - 若同一请求既改 port 又改 enable：按最终状态决定（`enable=true` 且 port 变 → revoke old + allow new；`enable=false` → revoke old）
     - 其它字段变动：不触发

### 5. HTTP Handler 响应

`NodeHandler.Create` / `Update` / `Delete` 不直接感知防火墙失败（因为钩子是异步的），但：

- 响应体新增 `firewall_warning` 字段，值是 `fw.Enabled()` 为 true 时的提示文案（例如 `"防火墙同步已异步触发，如需核对请查看系统日志"`），为 false 时不输出该字段
- 防火墙失败的用户感知走 notify 告警渠道（Telegram / 企微），不依赖 HTTP 响应回显

选择异步而非同步的理由：
- 避免 5s 超时阻塞 HTTP 响应
- 与现有 `go h.syncSvc.Sync()` 行为一致
- 幂等操作天然允许重试和对齐

### 6. 启动 reconcile

`cmd/server/main.go` 在实例化 `FirewallService` 后：

```go
fwSvc, err := firewall.NewService(cfg.Firewall, notifySvc)
if err != nil {
    log.Printf("防火墙服务初始化失败: %v（已降级为关闭状态）", err)
}
nodeSvc := service.NewNodeService(db, fwSvc)
// ... 其它初始化
if fwSvc.Enabled() {
    go func() {
        nodes, _ := nodeSvc.ListEnabled()
        ports := make([]int, 0, len(nodes))
        for _, n := range nodes { ports = append(ports, n.Port) }
        fwSvc.EnsureAll(context.Background(), ports)
    }()
}
```

`EnsureAll` 逐个端口 `Allow`，失败仅 warn 不中断；整体非阻塞，服务正常对外提供 HTTP。

### 7. install.sh 变更

扩展 `setup_firewall()`：

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
    # 注：install.sh 现有的 confirm() 默认 N，需先扩展签名以支持默认值 Y
    if confirm_default_yes "是否启用节点端口自动放行（新增/删除节点时自动同步防火墙）?"; then
        FIREWALL_ENABLE="true"
        FIREWALL_BACKEND="$detected"
        info "已启用节点端口自动同步 (backend=$detected)"
    else
        FIREWALL_ENABLE="false"
        FIREWALL_BACKEND=""
    fi
}
```

`generate_config` 的 heredoc 模板追加：

```yaml
firewall:
  enable: ${FIREWALL_ENABLE:-false}
  backend: "${FIREWALL_BACKEND:-}"
```

现有 `confirm()` 签名：`confirm "msg"` 读 `[y/N]` 默认 N。本功能需要默认 Y，新增一个 `confirm_default_yes()` 辅助（读 `[Y/n]`，空输入或 y/Y 视为 true），放在脚本顶部与 `confirm()` 同区域。

## 数据流

新增节点：
```
前端 POST /api/nodes
  └─ handler.Create → service.Create → DB INSERT
      └─ go fw.Allow(port)
          └─ ufw allow <port>/tcp + udp   （或 firewall-cmd 等价命令）
              └─ 失败 → notify.SendAll("节点端口 X 放行失败: ...")
  └─ 200 { ...node, firewall_warning? }
```

删除节点：
```
前端 DELETE /api/nodes/:id
  └─ handler.Delete → service.GetByID → service.Delete(DB DELETE)
      └─ go fw.Revoke(oldPort)
          └─ ufw delete allow <port>/tcp + udp
              └─ 失败 → notify.Send(...)
```

启动时：
```
main.go
  └─ firewall.NewService(cfg, notify)
  └─ go fw.EnsureAll(listEnabled().Ports)
```

## 错误处理

| 场景 | 行为 |
|---|---|
| `cfg.Firewall.Enable=false` | 服务完全 no-op，HTTP 响应不带 `firewall_warning` |
| `cfg.Firewall.Backend` 为未知值 | `NewService` 返回 error，`main.go` warn 并降级 enabled=false |
| 具体命令非 0 退出（rule 已存在/不存在除外） | warn 日志 + notify 告警，返回 error（异步调用方已不 care） |
| 命令执行超时（>5s） | 同上 |
| `EnsureAll` 中某端口失败 | 仅记该端口，继续处理后续端口 |
| install.sh 未检测到防火墙工具 | `firewall.enable=false` 写入 config，保持现状 warn |

## 测试策略

### 单元测试（`internal/service/firewall`）

- `Service` 注入 `fakeBackend`（内存记录 Allow/Revoke 调用）：
  - `enabled=false` 所有方法 no-op
  - `Allow` 命中 backend 失败时触发 notify
  - `EnsureAll` 遍历所有端口，一个失败不影响其他
- Backend（ufw / firewalld）实现注入 `runner func(ctx, name, args...) ([]byte, error)`：
  - 断言命令行参数序列
  - mock stderr 包含 `Skipping adding existing rule` / `ALREADY_ENABLED` → 返回 nil
  - mock 超时 → 返回 error

### 集成测试（`internal/service`）

- `NodeService` 测试文件注入 fake `firewall.Service`，验证：
  - Create 调用一次 Allow(newPort)
  - Delete 调用一次 Revoke(oldPort)
  - Update 改端口 → Revoke(old) + Allow(new) 各一次
  - Update 关闭 enable → Revoke(old)
  - Update 重新启用 enable → Allow(port)
  - Update 只改 name → 不调用

### 手工验证

- Ubuntu 22.04 (ufw)：安装 → 启用防火墙同步 → 面板建/改/删节点 → `ufw status numbered` 核对
- CentOS Stream (firewalld)：同上 → `firewall-cmd --list-ports` 核对
- 停掉 ufw 服务后操作节点：确认服务不崩、仅 notify 告警
- 从未启用该功能的旧版本升级后启动：确认 EnsureAll 补齐全部 enable=true 节点端口

## 发布与兼容性

- `firewall.enable` 默认 false，升级时不动存量系统的防火墙
- 旧版 `config.yaml` 缺该段时 yaml 解析得到零值（enable=false / backend=""），符合默认行为
- install.sh 运行在已安装系统上（update 子命令）不重复问，仅 install 子命令的首次流程走交互

## 开放问题

无（设计已覆盖澄清问答中的全部决策）。

# ProxyPanel 迭代路线图

> 基于当前代码结构（Gin + SQLite 单机，Xray/Sing-box 双内核，5 格式订阅）整理的后续可迭代方向。
> 最后更新：2026-04-23

---

## P0 — 近期高价值（业务核心）

### 1. 多节点集群与远程 Agent
- **现状**：`internal/service/health_check.go` 仅做单机探活，配置下发靠 `kernel_sync.go` 本机启停。
- **目标**：Master / Agent 架构，Master 下发配置、Agent 上报心跳、在线用户数、负载、流量。
- **拆分**：
  - 定义 Agent 通信协议（gRPC 或 HTTP + 签名）
  - Node 模型增加 `agent_endpoint / agent_token / last_seen`
  - Master 侧改 `kernel_sync` 为"本机模式 / 远程模式"双路径
  - 前端节点页加在线状态徽标
- **验收**：两台 VPS，一台 Master 一台 Agent，配置变更 5 秒内生效；Agent 离线 30s 内前端显示离线。

### 2. 真实流量回传（替换估算）
- **现状**：`traffic.go` 多为估算或订阅端计数。
- **目标**：对接 Xray Stats gRPC API 与 Sing-box Clash API，周期性拉取用户级上/下行。
- **拆分**：
  - 新增 `internal/service/traffic/collector.go`，内核差异封装
  - 用户维度 + 节点维度 + 小时级聚合入库
  - 图表页补"按节点分布 / 按时段趋势"
- **验收**：实际跑流量和面板显示误差 < 5%。

### 3. 订阅令牌安全 ✅ 已完成 2026-04-23
- 多 token 模型（`subscription_tokens` 表），支持启用/禁用、过期、首访 IP 绑定、轮换
- UA 自动识别（Surge / Clash / Sing-box / Shadowrocket / v2ray），URL `?format=` 优先
- 新端点 `/api/sub/t/:token`；旧 `/api/sub/:uuid` 保留向后兼容，响应头 `X-Subscription-Deprecated`
- 管理 UI：用户详情弹窗新增「Token 管理」页
- 设计/计划：[specs/2026-04-23-subscription-token-security-design.md](specs/2026-04-23-subscription-token-security-design.md) · [plans/2026-04-23-subscription-token-security-plan.md](plans/2026-04-23-subscription-token-security-plan.md)

### 4. 套餐 / 节点分组
- **现状**：用户-节点多对多，分发粒度粗。
- **目标**：引入 `plan` 实体（节点组 + 流量上限 + 有效期 + 价格占位），用户关联到 plan。
- **拆分**：
  - DB 迁移：`plans`、`user.plan_id`
  - 到期自动禁用 + 提前 3/1 天告警（复用 `notify`）
  - 前端增套餐管理页

---

## P1 — 中期（运维与体验）

### 5. 可观测性（Prometheus + Grafana）
- `internal/service/metrics.go` 已有基础，补 `/metrics` HTTP 端点。
- 暴露指标：
  - `proxy_panel_kernel_sync_duration_seconds`
  - `proxy_panel_subscription_requests_total{client}`
  - `proxy_panel_node_online`、`proxy_panel_traffic_bytes`
- 随仓库提供一份 Grafana dashboard JSON。

### 6. 审计日志检索
- `audit.go` 已落库，补前端过滤 UI（操作人 / 时间 / 资源类型）与 CSV 导出。

### 7. 配置热加载（减少断连）
- **现状**：修改 setting → `kernel_sync` 全量重启内核 → 所有连接断开。
- **目标**：
  - Xray：用 API 做 inbound/outbound 增删改
  - Sing-box：用 cache file + 平滑切换
- **验收**：新增/删除用户不断开其它人连接。

### 8. i18n
- 前端抽出文案到 `web/src/i18n/`，先支持 zh-CN / en-US。

### 9. 备份增强
- `backup.go` 增加定时任务 + 远端目的地（S3 / WebDAV），恢复时做 schema 校验。

---

## P2 — 长期（架构与商业化）

### 10. 存储抽象
- 为多节点做准备，抽 `database/` 层接口，提供 SQLite / MySQL / Postgres 实现。
- **动机**：多 Master 或高并发场景 SQLite WAL 会成为瓶颈。
- **风险**：当前 `sqlx` 风格直接拼 SQL，迁移成本随代码量线性增长 —— 越早越好。

### 11. 计费与支付
- 接 Stripe / 爱发电 / USDT，用户自助续费、套餐升降级。
- 依赖 #4（套餐模型）。

### 12. 移动端 PWA
- 用户侧订阅二维码 + 流量查看，管理员侧关键告警推送。

---

## 技术债 / 质量

### 13. 内核配置生成的 Golden Test
- **动机**：最近多个 commit 在修 HY2 的 `masquerade / server_name / alpn` 兼容问题（`d881be6`、`681318b`），没有回归测试保护。
- **方案**：每种协议准备一组输入 fixture，生成的 JSON 与 `testdata/*.golden.json` 比对。
- **覆盖**：VLESS / VMess / Trojan / Shadowsocks / Hysteria2 / Reality。

### 14. SQL Scan 与 Struct 的编译期校验
- **动机**：`502f442` 修 `ListByUserID` scanNode 字段遗漏，此类 bug 运行时才暴露。
- **选项**：
  - 迁移到 `sqlc`（推荐，编译期强校验）
  - 或写反射测试：扫描所有 `scanXxx` 函数，对比 struct 字段数
- **优先级**：中 —— 每新增一个字段都有风险。

### 15. kernel_sync 防抖覆盖率
- `kernel_sync_debounce_test.go` 已存在，补：并发写入、配置回滚、失败重试场景。

---

## 提案模板

每个条目启动前建议走一次 `superpowers:brainstorming`，然后用 `superpowers:writing-plans` 产出独立 plan 放到 `plans/YYYY-MM-DD-<topic>-plan.md`。

## 里程碑建议

| 里程碑 | 包含 | 目标周期 |
|---|---|---|
| M1 真实流量 + 订阅安全 | #2 ~~#3~~ #13 | 2-3 周 |
| M2 多节点集群 | #1 #7 #15 | 4-6 周 |
| M3 商业化闭环 | #4 #11 #6 | 4 周 |
| M4 架构升级 | #10 #5 #14 | 6 周 |

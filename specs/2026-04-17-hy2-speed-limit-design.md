# Hysteria2 节点限速设计

日期：2026-04-17
状态：已确认，待实施

## 背景

当前 `users.speed_limit` UI 可填但**从未下发到内核**，纯装饰。小团队共享场景（<10 人朋友间）需要防止"一人独占整条管道"。

## 目标

- Hy2 节点支持节点级总带宽上限下发到 sing-box
- 保留用户维度的限速字段，在单用户独享节点时精确生效
- 不引入外部依赖（tc / iptables / eBPF）

## 非目标

- 不做 VLESS/VMess/Trojan 协议限速（Xray-core 无原生支持）
- 不做严格按源 IP 的 per-user 限速
- 不改 `user_nodes` 关联筛选逻辑

## 设计

### 数据模型

- **节点**：`nodes.settings` JSON 追加两个字段
  - `max_up_mbps` (int, 0 或缺省 = 不限速)
  - `max_down_mbps` (int, 0 或缺省 = 不限速)
  - 字段存入现有 TEXT 列，无 schema 迁移
- **用户**：`users.speed_limit` 字段保持（单一 int，Mbps，双向对称上限，0 = 不限速）

### 前端变更

**[web/src/views/Nodes.vue]**（Hy2 配置块，SNI/证书下方）：
- `el-input-number` "最大上行 (Mbps)"：`:min=0 :max=20`，默认 10
- `el-input-number` "最大下行 (Mbps)"：`:min=0 :max=20`，默认 10
- 说明文字："0 = 不限速。该值是节点总带宽上限，所有使用此节点的用户共享。"

**[web/src/views/Users.vue]**（speed_limit 字段保留）：
- 加 tooltip：**"仅在该用户独享某节点时严格生效；多用户共用同一节点时退化为节点级总带宽限制。"**

### 后端下发逻辑

**"单用户"判定口径**：`len(users) == 1`，其中 `users` 是 `buildInbound` 接收的全量参数（当前架构下即 `KernelSyncService.loadUsers()` 返回的全系统 enable=1 用户，不按 `user_nodes` 关联筛选）。若未来引入按节点关联筛选，此判定自动跟随收敛到"该节点关联用户数"。

**[internal/kernel/singbox.go]** `buildInbound` hy2 分支：

```go
upMbps   := getSettingInt(s, "max_up_mbps", 0)
downMbps := getSettingInt(s, "max_down_mbps", 0)

// 单用户场景下用户 speed_limit 精确生效（取更严格的那个）
if len(users) == 1 && users[0].SpeedLimit > 0 {
    userLim := int(users[0].SpeedLimit)
    if upMbps == 0 || userLim < upMbps {
        upMbps = userLim
    }
    if downMbps == 0 || userLim < downMbps {
        downMbps = userLim
    }
}

if upMbps > 0   { inbound["up_mbps"]   = upMbps   }
if downMbps > 0 { inbound["down_mbps"] = downMbps }
```

### 工具函数

**[internal/kernel/xray.go]** 新增 `getSettingInt(m map[string]interface{}, key string, defaultVal int) int`，与现有 `getSettingStr` 同风格，处理 `float64`/`int`/`int64`/`string` 多种 JSON 反序列化来源类型。

### 兼容性

- 历史节点 settings 无新字段 → `getSettingInt` 返回 0 → 不生效，等同当前无限速行为
- 历史用户 `speed_limit` 字段继续存在，语义未改

## 测试

手工验证清单（客户端侧用 speedtest-cli 或 iperf 跑流量）：

| 场景 | 配置 | 期望 |
|---|---|---|
| 节点级限速 | `max_down_mbps=5`, 无 user speed_limit | 配置含 `down_mbps=5`，实测 ≈5 Mbps |
| 用户级更严 | 唯一用户 `speed_limit=3`, `max_down_mbps=10` | 下发 `down_mbps=3`（取 min）|
| 用户级更宽 | 唯一用户 `speed_limit=100`, `max_down_mbps=10` | 下发 `down_mbps=10`（节点 max 兜底）|
| 节点不填+单用户 | 唯一用户 `speed_limit=3`, 节点无 max | 下发 `down_mbps=3`（用户级替代生效）|
| 多用户降级 | 两个启用用户（任意 speed_limit）+ `max_down_mbps=10` | 下发 `down_mbps=10`，user 字段被忽略 |
| 完全无限速 | 节点不填 + 所有用户 speed_limit=0 | 配置无 `down_mbps`/`up_mbps` 字段 |

## 风险与限制

- **多用户共用同一 hy2 节点时，user 侧 speed_limit 不严格生效**——这是 sing-box `hysteria2.User` 无 per-user bandwidth 字段的根本限制，通过 UI tooltip 与 spec 明确告知
- **上限 20 Mbps 是 UI 硬编码**——防误填，未来需要突破可改常量；若有需要可进一步把上限抽成全局配置

## 实施工作量

估计 1.5-2 小时：
- 后端：singbox.go + xray.go（helper）约 30 行
- 前端：Nodes.vue 2 个 input、Users.vue tooltip 约 40 行
- 手工测试 + 发版 v1.1.9

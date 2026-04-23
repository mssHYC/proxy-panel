# 订阅 Token 安全 — 设计文档

- **状态**：Approved (brainstorming)
- **日期**：2026-04-23
- **对应 ROADMAP 条目**：P0 #3 订阅令牌安全
- **作者**：brainstorming via superpowers

---

## 1. 背景与目标

### 1.1 现状

订阅端点 `GET /subscribe/:uuid` 直接使用 `User.UUID` 作为凭证。`UUID` 同时是内核下发时的用户身份字段（Xray/Sing-box 配置里的 id/uuid/email 关联），一旦外泄无法回收 —— 更换 UUID 会破坏内核侧已建立的所有连接。

格式选择靠 `?format=` 查询参数，没有 UA 识别；没有过期、IP 绑定、启用/禁用、审计。

### 1.2 目标

1. 把「订阅凭证」从「用户身份」中彻底解耦，一个用户可拥有多个命名 token。
2. 每个 token 支持：启用/禁用、过期时间、首访自动 IP 绑定、轮换。
3. 订阅请求默认按 URL 参数返回格式，未指定时自动识别客户端 UA。
4. 保持向后兼容：现有 `/subscribe/:uuid` 链接继续可用一段时间。

### 1.3 非目标（本次不做）

- 一次性 / 限次 token (`max_uses`)
- Token 灰度回收（宽限期）
- 限制单 token 只能拉特定格式 (`allowed_formats`)
- 订阅链接 JWT 化（随机串已够）
- 多设备会话管理等高级审计

---

## 2. 数据模型

### 2.1 新表 `subscription_tokens`

```sql
CREATE TABLE subscription_tokens (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL,
  name          TEXT NOT NULL,                      -- 用户自命名，如 "iPhone"
  token         TEXT NOT NULL UNIQUE,               -- 32 字节 base64url 随机串
  enabled       INTEGER NOT NULL DEFAULT 1,
  expires_at    DATETIME,                           -- NULL = 永不过期
  ip_bind_enabled INTEGER NOT NULL DEFAULT 1,       -- 0 = 关闭 IP 绑定
  bound_ip      TEXT,                               -- NULL 且 ip_bind_enabled=1 → 首访自动填
  last_ip       TEXT,
  last_ua       TEXT,
  last_used_at  DATETIME,
  use_count     INTEGER NOT NULL DEFAULT 0,
  created_at    DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_sub_tokens_user ON subscription_tokens(user_id);
CREATE INDEX idx_sub_tokens_token ON subscription_tokens(token);
```

### 2.2 User 表不变

`User.UUID` 继续承担内核身份字段职责，与订阅 token 完全解耦。

### 2.3 Token 字符串生成

- 32 字节 `crypto/rand` → base64url 无填充（长度 43）
- 碰撞概率忽略不计；入库前用 UNIQUE 约束兜底，冲突则重试一次

---

## 3. 路由

| Method | Path | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/subscribe/t/:token` | 无 | 新订阅端点 |
| GET | `/subscribe/:uuid` | 无 | **Deprecated**，保留向后兼容 |
| GET | `/api/users/:id/sub-tokens` | JWT | 列出指定用户的 token |
| POST | `/api/users/:id/sub-tokens` | JWT | 创建 token |
| PATCH | `/api/sub-tokens/:id` | JWT | 改 `name`/`enabled`/`expires_at`/`reset_bind` |
| POST | `/api/sub-tokens/:id/rotate` | JWT | 生成新随机串，旧 token 立即失效（原 id 保留） |
| DELETE | `/api/sub-tokens/:id` | JWT | 删除 token |

### 3.1 旧端点行为

旧端点 `/subscribe/:uuid` 在迁移后：

1. 把 `:uuid` 当作 `token` 查 `subscription_tokens` 表（迁移会为每个用户创建一条 `token = uuid` 的记录，见 §7）。
2. 走与新端点完全相同的校验与生成流程。
3. 响应附加响应头：`X-Subscription-Deprecated: please migrate to /subscribe/t/<token>`。
4. 保留至少两个 release，之后删除。

---

## 4. 订阅请求处理流程（新端点）

```
1. 按 token 查 subscription_tokens
   ├─ 不存在              → 404
   ├─ enabled = 0         → 403 {"error": "token 已禁用"}
   ├─ expires_at < now()  → 410 Gone {"error": "token 已过期"}
   ├─ ip_bind_enabled = 0 → 跳过 IP 校验
   ├─ bound_ip IS NULL    → 原子 UPDATE：SET bound_ip = :ip WHERE id = :id AND bound_ip IS NULL
   └─ bound_ip ≠ :ip      → 403 {"error": "token 已绑定其他 IP"}

2. 按 user_id 查 User
   ├─ user.enable = 0        → 403 (沿用现有文案)
   └─ 流量耗尽                → 403 (沿用现有文案)

3. 决定格式
   format := c.Query("format")
   if format == "" {
       format = SniffFormat(c.GetHeader("User-Agent"))
   }
   if format == "" {
       format = "v2ray"
   }

4. 生成订阅内容（复用现有 subscription.GetGenerator）

5. 异步更新审计字段（不阻塞响应）
   UPDATE subscription_tokens
   SET last_ip = :ip, last_ua = :ua, last_used_at = now(), use_count = use_count + 1
   WHERE id = :id
```

### 4.1 客户端 IP 获取

统一用 `c.ClientIP()`，受 gin `trusted_proxies` 配置约束。部署在 nginx/Cloudflare 后必须正确配置 `X-Forwarded-For` 受信链，否则所有请求 IP 都会是代理 IP，导致：

- 首访绑定把全部用户绑到同一个代理 IP
- IP 校验全部通过

**风险缓解**：启动时检查 `trusted_proxies` 是否配置，若为空且检测到常见反代头时打印 warning。

---

## 5. UA 识别

新文件 `internal/service/subscription/ua.go`：

```go
package subscription

import "regexp"

var uaPatterns = []struct {
    re     *regexp.Regexp
    format string
}{
    {regexp.MustCompile(`(?i)surge`), "surge"},
    {regexp.MustCompile(`(?i)shadowrocket`), "shadowrocket"},
    {regexp.MustCompile(`(?i)quantumult`), "shadowrocket"},
    {regexp.MustCompile(`(?i)clash|stash|mihomo`), "clash"},
    {regexp.MustCompile(`(?i)sing-box|singbox`), "singbox"},
    {regexp.MustCompile(`(?i)v2ray|v2box`), "v2ray"},
}

// SniffFormat 根据 UA 识别订阅格式。无法识别返回空串。
func SniffFormat(ua string) string {
    for _, p := range uaPatterns {
        if p.re.MatchString(ua) {
            return p.format
        }
    }
    return ""
}
```

优先级：`?format=` > UA > `v2ray`（默认）。

---

## 6. 管理 API 细节

### 6.1 POST `/api/users/:id/sub-tokens`

请求：
```json
{"name": "iPhone", "expires_at": "2026-12-31T23:59:59Z"}
```
响应：
```json
{"id": 12, "name": "iPhone", "token": "<43-char>", "url": "https://panel.example.com/subscribe/t/<token>", ...}
```

### 6.2 PATCH `/api/sub-tokens/:id`

支持字段：`name`、`enabled`、`expires_at`（null 表示永不过期）、`ip_bind_enabled`、`reset_bind`（true = 清空 `bound_ip`）。

### 6.3 POST `/api/sub-tokens/:id/rotate`

生成新的 `token` 字符串，`use_count`/`last_*` 不清零（审计连续性），`bound_ip` 清空（新链接重新绑定）。旧 token 立即失效。

### 6.4 权限

当前项目仅有 admin 用户体系 —— 所有 `/api/sub-tokens/*` 走现有 JWT 中间件即可。如后续引入多租户再加 owner 校验。

---

## 7. 数据迁移

加入到现有 `internal/database` 迁移流程：

```sql
-- 1. 建表（见 §2.1）

-- 2. 为每个现有用户创建一条 default token，复用其 UUID 作为 token 值
-- default token 关闭 IP 绑定，避免破坏现有使用习惯
INSERT INTO subscription_tokens (user_id, name, token, enabled, ip_bind_enabled, created_at)
SELECT id, 'default', uuid, 1, 0, created_at
FROM users
WHERE NOT EXISTS (SELECT 1 FROM subscription_tokens st WHERE st.user_id = users.id);
```

**效果**：
- 旧链接 `/subscribe/<uuid>` 直接当成 token 查新表即可命中，获得与新端点一致的能力（只是 default token 没有过期/IP 绑定，行为与旧版等价）。
- 用户新建任何 token 后，可在 UI 上删除 default token 完成迁移。

---

## 8. 前端改动

用户列表 → 每行「订阅」按钮 → 弹出 Drawer：

- 顶部：「+ 新建 Token」按钮
- 列表项：
  - 名称（可编辑）
  - 订阅链接（带「复制」「二维码」按钮）
  - 过期时间（可编辑，空=永不）
  - IP 绑定开关（`ip_bind_enabled`）
  - 绑定 IP（显示当前值；「清除重绑」按钮，仅在开关打开时显示）
  - 启用开关
  - 「轮换」按钮（二次确认）
  - 「删除」按钮（二次确认）
- 底部：最后使用 IP / UA / 时间 / 使用次数

用户详情原有的「复制订阅链接」按钮：
- 若该用户有 default token（迁移遗留）且只有一个 token，显示但标注「旧版链接 - 请在 Token 管理中创建新链接并删除此默认 token」。
- 否则隐藏。

---

## 9. 限流

在订阅路由上挂一个 IP 限流中间件：同 IP 每分钟 60 次（足够任意客户端自动刷新频率，远超爆破可接受阈值）。

复用 `internal/router` 现有限流基础设施；如果目前只有登录限流，扩展成通用限流中间件，保留与登录限流的独立桶（key 前缀不同）。

超限返回 `429 Too Many Requests`。

---

## 10. 测试

### 10.1 单元测试

- `subscription.SniffFormat` 对所有客户端 UA 字符串返回正确 format
- token 生成函数产出长度、字符集正确的字符串
- token 校验逻辑各分支（不存在 / 禁用 / 过期 / 首访绑定 / IP 不匹配）

### 10.2 集成测试

- 两次不同 IP 访问同一 token：第一次成功并绑定，第二次 403
- 轮换 token 后，旧 token 立即返回 404
- 删除 token 后返回 404
- PATCH `reset_bind=true` 后原 IP 与新 IP 都能访问（新 IP 会重新绑定）
- 旧端点 `/subscribe/:uuid` 迁移后行为与之前一致且响应头带 `X-Subscription-Deprecated`
- UA 识别优先级：同时提供 `?format=` 和客户端 UA 时以 URL 为准

### 10.3 手工验收

- 真实 Surge/Clash/Sing-box 客户端拉取无 `?format=` 链接，各自拿到正确格式

---

## 11. 风险与缓解

| 风险 | 缓解 |
|---|---|
| `trusted_proxies` 未配置导致 IP 绑定失效 | 启动时检测 + warning；文档强调 |
| 用户把自己锁死（换 IP 后访问不了） | UI 显眼的「清除重绑」入口；错误响应体提示可联系管理员清绑 |
| 旧端点删除节奏 | 至少保留 2 个 release；每次生成订阅响应都带 Deprecated 头 |
| 迁移回滚 | 迁移使用 `NOT EXISTS` 保证幂等；表可直接 DROP，User 表未改 |

---

## 12. 实施范围外（后续可迭代）

- `max_uses` 一次性 / 限次 token
- Token 操作审计日志（谁在何时创建/轮换/删除）—— 可沿用现有 `audit.go`
- 订阅链接签名 JWT 化（自包含 payload，支持离线校验）
- Token 限制 CIDR 而非单 IP

---

## 13. 验收标准

1. 每个用户可通过 UI 创建 ≥ 1 个命名 token，并生成对应订阅链接。
2. 过期 token 返回 410，禁用 token 返回 403，不存在返回 404。
3. 首次访问 token 自动绑定 IP；不同 IP 访问被拒；UI 可清除重绑。
4. 轮换后旧 token 立即失效；删除后返回 404。
5. 未带 `?format=` 时，Surge/Clash/Sing-box/Shadowrocket 客户端各自拿到对应格式。
6. 现有 `/subscribe/:uuid` 链接仍可用，响应头带 Deprecated 提示。
7. IP 限流：同 IP 超过 60 req/min 返回 429。
8. 所有单元测试 + 集成测试通过。

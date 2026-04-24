# 分流配置重构设计

| 项 | 内容 |
|---|---|
| 文档日期 | 2026-04-24 |
| 分支 | feat/routing-refactor（待建） |
| 参考 | [sublink-worker](https://github.com/7Sageer/sublink-worker) |
| 作者 | huangyuchuan + Claude |

## 1. 背景与目标

### 1.1 现状

分流配置当前由两个 `settings` 表键承载：

- `custom_rules`：多行文本，逐行 `TYPE,VALUE,OUTBOUND`
- `custom_rules_mode`：`prepend` | `override`

生成端硬编码：

- `internal/service/subscription/rules.go`：全局变量 + 18 个固定出站组名
- `clash.go`（169-276、294-332 行）：rule-provider URL、默认规则清单
- `singbox.go`（194-256 行）：默认规则映射
- `surge.go` / `v2ray.go` / `shadowrocket.go`：各自硬编码

**痛点**：换规则源（镜像切换）要发版；用户无法增删出站组；无分类粒度启用开关；无"预设方案"概念；老文本格式难以结构化编辑。

### 1.2 目标

对齐 sublink-worker 的分流配置架构，引入：

1. 系统内置 18 个规则分类（URL 前缀可覆写）+ 用户自定义分类
2. 规范化出站组模型（seed 现有 18 组，可增删改）
3. 结构化自定义规则（替代裸文本）
4. 3 个预设方案 minimal / balanced / comprehensive，订阅 URL 支持 `?preset=` 临时覆盖
5. 生成器层引入格式无关的中间表示（IR），5 个客户端格式改为消费 IR
6. 启动时自动迁移老 `custom_rules` 文本

### 1.3 非目标

- 多 profile / token 绑 profile
- Rule-provider 远端资源的服务端缓存 / 刷新管理
- GeoIP MMDB 本地托管
- 分类的 `behavior` 三态区分（domain / ipcidr / classical），统一 site / ip 两类标签

## 2. 决策摘要

| 维度 | 选择 | 说明 |
|---|---|---|
| 重构范围 | 完全对齐 sublink-worker | 预设 + 结构化规则 + URL 预设参数 |
| 分类目录 | 代码内置 + seed + URL 前缀覆写 + 支持自定义 | 18 系统分类只读；可覆盖 base URL；可增自定义分类 |
| 出站组 | 规范化表，seed 现有 18 组 | 系统组不可删，用户可增删改 |
| 方案 profile | 单份持久化 + URL 参数预设 | 无多 profile；订阅 `?preset=xxx` 即时覆盖 |
| 存储 | 规范化表（4 张新表）+ settings 标量 | 不用大 JSON blob |
| 迁移 | 启动时自动导入老文本 | 迁移后删除老键 |
| 生成器分层 | 中间表示层 IR + 各端 translator | 新增 `internal/service/routing` 包 |

## 3. 数据模型

### 3.1 新增表

```sql
CREATE TABLE rule_categories (
  id                    INTEGER PRIMARY KEY AUTOINCREMENT,
  code                  TEXT NOT NULL UNIQUE,       -- 'google' | 'youtube' | 'my_steam'
  display_name          TEXT NOT NULL,
  kind                  TEXT NOT NULL,              -- 'system' | 'custom'
  site_tags             TEXT NOT NULL DEFAULT '[]', -- JSON: ["google","youtube"]
  ip_tags               TEXT NOT NULL DEFAULT '[]',
  inline_domain_suffix  TEXT NOT NULL DEFAULT '[]',
  inline_domain_keyword TEXT NOT NULL DEFAULT '[]',
  inline_ip_cidr        TEXT NOT NULL DEFAULT '[]',
  protocol              TEXT NOT NULL DEFAULT '',   -- 'tcp' | 'udp' | ''
  default_group_id      INTEGER,
  enabled               INTEGER NOT NULL DEFAULT 1,
  sort_order            INTEGER NOT NULL DEFAULT 0,
  created_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (default_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
);

CREATE TABLE outbound_groups (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  code          TEXT NOT NULL UNIQUE,    -- 'node_select' | 'auto_select' | 'streaming'
  display_name  TEXT NOT NULL,           -- '🚀 手动切换'
  type          TEXT NOT NULL,           -- 'selector' | 'urltest'
  members       TEXT NOT NULL DEFAULT '[]', -- JSON, 支持 '<ALL>' 宏
  kind          TEXT NOT NULL,           -- 'system' | 'custom'
  sort_order    INTEGER NOT NULL DEFAULT 0,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE custom_rules (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  name              TEXT NOT NULL,
  site_tags         TEXT NOT NULL DEFAULT '[]',
  ip_tags           TEXT NOT NULL DEFAULT '[]',
  domain_suffix     TEXT NOT NULL DEFAULT '[]',
  domain_keyword    TEXT NOT NULL DEFAULT '[]',
  ip_cidr           TEXT NOT NULL DEFAULT '[]',
  src_ip_cidr       TEXT NOT NULL DEFAULT '[]',
  protocol          TEXT NOT NULL DEFAULT '',
  port              TEXT NOT NULL DEFAULT '',
  outbound_group_id INTEGER,              -- 指向 outbound_groups.id
  outbound_literal  TEXT NOT NULL DEFAULT '', -- 'DIRECT' | 'REJECT' | ''（二选一）
  sort_order        INTEGER NOT NULL DEFAULT 0,
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (outbound_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
);

CREATE TABLE rule_presets (
  code               TEXT PRIMARY KEY,   -- 'minimal' | 'balanced' | 'comprehensive'
  display_name       TEXT NOT NULL,
  enabled_categories TEXT NOT NULL DEFAULT '[]' -- JSON: ['cn','private','google']
);
```

**约束**：`custom_rules` 中 `outbound_group_id` 与 `outbound_literal` 必须恰好一个非空，业务层校验（SQLite 触发器非必需）。

### 3.2 settings 新增键

| 键 | 默认值 | 说明 |
|---|---|---|
| `routing.site_ruleset_base_url.clash` | `https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/` | Clash .mrs 前缀 |
| `routing.ip_ruleset_base_url.clash` | 同上但 `geoip/` | |
| `routing.site_ruleset_base_url.singbox` | `https://ghfast.top/.../sing/geo/geosite/` | Sing-box .srs 前缀 |
| `routing.ip_ruleset_base_url.singbox` | 同上但 `geoip/` | |
| `routing.final_outbound` | `node_select` | 兜底出站 group code |
| `routing.active_preset` | `""` | 当前生效预设（空 = 自定义） |

### 3.3 seed 数据

- **18 系统分类**：与 sublink-worker 对齐 —— Ad Block / AI Services / Bilibili / Youtube / Google / Private / Location:CN / Telegram / Github / Microsoft / Apple / Social Media / Streaming / Gaming / Education / Financial / Cloud Services / Non-China。每条 `site_tags` / `ip_tags` 按 sublink 约定写入，`default_group_id` 指向对应系统组（如 Streaming→streaming 组，Private/CN→direct 兜底群）
- **18 系统出站组**：照搬 `internal/service/subscription/rules.go` 现有 18 个组名；至少包含 `auto_select`（urltest）、`node_select`（selector）两个基础组
- **3 预设**：
  - `minimal`：`['location_cn','private','non_china']`
  - `balanced`：minimal + `['github','google','youtube','ai_services','telegram']`
  - `comprehensive`：全部 18

## 4. 中间表示层（`internal/service/routing`）

### 4.1 包结构

```
internal/service/routing/
  plan.go        // Plan / Rule / OutboundGroup / ProviderURLs 类型
  builder.go     // BuildPlan 主入口
  presets.go     // 应用预设覆盖 enabled
  legacy.go      // 老 custom_rules 文本解析（迁移用）
  store.go       // DB 读写辅助（避免 handler 裸 SQL）
  builder_test.go
```

### 4.2 核心类型

```go
type Plan struct {
    Groups    []OutboundGroup
    Rules     []Rule
    Providers Providers
    Final     string              // group code
}

type Rule struct {
    SiteTags       []string
    IPTags         []string
    DomainSuffix   []string
    DomainKeyword  []string
    IPCIDR         []string
    SrcIPCIDR      []string
    Protocol       []string
    Port           []string
    Outbound       string         // group code 或 'DIRECT'/'REJECT'
}

type OutboundGroup struct {
    Code, DisplayName, Type string
    Members []string               // 支持 '<ALL>' 宏，展开为全节点 tag
}

type Providers struct {
    Site map[string]ProviderURLs   // tag → URL
    IP   map[string]ProviderURLs
}

type ProviderURLs struct {
    Clash   string
    Singbox string
}

type BuildOptions struct {
    PresetOverride string           // 'minimal'|'balanced'|'comprehensive'|''
    ClientFormat   string           // 'clash'|'singbox'|'surge'|'v2ray'|'shadowrocket'
}

func BuildPlan(ctx context.Context, db *sql.DB, opts BuildOptions) (*Plan, error)
```

### 4.3 规则合并顺序

`Plan.Rules` 生成顺序（与 sublink-worker 对齐）：

1. 所有 `custom_rules` 按 `sort_order` 追加（最高优先）
2. 所有 `enabled = 1` 的 `rule_categories` 按 `sort_order` 转为 Rule（site_tags/ip_tags/inline_* 合并进同一 Rule，`Outbound = category.default_group.code`）
3. 若 `PresetOverride` 非空：读 `rule_presets.enabled_categories` 作为第 2 步的 enabled 白名单（临时覆盖 DB 的 enabled 字段）

### 4.4 `<ALL>` 宏

`OutboundGroup.Members` 中的 `<ALL>` 由 translator 在渲染时展开为当前订阅的全部节点 tag。节点列表由 translator 负责传入（IR 层不感知节点）。

## 5. Translator（各端生成器瘦身）

### 5.1 Clash

- `Plan.Providers.Site/IP` → `rule-providers:` 块（`type: http`，`behavior: domain/ipcidr`，`format: mrs`，`url` 取 `ProviderURLs.Clash`）
- `Plan.Rules` → `rules:` 块
  - `SiteTags` 每项 → `RULE-SET,{tag},{outbound}`
  - `IPTags` 每项 → `RULE-SET,{tag}-ip,{outbound}`（IP 规则集 tag 加 `-ip` 后缀）
  - `DomainSuffix` 每项 → `DOMAIN-SUFFIX,{v},{outbound}`
  - `DomainKeyword` / `IPCIDR` 类似
  - 末尾 `MATCH,{Final}`
- `Plan.Groups` → `proxy-groups:`（`<ALL>` 宏展开为所有节点 name）

### 5.2 Sing-box

- `Plan.Providers` → `route.rule_set[]`（每项 `{tag, type: remote, format: binary, url}`）
- `Plan.Rules` → `route.rules[]`：单 Rule 可同时含 `rule_set`、`domain_suffix`、`ip_cidr`、`protocol` 字段
- `Plan.Groups` → `outbounds[]`（`selector` / `urltest`）
- `route.final = Plan.Final` 对应的 group tag

### 5.3 Surge

Surge 没有 rule-provider 概念，Translator 行为：

- `SiteTags` → `RULE-SET,{SITE_RULESET_BASE}{tag}.list,{outbound}`（指向可被 Surge 识别的远端 `.list` 文本；需要确认 sublink-worker Surge 输出用的是哪套 URL，若无公共 `.list` 源则 **降级**为 GEOSITE 规则并记日志）
- `IPTags` → 同理指向 `.list` 或降级为 `IP-CIDR`
- Inline 字段直接输出
- `Plan.Groups` → `[Proxy Group]` 节

### 5.4 V2Ray / Shadowrocket

能力降级：

- 保留 site_tags / ip_tags → `geosite:xxx` / `geoip:xxx`
- `domain_keyword` / `domain_suffix` / `ip_cidr` → 内联
- 不支持的字段（如 `src_ip_cidr`）→ skip + `log.Warn`

### 5.5 生成器入口统一

```go
// 当前：handler.Subscribe 里调 SetCustomRules(...) + 各生成器自读
// 重构后：
plan, _ := routing.BuildPlan(ctx, db, routing.BuildOptions{
    PresetOverride: r.URL.Query().Get("preset"),
    ClientFormat:   clientFormat,
})
content := translators[clientFormat].Render(plan, nodes)
```

## 6. 迁移

新 migration 文件 `internal/database/migrations/2026_04_24_routing_refactor.go`（配合现有 `migrations.go` 风格）：

1. `CREATE TABLE` ×4（顺序：`outbound_groups` → `rule_categories` → `custom_rules` → `rule_presets`，前两者存在 FK 依赖）
2. `INSERT` 18 系统分类 + 18 系统出站组 + 3 预设 + 6 个 settings 默认键
3. **自动导入老数据**：
   - 读 `settings.custom_rules`（若存在且非空）
   - 调用 `routing.ParseLegacyRules(text)`，按 18 个旧组名对齐到新 `outbound_groups.code`
   - 按 `settings.custom_rules_mode`：
     - `prepend`（默认）→ 解析结果 insert 进 `custom_rules`，`sort_order` 递增
     - `override` → 同上 + 将所有 `rule_categories.enabled = 0`（关闭默认分类，仅保留自定义）
   - 迁移成功后 `DELETE FROM settings WHERE key IN ('custom_rules', 'custom_rules_mode')`

**幂等**：migration 执行前检查 `routing_migration_v1` 键；已迁移则 skip 第 3 步。

**legacy 解析器**：`legacy.go` 对等实现 `web/src/views/settings/rules-types.ts` 的 `parseRules()`。支持行格式：`DOMAIN,value,Outbound` / `DOMAIN-SUFFIX,value,Outbound` / `DOMAIN-KEYWORD,...` / `IP-CIDR,...` / `IP-CIDR6,...` / `GEOSITE,...` / `GEOIP,...` / `PROCESS-NAME,...`，注释 `#` 忽略。

## 7. API 设计（`internal/handler/routing.go`）

| Method | Path | 功能 |
|---|---|---|
| GET | `/api/routing/config` | 返回 `{categories, groups, customRules, presets, settings}` 一次性 payload |
| POST | `/api/routing/categories` | 新增自定义分类（kind=custom） |
| PUT | `/api/routing/categories/:id` | 更新（系统分类仅允许改 enabled / default_group_id / sort_order） |
| DELETE | `/api/routing/categories/:id` | 删除（系统分类禁止） |
| POST | `/api/routing/groups` | 新增自定义出站组 |
| PUT | `/api/routing/groups/:id` | 更新（系统组仅允许改 members / display_name） |
| DELETE | `/api/routing/groups/:id` | 删除（系统组禁止；有分类引用时 409） |
| POST | `/api/routing/custom-rules` | 新增自定义规则 |
| PUT | `/api/routing/custom-rules/:id` | 更新 |
| DELETE | `/api/routing/custom-rules/:id` | 删除 |
| PUT | `/api/routing/custom-rules/order` | 批量改 sort_order |
| POST | `/api/routing/apply-preset` | `{code}` 覆盖当前启用分类（写 DB） |
| POST | `/api/routing/import-legacy` | 手动二次导入（body: `{text, mode}`） |

订阅端点（已有）新增 `?preset=` 查询参数，handler 转交给 `routing.BuildOptions.PresetOverride`。

所有 `/api/routing/*` 复用现有 admin 鉴权中间件。

## 8. 前端（`web/src/views/settings/routing/`）

新目录 `routing/`，`RulesSection.vue` 重写为顶层容器，含 4 个 tab：

1. **规则分类** `CategoriesTab.vue`
   - 表格列：启用开关 / 名称 / 类型 badge(system/custom) / site_tags(chip) / ip_tags(chip) / 默认出站组下拉 / 操作
   - 顶部：预设下拉（minimal/balanced/comprehensive/自定义）+ 应用按钮（带确认框）
   - 系统分类：只暴露启用/默认出站组编辑；自定义分类：完整编辑对话框

2. **出站组** `GroupsTab.vue`
   - 表格列：code / display_name / type / members(排序拖拽) / 操作
   - 成员编辑支持 `<ALL>` 宏 chip、选择其他组、输入字面量（DIRECT/REJECT）

3. **自定义规则** `CustomRulesTab.vue`
   - 结构化表格：name / site_tags 多选 / ip_tags 多选 / domain_suffix 多行 / ip_cidr / outbound 下拉（group 或字面量） / 拖拽排序

4. **高级** `AdvancedTab.vue`
   - 4 个 URL 前缀输入（clash/singbox × site/ip）
   - 兜底出站下拉
   - "从旧格式导入" 文本框 + 模式选择 + 提交按钮

API 客户端新增 `web/src/api/routing.ts`；删除 `web/src/views/settings/rules-types.ts`（legacy 解析移到后端）。

## 9. 向后兼容

- `GET /api/settings` 返回的 `custom_rules` / `custom_rules_mode` 字段：读侧返回空字符串并在响应 `meta.deprecated` 标注
- `PUT /api/settings` 中这两个字段：**忽略写入**，返回 200 但 `meta.warnings = ['routing.legacy_ignored']`
- 订阅 URL 本身不变；未传 `?preset=` 时行为与旧默认完全一致（`active_preset = ''` + 启用分类 + 自定义规则）

## 10. 测试

| 层 | 类型 | 内容 |
|---|---|---|
| `routing.BuildPlan` | 单元 | fixture 3 种预设 × 2 种自定义规则组合 → Plan 快照 |
| 5 个 translator | golden file | 同一 Plan IR → 5 个输出文件 diff |
| 迁移 | 集成 | 在临时 SQLite 中写老文本 → 跑 migration → 断言新表行数/字段 |
| legacy 解析 | 单元 | 覆盖 8 种规则类型 + 18 组名 + 注释 + 空行 |
| `/api/routing/*` | handler | CRUD 正反例 + 系统资源保护 |
| 订阅 `?preset=` | 集成 | 三种 preset 下 Clash 输出中 `rules:` 数量差异 |

## 11. 实施清单

- [ ] 新增 `internal/database/migrations/2026_04_24_routing_refactor.go`
- [ ] 新增 `internal/service/routing/` 包（plan / builder / presets / legacy / store + 测试）
- [ ] 新增 `internal/handler/routing.go` + 路由注册
- [ ] 重写 5 个 generator（`clash.go` / `singbox.go` / `surge.go` / `v2ray.go` / `shadowrocket.go`）改为消费 Plan
- [ ] 删除 `internal/service/subscription/rules.go`
- [ ] `internal/handler/subscription.go` 接入 `?preset=` 参数
- [ ] `internal/handler/setting.go` 接入 deprecation 逻辑
- [ ] 前端 `web/src/views/settings/routing/` 4 个 tab + `api/routing.ts`
- [ ] 删除 `web/src/views/settings/rules-types.ts` + `RulesTable.vue`（被新 tab 组件取代）
- [ ] 更新 README / ROADMAP

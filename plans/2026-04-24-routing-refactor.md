# 分流配置重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把分流配置从"裸文本 + 硬编码"改造为"规范化表 + 格式无关 IR + 多端 translator"，对齐 sublink-worker 的分流配置能力。

**Architecture:** 新增 `internal/service/routing` 包产出 `Plan`（格式无关的中间表示）；五个现有 generator 改为消费 `Plan` 并翻译为 Clash/Sing-box/Surge/V2Ray/Shadowrocket 各自格式。DB 层新增 4 张规范化表（`rule_categories` / `outbound_groups` / `custom_rules` / `rule_presets`），migration 时 seed 18 系统分类 + 18 系统出站组 + 3 预设方案，并自动导入老 `settings.custom_rules` 文本。前端 `RulesSection.vue` 拆成 4 个 tab。

**Tech Stack:** Go 1.22 / Gin / SQLite / Vue 3 + TypeScript / Vite

**Spec:** `specs/2026-04-24-routing-refactor-design.md`

---

## File Structure

### 新建

| 路径 | 职责 |
|---|---|
| `internal/service/routing/plan.go` | `Plan` / `Rule` / `OutboundGroup` / `Providers` 类型 |
| `internal/service/routing/presets.go` | 18 系统分类 / 18 系统组 / 3 预设常量（seed 数据源） |
| `internal/service/routing/store.go` | DB 读写：ListCategories / ListGroups / ListCustomRules 等 |
| `internal/service/routing/builder.go` | `BuildPlan(ctx, db, opts)` 主入口 |
| `internal/service/routing/legacy.go` | 解析老 `custom_rules` 文本为 `[]CustomRuleRow` |
| `internal/service/routing/builder_test.go` | IR 构建单元测试 |
| `internal/service/routing/legacy_test.go` | legacy 解析测试 |
| `internal/handler/routing.go` | `/api/routing/*` CRUD |
| `internal/handler/routing_test.go` | handler 级单测（可选，优先集成测试） |
| `web/src/api/routing.ts` | 前端 API 客户端 |
| `web/src/views/settings/routing/RoutingSection.vue` | 顶层容器（4 tab） |
| `web/src/views/settings/routing/CategoriesTab.vue` | 规则分类 tab |
| `web/src/views/settings/routing/GroupsTab.vue` | 出站组 tab |
| `web/src/views/settings/routing/CustomRulesTab.vue` | 自定义规则 tab |
| `web/src/views/settings/routing/AdvancedTab.vue` | 高级 tab（URL 前缀 / legacy 导入） |
| `web/src/views/settings/routing/types.ts` | 前端类型 |

### 修改

| 路径 | 改动 |
|---|---|
| `internal/database/migrations.go` | 追加 4 张新表 + seed 查询 + 自动迁移老文本逻辑 |
| `internal/service/subscription/clash.go` | 改为消费 `*routing.Plan` |
| `internal/service/subscription/singbox.go` | 同上 |
| `internal/service/subscription/surge.go` | 同上（带降级） |
| `internal/service/subscription/v2ray.go` | 同上（带降级） |
| `internal/service/subscription/shadowrocket.go` | 同上（带降级） |
| `internal/handler/subscription.go` | 用 `routing.BuildPlan` 替代 `SetCustomRules`；接入 `?preset=` |
| `internal/handler/setting.go` | `custom_rules`/`custom_rules_mode` 键 deprecate（读空、写忽略） |
| `internal/router/router.go` | 注册 `/api/routing/*` |
| `cmd/server/main.go` | 如有需要在启动时调 legacy 迁移（若放在 migrations.go 则无改动） |

### 删除

| 路径 | 原因 |
|---|---|
| `internal/service/subscription/rules.go` | 全局变量/ProxyGroupNames 被 `routing` 包取代 |
| `web/src/views/settings/RulesSection.vue` | 被 `routing/RoutingSection.vue` 取代 |
| `web/src/views/settings/RulesTable.vue` | 被新 tab 组件取代 |
| `web/src/views/settings/rules-types.ts` | 解析逻辑移到后端 legacy 包；前端类型移到 `routing/types.ts` |

---

## 执行顺序与依赖

- Task 1 → 2 → 3（后端基础设施）
- Task 4（legacy 解析）独立，可与 2-3 并行
- Task 5（migration 中调用 legacy 迁移）依赖 1、4
- Task 6（builder）依赖 1-3
- Task 7-11（5 个 translator）依赖 6；彼此独立
- Task 12（subscription handler）依赖 6-11
- Task 13-14（admin API）依赖 3
- Task 15（settings 兼容）独立
- Task 16-20（前端）依赖 13-14
- Task 21-22（清理 + 文档）最后

---

## Task 1: 新增 4 张表 schema + 索引

**Files:**
- Modify: `internal/database/migrations.go:115`（在 `subscription_tokens` seed 语句之后追加）

- [ ] **Step 1: 编辑 `migrations.go`，在 `queries` 切片末尾（现有 `INSERT INTO subscription_tokens ...` 之后）追加 4 张表**

```go
// 分流配置 - 规则分类
`CREATE TABLE IF NOT EXISTS outbound_groups (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    code          TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    type          TEXT NOT NULL,
    members       TEXT NOT NULL DEFAULT '[]',
    kind          TEXT NOT NULL,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE TABLE IF NOT EXISTS rule_categories (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    code                  TEXT NOT NULL UNIQUE,
    display_name          TEXT NOT NULL,
    kind                  TEXT NOT NULL,
    site_tags             TEXT NOT NULL DEFAULT '[]',
    ip_tags               TEXT NOT NULL DEFAULT '[]',
    inline_domain_suffix  TEXT NOT NULL DEFAULT '[]',
    inline_domain_keyword TEXT NOT NULL DEFAULT '[]',
    inline_ip_cidr        TEXT NOT NULL DEFAULT '[]',
    protocol              TEXT NOT NULL DEFAULT '',
    default_group_id      INTEGER,
    enabled               INTEGER NOT NULL DEFAULT 1,
    sort_order            INTEGER NOT NULL DEFAULT 0,
    created_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (default_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
)`,
`CREATE TABLE IF NOT EXISTS custom_rules (
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
    outbound_group_id INTEGER,
    outbound_literal  TEXT NOT NULL DEFAULT '',
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (outbound_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
)`,
`CREATE TABLE IF NOT EXISTS rule_presets (
    code               TEXT PRIMARY KEY,
    display_name       TEXT NOT NULL,
    enabled_categories TEXT NOT NULL DEFAULT '[]'
)`,
`CREATE INDEX IF NOT EXISTS idx_custom_rules_sort ON custom_rules(sort_order)`,
`CREATE INDEX IF NOT EXISTS idx_rule_categories_sort ON rule_categories(sort_order)`,
`CREATE INDEX IF NOT EXISTS idx_outbound_groups_sort ON outbound_groups(sort_order)`,
```

- [ ] **Step 2: 编译验证**

Run: `cd /Users/huangyuchuan/Desktop/proxy_panel && go build ./...`
Expected: 编译通过（仅 schema 新增，尚无消费方）

- [ ] **Step 3: 启动服务器冒烟测试**

Run: `go run ./cmd/server -c config.example.yaml 2>&1 | head -30`
Expected: 启动日志不含 SQL 错误，DB 文件出现 4 张新表。用 `sqlite3 data/proxy-panel.db '.schema rule_categories'` 验证。

- [ ] **Step 4: 提交**

```bash
git add internal/database/migrations.go
git commit -m "feat(routing): add schema for rule_categories/outbound_groups/custom_rules/rule_presets"
```

---

## Task 2: `routing` 包 — 类型定义 (`plan.go`)

**Files:**
- Create: `internal/service/routing/plan.go`

- [ ] **Step 1: 创建 `plan.go`**

```go
package routing

// Plan 是格式无关的分流规划中间表示。
// 由 BuildPlan 生成，交给各 translator 翻译为具体客户端格式。
type Plan struct {
    Groups    []OutboundGroup
    Rules     []Rule
    Providers Providers
    Final     string // 兜底出站：group code 或 DIRECT/REJECT
}

type Rule struct {
    SiteTags      []string
    IPTags        []string
    DomainSuffix  []string
    DomainKeyword []string
    IPCIDR        []string
    SrcIPCIDR     []string
    Protocol      []string
    Port          []string
    Outbound      string // group code 或 'DIRECT'/'REJECT'
}

type OutboundGroup struct {
    Code        string
    DisplayName string
    Type        string   // 'selector' | 'urltest'
    Members     []string // 支持 '<ALL>' 宏
}

type Providers struct {
    Site map[string]ProviderURLs
    IP   map[string]ProviderURLs
}

type ProviderURLs struct {
    Clash   string
    Singbox string
}

// BuildOptions 由 subscription handler 传入。
type BuildOptions struct {
    PresetOverride string // 'minimal'|'balanced'|'comprehensive'|''
    ClientFormat   string // 'clash'|'singbox'|'surge'|'v2ray'|'shadowrocket'
}

// IsLiteralOutbound 判断 outbound 是否为字面量（非 group code）。
func IsLiteralOutbound(s string) bool {
    return s == "DIRECT" || s == "REJECT"
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/service/routing/...`
Expected: 通过。

- [ ] **Step 3: 提交**

```bash
git add internal/service/routing/plan.go
git commit -m "feat(routing): add Plan/Rule/OutboundGroup IR types"
```

---

## Task 3: `routing/presets.go` — 18 系统分类 / 18 系统组 / 3 预设 常量

**Files:**
- Create: `internal/service/routing/presets.go`

- [ ] **Step 1: 创建 `presets.go`**

```go
package routing

// SystemGroup 描述一个系统预置出站组的 seed。
type SystemGroup struct {
    Code        string
    DisplayName string
    Type        string
    Members     []string
    SortOrder   int
}

// SystemCategory 描述一个系统预置规则分类的 seed。
type SystemCategory struct {
    Code                string
    DisplayName         string
    SiteTags            []string
    IPTags              []string
    InlineDomainSuffix  []string
    InlineDomainKeyword []string
    InlineIPCIDR        []string
    Protocol            string
    DefaultGroupCode    string // 指向 SystemGroup.Code
    Enabled             bool
    SortOrder           int
}

// SystemPreset 描述一个预设方案的 seed。
type SystemPreset struct {
    Code              string
    DisplayName       string
    EnabledCategories []string
}

// 默认 URL 前缀（可被 settings 覆写）。
const (
    DefaultClashSiteBase   = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/"
    DefaultClashIPBase     = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/"
    DefaultSingboxSiteBase = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/"
    DefaultSingboxIPBase   = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/"
    DefaultFinalGroup      = "node_select"
)

// SystemGroups 定义 18 个内置出站组。
// <ALL> 宏在 translator 渲染时展开为所有节点名。
var SystemGroups = []SystemGroup{
    {Code: "node_select", DisplayName: "🚀 手动切换", Type: "selector", Members: []string{"auto_select", "<ALL>", "DIRECT"}, SortOrder: 10},
    {Code: "auto_select", DisplayName: "⚡ 自动选择", Type: "urltest", Members: []string{"<ALL>"}, SortOrder: 20},
    {Code: "global_proxy", DisplayName: "🌐 全球代理", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 30},
    {Code: "streaming", DisplayName: "🎬 流媒体", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 40},
    {Code: "telegram", DisplayName: "✈️ Telegram", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 50},
    {Code: "google", DisplayName: "🔍 Google", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 60},
    {Code: "youtube", DisplayName: "📺 YouTube", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 70},
    {Code: "netflix", DisplayName: "🎥 Netflix", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 80},
    {Code: "spotify", DisplayName: "🎵 Spotify", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 90},
    {Code: "hbo", DisplayName: "🎞 HBO", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 100},
    {Code: "bing", DisplayName: "🔎 Bing", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 110},
    {Code: "openai", DisplayName: "🤖 OpenAI", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 120},
    {Code: "claude_ai", DisplayName: "🤖 ClaudeAI", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 130},
    {Code: "disney", DisplayName: "🏰 Disney", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 140},
    {Code: "github", DisplayName: "💻 GitHub", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 150},
    {Code: "cn_media", DisplayName: "🇨🇳 国内媒体", Type: "selector", Members: []string{"DIRECT", "node_select"}, SortOrder: 160},
    {Code: "direct", DisplayName: "🎯 本地直连", Type: "selector", Members: []string{"DIRECT", "node_select"}, SortOrder: 170},
    {Code: "fallback", DisplayName: "🐟 漏网之鱼", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 180},
}

// SystemCategories 定义 18 个内置分类。site_tags / ip_tags 与 MetaCubeX/meta-rules-dat 的 geosite/geoip 文件名对齐。
var SystemCategories = []SystemCategory{
    {Code: "private", DisplayName: "局域网", IPTags: []string{"private"}, DefaultGroupCode: "direct", Enabled: true, SortOrder: 10},
    {Code: "location_cn", DisplayName: "Location:CN", SiteTags: []string{"cn"}, IPTags: []string{"cn"}, DefaultGroupCode: "direct", Enabled: true, SortOrder: 20},
    {Code: "ad_block", DisplayName: "广告拦截", SiteTags: []string{"category-ads-all"}, DefaultGroupCode: "fallback", Enabled: false, SortOrder: 30},
    {Code: "ai_services", DisplayName: "AI 服务", SiteTags: []string{"openai", "anthropic", "gemini", "category-ai-chat-!cn"}, DefaultGroupCode: "openai", Enabled: true, SortOrder: 40},
    {Code: "bilibili", DisplayName: "Bilibili", SiteTags: []string{"bilibili"}, DefaultGroupCode: "cn_media", Enabled: false, SortOrder: 50},
    {Code: "youtube", DisplayName: "YouTube", SiteTags: []string{"youtube"}, IPTags: []string{"google"}, DefaultGroupCode: "youtube", Enabled: true, SortOrder: 60},
    {Code: "google", DisplayName: "Google", SiteTags: []string{"google"}, IPTags: []string{"google"}, DefaultGroupCode: "google", Enabled: true, SortOrder: 70},
    {Code: "telegram", DisplayName: "Telegram", SiteTags: []string{"telegram"}, IPTags: []string{"telegram"}, DefaultGroupCode: "telegram", Enabled: true, SortOrder: 80},
    {Code: "github", DisplayName: "GitHub", SiteTags: []string{"github"}, DefaultGroupCode: "github", Enabled: true, SortOrder: 90},
    {Code: "microsoft", DisplayName: "Microsoft", SiteTags: []string{"microsoft"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 100},
    {Code: "apple", DisplayName: "Apple", SiteTags: []string{"apple"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 110},
    {Code: "social_media", DisplayName: "社交媒体", SiteTags: []string{"facebook", "twitter", "instagram", "tiktok"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 120},
    {Code: "streaming", DisplayName: "流媒体", SiteTags: []string{"netflix", "disney", "hbo", "spotify"}, DefaultGroupCode: "streaming", Enabled: false, SortOrder: 130},
    {Code: "gaming", DisplayName: "游戏", SiteTags: []string{"category-games-!cn"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 140},
    {Code: "education", DisplayName: "教育", SiteTags: []string{"category-education-!cn"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 150},
    {Code: "financial", DisplayName: "金融", SiteTags: []string{"paypal", "stripe"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 160},
    {Code: "cloud_services", DisplayName: "云服务", SiteTags: []string{"amazon", "aws", "cloudflare"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 170},
    {Code: "non_china", DisplayName: "Non-China", SiteTags: []string{"geolocation-!cn"}, DefaultGroupCode: "fallback", Enabled: true, SortOrder: 900},
}

// SystemPresets 对应 sublink-worker 的 minimal / balanced / comprehensive。
var SystemPresets = []SystemPreset{
    {Code: "minimal", DisplayName: "最小规则", EnabledCategories: []string{"private", "location_cn", "non_china"}},
    {Code: "balanced", DisplayName: "均衡规则", EnabledCategories: []string{"private", "location_cn", "non_china", "github", "google", "youtube", "ai_services", "telegram"}},
    {Code: "comprehensive", DisplayName: "完整规则", EnabledCategories: allCategoryCodes()},
}

func allCategoryCodes() []string {
    out := make([]string, 0, len(SystemCategories))
    for _, c := range SystemCategories {
        out = append(out, c.Code)
    }
    return out
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/service/routing/...`
Expected: 通过。

- [ ] **Step 3: 提交**

```bash
git add internal/service/routing/presets.go
git commit -m "feat(routing): define 18 system categories + 18 system groups + 3 presets"
```

---

## Task 4: `routing/legacy.go` — 老文本规则解析器

**Files:**
- Create: `internal/service/routing/legacy.go`
- Create: `internal/service/routing/legacy_test.go`

**背景：** 老 `settings.custom_rules` 是多行文本，每行形如：
```
DOMAIN-SUFFIX,google.com,Google
IP-CIDR,1.1.1.1/32,DIRECT
GEOSITE,cn,本地直连
# 这是注释
```
需解析为结构化 `LegacyRule` 以便 migration 写入 `custom_rules` 表。

- [ ] **Step 1: 创建 `legacy_test.go`（先写测试）**

```go
package routing

import (
    "reflect"
    "testing"
)

func TestParseLegacyRules_AllTypes(t *testing.T) {
    text := `
# comment line
DOMAIN,example.com,Google
DOMAIN-SUFFIX,gmail.com,Google
DOMAIN-KEYWORD,youtube,YouTube
IP-CIDR,1.1.1.1/32,本地直连
IP-CIDR6,2001::/64,本地直连
GEOSITE,cn,本地直连
GEOIP,cn,本地直连
PROCESS-NAME,curl,DIRECT

`
    rules, err := ParseLegacyRules(text)
    if err != nil {
        t.Fatalf("err: %v", err)
    }
    if len(rules) != 8 {
        t.Fatalf("expected 8 rules, got %d", len(rules))
    }
    // 第一条：DOMAIN → domain_suffix 字段用来承载精确匹配？按 rules-types.ts 的实现应有 DOMAIN 类型：
    // 我们按 rules-types.ts 处理：DOMAIN 存到 DomainSuffix 并前缀 '='，这里简化为 Domain 字段。
    if rules[0].Type != "DOMAIN" || rules[0].Value != "example.com" || rules[0].Outbound != "Google" {
        t.Fatalf("rule[0] = %+v", rules[0])
    }
}

func TestParseLegacyRules_IgnoreBlankAndComments(t *testing.T) {
    text := "# hello\n\n  \n"
    rules, err := ParseLegacyRules(text)
    if err != nil {
        t.Fatalf("err: %v", err)
    }
    if len(rules) != 0 {
        t.Fatalf("expected 0, got %+v", rules)
    }
}

func TestParseLegacyRules_MalformedSkipped(t *testing.T) {
    text := "NOT-A-RULE\nDOMAIN-SUFFIX,foo.com\nDOMAIN-SUFFIX,foo.com,X"
    rules, err := ParseLegacyRules(text)
    if err != nil {
        t.Fatalf("err: %v", err)
    }
    // 前两行格式不对（字段不足）跳过，只保留最后一条
    if !reflect.DeepEqual(rules, []LegacyRule{{Type: "DOMAIN-SUFFIX", Value: "foo.com", Outbound: "X"}}) {
        t.Fatalf("got %+v", rules)
    }
}

func TestMapLegacyOutboundToCode(t *testing.T) {
    cases := map[string]string{
        "手动切换": "node_select",
        "自动选择": "auto_select",
        "Google": "google",
        "DIRECT":  "DIRECT",
        "REJECT":  "REJECT",
        "漏网之鱼": "fallback",
        "Unknown": "", // 未识别返回空，由调用方决定
    }
    for in, want := range cases {
        if got := MapLegacyOutboundToCode(in); got != want {
            t.Errorf("MapLegacyOutboundToCode(%q) = %q, want %q", in, got, want)
        }
    }
}
```

- [ ] **Step 2: 运行测试确认失败（未实现）**

Run: `go test ./internal/service/routing/ -run TestParseLegacy -v`
Expected: FAIL with undefined `ParseLegacyRules` / `LegacyRule` / `MapLegacyOutboundToCode`

- [ ] **Step 3: 创建 `legacy.go`**

```go
package routing

import (
    "strings"
)

// LegacyRule 是一条老格式规则的结构化形式。
type LegacyRule struct {
    Type     string // DOMAIN | DOMAIN-SUFFIX | DOMAIN-KEYWORD | IP-CIDR | IP-CIDR6 | GEOSITE | GEOIP | PROCESS-NAME
    Value    string
    Outbound string // 旧格式的中文组名或 DIRECT/REJECT
}

var legacyRuleTypes = map[string]bool{
    "DOMAIN":         true,
    "DOMAIN-SUFFIX":  true,
    "DOMAIN-KEYWORD": true,
    "IP-CIDR":        true,
    "IP-CIDR6":       true,
    "GEOSITE":        true,
    "GEOIP":          true,
    "PROCESS-NAME":   true,
}

// ParseLegacyRules 解析老的多行规则文本。
// 每行格式：TYPE,VALUE,OUTBOUND；#/// 开头或空行忽略；格式错误跳过不报错。
func ParseLegacyRules(text string) ([]LegacyRule, error) {
    var out []LegacyRule
    for _, raw := range strings.Split(text, "\n") {
        line := strings.TrimSpace(raw)
        if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
            continue
        }
        parts := strings.Split(line, ",")
        if len(parts) < 3 {
            continue
        }
        typ := strings.ToUpper(strings.TrimSpace(parts[0]))
        if !legacyRuleTypes[typ] {
            continue
        }
        out = append(out, LegacyRule{
            Type:     typ,
            Value:    strings.TrimSpace(parts[1]),
            Outbound: strings.TrimSpace(parts[2]),
        })
    }
    return out, nil
}

// legacyOutboundMap 把老的中文组名映射到新的 SystemGroup.Code。
// 与 SystemGroups 的 DisplayName（去 emoji 前缀）对齐。
var legacyOutboundMap = map[string]string{
    "手动切换":   "node_select",
    "自动选择":   "auto_select",
    "全球代理":   "global_proxy",
    "流媒体":    "streaming",
    "Telegram": "telegram",
    "Google":   "google",
    "YouTube":  "youtube",
    "Netflix":  "netflix",
    "Spotify":  "spotify",
    "HBO":      "hbo",
    "Bing":     "bing",
    "OpenAI":   "openai",
    "ClaudeAI": "claude_ai",
    "Disney":   "disney",
    "GitHub":   "github",
    "国内媒体":   "cn_media",
    "本地直连":   "direct",
    "漏网之鱼":   "fallback",
}

// MapLegacyOutboundToCode 把老组名映射到新 code。
// DIRECT/REJECT 原样返回；未识别返回空。
func MapLegacyOutboundToCode(name string) string {
    s := strings.TrimSpace(name)
    if s == "DIRECT" || s == "REJECT" {
        return s
    }
    if code, ok := legacyOutboundMap[s]; ok {
        return code
    }
    return ""
}

// ToCustomRuleFields 将 LegacyRule 转换为 custom_rules 表的字段切片。
// 返回 (siteTags, ipTags, domainSuffix, domainKeyword, ipCIDR)。
// 任一切片长度 ≤ 1（单行规则单字段）。
func (r LegacyRule) ToCustomRuleFields() (site, ip, ds, dk, ic []string) {
    switch r.Type {
    case "DOMAIN":
        // 无精确 DOMAIN 对应字段，降级为 domain_suffix（需要客户端自己判断；等价语义）
        ds = []string{r.Value}
    case "DOMAIN-SUFFIX":
        ds = []string{r.Value}
    case "DOMAIN-KEYWORD":
        dk = []string{r.Value}
    case "IP-CIDR", "IP-CIDR6":
        ic = []string{r.Value}
    case "GEOSITE":
        site = []string{r.Value}
    case "GEOIP":
        ip = []string{r.Value}
    case "PROCESS-NAME":
        // 当前 custom_rules 无 process 字段（spec 未包含），按降级丢弃
    }
    return
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/service/routing/ -run 'TestParseLegacy|TestMapLegacy' -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/service/routing/legacy.go internal/service/routing/legacy_test.go
git commit -m "feat(routing): legacy custom_rules text parser + outbound name mapping"
```

---

## Task 5: Migration 中 seed 系统数据 + 自动导入老文本

**Files:**
- Modify: `internal/database/migrations.go`（在 Task 1 的 4 张表语句之后追加 seed + 导入逻辑）

因为 seed 需要调用 `routing` 包常量，且插入要遵循 FK 顺序，这步把它从纯 SQL 字符串切片改为方法调用。

- [ ] **Step 1: 在 `migrations.go` 末尾（`addColumnIfNotExists` 方法之前）新增 `seedRouting` 方法**

```go
import (
    "encoding/json"
    "strings"

    "proxy-panel/internal/service/routing"
)
```

```go
// seedRouting 幂等 seed 18 系统组 / 18 系统分类 / 3 预设 / URL 前缀默认值，
// 并一次性自动导入老 settings.custom_rules / custom_rules_mode 文本到 custom_rules 表。
func (db *DB) seedRouting() error {
    // 1. 系统出站组（先 seed 组，供分类 FK）
    for _, g := range routing.SystemGroups {
        members, _ := json.Marshal(g.Members)
        if _, err := db.Exec(`INSERT INTO outbound_groups (code, display_name, type, members, kind, sort_order)
            VALUES (?, ?, ?, ?, 'system', ?)
            ON CONFLICT(code) DO UPDATE SET
                display_name=excluded.display_name,
                type=excluded.type,
                kind='system',
                sort_order=excluded.sort_order
                -- members 故意不覆盖：允许用户改成员后 seed 不回滚
            `, g.Code, g.DisplayName, g.Type, string(members), g.SortOrder); err != nil {
            return err
        }
    }

    // 2. 系统规则分类
    for _, c := range routing.SystemCategories {
        siteTags, _ := json.Marshal(c.SiteTags)
        ipTags, _ := json.Marshal(c.IPTags)
        ids, _ := json.Marshal(c.InlineDomainSuffix)
        idk, _ := json.Marshal(c.InlineDomainKeyword)
        iic, _ := json.Marshal(c.InlineIPCIDR)
        enabled := 0
        if c.Enabled {
            enabled = 1
        }
        var groupID *int64
        if c.DefaultGroupCode != "" {
            var id int64
            if err := db.QueryRow(`SELECT id FROM outbound_groups WHERE code = ?`, c.DefaultGroupCode).Scan(&id); err == nil {
                groupID = &id
            }
        }
        // 系统分类：首次 seed 后不覆盖 enabled / default_group_id / sort_order（用户可改）
        if _, err := db.Exec(`INSERT INTO rule_categories
            (code, display_name, kind, site_tags, ip_tags, inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order)
            VALUES (?, ?, 'system', ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(code) DO UPDATE SET
                display_name=excluded.display_name,
                kind='system',
                site_tags=excluded.site_tags,
                ip_tags=excluded.ip_tags,
                inline_domain_suffix=excluded.inline_domain_suffix,
                inline_domain_keyword=excluded.inline_domain_keyword,
                inline_ip_cidr=excluded.inline_ip_cidr,
                protocol=excluded.protocol
                -- default_group_id / enabled / sort_order 不覆盖
            `, c.Code, c.DisplayName, string(siteTags), string(ipTags), string(ids), string(idk), string(iic), c.Protocol, groupID, enabled, c.SortOrder); err != nil {
            return err
        }
    }

    // 3. 预设
    for _, p := range routing.SystemPresets {
        ec, _ := json.Marshal(p.EnabledCategories)
        if _, err := db.Exec(`INSERT INTO rule_presets (code, display_name, enabled_categories)
            VALUES (?, ?, ?)
            ON CONFLICT(code) DO UPDATE SET
                display_name=excluded.display_name,
                enabled_categories=excluded.enabled_categories
            `, p.Code, p.DisplayName, string(ec)); err != nil {
            return err
        }
    }

    // 4. settings 默认 URL 前缀（仅当不存在时插入）
    defaults := map[string]string{
        "routing.site_ruleset_base_url.clash":   routing.DefaultClashSiteBase,
        "routing.ip_ruleset_base_url.clash":     routing.DefaultClashIPBase,
        "routing.site_ruleset_base_url.singbox": routing.DefaultSingboxSiteBase,
        "routing.ip_ruleset_base_url.singbox":   routing.DefaultSingboxIPBase,
        "routing.final_outbound":                routing.DefaultFinalGroup,
        "routing.active_preset":                 "",
    }
    for k, v := range defaults {
        if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
            return err
        }
    }

    // 5. 自动导入老文本（仅执行一次，以 routing.legacy_imported 标记幂等）
    var marked string
    db.QueryRow(`SELECT value FROM settings WHERE key = 'routing.legacy_imported'`).Scan(&marked)
    if marked != "1" {
        if err := db.importLegacyRules(); err != nil {
            return err
        }
        if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('routing.legacy_imported', '1')`); err != nil {
            return err
        }
    }
    return nil
}

// importLegacyRules 读取老 custom_rules 文本，解析后写入 custom_rules 表。
// override 模式则把所有系统分类 enabled 置 0。完成后删除老键。
func (db *DB) importLegacyRules() error {
    var text, mode string
    db.QueryRow(`SELECT value FROM settings WHERE key = 'custom_rules'`).Scan(&text)
    db.QueryRow(`SELECT value FROM settings WHERE key = 'custom_rules_mode'`).Scan(&mode)
    if strings.TrimSpace(text) == "" {
        // 老数据不存在，直接清理 key
        _, _ = db.Exec(`DELETE FROM settings WHERE key IN ('custom_rules', 'custom_rules_mode')`)
        return nil
    }
    rules, err := routing.ParseLegacyRules(text)
    if err != nil {
        return err
    }
    groupIDByCode := map[string]int64{}
    gRows, err := db.Query(`SELECT code, id FROM outbound_groups`)
    if err != nil {
        return err
    }
    for gRows.Next() {
        var code string
        var id int64
        gRows.Scan(&code, &id)
        groupIDByCode[code] = id
    }
    gRows.Close()

    for i, r := range rules {
        code := routing.MapLegacyOutboundToCode(r.Outbound)
        var outboundGroupID *int64
        outboundLiteral := ""
        if code == "DIRECT" || code == "REJECT" {
            outboundLiteral = code
        } else if code != "" {
            id := groupIDByCode[code]
            outboundGroupID = &id
        } else {
            // 未识别：降级为 fallback 组
            id := groupIDByCode["fallback"]
            outboundGroupID = &id
        }
        site, ip, ds, dk, ic := r.ToCustomRuleFields()
        siteJSON, _ := json.Marshal(site)
        ipJSON, _ := json.Marshal(ip)
        dsJSON, _ := json.Marshal(ds)
        dkJSON, _ := json.Marshal(dk)
        icJSON, _ := json.Marshal(ic)
        name := "legacy-" + r.Type + "-" + r.Value
        if len(name) > 64 {
            name = name[:64]
        }
        if _, err := db.Exec(`INSERT INTO custom_rules
            (name, site_tags, ip_tags, domain_suffix, domain_keyword, ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order)
            VALUES (?, ?, ?, ?, ?, ?, '[]', '', '', ?, ?, ?)`,
            name, string(siteJSON), string(ipJSON), string(dsJSON), string(dkJSON), string(icJSON),
            outboundGroupID, outboundLiteral, i); err != nil {
            return err
        }
    }

    if strings.TrimSpace(mode) == "override" {
        if _, err := db.Exec(`UPDATE rule_categories SET enabled = 0 WHERE kind = 'system'`); err != nil {
            return err
        }
    }
    _, err = db.Exec(`DELETE FROM settings WHERE key IN ('custom_rules', 'custom_rules_mode')`)
    return err
}
```

- [ ] **Step 2: 在 `migrate()` 方法末尾（return nil 之前）调用 `seedRouting`**

```go
    if err := db.seedRouting(); err != nil {
        return err
    }
    return nil
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 通过。注意 `database` 包现在依赖 `routing` 包，`routing` 包不得反向依赖 `database`（目前只依赖标准库，OK）。

- [ ] **Step 4: 集成冒烟测试**

```bash
rm -f /tmp/routing-test.db
cat > /tmp/routing-seed.go <<'EOF'
package main
import (
    "database/sql"
    "fmt"
    _ "modernc.org/sqlite"
    "proxy-panel/internal/database"
)
func main() {
    d, err := database.New("/tmp/routing-test.db")
    if err != nil { panic(err) }
    _ = sql.ErrNoRows
    rows, _ := d.Query("SELECT code FROM rule_categories ORDER BY sort_order")
    defer rows.Close()
    var code string
    for rows.Next() { rows.Scan(&code); fmt.Println(code) }
}
EOF
# 若 database 包构造函数签名不同，调整为正确签名
go run /tmp/routing-seed.go
```
Expected: 输出 18 行分类 code（private / location_cn / ad_block ...）

（若构造签名不同，用 `sqlite3 /tmp/routing-test.db 'SELECT count(*) FROM rule_categories'` 替代；期望 18）

- [ ] **Step 5: 提交**

```bash
git add internal/database/migrations.go
git commit -m "feat(routing): seed system categories/groups/presets + auto-import legacy custom_rules"
```

---

## Task 6: `routing/store.go` — DB 读写辅助

**Files:**
- Create: `internal/service/routing/store.go`

- [ ] **Step 1: 创建 `store.go`**

```go
package routing

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
)

// CategoryRow 对应 rule_categories 表一行（业务层结构，JSON 字段已解码）。
type CategoryRow struct {
    ID                  int64
    Code                string
    DisplayName         string
    Kind                string
    SiteTags            []string
    IPTags              []string
    InlineDomainSuffix  []string
    InlineDomainKeyword []string
    InlineIPCIDR        []string
    Protocol            string
    DefaultGroupID      *int64
    Enabled             bool
    SortOrder           int
}

type GroupRow struct {
    ID          int64
    Code        string
    DisplayName string
    Type        string
    Members     []string
    Kind        string
    SortOrder   int
}

type CustomRuleRow struct {
    ID              int64
    Name            string
    SiteTags        []string
    IPTags          []string
    DomainSuffix    []string
    DomainKeyword   []string
    IPCIDR          []string
    SrcIPCIDR       []string
    Protocol        string
    Port            string
    OutboundGroupID *int64
    OutboundLiteral string
    SortOrder       int
}

type PresetRow struct {
    Code              string
    DisplayName       string
    EnabledCategories []string
}

// DB 是 *sql.DB 的最小接口，便于测试。
type DB interface {
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ListCategories(ctx context.Context, db DB) ([]CategoryRow, error) {
    rows, err := db.QueryContext(ctx, `SELECT id, code, display_name, kind, site_tags, ip_tags,
        inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order
        FROM rule_categories ORDER BY sort_order, id`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []CategoryRow
    for rows.Next() {
        var r CategoryRow
        var site, ip, ds, dk, ic string
        var gid sql.NullInt64
        var enabled int
        if err := rows.Scan(&r.ID, &r.Code, &r.DisplayName, &r.Kind, &site, &ip, &ds, &dk, &ic,
            &r.Protocol, &gid, &enabled, &r.SortOrder); err != nil {
            return nil, err
        }
        _ = json.Unmarshal([]byte(site), &r.SiteTags)
        _ = json.Unmarshal([]byte(ip), &r.IPTags)
        _ = json.Unmarshal([]byte(ds), &r.InlineDomainSuffix)
        _ = json.Unmarshal([]byte(dk), &r.InlineDomainKeyword)
        _ = json.Unmarshal([]byte(ic), &r.InlineIPCIDR)
        if gid.Valid {
            v := gid.Int64
            r.DefaultGroupID = &v
        }
        r.Enabled = enabled == 1
        out = append(out, r)
    }
    return out, rows.Err()
}

func ListGroups(ctx context.Context, db DB) ([]GroupRow, error) {
    rows, err := db.QueryContext(ctx, `SELECT id, code, display_name, type, members, kind, sort_order
        FROM outbound_groups ORDER BY sort_order, id`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []GroupRow
    for rows.Next() {
        var g GroupRow
        var members string
        if err := rows.Scan(&g.ID, &g.Code, &g.DisplayName, &g.Type, &members, &g.Kind, &g.SortOrder); err != nil {
            return nil, err
        }
        _ = json.Unmarshal([]byte(members), &g.Members)
        out = append(out, g)
    }
    return out, rows.Err()
}

func ListCustomRules(ctx context.Context, db DB) ([]CustomRuleRow, error) {
    rows, err := db.QueryContext(ctx, `SELECT id, name, site_tags, ip_tags, domain_suffix, domain_keyword,
        ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order
        FROM custom_rules ORDER BY sort_order, id`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []CustomRuleRow
    for rows.Next() {
        var r CustomRuleRow
        var site, ip, ds, dk, ic, sic string
        var gid sql.NullInt64
        if err := rows.Scan(&r.ID, &r.Name, &site, &ip, &ds, &dk, &ic, &sic,
            &r.Protocol, &r.Port, &gid, &r.OutboundLiteral, &r.SortOrder); err != nil {
            return nil, err
        }
        _ = json.Unmarshal([]byte(site), &r.SiteTags)
        _ = json.Unmarshal([]byte(ip), &r.IPTags)
        _ = json.Unmarshal([]byte(ds), &r.DomainSuffix)
        _ = json.Unmarshal([]byte(dk), &r.DomainKeyword)
        _ = json.Unmarshal([]byte(ic), &r.IPCIDR)
        _ = json.Unmarshal([]byte(sic), &r.SrcIPCIDR)
        if gid.Valid {
            v := gid.Int64
            r.OutboundGroupID = &v
        }
        out = append(out, r)
    }
    return out, rows.Err()
}

func GetPreset(ctx context.Context, db DB, code string) (*PresetRow, error) {
    var p PresetRow
    var ec string
    err := db.QueryRowContext(ctx, `SELECT code, display_name, enabled_categories FROM rule_presets WHERE code = ?`, code).
        Scan(&p.Code, &p.DisplayName, &ec)
    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    _ = json.Unmarshal([]byte(ec), &p.EnabledCategories)
    return &p, nil
}

// GetRoutingSetting 读 settings 表中 routing.* 标量，不存在返回默认值。
func GetRoutingSetting(ctx context.Context, db DB, key, fallback string) string {
    var v string
    err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
    if err != nil || v == "" {
        return fallback
    }
    return v
}

// ResolveGroupCode 通过 id 找 code（builder 在处理 custom_rules 时需要）。
func ResolveGroupCode(groups []GroupRow, id *int64) (string, error) {
    if id == nil {
        return "", nil
    }
    for _, g := range groups {
        if g.ID == *id {
            return g.Code, nil
        }
    }
    return "", fmt.Errorf("group id %d not found", *id)
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/service/routing/...`
Expected: 通过。

- [ ] **Step 3: 提交**

```bash
git add internal/service/routing/store.go
git commit -m "feat(routing): DB read helpers (ListCategories/Groups/CustomRules/GetPreset)"
```

---

## Task 7: `routing/builder.go` — `BuildPlan` 主入口（TDD）

**Files:**
- Create: `internal/service/routing/builder.go`
- Create: `internal/service/routing/builder_test.go`

- [ ] **Step 1: 先写测试 `builder_test.go`（使用 in-memory SQLite + seed）**

```go
package routing_test

import (
    "context"
    "database/sql"
    "testing"

    _ "modernc.org/sqlite"

    "proxy-panel/internal/service/routing"
)

// setupTestDB 创建 in-memory DB 并 seed 4 张表（复用 presets.go 常量，不跑 migration）。
// 仅 seed 最小集：1 个 group + 1 个 category + 1 个 preset。
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatal(err)
    }
    stmts := []string{
        `CREATE TABLE outbound_groups (id INTEGER PRIMARY KEY, code TEXT UNIQUE, display_name TEXT, type TEXT, members TEXT, kind TEXT, sort_order INT)`,
        `CREATE TABLE rule_categories (id INTEGER PRIMARY KEY, code TEXT UNIQUE, display_name TEXT, kind TEXT,
            site_tags TEXT, ip_tags TEXT, inline_domain_suffix TEXT, inline_domain_keyword TEXT, inline_ip_cidr TEXT,
            protocol TEXT, default_group_id INT, enabled INT, sort_order INT)`,
        `CREATE TABLE custom_rules (id INTEGER PRIMARY KEY, name TEXT, site_tags TEXT, ip_tags TEXT,
            domain_suffix TEXT, domain_keyword TEXT, ip_cidr TEXT, src_ip_cidr TEXT, protocol TEXT, port TEXT,
            outbound_group_id INT, outbound_literal TEXT, sort_order INT)`,
        `CREATE TABLE rule_presets (code TEXT PRIMARY KEY, display_name TEXT, enabled_categories TEXT)`,
        `CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
        `INSERT INTO outbound_groups VALUES (1, 'node_select', 'Node', 'selector', '["<ALL>","DIRECT"]', 'system', 10)`,
        `INSERT INTO outbound_groups VALUES (2, 'direct', 'Direct', 'selector', '["DIRECT","node_select"]', 'system', 170)`,
        `INSERT INTO rule_categories VALUES (1, 'location_cn', 'CN', 'system', '["cn"]', '["cn"]', '[]', '[]', '[]', '', 2, 1, 10)`,
        `INSERT INTO rule_categories VALUES (2, 'google',      'G',  'system', '["google"]', '[]', '[]', '[]', '[]', '', 1, 0, 20)`,
        `INSERT INTO rule_presets VALUES ('balanced', 'B', '["location_cn","google"]')`,
        `INSERT INTO settings VALUES ('routing.final_outbound', 'node_select')`,
        `INSERT INTO settings VALUES ('routing.site_ruleset_base_url.clash',   'https://ex.com/geosite/')`,
        `INSERT INTO settings VALUES ('routing.ip_ruleset_base_url.clash',     'https://ex.com/geoip/')`,
        `INSERT INTO settings VALUES ('routing.site_ruleset_base_url.singbox', 'https://sb.com/geosite/')`,
        `INSERT INTO settings VALUES ('routing.ip_ruleset_base_url.singbox',   'https://sb.com/geoip/')`,
    }
    for _, s := range stmts {
        if _, err := db.Exec(s); err != nil {
            t.Fatalf("seed: %v (%s)", err, s)
        }
    }
    return db
}

func TestBuildPlan_EnabledCategoriesOnly(t *testing.T) {
    db := setupTestDB(t)
    plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{})
    if err != nil {
        t.Fatal(err)
    }
    // 仅 location_cn enabled（google 未启用）
    if len(plan.Rules) != 1 {
        t.Fatalf("want 1 rule, got %d", len(plan.Rules))
    }
    if plan.Rules[0].Outbound != "direct" {
        t.Errorf("want outbound=direct, got %s", plan.Rules[0].Outbound)
    }
    if got := plan.Rules[0].SiteTags; len(got) != 1 || got[0] != "cn" {
        t.Errorf("site tags: %+v", got)
    }
    if plan.Final != "node_select" {
        t.Errorf("final=%s", plan.Final)
    }
    // providers 应含 cn 的 site + ip
    if _, ok := plan.Providers.Site["cn"]; !ok {
        t.Error("missing site provider cn")
    }
    if _, ok := plan.Providers.IP["cn"]; !ok {
        t.Error("missing ip provider cn")
    }
}

func TestBuildPlan_PresetOverride(t *testing.T) {
    db := setupTestDB(t)
    plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{PresetOverride: "balanced"})
    if err != nil {
        t.Fatal(err)
    }
    // preset 启用 location_cn + google
    if len(plan.Rules) != 2 {
        t.Fatalf("want 2 rules, got %d", len(plan.Rules))
    }
}

func TestBuildPlan_CustomRulesFirst(t *testing.T) {
    db := setupTestDB(t)
    _, _ = db.Exec(`INSERT INTO custom_rules VALUES
        (1, 'test', '[]', '[]', '["example.com"]', '[]', '[]', '[]', '', '', NULL, 'REJECT', 0)`)
    plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{})
    if err != nil {
        t.Fatal(err)
    }
    if len(plan.Rules) < 1 || plan.Rules[0].Outbound != "REJECT" {
        t.Fatalf("custom rule not first: %+v", plan.Rules)
    }
    if plan.Rules[0].DomainSuffix[0] != "example.com" {
        t.Error("custom rule fields lost")
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/routing/ -run TestBuildPlan -v`
Expected: FAIL with undefined `BuildPlan`

- [ ] **Step 3: 实现 `builder.go`**

```go
package routing

import (
    "context"
    "fmt"
)

// BuildPlan 从 DB 读规范化表 + 应用预设覆盖 → 输出格式无关 Plan。
func BuildPlan(ctx context.Context, db DB, opts BuildOptions) (*Plan, error) {
    groups, err := ListGroups(ctx, db)
    if err != nil {
        return nil, fmt.Errorf("list groups: %w", err)
    }
    cats, err := ListCategories(ctx, db)
    if err != nil {
        return nil, fmt.Errorf("list categories: %w", err)
    }
    customs, err := ListCustomRules(ctx, db)
    if err != nil {
        return nil, fmt.Errorf("list custom rules: %w", err)
    }

    // 应用预设覆盖：若指定 preset，则仅其 enabled_categories 中的 code 被视为 enabled。
    enabledOverride := map[string]bool{}
    usingOverride := false
    if opts.PresetOverride != "" {
        preset, err := GetPreset(ctx, db, opts.PresetOverride)
        if err != nil {
            return nil, fmt.Errorf("get preset %q: %w", opts.PresetOverride, err)
        }
        if preset != nil {
            usingOverride = true
            for _, c := range preset.EnabledCategories {
                enabledOverride[c] = true
            }
        }
    }

    plan := &Plan{
        Providers: Providers{
            Site: map[string]ProviderURLs{},
            IP:   map[string]ProviderURLs{},
        },
    }

    // 1. 出站组 IR（kind 字段丢弃，translator 不关心）
    for _, g := range groups {
        plan.Groups = append(plan.Groups, OutboundGroup{
            Code: g.Code, DisplayName: g.DisplayName, Type: g.Type, Members: g.Members,
        })
    }

    // 2. 自定义规则（优先级最高）
    for _, cr := range customs {
        outbound := cr.OutboundLiteral
        if outbound == "" {
            code, err := ResolveGroupCode(groups, cr.OutboundGroupID)
            if err != nil {
                return nil, fmt.Errorf("custom rule %d: %w", cr.ID, err)
            }
            outbound = code
        }
        if outbound == "" {
            continue // 无有效出站则跳过
        }
        plan.Rules = append(plan.Rules, Rule{
            SiteTags: cr.SiteTags, IPTags: cr.IPTags,
            DomainSuffix: cr.DomainSuffix, DomainKeyword: cr.DomainKeyword,
            IPCIDR: cr.IPCIDR, SrcIPCIDR: cr.SrcIPCIDR,
            Protocol: splitCSV(cr.Protocol), Port: splitCSV(cr.Port),
            Outbound: outbound,
        })
        collectProviders(plan, cr.SiteTags, cr.IPTags)
    }

    // 3. 启用的分类规则
    for _, c := range cats {
        enabled := c.Enabled
        if usingOverride {
            enabled = enabledOverride[c.Code]
        }
        if !enabled {
            continue
        }
        outboundCode, err := ResolveGroupCode(groups, c.DefaultGroupID)
        if err != nil {
            return nil, fmt.Errorf("category %s: %w", c.Code, err)
        }
        if outboundCode == "" {
            // 分类无默认出站，跳过（避免产出无目的规则）
            continue
        }
        plan.Rules = append(plan.Rules, Rule{
            SiteTags: c.SiteTags, IPTags: c.IPTags,
            DomainSuffix: c.InlineDomainSuffix, DomainKeyword: c.InlineDomainKeyword,
            IPCIDR: c.InlineIPCIDR,
            Protocol: splitCSV(c.Protocol),
            Outbound: outboundCode,
        })
        collectProviders(plan, c.SiteTags, c.IPTags)
    }

    // 4. Providers URL 具化
    clashSite := GetRoutingSetting(ctx, db, "routing.site_ruleset_base_url.clash", DefaultClashSiteBase)
    clashIP := GetRoutingSetting(ctx, db, "routing.ip_ruleset_base_url.clash", DefaultClashIPBase)
    sbSite := GetRoutingSetting(ctx, db, "routing.site_ruleset_base_url.singbox", DefaultSingboxSiteBase)
    sbIP := GetRoutingSetting(ctx, db, "routing.ip_ruleset_base_url.singbox", DefaultSingboxIPBase)
    for tag := range plan.Providers.Site {
        plan.Providers.Site[tag] = ProviderURLs{
            Clash:   clashSite + tag + ".mrs",
            Singbox: sbSite + tag + ".srs",
        }
    }
    for tag := range plan.Providers.IP {
        plan.Providers.IP[tag] = ProviderURLs{
            Clash:   clashIP + tag + ".mrs",
            Singbox: sbIP + tag + ".srs",
        }
    }

    // 5. Final
    plan.Final = GetRoutingSetting(ctx, db, "routing.final_outbound", DefaultFinalGroup)

    return plan, nil
}

func collectProviders(plan *Plan, siteTags, ipTags []string) {
    for _, t := range siteTags {
        if _, ok := plan.Providers.Site[t]; !ok {
            plan.Providers.Site[t] = ProviderURLs{}
        }
    }
    for _, t := range ipTags {
        if _, ok := plan.Providers.IP[t]; !ok {
            plan.Providers.IP[t] = ProviderURLs{}
        }
    }
}

func splitCSV(s string) []string {
    if s == "" {
        return nil
    }
    var out []string
    start := 0
    for i := 0; i <= len(s); i++ {
        if i == len(s) || s[i] == ',' {
            v := trimSpace(s[start:i])
            if v != "" {
                out = append(out, v)
            }
            start = i + 1
        }
    }
    return out
}

func trimSpace(s string) string {
    for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
        s = s[1:]
    }
    for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
        s = s[:len(s)-1]
    }
    return s
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/service/routing/ -run TestBuildPlan -v`
Expected: PASS（3 个测试全过）

- [ ] **Step 5: 提交**

```bash
git add internal/service/routing/builder.go internal/service/routing/builder_test.go
git commit -m "feat(routing): BuildPlan with preset override + custom rules prepend"
```

---

## Task 8: Clash Translator 重写（保留 `clash.go`，消费 Plan）

**Files:**
- Modify: `internal/service/subscription/clash.go`

**当前 clash.go（497 行）**：前半是节点渲染（proxies YAML），后半（约 169-332 行）是硬编码 rule-provider + rules。节点渲染部分**保留不动**。

- [ ] **Step 1: 阅读 `internal/service/subscription/clash.go`，标注要替换的区段**

关键区段（行号参考当前 HEAD）：
- 169-276：`ruleProviders` 硬编码 + proxy-groups 硬编码
- 294-332：`generateRules()` 自定义/默认规则拼接
- 修改入口：`Generator.Generate(nodes, user, baseURL)` 内部调用生成规则的地方

- [ ] **Step 2: 在 `subscription.Generator` 接口/结构体上新增可选 DB 字段**

搜索 `type.*Generator` 定义文件，确定注入方式。若当前 `Generator` 是一个 interface 实例，需要改构造函数：

- 在 `subscription.ClashGenerator` 结构体加 `db DB`（见 Step 3 的接口定义）
- `GetGenerator(format)` 改为 `GetGenerator(format, db DB)` — 但这会扩散到五端；**简化方案**：在 handler 端先 `routing.BuildPlan` 再通过一个新方法 `GenerateWithPlan(plan, nodes, user, baseURL)` 传入。

```go
// 在 subscription 包新增共享接口：
type RoutingAwareGenerator interface {
    Generator
    GenerateWithPlan(plan *routing.Plan, nodes []*db.Node, user *db.User, baseURL string) (string, string, error)
}
```

- [ ] **Step 3: 重写 Clash 规则/组渲染为消费 `*routing.Plan`**

替换 169-332 行区段为：

```go
// renderClashRoutingFromPlan 根据 Plan 产出 clash 的 rule-providers / proxy-groups / rules 三个 YAML 块。
func renderClashRoutingFromPlan(plan *routing.Plan, allNodeNames []string) (providers map[string]any, groups []map[string]any, rules []string) {
    providers = map[string]any{}
    for tag, urls := range plan.Providers.Site {
        providers[tag] = map[string]any{
            "type": "http", "behavior": "domain", "format": "mrs",
            "url": urls.Clash, "interval": 86400,
            "path": "./rule_provider/site/" + tag + ".mrs",
        }
    }
    for tag, urls := range plan.Providers.IP {
        providers[tag+"-ip"] = map[string]any{
            "type": "http", "behavior": "ipcidr", "format": "mrs",
            "url": urls.Clash, "interval": 86400,
            "path": "./rule_provider/ip/" + tag + ".mrs",
        }
    }

    // proxy-groups：<ALL> 宏展开为全节点
    for _, g := range plan.Groups {
        members := expandMacros(g.Members, allNodeNames)
        group := map[string]any{
            "name": g.DisplayName, "type": g.Type, "proxies": members,
        }
        if g.Type == "urltest" {
            group["url"] = "http://www.gstatic.com/generate_204"
            group["interval"] = 300
        }
        groups = append(groups, group)
    }

    // rules：按 Plan.Rules 顺序输出
    for _, r := range plan.Rules {
        out := clashOutboundName(r.Outbound, plan.Groups)
        for _, t := range r.SiteTags {
            rules = append(rules, fmt.Sprintf("RULE-SET,%s,%s", t, out))
        }
        for _, t := range r.IPTags {
            rules = append(rules, fmt.Sprintf("RULE-SET,%s-ip,%s", t, out))
        }
        for _, v := range r.DomainSuffix {
            rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", v, out))
        }
        for _, v := range r.DomainKeyword {
            rules = append(rules, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", v, out))
        }
        for _, v := range r.IPCIDR {
            rules = append(rules, fmt.Sprintf("IP-CIDR,%s,%s", v, out))
        }
        for _, v := range r.SrcIPCIDR {
            rules = append(rules, fmt.Sprintf("SRC-IP-CIDR,%s,%s", v, out))
        }
    }
    // 兜底
    rules = append(rules, fmt.Sprintf("MATCH,%s", clashOutboundName(plan.Final, plan.Groups)))
    return
}

// clashOutboundName 把 group code / 字面量 转为 Clash 使用的 proxy-group 名或 DIRECT/REJECT。
func clashOutboundName(codeOrLiteral string, groups []routing.OutboundGroup) string {
    if routing.IsLiteralOutbound(codeOrLiteral) {
        return codeOrLiteral
    }
    for _, g := range groups {
        if g.Code == codeOrLiteral {
            return g.DisplayName
        }
    }
    return codeOrLiteral // 兜底
}

// expandMacros 展开 <ALL>（全节点）、引用的 group code → DisplayName。
func expandMacros(members, allNodeNames []string) []string {
    var out []string
    for _, m := range members {
        switch {
        case m == "<ALL>":
            out = append(out, allNodeNames...)
        case routing.IsLiteralOutbound(m):
            out = append(out, m)
        default:
            // 假设是 group code；translator 上下文里 group displayName 已经可通过 name 映射
            // 为简化，这里直接透传 code，由顶层 Generate 做二次映射（在填 proxy-groups 前）
            out = append(out, m)
        }
    }
    return out
}
```

然后在 `ClashGenerator.GenerateWithPlan` 里：

```go
func (g *ClashGenerator) GenerateWithPlan(plan *routing.Plan, nodes []*db.Node, user *db.User, baseURL string) (string, string, error) {
    // ...节点渲染保持原状，得到 proxies 列表和 allNodeNames
    providers, groupsYAML, rules := renderClashRoutingFromPlan(plan, allNodeNames)

    // 二次映射：把 groupsYAML 中 proxies 里的 group code 换成 DisplayName
    codeToName := map[string]string{}
    for _, g := range plan.Groups {
        codeToName[g.Code] = g.DisplayName
    }
    for _, gm := range groupsYAML {
        if prox, ok := gm["proxies"].([]string); ok {
            for i, p := range prox {
                if n, ok := codeToName[p]; ok {
                    prox[i] = n
                }
            }
        }
    }

    // 组装最终 YAML：proxies / proxy-groups: groupsYAML / rule-providers: providers / rules: rules
    // ...（保留原 YAML 序列化逻辑）
}
```

- [ ] **Step 4: 删除 `clash.go` 中旧的硬编码 rule-provider/proxy-groups/defaultRules 代码段**

- [ ] **Step 5: 保留旧 `Generate` 方法为 stub（过渡期，直接返回错误或空），避免其他调用方崩溃**

```go
func (g *ClashGenerator) Generate(nodes []*db.Node, user *db.User, baseURL string) (string, string, error) {
    return "", "", fmt.Errorf("clash generator requires routing plan; use GenerateWithPlan")
}
```

- [ ] **Step 6: 编译**

Run: `go build ./...`
Expected: 通过（handler 调用 `Generate` 处会运行时失败，但 Task 12 会接入 `GenerateWithPlan`）

- [ ] **Step 7: 提交**

```bash
git add internal/service/subscription/clash.go
git commit -m "refactor(subscription/clash): consume routing.Plan instead of hardcoded rules"
```

---

## Task 9: Sing-box Translator

**Files:**
- Modify: `internal/service/subscription/singbox.go`

- [ ] **Step 1: 在 `singbox.go` 末尾新增 `renderSingboxRoutingFromPlan`**

```go
func renderSingboxRoutingFromPlan(plan *routing.Plan, allNodeTags []string) (ruleSets []map[string]any, outbounds []map[string]any, rules []map[string]any, final string) {
    for tag, urls := range plan.Providers.Site {
        ruleSets = append(ruleSets, map[string]any{
            "tag": tag, "type": "remote", "format": "binary",
            "url": urls.Singbox, "download_detour": "direct",
        })
    }
    for tag, urls := range plan.Providers.IP {
        ruleSets = append(ruleSets, map[string]any{
            "tag": tag + "-ip", "type": "remote", "format": "binary",
            "url": urls.Singbox, "download_detour": "direct",
        })
    }

    codeToTag := map[string]string{}
    for _, g := range plan.Groups {
        tag := g.DisplayName
        codeToTag[g.Code] = tag
        members := []string{}
        for _, m := range g.Members {
            switch {
            case m == "<ALL>":
                members = append(members, allNodeTags...)
            case m == "DIRECT" || m == "REJECT":
                members = append(members, toSingboxLiteral(m))
            default:
                members = append(members, m) // code → 稍后映射
            }
        }
        outbounds = append(outbounds, map[string]any{
            "tag": tag, "type": g.Type, "outbounds": members,
        })
    }
    // 二次映射 outbounds 里的 code → tag
    for _, o := range outbounds {
        if arr, ok := o["outbounds"].([]string); ok {
            for i, m := range arr {
                if t, ok := codeToTag[m]; ok {
                    arr[i] = t
                }
            }
        }
    }

    // rules
    for _, r := range plan.Rules {
        out := singboxOutboundName(r.Outbound, codeToTag)
        rule := map[string]any{"outbound": out}
        if len(r.SiteTags) > 0 {
            rule["rule_set"] = r.SiteTags
        }
        if len(r.IPTags) > 0 {
            ipRefs := []string{}
            for _, t := range r.IPTags {
                ipRefs = append(ipRefs, t+"-ip")
            }
            existing, _ := rule["rule_set"].([]string)
            rule["rule_set"] = append(existing, ipRefs...)
        }
        if len(r.DomainSuffix) > 0 { rule["domain_suffix"] = r.DomainSuffix }
        if len(r.DomainKeyword) > 0 { rule["domain_keyword"] = r.DomainKeyword }
        if len(r.IPCIDR) > 0 { rule["ip_cidr"] = r.IPCIDR }
        if len(r.SrcIPCIDR) > 0 { rule["source_ip_cidr"] = r.SrcIPCIDR }
        if len(r.Protocol) > 0 { rule["protocol"] = r.Protocol }
        rules = append(rules, rule)
    }

    final = singboxOutboundName(plan.Final, codeToTag)
    return
}

func toSingboxLiteral(lit string) string {
    if lit == "DIRECT" { return "direct" }
    if lit == "REJECT" { return "block" }
    return lit
}

func singboxOutboundName(codeOrLit string, codeToTag map[string]string) string {
    if codeOrLit == "DIRECT" || codeOrLit == "REJECT" {
        return toSingboxLiteral(codeOrLit)
    }
    if t, ok := codeToTag[codeOrLit]; ok {
        return t
    }
    return codeOrLit
}
```

- [ ] **Step 2: 新增 `SingboxGenerator.GenerateWithPlan`，内部把 ruleSets/outbounds/rules/final 装到最终 JSON 结构中**

原 `Generate` 内生成 outbounds 节点数组的逻辑保留；在其输出 JSON 中：
- 合并 plan 出站组 outbounds 到节点 outbounds 列表
- `route.rule_set = ruleSets`
- `route.rules = rules`
- `route.final = final`

- [ ] **Step 3: 旧 `Generate` 改为调用 stub 错误**

```go
func (g *SingboxGenerator) Generate(nodes []*db.Node, user *db.User, baseURL string) (string, string, error) {
    return "", "", fmt.Errorf("singbox generator requires routing plan; use GenerateWithPlan")
}
```

- [ ] **Step 4: 编译**

Run: `go build ./...`
Expected: 通过。

- [ ] **Step 5: 提交**

```bash
git add internal/service/subscription/singbox.go
git commit -m "refactor(subscription/singbox): consume routing.Plan; route.rule_set/rules/final from IR"
```

---

## Task 10: Surge Translator

**Files:**
- Modify: `internal/service/subscription/surge.go`

Surge 无 rule-provider 概念。site_tags / ip_tags 需用 `RULE-SET` 指向远端 `.list` URL，若无则降级为 `GEOSITE,<tag>` / `GEOIP,<tag>`。

- [ ] **Step 1: 追加 `renderSurgeRoutingFromPlan`**

```go
// Surge 不支持 rule-provider，直接把 site_tags 渲染为 RULE-SET URL（或降级 GEOSITE）。
func renderSurgeRoutingFromPlan(plan *routing.Plan) (proxyGroups []string, rules []string) {
    // [Proxy Group] 块：name = displayName
    for _, g := range plan.Groups {
        members := []string{}
        for _, m := range g.Members {
            if m == "<ALL>" {
                members = append(members, "<ALL>") // Surge 没等价宏，translator 层展开
                continue
            }
            if m == "DIRECT" || m == "REJECT" {
                members = append(members, m)
                continue
            }
            // group code → displayName
            for _, g2 := range plan.Groups {
                if g2.Code == m {
                    members = append(members, g2.DisplayName)
                    break
                }
            }
        }
        proxyGroups = append(proxyGroups,
            fmt.Sprintf("%s = %s, %s", g.DisplayName, g.Type, strings.Join(members, ", ")))
    }

    // [Rule]
    for _, r := range plan.Rules {
        out := surgeOutbound(r.Outbound, plan.Groups)
        for _, t := range r.SiteTags {
            // Surge 识别 GEOSITE 需要 mitm/脚本支持；保守降级：用 DOMAIN-SET 指向 .list
            rules = append(rules, fmt.Sprintf("DOMAIN-SET,%ssite/%s.list,%s", "https://ruleset.example.com/", t, out))
        }
        for _, t := range r.IPTags {
            rules = append(rules, fmt.Sprintf("GEOIP,%s,%s", t, out))
        }
        for _, v := range r.DomainSuffix {
            rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", v, out))
        }
        for _, v := range r.DomainKeyword {
            rules = append(rules, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", v, out))
        }
        for _, v := range r.IPCIDR {
            rules = append(rules, fmt.Sprintf("IP-CIDR,%s,%s", v, out))
        }
    }
    rules = append(rules, fmt.Sprintf("FINAL,%s", surgeOutbound(plan.Final, plan.Groups)))
    return
}

func surgeOutbound(codeOrLit string, groups []routing.OutboundGroup) string {
    if codeOrLit == "DIRECT" || codeOrLit == "REJECT" {
        return codeOrLit
    }
    for _, g := range groups {
        if g.Code == codeOrLit {
            return g.DisplayName
        }
    }
    return codeOrLit
}
```

**注**：上述 `DOMAIN-SET` URL 指向 `https://ruleset.example.com/site/<tag>.list` 是**占位**。真实实现时需在 Task 3 的 settings 键中新增一条 `routing.surge_site_ruleset_base_url`（默认空字符串 = 降级为 GEOSITE），并在 translator 里判空决定用 URL 还是 `GEOSITE,<tag>`。**在实现本任务时同步添加该 settings 键和 translator 分支逻辑**，代码：

```go
// 在 builder.go collectProviders 之后读取：
surgeSiteBase := GetRoutingSetting(ctx, db, "routing.surge_site_ruleset_base_url", "")
// Plan 增加 SurgeSiteBase 字段，translator 判空后决定渲染方式。
```

- [ ] **Step 2: 修改 `Plan` 增加 `SurgeSiteBase string`（plan.go）+ builder 写入 + translator 使用**

```go
// plan.go
type Plan struct {
    // ... existing fields
    SurgeSiteBase string
}

// builder.go BuildPlan 末尾：
plan.SurgeSiteBase = GetRoutingSetting(ctx, db, "routing.surge_site_ruleset_base_url", "")
```

```go
// surge.go renderSurgeRoutingFromPlan 修正 site_tags 分支：
for _, t := range r.SiteTags {
    if plan.SurgeSiteBase != "" {
        rules = append(rules, fmt.Sprintf("DOMAIN-SET,%s%s.list,%s", plan.SurgeSiteBase, t, out))
    } else {
        rules = append(rules, fmt.Sprintf("GEOSITE,%s,%s", t, out))
    }
}
```

- [ ] **Step 3: `SurgeGenerator.GenerateWithPlan` 接入；旧 `Generate` 改 stub 错误**

- [ ] **Step 4: 编译**

Run: `go build ./...`
Expected: 通过。

- [ ] **Step 5: 提交**

```bash
git add internal/service/subscription/surge.go internal/service/routing/plan.go internal/service/routing/builder.go
git commit -m "refactor(subscription/surge): consume routing.Plan with GEOSITE fallback"
```

---

## Task 11: V2Ray + Shadowrocket Translator

**Files:**
- Modify: `internal/service/subscription/v2ray.go`
- Modify: `internal/service/subscription/shadowrocket.go`

V2Ray 规则引擎较弱，只保留 `geosite:<tag>` / `geoip:<tag>` / domain / ip 映射；不支持的字段 skip + `log.Warn`。

- [ ] **Step 1: 在 v2ray.go 中添加 `renderV2RayRoutingFromPlan` 返回 V2Ray 配置的 `routing` 节**

```go
func renderV2RayRoutingFromPlan(plan *routing.Plan) map[string]any {
    rules := []map[string]any{}
    for _, r := range plan.Rules {
        out := v2rayOutbound(r.Outbound, plan.Groups)
        rule := map[string]any{
            "type":        "field",
            "outboundTag": out,
        }
        if len(r.SiteTags) > 0 {
            domains := []string{}
            for _, t := range r.SiteTags { domains = append(domains, "geosite:"+t) }
            rule["domain"] = domains
        }
        if len(r.DomainSuffix) > 0 {
            existing, _ := rule["domain"].([]string)
            for _, v := range r.DomainSuffix { existing = append(existing, "domain:"+v) }
            rule["domain"] = existing
        }
        if len(r.DomainKeyword) > 0 {
            existing, _ := rule["domain"].([]string)
            existing = append(existing, r.DomainKeyword...)
            rule["domain"] = existing
        }
        ips := []string{}
        for _, t := range r.IPTags { ips = append(ips, "geoip:"+t) }
        ips = append(ips, r.IPCIDR...)
        if len(ips) > 0 { rule["ip"] = ips }
        if len(r.SrcIPCIDR) > 0 {
            log.Printf("[routing/v2ray] 忽略 src_ip_cidr (不支持): %v", r.SrcIPCIDR)
        }
        rules = append(rules, rule)
    }
    // V2Ray 没有显式 Final 字段；依靠规则末尾 catch-all
    rules = append(rules, map[string]any{
        "type": "field", "port": "0-65535", "outboundTag": v2rayOutbound(plan.Final, plan.Groups),
    })
    return map[string]any{"domainStrategy": "IPIfNonMatch", "rules": rules}
}

func v2rayOutbound(codeOrLit string, groups []routing.OutboundGroup) string {
    if codeOrLit == "DIRECT" { return "direct" }
    if codeOrLit == "REJECT" { return "block" }
    for _, g := range groups {
        if g.Code == codeOrLit { return g.DisplayName }
    }
    return codeOrLit
}
```

- [ ] **Step 2: `V2RayGenerator.GenerateWithPlan` 接入；旧 `Generate` → stub 错误**

- [ ] **Step 3: Shadowrocket (`shadowrocket.go`) 实现 `renderShadowrocketRoutingFromPlan`**

Shadowrocket 使用 Clash 风格的规则行（`DOMAIN-SUFFIX,...,xxx`）但不支持 rule-provider；复用 `Plan.SurgeSiteBase`（空则降级为 `GEOSITE`）。

```go
func renderShadowrocketRoutingFromPlan(plan *routing.Plan) (proxyGroups []string, rules []string) {
    codeToName := map[string]string{}
    for _, g := range plan.Groups { codeToName[g.Code] = g.DisplayName }

    for _, g := range plan.Groups {
        members := []string{}
        for _, m := range g.Members {
            switch {
            case m == "<ALL>":
                members = append(members, "<ALL>") // 由顶层 Generate 展开
            case m == "DIRECT" || m == "REJECT":
                members = append(members, m)
            default:
                if n, ok := codeToName[m]; ok { members = append(members, n) } else { members = append(members, m) }
            }
        }
        proxyGroups = append(proxyGroups,
            fmt.Sprintf("%s = %s, %s", g.DisplayName, g.Type, strings.Join(members, ", ")))
    }

    for _, r := range plan.Rules {
        out := shadowrocketOutbound(r.Outbound, codeToName)
        for _, t := range r.SiteTags {
            if plan.SurgeSiteBase != "" {
                rules = append(rules, fmt.Sprintf("RULE-SET,%s%s.list,%s", plan.SurgeSiteBase, t, out))
            } else {
                rules = append(rules, fmt.Sprintf("GEOSITE,%s,%s", t, out))
            }
        }
        for _, t := range r.IPTags {
            rules = append(rules, fmt.Sprintf("GEOIP,%s,%s", t, out))
        }
        for _, v := range r.DomainSuffix {
            rules = append(rules, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", v, out))
        }
        for _, v := range r.DomainKeyword {
            rules = append(rules, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", v, out))
        }
        for _, v := range r.IPCIDR {
            rules = append(rules, fmt.Sprintf("IP-CIDR,%s,%s", v, out))
        }
    }
    rules = append(rules, fmt.Sprintf("FINAL,%s", shadowrocketOutbound(plan.Final, codeToName)))
    return
}

func shadowrocketOutbound(codeOrLit string, codeToName map[string]string) string {
    if codeOrLit == "DIRECT" || codeOrLit == "REJECT" { return codeOrLit }
    if n, ok := codeToName[codeOrLit]; ok { return n }
    return codeOrLit
}
```

- [ ] **Step 4: 编译**

Run: `go build ./...`
Expected: 通过。

- [ ] **Step 5: 提交**

```bash
git add internal/service/subscription/v2ray.go internal/service/subscription/shadowrocket.go
git commit -m "refactor(subscription/v2ray,shadowrocket): consume routing.Plan with graceful degradation"
```

---

## Task 12: 订阅 handler 接入 `BuildPlan` + `?preset=` 参数

**Files:**
- Modify: `internal/handler/subscription.go:99-114`

- [ ] **Step 1: 替换 subscription.go 里 100-111 行的 customRules 读取 + `SetCustomRules` 调用段**

```go
    // 分流规划 IR
    preset := c.Query("preset")
    plan, err := routing.BuildPlan(c.Request.Context(), h.db, routing.BuildOptions{
        PresetOverride: preset,
        ClientFormat:   format,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "构建分流规划失败: " + err.Error()})
        return
    }

    gen := subscription.GetGenerator(format)
    ra, ok := gen.(subscription.RoutingAwareGenerator)
    if !ok {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "生成器不支持分流规划"})
        return
    }
    content, contentType, err := ra.GenerateWithPlan(plan, nodes, user, baseURL)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败"})
        return
    }
```

- [ ] **Step 2: 在 `subscription.go` 顶部 import 新增**

```go
    "proxy-panel/internal/service/routing"
```

- [ ] **Step 3: 删除旧的 `customRulesStr` / `customRulesMode` 变量与 SQL 读取代码**

- [ ] **Step 4: 编译 + 运行冒烟**

Run: `go build ./... && go run ./cmd/server -c config.example.yaml &`
再手动：`curl -s 'http://localhost:PORT/api/sub/t/<token>?format=clash' | head -20`
Expected: 返回有效 Clash YAML（包含 rule-providers / proxy-groups / rules 节）。再加 `&preset=minimal` 观察 rules 数量减少。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/subscription.go
git commit -m "feat(subscription): use routing.BuildPlan + ?preset= query param"
```

---

## Task 13: `routing` admin store — 写入 helpers（CRUD service 层）

**Files:**
- Create: `internal/service/routing/admin.go`
- Create: `internal/service/routing/admin_test.go`

把 CRUD 操作放到 service 层，便于 handler 复用与单测。

- [ ] **Step 1: 创建 `admin.go`**

```go
package routing

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
)

var (
    ErrNotFound         = errors.New("not found")
    ErrSystemImmutable  = errors.New("system resource is immutable")
    ErrInvalidOutbound  = errors.New("custom rule must have exactly one outbound")
    ErrGroupReferenced  = errors.New("group still referenced by categories or rules")
)

// FullDB 是有写能力的 DB。
type FullDB interface {
    DB
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ---- Category CRUD ----

type CategoryInput struct {
    Code                string
    DisplayName         string
    SiteTags            []string
    IPTags              []string
    InlineDomainSuffix  []string
    InlineDomainKeyword []string
    InlineIPCIDR        []string
    Protocol            string
    DefaultGroupID      *int64
    Enabled             bool
    SortOrder           int
}

func CreateCategory(ctx context.Context, db FullDB, in CategoryInput) (int64, error) {
    site, _ := json.Marshal(in.SiteTags)
    ip, _ := json.Marshal(in.IPTags)
    ds, _ := json.Marshal(in.InlineDomainSuffix)
    dk, _ := json.Marshal(in.InlineDomainKeyword)
    ic, _ := json.Marshal(in.InlineIPCIDR)
    enabled := 0
    if in.Enabled { enabled = 1 }
    res, err := db.ExecContext(ctx, `INSERT INTO rule_categories
        (code, display_name, kind, site_tags, ip_tags, inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order)
        VALUES (?, ?, 'custom', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        in.Code, in.DisplayName, string(site), string(ip), string(ds), string(dk), string(ic),
        in.Protocol, in.DefaultGroupID, enabled, in.SortOrder)
    if err != nil { return 0, err }
    return res.LastInsertId()
}

func UpdateCategory(ctx context.Context, db FullDB, id int64, in CategoryInput, isSystem bool) error {
    if isSystem {
        // 系统分类仅允许改 enabled / default_group_id / sort_order
        enabled := 0
        if in.Enabled { enabled = 1 }
        _, err := db.ExecContext(ctx, `UPDATE rule_categories SET enabled=?, default_group_id=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
            enabled, in.DefaultGroupID, in.SortOrder, id)
        return err
    }
    site, _ := json.Marshal(in.SiteTags)
    ip, _ := json.Marshal(in.IPTags)
    ds, _ := json.Marshal(in.InlineDomainSuffix)
    dk, _ := json.Marshal(in.InlineDomainKeyword)
    ic, _ := json.Marshal(in.InlineIPCIDR)
    enabled := 0
    if in.Enabled { enabled = 1 }
    _, err := db.ExecContext(ctx, `UPDATE rule_categories SET
        display_name=?, site_tags=?, ip_tags=?, inline_domain_suffix=?, inline_domain_keyword=?, inline_ip_cidr=?,
        protocol=?, default_group_id=?, enabled=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
        in.DisplayName, string(site), string(ip), string(ds), string(dk), string(ic),
        in.Protocol, in.DefaultGroupID, enabled, in.SortOrder, id)
    return err
}

func DeleteCategory(ctx context.Context, db FullDB, id int64) error {
    var kind string
    if err := db.QueryRowContext(ctx, `SELECT kind FROM rule_categories WHERE id=?`, id).Scan(&kind); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }
        return err
    }
    if kind == "system" { return ErrSystemImmutable }
    _, err := db.ExecContext(ctx, `DELETE FROM rule_categories WHERE id=?`, id)
    return err
}

// ---- Group CRUD ----

type GroupInput struct {
    Code        string
    DisplayName string
    Type        string
    Members     []string
    SortOrder   int
}

func CreateGroup(ctx context.Context, db FullDB, in GroupInput) (int64, error) {
    members, _ := json.Marshal(in.Members)
    res, err := db.ExecContext(ctx, `INSERT INTO outbound_groups (code, display_name, type, members, kind, sort_order)
        VALUES (?, ?, ?, ?, 'custom', ?)`,
        in.Code, in.DisplayName, in.Type, string(members), in.SortOrder)
    if err != nil { return 0, err }
    return res.LastInsertId()
}

func UpdateGroup(ctx context.Context, db FullDB, id int64, in GroupInput, isSystem bool) error {
    members, _ := json.Marshal(in.Members)
    if isSystem {
        _, err := db.ExecContext(ctx, `UPDATE outbound_groups SET display_name=?, members=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
            in.DisplayName, string(members), in.SortOrder, id)
        return err
    }
    _, err := db.ExecContext(ctx, `UPDATE outbound_groups SET code=?, display_name=?, type=?, members=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
        in.Code, in.DisplayName, in.Type, string(members), in.SortOrder, id)
    return err
}

func DeleteGroup(ctx context.Context, db FullDB, id int64) error {
    var kind string
    if err := db.QueryRowContext(ctx, `SELECT kind FROM outbound_groups WHERE id=?`, id).Scan(&kind); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }
        return err
    }
    if kind == "system" { return ErrSystemImmutable }

    var catCount, ruleCount int
    _ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rule_categories WHERE default_group_id=?`, id).Scan(&catCount)
    _ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_rules WHERE outbound_group_id=?`, id).Scan(&ruleCount)
    if catCount > 0 || ruleCount > 0 {
        return fmt.Errorf("%w: %d categories, %d rules", ErrGroupReferenced, catCount, ruleCount)
    }
    _, err := db.ExecContext(ctx, `DELETE FROM outbound_groups WHERE id=?`, id)
    return err
}

// ---- CustomRule CRUD ----

type CustomRuleInput struct {
    Name            string
    SiteTags        []string
    IPTags          []string
    DomainSuffix    []string
    DomainKeyword   []string
    IPCIDR          []string
    SrcIPCIDR       []string
    Protocol        string
    Port            string
    OutboundGroupID *int64
    OutboundLiteral string
    SortOrder       int
}

func (i CustomRuleInput) Validate() error {
    if (i.OutboundGroupID == nil) == (i.OutboundLiteral == "") {
        return ErrInvalidOutbound
    }
    return nil
}

func CreateCustomRule(ctx context.Context, db FullDB, in CustomRuleInput) (int64, error) {
    if err := in.Validate(); err != nil { return 0, err }
    site, _ := json.Marshal(in.SiteTags)
    ip, _ := json.Marshal(in.IPTags)
    ds, _ := json.Marshal(in.DomainSuffix)
    dk, _ := json.Marshal(in.DomainKeyword)
    ic, _ := json.Marshal(in.IPCIDR)
    sic, _ := json.Marshal(in.SrcIPCIDR)
    res, err := db.ExecContext(ctx, `INSERT INTO custom_rules
        (name, site_tags, ip_tags, domain_suffix, domain_keyword, ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        in.Name, string(site), string(ip), string(ds), string(dk), string(ic), string(sic),
        in.Protocol, in.Port, in.OutboundGroupID, in.OutboundLiteral, in.SortOrder)
    if err != nil { return 0, err }
    return res.LastInsertId()
}

func UpdateCustomRule(ctx context.Context, db FullDB, id int64, in CustomRuleInput) error {
    if err := in.Validate(); err != nil { return err }
    site, _ := json.Marshal(in.SiteTags)
    ip, _ := json.Marshal(in.IPTags)
    ds, _ := json.Marshal(in.DomainSuffix)
    dk, _ := json.Marshal(in.DomainKeyword)
    ic, _ := json.Marshal(in.IPCIDR)
    sic, _ := json.Marshal(in.SrcIPCIDR)
    _, err := db.ExecContext(ctx, `UPDATE custom_rules SET
        name=?, site_tags=?, ip_tags=?, domain_suffix=?, domain_keyword=?, ip_cidr=?, src_ip_cidr=?,
        protocol=?, port=?, outbound_group_id=?, outbound_literal=?, sort_order=?, updated_at=CURRENT_TIMESTAMP
        WHERE id=?`,
        in.Name, string(site), string(ip), string(ds), string(dk), string(ic), string(sic),
        in.Protocol, in.Port, in.OutboundGroupID, in.OutboundLiteral, in.SortOrder, id)
    return err
}

func DeleteCustomRule(ctx context.Context, db FullDB, id int64) error {
    _, err := db.ExecContext(ctx, `DELETE FROM custom_rules WHERE id=?`, id)
    return err
}

// ---- Apply Preset ----

// ApplyPreset 将预设的 enabled_categories 覆盖到 DB（持久化），并记 active_preset。
func ApplyPreset(ctx context.Context, db FullDB, code string) error {
    preset, err := GetPreset(ctx, db, code)
    if err != nil { return err }
    if preset == nil { return ErrNotFound }

    allowed := map[string]bool{}
    for _, c := range preset.EnabledCategories {
        allowed[c] = true
    }
    cats, err := ListCategories(ctx, db)
    if err != nil { return err }
    for _, c := range cats {
        enabled := 0
        if allowed[c.Code] { enabled = 1 }
        if _, err := db.ExecContext(ctx, `UPDATE rule_categories SET enabled=? WHERE id=?`, enabled, c.ID); err != nil {
            return err
        }
    }
    _, err = db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('routing.active_preset', ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, code)
    return err
}
```

- [ ] **Step 2: 最小测试 `admin_test.go`（只覆盖最易出错的路径）**

```go
package routing_test

import (
    "context"
    "errors"
    "testing"

    "proxy-panel/internal/service/routing"
)

func TestCustomRuleInput_Validate(t *testing.T) {
    id := int64(1)
    cases := []struct {
        name string
        in   routing.CustomRuleInput
        err  error
    }{
        {"both", routing.CustomRuleInput{OutboundGroupID: &id, OutboundLiteral: "DIRECT"}, routing.ErrInvalidOutbound},
        {"neither", routing.CustomRuleInput{}, routing.ErrInvalidOutbound},
        {"group only", routing.CustomRuleInput{OutboundGroupID: &id}, nil},
        {"literal only", routing.CustomRuleInput{OutboundLiteral: "DIRECT"}, nil},
    }
    for _, tc := range cases {
        got := tc.in.Validate()
        if !errors.Is(got, tc.err) {
            t.Errorf("%s: got %v, want %v", tc.name, got, tc.err)
        }
    }
}

func TestDeleteGroup_SystemImmutable(t *testing.T) {
    db := setupTestDB(t) // reuse from builder_test.go
    if err := routing.DeleteGroup(context.Background(), db, 1); !errors.Is(err, routing.ErrSystemImmutable) {
        t.Errorf("want ErrSystemImmutable, got %v", err)
    }
}
```

`setupTestDB` 在 builder_test.go 已定义（同包 `routing_test`），可直接复用。

- [ ] **Step 3: 运行测试**

Run: `go test ./internal/service/routing/ -v`
Expected: 全 PASS。

- [ ] **Step 4: 提交**

```bash
git add internal/service/routing/admin.go internal/service/routing/admin_test.go
git commit -m "feat(routing): admin CRUD helpers + preset apply"
```

---

## Task 14: `/api/routing/*` HTTP handler

**Files:**
- Create: `internal/handler/routing.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: 创建 `internal/handler/routing.go`**

```go
package handler

import (
    "errors"
    "net/http"
    "strconv"

    "proxy-panel/internal/database"
    "proxy-panel/internal/service/routing"

    "github.com/gin-gonic/gin"
)

type RoutingHandler struct {
    db *database.DB
}

func NewRoutingHandler(db *database.DB) *RoutingHandler {
    return &RoutingHandler{db: db}
}

// GET /api/routing/config
func (h *RoutingHandler) GetConfig(c *gin.Context) {
    ctx := c.Request.Context()
    cats, err := routing.ListCategories(ctx, h.db)
    if err != nil { h.fail(c, err); return }
    groups, err := routing.ListGroups(ctx, h.db)
    if err != nil { h.fail(c, err); return }
    custom, err := routing.ListCustomRules(ctx, h.db)
    if err != nil { h.fail(c, err); return }

    presets := []routing.PresetRow{}
    rows, err := h.db.QueryContext(ctx, `SELECT code, display_name, enabled_categories FROM rule_presets ORDER BY code`)
    if err != nil { h.fail(c, err); return }
    defer rows.Close()
    for rows.Next() {
        var p routing.PresetRow
        var ec string
        rows.Scan(&p.Code, &p.DisplayName, &ec)
        _ = jsonUnmarshalStrict(ec, &p.EnabledCategories)
        presets = append(presets, p)
    }

    settings := map[string]string{}
    for _, k := range []string{
        "routing.site_ruleset_base_url.clash", "routing.ip_ruleset_base_url.clash",
        "routing.site_ruleset_base_url.singbox", "routing.ip_ruleset_base_url.singbox",
        "routing.surge_site_ruleset_base_url",
        "routing.final_outbound", "routing.active_preset",
    } {
        settings[k] = routing.GetRoutingSetting(ctx, h.db, k, "")
    }

    c.JSON(http.StatusOK, gin.H{
        "categories": cats, "groups": groups, "customRules": custom,
        "presets": presets, "settings": settings,
    })
}

// ---- Categories ----

func (h *RoutingHandler) CreateCategory(c *gin.Context) {
    var in routing.CategoryInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    id, err := routing.CreateCategory(c.Request.Context(), h.db, in)
    if err != nil { h.fail(c, err); return }
    c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateCategory(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    var in routing.CategoryInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    var kind string
    if err := h.db.QueryRowContext(c.Request.Context(), `SELECT kind FROM rule_categories WHERE id=?`, id).Scan(&kind); err != nil {
        h.fail(c, err); return
    }
    if err := routing.UpdateCategory(c.Request.Context(), h.db, id, in, kind == "system"); err != nil {
        h.fail(c, err); return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteCategory(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    if err := routing.DeleteCategory(c.Request.Context(), h.db, id); err != nil {
        if errors.Is(err, routing.ErrSystemImmutable) { c.JSON(http.StatusForbidden, gin.H{"error": err.Error()}); return }
        if errors.Is(err, routing.ErrNotFound) { c.JSON(http.StatusNotFound, gin.H{"error": err.Error()}); return }
        h.fail(c, err); return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Groups ----（与 Categories 同构，字段替换为 GroupInput；略）

func (h *RoutingHandler) CreateGroup(c *gin.Context)  { /* 同构 — 完整实现见下 */ }
func (h *RoutingHandler) UpdateGroup(c *gin.Context)  { /* 同构 */ }
func (h *RoutingHandler) DeleteGroup(c *gin.Context)  { /* 同构 */ }

// ---- CustomRules ----（同构，字段 CustomRuleInput；Validate 失败返回 400）

func (h *RoutingHandler) CreateCustomRule(c *gin.Context) { /* 同构 */ }
func (h *RoutingHandler) UpdateCustomRule(c *gin.Context) { /* 同构 */ }
func (h *RoutingHandler) DeleteCustomRule(c *gin.Context) { /* 同构 */ }

// ---- 其他 ----

func (h *RoutingHandler) ApplyPreset(c *gin.Context) {
    var body struct{ Code string `json:"code"` }
    if err := c.ShouldBindJSON(&body); err != nil { h.badReq(c, err); return }
    if err := routing.ApplyPreset(c.Request.Context(), h.db, body.Code); err != nil {
        if errors.Is(err, routing.ErrNotFound) { c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"}); return }
        h.fail(c, err); return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ImportLegacy 手动触发老文本导入（body: {text, mode}）
func (h *RoutingHandler) ImportLegacy(c *gin.Context) {
    var body struct {
        Text string `json:"text"`
        Mode string `json:"mode"` // prepend | override
    }
    if err := c.ShouldBindJSON(&body); err != nil { h.badReq(c, err); return }
    rules, err := routing.ParseLegacyRules(body.Text)
    if err != nil { h.fail(c, err); return }
    ctx := c.Request.Context()
    groups, _ := routing.ListGroups(ctx, h.db)
    groupIDByCode := map[string]int64{}
    for _, g := range groups { groupIDByCode[g.Code] = g.ID }
    imported := 0
    for i, r := range rules {
        code := routing.MapLegacyOutboundToCode(r.Outbound)
        var gid *int64
        lit := ""
        switch {
        case code == "DIRECT", code == "REJECT":
            lit = code
        case code != "":
            v := groupIDByCode[code]; gid = &v
        default:
            v := groupIDByCode["fallback"]; gid = &v
        }
        site, ip, ds, dk, ic := r.ToCustomRuleFields()
        _, err := routing.CreateCustomRule(ctx, h.db, routing.CustomRuleInput{
            Name: "import-" + r.Type + "-" + r.Value,
            SiteTags: site, IPTags: ip, DomainSuffix: ds, DomainKeyword: dk, IPCIDR: ic,
            OutboundGroupID: gid, OutboundLiteral: lit, SortOrder: i,
        })
        if err != nil { h.fail(c, err); return }
        imported++
    }
    if body.Mode == "override" {
        h.db.Exec(`UPDATE rule_categories SET enabled = 0 WHERE kind = 'system'`)
    }
    c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (h *RoutingHandler) fail(c *gin.Context, err error)   { c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()}) }
func (h *RoutingHandler) badReq(c *gin.Context, err error) { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) }

func jsonUnmarshalStrict(s string, out any) error {
    if s == "" { return nil }
    return json.Unmarshal([]byte(s), out)
}
```

**说明：** `CreateGroup/UpdateGroup/DeleteGroup/CreateCustomRule/UpdateCustomRule/DeleteCustomRule` 与 Category 三方法结构一模一样——参照 `CreateCategory/UpdateCategory/DeleteCategory` 写出，把 `CategoryInput` 换成 `GroupInput` / `CustomRuleInput`，调用 `routing.CreateGroup`/`UpdateGroup`/`DeleteGroup`/`CreateCustomRule` 等。其中：
- `UpdateGroup` 需要先查 `outbound_groups` 的 kind 字段判断 isSystem
- `CreateCustomRule`/`UpdateCustomRule` 若 `in.Validate()` 失败，handler 返回 400 并带 `err.Error()`

完整 Groups/CustomRules 实现不做省略：

```go
func (h *RoutingHandler) CreateGroup(c *gin.Context) {
    var in routing.GroupInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    id, err := routing.CreateGroup(c.Request.Context(), h.db, in)
    if err != nil { h.fail(c, err); return }
    c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateGroup(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    var in routing.GroupInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    var kind string
    if err := h.db.QueryRowContext(c.Request.Context(), `SELECT kind FROM outbound_groups WHERE id=?`, id).Scan(&kind); err != nil {
        h.fail(c, err); return
    }
    if err := routing.UpdateGroup(c.Request.Context(), h.db, id, in, kind == "system"); err != nil {
        h.fail(c, err); return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteGroup(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    err := routing.DeleteGroup(c.Request.Context(), h.db, id)
    switch {
    case errors.Is(err, routing.ErrSystemImmutable):
        c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
    case errors.Is(err, routing.ErrNotFound):
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
    case errors.Is(err, routing.ErrGroupReferenced):
        c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
    case err != nil:
        h.fail(c, err)
    default:
        c.JSON(http.StatusOK, gin.H{"ok": true})
    }
}

func (h *RoutingHandler) CreateCustomRule(c *gin.Context) {
    var in routing.CustomRuleInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    id, err := routing.CreateCustomRule(c.Request.Context(), h.db, in)
    if errors.Is(err, routing.ErrInvalidOutbound) { h.badReq(c, err); return }
    if err != nil { h.fail(c, err); return }
    c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateCustomRule(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    var in routing.CustomRuleInput
    if err := c.ShouldBindJSON(&in); err != nil { h.badReq(c, err); return }
    err := routing.UpdateCustomRule(c.Request.Context(), h.db, id, in)
    if errors.Is(err, routing.ErrInvalidOutbound) { h.badReq(c, err); return }
    if err != nil { h.fail(c, err); return }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteCustomRule(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    if err := routing.DeleteCustomRule(c.Request.Context(), h.db, id); err != nil {
        h.fail(c, err); return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

- [ ] **Step 2: 在 `internal/router/router.go` 注册**

在现有 `auth.GET("/users/:id/sub-tokens", ...)` 等路由所在 `auth` 组里追加（紧随最后一条 `sub-tokens` 路由之后）：

```go
    routingHandler := handler.NewRoutingHandler(db)
    auth.GET("/routing/config", routingHandler.GetConfig)
    auth.POST("/routing/categories", routingHandler.CreateCategory)
    auth.PUT("/routing/categories/:id", routingHandler.UpdateCategory)
    auth.DELETE("/routing/categories/:id", routingHandler.DeleteCategory)
    auth.POST("/routing/groups", routingHandler.CreateGroup)
    auth.PUT("/routing/groups/:id", routingHandler.UpdateGroup)
    auth.DELETE("/routing/groups/:id", routingHandler.DeleteGroup)
    auth.POST("/routing/custom-rules", routingHandler.CreateCustomRule)
    auth.PUT("/routing/custom-rules/:id", routingHandler.UpdateCustomRule)
    auth.DELETE("/routing/custom-rules/:id", routingHandler.DeleteCustomRule)
    auth.POST("/routing/apply-preset", routingHandler.ApplyPreset)
    auth.POST("/routing/import-legacy", routingHandler.ImportLegacy)
```

- [ ] **Step 3: `handler/routing.go` 顶部 import**

```go
import (
    "encoding/json"
    "errors"
    "net/http"
    "strconv"

    "proxy-panel/internal/database"
    "proxy-panel/internal/service/routing"

    "github.com/gin-gonic/gin"
)
```

- [ ] **Step 4: 编译 + 冒烟**

Run: `go build ./... && go run ./cmd/server -c config.example.yaml` → `curl -H "Cookie: ..." http://localhost:PORT/api/routing/config | jq .categories | head`
Expected: 18 条分类 JSON。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/routing.go internal/router/router.go
git commit -m "feat(routing): admin HTTP API /api/routing/*"
```

---

## Task 15: `setting.go` — 老 `custom_rules` 键 deprecation

**Files:**
- Modify: `internal/handler/setting.go:114-120`

- [ ] **Step 1: 在 `Update()` 循环写入之前添加过滤**

```go
    // deprecated 键：忽略写入，通过响应 warnings 提示
    deprecated := []string{"custom_rules", "custom_rules_mode"}
    warnings := []string{}
    for _, k := range deprecated {
        if _, ok := settings[k]; ok {
            delete(settings, k)
            warnings = append(warnings, "routing.legacy_ignored:"+k)
        }
    }
```

- [ ] **Step 2: 修改 `Get()` 末尾，把这两个键回显为空字符串并加 meta**

```go
    settings["custom_rules"] = ""
    settings["custom_rules_mode"] = ""
    c.JSON(http.StatusOK, gin.H{
        "settings": settings,
        "deprecated": []string{"custom_rules", "custom_rules_mode"},
    })
```

**注意：** `Get()` 原本直接 `c.JSON(http.StatusOK, settings)`。若前端已依赖直接是 map 的响应体，这个包装层会破坏前端。**更安全的做法**：只加个独立字段而不改顶层结构，或新增 `deprecated` header。

采用 header 方案（更兼容）：

```go
    settings["custom_rules"] = ""
    settings["custom_rules_mode"] = ""
    c.Header("X-Deprecated-Settings", "custom_rules,custom_rules_mode")
    c.JSON(http.StatusOK, settings)
```

- [ ] **Step 3: `Update()` 返回时带 warnings**

```go
    resp := gin.H{"message": "保存成功"}
    if len(warnings) > 0 {
        resp["warnings"] = warnings
    }
    c.JSON(http.StatusOK, resp)
```

- [ ] **Step 4: 编译**

Run: `go build ./...`
Expected: 通过。

- [ ] **Step 5: 提交**

```bash
git add internal/handler/setting.go
git commit -m "feat(setting): deprecate custom_rules/custom_rules_mode keys"
```

---

## Task 16: 前端 API 客户端 + 类型

**Files:**
- Create: `web/src/views/settings/routing/types.ts`
- Create: `web/src/api/routing.ts`

- [ ] **Step 1: 创建 `types.ts`**

```ts
export interface Category {
  id: number
  code: string
  displayName: string
  kind: 'system' | 'custom'
  siteTags: string[]
  ipTags: string[]
  inlineDomainSuffix: string[]
  inlineDomainKeyword: string[]
  inlineIPCIDR: string[]
  protocol: string
  defaultGroupId: number | null
  enabled: boolean
  sortOrder: number
}

export interface Group {
  id: number
  code: string
  displayName: string
  type: 'selector' | 'urltest'
  members: string[]
  kind: 'system' | 'custom'
  sortOrder: number
}

export interface CustomRule {
  id: number
  name: string
  siteTags: string[]
  ipTags: string[]
  domainSuffix: string[]
  domainKeyword: string[]
  ipCIDR: string[]
  srcIPCIDR: string[]
  protocol: string
  port: string
  outboundGroupId: number | null
  outboundLiteral: string
  sortOrder: number
}

export interface Preset {
  code: string
  displayName: string
  enabledCategories: string[]
}

export interface RoutingConfig {
  categories: Category[]
  groups: Group[]
  customRules: CustomRule[]
  presets: Preset[]
  settings: Record<string, string>
}
```

- [ ] **Step 2: 创建 `api/routing.ts`**

```ts
import http from './http' // 复用项目现有 axios 实例，路径按实际调整
import type { RoutingConfig, Category, Group, CustomRule } from '@/views/settings/routing/types'

export const getRoutingConfig = () =>
  http.get<RoutingConfig>('/api/routing/config').then(r => r.data)

export const createCategory = (body: Partial<Category>) =>
  http.post('/api/routing/categories', body).then(r => r.data)

export const updateCategory = (id: number, body: Partial<Category>) =>
  http.put(`/api/routing/categories/${id}`, body).then(r => r.data)

export const deleteCategory = (id: number) =>
  http.delete(`/api/routing/categories/${id}`).then(r => r.data)

export const createGroup = (body: Partial<Group>) =>
  http.post('/api/routing/groups', body).then(r => r.data)

export const updateGroup = (id: number, body: Partial<Group>) =>
  http.put(`/api/routing/groups/${id}`, body).then(r => r.data)

export const deleteGroup = (id: number) =>
  http.delete(`/api/routing/groups/${id}`).then(r => r.data)

export const createCustomRule = (body: Partial<CustomRule>) =>
  http.post('/api/routing/custom-rules', body).then(r => r.data)

export const updateCustomRule = (id: number, body: Partial<CustomRule>) =>
  http.put(`/api/routing/custom-rules/${id}`, body).then(r => r.data)

export const deleteCustomRule = (id: number) =>
  http.delete(`/api/routing/custom-rules/${id}`).then(r => r.data)

export const applyPreset = (code: string) =>
  http.post('/api/routing/apply-preset', { code }).then(r => r.data)

export const importLegacy = (text: string, mode: 'prepend' | 'override') =>
  http.post('/api/routing/import-legacy', { text, mode }).then(r => r.data)
```

- [ ] **Step 3: 验证前端类型检查通过**

Run: `cd web && npm run typecheck`（或 `vue-tsc --noEmit`）
Expected: 无错误。若 `http` 路径不同，先查 `web/src/api/` 现有文件的 import。

- [ ] **Step 4: 提交**

```bash
git add web/src/views/settings/routing/types.ts web/src/api/routing.ts
git commit -m "feat(web/routing): API client + types"
```

---

## Task 17: 前端容器 `RoutingSection.vue`

**Files:**
- Create: `web/src/views/settings/routing/RoutingSection.vue`

- [ ] **Step 1: 创建容器 + 4 个 tab 占位**

```vue
<template>
  <div class="routing-section">
    <el-tabs v-model="active">
      <el-tab-pane label="规则分类" name="categories">
        <CategoriesTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="出站组" name="groups">
        <GroupsTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="自定义规则" name="custom">
        <CustomRulesTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
      <el-tab-pane label="高级" name="advanced">
        <AdvancedTab v-if="config" :config="config" @refresh="load" />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getRoutingConfig } from '@/api/routing'
import type { RoutingConfig } from './types'
import CategoriesTab from './CategoriesTab.vue'
import GroupsTab from './GroupsTab.vue'
import CustomRulesTab from './CustomRulesTab.vue'
import AdvancedTab from './AdvancedTab.vue'

const active = ref('categories')
const config = ref<RoutingConfig | null>(null)

async function load() {
  config.value = await getRoutingConfig()
}
onMounted(load)
</script>
```

- [ ] **Step 2: 在父页面（Settings 主 view）挂载替换原 `RulesSection`**

查找 `RulesSection` 在父组件里的引用位置：

Run: `grep -rn "RulesSection" web/src/ --include='*.vue'`

替换为：

```vue
<RoutingSection />
```

并更新 import 路径为 `'./routing/RoutingSection.vue'`。

- [ ] **Step 3: 4 个 tab 文件先写空占位**，让 typecheck 通过：

```vue
<!-- CategoriesTab.vue / GroupsTab.vue / CustomRulesTab.vue / AdvancedTab.vue 各一份 -->
<template><div>TODO</div></template>
<script setup lang="ts">
import type { RoutingConfig } from './types'
defineProps<{ config: RoutingConfig }>()
defineEmits<{ (e: 'refresh'): void }>()
</script>
```

- [ ] **Step 4: 前端类型检查 + 启动验证**

Run: `cd web && npm run typecheck && npm run dev`
Expected: 无错误；浏览器打开设置页，看到 4 个 tab，每个显示 "TODO"。

- [ ] **Step 5: 提交**

```bash
git add web/src/views/settings/routing/ web/src/views/settings/<parent-view>.vue
git commit -m "feat(web/routing): container + 4 tab skeleton"
```

---

## Task 18: `CategoriesTab.vue` 实现

**Files:**
- Modify: `web/src/views/settings/routing/CategoriesTab.vue`

- [ ] **Step 1: 实现分类表格 + 预设选择**

```vue
<template>
  <div>
    <div class="preset-bar">
      <span>应用预设方案：</span>
      <el-select v-model="presetCode" placeholder="选择预设" style="width: 200px">
        <el-option v-for="p in config.presets" :key="p.code" :label="p.displayName" :value="p.code" />
      </el-select>
      <el-button type="primary" :disabled="!presetCode" @click="onApplyPreset">应用（覆盖启用分类）</el-button>
      <el-button @click="onAddCustom">+ 新增自定义分类</el-button>
    </div>

    <el-table :data="config.categories" row-key="id" border>
      <el-table-column prop="displayName" label="名称" width="180" />
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag :type="row.kind === 'system' ? 'info' : 'success'" size="small">
            {{ row.kind === 'system' ? '系统' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Site Tags">
        <template #default="{ row }">
          <el-tag v-for="t in row.siteTags" :key="t" size="small">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="IP Tags">
        <template #default="{ row }">
          <el-tag v-for="t in row.ipTags" :key="t" size="small" type="warning">{{ t }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="默认出站组" width="200">
        <template #default="{ row }">
          <el-select v-model="row.defaultGroupId" size="small" @change="onUpdate(row)">
            <el-option v-for="g in config.groups" :key="g.id" :label="g.displayName" :value="g.id" />
          </el-select>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="80">
        <template #default="{ row }">
          <el-switch v-model="row.enabled" @change="onUpdate(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button v-if="row.kind === 'custom'" size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 编辑对话框：仅 code/displayName/siteTags/ipTags/inline* 字段，系统分类禁用大多数字段 -->
    <CategoryEditDialog v-model="editing" :readonly="editingIsSystem" :groups="config.groups"
      @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { applyPreset, updateCategory, deleteCategory } from '@/api/routing'
import type { RoutingConfig, Category } from './types'
import CategoryEditDialog from './CategoryEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()

const presetCode = ref('')
const editing = ref<Category | null>(null)
const editingIsSystem = ref(false)

async function onApplyPreset() {
  await ElMessageBox.confirm('将覆盖当前启用的分类，确定？', '应用预设', { type: 'warning' })
  await applyPreset(presetCode.value)
  ElMessage.success('已应用')
  emit('refresh')
}

async function onUpdate(row: Category) {
  await updateCategory(row.id, row)
  ElMessage.success('已保存')
  emit('refresh')
}

function onEdit(row: Category) {
  editingIsSystem.value = row.kind === 'system'
  editing.value = { ...row }
}

function onAddCustom() {
  editingIsSystem.value = false
  editing.value = {
    id: 0, code: '', displayName: '', kind: 'custom',
    siteTags: [], ipTags: [], inlineDomainSuffix: [], inlineDomainKeyword: [], inlineIPCIDR: [],
    protocol: '', defaultGroupId: null, enabled: true, sortOrder: 500,
  }
}

async function onSave(row: Category) {
  if (row.id === 0) {
    const { createCategory } = await import('@/api/routing')
    await createCategory(row)
  } else {
    await updateCategory(row.id, row)
  }
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}

async function onDelete(row: Category) {
  await ElMessageBox.confirm(`删除自定义分类 ${row.displayName}？`, '确认', { type: 'warning' })
  await deleteCategory(row.id)
  emit('refresh')
}
</script>
```

- [ ] **Step 2: 创建 `CategoryEditDialog.vue`**（编辑表单）

```vue
<template>
  <el-dialog :model-value="!!modelValue" title="编辑分类" @update:model-value="$emit('update:modelValue', null)" width="640px">
    <el-form v-if="modelValue" label-width="140px">
      <el-form-item label="Code"><el-input v-model="modelValue.code" :disabled="readonly" /></el-form-item>
      <el-form-item label="显示名"><el-input v-model="modelValue.displayName" :disabled="readonly" /></el-form-item>
      <el-form-item label="Site Tags">
        <el-select v-model="modelValue.siteTags" :disabled="readonly" multiple filterable allow-create
          placeholder="如 google, youtube" style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="IP Tags">
        <el-select v-model="modelValue.ipTags" :disabled="readonly" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="内联 domain_suffix">
        <el-select v-model="modelValue.inlineDomainSuffix" :disabled="readonly" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="内联 domain_keyword">
        <el-select v-model="modelValue.inlineDomainKeyword" :disabled="readonly" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="内联 ip_cidr">
        <el-select v-model="modelValue.inlineIPCIDR" :disabled="readonly" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="默认出站组">
        <el-select v-model="modelValue.defaultGroupId" style="width: 100%">
          <el-option v-for="g in groups" :key="g.id" :label="g.displayName" :value="g.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="排序"><el-input-number v-model="modelValue.sortOrder" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', null)">取消</el-button>
      <el-button type="primary" @click="$emit('save', modelValue!)">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { Category, Group } from './types'
defineProps<{ modelValue: Category | null; readonly: boolean; groups: Group[] }>()
defineEmits<{ (e: 'update:modelValue', v: Category | null): void; (e: 'save', v: Category): void }>()
</script>
```

- [ ] **Step 3: 前端 typecheck + 手动验证**

Run: `cd web && npm run typecheck && npm run dev`
手动：开分类 tab，确认 18 条系统分类显示；切换启用开关；下拉改默认出站组；点"应用预设"。

- [ ] **Step 4: 提交**

```bash
git add web/src/views/settings/routing/CategoriesTab.vue web/src/views/settings/routing/CategoryEditDialog.vue
git commit -m "feat(web/routing): categories tab + edit dialog"
```

---

## Task 19: `GroupsTab.vue` 实现

**Files:**
- Modify: `web/src/views/settings/routing/GroupsTab.vue`

- [ ] **Step 1: 实现**

```vue
<template>
  <div>
    <el-button @click="onAdd">+ 新增自定义组</el-button>
    <el-table :data="config.groups" border style="margin-top: 12px">
      <el-table-column prop="displayName" label="显示名" width="200" />
      <el-table-column prop="code" label="Code" width="160" />
      <el-table-column prop="type" label="类型" width="100" />
      <el-table-column label="成员">
        <template #default="{ row }">
          <el-tag v-for="m in row.members" :key="m" size="small" style="margin-right: 4px">{{ m }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag :type="row.kind === 'system' ? 'info' : 'success'" size="small">
            {{ row.kind === 'system' ? '系统' : '自定义' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="160">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button v-if="row.kind === 'custom'" size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <GroupEditDialog v-model="editing" :groups="config.groups" :readonly-code="editingIsSystem" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createGroup, updateGroup, deleteGroup } from '@/api/routing'
import type { RoutingConfig, Group } from './types'
import GroupEditDialog from './GroupEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const editing = ref<Group | null>(null)
const editingIsSystem = ref(false)

function onAdd() {
  editingIsSystem.value = false
  editing.value = { id: 0, code: '', displayName: '', type: 'selector', members: [], kind: 'custom', sortOrder: 500 }
}

function onEdit(row: Group) {
  editingIsSystem.value = row.kind === 'system'
  editing.value = { ...row, members: [...row.members] }
}

async function onSave(row: Group) {
  if (row.id === 0) await createGroup(row)
  else await updateGroup(row.id, row)
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}

async function onDelete(row: Group) {
  await ElMessageBox.confirm(`删除出站组 ${row.displayName}？`, '确认', { type: 'warning' })
  try {
    await deleteGroup(row.id)
    emit('refresh')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '删除失败')
  }
}
</script>
```

- [ ] **Step 2: 创建 `GroupEditDialog.vue`**

```vue
<template>
  <el-dialog :model-value="!!modelValue" title="编辑出站组" @update:model-value="$emit('update:modelValue', null)" width="640px">
    <el-form v-if="modelValue" label-width="120px">
      <el-form-item label="Code"><el-input v-model="modelValue.code" :disabled="readonlyCode" /></el-form-item>
      <el-form-item label="显示名"><el-input v-model="modelValue.displayName" /></el-form-item>
      <el-form-item label="类型">
        <el-select v-model="modelValue.type" :disabled="readonlyCode">
          <el-option label="selector" value="selector" />
          <el-option label="urltest" value="urltest" />
        </el-select>
      </el-form-item>
      <el-form-item label="成员">
        <el-select v-model="modelValue.members" multiple filterable allow-create style="width: 100%">
          <el-option label="&lt;ALL&gt; (全部节点)" value="<ALL>" />
          <el-option label="DIRECT" value="DIRECT" />
          <el-option label="REJECT" value="REJECT" />
          <el-option v-for="g in groups" :key="g.code" :label="g.displayName + ' (' + g.code + ')'" :value="g.code" />
        </el-select>
      </el-form-item>
      <el-form-item label="排序"><el-input-number v-model="modelValue.sortOrder" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', null)">取消</el-button>
      <el-button type="primary" @click="$emit('save', modelValue!)">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { Group } from './types'
defineProps<{ modelValue: Group | null; readonlyCode: boolean; groups: Group[] }>()
defineEmits<{ (e: 'update:modelValue', v: Group | null): void; (e: 'save', v: Group): void }>()
</script>
```

- [ ] **Step 3: typecheck + 手动验证**

Run: `cd web && npm run typecheck && npm run dev`
手动：看 18 个系统组显示；新增 custom 组；编辑成员；删除（系统组删除应 403）

- [ ] **Step 4: 提交**

```bash
git add web/src/views/settings/routing/GroupsTab.vue web/src/views/settings/routing/GroupEditDialog.vue
git commit -m "feat(web/routing): groups tab + edit dialog"
```

---

## Task 20: `CustomRulesTab.vue` + `AdvancedTab.vue`

**Files:**
- Modify: `web/src/views/settings/routing/CustomRulesTab.vue`
- Modify: `web/src/views/settings/routing/AdvancedTab.vue`

- [ ] **Step 1: `CustomRulesTab.vue`**

```vue
<template>
  <div>
    <el-button @click="onAdd">+ 新增规则</el-button>
    <el-table :data="config.customRules" border style="margin-top: 12px">
      <el-table-column prop="name" label="名称" width="180" />
      <el-table-column label="Site">
        <template #default="{ row }"><el-tag v-for="t in row.siteTags" :key="t" size="small">{{ t }}</el-tag></template>
      </el-table-column>
      <el-table-column label="IP">
        <template #default="{ row }"><el-tag v-for="t in row.ipTags" :key="t" size="small" type="warning">{{ t }}</el-tag></template>
      </el-table-column>
      <el-table-column label="Domain Suffix">
        <template #default="{ row }">{{ row.domainSuffix.join(', ') }}</template>
      </el-table-column>
      <el-table-column label="IP CIDR">
        <template #default="{ row }">{{ row.ipCIDR.join(', ') }}</template>
      </el-table-column>
      <el-table-column label="出站" width="160">
        <template #default="{ row }">
          {{ row.outboundLiteral || groupName(row.outboundGroupId) }}
        </template>
      </el-table-column>
      <el-table-column prop="sortOrder" label="排序" width="80" />
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="onEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="onDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <CustomRuleEditDialog v-model="editing" :groups="config.groups" @save="onSave" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createCustomRule, updateCustomRule, deleteCustomRule } from '@/api/routing'
import type { RoutingConfig, CustomRule } from './types'
import CustomRuleEditDialog from './CustomRuleEditDialog.vue'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const editing = ref<CustomRule | null>(null)

function groupName(id: number | null) {
  return props.config.groups.find(g => g.id === id)?.displayName || '-'
}
function onAdd() {
  editing.value = {
    id: 0, name: '', siteTags: [], ipTags: [], domainSuffix: [], domainKeyword: [], ipCIDR: [], srcIPCIDR: [],
    protocol: '', port: '', outboundGroupId: props.config.groups[0]?.id ?? null, outboundLiteral: '', sortOrder: 100,
  }
}
function onEdit(row: CustomRule) { editing.value = { ...row } }
async function onSave(row: CustomRule) {
  if (row.id === 0) await createCustomRule(row)
  else await updateCustomRule(row.id, row)
  editing.value = null
  ElMessage.success('已保存')
  emit('refresh')
}
async function onDelete(row: CustomRule) {
  await ElMessageBox.confirm(`删除规则 ${row.name}？`, '确认', { type: 'warning' })
  await deleteCustomRule(row.id)
  emit('refresh')
}
</script>
```

- [ ] **Step 2: 创建 `CustomRuleEditDialog.vue`**

```vue
<template>
  <el-dialog :model-value="!!modelValue" title="编辑自定义规则" @update:model-value="$emit('update:modelValue', null)" width="720px">
    <el-form v-if="modelValue" label-width="140px">
      <el-form-item label="名称"><el-input v-model="modelValue.name" /></el-form-item>
      <el-form-item label="Site Tags">
        <el-select v-model="modelValue.siteTags" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="IP Tags">
        <el-select v-model="modelValue.ipTags" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="Domain Suffix">
        <el-select v-model="modelValue.domainSuffix" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="Domain Keyword">
        <el-select v-model="modelValue.domainKeyword" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="IP CIDR">
        <el-select v-model="modelValue.ipCIDR" multiple filterable allow-create style="width: 100%"></el-select>
      </el-form-item>
      <el-form-item label="出站">
        <el-radio-group v-model="outboundMode">
          <el-radio label="group">出站组</el-radio>
          <el-radio label="literal">字面量</el-radio>
        </el-radio-group>
        <el-select v-if="outboundMode === 'group'" v-model="modelValue.outboundGroupId" style="width: 100%; margin-top: 8px">
          <el-option v-for="g in groups" :key="g.id" :label="g.displayName" :value="g.id" />
        </el-select>
        <el-select v-else v-model="modelValue.outboundLiteral" style="width: 100%; margin-top: 8px">
          <el-option label="DIRECT" value="DIRECT" />
          <el-option label="REJECT" value="REJECT" />
        </el-select>
      </el-form-item>
      <el-form-item label="排序"><el-input-number v-model="modelValue.sortOrder" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', null)">取消</el-button>
      <el-button type="primary" @click="onSave">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { CustomRule, Group } from './types'
const props = defineProps<{ modelValue: CustomRule | null; groups: Group[] }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: CustomRule | null): void; (e: 'save', v: CustomRule): void }>()
const outboundMode = ref<'group' | 'literal'>('group')
watch(() => props.modelValue, (v) => {
  if (!v) return
  outboundMode.value = v.outboundLiteral ? 'literal' : 'group'
})
function onSave() {
  if (!props.modelValue) return
  const v = { ...props.modelValue }
  if (outboundMode.value === 'group') v.outboundLiteral = ''
  else v.outboundGroupId = null
  emit('save', v)
}
</script>
```

- [ ] **Step 3: `AdvancedTab.vue`**

```vue
<template>
  <div>
    <h3>URL 前缀覆写</h3>
    <el-form label-width="280px" style="max-width: 900px">
      <el-form-item label="Clash geosite (.mrs) 前缀">
        <el-input v-model="s['routing.site_ruleset_base_url.clash']" @change="save('routing.site_ruleset_base_url.clash')" />
      </el-form-item>
      <el-form-item label="Clash geoip (.mrs) 前缀">
        <el-input v-model="s['routing.ip_ruleset_base_url.clash']" @change="save('routing.ip_ruleset_base_url.clash')" />
      </el-form-item>
      <el-form-item label="Sing-box geosite (.srs) 前缀">
        <el-input v-model="s['routing.site_ruleset_base_url.singbox']" @change="save('routing.site_ruleset_base_url.singbox')" />
      </el-form-item>
      <el-form-item label="Sing-box geoip (.srs) 前缀">
        <el-input v-model="s['routing.ip_ruleset_base_url.singbox']" @change="save('routing.ip_ruleset_base_url.singbox')" />
      </el-form-item>
      <el-form-item label="Surge/Shadowrocket site 前缀（空=降级 GEOSITE）">
        <el-input v-model="s['routing.surge_site_ruleset_base_url']" @change="save('routing.surge_site_ruleset_base_url')" />
      </el-form-item>
      <el-form-item label="兜底出站组">
        <el-select v-model="s['routing.final_outbound']" @change="save('routing.final_outbound')" style="width: 300px">
          <el-option v-for="g in config.groups" :key="g.code" :label="g.displayName" :value="g.code" />
        </el-select>
      </el-form-item>
    </el-form>

    <h3 style="margin-top: 24px">从旧格式导入</h3>
    <el-form label-width="120px" style="max-width: 900px">
      <el-form-item label="旧规则文本">
        <el-input v-model="legacyText" type="textarea" :rows="10" placeholder="每行 TYPE,VALUE,OUTBOUND" />
      </el-form-item>
      <el-form-item label="模式">
        <el-select v-model="legacyMode" style="width: 200px">
          <el-option label="追加（保留启用分类）" value="prepend" />
          <el-option label="覆盖（关闭所有系统分类）" value="override" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="onImport">导入</el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { importLegacy } from '@/api/routing'
import { updateSettings } from '@/api/setting' // 复用已有 API
import type { RoutingConfig } from './types'

const props = defineProps<{ config: RoutingConfig }>()
const emit = defineEmits<{ (e: 'refresh'): void }>()
const s = reactive<Record<string, string>>({ ...props.config.settings })
const legacyText = ref('')
const legacyMode = ref<'prepend' | 'override'>('prepend')

async function save(key: string) {
  await updateSettings({ [key]: s[key] })
  ElMessage.success('已保存')
}

async function onImport() {
  const res = await importLegacy(legacyText.value, legacyMode.value)
  ElMessage.success(`导入 ${res.imported} 条`)
  legacyText.value = ''
  emit('refresh')
}
</script>
```

- [ ] **Step 4: 前端 typecheck + 手动回归**

Run: `cd web && npm run typecheck && npm run dev`
手动：
- 自定义规则 tab 新增/编辑/删除
- 高级 tab 改 URL 前缀保存；粘贴老文本导入验证

- [ ] **Step 5: 提交**

```bash
git add web/src/views/settings/routing/CustomRulesTab.vue web/src/views/settings/routing/CustomRuleEditDialog.vue web/src/views/settings/routing/AdvancedTab.vue
git commit -m "feat(web/routing): custom rules tab + advanced tab"
```

---

## Task 21: 清理旧文件

**Files:**
- Delete: `internal/service/subscription/rules.go`
- Delete: `web/src/views/settings/RulesSection.vue`
- Delete: `web/src/views/settings/RulesTable.vue`
- Delete: `web/src/views/settings/rules-types.ts`

- [ ] **Step 1: 确认无残余引用**

Run:
```bash
grep -rn "subscription.SetCustomRules\|subscription.GetCustomRules\|subscription.IsOverrideMode\|subscription.ProxyGroupNames" --include='*.go' .
grep -rn "RulesSection\|RulesTable\|rules-types" web/src
```
Expected: 两个命令都无命中。若有残余，修复引用后继续。

- [ ] **Step 2: 删除文件**

```bash
rm internal/service/subscription/rules.go
rm web/src/views/settings/RulesSection.vue
rm web/src/views/settings/RulesTable.vue
rm web/src/views/settings/rules-types.ts
```

- [ ] **Step 3: 编译 + typecheck + 启动冒烟**

```bash
go build ./...
cd web && npm run typecheck && npm run build
```
Expected: 无错误。

- [ ] **Step 4: 全量订阅冒烟**

启动服务；对每一种 format 都 curl 验证非空响应：
```bash
for f in clash singbox surge v2ray shadowrocket; do
  curl -s "http://localhost:PORT/api/sub/t/<token>?format=$f" | head -5
done
```
Expected: 5 种格式都返回合法内容。

- [ ] **Step 5: 提交**

```bash
git add -A
git commit -m "chore(routing): remove legacy rules.go and old UI components"
```

---

## Task 22: 文档更新

**Files:**
- Modify: `README.md`（如有"分流配置"章节）
- Modify: `specs/ROADMAP.md` 或等价文件（标记 P0 项完成）

- [ ] **Step 1: README 中描述新分流能力**

查找现有 README 里的分流/规则相关段落：

```bash
grep -n "规则\|分流\|custom_rules" README.md
```

更新为：
```md
### 分流配置
- 18 个内置规则分类（Google / YouTube / Telegram / AI 等）+ 用户自定义分类
- 18 个内置出站组 + 用户自定义组；支持 selector / urltest
- 结构化自定义规则（site_tags / ip_tags / domain_suffix / ip_cidr 等）
- 3 个预设方案（minimal / balanced / comprehensive）：一键应用 或订阅 URL `?preset=xxx` 临时覆盖
- rule-provider URL 前缀可覆写（方便切 gh-proxy 镜像）
- 五端订阅格式：Clash / Sing-box / Surge / V2Ray / Shadowrocket（弱规则引擎自动降级）
```

- [ ] **Step 2: ROADMAP 标记**

找到分流相关条目，打勾为完成。

- [ ] **Step 3: 提交**

```bash
git add README.md specs/ROADMAP.md
git commit -m "docs: update README + ROADMAP for routing refactor"
```

---

## 完成后的验收清单

- [ ] `go test ./internal/service/routing/...` 全 PASS
- [ ] `go build ./...` 通过
- [ ] `cd web && npm run typecheck && npm run build` 通过
- [ ] 启动服务，老用户（有 `settings.custom_rules` 文本）首次迁移后 `custom_rules` 表含对应行
- [ ] `/api/routing/config` 返回 18 分类 + 18 组 + 3 预设
- [ ] 五端订阅 curl 均返回合法内容；`?preset=minimal` 观察到 rules 数量变化
- [ ] 老订阅链接 `/api/sub/:uuid` 行为等价（无 `?preset` 时）

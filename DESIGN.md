# DESIGN.md — ProxyPanel

> 本文件定义具体的视觉系统数值。配合 PRODUCT.md 使用。

## Direction

- **Register**: product
- **Theme**: 浅色为主，深色作为后续 opt-in（v1 不交付）
- **Color strategy**: Restrained — 中性色 + 一个强调色 ≤10% 表面
- **Aesthetic lane**: Editorial-typographic — 中文 serif 标题 + 等宽数字 + 几何中文黑体身体

> 物理场景句：周二下午 14:00，开发者在 14 寸笔记本上室内明亮环境下打开面板，加用户、确认昨夜流量预警来源。三分钟内回到 IDE。

## Color tokens (OKLCH)

> 使用 OKLCH。极端亮度处降低 chroma。所有"中性灰"都向品牌 hue 偏一点（chroma 0.005–0.01）。

### Surface

| Token | OKLCH | 用途 |
|---|---|---|
| `--surface-base`        | `oklch(0.985 0.003 80)` | 全局背景（暖白，不是 #fff） |
| `--surface-raised`      | `oklch(0.995 0.003 80)` | 抬起的内容（侧栏、modal 内） |
| `--surface-sunken`      | `oklch(0.965 0.004 80)` | 凹陷区域（表头条带、code block） |
| `--surface-overlay`     | `oklch(0.99  0.003 80 / 0.85)` | 浮层背景（带轻微 backdrop） |

### Ink (foreground)

| Token | OKLCH | 用途 |
|---|---|---|
| `--ink-strong`   | `oklch(0.22 0.01 80)` | 主标题 / 关键数字 |
| `--ink-base`     | `oklch(0.32 0.008 80)` | 正文 |
| `--ink-muted`    | `oklch(0.52 0.008 80)` | 标签、次要说明 |
| `--ink-soft`     | `oklch(0.68 0.006 80)` | placeholder、禁用 |
| `--ink-faint`    | `oklch(0.86 0.004 80)` | 分隔线、最弱边界 |

### Accent — 单一强调色，仅 ≤10% 表面

选 **赤铁红（hematite red）** —— 不是营销红、不是错误红、不是 China red。
取自老书脊封面、矿物石墨与赤铁矿石之间的色温。

| Token | OKLCH | 用途 |
|---|---|---|
| `--accent`        | `oklch(0.48 0.13 28)`  | 主操作（按钮、选中标签、焦点环） |
| `--accent-soft`   | `oklch(0.94 0.025 28)` | 选中行、tag 浅底 |
| `--accent-ink`    | `oklch(0.30 0.10 28)`  | 强调色文字（在白底上） |

### Status

| Token | OKLCH | 用途 |
|---|---|---|
| `--status-ok`         | `oklch(0.55 0.10 155)` | 在线、运行中、正常 |
| `--status-warn`       | `oklch(0.68 0.13 70)`  | 流量预警、即将到期 |
| `--status-crit`       | `oklch(0.50 0.16 28)`  | 离线、超额、失败 |
| `--status-info`       | `oklch(0.55 0.07 235)` | 提示、信息 |
| `--status-ok-soft`    | `oklch(0.94 0.025 155)` |
| `--status-warn-soft`  | `oklch(0.94 0.04  70)`  |
| `--status-crit-soft`  | `oklch(0.94 0.025 28)`  |

## Typography

### Families

```
--font-serif:   'Noto Serif SC', 'Source Han Serif SC', 'Songti SC', ui-serif, serif
--font-sans:    'Inter', 'PingFang SC', 'Hiragino Sans GB', system-ui, sans-serif
--font-mono:    'JetBrains Mono', ui-monospace, 'SFMono-Regular', 'Cascadia Code', monospace
```

- **Serif**：仅用于 H1 / 大区块标题（仪表盘大标题、Login 顶标）
- **Sans**：所有 UI 默认字体
- **Mono**：所有数字、UUID、端口、域名、协议名、订阅 URL、code

> 全站默认 `font-feature-settings: "ss01", "tnum"`，让 Sans 也输出等宽数字。Mono 区只在确实是 code/identifier 的位置使用。

### Scale (≥1.25 ratio)

| Token | size / line-height / weight |
|---|---|
| `--text-display` | 32 / 40 / 500（serif）|
| `--text-h1`      | 24 / 32 / 600（sans，紧字距）|
| `--text-h2`      | 18 / 26 / 600 |
| `--text-h3`      | 15 / 22 / 600 |
| `--text-body`    | 14 / 22 / 400 |
| `--text-meta`    | 13 / 20 / 400（标签、表头）|
| `--text-micro`   | 12 / 16 / 500（dot 状态文字、键盘提示）|

### Letter spacing

- Display / H1：`-0.01em`
- Meta / micro / 大写小标题：`+0.04em`
- 其他：默认

## Spacing & rhythm

> 4 px 基准。但视觉节奏靠**变化**，不是均匀。

| Token | px |
|---|---|
| `--space-1` | 4 |
| `--space-2` | 8 |
| `--space-3` | 12 |
| `--space-4` | 16 |
| `--space-5` | 24 |
| `--space-6` | 32 |
| `--space-7` | 48 |
| `--space-8` | 72 |

**Section rhythm**：仪表盘各区块之间 `48–72`，区块内行间 `12–24`。

## Layout

- **App shell**：左侧导航 240px 固定（不再 220）。轻底色 `--surface-raised`，无重边框，仅一条 `--ink-faint` 1px 分隔线。无深色 sidebar。
- **Header**：56px。仅当前章节 H2 + 右侧用户菜单。无渐变、无阴影。
- **Content**：最大宽度 1200px 居中（设置页 880）。横向 padding `--space-6`，顶部 padding `--space-7`。
- **Density profiles**：
  - **Settings**：每条 64px+，带说明
  - **Dashboard**：节奏宽松，section 间 `--space-7`
  - **Tables (Users/Nodes/Logs)**：行高 44px，内边距 `--space-3 --space-4`

## Surfaces & elevation

- **No card by default.** 内容直接落在 `--surface-base`，靠分组标题 + 间距分区。
- **Section** ≠ Card：用 `padding-top` + 小标题（`--text-meta` 大写 + 字距 `+0.04em`）做分组，不画框。
- 当确实需要"抬起"时（modal、popover、临时面板）：`--surface-raised` + 1px `--ink-faint` 边 + `box-shadow: 0 1px 2px oklch(0.2 0.01 80 / 0.04), 0 8px 24px oklch(0.2 0.01 80 / 0.06)`。
- **绝对禁止**：左侧色条 border、卡片内嵌套卡片、图标圆形浅色底。

## Components

### Status dot

`6px` 圆点 + `--space-2` + 文字。颜色用 `--status-*`。**取代**所有 `el-tag` 状态用法（除非真的是可点击的 tag）。

```html
<span class="status-dot" data-state="ok"></span> 运行中
```

### Tabular row

```
| Name (sans 14)   | Protocol (mono 13) | Quota (mono 14, tnum) | Used (mono 14 tnum) | Status (dot+meta) | actions |
```

行间分隔 `1px solid --ink-faint`。鼠标悬停整行 `--surface-sunken`。选中 `--accent-soft` + `--accent-ink` 文字。

### Buttons

- **Primary**：`--accent` 底，白字，6px 圆角，padding `8 14`，`text-meta` 600。
- **Secondary**：透明底，`--ink-base` 文字，`--ink-faint` 1px 边。
- **Danger**：`--status-crit` 文字，无底，仅 hover 时 `--status-crit-soft` 底。
- **Ghost / link**：纯文字 + 下划线（hover 时显现）。
- 高度统一 36px（dense table 操作 28px）。

### Input

底白、1px `--ink-faint` 边、focus 时边变 `--accent` + 2px 外发光环 `oklch(from var(--accent) l c h / 0.2)`。无内阴影。圆角 6px。

### Tag (true tags, e.g. protocol vless/trojan)

中性底 `--surface-sunken` + `--ink-base` 文字 + mono 13。无图标，无背景圆。

## Motion

- 默认 transition `150ms cubic-bezier(0.2, 0, 0, 1)`（ease-out-quart）。
- **不动**布局属性（width/height/margin/padding）。
- 表格悬停、按钮 hover、tab 切换：仅过渡颜色 / opacity / transform。
- 页面切换：fade 120ms（非交叉位移）。

## Iconography

- 优先 Element Plus 自带 line icons（已有），尺寸固定 16 / 20。
- **不用图标圆形浅色底**。图标就是图标，不戴帽子。
- 状态用 dot 不用图标。

## Numbers & identifiers

所有：
- 流量值（GB / MB / B）
- 端口
- UUID（截断显示前 8 位 + … + 后 4 位，hover 全显，click 复制）
- 时间戳（`YYYY-MM-DD HH:mm:ss`）
- 域名 / 协议名

→ Mono 字体 + tabular-nums + `--ink-base`。表格里的数字列右对齐。

## Empty / loading / error states

- **空表格**：居中一行 serif `--text-h2` + 一行 meta 说明 + 一个 primary 按钮（"添加第一个节点"）。
- **空仪表盘**：极少出现。若用户/节点都为 0，直接渲染引导卡片"开始之前，先添加一个节点"。
- **加载**：表格首屏不要 spinner——用 6 行骨架（高度 44px、`--surface-sunken` 条带）。后续刷新用顶部细进度条。
- **错误**：`--status-crit` dot + 文案 + "重试"链接。不弹窗。

## Forbidden in this project

复述并扩展 impeccable 通用禁令：

- ❌ 深色 sidebar + 浅色 main 的 admin 经典布局
- ❌ 4 宫格 hero metric 卡片（图标圆 + 数字 + 标签）
- ❌ 任何 `border-left` ≥ 2px 的状态强调
- ❌ 渐变文字 / 渐变 metric 数字
- ❌ Element Plus 默认蓝 (#409eff)
- ❌ emoji 状态（🟢❌✅）
- ❌ 卡片内嵌套卡片
- ❌ 大量 `el-card shadow="hover"` 包裹一切
- ❌ 中英文之间无空格
- ❌ 数字与单位之间用全角空格

# PRODUCT.md — ProxyPanel

> 设计上下文文档。配合 DESIGN.md 使用。本文件描述「为谁、为何、长什么样不该长什么样」，DESIGN.md 描述「具体颜色、字体、间距是多少」。

## Register

**product** — 设计服务于产品。这是一个工具/管理后台，不是营销页。设计目标是「让运维者三分钟解决问题」，不是「让访客被打动」。

## Product Purpose

ProxyPanel 是面向**个人/小团队运维者**的自建代理面板，差异化点：
- 灵活的流量刷新周期（按月/周/日/自定义 cron）
- 服务器级总流量配额预警（VPS 月配额场景）
- Surge / Clash / Shadowrocket / Sing-box / V2Ray 多端订阅原生支持
- Telegram / 企业微信告警
- 一键部署（3 分钟干净 VPS → 可访问）

竞品（x-ui / 3x-ui / Xray-UI）的特征：深色赛博/终端风、密集表格、emoji 状态、配色廉价、信息密度高但层次混乱。**ProxyPanel 不应该看起来像它们。**

## Users

### 主要用户：运维者

- 1–3 台 VPS，5–50 个用户。
- 高频但短时使用：每天打开 3–5 次，每次 1–3 分钟，目的明确（加用户、看流量、查告警原因）。
- **典型场景句**：周二下午两点，开发者在 14 寸笔记本上停下手头的代码，打开面板加一个新朋友的订阅、确认昨晚的流量预警是哪台节点引起的，三分钟后回去写代码。
- 已经是 Surge / Mihomo Party / Stash 用户，对「克制、信息密度高但不拥挤、字体讲究」的审美有判断力。

### 次要用户：被服务的终端用户

- 不登录面板，只通过订阅链接消费数据。**不是本设计的目标。**

## Tone & Voice

- **像设置面板，不像驾驶舱。** 不用「实时大屏」「指挥中心」「可视化平台」的视觉语言。
- **静、克制、可读。** 没有不必要的动效、不必要的装饰、不必要的图标背景圆。
- **中文优先，英文为辅。** 专有名词（VLESS、Reality、Surge、Trojan）保持英文不翻译。
- **数字第一，标签第二。** 关键数据用大字号，标签用小字号。但不要做"巨大数字 + 渐变"的 SaaS hero metric 模板。

## Anti-references — 不应该看起来像

| 反面参考 | 为什么不要 |
|---|---|
| x-ui / 3x-ui / Xray-UI | 深色赛博、emoji、廉价饱和色，本项目核心反对的对象 |
| 默认 Element Plus 蓝 + 深色 sidebar 后台 | 训练数据一级反射，AI slop 测试直接挂 |
| Vercel / Linear 复刻 | 二级反射，"avoid SaaS cream → 复刻 Linear" 已是新 cliché |
| Grafana / Datadog 风深色仪表盘 | 二级反射，"observability → dark blue + 大量图表" 套路 |
| 渐变 hero metric 卡片 / 大数字 + 渐变文字 | impeccable 禁止 |

## Positive references — 可以借鉴气质（但不复刻）

- **Surge for Mac 设置面板**：分组清晰、标签–值排版、字体讲究、零装饰。
- **iA Writer 设置**：信息密度高但呼吸感好，单色 + 等宽数字。
- **Things 3 偏好面板**：圆角节制、阴影几乎不用、状态用颜色不用图标背景。
- **macOS Sequoia 系统设置**：左导航 + 右内容的经典两栏，但每行分区有节奏。

## Strategic principles

1. **Tabular over card.** 管理资源（用户、节点、流量记录）天然是表格内容，不要把表格内容塞进卡片网格。卡片只用于真正异质的内容（状态摘要、单一图表、告警 callout）。
2. **One number per region.** 仪表盘上同一区域不出现两个同等大的数字。每块只回答一个问题。
3. **Status as text + color, not as icon-in-circle.** 用 tag/dot/字体颜色表达状态，不要再做 "圆角方块装个图标染个浅色背景"。
4. **Density adapts to task.** 表格视图密集（一屏多行），仪表盘宽松（呼吸 + 节奏），设置页非常宽松（每条带说明）。三种密度区分明显。
5. **Mono digits.** 所有数字（流量、端口、UUID、时间戳）用等宽数字字符（`font-variant-numeric: tabular-nums`），扫读对齐。
6. **No emoji status.** ✅❌🟢 全部去掉，改用 dot/tag。

## Constraints

- 技术栈保持 Vue 3 + Vite。Element Plus 可保留作底层组件库，但视觉壳必须完全替换（自定义 token + override + 必要时自己写组件壳）。
- 中文为主，所有界面文案中文优先。
- 桌面优先（≥ 1280px 是主要场景），平板（≥ 768）完整可用；手机（< 768）为次要但必须可用场景：地铁/咖啡馆里收到流量预警时单手查节点状态、加用户、临时禁用节点。
- 手机端**不复刻**"移动 SaaS 后台"模板（底 tab 栏、卡片网格、巨大数字 + 浅色卡片）。保持桌面 editorial 气质：列表行 + dot + mono 数字 + serif 标题，靠分隔线分区不靠卡片。
- 后端 Go + SQLite，前端是嵌入到二进制的静态产物。包体积要克制（当前 vendor-echarts 已 lazy load，保持）。

## Out of scope

- 不做用户自助门户（PRD v1.2 才考虑）。
- 不做多管理员 / 角色权限（PRD v1.2）。

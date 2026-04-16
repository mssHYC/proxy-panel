# ProxyPanel 产品需求文档 (PRD)

| 项 | 内容 |
|---|---|
| 文档版本 | v1.0 |
| 更新日期 | 2026-04-15 |
| 目标读者 | 产品负责人 / 开发 / 运维 |

---

## 1. 项目背景

当前自建代理生态以 x-ui / 3x-ui / Xray-UI 等面板为主，痛点集中在:

1. **流量管理粗放** —— 大多只支持简单的总额限制,缺少灵活的刷新周期 (按月/周/日/自定义日期) 与服务器级总流量配额。
2. **告警链路缺失** —— 流量超限、节点离线、用户到期等关键事件无法主动推送到运维人员的常用工作群。
3. **多端订阅支持不全** —— 主流面板对 Surge (iOS/macOS 高级用户主力客户端) 的原生格式支持普遍缺位,用户需要自行用 sub-converter 中转。
4. **部署门槛仍高** —— 多数面板需手动配置内核 / Nginx / 证书 / 数据库,新手上手成本高。

ProxyPanel 在以上四个维度做差异化,目标是做"运维友好 + 客户端友好"的自建代理管理面板。

## 2. 产品目标

| 指标 | 目标值 |
|---|---|
| 一键部署完成时间 (干净 VPS → 可访问面板) | ≤ 3 分钟 |
| 新增用户 → 客户端可连接 | ≤ 30 秒 |
| 支持的代理协议 | VLESS / VMess / Trojan / Shadowsocks / Hysteria2 |
| 支持的客户端订阅格式 | Surge / Clash (Mihomo) / V2Ray / Shadowrocket / Sing-box |
| 告警通道 | Telegram / 企业微信 (可扩展飞书、钉钉) |

## 3. 用户画像与场景

### 3.1 主要用户:个人/小团队运维者

- **背景**:1-3 台 VPS,服务 5-50 个用户 (家人、朋友、小团队)。
- **痛点**:
  - 月底忘了重置流量,用户超额或浪费配额。
  - VPS 流量超额被服务商限速,事后才发现。
  - Surge 用户需要手动配置,每次加节点都要发一遍。
- **典型工作流**:
  1. 部署一台新 VPS,用一键脚本在 3 分钟内拉起面板。
  2. 添加 5 个用户,设置每月 100GB 配额、每月 1 号重置。
  3. 配置 Telegram Bot,设置服务器总流量 1000GB,80% 时预警。
  4. 把订阅链接发给用户,用户在 Surge / Clash / Shadowrocket 里直接导入。

### 3.2 次要用户:被服务的终端用户

- **诉求**:订阅链接稳定、客户端兼容性好、能看到自己剩余流量和到期时间。
- **接触面**:订阅链接 (含 `Subscription-Userinfo` 流量信息头),无需登录面板。

## 4. 核心功能模块

### 4.1 用户管理

| 字段 | 说明 |
|---|---|
| 用户名 / UUID / 邮箱 | UUID 作为代理凭证,邮箱用于通知 |
| 协议 | vless / vmess / trojan / ss (单用户绑定一个主协议) |
| 流量配额 | 单位 GB,0 = 无限制 |
| 限速 | 单位 Mbps,0 = 无限制,通过 Xray policy 实现 |
| 刷新周期 | 按月 (指定日期 1-31) / 按周 / 按日 / 自定义 cron |
| 到期时间 | 可选,到期自动停用并通知 |
| 启用/停用 | 手动开关 + 自动 (流量超限/到期) |

**关键操作**:
- 新增 / 编辑 / 删除用户
- 重置单用户流量
- 批量重置 (按刷新日期)
- 用户级别的订阅链接生成

### 4.2 节点管理

| 字段 | 说明 |
|---|---|
| 节点名称 | 显示在订阅中 |
| 主机 / 端口 | 客户端连接地址 |
| 协议 | vless / vmess / trojan / ss / hysteria2 |
| 传输 | tcp / ws / grpc / h2 / reality |
| 内核类型 | xray 或 singbox (决定哪个内核处理流量) |
| 协议特定配置 | TLS / SNI / Reality 公钥 / WS path / Hy2 密码等 (JSON 存储) |

**关键操作**:
- 新增 / 编辑 / 删除节点
- 启用/停用 / 排序
- 节点健康检查 (TCP 连通性 + 协议握手)

### 4.3 流量管理

#### 4.3.1 用户流量

- **采集**:每 60 秒 (可配) 通过 Xray Stats API / Sing-box Clash API 拉取上下行字节数。
- **存储**:
  - `users.traffic_used / traffic_up / traffic_down` 累计值。
  - `traffic_logs` 表存原始增量,用于历史图表。
- **预警阈值**:全局 + 单用户可配,默认 80% 警告、100% 自动停用。

#### 4.3.2 服务器总流量 ⭐ (差异化)

- **场景**:VPS 服务商通常按月给固定流量 (如 RackNerd 1TB/月),超额会被限速或停机。
- **配置**:
  - `server_limit_gb`:总流量限额。
  - `warn_percent`:预警百分比 (默认 80%)。
  - `reset_cron`:与 VPS 计费周期对齐的重置时间。
- **行为**:
  - 达到预警阈值 → 推送告警,标记 `warn_sent` 防重发。
  - 达到限额 → 推送严重告警,可选自动停止内核。
  - 重置时清零并发送重置通知。

#### 4.3.3 刷新周期

四种模式覆盖常见场景:

| 模式 | 配置 | 适用 |
|---|---|---|
| 按月 | `reset_day` = 1-31 | 按月套餐 (主流) |
| 按周 | cron `0 0 * * 0` | 周配额 |
| 按日 | cron `0 0 * * *` | 限时体验 |
| 自定义 | 任意 cron 表达式 | VPS 计费日对齐 |

**用户级别刷新日期独立**:支持不同用户在不同日期重置 (例如订阅日不同)。

### 4.4 订阅链接 ⭐ (差异化)

#### 4.4.1 端点设计

```
GET /api/sub/:uuid?format=surge
```

| format 参数 | 客户端 | 输出格式 |
|---|---|---|
| `surge` | Surge (iOS/macOS) | Surge 配置文件 (`[Proxy]/[Proxy Group]/[Rule]`) |
| `clash` | Clash / Mihomo / Stash | YAML |
| `v2ray` | V2RayN / V2RayNG | base64 编码的 URI 列表 |
| `shadowrocket` | Shadowrocket (iOS) | base64 编码的 URI 列表 |
| `singbox` | Sing-box | JSON outbounds |

#### 4.4.2 Surge 格式细节 ⭐

社区面板普遍痛点。Surge 不直接支持 VLESS,需要:
- VMess / Trojan / SS / Hysteria2 → 原生 Surge 语法。
- VLESS → 标注 `# requires plugin` 并提示用户使用替代节点。
- 自动生成 `[Proxy Group]` (select + url-test)。
- 默认规则:`GEOIP,CN,DIRECT` + `FINAL,Proxy`。

#### 4.4.3 流量信息头

所有格式响应都附带:
```
Subscription-Userinfo: upload=N; download=N; total=N; expire=TIMESTAMP
```
让客户端 (Surge / Shadowrocket) 直接显示剩余流量和到期时间。

### 4.5 告警通知

#### 4.5.1 告警类型

| 类型 | 触发条件 | 默认开关 |
|---|---|---|
| 用户流量预警 | 单用户流量 ≥ 阈值 | 开 |
| 用户流量耗尽 | 单用户流量 ≥ 100% | 开 |
| 服务器流量预警 | 总流量 ≥ 阈值 | 开 |
| 服务器流量耗尽 | 总流量 ≥ 100% | 开 |
| 用户到期前 | 到期前 3 天 | 开 |
| 用户已到期 | 到期当日 | 开 |
| 流量重置完成 | 周期重置触发 | 开 |
| 节点离线 | 健康检查失败 | 开 |

#### 4.5.2 通道

| 通道 | 接入方式 | 配置项 |
|---|---|---|
| Telegram | Bot API | bot_token + chat_id |
| 企业微信 | 群机器人 Webhook | webhook_url |
| (后续) 飞书 / 钉钉 | Webhook | (预留接口) |

#### 4.5.3 防骚扰

- 同类告警在同周期内只发一次 (`warn_sent` 标志)。
- 重置后标志清零。
- 测试通道按钮 (面板上一键发测试消息)。

### 4.6 仪表盘

首屏展示:

- 用户总数 / 活跃用户数
- 在线用户数 (基于最近 5 分钟有流量的用户)
- 节点总数 / 在线节点数
- 服务器总流量 (上行 / 下行 / 已用占比)
- 今日流量
- 近 30 天流量趋势图
- 内核状态 (运行/停止 + 切换按钮)

### 4.7 一键部署 ⭐

#### 4.7.1 一行命令安装

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/<repo>/main/install.sh)
```

#### 4.7.2 脚本能力

| 能力 | 说明 |
|---|---|
| 系统检测 | Ubuntu 20.04+ / Debian 11+ / CentOS 8+ + amd64/arm64 |
| 依赖安装 | curl, jq, sqlite3, systemd |
| 内核下载 | 自动从 GitHub Release 拉取 Xray + Sing-box |
| 面板二进制 | 拉取预编译的 `proxy-panel` 二进制 + 前端静态文件 |
| 配置生成 | 交互式询问端口、管理员密码、TG Bot |
| systemd 服务 | 注册 `proxy-panel.service`,设置开机自启 |
| 防火墙 | 自动放行管理端口 + 节点端口 (ufw / firewalld) |
| TLS / 证书 | 可选:Cloudflare 模式 或 acme.sh 自动签发 |

#### 4.7.3 TLS 证书管理

部署脚本提供两种证书方案,安装时交互选择:

| 方案 | 适用场景 | 工作方式 | 维护成本 |
|---|---|---|---|
| **方案 A: Cloudflare 模式** | 域名已托管 Cloudflare,习惯用 CF Tunnel 或 CDN | 面板走 CF Tunnel (零证书),节点走 CF Origin Certificate (15 年有效) 或 Reality (无需证书) | 几乎为零 |
| **方案 B: acme.sh 自签** | 域名不在 Cloudflare,或需要直连 TLS | 安装 acme.sh,通过 standalone / DNS API 自动签发 Let's Encrypt 证书,cron 自动续期 | 低,全自动 |
| **方案 C: 不使用 TLS** | 纯 Reality 节点 + 内网/IP 直连面板 | 面板监听 HTTP,节点全部用 Reality 或自签 | 零 |

**方案 A 细节 (Cloudflare)**:
- 面板:脚本自动生成 `cloudflared` 配置,一条 Tunnel 暴露面板端口,外部通过 `https://panel.your-domain.com` 访问。
- 节点 (WS+CDN):使用 Cloudflare Origin Certificate,脚本自动下载并存放到 `/opt/proxy-panel/certs/`。
- 节点 (Reality):无需任何证书。
- 所需信息:Cloudflare API Token + Zone ID (脚本交互引导)。

**方案 B 细节 (acme.sh)**:
- 安装 acme.sh 到 `/root/.acme.sh/`。
- 签发方式:优先 standalone (需 80 端口空闲),备选 DNS API (Cloudflare / Aliyun / DNSPod 等)。
- 证书路径:`/opt/proxy-panel/certs/{domain}.{crt,key}`。
- 自动续期:acme.sh 内置 cron,续期后自动 reload 面板 + 内核。
- 所需信息:域名 + 签发方式选择,DNS API 需要额外凭据。

**各协议证书需求总结**:

| 协议 | 需要真实证书? | 推荐方案 |
|---|---|---|
| VLESS + Reality | ❌ 不需要 | 方案 C 即可 |
| VLESS + WS + CDN | ❌ CF 边缘处理 | 方案 A (Origin Cert 回源) |
| VLESS + TLS + Vision | ✅ 需要 | 方案 B |
| Trojan | ✅ 需要 | 方案 B |
| Hysteria2 | ⚠️ 可自签 | 方案 B 或脚本自动生成自签证书 |
| VMess + WS + TLS | ✅ 需要 | 方案 B |
| Shadowsocks | ❌ 不需要 | 方案 C |

#### 4.7.4 子命令

```bash
proxy-panel install    # 安装
proxy-panel update     # 升级 (保留配置和数据)
proxy-panel uninstall  # 卸载
proxy-panel status     # 查看状态
proxy-panel restart    # 重启
proxy-panel reset-pwd  # 重置管理员密码
proxy-panel logs       # 查看日志
```

## 5. 用户流程图

### 5.1 部署流程

```
用户拿到 VPS
   ↓
SSH 进入,运行 install.sh
   ↓
脚本检测系统 + 安装依赖 (30s)
   ↓
下载 Xray + Sing-box + Panel (60s)
   ↓
交互问答: 端口/密码/TG (30s)
   ↓
启动 systemd 服务
   ↓
浏览器访问 http://VPS:8080,登录管理员账户
```

### 5.2 添加用户流程

```
登录面板 → 用户管理 → 新增
   ↓
填写: 用户名 / 协议 / 流量限额 / 刷新日 / 到期时间
   ↓
保存 → 自动生成 UUID
   ↓
点击"复制订阅链接"
   ↓
选择客户端格式 (Surge / Clash / 等)
   ↓
发给用户,用户在客户端导入
```

## 6. 非功能需求

| 项 | 要求 |
|---|---|
| **性能** | 单机支持 ≥ 500 用户 / 50 节点;面板内存占用 < 100MB |
| **可用性** | 面板崩溃不影响代理服务 (内核独立进程) |
| **安全** | JWT 鉴权 / SQLite 文件权限 0600 / 密码 SHA256+salt |
| **可观测** | 结构化日志 (stdout) + 关键事件入库 (`alert_records`) |
| **国际化** | v1 中英双语,默认中文 |
| **数据备份** | SQLite 文件 + 配置文件,提供导出/导入接口 |

## 7. 版本规划

### v1.0 (MVP)
- 用户/节点 CRUD
- Xray 内核集成
- 流量统计与刷新
- 服务器总流量预警
- Surge / Clash / V2Ray / Shadowrocket 订阅
- Telegram + 企微告警
- 一键部署脚本

### v1.1
- Sing-box 内核完整支持 (Reality / Hysteria2 服务端)
- 节点健康检查
- 用户级别细粒度限速 (按时段)
- 飞书 / 钉钉通知

### v1.2
- 多管理员账户 + 角色权限
- 节点流量分发统计
- 用户自助门户 (查看流量、修改密码)
- 数据库迁移到可选 PostgreSQL

### v2.0
- 多服务器纳管 (主从架构)
- 节点自动调度 (按延迟 / 负载)
- 计费模块 (订阅式付费)

## 8. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| Xray Stats API 在某些版本字段变化 | 流量采集失败 | 锁定测试过的 Xray 版本,新版本上线前回归测试 |
| Surge 不支持 VLESS | 高级用户体验受损 | 文档明确说明,推荐 VMess/Trojan 给 Surge 用户 |
| 服务商封锁面板域名 | 面板无法访问 | 支持 IP 直连 + 自定义路径 |
| 用户密码泄露 | 面板被入侵 | 强密码策略 + 登录失败限频 + 可选 IP 白名单 |
| SQLite 在大量用户下性能下降 | 响应变慢 | v1.2 提供 PostgreSQL 选项,文档建议 > 500 用户切换 |

## 9. 验收标准

| 功能 | 验收点 |
|---|---|
| 一键部署 | 干净的 Ubuntu 22.04 上 3 分钟内完成,面板可访问 |
| 用户管理 | 创建用户后 30 秒内客户端可连接 |
| 流量统计 | 统计误差 < 5% (与 vnstat 对比) |
| 订阅 (Surge) | Surge 直接导入,Proxy Group 正确,流量信息显示 |
| 订阅 (Clash) | Clash for Windows / Mihomo Party 直接导入 |
| 告警 | TG/企微 在 60 秒内收到测试消息 |
| 流量重置 | 配置每月 1 号,实际触发误差 < 1 分钟 |

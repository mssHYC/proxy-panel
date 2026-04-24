# ProxyPanel

自建代理管理面板，支持多协议、多客户端订阅、流量管理与告警通知。

## 功能特性

- **多协议支持** — VLESS / VMess / Trojan / Shadowsocks / Hysteria2
- **双内核** — Xray + Sing-box，节点级别自由选择
- **多端订阅** — Surge / Clash (Mihomo) / V2Ray / Shadowrocket / Sing-box 五格式一键生成
- **完整分流规则** — 内置 18 组代理策略 (YouTube/Google/Telegram/OpenAI/ClaudeAI 等)，支持自定义规则完全替换
- **流量管理** — 用户配额 + 服务器总流量配额，灵活的重置周期 (按月/周/日/自定义 cron)
- **告警通知** — Telegram Bot + 企业微信 Webhook，流量预警/耗尽/到期/节点离线
- **用户-节点关联** — 多选节点分配给用户，精细化访问控制
- **账号安全** — bcrypt 密码加密、JWT 鉴权、TOTP 二次验证 (Google Authenticator)、登录限流
- **一键部署** — bash 脚本 3 分钟完成部署，支持三种 TLS 方案

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.22 + Gin + SQLite (WAL) |
| 前端 | Vue 3 + Vite + Element Plus + TailwindCSS + ECharts |
| 内核 | Xray-core + Sing-box |
| 部署 | systemd + bash 脚本 |

## 快速开始

### 一键部署 (Linux VPS)

```bash
curl -fsSL https://raw.githubusercontent.com/mssHYC/proxy-panel/main/scripts/install.sh -o install.sh && bash install.sh install
```

脚本会自动完成：系统检测 → 依赖安装 → 下载内核 → 交互配置 → 启动服务。

### 本地开发

```bash
# 1. 克隆项目
git clone <repo> && cd proxy-panel

# 2. 启动后端
cp config.example.yaml config.yaml
go build -o proxy-panel ./cmd/server/
./proxy-panel -config config.yaml

# 3. 启动前端 (另一个终端)
cd web
npm install
npm run dev

# 4. 访问
# 前端开发: http://localhost:5173
# 后端 API: http://localhost:8080
# 默认账号: admin / admin123
```

### 一体化运行

```bash
cd web && npm run build && cd ..
go build -o proxy-panel ./cmd/server/
./proxy-panel -config config.yaml
# 访问 http://localhost:8080
```

## 项目结构

```
proxy-panel/
├── cmd/server/main.go              # 入口
├── config.example.yaml             # 示例配置
├── internal/
│   ├── config/                     # 配置加载
│   ├── database/                   # SQLite + 迁移
│   ├── model/                      # 数据模型
│   ├── router/                     # 路由 + JWT + 限流
│   ├── handler/                    # HTTP 处理器
│   ├── service/                    # 业务逻辑
│   │   ├── subscription/           # 5 格式订阅生成
│   │   └── notify/                 # Telegram + 企微
│   └── kernel/                     # Xray/Sing-box 抽象
├── web/                            # Vue 3 前端
│   └── src/
│       ├── views/                  # 6 个页面
│       ├── components/             # 通用组件
│       ├── api/                    # API 封装
│       └── stores/                 # Pinia 状态
└── scripts/install.sh              # 一键部署脚本
```

## 功能页面

| 页面 | 功能 |
|---|---|
| 仪表盘 | 用户/节点统计、服务器流量、内核状态、30 天流量趋势图 |
| 用户管理 | 用户 CRUD、节点多选分配、流量配额、订阅链接生成 |
| 节点管理 | 节点 CRUD、协议/传输/安全动态表单 (参照 3x-ui)、证书路径引用 |
| 流量统计 | 服务器总流量监控、限额设置、历史流量图表 |
| 系统设置 | 账号管理 (改密码/用户名)、TOTP 二次验证、通知配置、自定义分流规则 |
| 登录 | JWT 认证、TOTP 二次验证流程 |

## API 端点

### 公开端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | 登录 |
| POST | `/api/auth/2fa/verify` | TOTP 二次验证 |
| GET | `/api/sub/t/:token` | 订阅链接，推荐路径；支持多 token、过期、IP 绑定、UA 自动识别 (`?format=` 仍可手动覆盖) |
| GET | `/api/sub/:uuid` | 旧订阅链接，保留兼容（响应头 `X-Subscription-Deprecated`） |

### 认证端点 (需 JWT)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/dashboard` | 仪表盘 |
| GET/POST | `/api/users` | 用户列表/新增 |
| GET/PUT/DELETE | `/api/users/:id` | 用户详情/编辑/删除 |
| POST | `/api/users/:id/reset-traffic` | 重置流量 |
| POST | `/api/users/:id/reset-uuid` | 重置 UUID |
| GET/POST | `/api/nodes` | 节点列表/新增 |
| PUT/DELETE | `/api/nodes/:id` | 节点编辑/删除 |
| GET | `/api/kernel/status` | 内核状态 |
| POST | `/api/kernel/restart` | 重启内核 |
| GET | `/api/traffic/server` | 服务器流量 |
| POST | `/api/traffic/server/limit` | 设置流量限额 |
| GET | `/api/traffic/history` | 历史流量 |
| GET/PUT | `/api/settings` | 系统设置 |
| POST | `/api/notify/test` | 测试通知 |
| PUT | `/api/auth/password` | 修改密码 |
| PUT | `/api/auth/username` | 修改用户名 |
| GET | `/api/auth/2fa/status` | 2FA 状态 |
| POST | `/api/auth/2fa/setup` | 生成 TOTP 密钥 |
| POST | `/api/auth/2fa/enable` | 启用 2FA |
| POST | `/api/auth/2fa/disable` | 关闭 2FA |

## 分流配置

- **18 个内置规则分类** — Google / YouTube / Telegram / AI 服务 / CN / 流媒体 等，支持用户自定义分类
- **18 个内置出站组** — 🚀 手动切换 / ⚡ 自动选择 / 🎬 流媒体 等，支持用户自定义组，类型可选 selector 或 urltest
- **结构化自定义规则** — site_tags / ip_tags / domain_suffix / domain_keyword / ip_cidr 等字段，替代原有多行文本
- **3 个预设方案** — `minimal` / `balanced` / `comprehensive`；可一键覆盖当前启用分类，或在订阅 URL 上加 `?preset=balanced` 即时覆盖
- **rule-provider URL 前缀可覆写** — Clash / Sing-box 各自独立，方便切 gh-proxy 镜像
- **支持客户端格式** — Clash / Sing-box / Surge / V2Ray / Shadowrocket；V2Ray 用 URI base64（无法承载路由），其他四端消费统一 IR 后按各自语法渲染
- **首次升级自动导入** — 老 `settings.custom_rules` 文本自动导入到结构化规则表

## 部署脚本

```bash
proxy-panel install      # 全新安装
proxy-panel update       # 升级 (保留配置和数据)
proxy-panel uninstall    # 卸载
proxy-panel status       # 查看状态
proxy-panel restart      # 重启服务
proxy-panel logs         # 查看日志
proxy-panel reset-pwd    # 重置管理员密码
proxy-panel backup       # 备份数据
proxy-panel restore FILE # 从备份恢复
```

### TLS 证书方案

| 方案 | 适用场景 | 要求 |
|---|---|---|
| HTTP 验证 (standalone) | 简单直连，无需 API Key | 80 端口空闲 + 域名 A 记录 |
| Cloudflare DNS API | 支持通配符，兼容 CDN 橙色云朵 | CF 托管域名 + API Token |
| DNSPod DNS API | 国内腾讯云用户 | DNSPod 托管域名 + API Token |
| Aliyun DNS API | 阿里云用户 | 阿里云托管域名 + AccessKey |
| 自定义证书 | 已有证书 (商业/其他工具申请) | .crt + .key 文件 |
| 不使用 TLS | 纯 Reality 节点 + IP 直连面板 | 无 |

## 配置文件

参考 [config.example.yaml](config.example.yaml)，主要配置项：

```yaml
server:
  port: 8080          # 面板端口
  tls: false          # 是否启用 TLS

auth:
  admin_user: admin   # 管理员用户名
  admin_pass: admin123 # 管理员密码 (支持 bcrypt hash)
  jwt_secret: xxx     # JWT 签名密钥

traffic:
  server_limit_gb: 1000  # 服务器总流量限额 (GB)
  warn_percent: 80       # 预警阈值 (%)
  collect_interval_sec: 60 # 流量采集间隔 (秒)

notify:
  telegram:
    enable: false
    bot_token: ""
    chat_id: ""
```

## 版本规划

| 版本 | 状态 | 关键能力 |
|---|---|---|
| v1.0 | 已完成 | 用户/节点管理、Xray 集成、5 格式订阅、流量管理、告警通知、2FA |
| v1.1 | 规划中 | Sing-box 完整支持、节点健康检查、飞书/钉钉通知 |
| v1.2 | 规划中 | 多管理员、用户自助门户、PostgreSQL 选项 |
| v2.0 | 规划中 | 多服务器纳管、节点调度、计费模块 |

## License

MIT

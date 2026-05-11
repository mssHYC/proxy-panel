# ProxyPanel

轻量、自托管的代理节点管理面板。一个二进制 + 一份配置，覆盖用户、节点、订阅、流量、告警的完整生命周期。

## 特性

- **多协议** — VLESS / VMess / Trojan / Shadowsocks / Hysteria2，Reality / XTLS / WS / gRPC 传输齐备
- **双内核** — Xray 与 Sing-box 并存，按节点切换
- **多端订阅** — Surge / Clash (Mihomo) / Sing-box / V2Ray / Shadowrocket，UA 自动识别，URL 可强制覆盖
- **结构化分流** — 18 组内置策略 + 自定义规则（site_tags / ip_cidr / domain_suffix 等字段），三套预设一键切换
- **流量治理** — 用户配额 + 服务器总额，按月/周/日/cron 周期重置，预警与耗尽自动停发
- **节点编排** — 节点分组、套餐订阅、健康检查（含 Hysteria2 QUIC 探测）
- **告警** — Telegram / 企业微信 Webhook，节点离线、流量预警、到期提醒
- **安全** — bcrypt + JWT + TOTP（Google Authenticator）+ 登录限流 + 审计日志
- **部署** — bash 脚本三分钟落地，或 `docker compose up -d`

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go 1.25 · Gin · SQLite (WAL) · cron · Prometheus |
| 前端 | Vue 3 · Vite · TailwindCSS v4 · Reka UI · ECharts · Pinia |
| 内核 | Xray-core · Sing-box |
| 部署 | systemd · Docker · bash |

## 快速开始

### 一键脚本（Linux VPS）

```bash
curl -fsSL https://raw.githubusercontent.com/mssHYC/proxy-panel/main/scripts/install.sh -o install.sh && bash install.sh install
```

系统检测 → 依赖安装 → 内核下载 → 交互配置 → 服务启动，全程自动。

### Docker

```bash
docker compose up -d
# 访问 http://<host>:8080
```

### 本地开发

```bash
# 后端
cp config.example.yaml config.yaml
go run ./cmd/server -config config.yaml

# 前端（另一个终端）
cd web && npm install && npm run dev
```

默认账号 `admin / admin123`，前端开发服务 `http://localhost:5173`，后端 `http://localhost:8080`。

### 一体化构建

```bash
cd web && npm run build && cd ..
go build -o proxy-panel ./cmd/server
./proxy-panel -config config.yaml
```

## 项目结构

```
proxy-panel/
├── cmd/server/         入口
├── internal/
│   ├── config/         配置加载
│   ├── database/       SQLite + 迁移
│   ├── model/          数据模型
│   ├── router/         路由 / JWT / 限流
│   ├── handler/        HTTP 处理器
│   ├── service/        业务逻辑（订阅、通知、健康检查等）
│   └── kernel/         Xray / Sing-box 抽象
├── web/                Vue 3 前端
├── scripts/install.sh  一键部署脚本
├── Dockerfile
└── docker-compose.yml
```

## 面板页面

| 页面 | 能力 |
|---|---|
| 仪表盘 | 用户/节点统计、服务器流量、内核状态、30 天趋势 |
| 用户管理 | CRUD、套餐分配、节点绑定、配额、订阅链接 |
| 节点管理 | 协议/传输/安全动态表单（参照 3x-ui）、证书引用、健康状态 |
| 节点分组 | 多节点编排，套餐与分组关联 |
| 套餐 | 流量配额、周期、绑定节点分组 |
| 流量统计 | 服务器总流量、限额、历史曲线 |
| 系统设置 | 账号、TOTP、通知、分流规则、订阅模板 |
| 审计日志 | 关键操作可追溯 |
| 登录 | JWT + TOTP 二步验证 |

## 订阅与分流

- **5 种客户端格式** — UA 自动识别，URL `?format=` 可手动覆盖
- **推荐路径** `/api/sub/t/:token` — 多 token、过期、IP 绑定、UA 嗅探
- **兼容路径** `/api/sub/:uuid` — 保留旧链接，响应头携带 `X-Subscription-Deprecated`
- **预设方案** `minimal` / `balanced` / `comprehensive`，URL `?preset=` 即时切换
- **rule-provider 前缀可覆写** — Clash 与 Sing-box 各自独立，便于切换 gh-proxy 镜像
- **结构化规则** — `site_tags` / `ip_tags` / `domain_suffix` / `domain_keyword` / `ip_cidr`；老文本规则首次升级自动迁移

## 部署脚本子命令

```
install     全新安装
update      升级（保留配置与数据）
uninstall   卸载
status      查看状态
restart     重启服务
logs        查看日志
reset-pwd   重置管理员密码
backup      备份数据
restore F   从备份恢复
```

### TLS 方案

| 方案 | 场景 | 要求 |
|---|---|---|
| HTTP standalone | 简单直连 | 80 端口空闲 + A 记录 |
| Cloudflare DNS | 通配符 / 橙色云朵 | CF 托管 + API Token |
| DNSPod / Aliyun DNS | 国内云用户 | 对应 API 凭证 |
| 自定义证书 | 已有 .crt/.key | 文件路径 |
| 不启用 | 纯 Reality + IP 直连面板 | — |

## 配置文件

参考 [config.example.yaml](config.example.yaml)：

```yaml
server:
  port: 8080
  tls: false

auth:
  admin_user: admin
  admin_pass: admin123     # 支持 bcrypt hash
  jwt_secret: change-me

traffic:
  server_limit_gb: 1000
  warn_percent: 80
  collect_interval_sec: 60

notify:
  telegram:
    enable: false
    bot_token: ""
    chat_id: ""
```

## License

MIT

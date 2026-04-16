# ProxyPanel 技术设计文档

| 项 | 内容 |
|---|---|
| 文档版本 | v1.0 |
| 更新日期 | 2026-04-15 |
| 配套文档 | PRD.md |

---

## 1. 技术栈选型

| 层 | 技术 | 选型理由 |
|---|---|---|
| 代理内核 | Xray-core + Sing-box (双内核) | Xray 生态成熟,Sing-box 对 Reality/Hy2 支持更好;由用户在节点级别选择 |
| 后端语言 | Go 1.22 | 与内核同语言,部署简单 (单二进制),并发性能好 |
| Web 框架 | Gin | 成熟稳定,中间件生态丰富 |
| 数据库 | SQLite (WAL) | 单文件部署,< 500 用户性能足够;可平滑迁移 PostgreSQL |
| 调度 | robfig/cron v3 | 标准 cron 表达式,API 简单 |
| 鉴权 | JWT (HS256) | 无状态,适合前后端分离 |
| 前端框架 | Vue 3 + Vite + Pinia | 用户已有 sevenKitchen 项目栈,直接复用 |
| UI 组件 | Element Plus + TailwindCSS | 后台快速搭建 + 自定义样式空间 |
| 部署 | systemd + bash 脚本 | 不依赖 Docker,轻量,易调试 |

## 2. 系统架构

### 2.1 进程模型

```
┌─────────────────────────────────────────────┐
│  systemd                                     │
│  ├── proxy-panel.service  (Go 后端 + 静态)   │
│  ├── xray.service         (代理内核)         │
│  └── sing-box.service     (代理内核, 可选)   │
└─────────────────────────────────────────────┘
```

**关键决策**:面板与内核**独立进程**。面板崩溃不影响代理服务;内核重启由面板通过 systemd / 进程管理触发,不直接 fork。

### 2.2 模块依赖

```
cmd/server/main.go
   ↓
internal/router            ← HTTP 入口 (Gin)
   ↓
internal/service/{user, traffic, subscription, notify, scheduler}
   ↓                                          ↓
internal/kernel (Xray + Sing-box 抽象)    internal/database (SQLite)
   ↓
内核进程 (子进程或 systemd)
```

### 2.3 数据流

#### 流量采集流
```
[Scheduler 每 60s]
   → kernel.Active().GetTrafficStats()
   → 解析 (email, up, down)
   → user.UpdateTraffic() + traffic.RecordLog()
   → traffic.UpdateServerTraffic()
   → 阈值判断 → notify.SendAll()
```

#### 订阅请求流
```
GET /api/sub/:uuid?format=surge
   → users.GetByUUID() (校验启用 + 流量)
   → db.Query("SELECT FROM nodes WHERE enable=1")
   → subscription.ToSurge(nodes, user)
   → 返回 + Subscription-Userinfo header
```

## 3. 数据模型

### 3.1 ER 图概览

```
users  ─┐
        │
        ├─< traffic_logs >─ nodes
        │
inbounds (内核入站配置, 1:1 与 nodes)

server_traffic  (单行, 服务器总流量)
alert_records   (告警历史)
settings        (kv 配置)
```

### 3.2 表结构 (核心)

#### users
| 字段 | 类型 | 说明 |
|---|---|---|
| id | INTEGER PK | 自增 |
| uuid | TEXT UNIQUE | 代理凭证 (vless id / trojan password) |
| username | TEXT UNIQUE | 显示名 |
| password | TEXT | SHA256 hash (用户登录用,可选) |
| traffic_limit | INTEGER | 字节,0 = 无限制 |
| traffic_used | INTEGER | 累计已用字节 |
| traffic_up / traffic_down | INTEGER | 上下行分别累计 |
| speed_limit | INTEGER | 字节/秒 |
| reset_day | INTEGER | 1-31,月内重置日 |
| reset_cron | TEXT | 自定义 cron,空时用 reset_day |
| enable | INTEGER | 0/1 |
| expires_at | DATETIME | 到期时间 |
| protocol | TEXT | vless/vmess/trojan/ss |

#### server_traffic (单行)
| 字段 | 说明 |
|---|---|
| total_up / total_down | 服务器累计上下行字节 |
| limit_bytes | 总流量限额 |
| warn_sent / limit_sent | 告警去重标志 |
| reset_at | 上次重置时间 |

#### nodes
| 字段 | 说明 |
|---|---|
| name | 显示名 |
| host / port | 客户端连接地址 |
| protocol | vless/vmess/trojan/ss/hysteria2 |
| transport | tcp/ws/grpc/h2/reality |
| kernel_type | xray / singbox |
| settings | JSON,协议特定参数 |

#### traffic_logs (高频写入)
| 字段 | 说明 |
|---|---|
| user_id, node_id | 外键 |
| upload, download | 增量字节 |
| timestamp | 索引,用于历史聚合 |

**索引**:`(user_id, timestamp)`、`(timestamp)` 单独建索引以加速时间范围查询。

**保留策略**:7 天内逐条保留,7-90 天按日聚合,> 90 天清理。

## 4. 内核抽象层

### 4.1 接口定义

```go
type Engine interface {
    Name() string
    Start() / Stop() / Restart() / IsRunning() error/bool
    GetTrafficStats() (map[string]*UserTraffic, error)
    AddUser / RemoveUser (tag, uuid, email, protocol) error
    GenerateConfig(inbounds, users) ([]byte, error)
    WriteConfig(data) error
}
```

实现:`XrayEngine`、`SingboxEngine`,统一由 `Manager` 协调。

### 4.2 Xray 集成

**配置生成**:
- `policy.levels.0.statsUserUplink/Downlink = true` 启用用户级流量统计
- API inbound (`dokodemo-door`,监听 127.0.0.1:10085) 暴露 Stats 服务
- 路由规则将 API tag 流量导到 API outbound

**流量采集**:
```bash
xray api statsquery --server=127.0.0.1:10085 -pattern "user>>>"
```
返回 `user>>>{email}>>>traffic>>>{uplink|downlink}` 格式,解析为 `map[email]UserTraffic`。

**热加载用户**:
```bash
xray api adi  # add inbound user
xray api rmi  # remove inbound user
```
避免重启内核断开存量连接。

### 4.3 Sing-box 集成

**配置生成**:JSON 格式,所有用户在 inbound `users` 数组中。

**流量采集**:通过 `experimental.clash_api` 暴露的 HTTP API (`/connections`、`/traffic`),按 user tag 聚合。

**用户变更**:Sing-box 不支持热加载,需要重启 (服务中断 ~1s)。**优化方案**:批量处理用户变更,延迟 5s 后统一 reload。

### 4.4 内核选择策略

| 协议 | 推荐内核 | 原因 |
|---|---|---|
| VLESS Reality | Xray | 用户级流量统计成熟 |
| VLESS WS+TLS | Xray | 同上 |
| Trojan | Xray / Sing-box | 等价 |
| Hysteria2 | Sing-box | 实现更新更活跃 |
| TUIC | Sing-box | Xray 不支持 |

节点配置时由用户选择 `kernel_type`,Manager 根据节点配置决定加载哪个内核。

## 5. 订阅协议

### 5.1 Surge 格式细节

```ini
#!MANAGED-CONFIG https://sub.example.com/sub interval=86400 strict=false

[General]
loglevel = notify
skip-proxy = 127.0.0.1, 192.168.0.0/16, ...
dns-server = 8.8.8.8, 1.1.1.1

[Proxy]
DIRECT = direct
NodeName = vmess, host, port, username=UUID, tls=true, sni=..., ws=true, ws-path=/path

[Proxy Group]
Proxy = select, NodeA, NodeB, DIRECT
Auto = url-test, NodeA, NodeB, url=http://www.gstatic.com/generate_204, interval=300

[Rule]
GEOIP,CN,DIRECT
FINAL,Proxy
```

**协议映射**:

| 内部协议 | Surge 行 |
|---|---|
| vmess | `name = vmess, host, port, username=UUID, tls=true, sni=..., ws=true, ws-path=...` |
| trojan | `name = trojan, host, port, password=UUID, sni=...` |
| ss | `name = ss, host, port, encrypt-method=..., password=...` |
| hysteria2 | `name = hysteria2, host, port, password=..., download-bandwidth=...` |
| vless | 注释行 + 提示 (Surge 不原生支持) |

### 5.2 Clash (Mihomo) 格式

YAML 输出,`proxies` + `proxy-groups` + `rules` 三段式。VLESS Reality 通过 `reality-opts` 字段表达。

### 5.3 V2Ray / Shadowrocket

base64 编码的 URI 列表,每行一个节点。URI 按官方规范:
- `vless://uuid@host:port?type=...&security=...&pbk=...&sid=...#name`
- `vmess://base64({v,ps,add,port,id,aid,net,type,tls,sni,path,host})`
- `trojan://password@host:port?sni=...#name`
- `ss://base64(method:password)@host:port#name`
- `hysteria2://password@host:port?sni=...&obfs=...#name`

### 5.4 Subscription-Userinfo Header

所有响应附带:
```
Subscription-Userinfo: upload=N; download=N; total=N; expire=TIMESTAMP
```

Surge / Shadowrocket / Clash 解析后展示用户的剩余流量和到期时间。

## 6. API 设计

### 6.1 鉴权

- `POST /api/auth/login` → 返回 JWT (24h 有效)
- 其他 `/api/*` 需要 `Authorization: Bearer {token}`
- 订阅 `/api/sub/:uuid` 不需要鉴权 (UUID 即凭证)

### 6.2 主要端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/dashboard | 仪表盘汇总数据 |
| GET/POST | /api/users | 列表 / 新增 |
| GET/PUT/DELETE | /api/users/:id | 单用户操作 |
| POST | /api/users/:id/reset-traffic | 重置单用户流量 |
| GET/POST | /api/nodes | 节点列表 / 新增 |
| PUT/DELETE | /api/nodes/:id | 节点编辑 / 删除 |
| GET | /api/kernel/status | 内核状态 |
| POST | /api/kernel/restart | 重启内核 |
| POST | /api/kernel/switch | 切换 xray/singbox |
| GET | /api/traffic/server | 服务器总流量 |
| POST | /api/traffic/server/limit | 设置总流量限额 |
| GET | /api/traffic/history | 历史流量图表数据 |
| GET/PUT | /api/settings | 配置读写 |
| POST | /api/notify/test | 测试告警通道 |
| GET | /api/sub/:uuid | 订阅链接 (公开) |

### 6.3 错误响应统一格式

```json
{ "error": "human readable message", "code": "ERR_CODE" }
```

### 6.4 速率限制

- 登录端点:同 IP 5 次/分钟,失败后指数退避。
- 订阅端点:单 UUID 30 次/分钟。
- 其他端点:无限制 (面板自用)。

## 7. 调度器设计

### 7.1 任务列表

| 任务 | 触发 | 行为 |
|---|---|---|
| 流量采集 | 每 60s (可配) | 从内核拉取增量,落库,触发阈值检查 |
| 全局流量重置 | `traffic.reset_cron` | 重置 `users.traffic_used` + `server_traffic` |
| 用户级重置 | 每天 00:00 | 按 `reset_day = 今天` 批量重置 |
| 用户到期检查 | 每天 08:00 | 到期停用 + 即将到期 (< 3 天) 通知 |
| 节点健康检查 | 每 5 分钟 (v1.1) | TCP 探测 + 协议握手,失败通知 |

### 7.2 防雪崩

- 流量采集失败时,记录日志但不中断后续轮询。
- 告警发送失败重试 3 次,间隔 5/10/30 秒,失败后落 `alert_records` 标记 `failed`。

## 8. 一键部署脚本设计

### 8.1 脚本流程

```
install.sh main()
   ├── check_root
   ├── detect_os         (识别 Ubuntu/Debian/CentOS)
   ├── detect_arch       (amd64/arm64/armv7)
   ├── install_deps      (curl jq sqlite3)
   ├── download_xray     (从 GitHub Release)
   ├── download_singbox  (从 GitHub Release)
   ├── download_panel    (二进制 + 静态文件)
   ├── interactive_config
   │     - 询问端口 (默认 8080)
   │     - 询问管理员密码
   │     - 询问 TLS 方案 (Cloudflare / acme.sh / 无)
   │     - 询问 TG Bot Token (可选)
   │     - 询问总流量限额 (默认 1000GB)
   ├── setup_tls          (按选择执行证书方案)
   ├── generate_config_yaml
   ├── init_database     (运行迁移)
   ├── setup_systemd     (写入 .service 文件)
   ├── setup_firewall    (放行端口)
   ├── start_services
   └── print_summary     (访问地址 + 凭证)
```

### 8.2 子命令

```bash
proxy-panel install      # 全新安装
proxy-panel update       # 拉取新版本,保留 config.yaml + panel.db
proxy-panel uninstall    # 卸载,可选保留数据
proxy-panel status       # systemctl status + 关键指标
proxy-panel restart      # systemctl restart
proxy-panel logs [-f]    # journalctl -u proxy-panel
proxy-panel reset-pwd    # 修改 config.yaml 中的 admin password
proxy-panel backup       # 打包 config.yaml + panel.db 到 tar.gz
proxy-panel restore FILE # 从备份恢复
proxy-panel cert status  # 查看证书状态 (域名/到期时间)
proxy-panel cert renew   # 手动触发续期
proxy-panel cert switch  # 重新选择证书方案
```

### 8.3 文件布局

```
/opt/proxy-panel/
├── proxy-panel              # 主二进制 (软链到 /usr/local/bin/)
├── config.yaml              # 配置
├── data/
│   └── panel.db             # SQLite 数据
├── certs/                   # TLS 证书
│   ├── {domain}.crt         # 公钥 (acme.sh 签发或 CF Origin)
│   └── {domain}.key         # 私钥
├── kernel/
│   ├── xray.json            # 自动生成的 Xray 配置
│   └── singbox.json         # 自动生成的 Sing-box 配置
└── web/                     # 前端静态文件
    ├── index.html
    └── assets/
```

### 8.4 systemd 服务

```ini
[Unit]
Description=ProxyPanel
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/proxy-panel
ExecStart=/opt/proxy-panel/proxy-panel -config /opt/proxy-panel/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 8.5 TLS 证书管理

脚本通过交互式菜单让用户选择证书方案,选择后自动完成对应流程。

#### 8.5.1 方案 A: Cloudflare 模式

面板通过 Cloudflare Tunnel 暴露,节点使用 Origin Certificate 或 Reality (无需证书)。

**脚本流程**:

```
setup_tls_cloudflare()
   ├── check_cloudflared_installed || install_cloudflared
   ├── prompt: Cloudflare API Token
   ├── prompt: Tunnel 域名 (如 panel.example.com)
   ├── cloudflared tunnel create proxy-panel
   ├── write /etc/cloudflared/config.yml
   │     ingress:
   │       - hostname: panel.example.com
   │         service: http://localhost:8080
   │       - service: http_status:404
   ├── cloudflared tunnel route dns proxy-panel panel.example.com
   ├── systemctl enable --now cloudflared
   └── (可选) 下载 Origin Certificate 到 /opt/proxy-panel/certs/
         - 用于 WS+CDN 回源节点
         - 15 年有效,基本免维护
```

**config.yaml 对应配置**:
```yaml
server:
  tls: false  # Tunnel 模式下后端不需要 TLS
  # 或 WS+CDN 节点回源:
  # cert: /opt/proxy-panel/certs/origin.crt
  # key: /opt/proxy-panel/certs/origin.key
```

#### 8.5.2 方案 B: acme.sh 自动签发

适用于不走 Cloudflare 的场景,通过 Let's Encrypt 获取免费证书。

**脚本流程**:

```
setup_tls_acme()
   ├── prompt: 域名 (如 panel.example.com)
   ├── prompt: 签发方式
   │     [1] standalone (需要 80 端口空闲)
   │     [2] DNS API - Cloudflare
   │     [3] DNS API - Aliyun
   │     [4] DNS API - DNSPod
   │     [5] DNS API - 其他 (手动输入)
   ├── install_acme_sh (如未安装)
   │     curl https://get.acme.sh | sh -s email=your@email.com
   ├── 根据签发方式执行:
   │     standalone: acme.sh --issue -d $DOMAIN --standalone
   │     dns_cf:     CF_Token=xxx acme.sh --issue -d $DOMAIN --dns dns_cf
   │     dns_ali:    Ali_Key=xxx Ali_Secret=xxx acme.sh --issue --dns dns_ali
   │     dns_dp:     DP_Id=xxx DP_Key=xxx acme.sh --issue --dns dns_dp
   ├── acme.sh --install-cert -d $DOMAIN \
   │     --key-file  /opt/proxy-panel/certs/$DOMAIN.key \
   │     --fullchain-file /opt/proxy-panel/certs/$DOMAIN.crt \
   │     --reloadcmd "systemctl reload proxy-panel && systemctl reload xray"
   └── 验证证书文件存在 + 权限设置 (0600)
```

**config.yaml 对应配置**:
```yaml
server:
  tls: true
  cert: /opt/proxy-panel/certs/panel.example.com.crt
  key: /opt/proxy-panel/certs/panel.example.com.key
```

**自动续期**:acme.sh 安装时自动注册 cron job (`0 0 * * * /root/.acme.sh/acme.sh --cron`),续期成功后通过 `--reloadcmd` 自动 reload 面板和内核,全程无需人工介入。

#### 8.5.3 方案 C: 不使用 TLS

纯 Reality 节点场景,面板通过 IP 直连 HTTP 访问。

```yaml
server:
  tls: false
```

脚本跳过所有证书步骤。

#### 8.5.4 Hysteria2 自签证书

如果用户配置了 Hysteria2 节点但未选择方案 B,脚本自动生成自签证书:

```bash
openssl ecparam -genkey -name prime256v1 -out /opt/proxy-panel/certs/hy2.key
openssl req -new -x509 -days 3650 -key /opt/proxy-panel/certs/hy2.key \
    -out /opt/proxy-panel/certs/hy2.crt -subj "/CN=www.example.com"
```

客户端侧配置 `skip-cert-verify: true` 或 `insecure: true`。

#### 8.5.5 证书子命令

```bash
proxy-panel cert status   # 查看当前证书信息 (颁发者/域名/到期时间)
proxy-panel cert renew    # 手动触发续期 (acme.sh 方案)
proxy-panel cert switch   # 切换证书方案 (重新走交互流程)
```

#### 8.5.6 证书路径约定

| 文件 | 路径 | 来源 |
|---|---|---|
| 面板 TLS 证书 | `/opt/proxy-panel/certs/{domain}.crt` | acme.sh 或 CF Origin |
| 面板 TLS 私钥 | `/opt/proxy-panel/certs/{domain}.key` | 同上 |
| Hy2 自签证书 | `/opt/proxy-panel/certs/hy2.crt` | openssl 生成 |
| Hy2 自签私钥 | `/opt/proxy-panel/certs/hy2.key` | 同上 |
| CF Tunnel 凭证 | `/etc/cloudflared/{tunnel-id}.json` | cloudflared 生成 |

内核配置生成器在构建 Xray/Sing-box 配置时,根据节点的 `protocol + transport` 自动引用正确的证书路径,无需用户手动指定。

## 9. 前端架构

### 9.1 项目结构

```
web/
├── src/
│   ├── api/             # axios 封装,按模块分文件 (user.ts / node.ts ...)
│   ├── stores/          # Pinia (auth, dashboard)
│   ├── views/
│   │   ├── Login.vue
│   │   ├── Dashboard.vue
│   │   ├── Users.vue
│   │   ├── Nodes.vue
│   │   ├── Traffic.vue
│   │   └── Settings.vue
│   ├── components/      # 通用组件
│   ├── router/
│   └── main.ts
├── vite.config.ts
└── package.json
```

### 9.2 关键交互

- **登录**:JWT 存 localStorage,axios 拦截器自动加 Authorization header,401 自动跳登录。
- **订阅链接展示**:点击用户行 → 展开二维码 + 5 个客户端的复制按钮。
- **流量图表**:Echarts 渲染近 30 天流量,支持按用户/节点筛选。
- **告警测试**:Settings 页面"发送测试消息"按钮直接调 `/api/notify/test`。

### 9.3 构建产物

`vite build` 输出到 `dist/`,部署时复制到 `/opt/proxy-panel/web/`。Go 后端通过 `engine.Static` 提供静态文件。

## 10. 安全设计

| 风险 | 对策 |
|---|---|
| 管理员密码弱 | 部署脚本强制 ≥ 8 位,首次登录提示修改 |
| JWT 密钥泄露 | 部署时生成 32 字节随机字符串,写入 config.yaml (0600) |
| 数据库被读取 | SQLite 文件 0600,部署在 /opt/proxy-panel/data |
| 暴力登录 | 同 IP 5 次/分钟限流,失败超限锁定 30 分钟 |
| 订阅链接泄露 | UUID v4 (122 bit 熵),用户怀疑泄露可一键重置 UUID |
| HTTPS | 部署脚本可选三种方案:CF Tunnel (零证书) / acme.sh (自动续期) / 无 TLS;证书文件 0600 权限 |
| 证书私钥保护 | 私钥文件 0600,仅 root 可读;acme.sh 续期后自动 reload,不暴露到日志 |
| 内核进程权限 | systemd 用专用低权限用户运行 (v1.1) |
| API 注入 | 所有 SQL 用参数化查询,JSON 字段反序列化用结构体白名单 |

## 11. 性能与可观测

### 11.1 性能基线

| 场景 | 目标 |
|---|---|
| 面板内存占用 | < 100MB (500 用户) |
| 流量采集 1 次耗时 | < 500ms (500 用户) |
| 订阅生成 1 次耗时 | < 100ms (50 节点) |
| 数据库 QPS | < 50 (写) / 200 (读) |

### 11.2 日志

- 结构化 (key=value 或 JSON 格式) 输出到 stdout。
- systemd journald 采集,通过 `proxy-panel logs` 查看。
- 关键事件 (告警发送、配置变更、用户停用) 同时落 `alert_records`。

### 11.3 监控 (可选 v1.1)

- 暴露 `/metrics` (Prometheus 格式):用户数、流量字节、API QPS、告警发送数。

## 12. 测试策略

| 层 | 工具 | 覆盖 |
|---|---|---|
| 单元测试 | go test | 订阅生成 / 流量计算 / cron 解析 |
| 集成测试 | testcontainers + docker | 启动 Xray + 调用 Stats API |
| 端到端 | Playwright | 登录 → 创建用户 → 订阅可用 |
| 部署测试 | Vagrant + Ubuntu 22.04 | install.sh 全流程 |

## 13. 后续演进

| 版本 | 关键能力 |
|---|---|
| v1.1 | 节点健康检查 / Sing-box 完整支持 / 飞书钉钉 / 多管理员 |
| v1.2 | 用户自助门户 / PostgreSQL 选项 / 操作审计日志 |
| v2.0 | 多服务器纳管 (主从) / 节点调度 / 计费模块 |

## 14. 关键决策记录 (ADR)

### ADR-001: 为什么不用 Docker
- **决策**:使用裸机 systemd 部署。
- **理由**:目标用户多为单 VPS 用户,Docker 增加 200MB+ 内存开销和学习成本。Go 单二进制天然适合裸机。
- **代价**:不同发行版需要适配 (脚本中已处理)。

### ADR-002: 为什么用 SQLite 而非 PostgreSQL
- **决策**:默认 SQLite,v1.2 提供 PG 选项。
- **理由**:< 500 用户场景下 SQLite 性能足够,WAL 模式支持并发读;部署零依赖。
- **触发切换**:用户数 > 500 或写 QPS > 50。

### ADR-003: 为什么双内核而非只用 Xray
- **决策**:Xray + Sing-box 共存,节点级别选择。
- **理由**:Hy2 / TUIC 在 Sing-box 上实现质量更高;Reality 在 Xray 上更成熟。给用户选择权。
- **代价**:配置生成和流量采集需要适配两套接口,代码复杂度上升。

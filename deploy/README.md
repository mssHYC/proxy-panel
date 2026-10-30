# 管理接口隔离与 443 静态网站

本次配置改变需要先阅读迁移说明。网站用于让正常浏览器访问得到完整页面，不会恢复已被拦截的 TCP/IP，也不能保证不被识别或封禁。

## 升级前

1. 备份数据库、config.yaml 和内核配置，保留 SSH 会话及云厂商控制台。先在备用实例验证。
2. **未设置 `server.listen` 的旧配置沿用原来的全接口监听（`0.0.0.0`）；已设置的地址保持不变。** 兼容逻辑在程序读取配置时执行，因此安装脚本升级和直接替换二进制都适用，不会改写配置文件。新安装仍显式设置 `127.0.0.1`。若 v1.19.0 升级后面板失联，可通过 SSH 在原 `server` 段补上 `listen: "0.0.0.0"` 并重启面板恢复原入口；保留原端口、TLS 及域名设置。需要收紧旧公网入口时，先配置独立的 HTTPS 反代及访问限制，再显式改为回环监听。
3. `/metrics` 默认返回 404。需要采集时设置独立的 `server.metrics_token`（例如 `openssl rand -hex 32`），采集请求带 `Authorization: Bearer <令牌>`。域名限制仍然生效。
4. 本版本的 sing-box 生成配置要求 **1.12 或更新版本**（现代 DNS / route action 配置）。升级核心后先执行 `sing-box check -c <配置路径>`。Xray 配置用 `xray run -test -config <配置路径>` 检查。
5. 代理出站默认拒绝回环、内网、链路本地、组播和本机网卡地址，包括访问这些地址的域名。这会影响通过代理访问 NAS / 内网服务的旧用法。接口地址在生成配置时读取，网卡地址变化后需重新同步；云 NAT 的外部映射地址不一定在本机网卡上，须额外在防火墙限制管理端口。
6. Xray 普通 TLS 默认拒绝未知或空 SNI，旧客户端应填写证书覆盖的域名；仅填 IP 且不发送 SNI 的 TLS 连接需要修改。已存在的不兼容配置（例如 WS + Vision）会报错，需修正后同步内核。启用 Vision 或改变 ALPN 后，重新获取客户端订阅。现有热添加接口无法传递 Flow，向 Vision 节点添加用户会触发完整配置同步并重启内核，活动连接可能短暂中断。

## 管理入口

新安装默认只监听 `127.0.0.1:8080`，从自己的电脑运行：

```sh
ssh -N -L 8080:127.0.0.1:8080 root@你的服务器
```

HTTP 模式在浏览器打开 `http://127.0.0.1:8080`。启用了 TLS 或 `server.domain` 时，需要使用证书对应域名并将该域名在本机解析到回环地址；Host 限制不能代替访问鉴权。

需要长期公网订阅时，使用单独的 HTTPS 域名/入口，反代只放行所需订阅路径，其他面板路径限制管理员来源或使用 VPN。不要把面板页面、`/api` 或 `/metrics` 接到下面的 443 网站回落上。已有防火墙放行规则不会自动删除，应在确认新访问方式后关闭不再需要的管理端口；保留 SSH。

Xray API 继续监听回环，保留用户热更新所需的 HandlerService；代理出站拒绝访问管理网段及本机地址。sing-box 的 Clash API 增加随机 Bearer secret，面板采集携带该令牌，核心配置文件保留 0600 权限。出站规则是防护的一层，不替代主机/云防火墙和进程隔离，尤其不能把它当作对所有 DNS 重绑定、NAT64 或其他网络转换的完整防御。

Docker：容器内配置 `server.listen: "0.0.0.0"`，Compose 在宿主机仅发布 `127.0.0.1:8080`。不要把容器内回环监听与宿主机回环端口映射混淆。

## 在 TCP 443 上共用网站和代理

### HTTPUpgrade 节点：管理脚本自动部署

对于 **Xray / VLESS 或 VMess / HTTPUpgrade / TLS / 443** 节点，无需手工安装 Nginx、写反代或修改数据库。同步新版管理脚本后执行：

```bash
proxy-panel website setup
# 额外提供网站域名时，该域名也必须被节点证书覆盖：
proxy-panel website setup site.example.com
proxy-panel website status
```

`install` 和成功的 `update` 结束时也会询问是否执行该流程。新安装尚无节点时，先在面板创建上述节点，再执行 `website setup`。非交互安装/升级保留原行为，提示可用命令。普通 TCP、Reality 等其他传输不会被自动改写。

脚本通过系统包管理器安装 Nginx、Python 3/PyYAML、OpenSSL 等依赖，支持原有 Debian/Ubuntu 和 AlmaLinux/Rocky/CentOS/Fedora 安装入口。Nginx 需支持 `ssl_reject_handshake`（1.19.4+）。脚本是自包含的，无需单独下载网站助手；面板需支持 `-check-health`（v1.19.1+）。

流量路径为 **公网非回环地址 TCP 443 → Nginx 网站 / HTTPUpgrade 路径 → Xray 127.0.0.1:443**。客户端的服务器地址、对外端口、路径和凭据保留，仅将节点 ALPN 对齐 `http/1.1`；部署后刷新订阅。反代保留 Host/Upgrade，验证到 Xray 的 TLS 证书。未知 SNI 拒绝握手，网站不转发管理面板。直接使用 IP 打开 HTTPS 不是验收方式。

托管服务名为 `proxy-panel-web.service`，配置位于 `/etc/nginx/proxy-panel/`，网站位于 `/var/www/proxy-panel-managed-site/`。它使用独立 Nginx 配置，不覆盖已有 Nginx 虚拟主机，也不会启动系统自带的默认 80 网站。重复部署保留已编辑的首页。已有未托管的网站目录、回环节点或多个 443 节点会明确中止，避免接管其他部署。

切换前检查证书/私钥匹配、证书链和域名、Xray 与 Nginx 配置；停服后备份数据库、内核及网站配置，再复检并切换。通过网站 HTTPS 200、HTTPUpgrade 101 和面板健康检查后才确认完成。部署失败会恢复原节点、文件和原服务状态；备份保留在 `/opt/proxy-panel/backups/website-*`。依赖软件及为反代安装的 SELinux 支持规则不会在失败时卸载。SELinux 保持 enforcing；按需设置证书标签与 `httpd_can_network_connect`，不会关闭 SELinux。

`proxy-panel-web-cert.path` 监听原证书/私钥文件变化，先验证 Nginx 配置再重载网站；继续使用原来的证书签发/续期方式。它不自动申请新域名证书，也不降低证书校验。例如节点 SNI/CDN 域名为 `cdn.example.com`，但证书只覆盖 `origin.example.com`，会在切换前明确中止；必须先申请覆盖所需域名的证书。管理域名仅在被该证书覆盖时自动作为网站别名加入。

上述 101 检查验证 HTTPUpgrade 隧道建立，部署后仍应使用真实客户端验证代理外网访问。

### 普通 TCP + TLS：网站回落

拓扑：浏览器 / 代理客户端 → **Xray TCP 443（TLS）** → 正常代理流量或 **Nginx 127.0.0.1:8081（HTTP 网站）**。

适用：Xray 的 **VLESS / TCP / TLS** 或 **Trojan / TCP / TLS**。VLESS 可选 `xtls-rprx-vision`。Reality、WS、gRPC 和 sing-box 节点不使用这项回落设置。Reality 的目标站机制不同；不要在同一个 TCP 地址和端口同时开启这两种监听。

### 1. 部署网站

服务器安装 Nginx 后，在本仓库根目录运行以下命令。先检查 8081 是否被占用；已有同名站点文件请先备份，勿直接覆盖自定义内容。

```sh
sudo install -d -m 755 /var/www/proxy-panel-site
sudo install -m 644 deploy/camouflage/index.html /var/www/proxy-panel-site/index.html
sudo install -m 644 deploy/camouflage/nginx.conf /etc/nginx/conf.d/proxy-panel-site.conf
sudo nginx -t
sudo systemctl reload nginx
curl -I http://127.0.0.1:8081/
```

如果 Nginx 尚未启动，在 `nginx -t` 成功后使用 `systemctl start nginx`。示例为静态个人笔记页，可直接编辑 HTML；无外链资源、登录入口或面板信息。示例配置不占用公网 80/443，但系统自带的 Nginx 默认站点可能仍监听公网，应检查 `nginx -T` 和 `ss -lntp`，停用不需要的默认站点。**不要对公网放行 8081**。

### 2. 配置面板节点

- 内核 Xray，协议 VLESS，传输 TCP，安全 TLS，端口 **443**；监听地址显式填写 `0.0.0.0`，或按实际网络填写 IPv6 地址。
- Host 和 SNI 填自己的域名，DNS 指向该服务器。证书和私钥路径必须有效，证书覆盖该域名；客户端保持证书验证开启。
- Flow 可选 `xtls-rprx-vision`，网站回落端口填 **8081**，ALPN 选 **http/1.1**（回落启用且 ALPN 留空时自动补齐）。本示例不支持 h2 回落，不要同时选 h2。
- 保存并同步内核，然后刷新客户端订阅。浏览器使用 `https://你的域名/` 访问网站。未知 SNI 会被拒绝，直接打开 IP 不是本方案的验证方式。

对应节点 Settings 示例（替换域名及路径）：

```json
{
  "listen": "0.0.0.0",
  "security": "tls",
  "sni": "site.example.com",
  "cert_path": "/opt/proxy-panel/certs/site.example.com.crt",
  "key_path": "/opt/proxy-panel/certs/site.example.com.key",
  "flow": "xtls-rprx-vision",
  "fingerprint": "chrome",
  "alpn": ["http/1.1"],
  "fallback_port": 8081
}
```

443 的证书由 Xray 使用，Nginx 只接收解密后的 HTTP/1.1。回落目标固定为回环地址，不允许填写远程目标，也不要将回落端口设为面板或管理 API 端口。TCP 443 上已有 Reality、Web 服务或面板时需要先安排端口迁移；UDP 443 的 Hysteria2 可以与 TCP 443 共存。

### 3. 验收与回滚

- `ss -lntp`：Xray 占用 TCP 443，网站只监听 `127.0.0.1:8081`，管理端口不在公网监听。
- `curl --resolve site.example.com:443:服务器IP https://site.example.com/`：应得到“山间手记”；需要有效域名证书，不使用 `-k` 作为正式验收。
- 同一节点的真实客户端能访问外网，但不能通过代理访问 `127.0.0.1`、`::1`、内网或服务器自己的地址。
- 错误 SNI 的 TLS 握手失败；没有指标令牌时 `/metrics` 为 401（未启用时 404）；正常流量采集仍工作。

关闭网站回落时清空面板的回落端口并保存；回滚整个版本时恢复备份的二进制与配置。网站文件可保留；删掉 Nginx 配置之前先关闭回落，避免正常网页访问断开。

## 规则集下载源升级

Clash 的 MRS 与 sing-box 的 SRS 默认改为通过 `cdn.jsdelivr.net` 下载。启动时仅将数据库中与旧 `ghfast.top` 默认值完全一致的四项前缀替换为 jsDelivr，保留自定义地址。系统分类同步将 `gemini` 改为 `google-gemini`，将不存在的 `category-education-!cn` 改用 `category-scholar-!cn`（非中国学术类别）；分类的启用状态、出站组及顺序保持原值，自定义规则不改写。

升级并重启面板后，**刷新客户端订阅，再更新规则集**。客户端已经保存的旧地址不会仅因服务端重启而改变。

前缀需要保留末尾 `/`。直接访问 jsDelivr 目录可能得到 `Package size exceeded the configured limit of 150 MB`；应测试带文件名的地址，例如：

- Clash：<https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/cn.mrs>
- sing-box：<https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/cn.srs>

2026-10-30 在测试网络中，四个目录返回上述错误，但修正标签后的全部 64 个内置文件均下载成功。目录错误本身不能判断文件下载是否可用；CDN 缓存及客户端网络仍可能影响实际结果，不能保证各地或未来持续可用。

## 参考与边界

参考 [v2ray-agent 的部署实现](https://github.com/mack-a/v2ray-agent/blob/ade6c61b41e0bf631fa53602507a424b49ce2fa5/install.sh)，采用本地网站回落、TLS SNI 校验和 Vision 配置联动；未照搬其固定端口或路由标签。

- [Xray fallbacks](https://xtls.github.io/config/features/fallback.html)
- [Xray TLS 配置](https://xtls.github.io/config/transports/tls.html)
- [sing-box Clash API 鉴权](https://sing-box.sagernet.org/configuration/experimental/clash-api/)

这些修改修复实际可见的配置缺口、减少管理暴露，不构成对 IP 被封原因的定论。已被拦截的 IP 或 TCP 端口需要单独处理。

## 开发验证

常规检查：`go test ./...`、`cd web && npm ci && npm run build`、`bash -n scripts/install.sh`。

真实内核测试可选执行：

```sh
XRAY_BINARY=/绝对路径/xray SINGBOX_BINARY=/绝对路径/sing-box \
  go test ./internal/kernel -run TestCoreIntegration -count=1 -v
```

测试只启动临时回环高端口，不操作 systemd；使用测试证书，访问 Cloudflare `https://1.1.1.1/cdn-cgi/trace` 作为正常转发对照，检查网站回落、未知 SNI 拒绝、IPv4/IPv6/localhost 隔离、管理接口及统计采集。需要外网可达。

## Reality 目标检查与 TLS 参数

节点编辑页的「检查目标」从 VPS 出发验证 DNS、证书与 SNI、TLS 1.3 和 HTTP/2。新建 Reality 节点、更换 dest / server_names、从禁用状态重新启用时，后端也会在写入数据库前检查；检查失败保留原配置。修改名称或保存未变化的目标不会重复联网，已有节点也不会因为面板启动时目标网站临时不可用而被关闭。

检查只接受公网地址，拒绝本机接口、内网和保留地址；连接固定到这次解析并验证的 IP，不使用环境代理、不跟随 HTTP 跳转，整个检查限时 10 秒。支持域名、IPv4 和带方括号的 IPv6 目标，省略端口时使用 443。SNI 必须是具体域名且匹配目标证书；CDN/共享边缘响应头和首页跳转会显示风险提示。CDN 检测是启发式提示，不是完整的归属或滥用审计；DNS 在保存后仍可能变化。

普通 TLS 的 ALPN 支持数组和旧版逗号分隔字符串，Xray 与 sing-box 生成器保持一致。订阅同时兼容 `sni`、旧 `serverName` 及 Reality `server_names` 的首项，避免服务端配置了 SNI 而客户端丢失。Xray 普通 TLS 保持 `rejectUnknownSni: true`，由真实内核握手测试覆盖。sing-box 的 `server_name` 不等同于 Xray 的未知 SNI 拒绝策略；若前置 Nginx 终结 TLS，仍需单独配置默认虚拟主机的 `ssl_reject_handshake on`。Hysteria2 的 QUIC/TLS 默认协商方式保持原样。

协议依据：[Reality 目标标准](https://github.com/XTLS/REALITY)、[Xray TLS](https://xtls.github.io/config/transports/tls.html)、[sing-box TLS](https://sing-box.sagernet.org/configuration/shared/tls/)。

## 有预检和回退的升级

新 `scripts/install.sh update`（检测到已有安装时的 `install` 同样走此路径）按以下顺序升级：

1. 当前服务继续运行，下载候选面板；核对 GitHub release asset 的 SHA-256 后才解包、执行。缺少摘要、下载失败或旧候选不支持预检时直接中止。
2. 可选更新内核时使用候选面板内置 `internal/upgrade/compatibility.json` 的版本，不追随内核的 latest。当前测试组合为 Xray **26.10.29** / sing-box **1.14.4**；CI 和发布流程验证 27 种生成配置，以及真实 TLS / Vision / 网站回落和管理地址隔离。
3. 使用 SQLite 一致快照，在副本上执行候选面板的迁移和配置生成。候选内核分别检查当前配置和新生成配置，验证面板证书/私钥；不启动服务，不修改线上数据库。
4. 预检通过后才停服务，备份程序、数据库（含 WAL）、面板配置与内核配置，再对冻结的数据复检。候选文件在目标目录复制后原子重命名；不改写原监听地址、端口或 TLS 设置。
5. 恢复原本运行的服务，检查 systemd 状态及面板实际 HTTP/TLS 响应。启动或健康检查失败自动恢复旧二进制、数据库和配置；失败诊断目录和备份均仅 root 可访问。正常成功也保留回退备份。

**旧安装需先更新管理脚本，再用新脚本升级**；单独替换面板二进制不会更新 `/usr/local/bin/proxy-panel` 里的旧升级逻辑。应在包含本修复的 release 发布后操作：

```bash
curl -fsSL https://raw.githubusercontent.com/mssHYC/proxy-panel/main/scripts/install.sh -o /tmp/proxy-panel-install.sh
sudo install -m 755 /tmp/proxy-panel-install.sh /usr/local/bin/proxy-panel
sudo proxy-panel update
```

旧版管理脚本在部分 AlmaLinux 等精简系统上可能报 `cmp: command not found`，随后误报“配置路径发生变化”并回退。AlmaLinux 可先运行 `sudo dnf install -y diffutils`，成功后重试升级。新脚本复用已检查的 `jq` 比较预检清单，不再依赖 `cmp`，并分别报告路径变化与清单读取/解析失败。

手动回退使用升级完成时打印的真实备份目录：

```bash
sudo proxy-panel rollback-update /opt/proxy-panel/backups/update-实际时间与编号
```

回退会恢复升级时的数据库，之后新增的数据不会保留。自动回退处理脚本执行期间的失败；断电、`kill -9`、磁盘损坏等需要使用保留备份人工恢复。健康检查验证本机服务，不代替从中国大陆验证线路可达性。

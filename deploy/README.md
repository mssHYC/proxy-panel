# 管理接口隔离与 443 静态网站

本次配置改变需要先阅读迁移说明。网站用于让正常浏览器访问得到完整页面，不会恢复已被拦截的 TCP/IP，也不能保证不被识别或封禁。

## 升级前

1. 备份数据库、config.yaml 和内核配置，保留 SSH 会话及云厂商控制台。先在备用实例验证。
2. **未设置 `server.listen` 的旧安装升级后改为 `127.0.0.1`。** 面板和原来的公网订阅 URL 都可能不可达。先建立 SSH 隧道，或提前配置独立的 HTTPS 反代及访问限制。确实需要维持原来的公网 TLS 入口时，可显式设 `server.listen: "0.0.0.0"` 并配置有效证书、限制管理来源；不要把明文 HTTP 管理入口开放到公网。
3. `/metrics` 默认返回 404。需要采集时设置独立的 `server.metrics_token`（例如 `openssl rand -hex 32`），采集请求带 `Authorization: Bearer <令牌>`。域名限制仍然生效。
4. 本版本的 sing-box 生成配置要求 **1.12 或更新版本**（现代 DNS / route action 配置）。升级核心后先执行 `sing-box check -c <配置路径>`。Xray 配置用 `xray run -test -config <配置路径>` 检查。
5. 代理出站默认拒绝回环、内网、链路本地、组播和本机网卡地址，包括访问这些地址的域名。这会影响通过代理访问 NAS / 内网服务的旧用法。接口地址在生成配置时读取，网卡地址变化后需重新同步；云 NAT 的外部映射地址不一定在本机网卡上，须额外在防火墙限制管理端口。
6. Xray 普通 TLS 默认拒绝未知或空 SNI，旧客户端应填写证书覆盖的域名；仅填 IP 且不发送 SNI 的 TLS 连接需要修改。已存在的不兼容配置（例如 WS + Vision）会报错，需修正后同步内核。启用 Vision 或改变 ALPN 后，重新获取客户端订阅。现有热添加接口无法传递 Flow，向 Vision 节点添加用户会触发完整配置同步并重启内核，活动连接可能短暂中断。

## 管理入口

默认面板只监听 `127.0.0.1:8080`，从自己的电脑运行：

```sh
ssh -N -L 8080:127.0.0.1:8080 root@你的服务器
```

HTTP 模式在浏览器打开 `http://127.0.0.1:8080`。启用了 TLS 或 `server.domain` 时，需要使用证书对应域名并将该域名在本机解析到回环地址；Host 限制不能代替访问鉴权。

需要长期公网订阅时，使用单独的 HTTPS 域名/入口，反代只放行所需订阅路径，其他面板路径限制管理员来源或使用 VPN。不要把面板页面、`/api` 或 `/metrics` 接到下面的 443 网站回落上。已有防火墙放行规则不会自动删除，应在确认新访问方式后关闭不再需要的管理端口；保留 SSH。

Xray API 继续监听回环，保留用户热更新所需的 HandlerService；代理出站拒绝访问管理网段及本机地址。sing-box 的 Clash API 增加随机 Bearer secret，面板采集携带该令牌，核心配置文件保留 0600 权限。出站规则是防护的一层，不替代主机/云防火墙和进程隔离，尤其不能把它当作对所有 DNS 重绑定、NAT64 或其他网络转换的完整防御。

Docker：容器内配置 `server.listen: "0.0.0.0"`，Compose 在宿主机仅发布 `127.0.0.1:8080`。不要把容器内回环监听与宿主机回环端口映射混淆。

## 在 TCP 443 上共用网站和代理

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

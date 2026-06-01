# ProxyPanel

轻量、自托管的代理节点管理面板。一个二进制 + 一份配置，覆盖用户、节点、订阅、流量、告警的完整生命周期。

> 源码与文档：https://github.com/mssHYC/proxy-panel

## 支持架构

- `linux/amd64`（x86 服务器）
- `linux/arm64`（ARM 服务器 / Apple Silicon）

拉取时 Docker 会自动选择匹配你机器的架构。

## 镜像说明

镜像仅含**面板本体**（单个 Go 二进制 + 内嵌 Vue 前端），体积约 30MB，非 root 用户运行。

⚠️ 镜像**不含** Xray / Sing-box 内核，也**不含**配置文件。内核需由宿主机或独立 sidecar 容器提供，通过 `/app/kernel` 目录共享。

## 快速开始

### 1. 准备 config.yaml

镜像内置的是 `config.example.yaml` 模板，真实配置需你自己提供。关键是 `auth` 段：

```yaml
server:
  port: 8080
  tls: false

database:
  path: data/panel.db

auth:
  jwt_secret: "用 openssl rand -hex 32 生成的随机串"
  admin_user: admin
  admin_pass: "你的强密码"          # 明文即可，首次启动自动 bcrypt 加密
  token_expiry_hours: 24
```

生成强 `jwt_secret`：

```bash
openssl rand -hex 32
```

> 启动时会强制校验：`jwt_secret` 不能为空/占位值且至少 16 字节；`admin_pass` 不能是默认占位 `admin123`。不达标直接拒绝启动，避免裸奔。

### 2. 运行

**docker run：**

```bash
docker run -d --name proxy-panel \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/data:/app/data \
  nuebaoxdd/proxy-panel:latest
```

**docker compose：**

```yaml
services:
  proxy-panel:
    image: nuebaoxdd/proxy-panel:latest
    container_name: proxy-panel
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./data:/app/data
    environment:
      - TZ=Asia/Shanghai
```

启动后访问 `http://<host>:8080`。

## 默认账号与密码

镜像**没有硬编码的默认账号密码**，由你的 `config.yaml` 决定。

首次以**空的 `data/` 目录**启动时，面板会把 `config.yaml` 里的凭据播种进数据库：

| 项 | 来源 |
|---|---|
| 用户名 | `auth.admin_user`（示例为 `admin`） |
| 密码 | `auth.admin_pass`（明文会自动转 bcrypt 存储） |

**重要：账号密码只在首次启动（数据库为空时）生效一次。** 之后凭据存于数据库，再改 `config.yaml` 不会生效。

- **改密码**：登录面板 → 系统设置 → 修改密码。
- **重置**：删除 `data/panel.db` 后重启，会用 config 里的凭据重新播种（⚠️ 会清空所有节点/用户数据）。

## 数据持久化

| 路径 | 用途 |
|---|---|
| `/app/data` | SQLite 数据库，**必须挂卷持久化**，否则容器重建即丢数据 |
| `/app/kernel` | （可选）与 Xray/Sing-box 内核共享配置 |

## 标签

| 标签 | 说明 |
|---|---|
| `latest` | 最新版 |
| `v1.0.0` | 固定版本 |

## License

MIT

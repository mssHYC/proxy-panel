# ProxyPanel MVP (v1.0) 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个完整的代理管理面板 MVP，包含用户/节点管理、Xray 内核集成、流量统计与刷新、多格式订阅生成、Telegram/企微告警、Vue 3 前端和一键部署脚本。

**Architecture:** Go (Gin) 后端 + SQLite (WAL) 数据库 + Vue 3 (Element Plus + TailwindCSS) 前端。面板与代理内核 (Xray/Sing-box) 独立进程运行，通过 Stats API 采集流量。前端构建产物由 Go 后端静态托管。

**Tech Stack:** Go 1.22 / Gin / SQLite / robfig/cron v3 / JWT / Vue 3 / Vite / Pinia / Element Plus / TailwindCSS / ECharts

---

## 文件结构总览

```
proxy-panel/
├── cmd/server/main.go                    # 入口
├── config.example.yaml                   # 示例配置
├── internal/
│   ├── config/config.go                  # 配置加载 (YAML → struct)
│   ├── database/
│   │   ├── database.go                   # SQLite 连接 + WAL 初始化
│   │   └── migrations.go                 # 建表 SQL
│   ├── model/
│   │   ├── user.go                       # User 结构体
│   │   ├── node.go                       # Node 结构体
│   │   ├── traffic.go                    # TrafficLog / ServerTraffic 结构体
│   │   ├── setting.go                    # Setting KV 结构体
│   │   └── alert.go                      # AlertRecord 结构体
│   ├── router/
│   │   ├── router.go                     # 路由注册
│   │   └── middleware.go                 # JWT 中间件 + 限流
│   ├── handler/
│   │   ├── auth.go                       # 登录
│   │   ├── user.go                       # 用户 CRUD
│   │   ├── node.go                       # 节点 CRUD
│   │   ├── dashboard.go                  # 仪表盘
│   │   ├── traffic.go                    # 流量相关
│   │   ├── subscription.go               # 订阅入口
│   │   ├── kernel.go                     # 内核控制
│   │   ├── setting.go                    # 配置管理
│   │   └── notify.go                     # 通知测试
│   ├── service/
│   │   ├── user.go                       # 用户业务逻辑
│   │   ├── node.go                       # 节点业务逻辑
│   │   ├── traffic.go                    # 流量采集 + 阈值检查
│   │   ├── subscription/
│   │   │   ├── subscription.go           # 统一入口
│   │   │   ├── surge.go                  # Surge 格式
│   │   │   ├── clash.go                  # Clash/Mihomo 格式
│   │   │   ├── v2ray.go                  # V2Ray URI 格式
│   │   │   ├── shadowrocket.go           # Shadowrocket 格式
│   │   │   └── singbox.go               # Sing-box JSON 格式
│   │   ├── notify/
│   │   │   ├── notify.go                 # 通知统一入口
│   │   │   ├── telegram.go              # Telegram Bot
│   │   │   └── wechat.go               # 企业微信 Webhook
│   │   └── scheduler.go                 # 定时任务调度
│   └── kernel/
│       ├── engine.go                     # Engine 接口定义
│       ├── manager.go                    # Manager 协调多内核
│       ├── xray.go                       # Xray 实现
│       └── singbox.go                    # Sing-box 实现 (v1.0 基础框架)
├── web/                                  # Vue 3 前端
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.ts
│       ├── App.vue
│       ├── api/
│       │   ├── request.ts                # axios 封装
│       │   ├── auth.ts
│       │   ├── user.ts
│       │   ├── node.ts
│       │   ├── dashboard.ts
│       │   ├── traffic.ts
│       │   ├── setting.ts
│       │   └── notify.ts
│       ├── stores/
│       │   └── auth.ts                   # Pinia auth store
│       ├── router/
│       │   └── index.ts
│       ├── views/
│       │   ├── Login.vue
│       │   ├── Dashboard.vue
│       │   ├── Users.vue
│       │   ├── Nodes.vue
│       │   ├── Traffic.vue
│       │   └── Settings.vue
│       ├── components/
│       │   ├── Layout.vue                # 主布局 (侧边栏 + 顶栏)
│       │   ├── SubscriptionDialog.vue    # 订阅链接弹窗
│       │   └── TrafficChart.vue          # 流量图表
│       └── utils/
│           └── format.ts                 # 字节格式化等工具
├── scripts/
│   └── install.sh                        # 一键部署脚本
├── go.mod
└── go.sum
```

---

## Phase 1: 后端基础框架

### Task 1: 项目初始化 + Go Module

**Files:**
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `config.example.yaml`
- Create: `internal/config/config.go`

- [ ] **Step 1: 初始化 Go Module**

```bash
cd /Users/huangyuchuan/Desktop/proxy_panel
go mod init proxy-panel
```

- [ ] **Step 2: 创建配置结构体**

创建 `internal/config/config.go`：

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Traffic  TrafficConfig  `yaml:"traffic"`
	Notify   NotifyConfig   `yaml:"notify"`
	Kernel   KernelConfig   `yaml:"kernel"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	TLS  bool   `yaml:"tls"`
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret    string `yaml:"jwt_secret"`
	AdminUser    string `yaml:"admin_user"`
	AdminPass    string `yaml:"admin_pass"`
	TokenExpiry  int    `yaml:"token_expiry_hours"`
}

type TrafficConfig struct {
	CollectInterval int    `yaml:"collect_interval_sec"`
	ServerLimitGB   int    `yaml:"server_limit_gb"`
	WarnPercent     int    `yaml:"warn_percent"`
	ResetCron       string `yaml:"reset_cron"`
}

type NotifyConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Wechat   WechatConfig   `yaml:"wechat"`
}

type TelegramConfig struct {
	Enable   bool   `yaml:"enable"`
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type WechatConfig struct {
	Enable     bool   `yaml:"enable"`
	WebhookURL string `yaml:"webhook_url"`
}

type KernelConfig struct {
	XrayPath    string `yaml:"xray_path"`
	XrayConfig  string `yaml:"xray_config"`
	XrayAPIPort int    `yaml:"xray_api_port"`
	SingboxPath   string `yaml:"singbox_path"`
	SingboxConfig string `yaml:"singbox_config"`
	SingboxAPIPort int   `yaml:"singbox_api_port"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Server:  ServerConfig{Port: 8080},
		Database: DatabaseConfig{Path: "data/panel.db"},
		Auth:    AuthConfig{TokenExpiry: 24},
		Traffic: TrafficConfig{CollectInterval: 60, WarnPercent: 80},
		Kernel:  KernelConfig{XrayAPIPort: 10085, SingboxAPIPort: 9090},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
```

- [ ] **Step 3: 创建示例配置**

创建 `config.example.yaml`：

```yaml
server:
  port: 8080
  tls: false
  # cert: /opt/proxy-panel/certs/domain.crt
  # key: /opt/proxy-panel/certs/domain.key

database:
  path: data/panel.db

auth:
  jwt_secret: "change-me-to-random-32-bytes"
  admin_user: admin
  admin_pass: "admin123"
  token_expiry_hours: 24

traffic:
  collect_interval_sec: 60
  server_limit_gb: 1000
  warn_percent: 80
  reset_cron: "0 0 1 * *"

notify:
  telegram:
    enable: false
    bot_token: ""
    chat_id: ""
  wechat:
    enable: false
    webhook_url: ""

kernel:
  xray_path: /usr/local/bin/xray
  xray_config: /opt/proxy-panel/kernel/xray.json
  xray_api_port: 10085
  singbox_path: /usr/local/bin/sing-box
  singbox_config: /opt/proxy-panel/kernel/singbox.json
  singbox_api_port: 9090
```

- [ ] **Step 4: 创建 main.go 骨架**

创建 `cmd/server/main.go`：

```go
package main

import (
	"flag"
	"log"

	"proxy-panel/internal/config"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("ProxyPanel 启动中，端口: %d", cfg.Server.Port)
	// 后续步骤填充: database → router → scheduler → listen
}
```

- [ ] **Step 5: 安装依赖并验证编译**

```bash
cd /Users/huangyuchuan/Desktop/proxy_panel
go get gopkg.in/yaml.v3
go build ./cmd/server/
```

预期：编译成功，生成 `server` 二进制。

- [ ] **Step 6: 提交**

```bash
git init
git add go.mod go.sum cmd/ internal/config/ config.example.yaml
git commit -m "feat: 项目初始化，配置加载模块"
```

---

### Task 2: 数据库层 + 迁移

**Files:**
- Create: `internal/database/database.go`
- Create: `internal/database/migrations.go`
- Create: `internal/model/user.go`
- Create: `internal/model/node.go`
- Create: `internal/model/traffic.go`
- Create: `internal/model/setting.go`
- Create: `internal/model/alert.go`

- [ ] **Step 1: 创建数据模型**

创建 `internal/model/user.go`：

```go
package model

import "time"

type User struct {
	ID           int64      `json:"id" db:"id"`
	UUID         string     `json:"uuid" db:"uuid"`
	Username     string     `json:"username" db:"username"`
	Password     string     `json:"-" db:"password"`
	Email        string     `json:"email" db:"email"`
	Protocol     string     `json:"protocol" db:"protocol"`
	TrafficLimit int64      `json:"traffic_limit" db:"traffic_limit"`
	TrafficUsed  int64      `json:"traffic_used" db:"traffic_used"`
	TrafficUp    int64      `json:"traffic_up" db:"traffic_up"`
	TrafficDown  int64      `json:"traffic_down" db:"traffic_down"`
	SpeedLimit   int64      `json:"speed_limit" db:"speed_limit"`
	ResetDay     int        `json:"reset_day" db:"reset_day"`
	ResetCron    string     `json:"reset_cron" db:"reset_cron"`
	Enable       bool       `json:"enable" db:"enable"`
	ExpiresAt    *time.Time `json:"expires_at" db:"expires_at"`
	WarnSent     bool       `json:"warn_sent" db:"warn_sent"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}
```

创建 `internal/model/node.go`：

```go
package model

import "time"

type Node struct {
	ID         int64     `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	Host       string    `json:"host" db:"host"`
	Port       int       `json:"port" db:"port"`
	Protocol   string    `json:"protocol" db:"protocol"`
	Transport  string    `json:"transport" db:"transport"`
	KernelType string    `json:"kernel_type" db:"kernel_type"`
	Settings   string    `json:"settings" db:"settings"`
	Enable     bool      `json:"enable" db:"enable"`
	SortOrder  int       `json:"sort_order" db:"sort_order"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}
```

创建 `internal/model/traffic.go`：

```go
package model

import "time"

type TrafficLog struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	NodeID    int64     `json:"node_id" db:"node_id"`
	Upload    int64     `json:"upload" db:"upload"`
	Download  int64     `json:"download" db:"download"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

type ServerTraffic struct {
	ID        int64     `json:"id" db:"id"`
	TotalUp   int64     `json:"total_up" db:"total_up"`
	TotalDown int64     `json:"total_down" db:"total_down"`
	LimitBytes int64   `json:"limit_bytes" db:"limit_bytes"`
	WarnSent  bool      `json:"warn_sent" db:"warn_sent"`
	LimitSent bool      `json:"limit_sent" db:"limit_sent"`
	ResetAt   time.Time `json:"reset_at" db:"reset_at"`
}
```

创建 `internal/model/setting.go`：

```go
package model

type Setting struct {
	Key   string `json:"key" db:"key"`
	Value string `json:"value" db:"value"`
}
```

创建 `internal/model/alert.go`：

```go
package model

import "time"

type AlertRecord struct {
	ID        int64     `json:"id" db:"id"`
	Type      string    `json:"type" db:"type"`
	Message   string    `json:"message" db:"message"`
	Channel   string    `json:"channel" db:"channel"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

- [ ] **Step 2: 创建数据库连接**

创建 `internal/database/database.go`：

```go
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	return d, nil
}
```

- [ ] **Step 3: 创建迁移**

创建 `internal/database/migrations.go`：

```go
package database

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			password TEXT DEFAULT '',
			email TEXT DEFAULT '',
			protocol TEXT NOT NULL DEFAULT 'vless',
			traffic_limit INTEGER DEFAULT 0,
			traffic_used INTEGER DEFAULT 0,
			traffic_up INTEGER DEFAULT 0,
			traffic_down INTEGER DEFAULT 0,
			speed_limit INTEGER DEFAULT 0,
			reset_day INTEGER DEFAULT 1,
			reset_cron TEXT DEFAULT '',
			enable INTEGER DEFAULT 1,
			expires_at DATETIME,
			warn_sent INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'vless',
			transport TEXT NOT NULL DEFAULT 'tcp',
			kernel_type TEXT NOT NULL DEFAULT 'xray',
			settings TEXT DEFAULT '{}',
			enable INTEGER DEFAULT 1,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS traffic_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			node_id INTEGER DEFAULT 0,
			upload INTEGER DEFAULT 0,
			download INTEGER DEFAULT 0,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_user_time ON traffic_logs(user_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_time ON traffic_logs(timestamp)`,
		`CREATE TABLE IF NOT EXISTS server_traffic (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			total_up INTEGER DEFAULT 0,
			total_down INTEGER DEFAULT 0,
			limit_bytes INTEGER DEFAULT 0,
			warn_sent INTEGER DEFAULT 0,
			limit_sent INTEGER DEFAULT 0,
			reset_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT OR IGNORE INTO server_traffic (id, total_up, total_down) VALUES (1, 0, 0)`,
		`CREATE TABLE IF NOT EXISTS alert_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			channel TEXT DEFAULT '',
			status TEXT DEFAULT 'sent',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: 安装 SQLite 驱动并验证编译**

```bash
go get github.com/mattn/go-sqlite3
go build ./...
```

预期：编译成功。

- [ ] **Step 5: 提交**

```bash
git add internal/model/ internal/database/ go.mod go.sum
git commit -m "feat: 数据库层 + 数据模型 + 迁移"
```

---

### Task 3: JWT 认证 + 路由框架

**Files:**
- Create: `internal/router/router.go`
- Create: `internal/router/middleware.go`
- Create: `internal/handler/auth.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 创建 JWT 中间件**

创建 `internal/router/middleware.go`：

```go
package router

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWT 中间件
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权", "code": "ERR_UNAUTHORIZED"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌无效", "code": "ERR_INVALID_TOKEN"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌解析失败", "code": "ERR_INVALID_TOKEN"})
			c.Abort()
			return
		}
		c.Set("username", claims["username"])
		c.Next()
	}
}

// 登录限流：同 IP 5 次/分钟
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{attempts: make(map[string][]time.Time)}
}

func (rl *RateLimiter) LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		rl.mu.Lock()
		now := time.Now()
		// 清理 1 分钟前的记录
		valid := rl.attempts[ip][:0]
		for _, t := range rl.attempts[ip] {
			if now.Sub(t) < time.Minute {
				valid = append(valid, t)
			}
		}
		rl.attempts[ip] = valid

		if len(valid) >= 5 {
			rl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录请求过于频繁，请稍后再试", "code": "ERR_RATE_LIMIT"})
			c.Abort()
			return
		}
		rl.attempts[ip] = append(rl.attempts[ip], now)
		rl.mu.Unlock()
		c.Next()
	}
}

// 订阅限流：单 UUID 30 次/分钟
type SubRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewSubRateLimiter() *SubRateLimiter {
	return &SubRateLimiter{attempts: make(map[string][]time.Time)}
}

func (srl *SubRateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		uuid := c.Param("uuid")
		srl.mu.Lock()
		now := time.Now()
		valid := srl.attempts[uuid][:0]
		for _, t := range srl.attempts[uuid] {
			if now.Sub(t) < time.Minute {
				valid = append(valid, t)
			}
		}
		srl.attempts[uuid] = valid

		if len(valid) >= 30 {
			srl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁", "code": "ERR_RATE_LIMIT"})
			c.Abort()
			return
		}
		srl.attempts[uuid] = append(srl.attempts[uuid], now)
		srl.mu.Unlock()
		c.Next()
	}
}
```

- [ ] **Step 2: 创建登录 handler**

创建 `internal/handler/auth.go`：

```go
package handler

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"proxy-panel/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	// 校验管理员账号
	passHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Password)))
	adminHash := fmt.Sprintf("%x", sha256.Sum256([]byte(h.cfg.Auth.AdminPass)))

	if req.Username != h.cfg.Auth.AdminUser || passHash != adminHash {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误", "code": "ERR_LOGIN_FAILED"})
		return
	}

	// 生成 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": req.Username,
		"exp":      time.Now().Add(time.Duration(h.cfg.Auth.TokenExpiry) * time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(h.cfg.Auth.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败", "code": "ERR_INTERNAL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenStr})
}
```

- [ ] **Step 3: 创建路由注册**

创建 `internal/router/router.go`：

```go
package router

import (
	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"

	"github.com/gin-gonic/gin"
)

func Setup(cfg *config.Config, db *database.DB) *gin.Engine {
	r := gin.Default()

	// 静态文件
	r.Static("/assets", "./web/assets")
	r.StaticFile("/", "./web/index.html")
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/index.html")
	})

	authHandler := handler.NewAuthHandler(cfg)
	rateLimiter := NewRateLimiter()
	subLimiter := NewSubRateLimiter()

	api := r.Group("/api")
	{
		// 公开端点
		api.POST("/auth/login", rateLimiter.LoginRateLimit(), authHandler.Login)

		// 订阅端点 (UUID 即凭证)
		// api.GET("/sub/:uuid", subLimiter.Limit(), subHandler.Subscribe)
		_ = subLimiter // 后续 Task 使用

		// 需要认证的端点
		auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret))
		{
			_ = auth // 后续 Task 注册路由
		}
	}

	return r
}
```

- [ ] **Step 4: 更新 main.go**

更新 `cmd/server/main.go`：

```go
package main

import (
	"flag"
	"fmt"
	"log"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/router"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	r := router.Setup(cfg, db)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("ProxyPanel 启动成功，监听 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
```

- [ ] **Step 5: 安装依赖并验证编译**

```bash
go get github.com/gin-gonic/gin
go get github.com/golang-jwt/jwt/v5
go build ./cmd/server/
```

预期：编译成功。

- [ ] **Step 6: 提交**

```bash
git add internal/router/ internal/handler/auth.go cmd/server/main.go go.mod go.sum
git commit -m "feat: JWT 认证 + 路由框架 + 登录限流"
```

---

## Phase 2: 用户 + 节点 CRUD

### Task 4: 用户服务 + Handler

**Files:**
- Create: `internal/service/user.go`
- Create: `internal/handler/user.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: 创建用户服务**

创建 `internal/service/user.go`：

```go
package service

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"

	"github.com/google/uuid"
)

type UserService struct {
	db *database.DB
}

func NewUserService(db *database.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) List() ([]model.User, error) {
	rows, err := s.db.Query(`SELECT id, uuid, username, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		err := rows.Scan(&u.ID, &u.UUID, &u.Username, &u.Email, &u.Protocol,
			&u.TrafficLimit, &u.TrafficUsed, &u.TrafficUp, &u.TrafficDown, &u.SpeedLimit,
			&u.ResetDay, &u.ResetCron, &u.Enable, &u.ExpiresAt, &u.WarnSent, &u.CreatedAt, &u.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *UserService) GetByID(id int64) (*model.User, error) {
	var u model.User
	err := s.db.QueryRow(`SELECT id, uuid, username, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users WHERE id = ?`, id).Scan(
		&u.ID, &u.UUID, &u.Username, &u.Email, &u.Protocol,
		&u.TrafficLimit, &u.TrafficUsed, &u.TrafficUp, &u.TrafficDown, &u.SpeedLimit,
		&u.ResetDay, &u.ResetCron, &u.Enable, &u.ExpiresAt, &u.WarnSent, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (s *UserService) GetByUUID(uid string) (*model.User, error) {
	var u model.User
	err := s.db.QueryRow(`SELECT id, uuid, username, email, protocol,
		traffic_limit, traffic_used, traffic_up, traffic_down, speed_limit,
		reset_day, reset_cron, enable, expires_at, warn_sent, created_at, updated_at
		FROM users WHERE uuid = ?`, uid).Scan(
		&u.ID, &u.UUID, &u.Username, &u.Email, &u.Protocol,
		&u.TrafficLimit, &u.TrafficUsed, &u.TrafficUp, &u.TrafficDown, &u.SpeedLimit,
		&u.ResetDay, &u.ResetCron, &u.Enable, &u.ExpiresAt, &u.WarnSent, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

type CreateUserReq struct {
	Username     string `json:"username" binding:"required"`
	Email        string `json:"email"`
	Protocol     string `json:"protocol" binding:"required"`
	TrafficLimit int64  `json:"traffic_limit"`
	SpeedLimit   int64  `json:"speed_limit"`
	ResetDay     int    `json:"reset_day"`
	ResetCron    string `json:"reset_cron"`
	ExpiresAt    string `json:"expires_at"`
}

func (s *UserService) Create(req *CreateUserReq) (*model.User, error) {
	uid := uuid.New().String()

	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02 15:04:05", req.ExpiresAt)
		if err != nil {
			t, err = time.Parse("2006-01-02", req.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("到期时间格式错误")
			}
		}
		expiresAt = &t
	}

	result, err := s.db.Exec(`INSERT INTO users (uuid, username, email, protocol, traffic_limit, speed_limit, reset_day, reset_cron, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, req.Username, req.Email, req.Protocol,
		req.TrafficLimit, req.SpeedLimit, req.ResetDay, req.ResetCron, expiresAt)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetByID(id)
}

type UpdateUserReq struct {
	Username     *string `json:"username"`
	Email        *string `json:"email"`
	Protocol     *string `json:"protocol"`
	TrafficLimit *int64  `json:"traffic_limit"`
	SpeedLimit   *int64  `json:"speed_limit"`
	ResetDay     *int    `json:"reset_day"`
	ResetCron    *string `json:"reset_cron"`
	Enable       *bool   `json:"enable"`
	ExpiresAt    *string `json:"expires_at"`
}

func (s *UserService) Update(id int64, req *UpdateUserReq) (*model.User, error) {
	u, err := s.GetByID(id)
	if err != nil || u == nil {
		return nil, fmt.Errorf("用户不存在")
	}

	if req.Username != nil {
		u.Username = *req.Username
	}
	if req.Email != nil {
		u.Email = *req.Email
	}
	if req.Protocol != nil {
		u.Protocol = *req.Protocol
	}
	if req.TrafficLimit != nil {
		u.TrafficLimit = *req.TrafficLimit
	}
	if req.SpeedLimit != nil {
		u.SpeedLimit = *req.SpeedLimit
	}
	if req.ResetDay != nil {
		u.ResetDay = *req.ResetDay
	}
	if req.ResetCron != nil {
		u.ResetCron = *req.ResetCron
	}
	if req.Enable != nil {
		u.Enable = *req.Enable
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			u.ExpiresAt = nil
		} else {
			t, err := time.Parse("2006-01-02 15:04:05", *req.ExpiresAt)
			if err != nil {
				t, _ = time.Parse("2006-01-02", *req.ExpiresAt)
			}
			u.ExpiresAt = &t
		}
	}

	_, err = s.db.Exec(`UPDATE users SET username=?, email=?, protocol=?, traffic_limit=?, speed_limit=?,
		reset_day=?, reset_cron=?, enable=?, expires_at=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		u.Username, u.Email, u.Protocol, u.TrafficLimit, u.SpeedLimit,
		u.ResetDay, u.ResetCron, u.Enable, u.ExpiresAt, id)
	if err != nil {
		return nil, err
	}

	return s.GetByID(id)
}

func (s *UserService) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *UserService) ResetTraffic(id int64) error {
	_, err := s.db.Exec(`UPDATE users SET traffic_used=0, traffic_up=0, traffic_down=0, warn_sent=0, updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (s *UserService) ResetUUID(id int64) (string, error) {
	newUUID := uuid.New().String()
	_, err := s.db.Exec("UPDATE users SET uuid=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", newUUID, id)
	return newUUID, err
}

func (s *UserService) Count() (total int, enabled int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE enable=1").Scan(&enabled)
	return
}

func hashPassword(password string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(password)))
}
```

- [ ] **Step 2: 创建用户 handler**

创建 `internal/handler/user.go`：

```go
package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (h *UserHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在", "code": "ERR_NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error(), "code": "ERR_BAD_REQUEST"})
		return
	}
	user, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_CREATE_FAILED"})
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req service.UpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	user, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_UPDATE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败", "code": "ERR_INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (h *UserHandler) ResetTraffic(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.ResetTraffic(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置失败", "code": "ERR_INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "流量已重置"})
}
```

- [ ] **Step 3: 注册用户路由**

更新 `internal/router/router.go`，在 `auth` group 中添加：

```go
// 在 Setup 函数中
userSvc := service.NewUserService(db)
userHandler := handler.NewUserHandler(userSvc)

auth := api.Group("", JWTAuth(cfg.Auth.JWTSecret))
{
    auth.GET("/users", userHandler.List)
    auth.POST("/users", userHandler.Create)
    auth.GET("/users/:id", userHandler.Get)
    auth.PUT("/users/:id", userHandler.Update)
    auth.DELETE("/users/:id", userHandler.Delete)
    auth.POST("/users/:id/reset-traffic", userHandler.ResetTraffic)
}
```

- [ ] **Step 4: 安装 uuid 依赖并验证编译**

```bash
go get github.com/google/uuid
go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/user.go internal/handler/user.go internal/router/router.go go.mod go.sum
git commit -m "feat: 用户管理 CRUD + 流量重置"
```

---

### Task 5: 节点服务 + Handler

**Files:**
- Create: `internal/service/node.go`
- Create: `internal/handler/node.go`
- Modify: `internal/router/router.go`

- [ ] **Step 1: 创建节点服务**

创建 `internal/service/node.go`：

```go
package service

import (
	"database/sql"
	"fmt"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
)

type NodeService struct {
	db *database.DB
}

func NewNodeService(db *database.DB) *NodeService {
	return &NodeService{db: db}
}

func (s *NodeService) List() ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, protocol, transport, kernel_type, settings, enable, sort_order, created_at, updated_at
		FROM nodes ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		err := rows.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
			&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *NodeService) ListEnabled() ([]model.Node, error) {
	rows, err := s.db.Query(`SELECT id, name, host, port, protocol, transport, kernel_type, settings, enable, sort_order, created_at, updated_at
		FROM nodes WHERE enable=1 ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		err := rows.Scan(&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
			&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *NodeService) GetByID(id int64) (*model.Node, error) {
	var n model.Node
	err := s.db.QueryRow(`SELECT id, name, host, port, protocol, transport, kernel_type, settings, enable, sort_order, created_at, updated_at
		FROM nodes WHERE id = ?`, id).Scan(
		&n.ID, &n.Name, &n.Host, &n.Port, &n.Protocol, &n.Transport,
		&n.KernelType, &n.Settings, &n.Enable, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

type CreateNodeReq struct {
	Name       string `json:"name" binding:"required"`
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port" binding:"required"`
	Protocol   string `json:"protocol" binding:"required"`
	Transport  string `json:"transport"`
	KernelType string `json:"kernel_type"`
	Settings   string `json:"settings"`
	SortOrder  int    `json:"sort_order"`
}

func (s *NodeService) Create(req *CreateNodeReq) (*model.Node, error) {
	if req.Transport == "" {
		req.Transport = "tcp"
	}
	if req.KernelType == "" {
		req.KernelType = "xray"
	}
	if req.Settings == "" {
		req.Settings = "{}"
	}

	result, err := s.db.Exec(`INSERT INTO nodes (name, host, port, protocol, transport, kernel_type, settings, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.Host, req.Port, req.Protocol, req.Transport, req.KernelType, req.Settings, req.SortOrder)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetByID(id)
}

type UpdateNodeReq struct {
	Name       *string `json:"name"`
	Host       *string `json:"host"`
	Port       *int    `json:"port"`
	Protocol   *string `json:"protocol"`
	Transport  *string `json:"transport"`
	KernelType *string `json:"kernel_type"`
	Settings   *string `json:"settings"`
	Enable     *bool   `json:"enable"`
	SortOrder  *int    `json:"sort_order"`
}

func (s *NodeService) Update(id int64, req *UpdateNodeReq) (*model.Node, error) {
	n, err := s.GetByID(id)
	if err != nil || n == nil {
		return nil, fmt.Errorf("节点不存在")
	}

	if req.Name != nil { n.Name = *req.Name }
	if req.Host != nil { n.Host = *req.Host }
	if req.Port != nil { n.Port = *req.Port }
	if req.Protocol != nil { n.Protocol = *req.Protocol }
	if req.Transport != nil { n.Transport = *req.Transport }
	if req.KernelType != nil { n.KernelType = *req.KernelType }
	if req.Settings != nil { n.Settings = *req.Settings }
	if req.Enable != nil { n.Enable = *req.Enable }
	if req.SortOrder != nil { n.SortOrder = *req.SortOrder }

	_, err = s.db.Exec(`UPDATE nodes SET name=?, host=?, port=?, protocol=?, transport=?, kernel_type=?, settings=?, enable=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		n.Name, n.Host, n.Port, n.Protocol, n.Transport, n.KernelType, n.Settings, n.Enable, n.SortOrder, id)
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *NodeService) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM nodes WHERE id = ?", id)
	return err
}

func (s *NodeService) Count() (total int, enabled int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	if err != nil { return }
	err = s.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE enable=1").Scan(&enabled)
	return
}
```

- [ ] **Step 2: 创建节点 handler**

创建 `internal/handler/node.go`：

```go
package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

type NodeHandler struct {
	svc *service.NodeService
}

func NewNodeHandler(svc *service.NodeService) *NodeHandler {
	return &NodeHandler{svc: svc}
}

func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

func (h *NodeHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	if node == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在", "code": "ERR_NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *NodeHandler) Create(c *gin.Context) {
	var req service.CreateNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error(), "code": "ERR_BAD_REQUEST"})
		return
	}
	node, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_CREATE_FAILED"})
		return
	}
	c.JSON(http.StatusCreated, node)
}

func (h *NodeHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req service.UpdateNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	node, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_UPDATE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *NodeHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败", "code": "ERR_INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
```

- [ ] **Step 3: 注册节点路由**

在 `router.go` 的 `auth` group 中添加：

```go
nodeSvc := service.NewNodeService(db)
nodeHandler := handler.NewNodeHandler(nodeSvc)

auth.GET("/nodes", nodeHandler.List)
auth.POST("/nodes", nodeHandler.Create)
auth.GET("/nodes/:id", nodeHandler.Get)
auth.PUT("/nodes/:id", nodeHandler.Update)
auth.DELETE("/nodes/:id", nodeHandler.Delete)
```

- [ ] **Step 4: 验证编译**

```bash
go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/node.go internal/handler/node.go internal/router/router.go
git commit -m "feat: 节点管理 CRUD"
```

---

## Phase 3: 内核抽象层

### Task 6: Engine 接口 + Xray 实现

**Files:**
- Create: `internal/kernel/engine.go`
- Create: `internal/kernel/manager.go`
- Create: `internal/kernel/xray.go`
- Create: `internal/kernel/singbox.go`
- Create: `internal/handler/kernel.go`

- [ ] **Step 1: 定义 Engine 接口**

创建 `internal/kernel/engine.go`：

```go
package kernel

type UserTraffic struct {
	Upload   int64
	Download int64
}

type Engine interface {
	Name() string
	Start() error
	Stop() error
	Restart() error
	IsRunning() bool
	GetTrafficStats() (map[string]*UserTraffic, error)
	AddUser(tag, uuid, email, protocol string) error
	RemoveUser(tag, uuid, email string) error
	GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error)
	WriteConfig(data []byte) error
}

type NodeConfig struct {
	Tag       string
	Port      int
	Protocol  string
	Transport string
	Settings  map[string]interface{}
}

type UserConfig struct {
	UUID     string
	Email    string
	Protocol string
	SpeedLimit int64
}
```

- [ ] **Step 2: 创建 Xray Engine**

创建 `internal/kernel/xray.go`：

```go
package kernel

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type XrayEngine struct {
	binaryPath string
	configPath string
	apiPort    int
}

func NewXrayEngine(binaryPath, configPath string, apiPort int) *XrayEngine {
	return &XrayEngine{
		binaryPath: binaryPath,
		configPath: configPath,
		apiPort:    apiPort,
	}
}

func (x *XrayEngine) Name() string { return "xray" }

func (x *XrayEngine) Start() error {
	return exec.Command("systemctl", "start", "xray").Run()
}

func (x *XrayEngine) Stop() error {
	return exec.Command("systemctl", "stop", "xray").Run()
}

func (x *XrayEngine) Restart() error {
	return exec.Command("systemctl", "restart", "xray").Run()
}

func (x *XrayEngine) IsRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "xray").Run()
	return err == nil
}

func (x *XrayEngine) GetTrafficStats() (map[string]*UserTraffic, error) {
	cmd := exec.Command(x.binaryPath, "api", "statsquery",
		fmt.Sprintf("--server=127.0.0.1:%d", x.apiPort),
		"-pattern", "user>>>")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xray stats 查询失败: %w", err)
	}

	return parseXrayStats(string(output))
}

func parseXrayStats(output string) (map[string]*UserTraffic, error) {
	result := make(map[string]*UserTraffic)

	// 解析 JSON 格式的 stats 输出
	var stats struct {
		Stat []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		} `json:"stat"`
	}

	if err := json.Unmarshal([]byte(output), &stats); err != nil {
		// 尝试按行解析文本格式
		return parseXrayStatsText(output), nil
	}

	for _, s := range stats.Stat {
		// 格式: user>>>email>>>traffic>>>uplink/downlink
		parts := strings.Split(s.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" {
			continue
		}
		email := parts[1]
		direction := parts[3]

		if _, ok := result[email]; !ok {
			result[email] = &UserTraffic{}
		}
		switch direction {
		case "uplink":
			result[email].Upload = s.Value
		case "downlink":
			result[email].Download = s.Value
		}
	}

	return result, nil
}

func parseXrayStatsText(output string) map[string]*UserTraffic {
	result := make(map[string]*UserTraffic)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "user>>>") {
			continue
		}
		parts := strings.Split(line, ">>>")
		if len(parts) < 4 {
			continue
		}
		email := parts[1]
		if _, ok := result[email]; !ok {
			result[email] = &UserTraffic{}
		}
	}
	return result
}

func (x *XrayEngine) AddUser(tag, uuid, email, protocol string) error {
	// xray api adi --server=127.0.0.1:10085 -inbound <tag> -id <uuid> -email <email>
	args := []string{"api", "adi",
		fmt.Sprintf("--server=127.0.0.1:%d", x.apiPort),
		"-inbound", tag,
	}
	switch protocol {
	case "vless", "vmess":
		args = append(args, "-id", uuid, "-email", email)
	case "trojan":
		args = append(args, "-password", uuid, "-email", email)
	case "ss":
		args = append(args, "-password", uuid, "-email", email)
	}

	cmd := exec.Command(x.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("添加用户失败: %s, %w", string(output), err)
	}
	return nil
}

func (x *XrayEngine) RemoveUser(tag, uuid, email string) error {
	cmd := exec.Command(x.binaryPath, "api", "rmi",
		fmt.Sprintf("--server=127.0.0.1:%d", x.apiPort),
		"-inbound", tag, "-email", email)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("移除用户失败: %s, %w", string(output), err)
	}
	return nil
}

func (x *XrayEngine) GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error) {
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []string{"StatsService"},
		},
		"stats": map[string]interface{}{},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]interface{}{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		},
	}

	// API inbound
	inbounds := []map[string]interface{}{
		{
			"tag":      "api",
			"port":     x.apiPort,
			"listen":   "127.0.0.1",
			"protocol": "dokodemo-door",
			"settings": map[string]interface{}{
				"address": "127.0.0.1",
			},
		},
	}

	// 为每个节点生成 inbound
	for _, node := range nodes {
		inbound := buildXrayInbound(node, users)
		inbounds = append(inbounds, inbound)
	}

	config["inbounds"] = inbounds

	// outbounds
	config["outbounds"] = []map[string]interface{}{
		{"protocol": "freedom", "tag": "direct"},
		{"protocol": "blackhole", "tag": "blocked"},
	}

	// routing
	config["routing"] = map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"inboundTag":  []string{"api"},
				"outboundTag": "api",
				"type":        "field",
			},
		},
	}

	return json.MarshalIndent(config, "", "  ")
}

func buildXrayInbound(node NodeConfig, users []UserConfig) map[string]interface{} {
	inbound := map[string]interface{}{
		"tag":      fmt.Sprintf("in-%d", node.Port),
		"port":     node.Port,
		"listen":   "0.0.0.0",
		"protocol": node.Protocol,
	}

	// 构建用户列表
	var clientList []map[string]interface{}
	for _, u := range users {
		client := map[string]interface{}{
			"email": u.Email,
		}
		switch node.Protocol {
		case "vless":
			client["id"] = u.UUID
			client["flow"] = ""
		case "vmess":
			client["id"] = u.UUID
			client["alterId"] = 0
		case "trojan":
			client["password"] = u.UUID
		case "ss":
			client["password"] = u.UUID
			client["method"] = "aes-256-gcm"
		}
		clientList = append(clientList, client)
	}

	settings := map[string]interface{}{}
	switch node.Protocol {
	case "vless":
		settings["clients"] = clientList
		settings["decryption"] = "none"
	case "vmess":
		settings["clients"] = clientList
	case "trojan":
		settings["clients"] = clientList
	case "ss":
		// SS 在 Xray 中使用不同结构
		if len(clientList) > 0 {
			settings = clientList[0]
			settings["network"] = "tcp,udp"
		}
	}
	inbound["settings"] = settings

	// 传输配置
	streamSettings := map[string]interface{}{}
	switch node.Transport {
	case "ws":
		streamSettings["network"] = "ws"
		wsPath, _ := node.Settings["path"].(string)
		if wsPath == "" {
			wsPath = "/"
		}
		streamSettings["wsSettings"] = map[string]interface{}{
			"path": wsPath,
		}
	case "grpc":
		streamSettings["network"] = "grpc"
		serviceName, _ := node.Settings["service_name"].(string)
		streamSettings["grpcSettings"] = map[string]interface{}{
			"serviceName": serviceName,
		}
	case "reality":
		streamSettings["network"] = "tcp"
		streamSettings["security"] = "reality"
		streamSettings["realitySettings"] = map[string]interface{}{
			"dest":        node.Settings["dest"],
			"serverNames": node.Settings["server_names"],
			"privateKey":  node.Settings["private_key"],
			"shortIds":    node.Settings["short_ids"],
		}
	default:
		streamSettings["network"] = "tcp"
	}

	// TLS
	if tls, ok := node.Settings["tls"].(bool); ok && tls {
		streamSettings["security"] = "tls"
		streamSettings["tlsSettings"] = map[string]interface{}{
			"certificates": []map[string]interface{}{
				{
					"certificateFile": node.Settings["cert_path"],
					"keyFile":         node.Settings["key_path"],
				},
			},
		}
	}

	inbound["streamSettings"] = streamSettings

	return inbound
}

func (x *XrayEngine) WriteConfig(data []byte) error {
	if err := os.WriteFile(x.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	log.Printf("Xray 配置已写入: %s", x.configPath)
	return nil
}
```

- [ ] **Step 3: 创建 Sing-box Engine (基础框架)**

创建 `internal/kernel/singbox.go`：

```go
package kernel

import (
	"fmt"
	"os"
	"os/exec"
)

type SingboxEngine struct {
	binaryPath string
	configPath string
	apiPort    int
}

func NewSingboxEngine(binaryPath, configPath string, apiPort int) *SingboxEngine {
	return &SingboxEngine{
		binaryPath: binaryPath,
		configPath: configPath,
		apiPort:    apiPort,
	}
}

func (s *SingboxEngine) Name() string { return "singbox" }

func (s *SingboxEngine) Start() error {
	return exec.Command("systemctl", "start", "sing-box").Run()
}

func (s *SingboxEngine) Stop() error {
	return exec.Command("systemctl", "stop", "sing-box").Run()
}

func (s *SingboxEngine) Restart() error {
	return exec.Command("systemctl", "restart", "sing-box").Run()
}

func (s *SingboxEngine) IsRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "sing-box").Run()
	return err == nil
}

func (s *SingboxEngine) GetTrafficStats() (map[string]*UserTraffic, error) {
	// v1.1 完整实现，v1.0 返回空
	return make(map[string]*UserTraffic), nil
}

func (s *SingboxEngine) AddUser(tag, uuid, email, protocol string) error {
	// Sing-box 不支持热加载，需要重启
	return fmt.Errorf("sing-box 不支持热加载用户，请重启内核")
}

func (s *SingboxEngine) RemoveUser(tag, uuid, email string) error {
	return fmt.Errorf("sing-box 不支持热加载用户，请重启内核")
}

func (s *SingboxEngine) GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error) {
	// v1.1 完整实现
	return []byte("{}"), nil
}

func (s *SingboxEngine) WriteConfig(data []byte) error {
	return os.WriteFile(s.configPath, data, 0644)
}
```

- [ ] **Step 4: 创建 Manager**

创建 `internal/kernel/manager.go`：

```go
package kernel

import (
	"fmt"
	"sync"
)

type Manager struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

func NewManager() *Manager {
	return &Manager{
		engines: make(map[string]Engine),
	}
}

func (m *Manager) Register(engine Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engines[engine.Name()] = engine
}

func (m *Manager) Get(name string) (Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.engines[name]
	if !ok {
		return nil, fmt.Errorf("内核 %s 未注册", name)
	}
	return e, nil
}

func (m *Manager) Status() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := make(map[string]bool)
	for name, engine := range m.engines {
		status[name] = engine.IsRunning()
	}
	return status
}

func (m *Manager) GetTrafficStats() (map[string]*UserTraffic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	merged := make(map[string]*UserTraffic)
	for _, engine := range m.engines {
		if !engine.IsRunning() {
			continue
		}
		stats, err := engine.GetTrafficStats()
		if err != nil {
			continue
		}
		for email, t := range stats {
			if existing, ok := merged[email]; ok {
				existing.Upload += t.Upload
				existing.Download += t.Download
			} else {
				merged[email] = &UserTraffic{Upload: t.Upload, Download: t.Download}
			}
		}
	}
	return merged, nil
}
```

- [ ] **Step 5: 创建内核 handler**

创建 `internal/handler/kernel.go`：

```go
package handler

import (
	"net/http"

	"proxy-panel/internal/kernel"

	"github.com/gin-gonic/gin"
)

type KernelHandler struct {
	mgr *kernel.Manager
}

func NewKernelHandler(mgr *kernel.Manager) *KernelHandler {
	return &KernelHandler{mgr: mgr}
}

func (h *KernelHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"kernels": h.mgr.Status()})
}

func (h *KernelHandler) Restart(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	engine, err := h.mgr.Get(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_NOT_FOUND"})
		return
	}

	if err := engine.Restart(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重启失败: " + err.Error(), "code": "ERR_RESTART_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "重启成功"})
}
```

- [ ] **Step 6: 注册内核路由，更新 main.go 初始化 Manager**

在 `router.go` 中添加内核路由，在 `main.go` 中创建并注册 Manager。

- [ ] **Step 7: 验证编译并提交**

```bash
go build ./...
git add internal/kernel/ internal/handler/kernel.go
git commit -m "feat: 内核抽象层 (Xray + Sing-box 基础框架)"
```

---

## Phase 4: 流量管理 + 调度器

### Task 7: 流量服务 + 调度器

**Files:**
- Create: `internal/service/traffic.go`
- Create: `internal/service/scheduler.go`
- Create: `internal/handler/traffic.go`
- Create: `internal/handler/dashboard.go`

- [ ] **Step 1: 创建流量服务**

创建 `internal/service/traffic.go`：

```go
package service

import (
	"log"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
)

type TrafficService struct {
	db  *database.DB
	mgr *kernel.Manager
}

func NewTrafficService(db *database.DB, mgr *kernel.Manager) *TrafficService {
	return &TrafficService{db: db, mgr: mgr}
}

// 采集流量 (每 60s 调用)
func (s *TrafficService) Collect() error {
	stats, err := s.mgr.GetTrafficStats()
	if err != nil {
		return err
	}

	now := time.Now()
	for email, traffic := range stats {
		if traffic.Upload == 0 && traffic.Download == 0 {
			continue
		}

		// email = uuid (我们用 uuid 作为 email 注册到内核)
		var userID int64
		err := s.db.QueryRow("SELECT id FROM users WHERE uuid = ?", email).Scan(&userID)
		if err != nil {
			log.Printf("流量采集: 未找到用户 %s", email)
			continue
		}

		// 更新用户流量
		_, err = s.db.Exec(`UPDATE users SET
			traffic_used = traffic_used + ?,
			traffic_up = traffic_up + ?,
			traffic_down = traffic_down + ?,
			updated_at = ? WHERE id = ?`,
			traffic.Upload+traffic.Download, traffic.Upload, traffic.Download, now, userID)
		if err != nil {
			log.Printf("更新用户流量失败: %v", err)
			continue
		}

		// 记录流量日志
		_, err = s.db.Exec(`INSERT INTO traffic_logs (user_id, upload, download, timestamp)
			VALUES (?, ?, ?, ?)`, userID, traffic.Upload, traffic.Download, now)
		if err != nil {
			log.Printf("记录流量日志失败: %v", err)
		}

		// 更新服务器总流量
		_, err = s.db.Exec(`UPDATE server_traffic SET
			total_up = total_up + ?,
			total_down = total_down + ?
			WHERE id = 1`, traffic.Upload, traffic.Download)
		if err != nil {
			log.Printf("更新服务器流量失败: %v", err)
		}
	}

	return nil
}

// 检查用户流量阈值
func (s *TrafficService) CheckUserThresholds(warnPercent int) (warns []model.User, exhausted []model.User, err error) {
	rows, err := s.db.Query(`SELECT id, uuid, username, email, traffic_limit, traffic_used, warn_sent, enable
		FROM users WHERE traffic_limit > 0 AND enable = 1`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.UUID, &u.Username, &u.Email, &u.TrafficLimit, &u.TrafficUsed, &u.WarnSent, &u.Enable); err != nil {
			continue
		}

		percent := float64(u.TrafficUsed) / float64(u.TrafficLimit) * 100

		if percent >= 100 {
			// 自动停用
			s.db.Exec("UPDATE users SET enable=0, updated_at=CURRENT_TIMESTAMP WHERE id=?", u.ID)
			exhausted = append(exhausted, u)
		} else if percent >= float64(warnPercent) && !u.WarnSent {
			s.db.Exec("UPDATE users SET warn_sent=1 WHERE id=?", u.ID)
			warns = append(warns, u)
		}
	}
	return
}

// 检查服务器总流量阈值
func (s *TrafficService) CheckServerThreshold(warnPercent int) (warnNeeded bool, limitReached bool, st *model.ServerTraffic, err error) {
	st = &model.ServerTraffic{}
	err = s.db.QueryRow(`SELECT total_up, total_down, limit_bytes, warn_sent, limit_sent FROM server_traffic WHERE id=1`).Scan(
		&st.TotalUp, &st.TotalDown, &st.LimitBytes, &st.WarnSent, &st.LimitSent)
	if err != nil || st.LimitBytes == 0 {
		return
	}

	total := st.TotalUp + st.TotalDown
	percent := float64(total) / float64(st.LimitBytes) * 100

	if percent >= 100 && !st.LimitSent {
		limitReached = true
		s.db.Exec("UPDATE server_traffic SET limit_sent=1 WHERE id=1")
	} else if percent >= float64(warnPercent) && !st.WarnSent {
		warnNeeded = true
		s.db.Exec("UPDATE server_traffic SET warn_sent=1 WHERE id=1")
	}
	return
}

// 重置用户流量 (按 reset_day)
func (s *TrafficService) ResetByDay(day int) (int64, error) {
	result, err := s.db.Exec(`UPDATE users SET traffic_used=0, traffic_up=0, traffic_down=0, warn_sent=0, updated_at=CURRENT_TIMESTAMP
		WHERE reset_day=? AND reset_cron=''`, day)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// 重置服务器总流量
func (s *TrafficService) ResetServerTraffic() error {
	_, err := s.db.Exec(`UPDATE server_traffic SET total_up=0, total_down=0, warn_sent=0, limit_sent=0, reset_at=CURRENT_TIMESTAMP WHERE id=1`)
	return err
}

// 获取服务器流量
func (s *TrafficService) GetServerTraffic() (*model.ServerTraffic, error) {
	var st model.ServerTraffic
	err := s.db.QueryRow(`SELECT id, total_up, total_down, limit_bytes, warn_sent, limit_sent, reset_at FROM server_traffic WHERE id=1`).Scan(
		&st.ID, &st.TotalUp, &st.TotalDown, &st.LimitBytes, &st.WarnSent, &st.LimitSent, &st.ResetAt)
	return &st, err
}

// 设置服务器流量限额
func (s *TrafficService) SetServerLimit(limitGB int64) error {
	limitBytes := limitGB * 1024 * 1024 * 1024
	_, err := s.db.Exec("UPDATE server_traffic SET limit_bytes=? WHERE id=1", limitBytes)
	return err
}

// 获取历史流量数据 (近 N 天)
func (s *TrafficService) GetHistory(days int) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`SELECT DATE(timestamp) as date,
		SUM(upload) as upload, SUM(download) as download
		FROM traffic_logs
		WHERE timestamp >= datetime('now', ?)
		GROUP BY DATE(timestamp)
		ORDER BY date ASC`, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var date string
		var upload, download int64
		if err := rows.Scan(&date, &upload, &download); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"date":     date,
			"upload":   upload,
			"download": download,
		})
	}
	return result, nil
}
```

需要在文件顶部添加 `"fmt"` import。

- [ ] **Step 2: 创建调度器**

创建 `internal/service/scheduler.go`：

```go
package service

import (
	"log"
	"time"

	"proxy-panel/internal/config"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	cfg        *config.Config
	trafficSvc *TrafficService
	notifySvc  *NotifyService
}

func NewScheduler(cfg *config.Config, trafficSvc *TrafficService, notifySvc *NotifyService) *Scheduler {
	return &Scheduler{
		cron:       cron.New(),
		cfg:        cfg,
		trafficSvc: trafficSvc,
		notifySvc:  notifySvc,
	}
}

func (s *Scheduler) Start() {
	// 流量采集
	interval := s.cfg.Traffic.CollectInterval
	if interval <= 0 {
		interval = 60
	}
	s.cron.AddFunc(fmt.Sprintf("@every %ds", interval), func() {
		if err := s.trafficSvc.Collect(); err != nil {
			log.Printf("流量采集失败: %v", err)
			return
		}

		// 检查用户阈值
		warns, exhausted, err := s.trafficSvc.CheckUserThresholds(s.cfg.Traffic.WarnPercent)
		if err != nil {
			log.Printf("阈值检查失败: %v", err)
		}
		for _, u := range warns {
			msg := fmt.Sprintf("⚠️ 用户 %s 流量已达 %d%%", u.Username, s.cfg.Traffic.WarnPercent)
			s.notifySvc.SendAll(msg)
		}
		for _, u := range exhausted {
			msg := fmt.Sprintf("🚫 用户 %s 流量已耗尽，已自动停用", u.Username)
			s.notifySvc.SendAll(msg)
		}

		// 检查服务器总流量
		warnNeeded, limitReached, st, err := s.trafficSvc.CheckServerThreshold(s.cfg.Traffic.WarnPercent)
		if err == nil {
			if warnNeeded {
				total := st.TotalUp + st.TotalDown
				msg := fmt.Sprintf("⚠️ 服务器总流量预警: 已用 %s / %s",
					formatBytes(total), formatBytes(st.LimitBytes))
				s.notifySvc.SendAll(msg)
			}
			if limitReached {
				msg := "🚫 服务器总流量已达限额！"
				s.notifySvc.SendAll(msg)
			}
		}
	})

	// 用户级流量重置 (每天 00:00)
	s.cron.AddFunc("0 0 * * *", func() {
		day := time.Now().Day()
		affected, err := s.trafficSvc.ResetByDay(day)
		if err != nil {
			log.Printf("用户流量重置失败: %v", err)
			return
		}
		if affected > 0 {
			msg := fmt.Sprintf("✅ 已重置 %d 个用户的流量 (重置日: %d)", affected, day)
			s.notifySvc.SendAll(msg)
			log.Printf(msg)
		}
	})

	// 服务器总流量重置
	if s.cfg.Traffic.ResetCron != "" {
		s.cron.AddFunc(s.cfg.Traffic.ResetCron, func() {
			if err := s.trafficSvc.ResetServerTraffic(); err != nil {
				log.Printf("服务器流量重置失败: %v", err)
				return
			}
			s.notifySvc.SendAll("✅ 服务器总流量已重置")
			log.Println("服务器总流量已重置")
		})
	}

	// 用户到期检查 (每天 08:00)
	s.cron.AddFunc("0 8 * * *", func() {
		s.checkExpiry()
	})

	s.cron.Start()
	log.Println("调度器已启动")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

func (s *Scheduler) checkExpiry() {
	now := time.Now()

	// 已到期 → 停用
	rows, err := s.trafficSvc.db.Query(`SELECT id, username FROM users WHERE enable=1 AND expires_at IS NOT NULL AND expires_at <= ?`, now)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var username string
		rows.Scan(&id, &username)
		s.trafficSvc.db.Exec("UPDATE users SET enable=0, updated_at=CURRENT_TIMESTAMP WHERE id=?", id)
		s.notifySvc.SendAll(fmt.Sprintf("🚫 用户 %s 已到期，已自动停用", username))
	}

	// 即将到期 (3 天内) → 通知
	threeDays := now.Add(72 * time.Hour)
	rows2, err := s.trafficSvc.db.Query(`SELECT username, expires_at FROM users WHERE enable=1 AND expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ?`, now, threeDays)
	if err != nil {
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var username string
		var expiresAt time.Time
		rows2.Scan(&username, &expiresAt)
		s.notifySvc.SendAll(fmt.Sprintf("⏰ 用户 %s 将于 %s 到期", username, expiresAt.Format("2006-01-02")))
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
```

需要在文件顶部添加 `"fmt"` import。

- [ ] **Step 3: 创建流量 handler 和仪表盘 handler**

创建 `internal/handler/traffic.go` 和 `internal/handler/dashboard.go`，包含对应的 HTTP 接口。

- [ ] **Step 4: 安装 cron 依赖并验证编译**

```bash
go get github.com/robfig/cron/v3
go build ./...
```

- [ ] **Step 5: 提交**

```bash
git add internal/service/traffic.go internal/service/scheduler.go internal/handler/traffic.go internal/handler/dashboard.go
git commit -m "feat: 流量管理 + 调度器 + 仪表盘"
```

---

## Phase 5: 通知系统

### Task 8: Telegram + 企业微信通知

**Files:**
- Create: `internal/service/notify/notify.go`
- Create: `internal/service/notify/telegram.go`
- Create: `internal/service/notify/wechat.go`
- Create: `internal/handler/notify.go`

- [ ] **Step 1: 创建通知接口和统一入口**

创建 `internal/service/notify/notify.go`：

```go
package notify

import (
	"log"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
)

type Channel interface {
	Name() string
	Send(message string) error
}

type NotifyService struct {
	channels []Channel
	db       *database.DB
}

func NewNotifyService(cfg *config.Config, db *database.DB) *NotifyService {
	var channels []Channel

	if cfg.Notify.Telegram.Enable {
		channels = append(channels, NewTelegram(cfg.Notify.Telegram.BotToken, cfg.Notify.Telegram.ChatID))
	}
	if cfg.Notify.Wechat.Enable {
		channels = append(channels, NewWechat(cfg.Notify.Wechat.WebhookURL))
	}

	return &NotifyService{channels: channels, db: db}
}

func (s *NotifyService) SendAll(message string) {
	for _, ch := range s.channels {
		if err := ch.Send(message); err != nil {
			log.Printf("通知发送失败 [%s]: %v", ch.Name(), err)
			s.recordAlert(ch.Name(), message, "failed")
		} else {
			s.recordAlert(ch.Name(), message, "sent")
		}
	}
}

func (s *NotifyService) Test(channel string) error {
	for _, ch := range s.channels {
		if ch.Name() == channel || channel == "" {
			return ch.Send("🔔 ProxyPanel 测试消息 - 通知通道正常")
		}
	}
	return fmt.Errorf("通道 %s 未配置或未启用", channel)
}

func (s *NotifyService) recordAlert(channel, message, status string) {
	s.db.Exec(`INSERT INTO alert_records (type, message, channel, status) VALUES ('notify', ?, ?, ?)`,
		message, channel, status)
}
```

需要添加 `"fmt"` import。

- [ ] **Step 2: 创建 Telegram 实现**

创建 `internal/service/notify/telegram.go`：

```go
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Telegram struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	body, _ := json.Marshal(map[string]interface{}{
		"chat_id":    t.chatID,
		"text":       message,
		"parse_mode": "HTML",
	})

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram 返回状态码: %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 3: 创建企业微信实现**

创建 `internal/service/notify/wechat.go`：

```go
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Wechat struct {
	webhookURL string
	client     *http.Client
}

func NewWechat(webhookURL string) *Wechat {
	return &Wechat{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *Wechat) Name() string { return "wechat" }

func (w *Wechat) Send(message string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	})

	resp, err := w.client.Post(w.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("企业微信请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信返回状态码: %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: 创建通知 handler**

创建 `internal/handler/notify.go`：

```go
package handler

import (
	"net/http"

	notify "proxy-panel/internal/service/notify"

	"github.com/gin-gonic/gin"
)

type NotifyHandler struct {
	svc *notify.NotifyService
}

func NewNotifyHandler(svc *notify.NotifyService) *NotifyHandler {
	return &NotifyHandler{svc: svc}
}

func (h *NotifyHandler) Test(c *gin.Context) {
	var req struct {
		Channel string `json:"channel"`
	}
	c.ShouldBindJSON(&req)

	if err := h.svc.Test(req.Channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "ERR_NOTIFY_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "测试消息已发送"})
}
```

- [ ] **Step 5: 验证编译并提交**

```bash
go build ./...
git add internal/service/notify/ internal/handler/notify.go
git commit -m "feat: 通知系统 (Telegram + 企业微信)"
```

---

## Phase 6: 订阅生成

### Task 9: 五格式订阅生成

**Files:**
- Create: `internal/service/subscription/subscription.go`
- Create: `internal/service/subscription/surge.go`
- Create: `internal/service/subscription/clash.go`
- Create: `internal/service/subscription/v2ray.go`
- Create: `internal/service/subscription/shadowrocket.go`
- Create: `internal/service/subscription/singbox.go`
- Create: `internal/handler/subscription.go`

- [ ] **Step 1: 创建订阅统一入口**

创建 `internal/service/subscription/subscription.go`：

```go
package subscription

import "proxy-panel/internal/model"

type Generator interface {
	Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error)
	// 返回: (内容, Content-Type, 错误)
}

func GetGenerator(format string) Generator {
	switch format {
	case "surge":
		return &SurgeGenerator{}
	case "clash":
		return &ClashGenerator{}
	case "v2ray":
		return &V2RayGenerator{}
	case "shadowrocket":
		return &ShadowrocketGenerator{}
	case "singbox":
		return &SingboxGenerator{}
	default:
		return &V2RayGenerator{} // 默认 v2ray 格式
	}
}
```

- [ ] **Step 2: 创建 Surge 格式生成器**

创建 `internal/service/subscription/surge.go`：

```go
package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

type SurgeGenerator struct{}

func (g *SurgeGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var sb strings.Builder

	// Managed Config
	sb.WriteString(fmt.Sprintf("#!MANAGED-CONFIG %s/api/sub/%s?format=surge interval=86400 strict=false\n\n", baseURL, user.UUID))

	// [General]
	sb.WriteString("[General]\n")
	sb.WriteString("loglevel = notify\n")
	sb.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local\n")
	sb.WriteString("dns-server = 8.8.8.8, 1.1.1.1\n\n")

	// [Proxy]
	sb.WriteString("[Proxy]\n")
	sb.WriteString("DIRECT = direct\n")

	var proxyNames []string
	for _, node := range nodes {
		line := surgeProxyLine(node, user)
		if line != "" {
			sb.WriteString(line + "\n")
			proxyNames = append(proxyNames, node.Name)
		}
	}
	sb.WriteString("\n")

	// [Proxy Group]
	sb.WriteString("[Proxy Group]\n")
	if len(proxyNames) > 0 {
		sb.WriteString(fmt.Sprintf("Proxy = select, %s, DIRECT\n", strings.Join(proxyNames, ", ")))
		sb.WriteString(fmt.Sprintf("Auto = url-test, %s, url=http://www.gstatic.com/generate_204, interval=300\n", strings.Join(proxyNames, ", ")))
	}
	sb.WriteString("\n")

	// [Rule]
	sb.WriteString("[Rule]\n")
	sb.WriteString("GEOIP,CN,DIRECT\n")
	sb.WriteString("FINAL,Proxy\n")

	return sb.String(), "text/plain; charset=utf-8", nil
}

func surgeProxyLine(node model.Node, user *model.User) string {
	var settings map[string]interface{}
	json.Unmarshal([]byte(node.Settings), &settings)

	sni, _ := settings["sni"].(string)

	switch node.Protocol {
	case "vmess":
		parts := []string{
			fmt.Sprintf("%s = vmess, %s, %d, username=%s", node.Name, node.Host, node.Port, user.UUID),
		}
		if sni != "" {
			parts = append(parts, "tls=true", fmt.Sprintf("sni=%s", sni))
		}
		if node.Transport == "ws" {
			wsPath, _ := settings["path"].(string)
			parts = append(parts, "ws=true", fmt.Sprintf("ws-path=%s", wsPath))
			if wsHost, ok := settings["host"].(string); ok && wsHost != "" {
				parts = append(parts, fmt.Sprintf("ws-headers=Host:%s", wsHost))
			}
		}
		return strings.Join(parts, ", ")

	case "trojan":
		parts := []string{
			fmt.Sprintf("%s = trojan, %s, %d, password=%s", node.Name, node.Host, node.Port, user.UUID),
		}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		return strings.Join(parts, ", ")

	case "ss":
		method, _ := settings["method"].(string)
		if method == "" {
			method = "aes-256-gcm"
		}
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s",
			node.Name, node.Host, node.Port, method, user.UUID)

	case "hysteria2":
		parts := []string{
			fmt.Sprintf("%s = hysteria2, %s, %d, password=%s", node.Name, node.Host, node.Port, user.UUID),
		}
		if sni != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", sni))
		}
		bandwidth, _ := settings["download_bandwidth"].(float64)
		if bandwidth > 0 {
			parts = append(parts, fmt.Sprintf("download-bandwidth=%d", int(bandwidth)))
		}
		return strings.Join(parts, ", ")

	case "vless":
		return fmt.Sprintf("# %s = VLESS (Surge 不原生支持 VLESS，请使用其他客户端)", node.Name)

	default:
		return ""
	}
}
```

- [ ] **Step 3: 创建 Clash 格式生成器**

创建 `internal/service/subscription/clash.go`：

```go
package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

type ClashGenerator struct{}

func (g *ClashGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var sb strings.Builder

	sb.WriteString("port: 7890\nsocks-port: 7891\nallow-lan: false\nmode: rule\n\n")
	sb.WriteString("proxies:\n")

	var proxyNames []string
	for _, node := range nodes {
		proxy := clashProxy(node, user)
		if proxy != "" {
			sb.WriteString(proxy)
			proxyNames = append(proxyNames, node.Name)
		}
	}

	sb.WriteString("\nproxy-groups:\n")
	if len(proxyNames) > 0 {
		sb.WriteString("  - name: Proxy\n    type: select\n    proxies:\n")
		for _, name := range proxyNames {
			sb.WriteString(fmt.Sprintf("      - %s\n", name))
		}
		sb.WriteString("      - DIRECT\n")

		sb.WriteString("  - name: Auto\n    type: url-test\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n    proxies:\n")
		for _, name := range proxyNames {
			sb.WriteString(fmt.Sprintf("      - %s\n", name))
		}
	}

	sb.WriteString("\nrules:\n")
	sb.WriteString("  - GEOIP,CN,DIRECT\n")
	sb.WriteString("  - MATCH,Proxy\n")

	return sb.String(), "text/yaml; charset=utf-8", nil
}

func clashProxy(node model.Node, user *model.User) string {
	var settings map[string]interface{}
	json.Unmarshal([]byte(node.Settings), &settings)

	sni, _ := settings["sni"].(string)
	var sb strings.Builder

	switch node.Protocol {
	case "vless":
		sb.WriteString(fmt.Sprintf("  - name: %s\n    type: vless\n    server: %s\n    port: %d\n    uuid: %s\n",
			node.Name, node.Host, node.Port, user.UUID))
		if node.Transport == "reality" {
			sb.WriteString("    network: tcp\n    tls: true\n    udp: true\n")
			sb.WriteString(fmt.Sprintf("    servername: %s\n", sni))
			sb.WriteString("    reality-opts:\n")
			if pbk, ok := settings["public_key"].(string); ok {
				sb.WriteString(fmt.Sprintf("      public-key: %s\n", pbk))
			}
			if sid, ok := settings["short_id"].(string); ok {
				sb.WriteString(fmt.Sprintf("      short-id: %s\n", sid))
			}
			sb.WriteString("    client-fingerprint: chrome\n")
		} else if node.Transport == "ws" {
			sb.WriteString("    network: ws\n    tls: true\n    udp: true\n")
			if wsPath, ok := settings["path"].(string); ok {
				sb.WriteString(fmt.Sprintf("    ws-opts:\n      path: %s\n", wsPath))
			}
		} else {
			sb.WriteString("    network: tcp\n    udp: true\n")
		}

	case "vmess":
		sb.WriteString(fmt.Sprintf("  - name: %s\n    type: vmess\n    server: %s\n    port: %d\n    uuid: %s\n    alterId: 0\n    cipher: auto\n",
			node.Name, node.Host, node.Port, user.UUID))
		if node.Transport == "ws" {
			sb.WriteString("    network: ws\n")
			if wsPath, ok := settings["path"].(string); ok {
				sb.WriteString(fmt.Sprintf("    ws-opts:\n      path: %s\n", wsPath))
			}
		}
		if sni != "" {
			sb.WriteString("    tls: true\n")
			sb.WriteString(fmt.Sprintf("    servername: %s\n", sni))
		}

	case "trojan":
		sb.WriteString(fmt.Sprintf("  - name: %s\n    type: trojan\n    server: %s\n    port: %d\n    password: %s\n    udp: true\n",
			node.Name, node.Host, node.Port, user.UUID))
		if sni != "" {
			sb.WriteString(fmt.Sprintf("    sni: %s\n", sni))
		}

	case "ss":
		method, _ := settings["method"].(string)
		if method == "" {
			method = "aes-256-gcm"
		}
		sb.WriteString(fmt.Sprintf("  - name: %s\n    type: ss\n    server: %s\n    port: %d\n    cipher: %s\n    password: %s\n    udp: true\n",
			node.Name, node.Host, node.Port, method, user.UUID))

	case "hysteria2":
		sb.WriteString(fmt.Sprintf("  - name: %s\n    type: hysteria2\n    server: %s\n    port: %d\n    password: %s\n    udp: true\n",
			node.Name, node.Host, node.Port, user.UUID))
		if sni != "" {
			sb.WriteString(fmt.Sprintf("    sni: %s\n", sni))
		}
		skipVerify, _ := settings["skip_cert_verify"].(bool)
		if skipVerify {
			sb.WriteString("    skip-cert-verify: true\n")
		}

	default:
		return ""
	}

	return sb.String()
}
```

- [ ] **Step 4: 创建 V2Ray URI 格式生成器**

创建 `internal/service/subscription/v2ray.go`：

```go
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"proxy-panel/internal/model"
)

type V2RayGenerator struct{}

func (g *V2RayGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var lines []string
	for _, node := range nodes {
		uri := v2rayURI(node, user)
		if uri != "" {
			lines = append(lines, uri)
		}
	}
	content := base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
	return content, "text/plain; charset=utf-8", nil
}

func v2rayURI(node model.Node, user *model.User) string {
	var settings map[string]interface{}
	json.Unmarshal([]byte(node.Settings), &settings)

	sni, _ := settings["sni"].(string)

	switch node.Protocol {
	case "vless":
		params := url.Values{}
		params.Set("type", node.Transport)
		if node.Transport == "reality" {
			params.Set("type", "tcp")
			params.Set("security", "reality")
			if pbk, ok := settings["public_key"].(string); ok {
				params.Set("pbk", pbk)
			}
			if sid, ok := settings["short_id"].(string); ok {
				params.Set("sid", sid)
			}
			if sni != "" {
				params.Set("sni", sni)
			}
			params.Set("fp", "chrome")
			params.Set("flow", "xtls-rprx-vision")
		} else if node.Transport == "ws" {
			params.Set("security", "tls")
			if sni != "" {
				params.Set("sni", sni)
			}
			if path, ok := settings["path"].(string); ok {
				params.Set("path", path)
			}
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
			user.UUID, node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	case "vmess":
		vmessObj := map[string]interface{}{
			"v":    "2",
			"ps":   node.Name,
			"add":  node.Host,
			"port": node.Port,
			"id":   user.UUID,
			"aid":  0,
			"net":  node.Transport,
			"type": "none",
		}
		if sni != "" {
			vmessObj["tls"] = "tls"
			vmessObj["sni"] = sni
		}
		if node.Transport == "ws" {
			vmessObj["net"] = "ws"
			if path, ok := settings["path"].(string); ok {
				vmessObj["path"] = path
			}
			if host, ok := settings["host"].(string); ok {
				vmessObj["host"] = host
			}
		}
		data, _ := json.Marshal(vmessObj)
		return "vmess://" + base64.StdEncoding.EncodeToString(data)

	case "trojan":
		params := url.Values{}
		if sni != "" {
			params.Set("sni", sni)
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
			user.UUID, node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	case "ss":
		method, _ := settings["method"].(string)
		if method == "" {
			method = "aes-256-gcm"
		}
		userInfo := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", method, user.UUID)))
		return fmt.Sprintf("ss://%s@%s:%d#%s", userInfo, node.Host, node.Port, url.QueryEscape(node.Name))

	case "hysteria2":
		params := url.Values{}
		if sni != "" {
			params.Set("sni", sni)
		}
		if obfs, ok := settings["obfs"].(string); ok && obfs != "" {
			params.Set("obfs", obfs)
		}
		return fmt.Sprintf("hysteria2://%s@%s:%d?%s#%s",
			user.UUID, node.Host, node.Port, params.Encode(), url.QueryEscape(node.Name))

	default:
		return ""
	}
}
```

- [ ] **Step 5: 创建 Shadowrocket 格式生成器**

创建 `internal/service/subscription/shadowrocket.go`：

```go
package subscription

import (
	"encoding/base64"
	"strings"

	"proxy-panel/internal/model"
)

type ShadowrocketGenerator struct{}

func (g *ShadowrocketGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	// Shadowrocket 使用与 V2Ray 相同的 URI 格式，只是可能有些微差异
	var lines []string
	for _, node := range nodes {
		uri := v2rayURI(node, user) // 复用 V2Ray URI 生成逻辑
		if uri != "" {
			lines = append(lines, uri)
		}
	}
	content := base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
	return content, "text/plain; charset=utf-8", nil
}
```

- [ ] **Step 6: 创建 Sing-box JSON 格式生成器**

创建 `internal/service/subscription/singbox.go`：

```go
package subscription

import (
	"encoding/json"
	"fmt"

	"proxy-panel/internal/model"
)

type SingboxGenerator struct{}

func (g *SingboxGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var outbounds []map[string]interface{}

	// 用户节点
	var tags []string
	for _, node := range nodes {
		ob := singboxOutbound(node, user)
		if ob != nil {
			outbounds = append(outbounds, ob)
			tags = append(tags, node.Name)
		}
	}

	// selector
	selectorOutbound := map[string]interface{}{
		"type":      "selector",
		"tag":       "proxy",
		"outbounds": append(tags, "direct"),
		"default":   tags[0],
	}

	// auto (urltest)
	autoOutbound := map[string]interface{}{
		"type":      "urltest",
		"tag":       "auto",
		"outbounds": tags,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  "5m",
	}

	// direct + block
	directOutbound := map[string]interface{}{"type": "direct", "tag": "direct"}
	blockOutbound := map[string]interface{}{"type": "block", "tag": "block"}
	dnsOutbound := map[string]interface{}{"type": "dns", "tag": "dns-out"}

	allOutbounds := []map[string]interface{}{selectorOutbound, autoOutbound}
	allOutbounds = append(allOutbounds, outbounds...)
	allOutbounds = append(allOutbounds, directOutbound, blockOutbound, dnsOutbound)

	config := map[string]interface{}{
		"outbounds": allOutbounds,
		"route": map[string]interface{}{
			"rules": []map[string]interface{}{
				{"protocol": "dns", "outbound": "dns-out"},
				{"geoip": []string{"cn"}, "outbound": "direct"},
				{"geosite": []string{"cn"}, "outbound": "direct"},
			},
			"final": "proxy",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	return string(data), "application/json; charset=utf-8", err
}

func singboxOutbound(node model.Node, user *model.User) map[string]interface{} {
	var settings map[string]interface{}
	json.Unmarshal([]byte(node.Settings), &settings)

	sni, _ := settings["sni"].(string)

	ob := map[string]interface{}{
		"tag":    node.Name,
		"server": node.Host,
		"server_port": node.Port,
	}

	switch node.Protocol {
	case "vless":
		ob["type"] = "vless"
		ob["uuid"] = user.UUID
		if node.Transport == "reality" {
			ob["flow"] = "xtls-rprx-vision"
			ob["tls"] = map[string]interface{}{
				"enabled":     true,
				"server_name": sni,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": settings["public_key"],
					"short_id":   settings["short_id"],
				},
			}
		} else if node.Transport == "ws" {
			ob["transport"] = map[string]interface{}{
				"type": "ws",
				"path": settings["path"],
			}
			if sni != "" {
				ob["tls"] = map[string]interface{}{"enabled": true, "server_name": sni}
			}
		}

	case "vmess":
		ob["type"] = "vmess"
		ob["uuid"] = user.UUID
		ob["alter_id"] = 0
		if node.Transport == "ws" {
			ob["transport"] = map[string]interface{}{"type": "ws", "path": settings["path"]}
		}
		if sni != "" {
			ob["tls"] = map[string]interface{}{"enabled": true, "server_name": sni}
		}

	case "trojan":
		ob["type"] = "trojan"
		ob["password"] = user.UUID
		if sni != "" {
			ob["tls"] = map[string]interface{}{"enabled": true, "server_name": sni}
		}

	case "ss":
		ob["type"] = "shadowsocks"
		method, _ := settings["method"].(string)
		if method == "" {
			method = "aes-256-gcm"
		}
		ob["method"] = method
		ob["password"] = user.UUID

	case "hysteria2":
		ob["type"] = "hysteria2"
		ob["password"] = user.UUID
		if sni != "" {
			ob["tls"] = map[string]interface{}{"enabled": true, "server_name": sni}
		}
		skipVerify, _ := settings["skip_cert_verify"].(bool)
		if skipVerify {
			if tlsCfg, ok := ob["tls"].(map[string]interface{}); ok {
				tlsCfg["insecure"] = true
			} else {
				ob["tls"] = map[string]interface{}{"enabled": true, "insecure": true}
			}
		}

	default:
		return nil
	}

	return ob
}
```

- [ ] **Step 7: 创建订阅 handler**

创建 `internal/handler/subscription.go`：

```go
package handler

import (
	"fmt"
	"net/http"

	"proxy-panel/internal/service"
	"proxy-panel/internal/service/subscription"

	"github.com/gin-gonic/gin"
)

type SubscriptionHandler struct {
	userSvc *service.UserService
	nodeSvc *service.NodeService
}

func NewSubscriptionHandler(userSvc *service.UserService, nodeSvc *service.NodeService) *SubscriptionHandler {
	return &SubscriptionHandler{userSvc: userSvc, nodeSvc: nodeSvc}
}

func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	format := c.DefaultQuery("format", "v2ray")

	// 查询用户
	user, err := h.userSvc.GetByUUID(uuid)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在", "code": "ERR_NOT_FOUND"})
		return
	}

	if !user.Enable {
		c.JSON(http.StatusForbidden, gin.H{"error": "用户已停用", "code": "ERR_DISABLED"})
		return
	}

	// 检查流量
	if user.TrafficLimit > 0 && user.TrafficUsed >= user.TrafficLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "流量已耗尽", "code": "ERR_TRAFFIC_EXHAUSTED"})
		return
	}

	// 获取启用的节点
	nodes, err := h.nodeSvc.ListEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点失败", "code": "ERR_INTERNAL"})
		return
	}

	// 生成订阅
	gen := subscription.GetGenerator(format)
	baseURL := fmt.Sprintf("%s://%s", scheme(c), c.Request.Host)
	content, contentType, err := gen.Generate(nodes, user, baseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败", "code": "ERR_INTERNAL"})
		return
	}

	// Subscription-Userinfo header
	var expire int64
	if user.ExpiresAt != nil {
		expire = user.ExpiresAt.Unix()
	}
	c.Header("Subscription-Userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d",
		user.TrafficUp, user.TrafficDown, user.TrafficLimit, expire))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", user.Username))

	c.Data(http.StatusOK, contentType, []byte(content))
}

func scheme(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}
```

- [ ] **Step 8: 注册订阅路由**

在 `router.go` 中添加：

```go
subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc)
api.GET("/sub/:uuid", subLimiter.Limit(), subHandler.Subscribe)
```

- [ ] **Step 9: 验证编译并提交**

```bash
go build ./...
git add internal/service/subscription/ internal/handler/subscription.go internal/router/router.go
git commit -m "feat: 五格式订阅生成 (Surge/Clash/V2Ray/Shadowrocket/Sing-box)"
```

---

## Phase 7: 设置管理 + 完善路由

### Task 10: 设置 Handler + 完善 Router 整合

**Files:**
- Create: `internal/handler/setting.go`
- Modify: `internal/router/router.go` (最终版)
- Modify: `cmd/server/main.go` (最终版)

- [ ] **Step 1: 创建设置 handler**

创建 `internal/handler/setting.go`：

```go
package handler

import (
	"net/http"

	"proxy-panel/internal/database"

	"github.com/gin-gonic/gin"
)

type SettingHandler struct {
	db *database.DB
}

func NewSettingHandler(db *database.DB) *SettingHandler {
	return &SettingHandler{db: db}
}

func (h *SettingHandler) Get(c *gin.Context) {
	rows, err := h.db.Query("SELECT key, value FROM settings")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		rows.Scan(&key, &value)
		settings[key] = value
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) Update(c *gin.Context) {
	var settings map[string]string
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	for key, value := range settings {
		_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value=?`, key, value, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败", "code": "ERR_INTERNAL"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "保存成功"})
}
```

- [ ] **Step 2: 完善 router.go (注册所有路由)**

更新 `internal/router/router.go` 为最终版，将所有 handler 注册到对应路由。

- [ ] **Step 3: 完善 main.go (初始化所有组件)**

更新 `cmd/server/main.go`，初始化 Manager、所有 Service、Scheduler，注入到 Router。

- [ ] **Step 4: 验证编译并提交**

```bash
go build ./...
git add .
git commit -m "feat: 设置管理 + 完善路由整合 + 组件初始化"
```

---

## Phase 8: Vue 3 前端

### Task 11: 前端项目初始化

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tailwind.config.js`
- Create: `web/index.html`
- Create: `web/src/main.ts`
- Create: `web/src/App.vue`

- [ ] **Step 1: 初始化 Vue 3 项目**

```bash
cd /Users/huangyuchuan/Desktop/proxy_panel
npm create vite@latest web -- --template vue-ts
cd web
npm install
npm install element-plus @element-plus/icons-vue
npm install tailwindcss @tailwindcss/vite
npm install pinia vue-router@4 axios echarts
```

- [ ] **Step 2: 配置 Vite + TailwindCSS**

更新 `web/vite.config.ts`：

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
```

- [ ] **Step 3: 配置 main.ts**

```ts
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './style.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.mount('#app')
```

更新 `web/src/style.css`：

```css
@import "tailwindcss";
```

- [ ] **Step 4: 验证开发服务器启动**

```bash
cd web && npm run dev
```

预期：浏览器能访问 http://localhost:5173

- [ ] **Step 5: 提交**

```bash
git add web/
git commit -m "feat: Vue 3 前端项目初始化 (Element Plus + TailwindCSS)"
```

---

### Task 12: API 封装 + Auth Store + Router

**Files:**
- Create: `web/src/api/request.ts`
- Create: `web/src/api/auth.ts`
- Create: `web/src/api/user.ts`
- Create: `web/src/api/node.ts`
- Create: `web/src/api/dashboard.ts`
- Create: `web/src/api/traffic.ts`
- Create: `web/src/api/setting.ts`
- Create: `web/src/api/notify.ts`
- Create: `web/src/stores/auth.ts`
- Create: `web/src/router/index.ts`

- [ ] **Step 1: 创建 axios 封装**

创建 `web/src/api/request.ts`：

```ts
import axios from 'axios'
import { useAuthStore } from '../stores/auth'
import router from '../router'

const request = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

request.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

request.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      const auth = useAuthStore()
      auth.logout()
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

export default request
```

- [ ] **Step 2: 创建各模块 API**

创建 `web/src/api/auth.ts`：

```ts
import request from './request'

export const login = (username: string, password: string) =>
  request.post('/auth/login', { username, password })
```

创建 `web/src/api/user.ts`：

```ts
import request from './request'

export const getUsers = () => request.get('/users')
export const getUser = (id: number) => request.get(`/users/${id}`)
export const createUser = (data: any) => request.post('/users', data)
export const updateUser = (id: number, data: any) => request.put(`/users/${id}`, data)
export const deleteUser = (id: number) => request.delete(`/users/${id}`)
export const resetTraffic = (id: number) => request.post(`/users/${id}/reset-traffic`)
```

创建 `web/src/api/node.ts`：

```ts
import request from './request'

export const getNodes = () => request.get('/nodes')
export const createNode = (data: any) => request.post('/nodes', data)
export const updateNode = (id: number, data: any) => request.put(`/nodes/${id}`, data)
export const deleteNode = (id: number) => request.delete(`/nodes/${id}`)
```

创建 `web/src/api/dashboard.ts`：

```ts
import request from './request'

export const getDashboard = () => request.get('/dashboard')
```

创建 `web/src/api/traffic.ts`：

```ts
import request from './request'

export const getServerTraffic = () => request.get('/traffic/server')
export const setServerLimit = (limitGB: number) => request.post('/traffic/server/limit', { limit_gb: limitGB })
export const getTrafficHistory = (days: number = 30) => request.get('/traffic/history', { params: { days } })
```

创建 `web/src/api/setting.ts`：

```ts
import request from './request'

export const getSettings = () => request.get('/settings')
export const updateSettings = (data: Record<string, string>) => request.put('/settings', data)
```

创建 `web/src/api/notify.ts`：

```ts
import request from './request'

export const testNotify = (channel?: string) => request.post('/notify/test', { channel })
```

- [ ] **Step 3: 创建 Auth Store**

创建 `web/src/stores/auth.ts`：

```ts
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')

  function setToken(t: string) {
    token.value = t
    localStorage.setItem('token', t)
  }

  function logout() {
    token.value = ''
    localStorage.removeItem('token')
  }

  const isLoggedIn = () => !!token.value

  return { token, setToken, logout, isLoggedIn }
})
```

- [ ] **Step 4: 创建路由**

创建 `web/src/router/index.ts`：

```ts
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
  {
    path: '/',
    component: () => import('../components/Layout.vue'),
    children: [
      { path: '', name: 'Dashboard', component: () => import('../views/Dashboard.vue') },
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
      { path: 'nodes', name: 'Nodes', component: () => import('../views/Nodes.vue') },
      { path: 'traffic', name: 'Traffic', component: () => import('../views/Traffic.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.name !== 'Login' && !auth.isLoggedIn()) {
    return { name: 'Login' }
  }
})

export default router
```

- [ ] **Step 5: 提交**

```bash
git add web/src/api/ web/src/stores/ web/src/router/
git commit -m "feat: API 封装 + Auth Store + 路由守卫"
```

---

### Task 13: Layout + Login 页面

**Files:**
- Create: `web/src/components/Layout.vue`
- Create: `web/src/views/Login.vue`
- Create: `web/src/utils/format.ts`
- Modify: `web/src/App.vue`

- [ ] **Step 1: 创建 Layout 组件**

创建 `web/src/components/Layout.vue`，包含 Element Plus 的 el-container + el-aside (侧边栏菜单) + el-main 布局。侧边栏包含：仪表盘、用户管理、节点管理、流量统计、系统设置五个菜单项。

- [ ] **Step 2: 创建登录页面**

创建 `web/src/views/Login.vue`，包含用户名 + 密码表单，调用 login API，成功后跳转仪表盘。

- [ ] **Step 3: 创建工具函数**

创建 `web/src/utils/format.ts`：

```ts
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

export function formatDate(date: string | null): string {
  if (!date) return '-'
  return new Date(date).toLocaleDateString('zh-CN')
}
```

- [ ] **Step 4: 更新 App.vue**

```vue
<template>
  <router-view />
</template>
```

- [ ] **Step 5: 提交**

```bash
git add web/src/components/Layout.vue web/src/views/Login.vue web/src/utils/ web/src/App.vue
git commit -m "feat: Layout 布局 + 登录页面"
```

---

### Task 14: Dashboard 页面

**Files:**
- Create: `web/src/views/Dashboard.vue`
- Create: `web/src/components/TrafficChart.vue`

- [ ] **Step 1: 创建仪表盘页面**

创建 `web/src/views/Dashboard.vue`，包含：
- 统计卡片行：用户总数/活跃数、节点总数/在线数、今日流量、服务器总流量占比
- 内核状态卡片：Xray/Sing-box 运行状态 + 重启按钮
- 流量趋势图 (TrafficChart 组件)

- [ ] **Step 2: 创建流量图表组件**

创建 `web/src/components/TrafficChart.vue`，使用 ECharts 渲染近 30 天的上行/下行流量柱状图。

- [ ] **Step 3: 提交**

```bash
git add web/src/views/Dashboard.vue web/src/components/TrafficChart.vue
git commit -m "feat: 仪表盘页面 + 流量图表"
```

---

### Task 15: Users 页面

**Files:**
- Create: `web/src/views/Users.vue`
- Create: `web/src/components/SubscriptionDialog.vue`

- [ ] **Step 1: 创建用户管理页面**

创建 `web/src/views/Users.vue`，包含：
- 用户列表表格 (el-table)：显示用户名、协议、流量已用/限额、状态、到期时间、操作
- 新增/编辑用户对话框 (el-dialog + el-form)
- 操作列：编辑、删除、重置流量、查看订阅
- 流量进度条

- [ ] **Step 2: 创建订阅链接弹窗**

创建 `web/src/components/SubscriptionDialog.vue`，包含：
- 5 种客户端格式的订阅链接展示
- 每种格式的一键复制按钮
- 订阅链接二维码展示 (可选用 qrcode 库)

- [ ] **Step 3: 提交**

```bash
git add web/src/views/Users.vue web/src/components/SubscriptionDialog.vue
git commit -m "feat: 用户管理页面 + 订阅链接弹窗"
```

---

### Task 16: Nodes + Traffic + Settings 页面

**Files:**
- Create: `web/src/views/Nodes.vue`
- Create: `web/src/views/Traffic.vue`
- Create: `web/src/views/Settings.vue`

- [ ] **Step 1: 创建节点管理页面**

创建 `web/src/views/Nodes.vue`，包含：
- 节点列表表格：名称、地址:端口、协议、传输、内核类型、状态、操作
- 新增/编辑节点对话框：包含协议特定配置 (JSON 编辑器或动态表单)

- [ ] **Step 2: 创建流量统计页面**

创建 `web/src/views/Traffic.vue`，包含：
- 服务器总流量卡片 + 限额设置
- 历史流量图表 (支持按用户/全局筛选)
- 流量日志表格

- [ ] **Step 3: 创建系统设置页面**

创建 `web/src/views/Settings.vue`，包含：
- Telegram 配置 (Bot Token + Chat ID + 测试按钮)
- 企业微信配置 (Webhook URL + 测试按钮)
- 流量采集间隔设置
- 告警阈值设置

- [ ] **Step 4: 构建前端并验证**

```bash
cd web && npm run build
```

预期：`web/dist/` 目录生成。

- [ ] **Step 5: 提交**

```bash
git add web/src/views/Nodes.vue web/src/views/Traffic.vue web/src/views/Settings.vue
git commit -m "feat: 节点管理 + 流量统计 + 系统设置页面"
```

---

## Phase 9: 部署脚本

### Task 17: 一键部署脚本

**Files:**
- Create: `scripts/install.sh`

- [ ] **Step 1: 创建 install.sh**

创建 `scripts/install.sh`，实现以下功能：
- `check_root` - 检查 root 权限
- `detect_os` - 检测 Ubuntu/Debian/CentOS
- `detect_arch` - 检测 amd64/arm64
- `install_deps` - 安装 curl, jq, sqlite3
- `download_xray` - 从 GitHub Release 下载 Xray
- `download_singbox` - 从 GitHub Release 下载 Sing-box
- `download_panel` - 下载面板二进制 + 前端文件
- `interactive_config` - 交互式询问端口/密码/TLS方案/TG Bot/流量限额
- `setup_tls` - 根据选择执行证书方案 (Cloudflare/acme.sh/无)
- `generate_config_yaml` - 生成配置文件
- `init_database` - 初始化数据库
- `setup_systemd` - 注册 systemd 服务
- `setup_firewall` - 放行端口 (ufw/firewalld)
- `start_services` - 启动服务
- `print_summary` - 打印访问信息

子命令：install / update / uninstall / status / restart / logs / reset-pwd / backup / restore / cert

- [ ] **Step 2: 测试脚本语法**

```bash
bash -n scripts/install.sh
```

- [ ] **Step 3: 提交**

```bash
git add scripts/install.sh
git commit -m "feat: 一键部署脚本 (含三种 TLS 方案)"
```

---

## Phase 10: 集成测试 + 最终整合

### Task 18: 最终整合 + 端到端验证

**Files:**
- Modify: `cmd/server/main.go` (确保所有组件正确连接)
- Modify: `internal/router/router.go` (确保所有路由正确注册)

- [ ] **Step 1: 确保后端编译通过**

```bash
go build -o proxy-panel ./cmd/server/
```

- [ ] **Step 2: 确保前端构建通过**

```bash
cd web && npm run build
```

- [ ] **Step 3: 复制前端产物到正确位置**

```bash
cp -r web/dist/* web/
```

或修改 Go 后端指向 `web/dist/`。

- [ ] **Step 4: 使用示例配置启动服务**

```bash
cp config.example.yaml config.yaml
./proxy-panel -config config.yaml
```

预期：服务启动，能访问 http://localhost:8080

- [ ] **Step 5: 验证核心流程**
- 登录 API: `POST /api/auth/login`
- 创建用户: `POST /api/users`
- 创建节点: `POST /api/nodes`
- 获取订阅: `GET /api/sub/:uuid?format=surge`
- 仪表盘: `GET /api/dashboard`

- [ ] **Step 6: 最终提交**

```bash
git add .
git commit -m "feat: ProxyPanel v1.0 MVP 完成"
```

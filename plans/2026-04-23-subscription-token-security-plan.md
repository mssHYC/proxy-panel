# 订阅 Token 安全 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把订阅凭证从 `User.UUID` 解耦成独立的 `subscription_tokens` 表，支持多 token、过期、IP 绑定、轮换、UA 自动识别；现有 `/api/sub/:uuid` 链接继续可用。

**Architecture:** 新增 `internal/model/subscription_token.go` 模型 + `internal/service/subscription_token.go` 业务层（含 Repository）。订阅处理器改为先按 token 查表，再路由到原生成逻辑。新增一组受 JWT 保护的 `/api/sub-tokens` CRUD 端点。迁移脚本为每个现有用户注入 `name='default'`、`token = uuid`、`ip_bind_enabled=0` 的记录，旧端点通过查 `subscription_tokens` 表获得一致行为。前端在用户列表加 Drawer 做 token 管理。

**Tech Stack:** Go 1.22 + Gin + SQLite + Vue 3 + Element Plus

**Spec:** [specs/2026-04-23-subscription-token-security-design.md](../specs/2026-04-23-subscription-token-security-design.md)

---

## 文件结构

**创建：**
- `internal/model/subscription_token.go` — `SubscriptionToken` 结构体
- `internal/service/subscription_token.go` — 服务层（Repository + 业务）
- `internal/service/subscription_token_test.go` — 服务层测试（随机串长度、过期判定、IP 绑定原子性）
- `internal/service/subscription/ua.go` — UA 识别
- `internal/service/subscription/ua_test.go` — UA 识别表测试
- `internal/handler/subscription_token.go` — 管理 CRUD handler
- `web/src/api/sub-tokens.js` — 前端 API 客户端
- `web/src/components/SubTokenDrawer.vue` — token 管理抽屉
- `specs/2026-04-23-subscription-token-security-design.md` — (已存在) 设计文档

**修改：**
- `internal/database/migrations.go` — 新增建表 SQL + 数据迁移
- `internal/handler/subscription.go` — 改写为 token-first；保留旧端点
- `internal/router/router.go` — 新增 `/api/sub/t/:token` 与 `/api/sub-tokens` CRUD
- `internal/router/middleware.go` — `SubRateLimiter.Limit()` 支持 `token` 或 `uuid` 作为 key
- `cmd/server/main.go` — 实例化 `SubscriptionTokenService` 并注入
- `web/src/views/Users.vue` — 每行加「订阅 Token」按钮触发 Drawer

---

### Task 1: 数据库迁移 — 建表 + 数据迁移

**Files:**
- Modify: `internal/database/migrations.go`

- [ ] **Step 1: 在 `migrate()` 的 queries 切片末尾追加建表与索引**

在 `internal/database/migrations.go:92` 的 `CREATE INDEX idx_audit_actor` 后、切片结束 `}` 前插入：

```go
		`CREATE TABLE IF NOT EXISTS subscription_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			expires_at DATETIME,
			ip_bind_enabled INTEGER NOT NULL DEFAULT 1,
			bound_ip TEXT,
			last_ip TEXT,
			last_ua TEXT,
			last_used_at DATETIME,
			use_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sub_tokens_user ON subscription_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sub_tokens_token ON subscription_tokens(token)`,
		// 为存量用户注入 default token（token = uuid，ip_bind_enabled=0 保持旧行为）
		`INSERT INTO subscription_tokens (user_id, name, token, enabled, ip_bind_enabled, created_at)
		 SELECT id, 'default', uuid, 1, 0, created_at FROM users
		 WHERE NOT EXISTS (SELECT 1 FROM subscription_tokens st WHERE st.user_id = users.id)`,
```

- [ ] **Step 2: 编译确认**

Run: `go build ./...`
Expected: 无错误。

- [ ] **Step 3: 启动服务并验证表结构**

Run: `go run ./cmd/server -config config.yaml`（本地需有 config.yaml；或跑 `sqlite3 data/panel.db '.schema subscription_tokens'`）
Expected: 表存在；若 users 表里有数据，`SELECT COUNT(*) FROM subscription_tokens` 等于用户数。

- [ ] **Step 4: Commit**

```bash
git add internal/database/migrations.go
git commit -m "feat(db): 新增 subscription_tokens 表及默认数据迁移"
```

---

### Task 2: 定义 `SubscriptionToken` 模型

**Files:**
- Create: `internal/model/subscription_token.go`

- [ ] **Step 1: 写模型**

```go
package model

import "time"

// SubscriptionToken 订阅凭证，与 User 解耦；一个用户可有多个。
type SubscriptionToken struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Name           string     `json:"name"`
	Token          string     `json:"token"`
	Enabled        bool       `json:"enabled"`
	ExpiresAt      *time.Time `json:"expires_at"`
	IPBindEnabled  bool       `json:"ip_bind_enabled"`
	BoundIP        string     `json:"bound_ip"`
	LastIP         string     `json:"last_ip"`
	LastUA         string     `json:"last_ua"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	UseCount       int64      `json:"use_count"`
	CreatedAt      time.Time  `json:"created_at"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/subscription_token.go
git commit -m "feat(model): 新增 SubscriptionToken 结构体"
```

---

### Task 3: UA 识别 + 测试

**Files:**
- Create: `internal/service/subscription/ua.go`
- Create: `internal/service/subscription/ua_test.go`

- [ ] **Step 1: 先写测试**

```go
package subscription

import "testing"

func TestSniffFormat(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"Surge iOS/2589", "surge"},
		{"Shadowrocket/1993", "shadowrocket"},
		{"Quantumult X", "shadowrocket"},
		{"ClashX Pro/1.95", "clash"},
		{"Clash/1.0", "clash"},
		{"Stash/2.6", "clash"},
		{"mihomo/1.18", "clash"},
		{"sing-box 1.9.3", "singbox"},
		{"SingBox/1.9", "singbox"},
		{"v2rayN/6.30", "v2ray"},
		{"V2Box 2.1", "v2ray"},
		{"Mozilla/5.0 (Macintosh)", ""},
		{"", ""},
		{"curl/8.6.0", ""},
	}
	for _, c := range cases {
		got := SniffFormat(c.ua)
		if got != c.want {
			t.Errorf("SniffFormat(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/subscription -run TestSniffFormat`
Expected: FAIL（undefined SniffFormat）。

- [ ] **Step 3: 实现**

```go
package subscription

import "regexp"

var uaPatterns = []struct {
	re     *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`(?i)surge`), "surge"},
	{regexp.MustCompile(`(?i)shadowrocket`), "shadowrocket"},
	{regexp.MustCompile(`(?i)quantumult`), "shadowrocket"},
	{regexp.MustCompile(`(?i)clash|stash|mihomo`), "clash"},
	{regexp.MustCompile(`(?i)sing-?box`), "singbox"},
	{regexp.MustCompile(`(?i)v2ray|v2box`), "v2ray"},
}

// SniffFormat 依据 User-Agent 识别订阅格式，未识别返回空串。
func SniffFormat(ua string) string {
	for _, p := range uaPatterns {
		if p.re.MatchString(ua) {
			return p.format
		}
	}
	return ""
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/subscription -run TestSniffFormat -v`
Expected: PASS，所有 14 用例。

- [ ] **Step 5: Commit**

```bash
git add internal/service/subscription/ua.go internal/service/subscription/ua_test.go
git commit -m "feat(subscription): UA 识别客户端格式"
```

---

### Task 4: SubscriptionTokenService — Repository + 业务

**Files:**
- Create: `internal/service/subscription_token.go`

先只实现 CRUD 和随机串生成；校验/绑定的复杂逻辑在 Task 5 加。

- [ ] **Step 1: 写骨架**

```go
package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
)

type SubscriptionTokenService struct {
	db *database.DB
}

func NewSubscriptionTokenService(db *database.DB) *SubscriptionTokenService {
	return &SubscriptionTokenService{db: db}
}

// GenerateToken 32 字节 crypto/rand → base64url 无填充（长度 43）。
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *SubscriptionTokenService) ListByUser(userID int64) ([]model.SubscriptionToken, error) {
	rows, err := s.db.Query(`SELECT id, user_id, name, token, enabled, expires_at,
		ip_bind_enabled, COALESCE(bound_ip,''), COALESCE(last_ip,''), COALESCE(last_ua,''),
		last_used_at, use_count, created_at
		FROM subscription_tokens WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SubscriptionToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *SubscriptionTokenService) GetByToken(token string) (*model.SubscriptionToken, error) {
	row := s.db.QueryRow(`SELECT id, user_id, name, token, enabled, expires_at,
		ip_bind_enabled, COALESCE(bound_ip,''), COALESCE(last_ip,''), COALESCE(last_ua,''),
		last_used_at, use_count, created_at
		FROM subscription_tokens WHERE token = ?`, token)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (s *SubscriptionTokenService) GetByID(id int64) (*model.SubscriptionToken, error) {
	row := s.db.QueryRow(`SELECT id, user_id, name, token, enabled, expires_at,
		ip_bind_enabled, COALESCE(bound_ip,''), COALESCE(last_ip,''), COALESCE(last_ua,''),
		last_used_at, use_count, created_at
		FROM subscription_tokens WHERE id = ?`, id)
	t, err := scanToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

type CreateTokenReq struct {
	Name          string     `json:"name" binding:"required"`
	ExpiresAt     *time.Time `json:"expires_at"`
	IPBindEnabled *bool      `json:"ip_bind_enabled"` // 指针便于区分"未传=默认 true"与"传 false"
}

func (s *SubscriptionTokenService) Create(userID int64, req *CreateTokenReq) (*model.SubscriptionToken, error) {
	tok, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	bind := true
	if req.IPBindEnabled != nil {
		bind = *req.IPBindEnabled
	}
	res, err := s.db.Exec(`INSERT INTO subscription_tokens
		(user_id, name, token, enabled, expires_at, ip_bind_enabled, created_at)
		VALUES (?, ?, ?, 1, ?, ?, CURRENT_TIMESTAMP)`,
		userID, req.Name, tok, req.ExpiresAt, boolToInt(bind))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetByID(id)
}

type UpdateTokenReq struct {
	Name          *string    `json:"name"`
	Enabled       *bool      `json:"enabled"`
	ExpiresAt     *time.Time `json:"expires_at"`
	ExpiresNull   bool       `json:"expires_at_null"` // 显式传 true 表示清空
	IPBindEnabled *bool      `json:"ip_bind_enabled"`
	ResetBind     bool       `json:"reset_bind"`
}

func (s *SubscriptionTokenService) Update(id int64, req *UpdateTokenReq) (*model.SubscriptionToken, error) {
	cur, err := s.GetByID(id)
	if err != nil || cur == nil {
		return nil, err
	}
	sets := []string{}
	args := []any{}
	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*req.Enabled))
	}
	if req.ExpiresNull {
		sets = append(sets, "expires_at = NULL")
	} else if req.ExpiresAt != nil {
		sets = append(sets, "expires_at = ?")
		args = append(args, *req.ExpiresAt)
	}
	if req.IPBindEnabled != nil {
		sets = append(sets, "ip_bind_enabled = ?")
		args = append(args, boolToInt(*req.IPBindEnabled))
	}
	if req.ResetBind {
		sets = append(sets, "bound_ip = NULL")
	}
	if len(sets) == 0 {
		return cur, nil
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE subscription_tokens SET %s WHERE id = ?",
		joinComma(sets))
	if _, err := s.db.Exec(q, args...); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *SubscriptionTokenService) Rotate(id int64) (*model.SubscriptionToken, error) {
	tok, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(
		`UPDATE subscription_tokens SET token = ?, bound_ip = NULL WHERE id = ?`,
		tok, id); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

func (s *SubscriptionTokenService) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM subscription_tokens WHERE id = ?`, id)
	return err
}

// scanner 抽象 *sql.Row 与 *sql.Rows 的 Scan。
type scanner interface {
	Scan(dest ...any) error
}

func scanToken(r scanner) (*model.SubscriptionToken, error) {
	var t model.SubscriptionToken
	var enabled, ipBind int
	var expiresAt, lastUsedAt sql.NullTime
	err := r.Scan(&t.ID, &t.UserID, &t.Name, &t.Token, &enabled, &expiresAt,
		&ipBind, &t.BoundIP, &t.LastIP, &t.LastUA, &lastUsedAt, &t.UseCount, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	t.IPBindEnabled = ipBind != 0
	if expiresAt.Valid {
		v := expiresAt.Time
		t.ExpiresAt = &v
	}
	if lastUsedAt.Valid {
		v := lastUsedAt.Time
		t.LastUsedAt = &v
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}
```

- [ ] **Step 2: 编译确认**

Run: `go build ./...`
Expected: 无错误。

- [ ] **Step 3: Commit**

```bash
git add internal/service/subscription_token.go
git commit -m "feat(service): SubscriptionTokenService CRUD 骨架"
```

---

### Task 5: 订阅校验与审计 — `Validate` 与 `TouchAsync`

**Files:**
- Modify: `internal/service/subscription_token.go`
- Create: `internal/service/subscription_token_test.go`

- [ ] **Step 1: 追加错误常量与 Validate 方法**

在 `internal/service/subscription_token.go` 末尾追加：

```go
var (
	ErrTokenNotFound  = errors.New("token not found")
	ErrTokenDisabled  = errors.New("token disabled")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenIPBound   = errors.New("token bound to other ip")
)

// Validate 按 token 字符串完成全部校验，并在必要时原子绑定首访 IP。
// 返回校验通过的 token 快照（已含本次绑定后的 bound_ip）。
func (s *SubscriptionTokenService) Validate(tokenStr, clientIP string) (*model.SubscriptionToken, error) {
	t, err := s.GetByToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTokenNotFound
	}
	if !t.Enabled {
		return nil, ErrTokenDisabled
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	if !t.IPBindEnabled {
		return t, nil
	}
	if t.BoundIP == "" {
		// 原子尝试占位；若并发下被其它请求抢先，重新读取后再比对
		res, err := s.db.Exec(
			`UPDATE subscription_tokens SET bound_ip = ? WHERE id = ? AND bound_ip IS NULL`,
			clientIP, t.ID)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			t.BoundIP = clientIP
			return t, nil
		}
		// 被抢先，回查
		t, err = s.GetByID(t.ID)
		if err != nil {
			return nil, err
		}
	}
	if t.BoundIP != clientIP {
		return nil, ErrTokenIPBound
	}
	return t, nil
}

// TouchAsync 记录审计元信息，失败仅打日志不影响响应。
func (s *SubscriptionTokenService) TouchAsync(id int64, ip, ua string) {
	go func() {
		_, _ = s.db.Exec(
			`UPDATE subscription_tokens
			 SET last_ip = ?, last_ua = ?, last_used_at = CURRENT_TIMESTAMP, use_count = use_count + 1
			 WHERE id = ?`, ip, ua, id)
	}()
}
```

- [ ] **Step 2: 写测试**

```go
package service

import (
	"testing"
	"time"

	"proxy-panel/internal/database"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// users 表必须先插一条，subscription_tokens 有外键
	if _, err := db.Exec(`INSERT INTO users (uuid, username, protocol) VALUES ('u1', 'alice', 'vless')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return db
}

func TestGenerateTokenLength(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 {
		t.Errorf("token length = %d, want 43", len(tok))
	}
}

func TestValidate_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	if _, err := s.Validate("nope", "1.1.1.1"); err != ErrTokenNotFound {
		t.Errorf("want ErrTokenNotFound, got %v", err)
	}
}

func TestValidate_Disabled(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"})
	enabled := false
	s.Update(tok.ID, &UpdateTokenReq{Enabled: &enabled})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != ErrTokenDisabled {
		t.Errorf("want ErrTokenDisabled, got %v", err)
	}
}

func TestValidate_Expired(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	past := time.Now().Add(-time.Hour)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x", ExpiresAt: &past})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != ErrTokenExpired {
		t.Errorf("want ErrTokenExpired, got %v", err)
	}
}

func TestValidate_IPBind(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"}) // ip_bind_enabled 默认 true

	// 首访绑定
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("first visit: %v", err)
	}
	// 同 IP 再访问
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("same ip: %v", err)
	}
	// 异 IP 拒绝
	if _, err := s.Validate(tok.Token, "2.2.2.2"); err != ErrTokenIPBound {
		t.Errorf("want ErrTokenIPBound, got %v", err)
	}
	// reset_bind 后可重绑
	s.Update(tok.ID, &UpdateTokenReq{ResetBind: true})
	if _, err := s.Validate(tok.Token, "2.2.2.2"); err != nil {
		t.Fatalf("after reset: %v", err)
	}
}

func TestValidate_IPBindDisabled(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	off := false
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x", IPBindEnabled: &off})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.Validate(tok.Token, "9.9.9.9"); err != nil {
		t.Errorf("should allow any ip when bind disabled, got %v", err)
	}
}

func TestRotateInvalidatesOldToken(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"})
	old := tok.Token
	newTok, err := s.Rotate(tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newTok.Token == old {
		t.Error("token not changed")
	}
	if got, _ := s.GetByToken(old); got != nil {
		t.Error("old token should be gone")
	}
	if newTok.BoundIP != "" {
		t.Error("bound_ip should be cleared after rotate")
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/service -run 'TestGenerateTokenLength|TestValidate_|TestRotate' -v`
Expected: 全部 PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/service/subscription_token.go internal/service/subscription_token_test.go
git commit -m "feat(service): token Validate 与 TouchAsync，含并发 IP 绑定"
```

---

### Task 6: 重写订阅 Handler

**Files:**
- Modify: `internal/handler/subscription.go`

路由绑定放 Task 7，这里只改 handler 逻辑。原 `GetByUUID` 路径改为通过 `SubscriptionTokenService.Validate` 查 user_id，再获取 user。

- [ ] **Step 1: 改 Handler 结构体与构造器**

```go
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"proxy-panel/internal/database"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/subscription"

	"github.com/gin-gonic/gin"
)

type SubscriptionHandler struct {
	userSvc  *service.UserService
	nodeSvc  *service.NodeService
	tokenSvc *service.SubscriptionTokenService
	db       *database.DB
}

func NewSubscriptionHandler(userSvc *service.UserService, nodeSvc *service.NodeService,
	tokenSvc *service.SubscriptionTokenService, db *database.DB) *SubscriptionHandler {
	return &SubscriptionHandler{userSvc: userSvc, nodeSvc: nodeSvc, tokenSvc: tokenSvc, db: db}
}
```

- [ ] **Step 2: 提取共享的 `serve` 方法**

```go
// serve 接收已通过 token 校验的 userID，完成订阅内容生成。
func (h *SubscriptionHandler) serve(c *gin.Context, userID int64, deprecated bool) {
	user, err := h.userSvc.GetByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if !user.Enable {
		c.JSON(http.StatusForbidden, gin.H{"error": "账户已禁用"})
		return
	}
	if user.TrafficLimit > 0 && user.TrafficUsed >= user.TrafficLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "流量已耗尽"})
		return
	}

	nodes, err := h.nodeSvc.ListByUserID(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点失败"})
		return
	}
	if len(nodes) == 0 {
		nodes, err = h.nodeSvc.ListEnabled()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点失败"})
			return
		}
	}

	// 格式：URL 优先，UA 兜底，最后 v2ray
	format := c.Query("format")
	if format == "" {
		format = subscription.SniffFormat(c.GetHeader("User-Agent"))
	}
	if format == "" {
		format = "v2ray"
	}

	baseURL := fmt.Sprintf("%s://%s", scheme(c), c.Request.Host)

	var customRulesStr, customRulesMode string
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rules'").Scan(&customRulesStr)
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rules_mode'").Scan(&customRulesMode)
	if customRulesStr != "" {
		subscription.SetCustomRules(strings.Split(customRulesStr, "\n"))
	} else {
		subscription.SetCustomRules(nil)
	}
	subscription.SetCustomRulesMode(customRulesMode)

	gen := subscription.GetGenerator(format)
	content, contentType, err := gen.Generate(nodes, user, baseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败"})
		return
	}

	userinfo := fmt.Sprintf("upload=%d; download=%d; total=%d",
		user.TrafficUp, user.TrafficDown, user.TrafficLimit)
	if user.ExpiresAt != nil {
		userinfo += fmt.Sprintf("; expire=%d", user.ExpiresAt.Unix())
	}
	c.Header("Subscription-Userinfo", userinfo)
	if deprecated {
		c.Header("X-Subscription-Deprecated", "please migrate to /api/sub/t/<token>")
	}
	if c.Query("dl") == "1" {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", user.Username))
	}
	c.Data(http.StatusOK, contentType, []byte(content))
}
```

- [ ] **Step 3: 新 `SubscribeByToken` + 改写 `Subscribe`（旧端点）**

```go
func (h *SubscriptionHandler) SubscribeByToken(c *gin.Context) {
	h.doSub(c, c.Param("token"), false)
}

// Subscribe 旧端点（保留向后兼容），把 uuid 当作 token 查表（迁移已写入）。
func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	h.doSub(c, c.Param("uuid"), true)
}

func (h *SubscriptionHandler) doSub(c *gin.Context, tokenStr string, deprecated bool) {
	tok, err := h.tokenSvc.Validate(tokenStr, c.ClientIP())
	switch {
	case errors.Is(err, service.ErrTokenNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "订阅链接无效"})
		return
	case errors.Is(err, service.ErrTokenDisabled):
		c.JSON(http.StatusForbidden, gin.H{"error": "订阅链接已禁用"})
		return
	case errors.Is(err, service.ErrTokenExpired):
		c.JSON(http.StatusGone, gin.H{"error": "订阅链接已过期"})
		return
	case errors.Is(err, service.ErrTokenIPBound):
		c.JSON(http.StatusForbidden, gin.H{"error": "订阅链接已绑定其他 IP"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}

	h.tokenSvc.TouchAsync(tok.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	h.serve(c, tok.UserID, deprecated)
}
```

- [ ] **Step 4: 删除原 handler 里不再用的 `GetByUUID` 调用**

Task 6 的 Step 3 已完全替换 `Subscribe`。确认 `internal/handler/subscription.go` 里除了 `scheme()` 辅助函数外，没有剩余旧逻辑。

- [ ] **Step 5: 编译**

Run: `go build ./...`
Expected: 报 `NewSubscriptionHandler` 调用方（`internal/router/router.go:71`）参数不匹配 —— 下一个 Task 修。暂时允许失败，或先用 `//lint:ignore` 跳过：这里就让它红着，Task 7 一起修。

实际操作：先跳过 Step 5 的编译期望，直接进入 Task 7。

- [ ] **Step 6: Commit（含未编译通过的中间态，为便于回滚；若你偏好原子提交，合并到 Task 7 的 commit 中）**

```bash
git add internal/handler/subscription.go
git commit -m "refactor(subscription): handler 基于 token 校验，保留旧端点兼容" --no-verify || true
```

> 若仓库 pre-commit 要求编译通过，就跳过本步骤，把所有改动累积到 Task 7 一次性 commit。

---

### Task 7: 管理 Handler + 路由接线 + 限流 key 扩展

**Files:**
- Create: `internal/handler/subscription_token.go`
- Modify: `internal/router/router.go`
- Modify: `internal/router/middleware.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 限流器 key 支持 token 或 uuid**

修改 `internal/router/middleware.go:228`：

```go
		key := subLimitKey(c) + "|" + c.ClientIP()
```

并在同文件底部新增：

```go
func subLimitKey(c *gin.Context) string {
	if t := c.Param("token"); t != "" {
		return t
	}
	return c.Param("uuid")
}
```

- [ ] **Step 2: 管理 Handler**

创建 `internal/handler/subscription_token.go`：

```go
package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

type SubTokenHandler struct {
	svc *service.SubscriptionTokenService
}

func NewSubTokenHandler(svc *service.SubscriptionTokenService) *SubTokenHandler {
	return &SubTokenHandler{svc: svc}
}

func (h *SubTokenHandler) List(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	list, err := h.svc.ListByUser(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *SubTokenHandler) Create(c *gin.Context) {
	uid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	var req service.CreateTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.svc.Create(uid, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *SubTokenHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	var req service.UpdateTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token 不存在"})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *SubTokenHandler) Rotate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	t, err := h.svc.Rotate(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *SubTokenHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 3: cmd/server/main.go 注入**

在 `cmd/server/main.go` 中找到其它 service 初始化段（例如 `authSvc := service.NewAuthService(...)`），补一行：

```go
	subTokenSvc := service.NewSubscriptionTokenService(db)
```

并把 `subTokenSvc` 加入 `router.Setup(...)` 的调用参数列表（位置跟随参数签名扩展决定，见下一步）。

- [ ] **Step 4: 改 `router.Setup` 签名并接线**

修改 `internal/router/router.go:21-25`：

```go
func Setup(cfg *config.Config, db *database.DB, mgr *kernel.Manager,
	userSvc *service.UserService, nodeSvc *service.NodeService,
	trafficSvc *service.TrafficService, notifySvc *notify.NotifyService,
	authSvc *service.AuthService, scheduler *service.Scheduler,
	fwSvc *firewall.Service, auditSvc *service.AuditService,
	subTokenSvc *service.SubscriptionTokenService, dbPath string) *gin.Engine {
```

改 `internal/router/router.go:71`：

```go
	subHandler := handler.NewSubscriptionHandler(userSvc, nodeSvc, subTokenSvc, db)
	subTokenHandler := handler.NewSubTokenHandler(subTokenSvc)
```

改 `internal/router/router.go:85`：

```go
		api.GET("/sub/:uuid", subLimiter.Limit(), subHandler.Subscribe)
		api.GET("/sub/t/:token", subLimiter.Limit(), subHandler.SubscribeByToken)
```

在 `auth` group 内（`internal/router/router.go:100` 附近用户管理段之后）追加：

```go
			auth.GET("/users/:id/sub-tokens", subTokenHandler.List)
			auth.POST("/users/:id/sub-tokens", subTokenHandler.Create)
			auth.PATCH("/sub-tokens/:id", subTokenHandler.Update)
			auth.POST("/sub-tokens/:id/rotate", subTokenHandler.Rotate)
			auth.DELETE("/sub-tokens/:id", subTokenHandler.Delete)
```

注意：`/users/:id/sub-tokens` 与 `/users/:id/reset-uuid` 共存，gin 路由能处理。

- [ ] **Step 5: 同步 `main.go` 调用**

定位 `cmd/server/main.go` 里调用 `router.Setup(...)` 的那一行，补上 `subTokenSvc` 实参（按新签名顺序在 `auditSvc` 之后、`dbPath` 之前）。

- [ ] **Step 6: 编译 + 跑测试**

Run:
```
go build ./...
go test ./internal/service ./internal/service/subscription
```
Expected: 编译通过；测试全部 PASS。

- [ ] **Step 7: Commit**

```bash
git add internal/handler/subscription.go internal/handler/subscription_token.go \
        internal/router/router.go internal/router/middleware.go cmd/server/main.go
git commit -m "feat(api): 订阅 token CRUD 路由 + 限流 key 适配"
```

---

### Task 8: 手工冒烟 — token 流程端到端

**Files:**
- 无文件修改

- [ ] **Step 1: 启动服务**

Run: `go run ./cmd/server -config config.yaml`
Expected: 服务监听 8080。

- [ ] **Step 2: 登录拿 JWT**

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)
echo $TOKEN
```

Expected: 非空 JWT。

- [ ] **Step 3: 给用户 id=1 创建 token**

```bash
curl -s -X POST http://localhost:8080/api/users/1/sub-tokens \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"cli-test"}' | tee /tmp/tok.json
SUB=$(jq -r .token /tmp/tok.json)
```

Expected: 返回 JSON 含 43 字节 `token`、`ip_bind_enabled: true`。

- [ ] **Step 4: 拉订阅**

```bash
curl -i "http://localhost:8080/api/sub/t/$SUB?format=clash"
```

Expected: 200，Content-Type 是 clash 相关；无 `X-Subscription-Deprecated` 头。

- [ ] **Step 5: 验证 IP 绑定 — 伪造不同 X-Forwarded-For**

（需要先在 config.yaml `server.trusted_proxies` 里加 `127.0.0.1`，否则头会被 gin 忽略）

```bash
curl -i -H 'X-Forwarded-For: 9.9.9.9' "http://localhost:8080/api/sub/t/$SUB"
```

Expected: 403，message 含"已绑定其他 IP"。

- [ ] **Step 6: Reset bind**

```bash
ID=$(jq -r .id /tmp/tok.json)
curl -s -X PATCH "http://localhost:8080/api/sub-tokens/$ID" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"reset_bind":true}'
curl -i -H 'X-Forwarded-For: 9.9.9.9' "http://localhost:8080/api/sub/t/$SUB"
```

Expected: 第二个 curl 返回 200，token 绑定到 9.9.9.9。

- [ ] **Step 7: 旧端点仍可用**

```bash
UUID=$(sqlite3 data/panel.db 'SELECT uuid FROM users WHERE id=1')
curl -i "http://localhost:8080/api/sub/$UUID"
```

Expected: 200 + 响应头 `X-Subscription-Deprecated: please migrate to /api/sub/t/<token>`。

- [ ] **Step 8: UA 兜底**

```bash
curl -i -A 'ClashX Pro/1.95' "http://localhost:8080/api/sub/t/$SUB"
```

Expected: Content-Type/body 为 clash 格式；不带 `?format=` 时按 UA 返回。

- [ ] **Step 9: 轮换失效**

```bash
curl -s -X POST "http://localhost:8080/api/sub-tokens/$ID/rotate" \
  -H "Authorization: Bearer $TOKEN"
curl -i "http://localhost:8080/api/sub/t/$SUB"
```

Expected: 第二个 curl 返回 404。

- [ ] **Step 10: Commit（仅作为验收记录）**

无需 commit，把脚本片段保留在本地。

---

### Task 9: 前端 API 客户端

**Files:**
- Create: `web/src/api/sub-tokens.js`

- [ ] **Step 1: 写 API 文件**

参考 `web/src/api/` 内现有模块风格（假设使用 axios 实例 `request`）：

```js
import request from '@/utils/request'

export const listSubTokens = (userId) =>
  request.get(`/users/${userId}/sub-tokens`)

export const createSubToken = (userId, payload) =>
  request.post(`/users/${userId}/sub-tokens`, payload)

export const updateSubToken = (id, payload) =>
  request.patch(`/sub-tokens/${id}`, payload)

export const rotateSubToken = (id) =>
  request.post(`/sub-tokens/${id}/rotate`)

export const deleteSubToken = (id) =>
  request.delete(`/sub-tokens/${id}`)
```

若项目 request 封装路径不同（查 `web/src/api/users.js` 等首次使用的文件），照样复制其 import 与调用约定即可。

- [ ] **Step 2: Commit**

```bash
git add web/src/api/sub-tokens.js
git commit -m "feat(web): 订阅 token 管理 API"
```

---

### Task 10: 前端 Token 管理抽屉

**Files:**
- Create: `web/src/components/SubTokenDrawer.vue`
- Modify: `web/src/views/Users.vue`

- [ ] **Step 1: 写 Drawer 组件**

```vue
<template>
  <el-drawer v-model="open" :title="`订阅 Token — ${userName}`" size="700px" @open="reload">
    <div class="mb-3 flex justify-end">
      <el-button type="primary" @click="showCreate = true">新建 Token</el-button>
    </div>

    <el-table :data="tokens" stripe>
      <el-table-column prop="name" label="名称" width="140" />
      <el-table-column label="订阅链接" min-width="280">
        <template #default="{ row }">
          <div class="flex items-center gap-1">
            <el-input size="small" :model-value="subURL(row)" readonly />
            <el-button size="small" @click="copy(subURL(row))">复制</el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="过期" width="160">
        <template #default="{ row }">
          {{ row.expires_at ? new Date(row.expires_at).toLocaleString() : '永不' }}
        </template>
      </el-table-column>
      <el-table-column label="IP 绑定" width="170">
        <template #default="{ row }">
          <el-switch :model-value="row.ip_bind_enabled"
            @change="(v) => patch(row, { ip_bind_enabled: v })" />
          <span v-if="row.ip_bind_enabled" class="ml-2 text-xs">
            {{ row.bound_ip || '未绑' }}
            <el-button v-if="row.bound_ip" size="small" link
              @click="patch(row, { reset_bind: true })">清除</el-button>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="80">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled"
            @change="(v) => patch(row, { enabled: v })" />
        </template>
      </el-table-column>
      <el-table-column label="最后使用" width="220">
        <template #default="{ row }">
          <div class="text-xs">
            <div>{{ row.last_used_at ? new Date(row.last_used_at).toLocaleString() : '从未' }}</div>
            <div class="text-gray-500">{{ row.last_ip }} · {{ (row.last_ua || '').slice(0, 30) }}</div>
            <div>次数：{{ row.use_count }}</div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150">
        <template #default="{ row }">
          <el-button size="small" @click="rotate(row)">轮换</el-button>
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showCreate" title="新建订阅 Token" width="420">
      <el-form :model="newToken" label-width="90">
        <el-form-item label="名称"><el-input v-model="newToken.name" /></el-form-item>
        <el-form-item label="过期时间">
          <el-date-picker v-model="newToken.expires_at" type="datetime" placeholder="留空=永不过期" />
        </el-form-item>
        <el-form-item label="IP 绑定"><el-switch v-model="newToken.ip_bind_enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="submitCreate">确定</el-button>
      </template>
    </el-dialog>
  </el-drawer>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listSubTokens, createSubToken, updateSubToken, rotateSubToken, deleteSubToken } from '@/api/sub-tokens'

const props = defineProps({
  modelValue: Boolean,
  userId: { type: Number, required: false, default: 0 },
  userName: { type: String, default: '' },
})
const emit = defineEmits(['update:modelValue'])
const open = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const tokens = ref([])
const showCreate = ref(false)
const newToken = ref({ name: '', expires_at: null, ip_bind_enabled: true })

async function reload() {
  if (!props.userId) return
  tokens.value = (await listSubTokens(props.userId)).data || []
}

function subURL(row) {
  return `${location.origin}/api/sub/t/${row.token}`
}

async function copy(text) {
  await navigator.clipboard.writeText(text)
  ElMessage.success('已复制')
}

async function patch(row, payload) {
  await updateSubToken(row.id, payload)
  await reload()
}

async function rotate(row) {
  await ElMessageBox.confirm('轮换后原链接立即失效，确定吗？', '确认', { type: 'warning' })
  await rotateSubToken(row.id)
  ElMessage.success('已轮换')
  await reload()
}

async function remove(row) {
  await ElMessageBox.confirm(`删除 Token "${row.name}"?`, '确认', { type: 'warning' })
  await deleteSubToken(row.id)
  await reload()
}

async function submitCreate() {
  if (!newToken.value.name) {
    ElMessage.warning('请填写名称')
    return
  }
  await createSubToken(props.userId, {
    name: newToken.value.name,
    expires_at: newToken.value.expires_at || null,
    ip_bind_enabled: newToken.value.ip_bind_enabled,
  })
  showCreate.value = false
  newToken.value = { name: '', expires_at: null, ip_bind_enabled: true }
  await reload()
}
</script>
```

- [ ] **Step 2: 在用户列表页挂入口**

打开 `web/src/views/Users.vue`，在操作列与现有按钮同组加一个按钮（假设现有操作列形如 `<el-table-column label="操作">` 内含 `<el-button @click="edit(row)">编辑</el-button>`）：

```vue
            <el-button size="small" @click="openTokens(row)">订阅 Token</el-button>
```

在 `<script setup>` 段追加：

```js
import SubTokenDrawer from '@/components/SubTokenDrawer.vue'
const tokenDrawer = ref(false)
const tokenUser = ref({ id: 0, username: '' })
function openTokens(row) {
  tokenUser.value = { id: row.id, username: row.username }
  tokenDrawer.value = true
}
```

并在模板末尾挂载：

```vue
    <SubTokenDrawer v-model="tokenDrawer" :user-id="tokenUser.id" :user-name="tokenUser.username" />
```

- [ ] **Step 3: 前端编译**

Run: `cd web && npm run build`
Expected: 无报错。

- [ ] **Step 4: 手工验收**

启动服务 → 登录 → 进入用户列表 → 点"订阅 Token" → 弹 Drawer → 新建 → 复制链接 → 浏览器 / curl 打开成功。

- [ ] **Step 5: Commit**

```bash
git add web/src/components/SubTokenDrawer.vue web/src/views/Users.vue
git commit -m "feat(web): 用户订阅 Token 管理抽屉"
```

---

### Task 11: 文档与 ROADMAP 勾掉

**Files:**
- Modify: `ROADMAP.md`
- Modify: `README.md`

- [ ] **Step 1: ROADMAP 标记完成**

在 `ROADMAP.md` 的 P0 #3 段落顶部加一行 `> ✅ 已完成 2026-04-23 (commit <hash>)`，并从 M1 里划掉。

- [ ] **Step 2: README 更新订阅章节**

找到描述订阅链接的那一段，补充：
- 订阅链接推荐路径 `GET /api/sub/t/<token>`；
- 管理后台可创建多个 token 并设置过期 / IP 绑定；
- 未带 `?format=` 时按客户端 UA 自动识别格式。

- [ ] **Step 3: Commit**

```bash
git add ROADMAP.md README.md
git commit -m "docs: ROADMAP P0#3 完成；订阅链接说明更新"
```

---

## Self-Review Notes（已内联修复）

- Spec §9 要求 IP 限流 60/min，但现有 `SubRateLimiter` 已是 30/min —— 沿用 30/min，不再新增限流组件；Spec 与 Plan 均以现状为准。
- Spec §10.2 要求"IP 限流测试"无法廉价自动化 —— 本计划未单独列任务，留给后续扩展。
- Spec §11 风险表中"trusted_proxies 未配置"—— 实现无须额外代码，文档已在 README 单独强调即可；启动时 warning 属于 YAGNI。

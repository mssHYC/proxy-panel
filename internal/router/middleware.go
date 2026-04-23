package router

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuditMiddleware 在写操作（POST/PUT/DELETE）完成后异步记一条审计日志。
// 排除订阅接口与登录接口（登录失败也不应记为 actor=匿名操作日志）。
func AuditMiddleware(audit *service.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if audit == nil {
			return
		}
		m := c.Request.Method
		if m != http.MethodPost && m != http.MethodPut && m != http.MethodDelete {
			return
		}
		if c.Writer.Status() >= 400 {
			return
		}
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/sub/") || strings.HasPrefix(path, "/api/auth/login") || strings.HasPrefix(path, "/api/auth/2fa/verify") {
			return
		}
		actor, _ := c.Get("username")
		actorStr := ""
		if s, ok := actor.(string); ok {
			actorStr = s
		}
		action := m + " " + path
		_ = audit.Log(actorStr, action, "", c.Param("id"), c.ClientIP(), "")
	}
}

// MetricsMiddleware 统计 HTTP 请求数到 Prometheus。为避免标签高基数爆炸，
// /api/sub/:uuid 这类含动态参数的路径统一合并为 "/api/sub/:uuid"。
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := fmt.Sprintf("%d", c.Writer.Status())
		service.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
	}
}

// DomainGuard - 域名访问限制中间件，拒绝通过 IP 直接访问
func DomainGuard(domain string) gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !strings.EqualFold(host, domain) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

// JWTAuth - JWT 认证中间件
// currentVersion 由 AuthService.GetTokenVersion 提供；token 的 ver claim 与之不一致即判定已吊销
// 改密/改用户名/开关 2FA 会使版本递增，历史 token 立即失效
func JWTAuth(secret string, currentVersion func() int) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权", "code": "ERR_UNAUTHORIZED"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
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
		// 强制要求 access token 类型，防止 2fa_pending 等其他用途的 token 被误用
		if t, _ := claims["type"].(string); t != "access" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌无效", "code": "ERR_INVALID_TOKEN"})
			c.Abort()
			return
		}
		// 版本校验：改密/改用户名/开关 2FA 会递增 token_version，使历史 token 立即失效
		if currentVersion != nil {
			ver, _ := claims["ver"].(float64) // JWT 数字默认被解析为 float64
			if int(ver) != currentVersion() {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌已失效，请重新登录", "code": "ERR_TOKEN_REVOKED"})
				c.Abort()
				return
			}
		}
		c.Set("username", claims["username"])
		c.Next()
	}
}

// limiterCleanupInterval - 限流器空 key 后台清理周期
const limiterCleanupInterval = 5 * time.Minute

// RateLimiter - 登录限流 (5次/分钟/IP)
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{attempts: make(map[string][]time.Time)}
	go rl.janitor()
	return rl
}

// janitor 周期性清理 attempts 中的过期/空 entry，避免长期运行时 map 无界增长
func (rl *RateLimiter) janitor() {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, v := range rl.attempts {
			var valid []time.Time
			for _, t := range v {
				if now.Sub(t) < time.Minute {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.attempts, k)
			} else {
				rl.attempts[k] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		rl.mu.Lock()
		now := time.Now()
		// 清理过期记录
		existing := rl.attempts[ip]
		var valid []time.Time
		for _, t := range existing {
			if now.Sub(t) < time.Minute {
				valid = append(valid, t)
			}
		}
		if len(valid) >= 5 {
			rl.attempts[ip] = valid
			rl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "登录请求过于频繁", "code": "ERR_RATE_LIMIT"})
			c.Abort()
			return
		}
		rl.attempts[ip] = append(valid, now)
		rl.mu.Unlock()
		c.Next()
	}
}

// SubRateLimiter - 订阅限流 (30次/分钟)，限流 key = uuid + IP
// 仅用 uuid 作为 key 时，攻击者拿到泄漏的订阅链接可精准 DoS 指定用户；
// 叠加 IP 维度后，攻击者即使刷爆自己的 IP 配额也无法阻断合法客户端
type SubRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewSubRateLimiter() *SubRateLimiter {
	srl := &SubRateLimiter{attempts: make(map[string][]time.Time)}
	go srl.janitor()
	return srl
}

func (srl *SubRateLimiter) janitor() {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		srl.mu.Lock()
		now := time.Now()
		for k, v := range srl.attempts {
			var valid []time.Time
			for _, t := range v {
				if now.Sub(t) < time.Minute {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(srl.attempts, k)
			} else {
				srl.attempts[k] = valid
			}
		}
		srl.mu.Unlock()
	}
}

func (srl *SubRateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := subLimitKey(c) + "|" + c.ClientIP()
		srl.mu.Lock()
		now := time.Now()
		// 清理过期记录
		existing := srl.attempts[key]
		var valid []time.Time
		for _, t := range existing {
			if now.Sub(t) < time.Minute {
				valid = append(valid, t)
			}
		}
		if len(valid) >= 30 {
			srl.attempts[key] = valid
			srl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁", "code": "ERR_RATE_LIMIT"})
			c.Abort()
			return
		}
		srl.attempts[key] = append(valid, now)
		srl.mu.Unlock()
		c.Next()
	}
}

func subLimitKey(c *gin.Context) string {
	if t := c.Param("token"); t != "" {
		return t
	}
	return c.Param("uuid")
}

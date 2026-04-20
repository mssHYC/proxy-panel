package router

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

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
		// 强制要求 access token 类型,防止 2fa_pending 等其他用途的 token 被误用
		if t, _ := claims["type"].(string); t != "access" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌无效", "code": "ERR_INVALID_TOKEN"})
			c.Abort()
			return
		}
		c.Set("username", claims["username"])
		c.Next()
	}
}

// RateLimiter - 登录限流 (5次/分钟/IP)
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

// SubRateLimiter - 订阅限流 (30次/分钟/UUID)
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
		// 清理过期记录
		existing := srl.attempts[uuid]
		var valid []time.Time
		for _, t := range existing {
			if now.Sub(t) < time.Minute {
				valid = append(valid, t)
			}
		}
		if len(valid) >= 30 {
			srl.attempts[uuid] = valid
			srl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁", "code": "ERR_RATE_LIMIT"})
			c.Abort()
			return
		}
		srl.attempts[uuid] = append(valid, now)
		srl.mu.Unlock()
		c.Next()
	}
}

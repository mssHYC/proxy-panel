package handler

import (
	"errors"
	"fmt"
	"net/http"

	"proxy-panel/internal/database"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/routing"
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

// SubscribeByToken 新订阅端点 GET /api/sub/t/:token
func (h *SubscriptionHandler) SubscribeByToken(c *gin.Context) {
	h.doSub(c, c.Param("token"), false)
}

// Subscribe 旧端点 GET /api/sub/:uuid，保留向后兼容；迁移已把 uuid 作为 token 写入表。
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

	format := c.Query("format")
	if format == "" {
		format = subscription.SniffFormat(c.GetHeader("User-Agent"))
	}
	if format == "" {
		format = "v2ray"
	}

	baseURL := fmt.Sprintf("%s://%s", scheme(c), c.Request.Host)

	gen := subscription.GetGenerator(format)
	var content, contentType string
	if ra, ok := gen.(subscription.RoutingAwareGenerator); ok {
		plan, err := routing.BuildPlan(c.Request.Context(), h.db, routing.BuildOptions{
			PresetOverride: c.Query("preset"),
			ClientFormat:   format,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "构建分流规划失败: " + err.Error()})
			return
		}
		content, contentType, err = ra.GenerateWithPlan(plan, nodes, user, baseURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败"})
			return
		}
	} else {
		var err error
		content, contentType, err = gen.Generate(nodes, user, baseURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败"})
			return
		}
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

func scheme(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

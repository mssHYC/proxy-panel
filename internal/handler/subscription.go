package handler

import (
	"fmt"
	"net/http"
	"strings"

	"proxy-panel/internal/database"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/subscription"

	"github.com/gin-gonic/gin"
)

// SubscriptionHandler 订阅处理器
type SubscriptionHandler struct {
	userSvc *service.UserService
	nodeSvc *service.NodeService
	db      *database.DB
}

// NewSubscriptionHandler 创建订阅处理器
func NewSubscriptionHandler(userSvc *service.UserService, nodeSvc *service.NodeService, db *database.DB) *SubscriptionHandler {
	return &SubscriptionHandler{userSvc: userSvc, nodeSvc: nodeSvc, db: db}
}

// Subscribe 处理订阅请求
func (h *SubscriptionHandler) Subscribe(c *gin.Context) {
	uuid := c.Param("uuid")
	format := c.DefaultQuery("format", "v2ray")

	// 查询用户
	user, err := h.userSvc.GetByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 检查用户是否启用
	if !user.Enable {
		c.JSON(http.StatusForbidden, gin.H{"error": "账户已禁用"})
		return
	}

	// 检查流量是否耗尽
	if user.TrafficLimit > 0 && user.TrafficUsed >= user.TrafficLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "流量已耗尽"})
		return
	}

	// 获取用户关联的节点，无关联则返回全部启用节点
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

	// 构建 baseURL
	baseURL := fmt.Sprintf("%s://%s", scheme(c), c.Request.Host)

	// 加载自定义规则和模式
	var customRulesStr, customRulesMode string
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rules'").Scan(&customRulesStr)
	h.db.QueryRow("SELECT value FROM settings WHERE key = 'custom_rules_mode'").Scan(&customRulesMode)
	if customRulesStr != "" {
		subscription.SetCustomRules(strings.Split(customRulesStr, "\n"))
	} else {
		subscription.SetCustomRules(nil)
	}
	subscription.SetCustomRulesMode(customRulesMode)

	// 生成订阅内容
	gen := subscription.GetGenerator(format)
	content, contentType, err := gen.Generate(nodes, user, baseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成订阅失败"})
		return
	}

	// 设置 Subscription-Userinfo 头
	userinfo := fmt.Sprintf("upload=%d; download=%d; total=%d",
		user.TrafficUp, user.TrafficDown, user.TrafficLimit)
	if user.ExpiresAt != nil {
		userinfo += fmt.Sprintf("; expire=%d", user.ExpiresAt.Unix())
	}
	c.Header("Subscription-Userinfo", userinfo)

	// 浏览器直接查看，客户端通过 Content-Type 识别格式
	// 仅在客户端主动请求下载时附加 filename
	if c.Query("dl") == "1" {
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", user.Username))
	}

	// 返回订阅内容
	c.Data(http.StatusOK, contentType, []byte(content))
}

// scheme 获取请求的协议（http/https）
func scheme(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

package handler

import (
	"net/http"

	"proxy-panel/internal/service/notify"

	"github.com/gin-gonic/gin"
)

// NotifyHandler 通知相关的 HTTP 处理器
type NotifyHandler struct {
	svc *notify.NotifyService
}

// NewNotifyHandler 创建通知处理器实例
func NewNotifyHandler(svc *notify.NotifyService) *NotifyHandler {
	return &NotifyHandler{svc: svc}
}

// Test 发送测试通知消息
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

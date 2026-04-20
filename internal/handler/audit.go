package handler

import (
	"net/http"
	"strconv"
	"time"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// AuditHandler 审计日志查询
type AuditHandler struct {
	svc *service.AuditService
}

// NewAuditHandler 构造审计 handler
func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// List GET /api/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	if page < 1 {
		page = 1
	}
	f := service.AuditFilter{
		Actor:  c.Query("actor"),
		Action: c.Query("action"),
		Limit:  size,
		Offset: (page - 1) * size,
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			f.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			f.To = &t
		}
	}

	items, total, err := h.svc.List(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

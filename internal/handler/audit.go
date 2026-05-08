package handler

import (
	"encoding/csv"
	"fmt"
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

// parseFilter 从 query 中解析通用过滤参数（List/Export 共用）
func parseFilter(c *gin.Context) service.AuditFilter {
	f := service.AuditFilter{
		Actor:      c.Query("actor"),
		Action:     c.Query("action"),
		TargetType: c.Query("target_type"),
		TargetID:   c.Query("target_id"),
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
	return f
}

// List GET /api/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	if page < 1 {
		page = 1
	}
	f := parseFilter(c)
	f.Limit = size
	f.Offset = (page - 1) * size

	items, total, err := h.svc.List(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Export GET /api/audit-logs/export 导出 CSV，复用 List 的过滤参数
func (h *AuditHandler) Export(c *gin.Context) {
	items, err := h.svc.Export(parseFilter(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("audit-logs-%s.csv", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM 让 Excel 正确识别中文
	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "created_at", "actor", "action", "target_type", "target_id", "ip", "detail"})
	for _, a := range items {
		_ = w.Write([]string{
			strconv.FormatInt(a.ID, 10),
			a.CreatedAt.Format(time.RFC3339),
			a.Actor,
			a.Action,
			a.TargetType,
			a.TargetID,
			a.IP,
			a.Detail,
		})
	}
	w.Flush()
}

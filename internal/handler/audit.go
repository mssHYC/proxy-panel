package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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

// parseFilter 从 query 中解析通用过滤参数。
// 非空但解析失败的 from/to 会返回 error，调用方应回 400 而不是静默忽略，
// 否则用户以为按时间范围查询/导出，实际可能取到更大范围。
func parseFilter(c *gin.Context) (service.AuditFilter, error) {
	f := service.AuditFilter{
		Actor:      c.Query("actor"),
		Action:     c.Query("action"),
		TargetType: c.Query("target_type"),
		TargetID:   c.Query("target_id"),
	}
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return f, fmt.Errorf("from 时间格式无效，需 RFC3339")
		}
		f.From = &t
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return f, fmt.Errorf("to 时间格式无效，需 RFC3339")
		}
		f.To = &t
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return f, fmt.Errorf("from 必须早于 to")
	}
	return f, nil
}

// List GET /api/audit-logs
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "50"))
	if page < 1 {
		page = 1
	}
	f, err := parseFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	f.Limit = size
	f.Offset = (page - 1) * size

	items, total, err := h.svc.List(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// neutralizeCSVCell 防止 Excel/LibreOffice CSV Formula Injection。
// 字段以 = + - @ \t \r 开头时前置单引号，让电子表格视为文本而非公式。
func neutralizeCSVCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// Export GET /api/audit-logs/export 导出 CSV，复用 List 的过滤参数
func (h *AuditHandler) Export(c *gin.Context) {
	f, err := parseFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, truncated, err := h.svc.Export(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("audit-logs-%s.csv", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("X-Export-Limit", strconv.Itoa(service.ExportMaxRows))
	if truncated {
		c.Header("X-Export-Truncated", "1")
	}
	// UTF-8 BOM 让 Excel 正确识别中文
	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}
	w := csv.NewWriter(c.Writer)
	if err := w.Write([]string{"id", "created_at", "actor", "action", "target_type", "target_id", "ip", "detail"}); err != nil {
		return
	}
	for _, a := range items {
		row := []string{
			strconv.FormatInt(a.ID, 10),
			a.CreatedAt.Format(time.RFC3339),
			neutralizeCSVCell(a.Actor),
			neutralizeCSVCell(a.Action),
			neutralizeCSVCell(a.TargetType),
			neutralizeCSVCell(a.TargetID),
			neutralizeCSVCell(a.IP),
			neutralizeCSVCell(strings.ReplaceAll(a.Detail, "\x00", "")),
		}
		if err := w.Write(row); err != nil {
			return
		}
	}
	w.Flush()
	_ = w.Error()
}

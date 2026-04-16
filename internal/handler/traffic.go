package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// TrafficHandler 流量管理处理器
type TrafficHandler struct {
	trafficSvc *service.TrafficService
}

// NewTrafficHandler 创建流量处理器
func NewTrafficHandler(trafficSvc *service.TrafficService) *TrafficHandler {
	return &TrafficHandler{trafficSvc: trafficSvc}
}

// GetServerTraffic 获取服务器全局流量
func (h *TrafficHandler) GetServerTraffic(c *gin.Context) {
	st, err := h.trafficSvc.GetServerTraffic()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, st)
}

// SetServerLimit 设置服务器流量限制
func (h *TrafficHandler) SetServerLimit(c *gin.Context) {
	var req struct {
		LimitGB int64 `json:"limit_gb"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	if err := h.trafficSvc.SetServerLimit(req.LimitGB); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "服务器流量限制已更新"})
}

// GetHistory 获取流量历史
func (h *TrafficHandler) GetHistory(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 30
	}

	history, err := h.trafficSvc.GetHistory(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": history})
}

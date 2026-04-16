package handler

import (
	"net/http"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// DashboardHandler 仪表盘处理器
type DashboardHandler struct {
	userSvc    *service.UserService
	nodeSvc    *service.NodeService
	trafficSvc *service.TrafficService
	mgr        *kernel.Manager
	db         *database.DB
}

// NewDashboardHandler 创建仪表盘处理器
func NewDashboardHandler(
	userSvc *service.UserService,
	nodeSvc *service.NodeService,
	trafficSvc *service.TrafficService,
	mgr *kernel.Manager,
	db *database.DB,
) *DashboardHandler {
	return &DashboardHandler{
		userSvc:    userSvc,
		nodeSvc:    nodeSvc,
		trafficSvc: trafficSvc,
		mgr:        mgr,
		db:         db,
	}
}

// Get 获取仪表盘概览数据
func (h *DashboardHandler) Get(c *gin.Context) {
	// 用户统计
	totalUsers, enabledUsers, err := h.userSvc.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户统计失败: " + err.Error()})
		return
	}

	// 节点统计
	totalNodes, enabledNodes, err := h.nodeSvc.Count()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点统计失败: " + err.Error()})
		return
	}

	// 服务器流量
	serverTraffic, err := h.trafficSvc.GetServerTraffic()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器流量失败: " + err.Error()})
		return
	}

	// 今日流量
	today := time.Now().Format("2006-01-02")
	var todayUp, todayDown int64
	err = h.db.QueryRow(`SELECT COALESCE(SUM(upload), 0), COALESCE(SUM(download), 0)
		FROM traffic_logs WHERE DATE(timestamp) = ?`, today).Scan(&todayUp, &todayDown)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取今日流量失败: " + err.Error()})
		return
	}

	// 内核状态
	kernelStatus := h.mgr.Status()

	c.JSON(http.StatusOK, gin.H{
		"users": gin.H{
			"total":   totalUsers,
			"enabled": enabledUsers,
		},
		"nodes": gin.H{
			"total":   totalNodes,
			"enabled": enabledNodes,
		},
		"server_traffic": serverTraffic,
		"today_traffic": gin.H{
			"upload":   todayUp,
			"download": todayDown,
			"total":    todayUp + todayDown,
		},
		"kernel_status": kernelStatus,
	})
}

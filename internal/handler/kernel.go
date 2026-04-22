package handler

import (
	"net/http"

	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// KernelHandler 内核管理处理器
type KernelHandler struct {
	mgr     *kernel.Manager
	syncSvc *service.KernelSyncService
}

// NewKernelHandler 创建内核处理器
func NewKernelHandler(mgr *kernel.Manager, syncSvc *service.KernelSyncService) *KernelHandler {
	return &KernelHandler{mgr: mgr, syncSvc: syncSvc}
}

// Sync 手动触发内核配置同步：跳过防抖窗口立即生效。
// 面板上"应用变更"按钮调用，用于批量修改后的即时下发。
func (h *KernelHandler) Sync(c *gin.Context) {
	if err := h.syncSvc.SyncNow(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "内核配置已同步"})
}

// Status 返回所有内核的运行状态
func (h *KernelHandler) Status(c *gin.Context) {
	status := h.mgr.Status()
	c.JSON(http.StatusOK, gin.H{"kernels": status})
}

// restartRequest 重启请求
type restartRequest struct {
	Name string `json:"name" binding:"required"`
}

// Restart 重启指定内核
func (h *KernelHandler) Restart(c *gin.Context) {
	var req restartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供内核名称"})
		return
	}

	engine, err := h.mgr.Get(req.Name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if err := engine.Restart(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重启失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": req.Name + " 已重启"})
}

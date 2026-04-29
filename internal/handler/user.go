package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户管理处理器
type UserHandler struct {
	svc     *service.UserService
	syncSvc *service.KernelSyncService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(svc *service.UserService, syncSvc *service.KernelSyncService) *UserHandler {
	return &UserHandler{svc: svc, syncSvc: syncSvc}
}

// List 获取用户列表
func (h *UserHandler) List(c *gin.Context) {
	users, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// Get 获取单个用户
func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}

	user, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// Create 创建用户
func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	user, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 新增用户走热路径——所有关联节点都是 xray 时不会重启内核，
	// 其他用户连接保持不变（P1 验收）。失败会自动 fallback 全量同步。
	// 异步执行以保持 handler 响应延迟与原 Trigger() 一致。
	op := service.UserKernelOp{UUID: user.UUID, Username: user.Username, Protocol: user.Protocol, NodeIDs: user.NodeIDs}
	go func() { _ = h.syncSvc.HotAddUser(op) }()
	c.JSON(http.StatusCreated, user)
}

// Update 更新用户
func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}

	var req service.UpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	user, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	h.syncSvc.Trigger()
	c.JSON(http.StatusOK, user)
}

// Delete 删除用户
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}

	// 快照必须在 DELETE 之前抓取——否则拿不到 UUID/NodeIDs 用于 xray rmi。
	// snapshot 拿不到（用户不存在）也不影响 svc.Delete 自己的报错路径。
	snapshot, _ := h.svc.GetByID(id)

	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if snapshot != nil {
		op := service.UserKernelOp{UUID: snapshot.UUID, Username: snapshot.Username, Protocol: snapshot.Protocol, NodeIDs: snapshot.NodeIDs}
		go func() { _ = h.syncSvc.HotRemoveUser(op) }()
	} else {
		// 没拿到快照（理论上不会到此分支因为 Delete 已成功），仍触发一次全量兜底。
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// ResetTraffic 重置用户流量
func (h *UserHandler) ResetTraffic(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}

	if err := h.svc.ResetTraffic(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "流量已重置"})
}

// ResetUUID 重置用户 UUID
func (h *UserHandler) ResetUUID(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	newUUID, err := h.svc.ResetUUID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "code": "ERR_INTERNAL"})
		return
	}
	h.syncSvc.Trigger()
	c.JSON(http.StatusOK, gin.H{"uuid": newUUID, "message": "UUID 已重置"})
}

package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

type NodeGroupHandler struct {
	svc     *service.NodeGroupService
	syncSvc *service.KernelSyncService
}

func NewNodeGroupHandler(svc *service.NodeGroupService, syncSvc *service.KernelSyncService) *NodeGroupHandler {
	return &NodeGroupHandler{svc: svc, syncSvc: syncSvc}
}

func (h *NodeGroupHandler) List(c *gin.Context) {
	groups, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

func (h *NodeGroupHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}
	g, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if g == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分组不存在"})
		return
	}
	c.JSON(http.StatusOK, g)
}

func (h *NodeGroupHandler) Create(c *gin.Context) {
	var req service.NodeGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	g, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusCreated, g)
}

func (h *NodeGroupHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}
	var req service.NodeGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	g, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if g == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分组不存在"})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusOK, g)
}

func (h *NodeGroupHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

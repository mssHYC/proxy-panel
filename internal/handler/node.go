package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// NodeHandler 节点管理处理器
type NodeHandler struct {
	svc *service.NodeService
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler(svc *service.NodeService) *NodeHandler {
	return &NodeHandler{svc: svc}
}

// List 获取节点列表
func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

// Get 获取单个节点
func (h *NodeHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	node, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if node == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
		return
	}

	c.JSON(http.StatusOK, node)
}

// Create 创建节点
func (h *NodeHandler) Create(c *gin.Context) {
	var req service.CreateNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	node, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, node)
}

// Update 更新节点
func (h *NodeHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	var req service.UpdateNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	node, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if node == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
		return
	}

	c.JSON(http.StatusOK, node)
}

// Delete 删除节点
func (h *NodeHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的节点 ID"})
		return
	}

	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

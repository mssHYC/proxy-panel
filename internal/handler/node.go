package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"

	"golang.org/x/crypto/curve25519"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

// NodeHandler 节点管理处理器
type NodeHandler struct {
	svc     *service.NodeService
	syncSvc *service.KernelSyncService
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler(svc *service.NodeService, syncSvc *service.KernelSyncService) *NodeHandler {
	return &NodeHandler{svc: svc, syncSvc: syncSvc}
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

	// 同步内核配置
	go h.syncSvc.Sync()

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

	// 同步内核配置
	go h.syncSvc.Sync()

	c.JSON(http.StatusOK, node)
}

// GenerateRealityKeypair 生成 x25519 密钥对和 Short IDs
func (h *NodeHandler) GenerateRealityKeypair(c *gin.Context) {
	// 生成 x25519 私钥
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败"})
		return
	}
	// x25519 clamping
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// 计算公钥
	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "计算公钥失败"})
		return
	}

	// 生成 Short IDs (8个，长度递减)
	shortIDs := make([]string, 0, 8)
	lengths := []int{8, 6, 8, 10, 2, 4, 8, 4}
	for _, l := range lengths {
		buf := make([]byte, (l+1)/2)
		rand.Read(buf)
		shortIDs = append(shortIDs, hex.EncodeToString(buf)[:l])
	}

	c.JSON(http.StatusOK, gin.H{
		"private_key": base64.RawURLEncoding.EncodeToString(privateKey[:]),
		"public_key":  base64.RawURLEncoding.EncodeToString(publicKey),
		"short_ids":   shortIDs,
	})
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

	// 同步内核配置
	go h.syncSvc.Sync()

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
)

type PlanHandler struct {
	svc     *service.PlanService
	syncSvc *service.KernelSyncService
}

func NewPlanHandler(svc *service.PlanService, syncSvc *service.KernelSyncService) *PlanHandler {
	return &PlanHandler{svc: svc, syncSvc: syncSvc}
}

func (h *PlanHandler) List(c *gin.Context) {
	plans, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

func (h *PlanHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的套餐 ID"})
		return
	}
	p, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "套餐不存在"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *PlanHandler) Create(c *gin.Context) {
	var req service.PlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	p, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusCreated, p)
}

func (h *PlanHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的套餐 ID"})
		return
	}
	var req service.PlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	p, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "套餐不存在"})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusOK, p)
}

func (h *PlanHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的套餐 ID"})
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

// AssignToUser POST /api/users/:id/plan
func (h *PlanHandler) AssignToUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户 ID"})
		return
	}
	var req service.AssignPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	if err := h.svc.AssignToUser(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.syncSvc != nil {
		h.syncSvc.Trigger()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

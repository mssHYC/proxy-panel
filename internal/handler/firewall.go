package handler

import (
	"context"
	"net/http"
	"time"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/service"
	"proxy-panel/internal/service/firewall"

	"github.com/gin-gonic/gin"
)

// FirewallHandler 提供防火墙的预检与立即应用接口
type FirewallHandler struct {
	fw      *firewall.Service
	cfg     *config.Config
	db      *database.DB
	nodeSvc *service.NodeService
}

// NewFirewallHandler 构造 FirewallHandler
func NewFirewallHandler(fw *firewall.Service, cfg *config.Config, db *database.DB, nodeSvc *service.NodeService) *FirewallHandler {
	return &FirewallHandler{fw: fw, cfg: cfg, db: db, nodeSvc: nodeSvc}
}

// Probe POST /api/firewall/probe { "backend": "ufw"|"firewalld" }
// 仅检测本机该 backend 是否可用，不修改任何状态
func (h *FirewallHandler) Probe(c *gin.Context) {
	var req struct {
		Backend string `json:"backend" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	if req.Backend != "ufw" && req.Backend != "firewalld" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backend 仅支持 ufw 或 firewalld", "code": "ERR_BAD_REQUEST"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.fw.Probe(ctx, req.Backend); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": req.Backend + " 可用"})
}

// Apply POST /api/firewall/apply
// 用 settings 表里最新的 firewall_enable/firewall_backend 立即热替换 Service
// 并对存量 enable 节点端口做一次 EnsureAll（异步，不阻塞响应）
func (h *FirewallHandler) Apply(c *gin.Context) {
	// 读 settings 表覆盖 cfg 副本
	newCfg := h.cfg.Firewall
	rows, err := h.db.Query("SELECT key, value FROM settings WHERE key IN (?, ?)",
		"firewall_enable", "firewall_backend")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取设置失败", "code": "ERR_INTERNAL"})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		switch k {
		case "firewall_enable":
			if v == "true" {
				newCfg.Enable = true
			} else if v == "false" {
				newCfg.Enable = false
			}
		case "firewall_backend":
			if v == "ufw" || v == "firewalld" || v == "" {
				newCfg.Backend = v
			}
		}
	}

	// 校验：enable=true 必须有 backend
	if newCfg.Enable && newCfg.Backend == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "启用防火墙时必须选择 backend，请先保存设置", "code": "ERR_BAD_REQUEST"})
		return
	}

	// 同步更新主配置，使 /settings 回显与实际行为一致
	h.cfg.Firewall = newCfg

	if err := h.fw.Swap(newCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "应用防火墙设置失败：" + err.Error(),
			"code":  "ERR_INTERNAL",
		})
		return
	}

	// 若启用，异步对齐存量端口
	var appliedPorts int
	if h.fw.Enabled() {
		nodes, err := h.nodeSvc.ListEnabled()
		if err == nil {
			ports := make([]int, 0, len(nodes))
			for _, n := range nodes {
				ports = append(ports, n.Port)
			}
			appliedPorts = len(ports)
			go h.fw.EnsureAll(context.Background(), ports)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":       h.fw.Enabled(),
		"backend":       h.fw.CurrentBackend(),
		"applied_ports": appliedPorts,
		"message":       "已立即应用",
	})
}

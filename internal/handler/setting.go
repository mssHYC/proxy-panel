package handler

import (
	"net/http"
	"strconv"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

// SettingHandler 系统设置处理器
type SettingHandler struct {
	db        *database.DB
	cfg       *config.Config
	scheduler *service.Scheduler
}

// NewSettingHandler 创建设置处理器
func NewSettingHandler(db *database.DB, cfg *config.Config, scheduler *service.Scheduler) *SettingHandler {
	return &SettingHandler{db: db, cfg: cfg, scheduler: scheduler}
}

// Get 获取所有设置
func (h *SettingHandler) Get(c *gin.Context) {
	rows, err := h.db.Query("SELECT key, value FROM settings")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "code": "ERR_INTERNAL"})
		return
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		rows.Scan(&key, &value)
		settings[key] = value
	}
	// 附带系统证书路径 (来自 config.yaml，install.sh 安装时生成)
	settings["system_cert_path"] = h.cfg.Server.Cert
	settings["system_key_path"] = h.cfg.Server.Key
	// 防火墙：若 settings 表未存，用当前生效 cfg 值回显
	if _, ok := settings["firewall_enable"]; !ok {
		if h.cfg.Firewall.Enable {
			settings["firewall_enable"] = "true"
		} else {
			settings["firewall_enable"] = "false"
		}
	}
	if _, ok := settings["firewall_backend"]; !ok {
		settings["firewall_backend"] = h.cfg.Firewall.Backend
	}
	// Deprecated routing keys — always return empty for backward compatibility
	settings["custom_rules"] = ""
	settings["custom_rules_mode"] = ""
	c.Header("X-Deprecated-Settings", "custom_rules,custom_rules_mode")
	c.JSON(http.StatusOK, settings)
}

// Update 更新设置
// 对调度相关键（reset_cron / collect_interval / warn_percent）做语法/范围校验；
// 保存成功后若涉及调度键，调用 scheduler.Reload 热替换 cron entry
func (h *SettingHandler) Update(c *gin.Context) {
	var settings map[string]string
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}

	// Deprecated routing keys — ignore writes; surface warning
	deprecated := []string{"custom_rules", "custom_rules_mode"}
	warnings := []string{}
	for _, k := range deprecated {
		if _, ok := settings[k]; ok {
			delete(settings, k)
			warnings = append(warnings, "routing.legacy_ignored:"+k)
		}
	}

	// 预校验调度相关键
	if v, ok := settings["reset_cron"]; ok && v != "" {
		if _, err := cron.ParseStandard(v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reset_cron 表达式无效：" + err.Error(), "code": "ERR_BAD_REQUEST"})
			return
		}
	}
	if v, ok := settings["collect_interval"]; ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "collect_interval 必须为 >=10 的整数", "code": "ERR_BAD_REQUEST"})
			return
		}
	}
	if v, ok := settings["warn_percent"]; ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "warn_percent 必须为 1-100 的整数", "code": "ERR_BAD_REQUEST"})
			return
		}
	}
	// 防火墙键校验：enable 须为 true/false；backend 须为 ufw/firewalld/""；
	// enable=true 时 backend 不可为空
	if v, ok := settings["firewall_enable"]; ok {
		if v != "true" && v != "false" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "firewall_enable 必须为 true 或 false", "code": "ERR_BAD_REQUEST"})
			return
		}
	}
	if v, ok := settings["firewall_backend"]; ok {
		if v != "" && v != "ufw" && v != "firewalld" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "firewall_backend 仅支持 ufw 或 firewalld", "code": "ERR_BAD_REQUEST"})
			return
		}
	}
	if settings["firewall_enable"] == "true" {
		backend, ok := settings["firewall_backend"]
		if !ok {
			backend = h.cfg.Firewall.Backend
		}
		if backend == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "启用防火墙时必须选择 backend", "code": "ERR_BAD_REQUEST"})
			return
		}
	}

	for key, value := range settings {
		_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=?`, key, value, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败", "code": "ERR_INTERNAL"})
			return
		}
	}

	// 若涉及调度键，热重载
	if h.scheduler != nil {
		_, hasReset := settings["reset_cron"]
		_, hasCollect := settings["collect_interval"]
		_, hasWarn := settings["warn_percent"]
		if hasReset || hasCollect || hasWarn {
			if err := h.scheduler.Reload(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "设置已保存但热重载失败：" + err.Error(), "code": "ERR_INTERNAL"})
				return
			}
		}
	}

	resp := gin.H{"message": "保存成功"}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	c.JSON(http.StatusOK, resp)
}

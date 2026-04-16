package handler

import (
	"net/http"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"

	"github.com/gin-gonic/gin"
)

// SettingHandler 系统设置处理器
type SettingHandler struct {
	db  *database.DB
	cfg *config.Config
}

// NewSettingHandler 创建设置处理器
func NewSettingHandler(db *database.DB, cfg *config.Config) *SettingHandler {
	return &SettingHandler{db: db, cfg: cfg}
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
	c.JSON(http.StatusOK, settings)
}

// Update 更新设置
func (h *SettingHandler) Update(c *gin.Context) {
	var settings map[string]string
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "code": "ERR_BAD_REQUEST"})
		return
	}
	for key, value := range settings {
		_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=?`, key, value, value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败", "code": "ERR_INTERNAL"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "保存成功"})
}

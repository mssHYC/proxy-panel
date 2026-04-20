package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"proxy-panel/internal/database"

	"github.com/gin-gonic/gin"
)

// BackupHandler 数据库导出/导入
type BackupHandler struct {
	db     *database.DB
	dbPath string
}

// NewBackupHandler 创建备份 handler；dbPath 传当前主数据库文件路径，供导入后替换
func NewBackupHandler(db *database.DB, dbPath string) *BackupHandler {
	return &BackupHandler{db: db, dbPath: dbPath}
}

// Export GET /api/backup/export
// 使用 SQLite 原生的 VACUUM INTO 生成一致性快照后流式返回。
func (h *BackupHandler) Export(c *gin.Context) {
	tmp, err := os.CreateTemp("", "pp-export-*.db")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时文件失败: " + err.Error()})
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	if _, err := h.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", tmpPath)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成快照失败: " + err.Error()})
		return
	}

	filename := fmt.Sprintf("proxy-panel-%s.db", time.Now().Format("20060102-150405"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/x-sqlite3")
	c.File(tmpPath)
}

// Import POST /api/backup/import
// 接收 multipart 上传；写入临时文件→覆盖主 DB→要求由 systemd 拉起重启。
func (h *BackupHandler) Import(c *gin.Context) {
	if h.dbPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "未配置数据库路径，无法导入"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少文件参数 file"})
		return
	}

	tmpPath := filepath.Join(filepath.Dir(h.dbPath), ".import.tmp")
	if err := c.SaveUploadedFile(file, tmpPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存上传文件失败: " + err.Error()})
		return
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打开上传文件失败: " + err.Error()})
		return
	}
	header := make([]byte, 16)
	n, _ := io.ReadFull(f, header)
	f.Close()
	if n < 16 || string(header[:15]) != "SQLite format 3" {
		os.Remove(tmpPath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "不是合法的 SQLite 数据库文件"})
		return
	}

	// 关闭当前 DB 连接池后覆盖文件；实际生产中应由 systemd 拉起重启。
	if err := h.db.Close(); err != nil {
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "关闭当前数据库失败: " + err.Error()})
		return
	}

	if err := os.Rename(tmpPath, h.dbPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "替换数据库文件失败: " + err.Error()})
		return
	}
	os.Remove(h.dbPath + "-wal")
	os.Remove(h.dbPath + "-shm")

	c.JSON(http.StatusOK, gin.H{"message": "导入完成，服务即将重启，请 60 秒后刷新"})

	// 让 systemd 拉起重启
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

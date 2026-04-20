package service

import (
	"log"
	"strconv"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"

	"github.com/robfig/cron/v3"
)

// ApplySettingsToConfig 读取 settings 表中的调度相关键，覆盖 cfg.Traffic 中的对应字段
// 调用时机：启动时（main）与热重载时（Scheduler.Reload）
// 非法值会被忽略并记 log，不阻塞主流程
func ApplySettingsToConfig(db *database.DB, cfg *config.Config) error {
	rows, err := db.Query("SELECT key, value FROM settings WHERE key IN (?, ?, ?)",
		"reset_cron", "collect_interval", "warn_percent")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		if v == "" {
			continue
		}
		switch k {
		case "reset_cron":
			if _, perr := cron.ParseStandard(v); perr != nil {
				log.Printf("[设置加载] reset_cron %q 解析失败，忽略: %v", v, perr)
				continue
			}
			cfg.Traffic.ResetCron = v
		case "collect_interval":
			n, perr := strconv.Atoi(v)
			if perr != nil || n < 10 {
				log.Printf("[设置加载] collect_interval %q 非法（需 >=10），忽略", v)
				continue
			}
			cfg.Traffic.CollectInterval = n
		case "warn_percent":
			n, perr := strconv.Atoi(v)
			if perr != nil || n < 1 || n > 100 {
				log.Printf("[设置加载] warn_percent %q 非法（需 1-100），忽略", v)
				continue
			}
			cfg.Traffic.WarnPercent = n
		}
	}
	return nil
}

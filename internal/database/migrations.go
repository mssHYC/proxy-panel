package database

import (
	"encoding/json"
	"strings"

	"proxy-panel/internal/service/routing"
)

// migrate 执行数据库迁移，创建所有必要的表和索引
func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			password TEXT DEFAULT '',
			email TEXT DEFAULT '',
			protocol TEXT NOT NULL DEFAULT 'vless',
			traffic_limit INTEGER DEFAULT 0,
			traffic_used INTEGER DEFAULT 0,
			traffic_up INTEGER DEFAULT 0,
			traffic_down INTEGER DEFAULT 0,
			speed_limit INTEGER DEFAULT 0,
			reset_day INTEGER DEFAULT 1,
			reset_cron TEXT DEFAULT '',
			enable INTEGER DEFAULT 1,
			expires_at DATETIME,
			warn_sent INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'vless',
			transport TEXT NOT NULL DEFAULT 'tcp',
			kernel_type TEXT NOT NULL DEFAULT 'xray',
			settings TEXT DEFAULT '{}',
			enable INTEGER DEFAULT 1,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS traffic_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			node_id INTEGER DEFAULT 0,
			upload INTEGER DEFAULT 0,
			download INTEGER DEFAULT 0,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_user_time ON traffic_logs(user_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_time ON traffic_logs(timestamp)`,
		`CREATE TABLE IF NOT EXISTS server_traffic (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			total_up INTEGER DEFAULT 0,
			total_down INTEGER DEFAULT 0,
			limit_bytes INTEGER DEFAULT 0,
			warn_sent INTEGER DEFAULT 0,
			limit_sent INTEGER DEFAULT 0,
			reset_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT OR IGNORE INTO server_traffic (id, total_up, total_down) VALUES (1, 0, 0)`,
		`CREATE TABLE IF NOT EXISTS alert_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			channel TEXT DEFAULT '',
			status TEXT DEFAULT 'sent',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_nodes (
			user_id INTEGER NOT NULL,
			node_id INTEGER NOT NULL,
			PRIMARY KEY (user_id, node_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			actor TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			target_type TEXT DEFAULT '',
			target_id TEXT DEFAULT '',
			ip TEXT DEFAULT '',
			detail TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor)`,
		`CREATE TABLE IF NOT EXISTS subscription_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			expires_at DATETIME,
			ip_bind_enabled INTEGER NOT NULL DEFAULT 1,
			bound_ip TEXT,
			last_ip TEXT,
			last_ua TEXT,
			last_used_at DATETIME,
			use_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sub_tokens_user ON subscription_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sub_tokens_token ON subscription_tokens(token)`,
		`INSERT INTO subscription_tokens (user_id, name, token, enabled, ip_bind_enabled, created_at)
		 SELECT id, 'default', uuid, 1, 0, created_at FROM users
		 WHERE NOT EXISTS (SELECT 1 FROM subscription_tokens st WHERE st.user_id = users.id)`,
		`CREATE TABLE IF NOT EXISTS outbound_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			type TEXT NOT NULL,
			members TEXT NOT NULL DEFAULT '[]',
			kind TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS rule_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			kind TEXT NOT NULL,
			site_tags TEXT NOT NULL DEFAULT '[]',
			ip_tags TEXT NOT NULL DEFAULT '[]',
			inline_domain_suffix TEXT NOT NULL DEFAULT '[]',
			inline_domain_keyword TEXT NOT NULL DEFAULT '[]',
			inline_ip_cidr TEXT NOT NULL DEFAULT '[]',
			protocol TEXT NOT NULL DEFAULT '',
			default_group_id INTEGER,
			enabled INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (default_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS custom_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			site_tags TEXT NOT NULL DEFAULT '[]',
			ip_tags TEXT NOT NULL DEFAULT '[]',
			domain_suffix TEXT NOT NULL DEFAULT '[]',
			domain_keyword TEXT NOT NULL DEFAULT '[]',
			ip_cidr TEXT NOT NULL DEFAULT '[]',
			src_ip_cidr TEXT NOT NULL DEFAULT '[]',
			protocol TEXT NOT NULL DEFAULT '',
			port TEXT NOT NULL DEFAULT '',
			outbound_group_id INTEGER,
			outbound_literal TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (outbound_group_id) REFERENCES outbound_groups(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS rule_presets (
			code TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			enabled_categories TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_rules_sort ON custom_rules(sort_order)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	if err := db.addColumnIfNotExists("nodes", "last_check_at", "DATETIME"); err != nil {
		return err
	}
	if err := db.addColumnIfNotExists("nodes", "last_check_ok", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumnIfNotExists("nodes", "last_check_err", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := db.addColumnIfNotExists("nodes", "fail_count", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := db.seedRouting(); err != nil {
		return err
	}
	return nil
}

// addColumnIfNotExists 针对 SQLite 幂等添加列。
func (db *DB) addColumnIfNotExists(table, column, typ string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + typ)
	return err
}

// seedRouting 幂等 seed 18 系统组 / 18 系统分类 / 3 预设 / URL 前缀默认值，
// 并一次性自动导入老 settings.custom_rules / custom_rules_mode 文本到 custom_rules 表。
func (db *DB) seedRouting() error {
	// 1. 系统出站组（先 seed 组，供分类 FK）
	for _, g := range routing.SystemGroups {
		members, _ := json.Marshal(g.Members)
		if _, err := db.Exec(`INSERT INTO outbound_groups (code, display_name, type, members, kind, sort_order)
			VALUES (?, ?, ?, ?, 'system', ?)
			ON CONFLICT(code) DO UPDATE SET
				display_name=excluded.display_name,
				type=excluded.type,
				kind='system',
				sort_order=excluded.sort_order
			`, g.Code, g.DisplayName, g.Type, string(members), g.SortOrder); err != nil {
			return err
		}
	}

	// 2. 系统规则分类
	for _, c := range routing.SystemCategories {
		siteTags, _ := json.Marshal(c.SiteTags)
		ipTags, _ := json.Marshal(c.IPTags)
		ids, _ := json.Marshal(c.InlineDomainSuffix)
		idk, _ := json.Marshal(c.InlineDomainKeyword)
		iic, _ := json.Marshal(c.InlineIPCIDR)
		enabled := 0
		if c.Enabled {
			enabled = 1
		}
		var groupID *int64
		if c.DefaultGroupCode != "" {
			var id int64
			if err := db.QueryRow(`SELECT id FROM outbound_groups WHERE code = ?`, c.DefaultGroupCode).Scan(&id); err == nil {
				groupID = &id
			}
		}
		if _, err := db.Exec(`INSERT INTO rule_categories
			(code, display_name, kind, site_tags, ip_tags, inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order)
			VALUES (?, ?, 'system', ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				display_name=excluded.display_name,
				kind='system',
				site_tags=excluded.site_tags,
				ip_tags=excluded.ip_tags,
				inline_domain_suffix=excluded.inline_domain_suffix,
				inline_domain_keyword=excluded.inline_domain_keyword,
				inline_ip_cidr=excluded.inline_ip_cidr,
				protocol=excluded.protocol
			`, c.Code, c.DisplayName, string(siteTags), string(ipTags), string(ids), string(idk), string(iic), c.Protocol, groupID, enabled, c.SortOrder); err != nil {
			return err
		}
	}

	// 3. 预设
	for _, p := range routing.SystemPresets {
		ec, _ := json.Marshal(p.EnabledCategories)
		if _, err := db.Exec(`INSERT INTO rule_presets (code, display_name, enabled_categories)
			VALUES (?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				display_name=excluded.display_name,
				enabled_categories=excluded.enabled_categories
			`, p.Code, p.DisplayName, string(ec)); err != nil {
			return err
		}
	}

	// 4. settings 默认 URL 前缀（仅当不存在时插入）
	defaults := map[string]string{
		"routing.site_ruleset_base_url.clash":   routing.DefaultClashSiteBase,
		"routing.ip_ruleset_base_url.clash":     routing.DefaultClashIPBase,
		"routing.site_ruleset_base_url.singbox": routing.DefaultSingboxSiteBase,
		"routing.ip_ruleset_base_url.singbox":   routing.DefaultSingboxIPBase,
		"routing.final_outbound":                routing.DefaultFinalGroup,
		"routing.active_preset":                 "",
	}
	for k, v := range defaults {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}

	// 5. 自动导入老文本（仅执行一次）
	var marked string
	db.QueryRow(`SELECT value FROM settings WHERE key = 'routing.legacy_imported'`).Scan(&marked)
	if marked != "1" {
		if err := db.importLegacyRules(); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('routing.legacy_imported', '1')`); err != nil {
			return err
		}
	}
	return nil
}

// importLegacyRules 读取老 custom_rules 文本，解析后写入 custom_rules 表。
// override 模式则把所有系统分类 enabled 置 0。完成后删除老键。
func (db *DB) importLegacyRules() error {
	var text, mode string
	db.QueryRow(`SELECT value FROM settings WHERE key = 'custom_rules'`).Scan(&text)
	db.QueryRow(`SELECT value FROM settings WHERE key = 'custom_rules_mode'`).Scan(&mode)
	if strings.TrimSpace(text) == "" {
		_, _ = db.Exec(`DELETE FROM settings WHERE key IN ('custom_rules', 'custom_rules_mode')`)
		return nil
	}
	rules, err := routing.ParseLegacyRules(text)
	if err != nil {
		return err
	}
	groupIDByCode := map[string]int64{}
	gRows, err := db.Query(`SELECT code, id FROM outbound_groups`)
	if err != nil {
		return err
	}
	for gRows.Next() {
		var code string
		var id int64
		gRows.Scan(&code, &id)
		groupIDByCode[code] = id
	}
	gRows.Close()

	for i, r := range rules {
		code := routing.MapLegacyOutboundToCode(r.Outbound)
		var outboundGroupID *int64
		outboundLiteral := ""
		if code == "DIRECT" || code == "REJECT" {
			outboundLiteral = code
		} else if code != "" {
			id := groupIDByCode[code]
			outboundGroupID = &id
		} else {
			id := groupIDByCode["fallback"]
			outboundGroupID = &id
		}
		site, ip, ds, dk, ic := r.ToCustomRuleFields()
		siteJSON, _ := json.Marshal(site)
		ipJSON, _ := json.Marshal(ip)
		dsJSON, _ := json.Marshal(ds)
		dkJSON, _ := json.Marshal(dk)
		icJSON, _ := json.Marshal(ic)
		name := "legacy-" + r.Type + "-" + r.Value
		if len(name) > 64 {
			name = name[:64]
		}
		if _, err := db.Exec(`INSERT INTO custom_rules
			(name, site_tags, ip_tags, domain_suffix, domain_keyword, ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, '[]', '', '', ?, ?, ?)`,
			name, string(siteJSON), string(ipJSON), string(dsJSON), string(dkJSON), string(icJSON),
			outboundGroupID, outboundLiteral, i); err != nil {
			return err
		}
	}

	if strings.TrimSpace(mode) == "override" {
		if _, err := db.Exec(`UPDATE rule_categories SET enabled = 0 WHERE kind = 'system'`); err != nil {
			return err
		}
	}
	_, err = db.Exec(`DELETE FROM settings WHERE key IN ('custom_rules', 'custom_rules_mode')`)
	return err
}

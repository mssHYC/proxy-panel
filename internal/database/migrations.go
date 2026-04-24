package database

import "strings"

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

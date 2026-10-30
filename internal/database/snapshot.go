package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
)

// Snapshot includes committed WAL data without migrating or modifying the live DB.
func Snapshot(source, destination string) error {
	path, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	uri := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", uri.String()+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = db.Exec("VACUUM INTO ?", destination); err != nil {
		return fmt.Errorf("数据库快照失败: %w", err)
	}
	return nil
}

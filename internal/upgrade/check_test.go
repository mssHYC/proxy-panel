package upgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
)

func TestCheckUsesMigratedSnapshotAndCandidateCore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "panel.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Committed WAL data must be present in the snapshot, not just the main DB file.
	_, err = db.Exec(`INSERT INTO nodes (name,host,port,protocol,transport,kernel_type,settings,enable) VALUES ('test','example.com',443,'vless','tcp','xray','{}',1)`)
	if err != nil {
		t.Fatal(err)
	}
	// A table missing in the live schema is recreated only in the candidate copy.
	if _, err = db.Exec("DROP TABLE traffic_logs"); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(dir, "core")
	script := "#!/bin/sh\ncase \"$*\" in *-test*) ;; *) exit 1;; esac\ngrep -q node-1 \"$4\"\n"
	if err = os.WriteFile(core, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Auth.AdminPass = "private-password"
	cfg.Auth.JWTSecret = "0123456789abcdefghijklmnop"
	cfg.Database.Path = dbPath
	cfg.Kernel.XrayPath = filepath.Join(dir, "missing-old-core")
	cfg.Kernel.XrayConfig = filepath.Join(dir, "xray.json")
	cfg.Kernel.SingboxConfig = filepath.Join(dir, "singbox.json")
	cfg.Kernel.SingboxPath = filepath.Join(dir, "missing-singbox")
	cfgPath := filepath.Join(dir, "config.yaml")
	if err = Check(cfg, cfgPath, filepath.Join(dir, "pass"), core, ""); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='traffic_logs'").Scan(&tables); err != nil || tables != 0 {
		t.Fatal("preflight migrated live schema", err)
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM nodes").Scan(&count); err != nil || count != 1 {
		t.Fatal("live DB changed", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "pass", "check.db")); !os.IsNotExist(err) {
		t.Fatal("credential snapshot left behind")
	}
	data, err := os.ReadFile(filepath.Join(dir, "pass", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err = json.Unmarshal(data, &m); err != nil || m.XrayPath != cfg.Kernel.XrayPath {
		t.Fatal("manifest paths", err)
	}
	if err = Check(cfg, cfgPath, filepath.Join(dir, "fail"), "/does/not/exist", ""); err == nil {
		t.Fatal("missing candidate accepted")
	}
	if err = os.WriteFile(core, []byte("#!/bin/sh\nexit 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = Check(cfg, cfgPath, filepath.Join(dir, "incompatible"), core, ""); err == nil {
		t.Fatal("incompatible candidate accepted")
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagementDefaultsAndTLSValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 8080\nauth:\n  jwt_secret: abcdefghijklmnopqrstuvwxyz123456\n  admin_pass: not-the-default\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Address() != "127.0.0.1:8080" {
		t.Fatal(cfg.Server.Address())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Server.TLS = true
	if cfg.Validate() == nil {
		t.Fatal("TLS silently downgrades without certificates")
	}
	cfg.Server.Cert, cfg.Server.Key = "cert.pem", "key.pem"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Server.Listen = "::1"
	if cfg.Server.Address() != "[::1]:8080" {
		t.Fatal(cfg.Server.Address())
	}
	cfg.Server.MetricsToken = "short"
	if cfg.Validate() == nil {
		t.Fatal("weak metrics token accepted")
	}
}

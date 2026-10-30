package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagementDefaultsAndTLSValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  listen: 127.0.0.1\n  port: 8080\nauth:\n  jwt_secret: abcdefghijklmnopqrstuvwxyz123456\n  admin_pass: not-the-default\n"), 0600); err != nil {
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

func TestLoadPreservesLegacyAndExplicitListen(t *testing.T) {
	for _, tt := range []struct {
		name, listen, want string
	}{
		{"legacy omitted", "", "0.0.0.0:12597"},
		{"explicit loopback", "  listen: 127.0.0.1\n", "127.0.0.1:12597"},
		{"explicit wildcard", "  listen: 0.0.0.0\n", "0.0.0.0:12597"},
		{"explicit interface", "  listen: 192.0.2.10\n", "192.0.2.10:12597"},
		{"explicit IPv6", "  listen: '::1'\n", "[::1]:12597"},
		{"explicit empty", "  listen: ''\n", "127.0.0.1:12597"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("server:\n"+tt.listen+"  port: 12597\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Server.Address(); got != tt.want {
				t.Fatalf("listen address = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExampleConfigKeepsNewInstallOnLoopback(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != "127.0.0.1" {
		t.Fatalf("new install listen = %q", cfg.Server.Listen)
	}
}

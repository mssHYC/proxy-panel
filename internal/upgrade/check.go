// Package upgrade validates a candidate release without starting services or
// changing the live database. The installer owns the stop/replace/rollback phase.
package upgrade

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"proxy-panel/internal/config"
	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"
)

//go:embed compatibility.json
var Compatibility []byte

type Manifest struct {
	Files       []string `json:"files"`
	XrayPath    string   `json:"xray_path"`
	SingboxPath string   `json:"singbox_path"`
}

func Check(cfg *config.Config, configPath, dir, xrayCandidate, singboxCandidate string) error {
	if (xrayCandidate != "" && cfg.Kernel.XrayPath == "") || (singboxCandidate != "" && cfg.Kernel.SingboxPath == "") {
		return fmt.Errorf("更新内核前必须在配置中指定其安装路径")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Server.TLS {
		if _, err := tls.LoadX509KeyPair(cfg.Server.Cert, cfg.Server.Key); err != nil {
			return fmt.Errorf("面板 TLS: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	snapshot := filepath.Join(dir, "check.db")
	if err := database.Snapshot(cfg.Database.Path, snapshot); err != nil {
		return err
	}
	defer func() {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(snapshot + suffix)
		}
	}()
	db, err := database.Open(snapshot)
	if err != nil {
		return err
	}
	defer db.Close()
	mgr := kernel.NewManager()
	mgr.Register(kernel.NewXrayEngine(cfg.Kernel.XrayPath, cfg.Kernel.XrayConfig, cfg.Kernel.XrayAPIPort))
	mgr.Register(kernel.NewSingboxEngine(cfg.Kernel.SingboxPath, cfg.Kernel.SingboxConfig, cfg.Kernel.SingboxAPIPort))
	configs, err := service.NewKernelSyncService(db, mgr).GenerateForCheck()
	if err != nil {
		return err
	}
	if xrayCandidate == "" {
		xrayCandidate = cfg.Kernel.XrayPath
	}
	if singboxCandidate == "" {
		singboxCandidate = cfg.Kernel.SingboxPath
	}
	for _, entry := range []struct{ name, binary, current string }{
		{"xray", xrayCandidate, cfg.Kernel.XrayConfig}, {"sing-box", singboxCandidate, cfg.Kernel.SingboxConfig},
	} {
		generated, enabled := configs[entry.name]
		_, existingErr := os.Stat(entry.current)
		if !enabled && os.IsNotExist(existingErr) {
			continue
		}
		// Validate both the currently installed config and what the new panel will generate.
		if existingErr == nil {
			if err := checkCore(entry.name, entry.binary, entry.current); err != nil {
				return err
			}
		}
		if enabled {
			path := filepath.Join(dir, entry.name+".json")
			if err := os.WriteFile(path, generated, 0600); err != nil {
				return err
			}
			defer os.Remove(path)
			if err := checkCore(entry.name, entry.binary, path); err != nil {
				return err
			}
		}
	}
	manifest := Manifest{XrayPath: cfg.Kernel.XrayPath, SingboxPath: cfg.Kernel.SingboxPath, Files: []string{
		configPath, cfg.Database.Path, cfg.Database.Path + "-wal", cfg.Database.Path + "-shm", cfg.Kernel.XrayConfig, cfg.Kernel.SingboxConfig, cfg.Kernel.XrayPath, cfg.Kernel.SingboxPath,
	}}
	files := manifest.Files
	manifest.Files = nil
	for _, path := range files {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, abs)
	}
	manifest.XrayPath, _ = filepath.Abs(manifest.XrayPath)
	if manifest.SingboxPath != "" {
		manifest.SingboxPath, _ = filepath.Abs(manifest.SingboxPath)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
}

func checkCore(name, binary, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"run", "-test", "-config", path}
	if name == "sing-box" {
		args = []string{"check", "-c", path}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	// Keep configs/credentials out of terminal output. The installer stores this
	// diagnostic log with root-only permissions inside the staging directory.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s 配置不兼容: %w\n%s", name, err, output)
	}
	return nil
}

// Health verifies the actual HTTP/TLS listener, including Host/SNI restrictions.
func Health(cfg *config.Config) error {
	address := cfg.Server.Listen
	if address == "" || address == "0.0.0.0" {
		address = "127.0.0.1"
	} else if address == "::" {
		address = "::1"
	}
	host := cfg.Server.Domain
	if host == "" {
		host = address
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	scheme := "http"
	if cfg.Server.TLS {
		scheme = "https"
		pem, err := os.ReadFile(cfg.Server.Cert)
		if err != nil {
			return err
		}
		pair, err := tls.LoadX509KeyPair(cfg.Server.Cert, cfg.Server.Key)
		if err != nil {
			return err
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return err
		}
		if cfg.Server.Domain == "" && len(leaf.DNSNames) > 0 {
			host = strings.Replace(leaf.DNSNames[0], "*.", "healthcheck.", 1)
			tc.ServerName = host
		}
		tc.RootCAs = x509.NewCertPool()
		if !tc.RootCAs.AppendCertsFromPEM(pem) {
			return fmt.Errorf("面板证书无法解析")
		}
	}
	tr := &http.Transport{Proxy: nil, TLSClientConfig: tc, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(address, strconv.Itoa(cfg.Server.Port)))
	}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(scheme + "://" + net.JoinHostPort(host, strconv.Itoa(cfg.Server.Port)) + "/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("面板健康检查 HTTP %d", resp.StatusCode)
	}
	return nil
}

package website_test

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"proxy-panel/internal/model"
	"proxy-panel/internal/website"
)

// This test owns real systemd units and privileged loopback ports. It is enabled
// only on the disposable Linux CI runner, never by ordinary go test ./....
func TestRealSystemdManagedPathPermissions(t *testing.T) {
	if os.Getenv("WEBSITE_SYSTEMD_TEST") != "1" {
		t.Skip("set WEBSITE_SYSTEMD_TEST=1 on a disposable Linux systemd runner")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("the systemd integration test requires Linux and root")
	}
	nginx := os.Getenv("NGINX_BINARY")
	if !filepath.IsAbs(nginx) {
		t.Fatal("NGINX_BINARY must be an absolute path")
	}
	const service = "proxy-panel-web.service"
	const unitFile = "/run/systemd/system/" + service
	for _, filename := range []string{unitFile, "/etc/systemd/system/" + service, "/run/proxy-panel-web.pid"} {
		if _, err := os.Lstat(filename); !os.IsNotExist(err) {
			t.Fatalf("refusing to overwrite existing deployment: %s (%v)", filename, err)
		}
	}
	state, err := exec.Command("systemctl", "show", "--property=LoadState", "--value", service).CombinedOutput()
	if err != nil || strings.TrimSpace(string(state)) != "not-found" {
		t.Fatalf("reserved test service is already present: %s %v", state, err)
	}
	// Reserve the backend first; refuse to stop or replace any existing listener.
	backend, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { backend.Close() })
	front, err := net.Listen("tcp", "127.0.0.2:443")
	if err != nil {
		t.Fatal(err)
	}
	front.Close()
	root, err := os.MkdirTemp("/opt", "proxy-panel-systemd-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	managed := filepath.Join(root, "managed")
	for _, directory := range []string{root, managed} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Reuse the verified TLS fixture, but use actual systemctl/nginx commands and
	// place every child-visible file outside PrivateTmp and ProtectHome.
	originalPath := os.Getenv("PATH")
	f := newFixture(t)
	t.Setenv("PATH", originalPath)
	f.config, f.manifest = filepath.Join(managed, "nginx.conf"), filepath.Join(managed, "managed.json")
	server := httptest.NewUnstartedServer(f.server.Config.Handler)
	server.Listener.Close()
	server.Listener, server.TLS = backend, f.server.TLS
	server.StartTLS()
	t.Cleanup(server.Close)
	cert, key := filepath.Join(root, "site.crt"), filepath.Join(root, "site.key")
	write(t, cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	encodedKey, err := x509.MarshalPKCS8PrivateKey(server.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	write(t, key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey}))
	write(t, filepath.Join(root, "index.html"), []byte("managed website"))
	if err := os.Chmod(filepath.Join(root, "index.html"), 0644); err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(f.node.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	settings["cert_path"], settings["key_path"] = cert, key
	encoded, _ := json.Marshal(settings)
	f.node.Settings = string(encoded)
	encoded, _ = json.Marshal(f.node)
	write(t, filepath.Join(root, "node.json"), encoded)
	manifest := map[string]any{"mode": "proxy", "node_id": f.node.ID, "database": filepath.Join(root, "panel.db"),
		"kernel_path": filepath.Join(root, "xray.json"), "path": "/old", "node_host": f.node.Host,
		"protocol": f.node.Protocol, "host": "example.com", "sni": "example.com", "cert": cert, "key": key,
		"domains": []string{"example.com"}, "ca": cert, "listens": []string{"127.0.0.2:443"}, "webroot": root}
	encoded, _ = json.Marshal(manifest)
	write(t, f.manifest, encoded)
	write(t, f.config, []byte(fmt.Sprintf(`pid /run/proxy-panel-web.pid;
error_log stderr warn;
events { worker_connections 64; }
http {
    access_log off;
    server {
        listen 127.0.0.2:443 ssl;
        server_name example.com;
        ssl_certificate "%s";
        ssl_certificate_key "%s";
        root "%s";
        location / { try_files $uri $uri/ =404; }
        location = "/old" {
            error_page 418 = @proxy_panel_node;
            if ($http_upgrade != "") { return 418; }
            try_files $uri $uri/ =404;
        }
        location @proxy_panel_node {
            proxy_pass https://127.0.0.1:443;
            proxy_http_version 1.1;
            proxy_set_header Host "example.com";
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_ssl_server_name on;
            proxy_ssl_name "example.com";
            proxy_ssl_verify on;
            proxy_ssl_trusted_certificate "%s";
            proxy_buffering off;
        }
    }
}
`, cert, key, root, cert)))
	unit := fmt.Sprintf(`[Service]
Type=simple
ExecStartPre=%s -e stderr -t -c %s
ExecStart=%s -e stderr -c %s -g "daemon off;"
ExecReload=%s -e stderr -t -c %s
ExecReload=/bin/kill -HUP $MAINPID
KillSignal=SIGQUIT
TimeoutStopSec=5
`, nginx, f.config, nginx, f.config, nginx, f.config)
	if err := os.WriteFile(unitFile, []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		exec.Command("systemctl", "stop", service).Run()
		os.Remove(unitFile)
		os.Remove("/run/proxy-panel-web.pid")
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "reset-failed", service).Run()
	})
	for _, args := range [][]string{{"daemon-reload"}, {"start", service}} {
		if output, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			t.Fatalf("systemctl %v: %v\n%s", args, err, output)
		}
	}
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	manager := website.New(f.manifest, f.config, filepath.Join(root, "panel.db"), filepath.Join(root, "xray.json"))
	baseline, err := manager.Prepare(f.node, candidate)
	if err != nil || baseline == nil {
		t.Fatalf("baseline preflight: %v", err)
	}
	if err := baseline.CheckOld(context.Background()); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "website.test")
	if err := os.WriteFile(helper, binary, 0755); err != nil {
		t.Fatal(err)
	}
	for _, writable := range []bool{false, true} {
		name := fmt.Sprintf("%s-%t", filepath.Base(root), writable)
		args := []string{"--wait", "--pipe", "--collect", "--unit=" + name,
			"--property=NoNewPrivileges=yes", "--property=ProtectSystem=strict", "--property=ProtectHome=yes",
			"--property=PrivateTmp=yes", "--property=ProtectKernelTunables=yes", "--property=ProtectKernelModules=yes",
			"--property=ProtectKernelLogs=yes", "--property=ProtectControlGroups=yes", "--property=RestrictSUIDSGID=yes",
			"--property=RestrictNamespaces=yes", "--property=LockPersonality=yes", "--property=RestrictRealtime=yes",
			"--property=SystemCallArchitectures=native", "--property=RuntimeMaxSec=45s",
			"--setenv=WEBSITE_SANDBOX_ROOT=" + root, fmt.Sprintf("--setenv=WEBSITE_SANDBOX_WRITABLE=%t", writable),
			"--setenv=PATH=" + filepath.Dir(nginx) + ":/usr/local/bin:/usr/bin:/bin"}
		if writable {
			args = append(args, "--property=ReadWritePaths="+managed)
		}
		args = append(args, helper, "-test.run=^TestWebsiteSystemdSandboxHelper$", "-test.v")
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		output, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
		cancel()
		if err != nil {
			diagnostics, _ := exec.Command("journalctl", "--no-pager", "-n", "30", "-u", service).CombinedOutput()
			t.Fatalf("sandbox writable=%t: %v\n%s\n%s", writable, err, output, diagnostics)
		}
		assertUpgradeStatus(t, "127.0.0.2:443", cert, "/old", 101)
		assertUpgradeStatus(t, "127.0.0.2:443", cert, "/new", 404)
	}
}

func TestWebsiteSystemdSandboxHelper(t *testing.T) {
	root := os.Getenv("WEBSITE_SANDBOX_ROOT")
	if root == "" {
		t.Skip("helper runs only inside the integration test's systemd sandbox")
	}
	data, err := os.ReadFile(filepath.Join(root, "node.json"))
	if err != nil {
		t.Fatal(err)
	}
	var old model.Node
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	candidate := old
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	managed := filepath.Join(root, "managed")
	manager := website.New(filepath.Join(managed, "managed.json"), filepath.Join(managed, "nginx.conf"),
		filepath.Join(root, "panel.db"), filepath.Join(root, "xray.json"))
	change, err := manager.Prepare(old, candidate)
	if os.Getenv("WEBSITE_SANDBOX_WRITABLE") != "true" {
		if err == nil || !strings.Contains(err.Error(), "read-only file system") {
			t.Fatalf("expected the hardening to reject unmanaged writes, got %v", err)
		}
		return
	}
	if err != nil || change == nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := change.CheckOld(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := change.Begin(old, candidate); err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertUpgradeStatus(t, "127.0.0.2:443", filepath.Join(root, "site.crt"), "/new", 101)
	assertUpgradeStatus(t, "127.0.0.2:443", filepath.Join(root, "site.crt"), "/old", 404)
	if err := change.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := change.Complete(); err != nil {
		t.Fatal(err)
	}
}

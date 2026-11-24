package website_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// NGINX_BINARY enables the same public change operation with a real Nginx master.
// The systemctl boundary maps reload to that isolated, unprivileged master.
func TestRealNginxPathChangeAndRollback(t *testing.T) {
	f, address, cert := startRealNginxSite(t)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.CheckOld(context.Background()); err != nil {
		details, _ := os.ReadFile(filepath.Join(filepath.Dir(f.config), "nginx.log"))
		t.Fatalf("baseline: %v\n%s", err, details)
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertUpgradeStatus(t, address, cert, "/new", 101)
	assertUpgradeStatus(t, address, cert, "/old", 404)
	if err := change.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertUpgradeStatus(t, address, cert, "/old", 101)
	assertUpgradeStatus(t, address, cert, "/new", 404)
}

func startRealNginxSite(t *testing.T) (*fixture, string, string) {
	t.Helper()
	nginx := os.Getenv("NGINX_BINARY")
	if nginx == "" {
		t.Skip("set NGINX_BINARY for the real Nginx integration test")
	}
	nginx, err := filepath.Abs(nginx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	dir := filepath.Dir(f.config)
	for _, p := range []string{dir, filepath.Dir(dir)} {
		if err := os.Chmod(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	cert := filepath.Join(dir, "ca.pem")
	key := filepath.Join(dir, "site.key")
	encoded, err := x509.MarshalPKCS8PrivateKey(f.server.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	write(t, key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
	write(t, filepath.Join(dir, "index.html"), []byte("managed website"))
	if err := os.Chmod(filepath.Join(dir, "index.html"), 0644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	var state map[string]any
	data, _ := os.ReadFile(f.manifest)
	json.Unmarshal(data, &state)
	state["listens"] = []string{address}
	state["cert"], state["key"] = cert, key
	data, _ = json.Marshal(state)
	write(t, f.manifest, data)
	var settings map[string]any
	json.Unmarshal([]byte(f.node.Settings), &settings)
	settings["cert_path"], settings["key_path"] = cert, key
	data, _ = json.Marshal(settings)
	f.node.Settings = string(data)
	config := fmt.Sprintf(`pid /run/proxy-panel-web.pid;
error_log stderr warn;
events { worker_connections 64; }
http {
    access_log off;
    server {
        listen %s ssl default_server;
        ssl_reject_handshake on;
    }
    server {
        listen %s ssl;
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
`, address, address, cert, key, dir, cert)
	write(t, f.config, []byte(config))
	runtime := filepath.Join(dir, "running.conf")
	pid := filepath.Join(dir, "nginx.pid")
	// Only process endpoints change: neither fixture needs privileged ports or /run.
	runtimeConfig := strings.ReplaceAll(config, "pid /run/proxy-panel-web.pid;", "pid \""+pid+"\";")
	runtimeConfig = strings.ReplaceAll(runtimeConfig, "https://127.0.0.1:443;", f.server.URL+";")
	write(t, runtime, []byte(runtimeConfig))
	log, err := os.Create(filepath.Join(dir, "nginx.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	process := exec.Command(nginx, "-p", dir+"/", "-e", "stderr", "-c", runtime, "-g", "daemon off;")
	process.Stdout, process.Stderr = log, log
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { process.Process.Signal(os.Interrupt); process.Wait() })
	t.Setenv("TEST_NGINX_BINARY", nginx)
	t.Setenv("TEST_NGINX_CONFIG", f.config)
	t.Setenv("TEST_NGINX_RUNTIME", runtime)
	t.Setenv("TEST_NGINX_PID", pid)
	t.Setenv("TEST_NGINX_BACKEND", f.server.URL)
	bin := filepath.Join(dir, "bin")
	write(t, filepath.Join(bin, "nginx"), []byte("#!/bin/sh\nexec \"$TEST_NGINX_BINARY\" \"$@\"\n"))
	write(t, filepath.Join(bin, "systemctl"), []byte(`#!/usr/bin/env python3
import os, pathlib, subprocess, sys
if os.environ.get('FAKE_RELOAD_FAIL'):
    sys.exit(1)
config = pathlib.Path(os.environ['TEST_NGINX_CONFIG']).read_text()
config = config.replace('pid /run/proxy-panel-web.pid;', 'pid "' + os.environ['TEST_NGINX_PID'] + '";')
config = config.replace('https://127.0.0.1:443;', os.environ['TEST_NGINX_BACKEND'] + ';')
if 'TEST_REALITY_FALLBACK' in os.environ:
    config = config.replace('127.0.0.2:443;', os.environ['TEST_REALITY_FALLBACK'] + ';')
    config = config.replace('127.0.0.1:9443', os.environ['TEST_REALITY_WEB'])
pathlib.Path(os.environ['TEST_NGINX_RUNTIME']).write_text(config)
sys.exit(subprocess.call([os.environ['TEST_NGINX_BINARY'], '-e', 'stderr', '-c', os.environ['TEST_NGINX_RUNTIME'], '-s', 'reload']))
`))
	for _, name := range []string{"nginx", "systemctl"} {
		if err := os.Chmod(filepath.Join(bin, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return f, address, cert
}

func assertUpgradeStatus(t *testing.T, address, ca, route string, expected int) {
	t.Helper()
	data, _ := os.ReadFile(ca)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(data)
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{RootCAs: pool, ServerName: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", route)
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, fmt.Sprint(" ", expected, " ")) {
		t.Fatalf("%s: expected %d, got %s", route, expected, status)
	}
}

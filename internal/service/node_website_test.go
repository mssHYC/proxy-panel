package service_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
	"proxy-panel/internal/service"
	"proxy-panel/internal/website"

	"github.com/gin-gonic/gin"
)

func TestNodeSaveSynchronizesManagedHTTPUpgradePath(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	settings := strings.Replace(f.node.Settings, "/old", "/new", 1)
	response := f.save(t, settings)
	if response.Code != http.StatusOK {
		t.Fatalf("save status = %d: %s", response.Code, response.Body.String())
	}
	f.requirePath(t, "/new", http.StatusSwitchingProtocols)
	f.requirePath(t, "/old", http.StatusNotFound)
	if got := f.getNode(t); got.Settings != settings {
		t.Fatalf("saved node does not expose new settings: %+v", got)
	}
}

func TestNodeSaveRestoresPathWhenNginxReloadFails(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	f.fail(t, "nginx")
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "已恢复旧配置") {
		t.Fatalf("save should report restored old configuration: %d %s", response.Code, response.Body.String())
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
	f.requirePath(t, "/new", http.StatusNotFound)
	if got := f.getNode(t); got.Settings != f.node.Settings {
		t.Fatal("failed save published a new path")
	}
}

func TestNodeSaveRestoresPathWhenXrayRestartFails(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	f.fail(t, "xray")
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "已恢复旧配置") {
		t.Fatalf("save should report restored old configuration: %d %s", response.Code, response.Body.String())
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
	f.requirePath(t, "/new", http.StatusNotFound)
	if got := f.getNode(t); got.Settings != f.node.Settings {
		t.Fatal("failed save published a new path")
	}
}

func TestNodeSaveRestoresPathWhenPersistenceFails(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	if _, err := f.db.Exec(`CREATE TRIGGER reject_node_update BEFORE UPDATE ON nodes BEGIN SELECT RAISE(ABORT, 'simulated storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "已恢复旧配置") {
		t.Fatalf("save should report restored old configuration: %d %s", response.Code, response.Body.String())
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
	f.requirePath(t, "/new", http.StatusNotFound)
	if got := f.getNode(t); got.Settings != f.node.Settings {
		t.Fatal("failed save published a new path")
	}
}

func TestNodeSaveRestoresPathWhenUpgradeProbeFails(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	f.fail(t, "health")
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "已恢复旧配置") {
		t.Fatalf("save should report restored old configuration: %d %s", response.Code, response.Body.String())
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
	if got := f.getNode(t); got.Settings != f.node.Settings {
		t.Fatal("failed save published a new path")
	}
}

func TestNodeSaveReportsIncompleteRollback(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	f.fail(t, "nginx")
	f.fail(t, "rollback")
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "回退未完成") || strings.Contains(response.Body.String(), "已恢复旧配置") {
		t.Fatalf("save must expose failed recovery: %d %s", response.Code, response.Body.String())
	}
	if got := f.getNode(t); got.Settings != f.node.Settings {
		t.Fatal("failed save published a new path")
	}
}

type nodeWebsiteFixture struct {
	root  string
	nodes *service.NodeService
	node  *model.Node
	web   *httptest.Server
	api   *gin.Engine
	db    *database.DB
}

func newNodeWebsiteFixture(t *testing.T) *nodeWebsiteFixture {
	t.Helper()
	f := &nodeWebsiteFixture{root: t.TempDir()}
	write := func(name string, data []byte, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(f.root, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "cdn.example.com"},
		DNSNames: []string{"cdn.example.com", "a.example.com", "b.example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certPath := write("site.crt", certPEM, 0600)
	keyPath := write("site.key", keyPEM, 0600)
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	f.web = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(f.root, "fail-health")); err == nil && r.URL.Path == "/new" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		nginx, _ := os.ReadFile(filepath.Join(f.root, "active-nginx.conf"))
		data, _ := os.ReadFile(filepath.Join(f.root, "active-xray.json"))
		var config struct {
			Inbounds []struct {
				StreamSettings struct {
					HTTPUpgrade struct {
						Path string `json:"path"`
					} `json:"httpupgradeSettings"`
				} `json:"streamSettings"`
			} `json:"inbounds"`
		}
		_ = json.Unmarshal(data, &config)
		for _, inbound := range config.Inbounds {
			if inbound.StreamSettings.HTTPUpgrade.Path == r.URL.Path &&
				strings.Contains(string(nginx), `location = "`+r.URL.Path+`" {`) && r.Header.Get("Upgrade") != "" {
				w.Header().Set("Connection", "Upgrade")
				w.Header().Set("Upgrade", "websocket")
				w.WriteHeader(http.StatusSwitchingProtocols)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	f.web.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	f.web.StartTLS()
	f.web.Client().Transport.(*http.Transport).TLSClientConfig.ServerName = "cdn.example.com"
	t.Cleanup(f.web.Close)

	dbPath := filepath.Join(f.root, "panel.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.db = db
	f.nodes = service.NewNodeService(db, nil)
	settings, _ := json.Marshal(map[string]any{
		"security": "tls", "tls": true, "listen": "127.0.0.1", "path": "/old",
		"host": "cdn.example.com", "sni": "cdn.example.com", "alpn": []string{"http/1.1"},
		"cert_path": certPath, "key_path": keyPath,
	})
	f.node, err = f.nodes.Create(&service.CreateNodeReq{Name: "website", Host: "cdn.example.com", Port: 443,
		Protocol: "vless", Transport: "httpupgrade", KernelType: "xray", Settings: string(settings)})
	if err != nil {
		t.Fatal(err)
	}
	kernelPath := filepath.Join(f.root, "xray.json")
	mgr := kernel.NewManager()
	mgr.Register(kernel.NewXrayEngine("xray", kernelPath, 10085))
	syncSvc := service.NewKernelSyncService(db, mgr)
	configs, err := syncSvc.GenerateForCheck()
	if err != nil {
		t.Fatal(err)
	}
	write("xray.json", configs["xray"], 0600)
	write("active-xray.json", configs["xray"], 0600)
	nginx := []byte("pid /run/proxy-panel-web.pid;\nhttp {\nserver {\n        listen " + f.web.Listener.Addr().String() + " ssl default_server;\n}\nserver {\n        listen " + f.web.Listener.Addr().String() + " ssl;\nlocation = \"/old\" {\nerror_page 418 = @proxy_panel_node;\n}\nlocation @proxy_panel_node {\nproxy_pass https://127.0.0.1:443;\n}\n}\n}\n")
	nginxPath := write("nginx.conf", nginx, 0644)
	write("active-nginx.conf", nginx, 0644)
	manifest, _ := json.Marshal(map[string]any{
		"mode": "proxy", "node_id": f.node.ID, "database": dbPath, "kernel_path": kernelPath,
		"node_host": f.node.Host, "protocol": f.node.Protocol, "path": "/old", "host": "cdn.example.com",
		"sni": "cdn.example.com", "domains": []string{"cdn.example.com"}, "cert": certPath, "key": keyPath,
		"ca": certPath, "listens": []string{f.web.Listener.Addr().String()}, "listen_before": "192.0.2.10",
		"webroot": f.root,
	})
	manifestPath := write("managed.json", manifest, 0600)
	write("nginx", []byte("#!/bin/sh\nexit 0\n"), 0755)
	write("systemctl", []byte(`#!/bin/sh
set -eu
if [ "$1" = restart ] && [ "$2" = xray ]; then
    if [ -f "$NODE_WEBSITE_TEST_ROOT/fail-xray" ] && grep -q '"/new"' "$NODE_WEBSITE_TEST_ROOT/xray.json"; then exit 1; fi
    if [ -f "$NODE_WEBSITE_TEST_ROOT/fail-rollback" ] && grep -q '"/old"' "$NODE_WEBSITE_TEST_ROOT/xray.json"; then exit 1; fi
    cp "$NODE_WEBSITE_TEST_ROOT/xray.json" "$NODE_WEBSITE_TEST_ROOT/active-xray.json"
    if [ -f "$NODE_WEBSITE_TEST_ROOT/kill-after-xray" ]; then rm "$NODE_WEBSITE_TEST_ROOT/kill-after-xray"; kill -KILL "$PPID"; exit 0; fi
elif { [ "$1" = reload ] || [ "$1" = reload-or-restart ]; } && [ "$2" = proxy-panel-web.service ]; then
    if [ -f "$NODE_WEBSITE_TEST_ROOT/fail-nginx" ] && grep -q '"/new"' "$NODE_WEBSITE_TEST_ROOT/nginx.conf"; then exit 1; fi
    cp "$NODE_WEBSITE_TEST_ROOT/nginx.conf" "$NODE_WEBSITE_TEST_ROOT/active-nginx.conf"
    if [ -f "$NODE_WEBSITE_TEST_ROOT/kill-after-nginx" ]; then rm "$NODE_WEBSITE_TEST_ROOT/kill-after-nginx"; kill -KILL "$PPID"; exit 0; fi
    if [ -f "$NODE_WEBSITE_TEST_ROOT/fail-clear" ]; then chmod 500 "$NODE_WEBSITE_TEST_ROOT"; fi
fi
`), 0755)
	t.Setenv("NODE_WEBSITE_TEST_ROOT", f.root)
	t.Setenv("PATH", f.root+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.nodes.ConfigureManagedWebsite(syncSvc, website.New(manifestPath, nginxPath, dbPath, kernelPath), kernelPath)
	gin.SetMode(gin.TestMode)
	f.api = gin.New()
	h := handler.NewNodeHandler(f.nodes, syncSvc)
	f.api.PUT("/nodes/:id", h.Update)
	f.api.GET("/nodes/:id", h.Get)
	return f
}

func (f *nodeWebsiteFixture) fail(t *testing.T, boundary string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, "fail-"+boundary), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func (f *nodeWebsiteFixture) requirePath(t *testing.T, path string, status int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.web.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	res, err := f.web.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		t.Fatalf("%s status = %d, want %d", path, res.StatusCode, status)
	}
}

func (f *nodeWebsiteFixture) save(t *testing.T, settings string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"settings": settings})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/nodes/%d", f.node.ID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.api.ServeHTTP(response, req)
	return response
}

func (f *nodeWebsiteFixture) getNode(t *testing.T) model.Node {
	t.Helper()
	response := httptest.NewRecorder()
	f.api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/nodes/%d", f.node.ID), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("get node: %s", response.Body.String())
	}
	var node model.Node
	if err := json.Unmarshal(response.Body.Bytes(), &node); err != nil {
		t.Fatal(err)
	}
	return node
}

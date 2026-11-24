package website_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"proxy-panel/internal/model"
)

// Use a real Nginx master with its stream module and a local TLS fallback.
// The wrapper translates only the backend ports to ephemeral test listeners.
func TestRealNginxSharedRealityEdits(t *testing.T) {
	f, public, ca := startRealNginxSite(t)
	backend := f.server.Listener.Addr().(*net.TCPAddr)
	t.Setenv("TEST_REALITY_FALLBACK", backend.String())
	web, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REALITY_WEB", web.Addr().String())
	web.Close()

	old := model.Node{ID: 7, Name: "Reality", Host: "example.com", Enable: true, Port: backend.Port,
		Protocol: "vless", Transport: "tcp", KernelType: "xray", Settings: `{"security":"reality","dest":"a.example.com:443","sni":"a.example.com"}`}
	save := func(next model.Node) {
		t.Helper()
		change, err := f.manager.PrepareUpdate(old, &next)
		if err != nil || change == nil {
			t.Fatalf("prepare: %v (change %v)", err, change != nil)
		}
		ctx := context.Background()
		if err := change.CheckOld(ctx); err != nil {
			t.Fatalf("baseline: %v", err)
		}
		if err := change.Apply(ctx); err != nil {
			details, _ := os.ReadFile(filepath.Join(filepath.Dir(f.config), "nginx.log"))
			t.Fatalf("apply: %v\n%s", err, details)
		}
		old = next
		assertUpgradeStatus(t, public, ca, "/old", 101)
	}
	pem, err := os.ReadFile(ca)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	assertSNI := func(sni string, want bool) {
		t.Helper()
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", public, &tls.Config{ServerName: sni, RootCAs: roots})
		if conn != nil {
			conn.Close()
		}
		if (err == nil) != want {
			t.Fatalf("public SNI %s, want success %v, got %v", sni, want, err)
		}
	}
	next := old
	next.Port = 443
	save(next)
	assertSNI("a.example.com", true)
	next = old
	next.Settings = strings.ReplaceAll(next.Settings, "a.example.com", "b.example.com")
	save(next)
	assertSNI("b.example.com", true)
	assertSNI("a.example.com", false)
	next = old
	next.Port = backend.Port
	save(next)
	assertSNI("b.example.com", false)
}

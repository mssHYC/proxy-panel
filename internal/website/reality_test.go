package website_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"proxy-panel/internal/model"
	"proxy-panel/internal/website"
)

func addSharedReality(t *testing.T, f *fixture) model.Node {
	t.Helper()
	stateData, err := os.ReadFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal(err)
	}
	state["reality_frontend"] = map[string]any{"node_id": 17, "sni": "a.example.com", "listen": "127.0.0.2:443", "web_listen": "127.0.0.1:9443",
		"stream_module": "/usr/lib/nginx/modules/ngx_stream_module.so", "previous_port": 1279, "previous_listen": "0.0.0.0", "installer_extension": true}
	stateData, _ = json.Marshal(state)
	write(t, f.manifest, stateData)
	config := fmt.Sprintf(`load_module "/usr/lib/nginx/modules/ngx_stream_module.so";
pid /run/proxy-panel-web.pid;
events { worker_connections 64; }
stream {
    map $ssl_preread_server_name $proxy_panel_backend {
        a.example.com 127.0.0.2:443;
        default 127.0.0.1:9443;
    }
    server {
        listen %s;
        ssl_preread on;
        proxy_pass $proxy_panel_backend;
        proxy_timeout 3600s;
    }
}
http {
    server {
        listen 127.0.0.1:9443 ssl default_server;
        ssl_reject_handshake on;
    }
    server {
        listen 127.0.0.1:9443 ssl;
        location = "/old" {
            error_page 418 = @proxy_panel_node;
        }
        location @proxy_panel_node {
            proxy_pass https://127.0.0.1:443;
        }
    }
}
`, f.server.Listener.Addr())
	write(t, f.config, []byte(config))
	return model.Node{ID: 17, Name: "Reality", Enable: true, Port: 443, Host: "node.example.com", KernelType: "xray", Transport: "tcp", Protocol: "vless",
		Settings: `{"security":"reality","listen":"127.0.0.2","opaque":9007199254740997,"dest":"a.example.com:443","sni":"a.example.com"}`}
}

func TestRealityChangesRecoverEitherCommittedRoute(t *testing.T) {
	f := newFixture(t)
	old := addSharedReality(t, f)
	ctx := context.Background()
	runChange := func(previous, next model.Node) model.Node {
		t.Helper()
		beforeSite, _ := os.ReadFile(f.manifest)
		beforeConf, _ := os.ReadFile(f.config)
		change, err := f.manager.PrepareUpdate(previous, &next, nil)
		if err != nil || change == nil {
			t.Fatalf("prepare: %v", err)
		}
		if err := change.CheckOld(ctx); err != nil {
			t.Fatal(err)
		}
		if err := change.Begin(previous, next); err != nil {
			t.Fatal(err)
		}
		if err := change.Apply(ctx); err != nil {
			t.Fatal(err)
		}
		afterSite, _ := os.ReadFile(f.manifest)
		afterConf, _ := os.ReadFile(f.config)
		pending, err := f.manager.Pending()
		if err != nil || pending == nil || pending.NodeID() != old.ID {
			t.Fatalf("pending: %v", err)
		}
		for _, side := range []struct {
			node       model.Node
			site, conf []byte
		}{{previous, beforeSite, beforeConf}, {next, afterSite, afterConf}} {
			if err := pending.Restore(ctx, side.node); err != nil {
				t.Fatal(err)
			}
			site, _ := os.ReadFile(f.manifest)
			conf, _ := os.ReadFile(f.config)
			if !bytes.Equal(side.site, site) || !bytes.Equal(side.conf, conf) {
				t.Fatal("journal restored the wrong configuration for the committed node")
			}
		}
		if err := change.Complete(); err != nil {
			t.Fatal(err)
		}
		return next
	}

	next := old
	next.Settings = strings.ReplaceAll(strings.Replace(next.Settings, `"listen":"127.0.0.2",`, "", 1), "a.example.com", "b.example.com")
	updated := runChange(old, next)
	site, _ := os.ReadFile(f.manifest)
	conf, _ := os.ReadFile(f.config)
	if strings.Contains(string(conf), "a.example.com") || !strings.Contains(string(conf), "b.example.com 127.0.0.2:443;") ||
		!strings.Contains(string(site), `"installer_extension":true`) || !strings.Contains(updated.Settings, `"listen":"127.0.0.2"`) {
		t.Fatal("SNI save lost the new map, original frontend data, or installer bind")
	}

	independent := httptest.NewTLSServer(http.NotFoundHandler())
	defer independent.Close()
	next = updated
	next.Port = independent.Listener.Addr().(*net.TCPAddr).Port
	detached := runChange(updated, next)
	site, _ = os.ReadFile(f.manifest)
	conf, _ = os.ReadFile(f.config)
	if strings.Contains(string(site), "reality_frontend") || strings.Contains(string(conf), "load_module") || strings.Contains(string(conf), "stream {") ||
		strings.Contains(string(conf), "9443") || !strings.Contains(string(conf), "listen "+f.server.Listener.Addr().String()+" ssl default_server;") ||
		!strings.Contains(detached.Settings, `"listen":"0.0.0.0"`) || !strings.Contains(detached.Settings, `"opaque":9007199254740997`) {
		t.Fatal("detach did not restore independent listeners")
	}
	// Upgrading a legacy proxy manifest establishes the explicit mode required
	// by current installers, and a failed attach can still restore the old bytes.
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(site, &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "mode")
	site, _ = json.Marshal(legacy)
	write(t, f.manifest, site)
	next = detached
	next.Port = 443
	attached := runChange(detached, next)
	site, _ = os.ReadFile(f.manifest)
	conf, _ = os.ReadFile(f.config)
	if !strings.Contains(string(site), `"mode":"proxy"`) || !strings.Contains(string(site), fmt.Sprintf(`"previous_port":%d`, detached.Port)) || !strings.Contains(string(site), `"previous_listen":"0.0.0.0"`) ||
		!strings.Contains(string(conf), "b.example.com 127.0.0.2:443;") || !strings.Contains(attached.Settings, `"listen":"127.0.0.2"`) {
		t.Fatal("attach failed to record the independent bind or new map")
	}
}

func TestRealityEditPreservesConflictGuards(t *testing.T) {
	f := newFixture(t)
	old := addSharedReality(t, f)
	config, _ := os.ReadFile(f.config)
	for _, sni := range []string{"EXAMPLE.COM.", "default", "include", "x; return 200", "~.*", "evil.example.com\nother.example.com"} {
		t.Run(sni, func(t *testing.T) {
			node := old
			settings, _ := json.Marshal(map[string]string{"security": "reality", "listen": "127.0.0.2", "dest": "target.example.com:443", "sni": sni})
			node.Settings = string(settings)
			if _, err := f.manager.PrepareUpdate(old, &node, nil); !errors.Is(err, website.ErrManagedConflict) {
				t.Fatalf("unsafe map key accepted: %v", err)
			}
		})
	}
	// The old Nginx worker still owns the local HTTPS backend while Xray is
	// switching. An empty listen would also capture that port.
	node := old
	node.Port = 9443
	node.Settings = strings.Replace(node.Settings, `"listen":"127.0.0.2",`, "", 1)
	if _, err := f.manager.PrepareUpdate(old, &node, nil); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("occupied 9443: %v", err)
	}
	node.Port = 15000
	if change, err := f.manager.PrepareUpdate(old, &node, nil); err != nil || change == nil || strings.Contains(node.Settings, `"listen"`) {
		t.Fatalf("empty independent bind: %v", err)
	}
	write(t, f.config, bytes.Replace(config, []byte("default 127.0.0.1:9443;"), []byte("extra.example.com 127.0.0.2:443;\n        default 127.0.0.1:9443;"), 1))
	if _, err := f.manager.PrepareUpdate(old, &node, nil); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("custom map could be overwritten: %v", err)
	}
}

func TestBrokenOldRealityTargetDoesNotPreventRepairOrRecovery(t *testing.T) {
	f := newFixture(t)
	old := addSharedReality(t, f)
	// Both names are deliberately outside this TLS fixture's certificate.
	old.Settings = strings.ReplaceAll(old.Settings, "a.example.com", "old.invalid")
	for _, file := range []string{f.config, f.manifest} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		write(t, file, bytes.ReplaceAll(data, []byte("a.example.com"), []byte("old.invalid")))
	}
	next := old
	next.Settings = strings.ReplaceAll(next.Settings, "old.invalid", "new.invalid")
	change, err := f.manager.PrepareUpdate(old, &next, nil)
	if err != nil || change == nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := change.CheckOld(context.Background()); err != nil {
		t.Fatalf("broken old target blocked repair: %v", err)
	}
	if err := change.Begin(old, next); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if err := change.Apply(ctx); err == nil || !strings.Contains(err.Error(), "Reality TLS") {
		t.Fatalf("new target escaped TLS verification: %v", err)
	}
	recovery, err := f.manager.Pending()
	if err != nil || recovery == nil {
		t.Fatalf("pending: %v", err)
	}
	if err := recovery.Restore(context.Background(), old); err != nil {
		t.Fatalf("broken old target blocked recovery: %v", err)
	}
	if err := recovery.Complete(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryCanStartWhenCommittedRealityTargetGoesOffline(t *testing.T) {
	f := newFixture(t)
	old := addSharedReality(t, f)
	oldConfig, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	var offline atomic.Bool
	var handshakes atomic.Int32
	target := httptest.NewUnstartedServer(http.NotFoundHandler())
	target.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		handshakes.Add(1)
		if offline.Load() {
			return nil, errors.New("target offline")
		}
		return nil, nil
	}}
	target.StartTLS()
	defer target.Close()

	next := old
	next.Port = target.Listener.Addr().(*net.TCPAddr).Port
	change, err := f.manager.PrepareUpdate(old, &next, nil)
	if err != nil || change == nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := change.Begin(old, next); err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	savedProbes := handshakes.Load()
	if savedProbes == 0 {
		t.Fatal("live save skipped Reality TLS verification")
	}
	offline.Store(true)
	write(t, f.config, oldConfig)
	pending, err := f.manager.Pending()
	if err != nil || pending == nil {
		t.Fatalf("pending: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pending.Restore(ctx, next); err != nil {
		t.Fatalf("offline target blocked startup recovery: %v", err)
	}
	restored, err := os.ReadFile(f.config)
	if err != nil || bytes.Contains(restored, []byte("stream {")) || !bytes.Contains(restored, []byte("listen "+f.server.Listener.Addr().String()+" ssl;")) {
		t.Fatalf("committed website listener did not recover: %v", err)
	}
	if handshakes.Load() != savedProbes {
		t.Fatal("startup retried the external target after commit")
	}
	if err := pending.Complete(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRealityDetachKeepsWebsiteInterface(t *testing.T) {
	f := newFixture(t)
	old := addSharedReality(t, f)
	raw, err := os.ReadFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	front := m["reality_frontend"].(map[string]any)
	delete(front, "previous_port")
	delete(front, "previous_listen")
	raw, _ = json.Marshal(m)
	write(t, f.manifest, raw)
	for _, clear := range []bool{false, true} {
		next := old
		next.Port = 38492
		if clear {
			next.Settings = strings.Replace(next.Settings, `"listen":"127.0.0.2",`, "", 1)
		}
		change, err := f.manager.PrepareUpdate(old, &next, nil)
		if err != nil || change == nil {
			t.Fatalf("prepare: %v", err)
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(next.Settings), &settings); err != nil {
			t.Fatal(err)
		}
		want := f.server.Listener.Addr().(*net.TCPAddr).IP.String()
		if (!clear && settings["listen"] != want) || (clear && settings["listen"] != nil) {
			t.Fatalf("explicit clear=%t, effective bind=%v", clear, settings["listen"])
		}
	}
}

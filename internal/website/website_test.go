package website_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"proxy-panel/internal/model"
	"proxy-panel/internal/website"
)

type fixture struct {
	server           *httptest.Server
	manager          *website.Manager
	node             model.Node
	manifest, config string
}

func newFixture(t *testing.T, intercept ...func(http.ResponseWriter, *http.Request) bool) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{manifest: filepath.Join(dir, "managed.json"), config: filepath.Join(dir, "nginx.conf")}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(intercept) > 0 && intercept[0](w, r) {
			return
		}
		data, _ := os.ReadFile(f.config)
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(string(data), `location = "`+r.URL.Path+`" {`) && r.Header.Get("Upgrade") == "websocket" {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "Upgrade")
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	f.server = server
	t.Cleanup(server.Close)
	ca := filepath.Join(dir, "ca.pem")
	write(t, ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	settings := `{"security":"tls","path":"/old","host":"example.com","sni":"example.com","listen":"127.0.0.1","alpn":["http/1.1"],"cert_path":"/fixture/site.crt","key_path":"/fixture/site.key"}`
	f.node = model.Node{ID: 3, Name: "HTTPUpgrade", Host: "example.com", Port: 443, Protocol: "vless", Transport: "httpupgrade", KernelType: "xray", Settings: settings, Enable: true}
	db, core := filepath.Join(dir, "panel.db"), filepath.Join(dir, "xray.json")
	manifest := map[string]any{"mode": "proxy", "node_id": 3, "database": db, "kernel_path": core, "path": "/old", "node_host": "example.com", "protocol": "vless", "host": "example.com", "sni": "example.com", "cert": "/fixture/site.crt", "key": "/fixture/site.key", "domains": []string{"example.com"}, "ca": ca, "listens": []string{server.Listener.Addr().String()}, "webroot": "/custom/site", "future_field": map[string]any{"keep": true}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	write(t, f.manifest, data)
	write(t, f.config, []byte("pid /run/proxy-panel-web.pid;\nerror_log stderr warn;\nhttp {\n    server {\n        location = \"/old\" {\n            error_page 418 = @proxy_panel_node;\n        }\n        location @proxy_panel_node { proxy_pass https://127.0.0.1:443; }\n    }\n}\n"))
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(bin, "getenforce"), []byte("#!/bin/sh\necho Disabled\n"))
	write(t, filepath.Join(bin, "nginx"), []byte("#!/bin/sh\n[ -z \"$FAKE_NGINX_FAIL\" ] || { echo invalid-config >&2; exit 1; }\nexit 0\n"))
	write(t, filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\n[ -z \"$FAKE_RELOAD_FAIL\" ] || { echo reload-failed >&2; exit 1; }\nexit 0\n"))
	for _, name := range []string{"getenforce", "nginx", "systemctl"} {
		if err := os.Chmod(filepath.Join(bin, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.manager = website.New(f.manifest, f.config, db, core)
	return f
}

func TestPathSaveWaitsPastTransientWorkerSuccess(t *testing.T) {
	var upgrades atomic.Int32
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		// A stale worker briefly reports success while another generation still
		// serves the previous route. The next new connection exposes the gap.
		if r.URL.Path == "/new" && upgrades.Add(1) == 2 {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		return false
	})
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertUpgradeStatus(t, f.server.Listener.Addr().String(), filepath.Join(filepath.Dir(f.config), "ca.pem"), "/new", 101)
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPathChangeBecomesAvailableAndPreservesManifestFields(t *testing.T) {
	f := newFixture(t)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if change == nil {
		t.Fatal("managed path change was ignored")
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state["path"] != "/new" || state["future_field"].(map[string]any)["keep"] != true {
		t.Fatalf("saved manifest lost path or extension: %s", data)
	}
	// Preparing the next save also checks that the deployed location and manifest agree.
	next := candidate
	next.Settings = strings.Replace(next.Settings, `"/new"`, `"/next"`, 1)
	if _, err := f.manager.Prepare(candidate, next); err != nil {
		t.Fatal(err)
	}
}

func TestFailedReloadCanRestoreTheOldPathAndExactManifest(t *testing.T) {
	f := newFixture(t)
	oldManifest, _ := os.ReadFile(f.manifest)
	oldConfig, _ := os.ReadFile(f.config)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.CheckOld(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_RELOAD_FAIL", "1")
	if err := change.Apply(context.Background()); err == nil {
		t.Fatal("failed reload was reported as success")
	}
	t.Setenv("FAKE_RELOAD_FAIL", "")
	if err := change.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifest, _ := os.ReadFile(f.manifest)
	config, _ := os.ReadFile(f.config)
	if string(manifest) != string(oldManifest) || string(config) != string(oldConfig) {
		t.Fatal("rollback did not restore the original files")
	}
	if err := change.CheckOld(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightFailureLeavesTheExistingWebsiteUntouched(t *testing.T) {
	f := newFixture(t)
	before, _ := os.ReadFile(f.config)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	t.Setenv("FAKE_NGINX_FAIL", "1")
	if _, err := f.manager.Prepare(f.node, candidate); err == nil {
		t.Fatal("invalid Nginx configuration was accepted")
	}
	after, _ := os.ReadFile(f.config)
	if string(before) != string(after) {
		t.Fatal("failed preflight changed the website")
	}
}

func TestManagedPathRejectsAmbiguousURLsAndCouplingChanges(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/", "//new", "/a/../b", "/a/./b", "/new?x=1", "/new%2fpart", "/new\"; return 200; #"} {
		t.Run(path, func(t *testing.T) {
			candidate := f.node
			var settings map[string]any
			json.Unmarshal([]byte(candidate.Settings), &settings)
			settings["path"] = path
			encoded, _ := json.Marshal(settings)
			candidate.Settings = string(encoded)
			if _, err := f.manager.Prepare(f.node, candidate); err == nil {
				t.Fatal("ambiguous or unsafe path was accepted")
			}
		})
	}
	for _, field := range []string{"host", "sni", "listen", "cert_path", "alpn"} {
		t.Run(field, func(t *testing.T) {
			candidate := f.node
			var settings map[string]any
			json.Unmarshal([]byte(candidate.Settings), &settings)
			settings[field] = "different"
			encoded, _ := json.Marshal(settings)
			candidate.Settings = string(encoded)
			if _, err := f.manager.Prepare(f.node, candidate); err == nil {
				t.Fatal("managed upstream change was silently accepted")
			}
		})
	}
}

func TestUnrelatedNodesAndStandaloneSitesDoNotRunNginx(t *testing.T) {
	f := newFixture(t)
	t.Setenv("FAKE_NGINX_FAIL", "1")
	candidate := f.node
	candidate.ID = 4
	candidate.Settings = `{"path":"/new"}`
	if change, err := f.manager.Prepare(candidate, candidate); err != nil || change != nil {
		t.Fatalf("unrelated node: %v %v", change, err)
	}
	candidate.Port = 14443
	write(t, f.manifest, []byte(`{"mode":"standalone","listens":["192.0.2.21:443"]}`))
	if change, err := f.manager.Prepare(f.node, candidate); err != nil || change != nil {
		t.Fatalf("standalone website: %v %v", change, err)
	}
	if err := os.Remove(f.manifest); err != nil {
		t.Fatal(err)
	}
	if change, err := f.manager.Prepare(f.node, candidate); err != nil || change != nil {
		t.Fatalf("unmanaged website: %v %v", change, err)
	}
}

func TestConcurrentWebsiteEditIsNotOverwritten(t *testing.T) {
	f := newFixture(t)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	edited := []byte("edited outside the panel")
	write(t, f.config, edited)
	if err := change.Apply(context.Background()); err == nil {
		t.Fatal("concurrent edit was overwritten")
	}
	if err := change.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(f.config)
	if string(actual) != string(edited) {
		t.Fatal("rollback overwrote concurrent edit")
	}
}

func TestSymlinkedConfigurationIsNotReplaced(t *testing.T) {
	f := newFixture(t)
	target := f.config + ".original"
	if err := os.Rename(f.config, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, f.config); err != nil {
		t.Fatal(err)
	}
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	if _, err := f.manager.Prepare(f.node, candidate); err == nil {
		t.Fatal("symlinked managed config would be overwritten")
	}
}

func TestPathSaveAcceptsEquivalentLegacySettingAliases(t *testing.T) {
	f := newFixture(t)
	legacy := f.node
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"sni":`, `"serverName":`)
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"cert_path":`, `"certPath":`)
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"key_path":`, `"keyPath":`)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	candidate.Settings = strings.Replace(candidate.Settings, `"security":"tls"`, `"security":"tls","tls":true`, 1)
	change, err := f.manager.Prepare(legacy, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if change == nil {
		t.Fatal("normalized path save was ignored")
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPathSaveAcceptsOmittedEmptyHost(t *testing.T) {
	f := newFixture(t)
	f.node.Settings = strings.Replace(f.node.Settings, `"host":"example.com"`, `"host":""`, 1)
	data, err := os.ReadFile(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	state["host"] = ""
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	write(t, f.manifest, data)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"host":"",`, "", 1)
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if change == nil {
		t.Fatal("path save with omitted empty Host was ignored")
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataEditsIgnoreBrokenWebsiteManifest(t *testing.T) {
	for _, broken := range []string{"invalid-json", "unreadable"} {
		t.Run(broken, func(t *testing.T) {
			f := newFixture(t)
			if broken == "invalid-json" {
				write(t, f.manifest, []byte("{"))
			} else {
				if err := os.Remove(f.manifest); err != nil {
					t.Fatal(err)
				}
				// A directory reliably fails ReadFile, including tests run as root.
				if err := os.Mkdir(f.manifest, 0700); err != nil {
					t.Fatal(err)
				}
			}
			candidate := f.node
			candidate.Name = "renamed"
			candidate.SortOrder = 7
			if change, err := f.manager.Prepare(f.node, candidate); err != nil || change != nil {
				t.Fatalf("metadata-only edit depends on broken website state: %v %v", change, err)
			}
		})
	}
}

func TestEquivalentFormSettingsIgnoreBrokenWebsiteManifest(t *testing.T) {
	f := newFixture(t)
	write(t, f.manifest, []byte("{"))
	legacy := f.node
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"sni":`, `"serverName":`)
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"cert_path":`, `"certPath":`)
	legacy.Settings = strings.ReplaceAll(legacy.Settings, `"key_path":`, `"keyPath":`)
	legacy.Settings = strings.Replace(legacy.Settings, `"host":"example.com"`, `"host":""`, 1)
	legacy.Settings = strings.Replace(legacy.Settings, `"alpn":["http/1.1"]`, `"alpn":"http/1.1"`, 1)
	candidate := f.node
	candidate.Name = "renamed through the complete settings form"
	var settings map[string]any
	if err := json.Unmarshal([]byte(candidate.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings, "host")
	settings["tls"] = true
	settings["fingerprint"] = "chrome"
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Settings = string(encoded)
	if change, err := f.manager.Prepare(legacy, candidate); err != nil || change != nil {
		t.Fatalf("equivalent form settings depend on broken website state: %v %v", change, err)
	}
}

func TestRouteChangesStillRequireReadableWebsiteManifest(t *testing.T) {
	f := newFixture(t)
	write(t, f.manifest, []byte("{"))
	changes := map[string]func(*model.Node){
		"address":   func(node *model.Node) { node.Host = "other.example.com" },
		"port":      func(node *model.Node) { node.Port = 8443 },
		"protocol":  func(node *model.Node) { node.Protocol = "vmess" },
		"transport": func(node *model.Node) { node.Transport = "ws" },
		"engine":    func(node *model.Node) { node.KernelType = "singbox" },
		"disabled":  func(node *model.Node) { node.Enable = false },
	}
	for _, field := range []string{"path", "host", "sni", "listen", "security", "alpn", "cert_path", "key_path"} {
		changes[field] = func(node *model.Node) {
			var settings map[string]any
			if err := json.Unmarshal([]byte(node.Settings), &settings); err != nil {
				t.Fatal(err)
			}
			settings[field] = "different"
			encoded, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			node.Settings = string(encoded)
		}
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			candidate := f.node
			change(&candidate)
			if _, err := f.manager.Prepare(f.node, candidate); err == nil {
				t.Fatal("route edit bypassed a broken managed website manifest")
			}
		})
	}
}

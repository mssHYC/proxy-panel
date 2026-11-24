package service_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
	"proxy-panel/internal/service"
	"proxy-panel/internal/website"
)

// The public TLS endpoint lives in nodeWebsiteFixture. This engine starts a
// local independent TLS listener on the actual address in Xray's generated
// config, so a stale installer loopback cannot pass the direct endpoint check.
type localRealityEngine struct {
	*kernel.XrayEngine
	root   string
	cert   tls.Certificate
	direct *http.Server
}

func (e *localRealityEngine) RestartContext(ctx context.Context) error {
	if e.direct != nil {
		e.direct.Close()
		e.direct = nil
	}
	if err := e.XrayEngine.RestartContext(ctx); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(e.root, "xray.json"))
	if err != nil {
		return err
	}
	var cfg struct {
		Inbounds []struct {
			Tag, Listen string
			Port        int
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	for _, inbound := range cfg.Inbounds {
		if inbound.Tag == "node-41" && inbound.Port != 443 {
			bind := inbound.Listen
			if bind == "0.0.0.0" {
				bind = "127.0.0.1"
			}
			ln, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(inbound.Port)))
			if err != nil {
				return err
			}
			server := &http.Server{TLSConfig: &tls.Config{Certificates: []tls.Certificate{e.cert}}}
			e.direct = server
			go server.ServeTLS(ln, "", "")
		}
	}
	return nil
}

func (e *localRealityEngine) ApplyConfigContext(ctx context.Context, data []byte) error {
	if err := e.WriteConfig(data); err != nil {
		return err
	}
	return e.RestartContext(ctx)
}

type managedRealityFixture struct {
	*nodeWebsiteFixture
	site *website.Manager
	port int
}

func newManagedRealityFixture(t *testing.T) *managedRealityFixture {
	t.Helper()
	f := newNodeWebsiteFixture(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	settings := `{"security":"reality","dest":"a.example.com:443","sni":"a.example.com","listen":"0.0.0.0"}`
	if _, err := f.db.Exec(`INSERT INTO nodes (id,name,host,port,protocol,transport,kernel_type,settings,enable) VALUES (41,'reality','vpn.example.com',?,'vless','tcp','xray',?,1)`, port, settings); err != nil {
		t.Fatal(err)
	}
	kernelPath := filepath.Join(f.root, "xray.json")
	engine := &localRealityEngine{XrayEngine: kernel.NewXrayEngine("xray", kernelPath, 10085), root: f.root, cert: f.web.TLS.Certificates[0]}
	manager := kernel.NewManager()
	manager.Register(engine)
	syncer := service.NewKernelSyncService(f.db, manager)
	configs, err := syncer.GenerateForCheck()
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyConfigContext(context.Background(), configs["xray"]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if engine.direct != nil {
			engine.direct.Close()
		}
	})
	site := website.New(filepath.Join(f.root, "managed.json"), filepath.Join(f.root, "nginx.conf"), filepath.Join(f.root, "panel.db"), kernelPath).WithDeploymentLock(filepath.Join(f.root, ".update.lock"))
	f.nodes.ConfigureManagedWebsite(syncer, site, kernelPath)
	f.nodes.SetRealityCheckForTest(func(context.Context, map[string]any) (*service.RealityCheck, error) {
		return &service.RealityCheck{}, nil
	})
	return &managedRealityFixture{f, site, port}
}

func (f *managedRealityFixture) put(t *testing.T, fields map[string]any, status int) {
	t.Helper()
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/nodes/41", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.api.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("PUT returned %d, want %d: %s", response.Code, status, response.Body)
	}
}

func (f *managedRealityFixture) verify(t *testing.T, port int, bind, sni string) *model.Node {
	t.Helper()
	node, err := f.nodes.GetByID(41)
	if err != nil || node == nil {
		t.Fatalf("node: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(node.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if node.Port != port || settings["listen"] != bind || settings["sni"] != sni {
		t.Fatalf("wrong DB route: port %d, settings %s", node.Port, node.Settings)
	}
	var live struct {
		Inbounds []struct {
			Tag, Listen    string
			Port           int
			StreamSettings struct {
				RealitySettings struct{ ServerNames []string }
			}
		}
	}
	if err := json.Unmarshal(f.read(t, "active-xray.json"), &live); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, in := range live.Inbounds {
		if in.Tag == "node-41" {
			found = in.Port == port && in.Listen == bind && len(in.StreamSettings.RealitySettings.ServerNames) == 1 && in.StreamSettings.RealitySettings.ServerNames[0] == sni
		}
	}
	if !found {
		t.Fatal("Xray's active inbound disagrees with DB")
	}
	var state struct {
		RealityFrontend *struct {
			SNI            string
			PreviousPort   int    `json:"previous_port"`
			PreviousListen string `json:"previous_listen"`
		} `json:"reality_frontend"`
	}
	if err := json.Unmarshal(f.read(t, "managed.json"), &state); err != nil {
		t.Fatal(err)
	}
	active := string(f.read(t, "active-nginx.conf"))
	if port == 443 {
		if state.RealityFrontend == nil || state.RealityFrontend.SNI != sni || state.RealityFrontend.PreviousPort != f.port || state.RealityFrontend.PreviousListen != "0.0.0.0" || !strings.Contains(active, sni+" 127.0.0.2:443;") {
			t.Fatal("active shared route, manifest or prior bind disagree")
		}
	} else if state.RealityFrontend != nil || strings.Contains(active, "ssl_preread") || !strings.Contains(active, "listen "+f.web.Listener.Addr().String()+" ssl;") {
		t.Fatal("independent node still has a shared website route")
	}
	if pending, err := f.site.Pending(); err != nil || pending != nil {
		t.Fatalf("uncleared journal: %v", err)
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
	return node
}

func (f *managedRealityFixture) read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestManagedRealitySaveDuringCLIDeployment(t *testing.T) {
	f := newManagedRealityFixture(t)
	unlock, err := f.site.LockDeployment()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	f.put(t, map[string]any{"port": 443}, 409)
	f.verify(t, f.port, "0.0.0.0", "a.example.com")
	name := "renamed"
	if _, err := f.nodes.Update(41, &service.UpdateNodeReq{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.RecoverManagedWebsite(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedRealitySaveRoundTrip(t *testing.T) {
	f := newManagedRealityFixture(t)
	f.put(t, map[string]any{"port": 443}, 200)
	shared := f.verify(t, 443, "127.0.0.2", "a.example.com")
	newSettings := strings.ReplaceAll(shared.Settings, "a.example.com", "b.example.com")
	newSettings = strings.Replace(newSettings, `"listen":"127.0.0.2"`, `"listen":""`, 1)
	f.put(t, map[string]any{"settings": newSettings}, 200)
	shared = f.verify(t, 443, "127.0.0.2", "b.example.com")
	f.put(t, map[string]any{"port": f.port, "settings": shared.Settings}, 200)
	f.verify(t, f.port, "0.0.0.0", "b.example.com")
}

func TestManagedRealityRejectsBusyPortAndRestoresOnDBFailure(t *testing.T) {
	f := newManagedRealityFixture(t)
	if _, err := f.db.Exec(`INSERT INTO nodes (name,host,port,protocol,transport,kernel_type,settings,enable) VALUES ('busy','other.example',9443,'vless','tcp','singbox','{}',1)`); err != nil {
		t.Fatal(err)
	}
	f.put(t, map[string]any{"port": 443}, 409)
	f.verify(t, f.port, "0.0.0.0", "a.example.com")
	if _, err := f.db.Exec(`UPDATE nodes SET enable=0 WHERE name='busy'`); err != nil {
		t.Fatal(err)
	}
	f.put(t, map[string]any{"port": 443}, 200)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	shared, err := f.nodes.GetByID(41)
	if err != nil || shared == nil {
		t.Fatalf("node: %v", err)
	}
	occupiedBind := strings.Replace(shared.Settings, "127.0.0.2", "127.0.0.1", 1)
	f.put(t, map[string]any{"port": occupied.Addr().(*net.TCPAddr).Port, "settings": occupiedBind}, 409)
	f.verify(t, 443, "127.0.0.2", "a.example.com")
	if _, err := f.db.Exec(`CREATE TRIGGER reject_reality_save BEFORE UPDATE ON nodes WHEN NEW.id=41 BEGIN SELECT RAISE(ABORT, 'storage full'); END`); err != nil {
		t.Fatal(err)
	}
	f.put(t, map[string]any{"port": f.port}, 500)
	f.verify(t, 443, "127.0.0.2", "a.example.com")
}

func TestManagedRealitySaveRejectsDriftedXrayConfig(t *testing.T) {
	f := newManagedRealityFixture(t)
	path := filepath.Join(f.root, "xray.json")
	config := bytes.Replace(f.read(t, "xray.json"), []byte(`"tag": "node-41"`), []byte(`"tag": "unmanaged"`), 1)
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	f.put(t, map[string]any{"port": 443}, 409)
	if !bytes.Equal(f.read(t, "xray.json"), config) {
		t.Fatal("save overwrote an externally changed config")
	}
	f.verify(t, f.port, "0.0.0.0", "a.example.com")
}

func TestManagedRealityStartupFollowsCommittedPort(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprintf("committed-%v", committed), func(t *testing.T) {
			f := newManagedRealityFixture(t)
			old, err := f.nodes.GetByID(41)
			if err != nil || old == nil {
				t.Fatalf("node: %v", err)
			}
			next := *old
			next.Port = 443
			change, err := f.site.PrepareUpdate(*old, &next)
			if err != nil || change == nil {
				t.Fatalf("prepare: %v", err)
			}
			if err := change.Begin(*old, next); err != nil {
				t.Fatal(err)
			}
			if committed {
				if _, err := f.db.Exec("UPDATE nodes SET port=?,settings=? WHERE id=41", next.Port, next.Settings); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.nodes.RecoverManagedWebsite(); err != nil {
				t.Fatal(err)
			}
			if committed {
				f.verify(t, 443, "127.0.0.2", "a.example.com")
			} else {
				f.verify(t, f.port, "0.0.0.0", "a.example.com")
			}
			if err := f.nodes.RecoverManagedWebsite(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

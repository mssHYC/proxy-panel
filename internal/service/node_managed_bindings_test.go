package service_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"proxy-panel/internal/handler"
	"proxy-panel/internal/service"
	"proxy-panel/internal/website"
)

func TestManagedWebsiteGuardsNodeMutations(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	stateFile := filepath.Join(f.root, "managed.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	state["listens"] = []string{"192.0.2.21:443"}
	state["reality_frontend"] = map[string]any{"node_id": 91, "sni": "origin.example.com", "listen": "127.0.0.2:443", "web_listen": "127.0.0.1:9443"}
	data, _ = json.Marshal(state)
	if err := os.WriteFile(stateFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, listen string
		port         int
	}{
		{"implicit wildcard", "", 443}, {"explicit wildcard", "::", 443}, {"public", "192.0.2.21", 443},
		{"mapped public", "::ffff:192.0.2.21", 443}, {"website backend", "127.0.0.1", 443},
		{"Reality backend", "127.0.0.2", 443}, {"Nginx backend", "127.0.0.1", 9443},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := json.Marshal(map[string]string{"listen": tc.listen})
			if _, err := f.nodes.Create(&service.CreateNodeReq{Name: "conflict", Host: "example.com", Protocol: "vless", Port: tc.port, Settings: string(s)}); !errors.Is(err, website.ErrManagedConflict) {
				t.Fatalf("create returned %v", err)
			}
		})
	}
	n, err := f.nodes.Create(&service.CreateNodeReq{Name: "editable", Host: "example.com", Protocol: "vless", Port: 1279})
	if err != nil {
		t.Fatal(err)
	}
	port := 443
	if _, err = f.nodes.Update(n.ID, &service.UpdateNodeReq{Port: &port}); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("update accepted occupied listener: %v", err)
	}
	enabled := false
	if _, err = f.nodes.Update(n.ID, &service.UpdateNodeReq{Port: &port, Enable: &enabled}); err != nil {
		t.Fatal(err)
	}
	enabled = true
	if _, err = f.nodes.Update(n.ID, &service.UpdateNodeReq{Enable: &enabled}); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("enable accepted collision: %v", err)
	}
	stored, err := f.nodes.GetByID(n.ID)
	if err != nil || stored.Enable {
		t.Fatalf("failed enable persisted: %v", err)
	}
	if _, err = f.nodes.Create(&service.CreateNodeReq{Name: "different IPv4 bind", Host: "example.com", Port: 443, Protocol: "vless", Settings: `{"listen":"127.0.0.3"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.nodes.Create(&service.CreateNodeReq{Name: "UDP", Host: "example.com", Port: 443, Protocol: "hysteria2", KernelType: "singbox"}); err != nil {
		t.Fatal(err)
	}
	if err = f.nodes.Delete(f.node.ID); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("deleted managed HTTPUpgrade: %v", err)
	}
	if got := f.getNode(t); got.ID != f.node.ID {
		t.Fatal("managed website node no longer exists")
	}
}

func TestManagedRealityRetainsItsRoutedSNI(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	// An existing installation binds a specific Reality node and SNI in Nginx.
	const settings = `{"listen":"127.0.0.2","security":"reality","dest":"example.com:443","serverNames":["a.example.com","b.example.com"]}`
	if _, err := f.db.Exec(`INSERT INTO nodes (id,name,host,port,protocol,transport,kernel_type,settings,enable) VALUES (41,'reality','example.com',443,'vless','tcp','xray',?,1)`, settings); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "managed.json")
	b, _ := os.ReadFile(path)
	var state map[string]any
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	state["reality_frontend"] = map[string]any{"node_id": 41, "listen": "127.0.0.2:443", "sni": "a.example.com", "web_listen": "127.0.0.1:9443"}
	b, _ = json.Marshal(state)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	other := `{"listen":"127.0.0.2","security":"reality","dest":"example.com:443","serverNames":["b.example.com","a.example.com"]}`
	off := false
	free := 14739
	for _, req := range []service.UpdateNodeReq{{Settings: &other}, {Enable: &off}, {Port: &free}} {
		if _, err := f.nodes.Update(41, &req); !errors.Is(err, website.ErrManagedConflict) {
			t.Fatalf("managed edit: %v", err)
		}
	}
	if err := f.nodes.Delete(41); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("managed delete: %v", err)
	}
	node, err := f.nodes.GetByID(41)
	if err != nil || node == nil || node.Settings != settings || node.Port != 443 || !node.Enable {
		t.Fatalf("managed Reality node changed: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	name := "renamed"
	if _, err := f.nodes.Update(41, &service.UpdateNodeReq{Name: &name}); err != nil {
		t.Fatalf("metadata edit: %v", err)
	}
}

func TestNodeHTTPRejectsManagedConflictsWithoutChangingLiveRoute(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	free, err := f.nodes.Create(&service.CreateNodeReq{Name: "other", Host: "other.example.com", Port: 18080, Protocol: "vless"})
	if err != nil {
		t.Fatal(err)
	}
	// No handler-level sync is available: a rejected request must never enqueue one.
	h := handler.NewNodeHandler(f.nodes, nil)
	api := gin.New()
	api.POST("/nodes", h.Create)
	api.PUT("/nodes/:id", h.Update)
	api.DELETE("/nodes/:id", h.Delete)
	websitePort := f.web.Listener.Addr().(*net.TCPAddr).Port
	before, err := f.nodes.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/nodes", fmt.Sprintf(`{"name":"duplicate","host":"other.example.com","protocol":"vless","port":%d}`, websitePort)},
		{"PUT", fmt.Sprint("/nodes/", free.ID), fmt.Sprintf(`{"port":%d}`, websitePort)},
		{"DELETE", fmt.Sprint("/nodes/", f.node.ID), ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "托管网站") {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	after, err := f.nodes.List()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected requests changed stored nodes: %v", err)
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
}

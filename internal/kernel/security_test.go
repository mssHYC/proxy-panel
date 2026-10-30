package kernel

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestXraySecurityBoundary(t *testing.T) {
	e := NewXrayEngine("", "", 10085)
	node := NodeConfig{Tag: "node-1", Port: 443, Protocol: "vless", Transport: "tcp", Settings: map[string]interface{}{
		"listen": "127.0.0.1", "security": "tls", "sni": "example.com", "flow": "xtls-rprx-vision",
		"fallback_port": float64(8081), "alpn": []interface{}{"http/1.1"},
	}}
	data, err := e.GenerateConfig([]NodeConfig{node}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	ib := cfg["inbounds"].([]interface{})[1].(map[string]interface{})
	if ib["listen"] != "127.0.0.1" {
		t.Fatal("listen setting ignored")
	}
	tls := ib["streamSettings"].(map[string]interface{})["tlsSettings"].(map[string]interface{})
	if tls["rejectUnknownSni"] != true {
		t.Fatal("unknown SNI accepted by default")
	}
	if tls["alpn"].([]interface{})[0] != "http/1.1" {
		t.Fatal("ALPN ignored")
	}
	fallbacks, ok := ib["settings"].(map[string]interface{})["fallbacks"].([]interface{})
	if !ok || fallbacks[0].(map[string]interface{})["dest"] != "127.0.0.1:8081" {
		t.Fatalf("missing local website fallback: %v", fallbacks)
	}
	routing := cfg["routing"].(map[string]interface{})
	if routing["domainStrategy"] != "IPOnDemand" {
		t.Fatal("domains must be resolved before private-IP matching")
	}
	rules := routing["rules"].([]interface{})
	if rules[0].(map[string]interface{})["outboundTag"] != "api" {
		t.Fatal("panel API route must precede the private-IP deny rule")
	}
	if rules[1].(map[string]interface{})["outboundTag"] != "blocked" || rules[1].(map[string]interface{})["ip"] == nil {
		t.Fatal("proxy traffic can reach private IPs")
	}
}

func TestSingboxControllerRequiresSecretAndKeepsStatsWorking(t *testing.T) {
	e := NewSingboxEngine("", "", 9090)
	data, err := e.GenerateConfig([]NodeConfig{{Tag: "node-1", Protocol: "hysteria2", Settings: map[string]interface{}{"listen": "::1"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	api := cfg["experimental"].(map[string]interface{})["clash_api"].(map[string]interface{})
	secret, _ := api["secret"].(string)
	if len(secret) < 32 {
		t.Fatal("controller has no strong secret")
	}
	if cfg["inbounds"].([]interface{})[0].(map[string]interface{})["listen"] != "::1" {
		t.Fatal("sing-box listen setting ignored")
	}
	if cfg["route"] == nil {
		t.Fatal("missing egress isolation")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("stats request is unauthenticated")
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"connections":[]}`))
	}))
	defer srv.Close()
	e.apiPort, _ = strconv.Atoi(srv.URL[len("http://127.0.0.1:"):])
	if _, err := e.GetTrafficStats(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidSecuritySettings(t *testing.T) {
	for _, tc := range []struct {
		name, engine, transport, security, protocol string
		extra                                       map[string]interface{}
	}{
		{"bad listen", "xray", "tcp", "tls", "vless", map[string]interface{}{"listen": "localhost"}},
		{"array listen", "xray", "tcp", "tls", "vless", map[string]interface{}{"listen": []string{"127.0.0.1"}}},
		{"WS Vision", "xray", "ws", "tls", "vless", map[string]interface{}{"flow": "xtls-rprx-vision"}},
		{"plaintext Vision", "xray", "tcp", "none", "vless", map[string]interface{}{"flow": "xtls-rprx-vision"}},
		{"Reality fallback", "xray", "tcp", "reality", "vless", map[string]interface{}{"fallback_port": 8081}},
		{"singbox fallback", "singbox", "tcp", "tls", "vless", map[string]interface{}{"fallback_port": 8081}},
		{"WS fallback", "xray", "ws", "tls", "vless", map[string]interface{}{"fallback_port": 8081}},
		{"VMess fallback", "xray", "tcp", "tls", "vmess", map[string]interface{}{"fallback_port": 8081}},
		{"loop fallback", "xray", "tcp", "tls", "vless", map[string]interface{}{"fallback_port": 443}},
		{"fractional port", "xray", "tcp", "tls", "vless", map[string]interface{}{"fallback_port": 8081.5}},
		{"h2 fallback", "xray", "tcp", "tls", "vless", map[string]interface{}{"fallback_port": 8081, "alpn": []string{"h2"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.extra["security"] = tc.security
			if ValidateNodeSecurity(NodeConfig{Protocol: tc.protocol, Transport: tc.transport, Port: 443, Settings: tc.extra}, tc.engine) == nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
}

func TestControllerSecretPersistsAndPrivateDestinationsAreBlocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "singbox.json")
	e := NewSingboxEngine("", path, 9090)
	data, err := e.GenerateConfig([]NodeConfig{{Protocol: "vless", Settings: map[string]interface{}{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.WriteConfig(data); err != nil {
		t.Fatal(err)
	}
	if NewSingboxEngine("", path, 9090).apiSecret != e.apiSecret {
		t.Fatal("panel restart rotated the active API secret")
	}
	prefixes := blockedDestinations()
	for _, address := range []string{"127.0.0.1", "10.1.1.1", "169.254.169.254", "192.168.1.1", "172.16.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1"} {
		found := false
		for _, cidr := range prefixes {
			_, p, err := net.ParseCIDR(cidr)
			if err != nil {
				t.Fatal(err)
			}
			if p.Contains(net.ParseIP(address)) {
				found = true
			}
		}
		if !found {
			t.Fatalf("management destination not blocked: %s", address)
		}
	}
	for _, cidr := range prefixes {
		_, p, _ := net.ParseCIDR(cidr)
		if p.Contains(net.ParseIP("1.1.1.1")) {
			t.Fatalf("public destination blocked by %s", cidr)
		}
	}
}

func TestExistingConfigPermissionsAreRestrictedBeforeWritingSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateConfig(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credentials readable by others: %o", info.Mode().Perm())
	}
}

package subscription

import (
	"net/url"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"proxy-panel/internal/model"
)

func TestOrdinaryTLSOptionsSurviveSubscriptionExport(t *testing.T) {
	node := model.Node{Name: "tls", Host: "example.com", Port: 443, Protocol: "vless", Transport: "tcp", Settings: `{"security":"tls","fingerprint":"firefox","alpn":["http/1.1"],"flow":"xtls-rprx-vision"}`}
	user := &model.User{UUID: "00000000-0000-0000-0000-000000000001"}
	var parsed struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte("proxies:\n"+(&ClashGenerator{}).buildProxy(node, user)), &parsed); err != nil {
		t.Fatal(err)
	}
	p := parsed.Proxies[0]
	if p["tls"] != true || p["client-fingerprint"] != "firefox" || p["flow"] != "xtls-rprx-vision" {
		t.Fatalf("lost TLS settings: %v", p)
	}
	if p["alpn"] == nil {
		t.Fatal("ALPN array lost")
	}
	u, err := url.Parse(buildNodeURI(node, user))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("fp") != "firefox" || u.Query().Get("alpn") != "http/1.1" {
		t.Fatalf("URI lost TLS settings: %s", u)
	}
	ob := (&SingboxGenerator{}).buildVLESS(node, user, parseSettings(node))
	tls := ob["tls"].(map[string]interface{})
	if tls["utls"] == nil || tls["alpn"] == nil {
		t.Fatalf("sing-box lost TLS settings: %v", tls)
	}
}

func TestClientControllerIsLocalByDefault(t *testing.T) {
	if strings.Contains(clashGlobalPreamble, "external-controller: 0.0.0.0") || strings.Contains(clashGlobalPreamble, "allow-lan: true") {
		t.Fatal("subscription exposes the client controller or proxy on the LAN")
	}
}

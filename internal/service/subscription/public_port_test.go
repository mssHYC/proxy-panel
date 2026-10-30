package subscription

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"proxy-panel/internal/model"
)

func TestVMessReverseProxyUsesPublicPortInEveryExport(t *testing.T) {
	user := &model.User{UUID: "00000000-0000-0000-0000-000000000001"}
	for _, transport := range []string{"ws", "httpupgrade"} {
		t.Run(transport, func(t *testing.T) {
			node := model.Node{Name: "vmess-web", Host: "cdn.example.com", Port: 18080,
				Protocol: "vmess", Transport: transport, KernelType: "xray",
				Settings: `{"listen":"127.0.0.1","security":"tls","sni":"cdn.example.com","host":"cdn.example.com","path":"/vmess","public_port":443}`}
			var clash struct {
				Proxies []map[string]interface{} `yaml:"proxies"`
			}
			if err := yaml.Unmarshal([]byte("proxies:\n"+(&ClashGenerator{}).buildProxy(node, user)), &clash); err != nil {
				t.Fatal(err)
			}
			if p := clash.Proxies[0]; p["port"] != 443 || p["server"] != node.Host || p["uuid"] != user.UUID || p["tls"] != true {
				t.Fatalf("Clash lost public endpoint or credentials: %v", p)
			}
			ob := (&SingboxGenerator{}).buildOutbound(node, user)
			if ob["server_port"] != 443 || ob["uuid"] != user.UUID {
				t.Fatalf("sing-box: %v", ob)
			}
			encoded := strings.TrimPrefix(buildNodeURI(node, user), "vmess://")
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			var uri map[string]interface{}
			if err := json.Unmarshal(data, &uri); err != nil {
				t.Fatal(err)
			}
			if uri["port"] != float64(443) || uri["id"] != user.UUID || uri["net"] != transport {
				t.Fatalf("VMess/Shadowrocket URI: %s", data)
			}
			line := (&SurgeGenerator{}).buildProxyLine(node, user)
			if transport == "ws" && !strings.Contains(line, "vmess, cdn.example.com, 443,") {
				t.Fatalf("Surge: %s", line)
			}
			if transport == "httpupgrade" && line != "" {
				t.Fatalf("Surge must omit unsupported HTTPUpgrade, got %s", line)
			}
			if node.Port != 18080 {
				t.Fatal("export changed backend port")
			}
		})
	}
}

func TestVMessWithoutReverseProxyKeepsOriginalPort(t *testing.T) {
	node := model.Node{Name: "direct", Host: "node.example.com", Port: 18080, Protocol: "vmess", Transport: "ws", Settings: `{"security":"tls"}`}
	if ob := (&SingboxGenerator{}).buildOutbound(node, &model.User{}); ob["server_port"] != 18080 {
		t.Fatalf("changed direct endpoint: %v", ob)
	}
}

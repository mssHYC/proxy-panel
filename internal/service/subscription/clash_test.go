package subscription

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// Clash Verge 等部分客户端不识别 mihomo 原生的 network: httpupgrade 写法，
// 需要用 ws + ws-opts.v2ray-http-upgrade: true 的兼容写法才能握手成功。
// 订阅生成器必须输出兼容写法，否则客户端节点"看似有配置，连接却一直超时"。
func TestClashGenerator_VLESSHttpupgradeUsesWSCompatForm(t *testing.T) {
	g := &ClashGenerator{}
	node := model.Node{
		Name:      "VLESS-HTTPUpgrade",
		Host:      "ccdn.example.com",
		Port:      443,
		Protocol:  "vless",
		Transport: "httpupgrade",
		Settings: `{
			"security": "tls",
			"sni": "ccdn.example.com",
			"tls": true,
			"path": "/a3f9c2",
			"host": "ccdn.example.com"
		}`,
	}
	user := &model.User{UUID: "a085accd-889a-4580-89b6-378bd28d4dd5"}

	proxy := g.buildProxy(node, user)

	mustContain := []string{
		"network: ws",              // 兼容写法：不是 network: httpupgrade
		"v2ray-http-upgrade: true", // 关键：告诉 mihomo 走 httpupgrade 语义
		"path: /a3f9c2",
		"Host: ccdn.example.com",
	}
	for _, s := range mustContain {
		if !strings.Contains(proxy, s) {
			t.Errorf("缺字段 %q\n生成的配置:\n%s", s, proxy)
		}
	}

	if strings.Contains(proxy, "network: httpupgrade") {
		t.Errorf("不应再输出原生 network: httpupgrade（Clash Verge 不识别）\n生成的配置:\n%s", proxy)
	}
}

func TestClashGenerator_DirectChinaIPRulesUseNoResolveAndValidYAML(t *testing.T) {
	g := &ClashGenerator{}
	plan := &routing.Plan{
		Groups: []routing.OutboundGroup{
			{Code: "direct", DisplayName: "本地直连", Type: "selector", Members: []string{"DIRECT", "node_select"}},
			{Code: "node_select", DisplayName: "手动切换", Type: "selector", Members: []string{"<ALL>", "DIRECT"}},
		},
		Providers: routing.Providers{
			Site: map[string]routing.ProviderURLs{
				"cn": {Clash: "https://example.com/geosite/cn.mrs"},
			},
			IP: map[string]routing.ProviderURLs{
				"cn":      {Clash: "https://example.com/geoip/cn.mrs"},
				"private": {Clash: "https://example.com/geoip/private.mrs"},
			},
		},
		Rules: []routing.Rule{
			{SiteTags: []string{"cn"}, IPTags: []string{"cn"}, IPCIDR: []string{"104.194.92.45/32"}, Outbound: "direct"},
			{IPTags: []string{"private"}, Outbound: "direct"},
		},
		Final: "node_select",
	}

	content, _, err := g.GenerateWithPlan(plan, nil, &model.User{UUID: "00000000-0000-0000-0000-000000000000"}, "", "")
	if err != nil {
		t.Fatalf("GenerateWithPlan returned error: %v", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("generated clash yaml should parse, error: %v\ncontent:\n%s", err, content)
	}
	for _, key := range []string{"proxies", "proxy-groups", "rule-providers", "rules"} {
		if _, ok := doc[key]; !ok {
			t.Fatalf("generated yaml missing top-level key %q\ncontent:\n%s", key, content)
		}
	}

	mustContain := []string{
		`RULE-SET,cn,本地直连`,
		`RULE-SET,cn-ip,本地直连,no-resolve`,
		`RULE-SET,private-ip,本地直连,no-resolve`,
		`IP-CIDR,104.194.92.45/32,本地直连,no-resolve`,
		`MATCH,手动切换`,
	}
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("missing expected rule %q\ncontent:\n%s", s, content)
		}
	}

	groups, ok := doc["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("proxy-groups should be a non-empty yaml list, got %#v", doc["proxy-groups"])
	}
	firstGroup, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("first proxy group should be a map, got %#v", groups[0])
	}
	if firstGroup["name"] != "本地直连" || firstGroup["type"] != "select" {
		t.Fatalf("direct group should render as a Clash select group, got %#v", firstGroup)
	}
	members, ok := firstGroup["proxies"].([]any)
	if !ok || len(members) == 0 || members[0] != "DIRECT" {
		t.Fatalf("direct group should keep DIRECT as the first member, got %#v", firstGroup["proxies"])
	}
}

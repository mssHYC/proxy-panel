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

// 防止 seed 退回 IPTags:["private"]：MetaCubeX private.mrs 含 198.18.0.0/15
// (RFC2544)，与 Mihomo fake-ip 默认段重叠，会让 fake-ip 流量在 DOMAIN 规则
// 之前就被识别为局域网走 DIRECT。系统预置必须用显式 InlineIPCIDR 列表。
func TestSystemCategories_PrivateAvoidsFakeIPRange(t *testing.T) {
	var private *routing.SystemCategory
	for i := range routing.SystemCategories {
		if routing.SystemCategories[i].Code == "private" {
			private = &routing.SystemCategories[i]
			break
		}
	}
	if private == nil {
		t.Fatalf("system category 'private' not found")
	}
	if len(private.IPTags) > 0 {
		t.Errorf("private category must not use geoip rule-set IPTags (含 198.18.0.0/15)，got %v", private.IPTags)
	}
	if len(private.InlineIPCIDR) == 0 {
		t.Fatalf("private category should declare InlineIPCIDR explicitly")
	}
	for _, c := range private.InlineIPCIDR {
		if c == "198.18.0.0/15" || c == "198.18.0.0/16" {
			t.Errorf("private CIDR list must not include fake-ip range %s", c)
		}
	}
	mustHave := []string{"10.0.0.0/8", "127.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, want := range mustHave {
		found := false
		for _, c := range private.InlineIPCIDR {
			if c == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("private CIDR list missing %s", want)
		}
	}
}

// 配套渲染回归：当 plan 用 InlineIPCIDR 描述 private 时，Clash 输出必须是
// IP-CIDR 行，而不是 RULE-SET,private-ip,... —— 后者会因 geoip:private 含
// fake-ip 段把 claude.ai/claude.com 等 fake-ip 流量劫持到 DIRECT。
func TestClashGenerator_PrivateRendersAsInlineCIDR(t *testing.T) {
	g := &ClashGenerator{}
	plan := &routing.Plan{
		Groups: []routing.OutboundGroup{
			{Code: "direct", DisplayName: "本地直连", Type: "selector", Members: []string{"DIRECT"}},
			{Code: "node_select", DisplayName: "手动切换", Type: "selector", Members: []string{"DIRECT"}},
		},
		Rules: []routing.Rule{
			{IPCIDR: []string{"10.0.0.0/8", "127.0.0.0/8", "192.168.0.0/16"}, Outbound: "direct"},
		},
		Final: "node_select",
	}
	content, _, err := g.GenerateWithPlan(plan, nil, &model.User{UUID: "00000000-0000-0000-0000-000000000000"}, "", "")
	if err != nil {
		t.Fatalf("GenerateWithPlan: %v", err)
	}
	if strings.Contains(content, "RULE-SET,private-ip") {
		t.Errorf("rendered config must not contain RULE-SET,private-ip (会引入 198.18.0.0/15)\n%s", content)
	}
	if strings.Contains(content, "198.18.0.0/15") || strings.Contains(content, "198.18.0.0/16") {
		t.Errorf("rendered config must not contain fake-ip range\n%s", content)
	}
	for _, want := range []string{
		"IP-CIDR,10.0.0.0/8,本地直连,no-resolve",
		"IP-CIDR,127.0.0.0/8,本地直连,no-resolve",
		"IP-CIDR,192.168.0.0/16,本地直连,no-resolve",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q\n%s", want, content)
		}
	}
}

// TUN 模式下,Mihomo 必须靠 fake-ip + 反查映射才能让 DOMAIN 规则命中——
// 没有 dns 块的话,fake-ip 反查不工作,流量按 IP 走规则,DOMAIN-SUFFIX
// 永远跳过,常见症状是 claude.ai/claude.com 这类 fake-ip 流量在嗅探来不及
// 时被某条 IP 规则带去 DIRECT。这里 pin 住几条关键字段,防止之后回退。
func TestClashGenerator_DNSBlockEnablesFakeIPForTUN(t *testing.T) {
	content := clashGlobalPreamble
	mustContain := []string{
		"\ndns:",
		"enable: true",
		"enhanced-mode: fake-ip",
		"respect-rules: true",
		"proxy-server-nameserver:",
		"nameserver-policy:",
	}
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("preamble 缺少必要 DNS 字段 %q\npreamble:\n%s", s, content)
		}
	}
}

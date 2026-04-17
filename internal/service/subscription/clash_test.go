package subscription

import (
	"strings"
	"testing"

	"proxy-panel/internal/model"
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
		"network: ws",               // 兼容写法：不是 network: httpupgrade
		"v2ray-http-upgrade: true",  // 关键：告诉 mihomo 走 httpupgrade 语义
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

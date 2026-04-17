package kernel

import (
	"testing"
)

// 回归：历史版本按 u.Protocol == node.Protocol 过滤 inbound.clients，而
// users 表 protocol 字段只能存一个值 → 跨协议节点的 clients 永远是 null → 客户端
// TLS 握手能过但收不到 VMess/Trojan 响应 → 挂死直到超时。
// 修复后，按 user_nodes 关联 (u.NodeIDs) 过滤，同一个用户只要关联多协议节点
// 就必须出现在每个对应 inbound 的 clients 里。
func TestXrayBuildInbound_SingleUserAcrossProtocols(t *testing.T) {
	e := &XrayEngine{}
	user := UserConfig{
		UUID:     "a085accd-889a-4580-89b6-378bd28d4dd5",
		Email:    "alice",
		Protocol: "vless", // 数据库里单值——但关联了 vmess/trojan 节点
		NodeIDs:  []int64{6, 7, 8},
	}
	users := []UserConfig{user}

	cases := []struct {
		name     string
		node     NodeConfig
		wantKey  string // clients[0] 中必含的字段（id / password）
		wantUUID string
	}{
		{
			name:     "vmess inbound 必须含该用户",
			node:     NodeConfig{ID: 6, Tag: "node-6", Port: 8458, Protocol: "vmess"},
			wantKey:  "id",
			wantUUID: user.UUID,
		},
		{
			name:     "trojan inbound 必须含该用户",
			node:     NodeConfig{ID: 7, Tag: "node-7", Port: 6756, Protocol: "trojan"},
			wantKey:  "password",
			wantUUID: user.UUID,
		},
		{
			name:     "vless inbound 也要含该用户（关联匹配）",
			node:     NodeConfig{ID: 8, Tag: "node-8", Port: 443, Protocol: "vless"},
			wantKey:  "id",
			wantUUID: user.UUID,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ib := e.buildInbound(c.node, users)
			settings, ok := ib["settings"].(map[string]interface{})
			if !ok {
				t.Fatalf("settings 缺失: %#v", ib)
			}
			clients, ok := settings["clients"].([]interface{})
			if !ok || len(clients) != 1 {
				t.Fatalf("clients 应只含 1 个用户；got %#v", settings["clients"])
			}
			cli := clients[0].(map[string]interface{})
			if cli[c.wantKey] != c.wantUUID {
				t.Errorf("%s want %q, got %v", c.wantKey, c.wantUUID, cli[c.wantKey])
			}
		})
	}
}

// 未关联的用户不能被注入到该节点 —— 避免 UUID 泄露横穿到用户没权限的节点。
func TestXrayBuildInbound_UnlinkedUserExcluded(t *testing.T) {
	e := &XrayEngine{}
	users := []UserConfig{
		{UUID: "u-linked", Email: "linked", Protocol: "vmess", NodeIDs: []int64{6}},
		{UUID: "u-other", Email: "other", Protocol: "vmess", NodeIDs: []int64{99}},
	}
	node := NodeConfig{ID: 6, Tag: "node-6", Port: 8458, Protocol: "vmess"}

	ib := e.buildInbound(node, users)
	clients := ib["settings"].(map[string]interface{})["clients"].([]interface{})
	if len(clients) != 1 {
		t.Fatalf("应只注入 1 个关联用户；got %d", len(clients))
	}
	if clients[0].(map[string]interface{})["id"] != "u-linked" {
		t.Errorf("注入的不是关联用户: %v", clients[0])
	}
}

// nodeID == 0 的测试桩（不设 ID 的老测试用例）应兜底为"接纳全部用户"，
// 保证已有 singbox/xray 旧测试无需逐个加 NodeIDs 也能继续通过。
func TestUserLinkedToNode_ZeroIDFallback(t *testing.T) {
	u := UserConfig{UUID: "x"} // NodeIDs 空
	if !userLinkedToNode(u, 0) {
		t.Errorf("nodeID==0 应兜底 true")
	}
	if userLinkedToNode(u, 5) {
		t.Errorf("nodeID!=0 且 NodeIDs 空，应为 false")
	}
	u.NodeIDs = []int64{1, 5, 9}
	if !userLinkedToNode(u, 5) {
		t.Errorf("NodeIDs 含 5 应为 true")
	}
	if userLinkedToNode(u, 7) {
		t.Errorf("NodeIDs 不含 7 应为 false")
	}
}

// Hy2 inbound 的 users 列表也必须按关联过滤；否则别的用户会被当成本节点用户。
func TestSingboxHy2BuildInbound_OnlyLinkedUsers(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		ID:       12,
		Tag:      "node-12",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{},
	}
	users := []UserConfig{
		{UUID: "u-linked", Email: "a", Protocol: "hysteria2", NodeIDs: []int64{12}},
		{UUID: "u-other", Email: "b", Protocol: "hysteria2", NodeIDs: []int64{99}},
	}
	ib := e.buildInbound(node, users)
	list := ib["users"].([]map[string]interface{})
	if len(list) != 1 {
		t.Fatalf("hy2 users 列表应只含 1 个关联用户；got %d: %v", len(list), list)
	}
	if list[0]["password"] != "u-linked" {
		t.Errorf("注入的 hy2 user 不是关联用户: %v", list[0])
	}
}

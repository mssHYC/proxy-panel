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

// 兜底语义：
//   - nodeID == 0：测试桩，无节点身份 → true（保持旧测试可跑）
//   - NodeIDs 为空：用户未设置节点白名单 → true（对齐订阅侧 ListByUserID→
//     ListEnabled 的降级语义；否则老部署升级后所有协议都超时）
//   - NodeIDs 非空：精确匹配
func TestUserLinkedToNode_Semantics(t *testing.T) {
	empty := UserConfig{UUID: "x"}
	if !userLinkedToNode(empty, 0) {
		t.Errorf("nodeID==0 应兜底 true")
	}
	if !userLinkedToNode(empty, 5) {
		t.Errorf("NodeIDs 空应兜底 true（对齐订阅降级语义）")
	}

	linked := UserConfig{UUID: "x", NodeIDs: []int64{1, 5, 9}}
	if !userLinkedToNode(linked, 5) {
		t.Errorf("NodeIDs 含 5 应为 true")
	}
	if userLinkedToNode(linked, 7) {
		t.Errorf("NodeIDs 不含 7 应为 false")
	}
}

// 回归：v1.1.15 引入的"NodeIDs 空 → false"导致老部署（user_nodes 表空）升级后
// 所有 inbound 的 clients 全空、所有协议超时，包括 hy2。
// 兜底修正后，NodeIDs 空的用户必须进入所有协议 inbound 的 users 列表。
func TestEmptyNodeIDs_FallbackInjectsIntoAllInbounds(t *testing.T) {
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "vless"}, // NodeIDs 空
	}

	ex := &XrayEngine{}
	xrayNodes := []NodeConfig{
		{ID: 6, Tag: "node-6", Port: 8458, Protocol: "vmess"},
		{ID: 7, Tag: "node-7", Port: 6756, Protocol: "trojan"},
		{ID: 8, Tag: "node-8", Port: 443, Protocol: "vless"},
	}
	for _, n := range xrayNodes {
		ib := ex.buildInbound(n, users)
		clients, _ := ib["settings"].(map[string]interface{})["clients"].([]interface{})
		if len(clients) != 1 {
			t.Errorf("xray %s(%s) clients 应含 1 人（NodeIDs 空兜底），got %d", n.Tag, n.Protocol, len(clients))
		}
	}

	sx := NewSingboxEngine("", "", 0)
	hy2 := NodeConfig{ID: 12, Tag: "node-12", Port: 3468, Protocol: "hysteria2", Settings: map[string]interface{}{}}
	ib := sx.buildInbound(hy2, users)
	list, _ := ib["users"].([]map[string]interface{})
	if len(list) != 1 {
		t.Errorf("hy2 users 列表应含 1 人（NodeIDs 空兜底），got %d", len(list))
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

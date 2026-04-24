package routing

// Plan 是格式无关的分流规划中间表示。
// 由 BuildPlan 生成，交给各 translator 翻译为具体客户端格式。
type Plan struct {
	Groups    []OutboundGroup
	Rules     []Rule
	Providers Providers
	Final     string // 兜底出站：group code 或 DIRECT/REJECT
}

type Rule struct {
	SiteTags      []string
	IPTags        []string
	DomainSuffix  []string
	DomainKeyword []string
	IPCIDR        []string
	SrcIPCIDR     []string
	Protocol      []string
	Port          []string
	Outbound      string // group code 或 'DIRECT'/'REJECT'
}

type OutboundGroup struct {
	Code        string
	DisplayName string
	Type        string   // 'selector' | 'urltest'
	Members     []string // 支持 '<ALL>' 宏
}

type Providers struct {
	Site map[string]ProviderURLs
	IP   map[string]ProviderURLs
}

type ProviderURLs struct {
	Clash   string
	Singbox string
}

// BuildOptions 由 subscription handler 传入。
type BuildOptions struct {
	PresetOverride string // 'minimal'|'balanced'|'comprehensive'|''
	ClientFormat   string // 'clash'|'singbox'|'surge'|'v2ray'|'shadowrocket'
}

// IsLiteralOutbound 判断 outbound 是否为字面量（非 group code）。
func IsLiteralOutbound(s string) bool {
	return s == "DIRECT" || s == "REJECT"
}

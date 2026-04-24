package routing

// SystemGroup 描述一个系统预置出站组的 seed。
type SystemGroup struct {
	Code        string
	DisplayName string
	Type        string
	Members     []string
	SortOrder   int
}

// SystemCategory 描述一个系统预置规则分类的 seed。
type SystemCategory struct {
	Code                string
	DisplayName         string
	SiteTags            []string
	IPTags              []string
	InlineDomainSuffix  []string
	InlineDomainKeyword []string
	InlineIPCIDR        []string
	Protocol            string
	DefaultGroupCode    string // 指向 SystemGroup.Code
	Enabled             bool
	SortOrder           int
}

// SystemPreset 描述一个预设方案的 seed。
type SystemPreset struct {
	Code              string
	DisplayName       string
	EnabledCategories []string
}

// 默认 URL 前缀（可被 settings 覆写）。
const (
	DefaultClashSiteBase   = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/"
	DefaultClashIPBase     = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/"
	DefaultSingboxSiteBase = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/"
	DefaultSingboxIPBase   = "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/"
	DefaultFinalGroup      = "node_select"
)

// SystemGroups 定义 18 个内置出站组。
// <ALL> 宏在 translator 渲染时展开为所有节点名。
var SystemGroups = []SystemGroup{
	{Code: "node_select", DisplayName: "🚀 手动切换", Type: "selector", Members: []string{"auto_select", "<ALL>", "DIRECT"}, SortOrder: 10},
	{Code: "auto_select", DisplayName: "⚡ 自动选择", Type: "urltest", Members: []string{"<ALL>"}, SortOrder: 20},
	{Code: "global_proxy", DisplayName: "🌐 全球代理", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 30},
	{Code: "streaming", DisplayName: "🎬 流媒体", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 40},
	{Code: "telegram", DisplayName: "✈️ Telegram", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 50},
	{Code: "google", DisplayName: "🔍 Google", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 60},
	{Code: "youtube", DisplayName: "📺 YouTube", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 70},
	{Code: "netflix", DisplayName: "🎥 Netflix", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 80},
	{Code: "spotify", DisplayName: "🎵 Spotify", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 90},
	{Code: "hbo", DisplayName: "🎞 HBO", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 100},
	{Code: "bing", DisplayName: "🔎 Bing", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 110},
	{Code: "openai", DisplayName: "🤖 OpenAI", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 120},
	{Code: "claude_ai", DisplayName: "🤖 ClaudeAI", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 130},
	{Code: "disney", DisplayName: "🏰 Disney", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 140},
	{Code: "github", DisplayName: "💻 GitHub", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 150},
	{Code: "cn_media", DisplayName: "🇨🇳 国内媒体", Type: "selector", Members: []string{"DIRECT", "node_select"}, SortOrder: 160},
	{Code: "direct", DisplayName: "🎯 本地直连", Type: "selector", Members: []string{"DIRECT", "node_select"}, SortOrder: 170},
	{Code: "fallback", DisplayName: "🐟 漏网之鱼", Type: "selector", Members: []string{"node_select", "auto_select", "DIRECT"}, SortOrder: 180},
}

// SystemCategories 定义 18 个内置分类。site_tags / ip_tags 与 MetaCubeX/meta-rules-dat 的 geosite/geoip 文件名对齐。
var SystemCategories = []SystemCategory{
	{Code: "private", DisplayName: "局域网", IPTags: []string{"private"}, DefaultGroupCode: "direct", Enabled: true, SortOrder: 10},
	{Code: "location_cn", DisplayName: "Location:CN", SiteTags: []string{"cn"}, IPTags: []string{"cn"}, DefaultGroupCode: "direct", Enabled: true, SortOrder: 20},
	{Code: "ad_block", DisplayName: "广告拦截", SiteTags: []string{"category-ads-all"}, DefaultGroupCode: "fallback", Enabled: false, SortOrder: 30},
	{Code: "ai_services", DisplayName: "AI 服务", SiteTags: []string{"openai", "anthropic", "gemini", "category-ai-chat-!cn"}, DefaultGroupCode: "openai", Enabled: true, SortOrder: 40},
	{Code: "bilibili", DisplayName: "Bilibili", SiteTags: []string{"bilibili"}, DefaultGroupCode: "cn_media", Enabled: false, SortOrder: 50},
	{Code: "youtube", DisplayName: "YouTube", SiteTags: []string{"youtube"}, DefaultGroupCode: "youtube", Enabled: true, SortOrder: 60},
	{Code: "google", DisplayName: "Google", SiteTags: []string{"google"}, IPTags: []string{"google"}, DefaultGroupCode: "google", Enabled: true, SortOrder: 70},
	{Code: "telegram", DisplayName: "Telegram", SiteTags: []string{"telegram"}, IPTags: []string{"telegram"}, DefaultGroupCode: "telegram", Enabled: true, SortOrder: 80},
	{Code: "github", DisplayName: "GitHub", SiteTags: []string{"github"}, DefaultGroupCode: "github", Enabled: true, SortOrder: 90},
	{Code: "microsoft", DisplayName: "Microsoft", SiteTags: []string{"microsoft"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 100},
	{Code: "apple", DisplayName: "Apple", SiteTags: []string{"apple"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 110},
	{Code: "social_media", DisplayName: "社交媒体", SiteTags: []string{"facebook", "twitter", "instagram", "tiktok"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 120},
	{Code: "streaming", DisplayName: "流媒体", SiteTags: []string{"netflix", "disney", "hbo", "spotify"}, DefaultGroupCode: "streaming", Enabled: false, SortOrder: 130},
	{Code: "gaming", DisplayName: "游戏", SiteTags: []string{"category-games-!cn"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 140},
	{Code: "education", DisplayName: "教育", SiteTags: []string{"category-education-!cn"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 150},
	{Code: "financial", DisplayName: "金融", SiteTags: []string{"paypal", "stripe"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 160},
	{Code: "cloud_services", DisplayName: "云服务", SiteTags: []string{"amazon", "aws", "cloudflare"}, DefaultGroupCode: "global_proxy", Enabled: false, SortOrder: 170},
	{Code: "non_china", DisplayName: "Non-China", SiteTags: []string{"geolocation-!cn"}, DefaultGroupCode: "fallback", Enabled: true, SortOrder: 900},
}

// SystemPresets 对应 sublink-worker 的 minimal / balanced / comprehensive。
var SystemPresets = []SystemPreset{
	{Code: "minimal", DisplayName: "最小规则", EnabledCategories: []string{"private", "location_cn", "non_china"}},
	{Code: "balanced", DisplayName: "均衡规则", EnabledCategories: []string{"private", "location_cn", "non_china", "github", "google", "youtube", "ai_services", "telegram"}},
	{Code: "comprehensive", DisplayName: "完整规则", EnabledCategories: allCategoryCodes()},
}

func allCategoryCodes() []string {
	out := make([]string, 0, len(SystemCategories))
	for _, c := range SystemCategories {
		out = append(out, c.Code)
	}
	return out
}

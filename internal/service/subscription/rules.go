package subscription

// 自定义规则，由外部设置
var customRules []string

// 自定义规则模式: "prepend" = 追加到默认规则前, "override" = 完全替换默认规则
var customRulesMode = "prepend"

// SetCustomRules 设置自定义规则
func SetCustomRules(rules []string) {
	customRules = rules
}

// GetCustomRules 获取自定义规则
func GetCustomRules() []string {
	return customRules
}

// SetCustomRulesMode 设置自定义规则模式
func SetCustomRulesMode(mode string) {
	if mode == "override" {
		customRulesMode = "override"
	} else {
		customRulesMode = "prepend"
	}
}

// IsOverrideMode 是否为完全替换模式
func IsOverrideMode() bool {
	return customRulesMode == "override" && len(customRules) > 0
}

// ProxyGroupNames 所有代理组名（跨格式共享）
var ProxyGroupNames = []string{
	"手动切换", "自动选择", "全球代理", "流媒体",
	"Telegram", "Google", "YouTube", "Netflix",
	"Spotify", "HBO", "Bing", "OpenAI", "ClaudeAI",
	"Disney", "GitHub", "国内媒体", "本地直连", "漏网之鱼",
}

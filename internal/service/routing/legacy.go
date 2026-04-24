package routing

import (
	"strings"
)

// LegacyRule 是一条老格式规则的结构化形式。
type LegacyRule struct {
	Type     string // DOMAIN | DOMAIN-SUFFIX | DOMAIN-KEYWORD | IP-CIDR | IP-CIDR6 | GEOSITE | GEOIP | PROCESS-NAME
	Value    string
	Outbound string // 旧格式的中文组名或 DIRECT/REJECT
}

var legacyRuleTypes = map[string]bool{
	"DOMAIN":         true,
	"DOMAIN-SUFFIX":  true,
	"DOMAIN-KEYWORD": true,
	"IP-CIDR":        true,
	"IP-CIDR6":       true,
	"GEOSITE":        true,
	"GEOIP":          true,
	"PROCESS-NAME":   true,
}

// ParseLegacyRules 解析老的多行规则文本。
// 每行格式：TYPE,VALUE,OUTBOUND；#/// 开头或空行忽略；格式错误跳过不报错。
func ParseLegacyRules(text string) ([]LegacyRule, error) {
	var out []LegacyRule
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(parts[0]))
		if !legacyRuleTypes[typ] {
			continue
		}
		out = append(out, LegacyRule{
			Type:     typ,
			Value:    strings.TrimSpace(parts[1]),
			Outbound: strings.TrimSpace(parts[2]),
		})
	}
	return out, nil
}

// legacyOutboundMap 把老的中文组名映射到新的 SystemGroup.Code。
var legacyOutboundMap = map[string]string{
	"手动切换":     "node_select",
	"自动选择":     "auto_select",
	"全球代理":     "global_proxy",
	"流媒体":      "streaming",
	"Telegram": "telegram",
	"Google":   "google",
	"YouTube":  "youtube",
	"Netflix":  "netflix",
	"Spotify":  "spotify",
	"HBO":      "hbo",
	"Bing":     "bing",
	"OpenAI":   "openai",
	"ClaudeAI": "claude_ai",
	"Disney":   "disney",
	"GitHub":   "github",
	"国内媒体":     "cn_media",
	"本地直连":     "direct",
	"漏网之鱼":     "fallback",
}

// MapLegacyOutboundToCode 把老组名映射到新 code。
// DIRECT/REJECT 原样返回；未识别返回空。
func MapLegacyOutboundToCode(name string) string {
	s := strings.TrimSpace(name)
	if s == "DIRECT" || s == "REJECT" {
		return s
	}
	if code, ok := legacyOutboundMap[s]; ok {
		return code
	}
	return ""
}

// ToCustomRuleFields 将 LegacyRule 转换为 custom_rules 表的字段切片。
// 返回 (siteTags, ipTags, domainSuffix, domainKeyword, ipCIDR)。
// 任一切片长度 ≤ 1（单行规则单字段）。
func (r LegacyRule) ToCustomRuleFields() (site, ip, ds, dk, ic []string) {
	switch r.Type {
	case "DOMAIN":
		ds = []string{r.Value}
	case "DOMAIN-SUFFIX":
		ds = []string{r.Value}
	case "DOMAIN-KEYWORD":
		dk = []string{r.Value}
	case "IP-CIDR", "IP-CIDR6":
		ic = []string{r.Value}
	case "GEOSITE":
		site = []string{r.Value}
	case "GEOIP":
		ip = []string{r.Value}
	case "PROCESS-NAME":
		// 当前 custom_rules 无 process 字段，降级为丢弃
	}
	return
}

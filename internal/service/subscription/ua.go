package subscription

import "regexp"

var uaPatterns = []struct {
	re     *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`(?i)surge`), "surge"},
	{regexp.MustCompile(`(?i)shadowrocket`), "shadowrocket"},
	{regexp.MustCompile(`(?i)quantumult`), "shadowrocket"},
	{regexp.MustCompile(`(?i)clash|stash|mihomo`), "clash"},
	{regexp.MustCompile(`(?i)sing-?box`), "singbox"},
	{regexp.MustCompile(`(?i)v2ray|v2box`), "v2ray"},
}

// SniffFormat 依据 User-Agent 识别订阅格式，未识别返回空串。
func SniffFormat(ua string) string {
	for _, p := range uaPatterns {
		if p.re.MatchString(ua) {
			return p.format
		}
	}
	return ""
}

package subscription

import (
	"fmt"
	"strings"

	"proxy-panel/internal/model"
)

// SurgeGenerator Surge 格式订阅生成器
type SurgeGenerator struct{}

func (g *SurgeGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	var b strings.Builder

	// 托管配置头
	b.WriteString(fmt.Sprintf("#!MANAGED-CONFIG %s/api/sub/%s?format=surge interval=86400 strict=false\n\n", baseURL, user.UUID))

	// [General]
	b.WriteString("[General]\n")
	b.WriteString("loglevel = notify\n")
	b.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local\n")
	b.WriteString("dns-server = 223.5.5.5, 119.29.29.29, system\n\n")

	// [Proxy]
	b.WriteString("[Proxy]\n")
	b.WriteString("DIRECT = direct\n")

	var proxyNames []string
	for _, node := range nodes {
		line := g.buildProxyLine(node, user)
		if line != "" {
			b.WriteString(line + "\n")
			proxyNames = append(proxyNames, node.Name)
		}
	}
	b.WriteString("\n")

	allNames := strings.Join(proxyNames, ", ")

	// [Proxy Group]
	b.WriteString("[Proxy Group]\n")
	// 手动切换
	b.WriteString(fmt.Sprintf("手动切换 = select, %s\n", allNames))
	// 自动选择
	b.WriteString(fmt.Sprintf("自动选择 = url-test, %s, url=http://www.gstatic.com/generate_204, interval=300\n", allNames))
	// 全球代理
	b.WriteString(fmt.Sprintf("全球代理 = select, 手动切换, 自动选择, %s\n", allNames))
	// 流媒体
	b.WriteString(fmt.Sprintf("流媒体 = select, 手动切换, 自动选择, DIRECT, %s\n", allNames))
	// Telegram
	b.WriteString(fmt.Sprintf("Telegram = select, 手动切换, 自动选择, %s\n", allNames))
	// Google
	b.WriteString(fmt.Sprintf("Google = select, 手动切换, 自动选择, %s, DIRECT\n", allNames))
	// YouTube
	b.WriteString(fmt.Sprintf("YouTube = select, 手动切换, 自动选择, %s\n", allNames))
	// Netflix
	b.WriteString(fmt.Sprintf("Netflix = select, 流媒体, 手动切换, 自动选择, %s\n", allNames))
	// Spotify
	b.WriteString(fmt.Sprintf("Spotify = select, 流媒体, 手动切换, 自动选择, DIRECT, %s\n", allNames))
	// HBO
	b.WriteString(fmt.Sprintf("HBO = select, 流媒体, 手动切换, 自动选择, %s\n", allNames))
	// Disney
	b.WriteString(fmt.Sprintf("Disney = select, 流媒体, 手动切换, 自动选择, %s\n", allNames))
	// Bing
	b.WriteString(fmt.Sprintf("Bing = select, 手动切换, 自动选择, %s\n", allNames))
	// OpenAI
	b.WriteString(fmt.Sprintf("OpenAI = select, 手动切换, 自动选择, %s\n", allNames))
	// ClaudeAI
	b.WriteString(fmt.Sprintf("ClaudeAI = select, 手动切换, 自动选择, %s\n", allNames))
	// GitHub
	b.WriteString(fmt.Sprintf("GitHub = select, 手动切换, 自动选择, %s, DIRECT\n", allNames))
	// 国内媒体
	b.WriteString(fmt.Sprintf("国内媒体 = select, DIRECT, %s\n", allNames))
	// 本地直连
	b.WriteString(fmt.Sprintf("本地直连 = select, DIRECT, 自动选择, %s\n", allNames))
	// 漏网之鱼
	b.WriteString(fmt.Sprintf("漏网之鱼 = select, DIRECT, 手动切换, 自动选择, %s\n", allNames))
	b.WriteString("\n")

	// [Rule]
	b.WriteString("[Rule]\n")

	if IsOverrideMode() {
		// 完全使用自定义规则
		for _, rule := range GetCustomRules() {
			rule = strings.TrimSpace(rule)
			if rule != "" && !strings.HasPrefix(rule, "#") {
				b.WriteString(rule + "\n")
			}
		}
	} else {
		// 自定义规则优先
		rules := GetCustomRules()
		if len(rules) > 0 {
			b.WriteString("# 自定义规则\n")
			for _, rule := range rules {
				rule = strings.TrimSpace(rule)
				if rule != "" {
					b.WriteString(rule + "\n")
				}
			}
		}
		// 默认规则
		b.WriteString("# 默认规则\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/YouTube/YouTube.list,YouTube\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Google/Google.list,Google\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/GitHub/GitHub.list,GitHub\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Telegram/Telegram.list,Telegram\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Spotify/Spotify.list,Spotify\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Netflix/Netflix.list,Netflix\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/HBO/HBO.list,HBO\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Bing/Bing.list,Bing\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/OpenAI/OpenAI.list,OpenAI\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Claude/Claude.list,ClaudeAI\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Disney/Disney.list,Disney\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/Global/Global_All_No_Resolve.list,全球代理\n")
		b.WriteString("RULE-SET,https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/ChinaMax/ChinaMax_No_Resolve.list,本地直连\n")
		b.WriteString("GEOIP,CN,本地直连\n")
		b.WriteString("FINAL,漏网之鱼\n")
	}

	return b.String(), "text/plain; charset=utf-8", nil
}

func (g *SurgeGenerator) buildProxyLine(node model.Node, user *model.User) string {
	s := parseSettings(node)
	port := fmt.Sprintf("%d", node.Port)

	switch node.Protocol {
	case "vmess":
		parts := []string{
			fmt.Sprintf("%s = vmess, %s, %s, username=%s", node.Name, node.Host, port, user.UUID),
		}
		if s.TLS {
			parts = append(parts, "tls=true")
			if s.SNI != "" {
				parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
			}
		}
		if node.Transport == "ws" {
			parts = append(parts, "ws=true")
			if s.Path != "" {
				parts = append(parts, fmt.Sprintf("ws-path=%s", s.Path))
			}
			if s.Host != "" {
				parts = append(parts, fmt.Sprintf("ws-headers=Host:%s", s.Host))
			}
		}
		return strings.Join(parts, ", ")

	case "trojan":
		parts := []string{
			fmt.Sprintf("%s = trojan, %s, %s, password=%s", node.Name, node.Host, port, user.UUID),
		}
		if s.SNI != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
		}
		return strings.Join(parts, ", ")

	case "ss":
		method := s.Method
		if method == "" {
			method = "aes-256-gcm"
		}
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		return fmt.Sprintf("%s = ss, %s, %s, encrypt-method=%s, password=%s", node.Name, node.Host, port, method, password)

	case "hysteria2":
		password := s.Password
		if password == "" {
			password = user.UUID
		}
		parts := []string{
			fmt.Sprintf("%s = hysteria2, %s, %s, password=%s", node.Name, node.Host, port, password),
		}
		if s.SNI != "" {
			parts = append(parts, fmt.Sprintf("sni=%s", s.SNI))
		}
		return strings.Join(parts, ", ")

	case "vless":
		return fmt.Sprintf("# %s = VLESS (Surge 不原生支持)", node.Name)

	default:
		return ""
	}
}

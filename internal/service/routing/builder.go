package routing

import (
	"context"
	"fmt"
)

// BuildPlan 从 DB 读规范化表 + 应用预设覆盖 → 输出格式无关 Plan。
func BuildPlan(ctx context.Context, db DB, opts BuildOptions) (*Plan, error) {
	groups, err := ListGroups(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	cats, err := ListCategories(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	customs, err := ListCustomRules(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("list custom rules: %w", err)
	}

	enabledOverride := map[string]bool{}
	usingOverride := false
	if opts.PresetOverride != "" {
		preset, err := GetPreset(ctx, db, opts.PresetOverride)
		if err != nil {
			return nil, fmt.Errorf("get preset %q: %w", opts.PresetOverride, err)
		}
		if preset != nil {
			usingOverride = true
			for _, c := range preset.EnabledCategories {
				enabledOverride[c] = true
			}
		}
	}

	plan := &Plan{
		Providers: Providers{
			Site: map[string]ProviderURLs{},
			IP:   map[string]ProviderURLs{},
		},
	}

	for _, g := range groups {
		plan.Groups = append(plan.Groups, OutboundGroup{
			Code: g.Code, DisplayName: g.DisplayName, Type: g.Type, Members: g.Members,
		})
	}

	for _, cr := range customs {
		outbound := cr.OutboundLiteral
		if outbound == "" {
			code, err := ResolveGroupCode(groups, cr.OutboundGroupID)
			if err != nil {
				return nil, fmt.Errorf("custom rule %d: %w", cr.ID, err)
			}
			outbound = code
		}
		if outbound == "" {
			continue
		}
		plan.Rules = append(plan.Rules, Rule{
			SiteTags: cr.SiteTags, IPTags: cr.IPTags,
			DomainSuffix: cr.DomainSuffix, DomainKeyword: cr.DomainKeyword,
			IPCIDR: cr.IPCIDR, SrcIPCIDR: cr.SrcIPCIDR,
			Protocol: splitCSV(cr.Protocol), Port: splitCSV(cr.Port),
			Outbound: outbound,
		})
		collectProviders(plan, cr.SiteTags, cr.IPTags)
	}

	for _, c := range cats {
		enabled := c.Enabled
		if usingOverride {
			enabled = enabledOverride[c.Code]
		}
		if !enabled {
			continue
		}
		outboundCode, err := ResolveGroupCode(groups, c.DefaultGroupID)
		if err != nil {
			return nil, fmt.Errorf("category %s: %w", c.Code, err)
		}
		if outboundCode == "" {
			continue
		}
		plan.Rules = append(plan.Rules, Rule{
			SiteTags: c.SiteTags, IPTags: c.IPTags,
			DomainSuffix: c.InlineDomainSuffix, DomainKeyword: c.InlineDomainKeyword,
			IPCIDR:   c.InlineIPCIDR,
			Protocol: splitCSV(c.Protocol),
			Outbound: outboundCode,
		})
		collectProviders(plan, c.SiteTags, c.IPTags)
	}

	clashSite := GetRoutingSetting(ctx, db, "routing.site_ruleset_base_url.clash", DefaultClashSiteBase)
	clashIP := GetRoutingSetting(ctx, db, "routing.ip_ruleset_base_url.clash", DefaultClashIPBase)
	sbSite := GetRoutingSetting(ctx, db, "routing.site_ruleset_base_url.singbox", DefaultSingboxSiteBase)
	sbIP := GetRoutingSetting(ctx, db, "routing.ip_ruleset_base_url.singbox", DefaultSingboxIPBase)
	for tag := range plan.Providers.Site {
		plan.Providers.Site[tag] = ProviderURLs{
			Clash:   clashSite + tag + ".mrs",
			Singbox: sbSite + tag + ".srs",
		}
	}
	for tag := range plan.Providers.IP {
		plan.Providers.IP[tag] = ProviderURLs{
			Clash:   clashIP + tag + ".mrs",
			Singbox: sbIP + tag + ".srs",
		}
	}

	plan.Final = GetRoutingSetting(ctx, db, "routing.final_outbound", DefaultFinalGroup)

	return plan, nil
}

func collectProviders(plan *Plan, siteTags, ipTags []string) {
	for _, t := range siteTags {
		if _, ok := plan.Providers.Site[t]; !ok {
			plan.Providers.Site[t] = ProviderURLs{}
		}
	}
	for _, t := range ipTags {
		if _, ok := plan.Providers.IP[t]; !ok {
			plan.Providers.IP[t] = ProviderURLs{}
		}
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			v := trimSpace(s[start:i])
			if v != "" {
				out = append(out, v)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

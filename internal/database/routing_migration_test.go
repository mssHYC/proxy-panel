package database_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"proxy-panel/internal/database"
	"proxy-panel/internal/service/routing"
)

var rulesetDefaults = map[string]string{
	"routing.site_ruleset_base_url.clash":   "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/",
	"routing.ip_ruleset_base_url.clash":     "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geoip/",
	"routing.site_ruleset_base_url.singbox": "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/",
	"routing.ip_ruleset_base_url.singbox":   "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/",
}

func TestFreshRoutingRulesetURLs(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for key, want := range rulesetDefaults {
		var got string
		if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{PresetOverride: "comprehensive"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"google-gemini", "category-scholar-!cn"} {
		if _, ok := plan.Providers.Site[tag]; !ok {
			t.Errorf("missing corrected provider %s", tag)
		}
	}
	for _, tag := range []string{"gemini", "category-education-!cn"} {
		if _, ok := plan.Providers.Site[tag]; ok {
			t.Errorf("obsolete provider %s is still generated", tag)
		}
	}
	for kind, providers := range map[string]map[string]routing.ProviderURLs{"site": plan.Providers.Site, "ip": plan.Providers.IP} {
		for tag, urls := range providers {
			for client, got := range map[string]string{"clash": urls.Clash, "singbox": urls.Singbox} {
				ext := map[string]string{"clash": ".mrs", "singbox": ".srs"}[client]
				want := rulesetDefaults["routing."+kind+"_ruleset_base_url."+client] + tag + ext
				if got != want {
					t.Errorf("%s/%s/%s = %q, want %q", client, kind, tag, got, want)
				}
			}
		}
	}
}

func TestRoutingRulesetUpgrade(t *testing.T) {
	for _, mode := range []string{"legacy defaults", "custom prefixes", "empty prefixes"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "panel.db")
			db, err := database.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if db != nil {
					db.Close()
				}
			}()
			wantValues := make(map[string]string)
			for key, current := range rulesetDefaults {
				value, want := "", ""
				switch mode {
				case "legacy defaults":
					value = strings.Replace(current, "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@", "https://ghfast.top/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/", 1)
					want = current
				case "custom prefixes":
					value = "https://rules.example.org/" + key + "/"
					want = value
				}
				wantValues[key] = want
				if _, err := db.Exec(`UPDATE settings SET value = ? WHERE key = ?`, value, key); err != nil {
					t.Fatal(err)
				}
			}
			for code, tags := range map[string]string{"ai_services": `["openai","gemini","category-ai-chat-!cn"]`, "education": `["category-education-!cn"]`} {
				if _, err := db.Exec(`UPDATE rule_categories SET site_tags = ?, enabled = 0, default_group_id = (SELECT id FROM outbound_groups WHERE code = 'direct'), sort_order = 1234 WHERE code = ?`, tags, code); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO custom_rules (name, site_tags, outbound_literal) VALUES ('own-rule', '["gemini"]', 'DIRECT')`); err != nil {
				t.Fatal(err)
			}
			// Reopen twice to exercise the upgrade and its idempotence.
			for attempt := 0; attempt < 2; attempt++ {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				db, err = database.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				for key, want := range wantValues {
					var got string
					if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&got); err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Errorf("%s after reopen %d = %q, want %q", key, attempt, got, want)
					}
				}
				for code, want := range map[string]string{"ai_services": `["openai","google-gemini","category-ai-chat-!cn"]`, "education": `["category-scholar-!cn"]`} {
					var tags, group string
					var enabled, order int
					if err := db.QueryRow(`SELECT c.site_tags, c.enabled, c.sort_order, g.code FROM rule_categories c JOIN outbound_groups g ON g.id = c.default_group_id WHERE c.code = ?`, code).Scan(&tags, &enabled, &order, &group); err != nil {
						t.Fatal(err)
					}
					if tags != want || enabled != 0 || order != 1234 || group != "direct" {
						t.Errorf("%s after reopen %d: tags=%s enabled=%d order=%d group=%s", code, attempt, tags, enabled, order, group)
					}
				}
				var tags string
				if err := db.QueryRow(`SELECT site_tags FROM custom_rules WHERE name = 'own-rule'`).Scan(&tags); err != nil {
					t.Fatal(err)
				}
				if tags != `["gemini"]` {
					t.Errorf("custom rule changed: %s", tags)
				}
			}
		})
	}
}

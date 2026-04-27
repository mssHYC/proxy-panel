package routing_test

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"proxy-panel/internal/service/routing"
)

// setupTestDB creates in-memory SQLite and seeds minimal fixture data.
// Must use the connection form that supports in-memory sharing across statements.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE outbound_groups (id INTEGER PRIMARY KEY, code TEXT UNIQUE, display_name TEXT, type TEXT, members TEXT, kind TEXT, sort_order INT)`,
		`CREATE TABLE rule_categories (id INTEGER PRIMARY KEY, code TEXT UNIQUE, display_name TEXT, kind TEXT,
            site_tags TEXT, ip_tags TEXT, inline_domain_suffix TEXT, inline_domain_keyword TEXT, inline_ip_cidr TEXT,
            protocol TEXT, default_group_id INT, enabled INT, sort_order INT)`,
		`CREATE TABLE custom_rules (id INTEGER PRIMARY KEY, name TEXT, site_tags TEXT, ip_tags TEXT,
            domain_suffix TEXT, domain_keyword TEXT, ip_cidr TEXT, src_ip_cidr TEXT, protocol TEXT, port TEXT,
            outbound_group_id INT, outbound_literal TEXT, sort_order INT)`,
		`CREATE TABLE rule_presets (code TEXT PRIMARY KEY, display_name TEXT, enabled_categories TEXT)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`INSERT INTO outbound_groups VALUES (1, 'node_select', 'Node', 'selector', '["<ALL>","DIRECT"]', 'system', 10)`,
		`INSERT INTO outbound_groups VALUES (2, 'direct', 'Direct', 'selector', '["DIRECT","node_select"]', 'system', 170)`,
		`INSERT INTO rule_categories VALUES (1, 'location_cn', 'CN', 'system', '["cn"]', '["cn"]', '[]', '[]', '[]', '', 2, 1, 10)`,
		`INSERT INTO rule_categories VALUES (2, 'google',      'G',  'system', '["google"]', '[]', '[]', '[]', '[]', '', 1, 0, 20)`,
		`INSERT INTO rule_presets VALUES ('balanced', 'B', '["location_cn","google"]')`,
		`INSERT INTO settings VALUES ('routing.final_outbound', 'node_select')`,
		`INSERT INTO settings VALUES ('routing.site_ruleset_base_url.clash',   'https://ex.com/geosite/')`,
		`INSERT INTO settings VALUES ('routing.ip_ruleset_base_url.clash',     'https://ex.com/geoip/')`,
		`INSERT INTO settings VALUES ('routing.site_ruleset_base_url.singbox', 'https://sb.com/geosite/')`,
		`INSERT INTO settings VALUES ('routing.ip_ruleset_base_url.singbox',   'https://sb.com/geoip/')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v (%s)", err, s)
		}
	}
	return db
}

func TestBuildPlan_EnabledCategoriesOnly(t *testing.T) {
	db := setupTestDB(t)
	plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(plan.Rules))
	}
	if plan.Rules[0].Outbound != "direct" {
		t.Errorf("want outbound=direct, got %s", plan.Rules[0].Outbound)
	}
	if got := plan.Rules[0].SiteTags; len(got) != 1 || got[0] != "cn" {
		t.Errorf("site tags: %+v", got)
	}
	if plan.Final != "node_select" {
		t.Errorf("final=%s", plan.Final)
	}
	if _, ok := plan.Providers.Site["cn"]; !ok {
		t.Error("missing site provider cn")
	}
	if _, ok := plan.Providers.IP["cn"]; !ok {
		t.Error("missing ip provider cn")
	}
}

func TestBuildPlan_PresetOverride(t *testing.T) {
	db := setupTestDB(t)
	plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{PresetOverride: "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rules) != 2 {
		t.Fatalf("want 2 rules, got %d", len(plan.Rules))
	}
}

func TestBuildPlan_CustomRulesFirst(t *testing.T) {
	db := setupTestDB(t)
	_, _ = db.Exec(`INSERT INTO custom_rules VALUES
        (1, 'test', '[]', '[]', '["example.com"]', '[]', '[]', '[]', '', '', NULL, 'REJECT', 0)`)
	plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rules) < 1 || plan.Rules[0].Outbound != "REJECT" {
		t.Fatalf("custom rule not first: %+v", plan.Rules)
	}
	if plan.Rules[0].DomainSuffix[0] != "example.com" {
		t.Error("custom rule fields lost")
	}
}

func TestBuildPlan_PanelHostStripsPort(t *testing.T) {
	db := setupTestDB(t)
	_, _ = db.Exec(`INSERT INTO outbound_groups VALUES (3, 'openai', 'AI', 'selector', '["node_select","DIRECT"]', 'system', 30)`)
	_, _ = db.Exec(`INSERT INTO rule_categories VALUES
        (3, 'ai_services', 'AI', 'system', '["anthropic"]', '[]', '[]', '[]', '[]', '', 3, 1, 5)`)

	plan, err := routing.BuildPlan(context.Background(), db, routing.BuildOptions{PanelHost: "claude.ai:443"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rules) < 2 {
		t.Fatalf("want at least panel + ai rules, got %+v", plan.Rules)
	}
	if got := plan.Rules[0].DomainSuffix; len(got) != 1 || got[0] != "claude.ai" {
		t.Fatalf("panel host should be normalized without port, got %+v", got)
	}
	if plan.Rules[0].Outbound != "DIRECT" {
		t.Fatalf("panel rule outbound=%s", plan.Rules[0].Outbound)
	}
	if plan.Rules[1].Outbound != "openai" {
		t.Fatalf("anthropic rule should still route to proxy group, got %+v", plan.Rules[1])
	}
}

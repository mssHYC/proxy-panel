package routing_test

import (
	"context"
	"errors"
	"testing"

	"proxy-panel/internal/service/routing"
)

func TestCustomRuleInput_Validate(t *testing.T) {
	id := int64(1)
	cases := []struct {
		name string
		in   routing.CustomRuleInput
		err  error
	}{
		{"both", routing.CustomRuleInput{OutboundGroupID: &id, OutboundLiteral: "DIRECT"}, routing.ErrInvalidOutbound},
		{"neither", routing.CustomRuleInput{}, routing.ErrInvalidOutbound},
		{"group only", routing.CustomRuleInput{OutboundGroupID: &id}, nil},
		{"literal only", routing.CustomRuleInput{OutboundLiteral: "DIRECT"}, nil},
	}
	for _, tc := range cases {
		got := tc.in.Validate()
		if !errors.Is(got, tc.err) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.err)
		}
	}
}

func TestDeleteGroup_SystemImmutable(t *testing.T) {
	db := setupTestDB(t) // reused from builder_test.go (same routing_test package)
	if err := routing.DeleteGroup(context.Background(), db, 1); !errors.Is(err, routing.ErrSystemImmutable) {
		t.Errorf("want ErrSystemImmutable, got %v", err)
	}
}

func TestApplyPreset_FlipsEnabled(t *testing.T) {
	db := setupTestDB(t)
	// balanced enables both location_cn and google
	if err := routing.ApplyPreset(context.Background(), db, "balanced"); err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	cats, err := routing.ListCategories(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range cats {
		got[c.Code] = c.Enabled
	}
	if !got["location_cn"] || !got["google"] {
		t.Errorf("expected both enabled after balanced preset, got %+v", got)
	}

	// active_preset setting persisted
	var v string
	_ = db.QueryRowContext(context.Background(), `SELECT value FROM settings WHERE key='routing.active_preset'`).Scan(&v)
	if v != "balanced" {
		t.Errorf("active_preset=%q, want balanced", v)
	}
}

func TestApplyPreset_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := routing.ApplyPreset(context.Background(), db, "nonexistent")
	if !errors.Is(err, routing.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteGroup_Referenced(t *testing.T) {
	db := setupTestDB(t)
	// Create a custom group (id=3), reference it from a new custom rule, then try to delete.
	if _, err := db.ExecContext(context.Background(), `INSERT INTO outbound_groups (id, code, display_name, type, members, kind, sort_order) VALUES (3, 'custom1', 'Custom 1', 'selector', '[]', 'custom', 200)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO custom_rules (name, site_tags, ip_tags, domain_suffix, domain_keyword, ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order) VALUES ('ref', '[]','[]','[]','[]','[]','[]','','', 3, '', 0)`); err != nil {
		t.Fatal(err)
	}
	err := routing.DeleteGroup(context.Background(), db, 3)
	if !errors.Is(err, routing.ErrGroupReferenced) {
		t.Errorf("want ErrGroupReferenced, got %v", err)
	}
}

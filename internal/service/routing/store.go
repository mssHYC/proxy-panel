package routing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// CategoryRow 对应 rule_categories 表一行（业务层结构，JSON 字段已解码）。
type CategoryRow struct {
	ID                  int64
	Code                string
	DisplayName         string
	Kind                string
	SiteTags            []string
	IPTags              []string
	InlineDomainSuffix  []string
	InlineDomainKeyword []string
	InlineIPCIDR        []string
	Protocol            string
	DefaultGroupID      *int64
	Enabled             bool
	SortOrder           int
}

type GroupRow struct {
	ID          int64
	Code        string
	DisplayName string
	Type        string
	Members     []string
	Kind        string
	SortOrder   int
}

type CustomRuleRow struct {
	ID              int64
	Name            string
	SiteTags        []string
	IPTags          []string
	DomainSuffix    []string
	DomainKeyword   []string
	IPCIDR          []string
	SrcIPCIDR       []string
	Protocol        string
	Port            string
	OutboundGroupID *int64
	OutboundLiteral string
	SortOrder       int
}

type PresetRow struct {
	Code              string
	DisplayName       string
	EnabledCategories []string
}

// DB 是 *sql.DB 的最小接口，便于测试。
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func ListCategories(ctx context.Context, db DB) ([]CategoryRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, code, display_name, kind, site_tags, ip_tags,
        inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order
        FROM rule_categories ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryRow
	for rows.Next() {
		var r CategoryRow
		var site, ip, ds, dk, ic string
		var gid sql.NullInt64
		var enabled int
		if err := rows.Scan(&r.ID, &r.Code, &r.DisplayName, &r.Kind, &site, &ip, &ds, &dk, &ic,
			&r.Protocol, &gid, &enabled, &r.SortOrder); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(site), &r.SiteTags)
		_ = json.Unmarshal([]byte(ip), &r.IPTags)
		_ = json.Unmarshal([]byte(ds), &r.InlineDomainSuffix)
		_ = json.Unmarshal([]byte(dk), &r.InlineDomainKeyword)
		_ = json.Unmarshal([]byte(ic), &r.InlineIPCIDR)
		if gid.Valid {
			v := gid.Int64
			r.DefaultGroupID = &v
		}
		r.Enabled = enabled == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func ListGroups(ctx context.Context, db DB) ([]GroupRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, code, display_name, type, members, kind, sort_order
        FROM outbound_groups ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupRow
	for rows.Next() {
		var g GroupRow
		var members string
		if err := rows.Scan(&g.ID, &g.Code, &g.DisplayName, &g.Type, &members, &g.Kind, &g.SortOrder); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(members), &g.Members)
		out = append(out, g)
	}
	return out, rows.Err()
}

func ListCustomRules(ctx context.Context, db DB) ([]CustomRuleRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, site_tags, ip_tags, domain_suffix, domain_keyword,
        ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order
        FROM custom_rules ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomRuleRow
	for rows.Next() {
		var r CustomRuleRow
		var site, ip, ds, dk, ic, sic string
		var gid sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &site, &ip, &ds, &dk, &ic, &sic,
			&r.Protocol, &r.Port, &gid, &r.OutboundLiteral, &r.SortOrder); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(site), &r.SiteTags)
		_ = json.Unmarshal([]byte(ip), &r.IPTags)
		_ = json.Unmarshal([]byte(ds), &r.DomainSuffix)
		_ = json.Unmarshal([]byte(dk), &r.DomainKeyword)
		_ = json.Unmarshal([]byte(ic), &r.IPCIDR)
		_ = json.Unmarshal([]byte(sic), &r.SrcIPCIDR)
		if gid.Valid {
			v := gid.Int64
			r.OutboundGroupID = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func GetPreset(ctx context.Context, db DB, code string) (*PresetRow, error) {
	var p PresetRow
	var ec string
	err := db.QueryRowContext(ctx, `SELECT code, display_name, enabled_categories FROM rule_presets WHERE code = ?`, code).
		Scan(&p.Code, &p.DisplayName, &ec)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(ec), &p.EnabledCategories)
	return &p, nil
}

// GetRoutingSetting 读 settings 表中 routing.* 标量，不存在返回默认值。
func GetRoutingSetting(ctx context.Context, db DB, key, fallback string) string {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil || v == "" {
		return fallback
	}
	return v
}

// ResolveGroupCode 通过 id 找 code（builder 在处理 custom_rules 时需要）。
func ResolveGroupCode(groups []GroupRow, id *int64) (string, error) {
	if id == nil {
		return "", nil
	}
	for _, g := range groups {
		if g.ID == *id {
			return g.Code, nil
		}
	}
	return "", fmt.Errorf("group id %d not found", *id)
}

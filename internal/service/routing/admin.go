package routing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrSystemImmutable = errors.New("system resource is immutable")
	ErrInvalidOutbound = errors.New("custom rule must have exactly one outbound")
	ErrGroupReferenced = errors.New("group still referenced by categories or rules")
)

// FullDB 是有写能力的 DB。
type FullDB interface {
	DB
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ---- Category CRUD ----

type CategoryInput struct {
	Code                string
	DisplayName         string
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

func CreateCategory(ctx context.Context, db FullDB, in CategoryInput) (int64, error) {
	site, _ := json.Marshal(in.SiteTags)
	ip, _ := json.Marshal(in.IPTags)
	ds, _ := json.Marshal(in.InlineDomainSuffix)
	dk, _ := json.Marshal(in.InlineDomainKeyword)
	ic, _ := json.Marshal(in.InlineIPCIDR)
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	res, err := db.ExecContext(ctx, `INSERT INTO rule_categories
        (code, display_name, kind, site_tags, ip_tags, inline_domain_suffix, inline_domain_keyword, inline_ip_cidr, protocol, default_group_id, enabled, sort_order)
        VALUES (?, ?, 'custom', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Code, in.DisplayName, string(site), string(ip), string(ds), string(dk), string(ic),
		in.Protocol, in.DefaultGroupID, enabled, in.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateCategory(ctx context.Context, db FullDB, id int64, in CategoryInput, isSystem bool) error {
	if isSystem {
		enabled := 0
		if in.Enabled {
			enabled = 1
		}
		_, err := db.ExecContext(ctx, `UPDATE rule_categories SET enabled=?, default_group_id=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			enabled, in.DefaultGroupID, in.SortOrder, id)
		return err
	}
	site, _ := json.Marshal(in.SiteTags)
	ip, _ := json.Marshal(in.IPTags)
	ds, _ := json.Marshal(in.InlineDomainSuffix)
	dk, _ := json.Marshal(in.InlineDomainKeyword)
	ic, _ := json.Marshal(in.InlineIPCIDR)
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `UPDATE rule_categories SET
        display_name=?, site_tags=?, ip_tags=?, inline_domain_suffix=?, inline_domain_keyword=?, inline_ip_cidr=?,
        protocol=?, default_group_id=?, enabled=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		in.DisplayName, string(site), string(ip), string(ds), string(dk), string(ic),
		in.Protocol, in.DefaultGroupID, enabled, in.SortOrder, id)
	return err
}

func DeleteCategory(ctx context.Context, db FullDB, id int64) error {
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT kind FROM rule_categories WHERE id=?`, id).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if kind == "system" {
		return ErrSystemImmutable
	}
	_, err := db.ExecContext(ctx, `DELETE FROM rule_categories WHERE id=?`, id)
	return err
}

// ---- Group CRUD ----

type GroupInput struct {
	Code        string
	DisplayName string
	Type        string
	Members     []string
	SortOrder   int
}

func CreateGroup(ctx context.Context, db FullDB, in GroupInput) (int64, error) {
	members, _ := json.Marshal(in.Members)
	res, err := db.ExecContext(ctx, `INSERT INTO outbound_groups (code, display_name, type, members, kind, sort_order)
        VALUES (?, ?, ?, ?, 'custom', ?)`,
		in.Code, in.DisplayName, in.Type, string(members), in.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateGroup(ctx context.Context, db FullDB, id int64, in GroupInput, isSystem bool) error {
	members, _ := json.Marshal(in.Members)
	if isSystem {
		_, err := db.ExecContext(ctx, `UPDATE outbound_groups SET display_name=?, members=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			in.DisplayName, string(members), in.SortOrder, id)
		return err
	}
	_, err := db.ExecContext(ctx, `UPDATE outbound_groups SET code=?, display_name=?, type=?, members=?, sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		in.Code, in.DisplayName, in.Type, string(members), in.SortOrder, id)
	return err
}

func DeleteGroup(ctx context.Context, db FullDB, id int64) error {
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT kind FROM outbound_groups WHERE id=?`, id).Scan(&kind); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if kind == "system" {
		return ErrSystemImmutable
	}

	var catCount, ruleCount int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rule_categories WHERE default_group_id=?`, id).Scan(&catCount)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_rules WHERE outbound_group_id=?`, id).Scan(&ruleCount)
	if catCount > 0 || ruleCount > 0 {
		return fmt.Errorf("%w: %d categories, %d rules", ErrGroupReferenced, catCount, ruleCount)
	}
	_, err := db.ExecContext(ctx, `DELETE FROM outbound_groups WHERE id=?`, id)
	return err
}

// ---- CustomRule CRUD ----

type CustomRuleInput struct {
	Name            string
	SiteTags        []string
	IPTags          []string
	DomainSuffix    []string
	DomainKeyword   []string
	IPCIDR          []string
	SrcIPCIDR       []string
	OutboundGroupID *int64
	OutboundLiteral string
	SortOrder       int
}

func (i CustomRuleInput) Validate() error {
	if (i.OutboundGroupID == nil) == (i.OutboundLiteral == "") {
		return ErrInvalidOutbound
	}
	if i.OutboundLiteral != "" && i.OutboundLiteral != "DIRECT" && i.OutboundLiteral != "REJECT" {
		return ErrInvalidOutbound
	}
	for _, c := range i.IPCIDR {
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("invalid ip_cidr %q: %w", c, err)
		}
	}
	for _, c := range i.SrcIPCIDR {
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("invalid src_ip_cidr %q: %w", c, err)
		}
	}
	return nil
}

func CreateCustomRule(ctx context.Context, db FullDB, in CustomRuleInput) (int64, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	site, _ := json.Marshal(in.SiteTags)
	ip, _ := json.Marshal(in.IPTags)
	ds, _ := json.Marshal(in.DomainSuffix)
	dk, _ := json.Marshal(in.DomainKeyword)
	ic, _ := json.Marshal(in.IPCIDR)
	sic, _ := json.Marshal(in.SrcIPCIDR)
	res, err := db.ExecContext(ctx, `INSERT INTO custom_rules
        (name, site_tags, ip_tags, domain_suffix, domain_keyword, ip_cidr, src_ip_cidr, protocol, port, outbound_group_id, outbound_literal, sort_order)
        VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?)`,
		in.Name, string(site), string(ip), string(ds), string(dk), string(ic), string(sic),
		in.OutboundGroupID, in.OutboundLiteral, in.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateCustomRule(ctx context.Context, db FullDB, id int64, in CustomRuleInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	site, _ := json.Marshal(in.SiteTags)
	ip, _ := json.Marshal(in.IPTags)
	ds, _ := json.Marshal(in.DomainSuffix)
	dk, _ := json.Marshal(in.DomainKeyword)
	ic, _ := json.Marshal(in.IPCIDR)
	sic, _ := json.Marshal(in.SrcIPCIDR)
	_, err := db.ExecContext(ctx, `UPDATE custom_rules SET
        name=?, site_tags=?, ip_tags=?, domain_suffix=?, domain_keyword=?, ip_cidr=?, src_ip_cidr=?,
        protocol='', port='', outbound_group_id=?, outbound_literal=?, sort_order=?, updated_at=CURRENT_TIMESTAMP
        WHERE id=?`,
		in.Name, string(site), string(ip), string(ds), string(dk), string(ic), string(sic),
		in.OutboundGroupID, in.OutboundLiteral, in.SortOrder, id)
	return err
}

func DeleteCustomRule(ctx context.Context, db FullDB, id int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM custom_rules WHERE id=?`, id)
	return err
}

// ---- Apply Preset ----

// ApplyPreset 将预设的 enabled_categories 覆盖到 DB（持久化），并记 active_preset。
func ApplyPreset(ctx context.Context, db FullDB, code string) error {
	preset, err := GetPreset(ctx, db, code)
	if err != nil {
		return err
	}
	if preset == nil {
		return ErrNotFound
	}

	allowed := map[string]bool{}
	for _, c := range preset.EnabledCategories {
		allowed[c] = true
	}
	cats, err := ListCategories(ctx, db)
	if err != nil {
		return err
	}
	for _, c := range cats {
		enabled := 0
		if allowed[c.Code] {
			enabled = 1
		}
		if _, err := db.ExecContext(ctx, `UPDATE rule_categories SET enabled=? WHERE id=?`, enabled, c.ID); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('routing.active_preset', ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, code)
	return err
}

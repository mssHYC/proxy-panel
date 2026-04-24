package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"proxy-panel/internal/database"
	"proxy-panel/internal/service/routing"

	"github.com/gin-gonic/gin"
)

type RoutingHandler struct {
	db *database.DB
}

func NewRoutingHandler(db *database.DB) *RoutingHandler {
	return &RoutingHandler{db: db}
}

// GET /api/routing/config
func (h *RoutingHandler) GetConfig(c *gin.Context) {
	ctx := c.Request.Context()
	cats, err := routing.ListCategories(ctx, h.db)
	if err != nil {
		h.fail(c, err)
		return
	}
	groups, err := routing.ListGroups(ctx, h.db)
	if err != nil {
		h.fail(c, err)
		return
	}
	custom, err := routing.ListCustomRules(ctx, h.db)
	if err != nil {
		h.fail(c, err)
		return
	}

	presets := []routing.PresetRow{}
	rows, err := h.db.QueryContext(ctx, `SELECT code, display_name, enabled_categories FROM rule_presets ORDER BY code`)
	if err != nil {
		h.fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p routing.PresetRow
		var ec string
		rows.Scan(&p.Code, &p.DisplayName, &ec)
		if ec != "" {
			_ = json.Unmarshal([]byte(ec), &p.EnabledCategories)
		}
		presets = append(presets, p)
	}

	settings := map[string]string{}
	for _, k := range []string{
		"routing.site_ruleset_base_url.clash", "routing.ip_ruleset_base_url.clash",
		"routing.site_ruleset_base_url.singbox", "routing.ip_ruleset_base_url.singbox",
		"routing.surge_site_ruleset_base_url",
		"routing.final_outbound", "routing.active_preset",
	} {
		settings[k] = routing.GetRoutingSetting(ctx, h.db, k, "")
	}

	c.JSON(http.StatusOK, gin.H{
		"categories":  cats,
		"groups":      groups,
		"customRules": custom,
		"presets":     presets,
		"settings":    settings,
	})
}

// ---- Categories ----

func (h *RoutingHandler) CreateCategory(c *gin.Context) {
	var in routing.CategoryInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	id, err := routing.CreateCategory(c.Request.Context(), h.db, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateCategory(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in routing.CategoryInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	var kind string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT kind FROM rule_categories WHERE id=?`, id).Scan(&kind); err != nil {
		h.fail(c, err)
		return
	}
	if err := routing.UpdateCategory(c.Request.Context(), h.db, id, in, kind == "system"); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteCategory(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := routing.DeleteCategory(c.Request.Context(), h.db, id); err != nil {
		if errors.Is(err, routing.ErrSystemImmutable) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, routing.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Groups ----

func (h *RoutingHandler) CreateGroup(c *gin.Context) {
	var in routing.GroupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	id, err := routing.CreateGroup(c.Request.Context(), h.db, in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateGroup(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in routing.GroupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	var kind string
	if err := h.db.QueryRowContext(c.Request.Context(), `SELECT kind FROM outbound_groups WHERE id=?`, id).Scan(&kind); err != nil {
		h.fail(c, err)
		return
	}
	if err := routing.UpdateGroup(c.Request.Context(), h.db, id, in, kind == "system"); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteGroup(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	err := routing.DeleteGroup(c.Request.Context(), h.db, id)
	switch {
	case errors.Is(err, routing.ErrSystemImmutable):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, routing.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, routing.ErrGroupReferenced):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err != nil:
		h.fail(c, err)
	default:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ---- CustomRules ----

func (h *RoutingHandler) CreateCustomRule(c *gin.Context) {
	var in routing.CustomRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	id, err := routing.CreateCustomRule(c.Request.Context(), h.db, in)
	if errors.Is(err, routing.ErrInvalidOutbound) {
		h.badReq(c, err)
		return
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *RoutingHandler) UpdateCustomRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in routing.CustomRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		h.badReq(c, err)
		return
	}
	err := routing.UpdateCustomRule(c.Request.Context(), h.db, id, in)
	if errors.Is(err, routing.ErrInvalidOutbound) {
		h.badReq(c, err)
		return
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RoutingHandler) DeleteCustomRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := routing.DeleteCustomRule(c.Request.Context(), h.db, id); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Preset ----

func (h *RoutingHandler) ApplyPreset(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.badReq(c, err)
		return
	}
	if err := routing.ApplyPreset(c.Request.Context(), h.db, body.Code); err != nil {
		if errors.Is(err, routing.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Import Legacy ----

func (h *RoutingHandler) ImportLegacy(c *gin.Context) {
	var body struct {
		Text string `json:"text"`
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.badReq(c, err)
		return
	}
	rules, err := routing.ParseLegacyRules(body.Text)
	if err != nil {
		h.fail(c, err)
		return
	}
	ctx := c.Request.Context()
	groups, _ := routing.ListGroups(ctx, h.db)
	groupIDByCode := map[string]int64{}
	for _, g := range groups {
		groupIDByCode[g.Code] = g.ID
	}
	imported := 0
	for i, r := range rules {
		code := routing.MapLegacyOutboundToCode(r.Outbound)
		var gid *int64
		lit := ""
		switch {
		case code == "DIRECT", code == "REJECT":
			lit = code
		case code != "":
			v := groupIDByCode[code]
			gid = &v
		default:
			v := groupIDByCode["fallback"]
			gid = &v
		}
		site, ip, ds, dk, ic := r.ToCustomRuleFields()
		_, err := routing.CreateCustomRule(ctx, h.db, routing.CustomRuleInput{
			Name:            "import-" + r.Type + "-" + r.Value,
			SiteTags:        site,
			IPTags:          ip,
			DomainSuffix:    ds,
			DomainKeyword:   dk,
			IPCIDR:          ic,
			OutboundGroupID: gid,
			OutboundLiteral: lit,
			SortOrder:       i,
		})
		if err != nil {
			h.fail(c, err)
			return
		}
		imported++
	}
	if body.Mode == "override" {
		h.db.Exec(`UPDATE rule_categories SET enabled = 0 WHERE kind = 'system'`)
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (h *RoutingHandler) fail(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
func (h *RoutingHandler) badReq(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

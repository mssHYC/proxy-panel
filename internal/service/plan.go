package service

import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
)

// PlanService 套餐业务逻辑
type PlanService struct {
	db *database.DB
}

func NewPlanService(db *database.DB) *PlanService {
	return &PlanService{db: db}
}

type PlanReq struct {
	Name         string  `json:"name" binding:"required"`
	TrafficLimit int64   `json:"traffic_limit"`
	DurationDays int     `json:"duration_days"`
	SortOrder    int     `json:"sort_order"`
	Enabled      *bool   `json:"enabled"`
	NodeGroupIDs []int64 `json:"node_group_ids"`
}

// AssignPlanReq 把套餐应用到指定用户。ResetTraffic / SetExpires 都默认为 true，
// 即按 traffic_limit 和 duration_days 重置；显式设为 false 时只改 plan_id。
type AssignPlanReq struct {
	PlanID        *int64 `json:"plan_id"` // nil → 解除套餐
	ResetTraffic  *bool  `json:"reset_traffic"`
	SetExpiresAt  *bool  `json:"set_expires_at"`
}

func (s *PlanService) List() ([]model.Plan, error) {
	rows, err := s.db.Query(`SELECT id, name, traffic_limit, duration_days, sort_order, enabled, created_at, updated_at
		FROM plans ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询套餐失败: %w", err)
	}
	defer rows.Close()
	var plans []model.Plan
	for rows.Next() {
		var p model.Plan
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &p.TrafficLimit, &p.DurationDays, &p.SortOrder, &enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		ids, err := s.getNodeGroupIDs(p.ID)
		if err != nil {
			return nil, err
		}
		p.NodeGroupIDs = ids
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (s *PlanService) GetByID(id int64) (*model.Plan, error) {
	row := s.db.QueryRow(`SELECT id, name, traffic_limit, duration_days, sort_order, enabled, created_at, updated_at
		FROM plans WHERE id = ?`, id)
	var p model.Plan
	var enabled int
	if err := row.Scan(&p.ID, &p.Name, &p.TrafficLimit, &p.DurationDays, &p.SortOrder, &enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.Enabled = enabled == 1
	ids, err := s.getNodeGroupIDs(p.ID)
	if err != nil {
		return nil, err
	}
	p.NodeGroupIDs = ids
	return &p, nil
}

func (s *PlanService) getNodeGroupIDs(planID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT node_group_id FROM plan_node_groups WHERE plan_id = ? ORDER BY node_group_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *PlanService) setNodeGroupIDs(planID int64, ids []int64) error {
	if _, err := s.db.Exec(`DELETE FROM plan_node_groups WHERE plan_id = ?`, planID); err != nil {
		return err
	}
	for _, gid := range ids {
		if _, err := s.db.Exec(`INSERT INTO plan_node_groups (plan_id, node_group_id) VALUES (?, ?)`, planID, gid); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlanService) Create(req *PlanReq) (*model.Plan, error) {
	now := time.Now()
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	res, err := s.db.Exec(`INSERT INTO plans (name, traffic_limit, duration_days, sort_order, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, req.Name, req.TrafficLimit, req.DurationDays, req.SortOrder, enabled, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建套餐失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.setNodeGroupIDs(id, req.NodeGroupIDs); err != nil {
		return nil, fmt.Errorf("保存套餐节点组关联失败: %w", err)
	}
	return s.GetByID(id)
}

func (s *PlanService) Update(id int64, req *PlanReq) (*model.Plan, error) {
	p, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	if _, err := s.db.Exec(`UPDATE plans SET name=?, traffic_limit=?, duration_days=?, sort_order=?, enabled=?, updated_at=?
		WHERE id=?`, req.Name, req.TrafficLimit, req.DurationDays, req.SortOrder, enabled, time.Now(), id); err != nil {
		return nil, fmt.Errorf("更新套餐失败: %w", err)
	}
	if err := s.setNodeGroupIDs(id, req.NodeGroupIDs); err != nil {
		return nil, fmt.Errorf("更新套餐节点组关联失败: %w", err)
	}
	return s.GetByID(id)
}

// Delete 删除套餐。先把所有引用该套餐的 users.plan_id 置为 NULL，
// 再删除套餐主表（事务内完成），避免出现"套餐被删但 users.plan_id 悬空"——
// 配合订阅 fallback 的修复，否则被删套餐用户可能重新落入"全部节点"。
func (s *PlanService) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("删除套餐开启事务失败: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE users SET plan_id = NULL, updated_at = ? WHERE plan_id = ?`,
		time.Now(), id); err != nil {
		return fmt.Errorf("解除用户套餐绑定失败: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM plans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除套餐失败: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("套餐不存在")
	}
	return tx.Commit()
}

// AssignToUser 把套餐应用到指定用户。
// - PlanID == nil 表示解除套餐：仅清空 users.plan_id，不动 traffic/expires。
// - 否则按套餐字段重置（默认重置流量并设置 expires_at = now + duration_days）。
func (s *PlanService) AssignToUser(userID int64, req *AssignPlanReq) error {
	if req.PlanID == nil {
		_, err := s.db.Exec(`UPDATE users SET plan_id = NULL, updated_at = ? WHERE id = ?`, time.Now(), userID)
		return err
	}
	plan, err := s.GetByID(*req.PlanID)
	if err != nil {
		return err
	}
	if plan == nil {
		return fmt.Errorf("套餐不存在")
	}
	resetTraffic := true
	setExpires := true
	if req.ResetTraffic != nil {
		resetTraffic = *req.ResetTraffic
	}
	if req.SetExpiresAt != nil {
		setExpires = *req.SetExpiresAt
	}

	now := time.Now()
	sets := []string{"plan_id = ?", "updated_at = ?"}
	args := []interface{}{plan.ID, now}
	if resetTraffic {
		sets = append(sets, "traffic_limit = ?", "traffic_used = 0", "traffic_up = 0", "traffic_down = 0", "warn_sent = 0")
		args = append(args, plan.TrafficLimit)
	}
	if setExpires {
		// duration_days = 0 语义为"不限期"：清空 expires_at，并把可能因到期被禁用的用户重新启用。
		// duration_days > 0 时按 now + N 天设置过期。
		if plan.DurationDays > 0 {
			sets = append(sets, "expires_at = ?", "enable = 1")
			args = append(args, now.AddDate(0, 0, plan.DurationDays))
		} else {
			sets = append(sets, "expires_at = NULL", "enable = 1")
		}
	}
	args = append(args, userID)

	q := "UPDATE users SET "
	for i, ss := range sets {
		if i > 0 {
			q += ", "
		}
		q += ss
	}
	q += " WHERE id = ?"

	res, err := s.db.Exec(q, args...)
	if err != nil {
		return fmt.Errorf("分配套餐失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

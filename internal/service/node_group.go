package service

import (
	"database/sql"
	"fmt"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
)

// NodeGroupService 节点分组业务逻辑
type NodeGroupService struct {
	db *database.DB
}

func NewNodeGroupService(db *database.DB) *NodeGroupService {
	return &NodeGroupService{db: db}
}

type NodeGroupReq struct {
	Name      string  `json:"name" binding:"required"`
	SortOrder int     `json:"sort_order"`
	NodeIDs   []int64 `json:"node_ids"`
}

func (s *NodeGroupService) List() ([]model.NodeGroup, error) {
	rows, err := s.db.Query(`SELECT id, name, sort_order, created_at, updated_at
		FROM node_groups ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询节点分组失败: %w", err)
	}
	defer rows.Close()
	var groups []model.NodeGroup
	for rows.Next() {
		var g model.NodeGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		ids, err := s.getNodeIDs(g.ID)
		if err != nil {
			return nil, err
		}
		g.NodeIDs = ids
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *NodeGroupService) GetByID(id int64) (*model.NodeGroup, error) {
	row := s.db.QueryRow(`SELECT id, name, sort_order, created_at, updated_at
		FROM node_groups WHERE id = ?`, id)
	var g model.NodeGroup
	if err := row.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ids, err := s.getNodeIDs(g.ID)
	if err != nil {
		return nil, err
	}
	g.NodeIDs = ids
	return &g, nil
}

func (s *NodeGroupService) getNodeIDs(groupID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT node_id FROM node_group_members WHERE node_group_id = ? ORDER BY node_id`, groupID)
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

func (s *NodeGroupService) setNodeIDs(groupID int64, nodeIDs []int64) error {
	if _, err := s.db.Exec(`DELETE FROM node_group_members WHERE node_group_id = ?`, groupID); err != nil {
		return err
	}
	for _, nid := range nodeIDs {
		if _, err := s.db.Exec(`INSERT INTO node_group_members (node_group_id, node_id) VALUES (?, ?)`, groupID, nid); err != nil {
			return err
		}
	}
	return nil
}

func (s *NodeGroupService) Create(req *NodeGroupReq) (*model.NodeGroup, error) {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO node_groups (name, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, req.Name, req.SortOrder, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建节点分组失败: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := s.setNodeIDs(id, req.NodeIDs); err != nil {
		return nil, fmt.Errorf("保存节点关联失败: %w", err)
	}
	return s.GetByID(id)
}

func (s *NodeGroupService) Update(id int64, req *NodeGroupReq) (*model.NodeGroup, error) {
	g, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, nil
	}
	if _, err := s.db.Exec(`UPDATE node_groups SET name = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		req.Name, req.SortOrder, time.Now(), id); err != nil {
		return nil, fmt.Errorf("更新节点分组失败: %w", err)
	}
	if err := s.setNodeIDs(id, req.NodeIDs); err != nil {
		return nil, fmt.Errorf("更新节点关联失败: %w", err)
	}
	return s.GetByID(id)
}

func (s *NodeGroupService) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM node_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除节点分组失败: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("节点分组不存在")
	}
	return nil
}

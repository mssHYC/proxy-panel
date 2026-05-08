package model

import "time"

// NodeGroup 节点分组：把一组节点打包，便于通过套餐统一授权访问。
type NodeGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	NodeIDs   []int64   `json:"node_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Plan 套餐：流量上限 + 有效期 + 关联的节点分组集合。
// 当用户被分配套餐后，该用户的可见节点 = user_nodes 直接关联 ∪ plan 关联节点分组下的节点。
type Plan struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	TrafficLimit int64     `json:"traffic_limit"`
	DurationDays int       `json:"duration_days"`
	SortOrder    int       `json:"sort_order"`
	Enabled      bool      `json:"enabled"`
	NodeGroupIDs []int64   `json:"node_group_ids"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

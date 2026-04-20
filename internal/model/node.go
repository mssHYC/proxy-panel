package model

import "time"

// Node 节点模型
type Node struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Protocol     string     `json:"protocol"`
	Transport    string     `json:"transport"`
	KernelType   string     `json:"kernel_type"`
	Settings     string     `json:"settings"`
	Enable       bool       `json:"enable"`
	SortOrder    int        `json:"sort_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastCheckAt  *time.Time `json:"last_check_at,omitempty"`
	LastCheckOK  bool       `json:"last_check_ok"`
	LastCheckErr string     `json:"last_check_err,omitempty"`
	FailCount    int        `json:"fail_count"`
}

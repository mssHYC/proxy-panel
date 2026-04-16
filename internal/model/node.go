package model

import "time"

// Node 节点模型
type Node struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Protocol   string    `json:"protocol"`
	Transport  string    `json:"transport"`
	KernelType string    `json:"kernel_type"`
	Settings   string    `json:"settings"`
	Enable     bool      `json:"enable"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

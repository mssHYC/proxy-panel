package model

import (
	"encoding/json"
	"time"
)

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

// PublicPort is the client-facing port; Port remains the core listener port.
func (n Node) PublicPort() int {
	if n.Protocol == "vmess" && (n.Transport == "ws" || n.Transport == "httpupgrade") {
		var s struct {
			PublicPort int    `json:"public_port"`
			Security   string `json:"security"`
			TLS        bool   `json:"tls"`
			Listen     string `json:"listen"`
		}
		if json.Unmarshal([]byte(n.Settings), &s) == nil && s.PublicPort == 443 &&
			(s.Security == "tls" || (s.Security == "" && s.TLS)) && s.Listen == "127.0.0.1" {
			return s.PublicPort
		}
	}
	return n.Port
}

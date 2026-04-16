package model

import "time"

// TrafficLog 流量日志模型
type TrafficLog struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	NodeID    int64     `json:"node_id"`
	Upload    int64     `json:"upload"`
	Download  int64     `json:"download"`
	Timestamp time.Time `json:"timestamp"`
}

// ServerTraffic 服务器全局流量模型（单行表）
type ServerTraffic struct {
	ID         int64     `json:"id"`
	TotalUp    int64     `json:"total_up"`
	TotalDown  int64     `json:"total_down"`
	LimitBytes int64     `json:"limit_bytes"`
	WarnSent   bool      `json:"warn_sent"`
	LimitSent  bool      `json:"limit_sent"`
	ResetAt    time.Time `json:"reset_at"`
}

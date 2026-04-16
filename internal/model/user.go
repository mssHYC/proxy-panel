package model

import "time"

// User 用户模型
type User struct {
	ID           int64      `json:"id"`
	UUID         string     `json:"uuid"`
	Username     string     `json:"username"`
	Password     string     `json:"-"`
	Email        string     `json:"email"`
	Protocol     string     `json:"protocol"`
	TrafficLimit int64      `json:"traffic_limit"`
	TrafficUsed  int64      `json:"traffic_used"`
	TrafficUp    int64      `json:"traffic_up"`
	TrafficDown  int64      `json:"traffic_down"`
	SpeedLimit   int64      `json:"speed_limit"`
	ResetDay     int        `json:"reset_day"`
	ResetCron    string     `json:"reset_cron"`
	Enable       bool       `json:"enable"`
	ExpiresAt    *time.Time `json:"expires_at"`
	WarnSent     bool       `json:"warn_sent"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	NodeIDs      []int64    `json:"node_ids"`
}

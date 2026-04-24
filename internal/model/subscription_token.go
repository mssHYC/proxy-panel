package model

import "time"

// SubscriptionToken 订阅凭证，与 User 解耦；一个用户可有多个。
type SubscriptionToken struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Name           string     `json:"name"`
	Token          string     `json:"token"`
	Enabled        bool       `json:"enabled"`
	ExpiresAt      *time.Time `json:"expires_at"`
	IPBindEnabled  bool       `json:"ip_bind_enabled"`
	BoundIP        string     `json:"bound_ip"`
	LastIP         string     `json:"last_ip"`
	LastUA         string     `json:"last_ua"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	UseCount       int64      `json:"use_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

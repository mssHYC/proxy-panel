package model

import "time"

// AlertRecord 告警记录模型
type AlertRecord struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

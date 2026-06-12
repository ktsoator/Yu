package session

import "time"

type Session struct {
	ID        string         `json:"id"`
	AppName   string         `json:"app_name"`
	UserID    string         `json:"user_id"`
	Events    []Event        `json:"events"`
	State     map[string]any `json:"state,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

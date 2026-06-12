package session

import "time"

type EventType string

const (
	EventMessageCreated EventType = "message.created"
	EventError          EventType = "error"
)

type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id"`
	Message   *Message  `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

package session

import "time"

type EventType string

const (
	// EventMessage is a finished user or assistant message. Assistant messages
	// that request tools carry them in Message.ToolCalls.
	EventMessage EventType = "message"
	// EventToolResult is the output of one executed tool call.
	EventToolResult EventType = "tool.result"
	// EventError records a failed invocation step.
	EventError EventType = "error"

	// EventContentDelta, EventReasoningDelta, EventToolCall, and
	// EventToolApproval are streaming/UI fragments of an assistant turn. They
	// are always Partial and never persisted.
	EventContentDelta   EventType = "content.delta"
	EventReasoningDelta EventType = "reasoning.delta"
	EventToolCall       EventType = "tool.call"
	EventToolApproval   EventType = "tool.approval"
)

// Event is the unit of session history: everything that happens during an
// invocation — messages, tool results, errors — is an appended event.
type Event struct {
	ID           string    `json:"id"`
	InvocationID string    `json:"invocation_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	Type         EventType `json:"type"`
	Author       string    `json:"author,omitempty"` // "user", agent name, or tool name
	Message      Message   `json:"message"`
	Error        string    `json:"error,omitempty"`
	// Partial marks streaming fragments that flow to renderers but are never
	// appended to session history.
	Partial   bool      `json:"partial,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

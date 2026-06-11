package llm

import (
	"context"
	"errors"
)

type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
)

// ToolCall is a tool invocation requested by the model. Arguments is the raw
// JSON arguments object as produced by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef describes a tool to the model: its name, purpose, and a JSON Schema
// for the arguments object the model should produce.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Message struct {
	Role      Role   `json:"role"`
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"` // chain-of-thought (reasoning_content); empty when thinking is off
	// ToolCalls is set on assistant messages that request tool execution.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a Tool-role result message back to the originating call.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// EventType identifies one streamed event emitted during a model turn.
type EventType string

const (
	EventContentDelta   EventType = "content_delta"
	EventReasoningDelta EventType = "reasoning_delta"
	EventToolCall       EventType = "tool_call"
)

// Event is one incremental model/agent event for the UI to render.
type Event struct {
	Type     EventType
	Text     string
	ToolName string
	ToolArgs string
}

// ErrEventStreamStopped is returned internally when an event consumer stops
// iteration early.
var ErrEventStreamStopped = errors.New("event stream stopped")

type Model interface {
	Name() string
	// Chat starts a conversation. tools advertises callable tools (may be empty);
	// onEvent is called per event during streaming (may be nil). Returning false
	// stops iteration. The return value is the fully assembled message for the
	// caller to store in history.
	Chat(ctx context.Context, messages []Message, tools []ToolDef, onEvent func(Event) bool) (Message, error)
}

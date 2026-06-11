package llm

import "context"

type Role string

const (
	System    Role = "system"
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
)

type Message struct {
	Role      Role
	Content   string
	Reasoning string // chain-of-thought (reasoning_content); empty when thinking is off
	// Phase 1 will add: ToolCalls []ToolCall; ToolCallID string
}

// Chunk is one incremental piece of streamed output.
type Chunk struct {
	Content   string
	Reasoning string
}

type Model interface {
	Name() string
	// Chat starts a conversation. onChunk is called per delta during streaming (may be nil);
	// the return value is the fully assembled message for the caller to store in history.
	Chat(ctx context.Context, messages []Message, onChunk func(Chunk)) (Message, error)
}

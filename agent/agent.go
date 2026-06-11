package agent

import (
	"context"

	"github.com/ktsoator/yu/llm"
)

type Config struct {
	Name        string
	Model       llm.Model
	Description string
	Instruction string
}

type Agent interface {
	Send(ctx context.Context, userInput string, onChunk func(llm.Chunk)) (llm.Message, error)
	SetModel(m llm.Model)
	SetThinking(on bool)
	Thinking() bool
	SupportsThinking() bool
}

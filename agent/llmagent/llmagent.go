package llmagent

import (
	"context"
	"fmt"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
)

type thinkingModel interface {
	SetThinking(on bool)
	Thinking() bool
	SupportsThinking() bool
}

type llmAgent struct {
	name        string
	description string
	model       llm.Model
	messages    []llm.Message
}

func New(cfg agent.Config) (agent.Agent, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	if cfg.Instruction == "" {
		return nil, fmt.Errorf("agent instruction is required")
	}
	return &llmAgent{
		name:        cfg.Name,
		description: cfg.Description,
		model:       cfg.Model,
		// Keep the system instruction as the first message in every request.
		messages: []llm.Message{{Role: llm.System, Content: cfg.Instruction}},
	}, nil
}

func (a *llmAgent) Send(ctx context.Context, userInput string, onChunk func(llm.Chunk)) (llm.Message, error) {
	// Add the user turn before calling the model so it sees the full history.
	// On provider errors, roll this append back and leave the conversation clean.
	a.messages = append(a.messages, llm.Message{Role: llm.User, Content: userInput})

	reply, err := a.model.Chat(ctx, a.messages, onChunk)
	if err != nil {
		a.messages = a.messages[:len(a.messages)-1]
		return llm.Message{}, err
	}
	a.messages = append(a.messages, reply)

	return reply, nil
}

// SetModel swaps the underlying model; conversation history is preserved.
func (a *llmAgent) SetModel(m llm.Model) {
	a.model = m
}

func (a *llmAgent) SetThinking(on bool) {
	if model, ok := a.model.(thinkingModel); ok {
		model.SetThinking(on)
	}
}

func (a *llmAgent) Thinking() bool {
	model, ok := a.model.(thinkingModel)
	return ok && model.Thinking()
}

func (a *llmAgent) SupportsThinking() bool {
	model, ok := a.model.(thinkingModel)
	return ok && model.SupportsThinking()
}

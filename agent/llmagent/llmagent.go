package llmagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/tool"
)

// maxToolIterations bounds the model→tool→model loop so a misbehaving model
// can't spin forever requesting tools.
const maxToolIterations = 60

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
	registry    tool.Registry
	toolDefs    []llm.ToolDef
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
		registry: tool.NewRegistry(cfg.Tools),
		toolDefs: toolDefs(cfg.Tools),
	}, nil
}

func (a *llmAgent) Run(ctx context.Context, userInput string) iter.Seq2[llm.Event, error] {
	return func(yield func(llm.Event, error) bool) {
		_, err := a.runTurn(ctx, userInput, func(ev llm.Event) bool {
			return yield(ev, nil)
		})
		if err != nil && !errors.Is(err, llm.ErrEventStreamStopped) {
			yield(llm.Event{}, err)
		}
	}
}

// runTurn handles one user input. It may call the model multiple times when the
// model asks for tools: model -> tools -> model -> final answer.
func (a *llmAgent) runTurn(ctx context.Context, userInput string, onEvent func(llm.Event) bool) (llm.Message, error) {
	// Remember where this turn started so any provider error rolls the whole
	// turn back (user message + any tool round-trips) and leaves history clean.
	start := len(a.messages)
	a.messages = append(a.messages, llm.Message{Role: llm.User, Content: userInput})

	for range maxToolIterations {
		reply, err := a.model.Chat(ctx, a.messages, a.toolDefs, onEvent)
		if err != nil {
			a.messages = a.messages[:start]
			return llm.Message{}, err
		}
		a.messages = append(a.messages, reply)

		if len(reply.ToolCalls) == 0 {
			return reply, nil
		}

		// Run each requested tool and feed the results back as Tool messages,
		// then loop so the model can use them to produce its next turn.
		if err := a.runTools(ctx, reply.ToolCalls, onEvent); err != nil {
			a.messages = a.messages[:start]
			return llm.Message{}, err
		}
	}

	return llm.Message{}, fmt.Errorf("tool loop exceeded %d iterations", maxToolIterations)
}

// runTools executes the model's requested tools and appends their results to
// conversation history as tool messages.
func (a *llmAgent) runTools(ctx context.Context, calls []llm.ToolCall, onEvent func(llm.Event) bool) error {
	for _, tc := range calls {
		result, err := a.runTool(ctx, tc, onEvent)
		if err != nil {
			return err
		}
		a.messages = append(a.messages, llm.Message{
			Role:       llm.Tool,
			ToolCallID: tc.ID,
			Content:    result,
		})
	}
	return nil
}

// runTool executes a single tool call, surfacing activity via onEvent. Tool
// errors are returned as text so the model can see and recover from them.
func (a *llmAgent) runTool(ctx context.Context, tc llm.ToolCall, onEvent func(llm.Event) bool) (string, error) {
	if !emitToolEvent(onEvent, tc) {
		return "", llm.ErrEventStreamStopped
	}
	t, ok := a.registry[tc.Name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", tc.Name), nil
	}
	result, err := t.Execute(ctx, json.RawMessage(tc.Arguments))
	if err != nil {
		result = "error: " + err.Error()
	}
	return result, nil
}

func emitToolEvent(onEvent func(llm.Event) bool, tc llm.ToolCall) bool {
	if onEvent == nil {
		return true
	}
	return onEvent(llm.Event{
		Type:     llm.EventToolCall,
		ToolName: tc.Name,
		ToolArgs: summarizeArgs(tc.Arguments),
	})
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

func toolDefs(tools []tool.Tool) []llm.ToolDef {
	if len(tools) == 0 {
		return nil
	}
	defs := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return defs
}

// summarizeArgs renders tool arguments compactly for the activity note,
// preferring a single "path" value when present.
func summarizeArgs(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return ""
	}
	if p, ok := m["path"].(string); ok {
		return p
	}
	return args
}

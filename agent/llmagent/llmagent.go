package llmagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
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
	instruction string
	model       llm.Model
	registry    *tool.Registry
	toolDefs    []llm.ToolDef
	approve     agent.ToolApprover
	env         agent.Environment
}

func New(cfg agent.Config) (agent.Agent, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	if cfg.Instruction == "" {
		return nil, fmt.Errorf("agent instruction is required")
	}
	name := cfg.Name
	if name == "" {
		name = "yu"
	}
	registry, err := tool.NewRegistry(cfg.Tools)
	if err != nil {
		return nil, err
	}
	return &llmAgent{
		name:        name,
		description: cfg.Description,
		instruction: cfg.Instruction,
		model:       cfg.Model,
		registry:    registry,
		toolDefs:    toolDefs(cfg.Tools),
		approve:     cfg.Approve,
		env:         cfg.Environment,
	}, nil
}

func (a *llmAgent) Name() string { return a.name }

func (a *llmAgent) Run(ctx context.Context, ictx *agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		err := a.run(ctx, ictx, func(ev *session.Event) bool {
			return yield(ev, nil)
		})
		if err != nil && !errors.Is(err, llm.ErrEventStreamStopped) {
			yield(nil, err)
		}
	}
}

// run drives one invocation. The session snapshot is converted to model
// messages once up front; the loop then advances a local message list so the
// runner's concurrent persistence never feeds back into this turn.
func (a *llmAgent) run(ctx context.Context, ictx *agent.InvocationContext, emit func(*session.Event) bool) error {
	if ictx == nil || ictx.Session == nil {
		return fmt.Errorf("invocation context with session is required")
	}
	messages := toLLMMessages(a.systemPrompt(), ictx.Session.Events)

	for range maxToolIterations {
		reply, err := a.model.Chat(ctx, messages, a.toolDefs, func(ev llm.Event) bool {
			return emit(a.partialEvent(ictx, ev))
		})
		if err != nil {
			return err
		}
		messages = append(messages, reply)
		if !emit(a.messageEvent(ictx, reply)) {
			return llm.ErrEventStreamStopped
		}

		if len(reply.ToolCalls) == 0 {
			return nil
		}

		// Run each requested tool and feed the results back as Tool messages,
		// then loop so the model can use them to produce its next turn.
		for _, tc := range reply.ToolCalls {
			result := a.execTool(ctx, ictx, tc)
			messages = append(messages, llm.Message{
				Role:       llm.Tool,
				Content:    result,
				ToolCallID: tc.ID,
			})
			if !emit(a.toolResultEvent(ictx, tc, result)) {
				return llm.ErrEventStreamStopped
			}
		}
	}

	return fmt.Errorf("tool loop exceeded %d iterations", maxToolIterations)
}

// systemPrompt composes the static instruction with the dynamic environment
// block, evaluated once per invocation. The working directory matches the one
// tools see, so the model and its tools agree on where they are.
func (a *llmAgent) systemPrompt() string {
	if a.env == nil {
		return a.instruction
	}
	wd, _ := os.Getwd()
	block := a.env(wd)
	if block == "" {
		return a.instruction
	}
	return a.instruction + "\n\n" + block
}

// execTool executes a single tool call. Tool errors are returned as text so
// the model can see and recover from them. Non-read-only tools are gated by the
// approver first, and a rejection is likewise returned as text so the model can
// adapt rather than the whole turn aborting.
func (a *llmAgent) execTool(ctx context.Context, ictx *agent.InvocationContext, tc llm.ToolCall) string {
	t, ok := a.registry.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", tc.Name)
	}
	if !t.ReadOnly() && a.approve != nil {
		ok, err := a.approve(t, tc.Arguments)
		if err != nil {
			return "error: " + err.Error()
		}
		if !ok {
			return "error: user rejected this tool call"
		}
	}
	toolCtx := toolContext(ctx, ictx)
	result, err := t.Execute(toolCtx, json.RawMessage(tc.Arguments))
	if err != nil {
		return "error: " + err.Error()
	}
	return result.Text()
}

func toolContext(ctx context.Context, ictx *agent.InvocationContext) tool.Context {
	wd, _ := os.Getwd()
	return tool.Context{
		Context:      ctx,
		AppName:      ictx.AppName,
		UserID:       ictx.UserID,
		SessionID:    ictx.Session.ID,
		InvocationID: ictx.InvocationID,
		WorkDir:      wd,
	}
}

func (a *llmAgent) partialEvent(ictx *agent.InvocationContext, ev llm.Event) *session.Event {
	out := &session.Event{
		InvocationID: ictx.InvocationID,
		SessionID:    ictx.Session.ID,
		Author:       a.name,
		Partial:      true,
	}
	switch ev.Type {
	case llm.EventReasoningDelta:
		out.Type = session.EventReasoningDelta
		out.Message = session.Message{Role: session.RoleAssistant, Reasoning: ev.Text}
	default:
		out.Type = session.EventContentDelta
		out.Message = session.Message{Role: session.RoleAssistant, Content: ev.Text}
	}
	return out
}

func (a *llmAgent) messageEvent(ictx *agent.InvocationContext, reply llm.Message) *session.Event {
	return &session.Event{
		InvocationID: ictx.InvocationID,
		SessionID:    ictx.Session.ID,
		Type:         session.EventMessage,
		Author:       a.name,
		Message:      toSessionMessage(reply),
	}
}

func (a *llmAgent) toolResultEvent(ictx *agent.InvocationContext, tc llm.ToolCall, result string) *session.Event {
	return &session.Event{
		InvocationID: ictx.InvocationID,
		SessionID:    ictx.Session.ID,
		Type:         session.EventToolResult,
		Author:       tc.Name,
		Message: session.Message{
			Role:       session.RoleTool,
			Name:       tc.Name,
			ToolCallID: tc.ID,
			Content:    result,
		},
	}
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

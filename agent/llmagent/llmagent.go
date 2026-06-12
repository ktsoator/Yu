package llmagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

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
	appName     string
	instruction string
	userID      string
	model       llm.Model
	sessions    session.Service
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
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = session.NewInMemoryService()
	}
	appName := cfg.AppName
	if appName == "" {
		appName = cfg.Name
	}
	if appName == "" {
		appName = "yu"
	}
	userID := cfg.UserID
	if userID == "" {
		userID = "local"
	}
	return &llmAgent{
		name:        cfg.Name,
		description: cfg.Description,
		appName:     appName,
		instruction: cfg.Instruction,
		userID:      userID,
		model:       cfg.Model,
		sessions:    sessions,
		registry:    tool.NewRegistry(cfg.Tools),
		toolDefs:    toolDefs(cfg.Tools),
	}, nil
}

func (a *llmAgent) Run(ctx context.Context, sessionID, userInput string) iter.Seq2[llm.Event, error] {
	return func(yield func(llm.Event, error) bool) {
		_, err := a.runTurn(ctx, sessionID, userInput, func(ev llm.Event) bool {
			return yield(ev, nil)
		})
		if err != nil && !errors.Is(err, llm.ErrEventStreamStopped) {
			yield(llm.Event{}, err)
		}
	}
}

// runTurn handles one user input. It may call the model multiple times when the
// model asks for tools: model -> tools -> model -> final answer.
func (a *llmAgent) runTurn(ctx context.Context, sessionID, userInput string, onEvent func(llm.Event) bool) (llm.Message, error) {
	sess, err := a.session(ctx, sessionID)
	if err != nil {
		return llm.Message{}, err
	}
	// Keep this turn local until it finishes so provider/tool errors do not
	// leave partial messages in the session history.
	turn := []session.Message{{Role: session.RoleUser, Content: userInput}}

	for range maxToolIterations {
		reply, err := a.model.Chat(ctx, toLLMMessages(sess.Messages, turn), a.toolDefs, onEvent)
		if err != nil {
			return llm.Message{}, err
		}
		turn = append(turn, toSessionMessage(reply))

		if len(reply.ToolCalls) == 0 {
			if err := a.commitTurn(ctx, sessionID, turn); err != nil {
				return llm.Message{}, err
			}
			return reply, nil
		}

		// Run each requested tool and feed the results back as Tool messages,
		// then loop so the model can use them to produce its next turn.
		toolMessages, err := a.runTools(ctx, reply.ToolCalls, onEvent)
		if err != nil {
			return llm.Message{}, err
		}
		turn = append(turn, toolMessages...)
	}

	return llm.Message{}, fmt.Errorf("tool loop exceeded %d iterations", maxToolIterations)
}

func (a *llmAgent) session(ctx context.Context, sessionID string) (*session.Session, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	resp, err := a.sessions.Get(ctx, &session.GetRequest{
		AppName:   a.appName,
		UserID:    a.userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Session.Messages) > 0 {
		return resp.Session, nil
	}
	if _, err := a.sessions.AppendMessage(ctx, &session.AppendMessageRequest{
		AppName:   a.appName,
		UserID:    a.userID,
		SessionID: sessionID,
		Message: session.Message{
			Role:    session.RoleSystem,
			Content: a.instruction,
		},
	}); err != nil {
		return nil, err
	}
	resp, err = a.sessions.Get(ctx, &session.GetRequest{
		AppName:   a.appName,
		UserID:    a.userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Session, nil
}

func (a *llmAgent) commitTurn(ctx context.Context, sessionID string, messages []session.Message) error {
	for _, msg := range messages {
		if _, err := a.sessions.AppendMessage(ctx, &session.AppendMessageRequest{
			AppName:   a.appName,
			UserID:    a.userID,
			SessionID: sessionID,
			Message:   msg,
		}); err != nil {
			return err
		}
	}
	return nil
}

// runTools executes the model's requested tools and appends their results to
// conversation history as tool messages.
func (a *llmAgent) runTools(ctx context.Context, calls []llm.ToolCall, onEvent func(llm.Event) bool) ([]session.Message, error) {
	messages := make([]session.Message, 0, len(calls))
	for _, tc := range calls {
		result, err := a.runTool(ctx, tc, onEvent)
		if err != nil {
			return nil, err
		}
		messages = append(messages, session.Message{
			Role:       session.RoleTool,
			Name:       tc.Name,
			ToolCallID: tc.ID,
			Content:    result,
		})
	}
	return messages, nil
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

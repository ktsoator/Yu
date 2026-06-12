package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
)

type fakeModel struct {
	replies []llm.Message
	err     error
}

func (m *fakeModel) Name() string { return "fake" }

func (m *fakeModel) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, onEvent func(llm.Event) bool) (llm.Message, error) {
	if m.err != nil {
		return llm.Message{}, m.err
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if onEvent != nil && reply.Content != "" {
		if !onEvent(llm.Event{Type: llm.EventContentDelta, Text: reply.Content}) {
			return llm.Message{}, llm.ErrEventStreamStopped
		}
	}
	return reply, nil
}

type weatherTool struct{}

func (weatherTool) Name() string        { return "get_weather" }
func (weatherTool) Description() string { return "Get the weather for a city." }
func (weatherTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
	}
}

func (weatherTool) Execute(_ tool.Context, args json.RawMessage) (tool.Result, error) {
	var in struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Content: in.City + ": sunny, 26C"}, nil
}

var _ tool.Executable = weatherTool{}

func newTestRunner(t *testing.T, model llm.Model, tools []tool.Executable) (*Runner, session.Service, string) {
	t.Helper()
	sessions := session.NewInMemoryService()
	created, err := sessions.Create(context.Background(), &session.CreateRequest{
		AppName: "yu",
		UserID:  "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ag, err := llmagent.New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Tools:       tools,
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{AppName: "yu", Agent: ag, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	return r, sessions, created.Session.ID
}

func getEvents(t *testing.T, sessions session.Service, sessionID string) []session.Event {
	t.Helper()
	resp, err := sessions.Get(context.Background(), &session.GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Session.Events
}

func TestRunPersistsTurn(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{{Role: llm.Assistant, Content: "hello"}}}
	r, sessions, sessionID := newTestRunner(t, model, nil)

	for _, err := range r.Run(context.Background(), "user-1", sessionID, "hi") {
		if err != nil {
			t.Fatal(err)
		}
	}

	events := getEvents(t, sessions, sessionID)
	if len(events) != 2 {
		t.Fatalf("expected 2 persisted events, got %d: %+v", len(events), events)
	}
	if events[0].Author != "user" || events[0].Message.Content != "hi" {
		t.Fatalf("unexpected user event: %+v", events[0])
	}
	if events[1].Message.Role != session.RoleAssistant || events[1].Message.Content != "hello" {
		t.Fatalf("unexpected assistant event: %+v", events[1])
	}
	if events[0].InvocationID == "" || events[0].InvocationID != events[1].InvocationID {
		t.Fatalf("expected shared invocation ID, got %q and %q", events[0].InvocationID, events[1].InvocationID)
	}
	for _, ev := range events {
		if ev.Partial {
			t.Fatalf("partial event was persisted: %+v", ev)
		}
		if ev.ID == "" {
			t.Fatalf("persisted event missing ID: %+v", ev)
		}
	}
}

func TestRunPersistsToolRoundTrip(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{
		{
			Role: llm.Assistant,
			ToolCalls: []llm.ToolCall{{
				ID:        "call-1",
				Name:      "get_weather",
				Arguments: `{"city":"Beijing"}`,
			}},
		},
		{Role: llm.Assistant, Content: "Beijing is sunny, 26C"},
	}}
	r, sessions, sessionID := newTestRunner(t, model, []tool.Executable{weatherTool{}})

	for _, err := range r.Run(context.Background(), "user-1", sessionID, "weather in beijing?") {
		if err != nil {
			t.Fatal(err)
		}
	}

	events := getEvents(t, sessions, sessionID)
	// user, assistant(tool call), tool result, final assistant
	if len(events) != 4 {
		t.Fatalf("expected 4 persisted events, got %d: %+v", len(events), events)
	}
	if len(events[1].Message.ToolCalls) != 1 {
		t.Fatalf("expected tool call event, got %+v", events[1])
	}
	if events[2].Type != session.EventToolResult || events[2].Message.Content != "Beijing: sunny, 26C" {
		t.Fatalf("unexpected tool result event: %+v", events[2])
	}
	if events[3].Message.Content != "Beijing is sunny, 26C" {
		t.Fatalf("unexpected final event: %+v", events[3])
	}
}

func TestRunRecordsErrorEvent(t *testing.T) {
	model := &fakeModel{err: fmt.Errorf("provider down")}
	r, sessions, sessionID := newTestRunner(t, model, nil)

	var gotErr error
	for _, err := range r.Run(context.Background(), "user-1", sessionID, "hi") {
		if err != nil {
			gotErr = err
		}
	}
	if gotErr == nil {
		t.Fatal("expected error from run")
	}

	events := getEvents(t, sessions, sessionID)
	// user event, then error event
	if len(events) != 2 {
		t.Fatalf("expected 2 persisted events, got %d: %+v", len(events), events)
	}
	if events[1].Type != session.EventError || events[1].Error != "provider down" {
		t.Fatalf("unexpected error event: %+v", events[1])
	}
}

// cancelingModel cancels the run's context from inside the model call,
// simulating Ctrl+C arriving mid-request.
type cancelingModel struct {
	cancel context.CancelFunc
}

func (m *cancelingModel) Name() string { return "canceling" }

func (m *cancelingModel) Chat(ctx context.Context, _ []llm.Message, _ []llm.ToolDef, _ func(llm.Event) bool) (llm.Message, error) {
	m.cancel()
	return llm.Message{}, ctx.Err()
}

func TestRunPersistsErrorEventAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &cancelingModel{cancel: cancel}
	r, sessions, sessionID := newTestRunner(t, model, nil)

	var gotErr error
	for _, err := range r.Run(ctx, "user-1", sessionID, "hi") {
		if err != nil {
			gotErr = err
		}
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", gotErr)
	}

	events := getEvents(t, sessions, sessionID)
	if len(events) != 2 {
		t.Fatalf("expected 2 persisted events, got %d: %+v", len(events), events)
	}
	if events[1].Type != session.EventError {
		t.Fatalf("expected error event despite cancelled context, got %+v", events[1])
	}
}

func TestRunSeparatesSessions(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{
		{Role: llm.Assistant, Content: "first"},
		{Role: llm.Assistant, Content: "second"},
	}}
	r, sessions, firstID := newTestRunner(t, model, nil)
	second, err := sessions.Create(context.Background(), &session.CreateRequest{
		AppName: "yu",
		UserID:  "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, err := range r.Run(context.Background(), "user-1", firstID, "one") {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, err := range r.Run(context.Background(), "user-1", second.Session.ID, "two") {
		if err != nil {
			t.Fatal(err)
		}
	}

	if events := getEvents(t, sessions, firstID); events[0].Message.Content != "one" {
		t.Fatalf("unexpected first session events: %+v", events)
	}
	if events := getEvents(t, sessions, second.Session.ID); events[0].Message.Content != "two" {
		t.Fatalf("unexpected second session events: %+v", events)
	}
}

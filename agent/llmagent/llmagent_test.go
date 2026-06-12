package llmagent

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
)

type fakeModel struct {
	replies []llm.Message
	seen    [][]llm.Message
}

func (m *fakeModel) Name() string { return "fake" }

func (m *fakeModel) Chat(_ context.Context, messages []llm.Message, _ []llm.ToolDef, onEvent func(llm.Event) bool) (llm.Message, error) {
	m.seen = append(m.seen, append([]llm.Message(nil), messages...))
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if onEvent != nil && reply.Content != "" {
		if !onEvent(llm.Event{Type: llm.EventContentDelta, Text: reply.Content}) {
			return llm.Message{}, llm.ErrEventStreamStopped
		}
	}
	return reply, nil
}

type fakeTool struct{}

func (fakeTool) Name() string        { return "echo" }
func (fakeTool) Description() string { return "Echo a value." }
func (fakeTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
}

func (fakeTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	return in.Value, nil
}

var _ tool.Tool = fakeTool{}

func testICtx(events ...session.Event) *agent.InvocationContext {
	return &agent.InvocationContext{
		InvocationID: "inv-test",
		AppName:      "yu",
		UserID:       "user-1",
		Session: &session.Session{
			ID:      "sess-1",
			AppName: "yu",
			UserID:  "user-1",
			Events:  events,
		},
	}
}

func userEvent(content string) session.Event {
	return session.Event{
		Type:    session.EventMessage,
		Author:  "user",
		Message: session.Message{Role: session.RoleUser, Content: content},
	}
}

func TestRunYieldsFinalMessageEvent(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{{
		Role:    llm.Assistant,
		Content: "hello",
	}}}
	ag, err := New(agent.Config{Name: "yu", Model: model, Instruction: "be useful"})
	if err != nil {
		t.Fatal(err)
	}

	events := collect(t, ag.Run(context.Background(), testICtx(userEvent("hi"))))

	final := finalEvents(events)
	if len(final) != 1 {
		t.Fatalf("expected 1 final event, got %d: %+v", len(final), final)
	}
	if final[0].Type != session.EventMessage || final[0].Message.Role != session.RoleAssistant {
		t.Fatalf("unexpected final event: %+v", final[0])
	}
	if final[0].Message.Content != "hello" {
		t.Fatalf("unexpected content: %q", final[0].Message.Content)
	}
	if final[0].InvocationID != "inv-test" {
		t.Fatalf("expected invocation ID propagated, got %q", final[0].InvocationID)
	}
	if !hasPartial(events, session.EventContentDelta) {
		t.Fatal("expected streamed content delta event")
	}
}

func TestRunInjectsInstructionPerRequest(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{{Role: llm.Assistant, Content: "ok"}}}
	ag, err := New(agent.Config{Name: "yu", Model: model, Instruction: "be useful"})
	if err != nil {
		t.Fatal(err)
	}

	collect(t, ag.Run(context.Background(), testICtx(userEvent("hi"))))

	seen := model.seen[0]
	if seen[0].Role != llm.System || seen[0].Content != "be useful" {
		t.Fatalf("expected system instruction first, got %+v", seen[0])
	}
	if seen[1].Role != llm.User || seen[1].Content != "hi" {
		t.Fatalf("expected user message second, got %+v", seen[1])
	}
}

func TestRunToolRoundTrip(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{
		{
			Role: llm.Assistant,
			ToolCalls: []llm.ToolCall{{
				ID:        "call-1",
				Name:      "echo",
				Arguments: `{"value":"from tool"}`,
			}},
		},
		{Role: llm.Assistant, Content: "done"},
	}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Tools:       []tool.Tool{fakeTool{}},
	})
	if err != nil {
		t.Fatal(err)
	}

	events := collect(t, ag.Run(context.Background(), testICtx(userEvent("use tool"))))

	final := finalEvents(events)
	if len(final) != 3 {
		t.Fatalf("expected 3 final events, got %d: %+v", len(final), final)
	}
	if len(final[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call event, got %+v", final[0])
	}
	if final[1].Type != session.EventToolResult || final[1].Message.Content != "from tool" {
		t.Fatalf("unexpected tool result event: %+v", final[1])
	}
	if final[1].Message.ToolCallID != "call-1" {
		t.Fatalf("expected tool result linked to call-1, got %+v", final[1])
	}
	if final[2].Message.Content != "done" {
		t.Fatalf("unexpected final assistant event: %+v", final[2])
	}

	if len(model.seen) != 2 {
		t.Fatalf("expected 2 model calls, got %d", len(model.seen))
	}
	secondCall := model.seen[1]
	if secondCall[len(secondCall)-1].Role != llm.Tool {
		t.Fatalf("expected second model call to include tool result, got %+v", secondCall)
	}
}

func collect(t *testing.T, events iter.Seq2[*session.Event, error]) []*session.Event {
	t.Helper()
	var out []*session.Event
	for ev, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func finalEvents(events []*session.Event) []*session.Event {
	var out []*session.Event
	for _, ev := range events {
		if !ev.Partial {
			out = append(out, ev)
		}
	}
	return out
}

func hasPartial(events []*session.Event, typ session.EventType) bool {
	for _, ev := range events {
		if ev.Partial && ev.Type == typ {
			return true
		}
	}
	return false
}

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

func TestRunStoresTurnInRequestedSession(t *testing.T) {
	ctx := context.Background()
	sessions := session.NewInMemoryService()
	sess := createTestSession(t, sessions, "user-1")
	model := &fakeModel{replies: []llm.Message{{
		Role:    llm.Assistant,
		Content: "hello",
	}}}

	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Sessions:    sessions,
		UserID:      "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, ag.Run(ctx, sess.ID, "hi"))

	got := getTestSession(t, sessions, "user-1", sess.ID)
	if len(got.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got.Messages))
	}
	if got.Messages[0].Role != session.RoleSystem {
		t.Fatalf("expected system message, got %s", got.Messages[0].Role)
	}
	if got.Messages[1].Role != session.RoleUser || got.Messages[1].Content != "hi" {
		t.Fatalf("unexpected user message: %+v", got.Messages[1])
	}
	if got.Messages[2].Role != session.RoleAssistant || got.Messages[2].Content != "hello" {
		t.Fatalf("unexpected assistant message: %+v", got.Messages[2])
	}
}

func TestRunStoresToolRoundTripInSession(t *testing.T) {
	ctx := context.Background()
	sessions := session.NewInMemoryService()
	sess := createTestSession(t, sessions, "user-1")
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
		Sessions:    sessions,
		UserID:      "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, ag.Run(ctx, sess.ID, "use tool"))

	got := getTestSession(t, sessions, "user-1", sess.ID)
	if len(got.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(got.Messages))
	}
	if len(got.Messages[2].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool call, got %+v", got.Messages[2])
	}
	if got.Messages[3].Role != session.RoleTool || got.Messages[3].Content != "from tool" {
		t.Fatalf("unexpected tool message: %+v", got.Messages[3])
	}
	if got.Messages[4].Role != session.RoleAssistant || got.Messages[4].Content != "done" {
		t.Fatalf("unexpected final assistant message: %+v", got.Messages[4])
	}
	if len(model.seen) != 2 {
		t.Fatalf("expected 2 model calls, got %d", len(model.seen))
	}
	secondCall := model.seen[1]
	if secondCall[len(secondCall)-1].Role != llm.Tool {
		t.Fatalf("expected second model call to include tool result, got %+v", secondCall)
	}
}

func TestRunUsesRequestedSession(t *testing.T) {
	ctx := context.Background()
	sessions := session.NewInMemoryService()
	first := createTestSession(t, sessions, "user-1")
	second := createTestSession(t, sessions, "user-1")
	model := &fakeModel{replies: []llm.Message{
		{Role: llm.Assistant, Content: "first"},
		{Role: llm.Assistant, Content: "second"},
	}}

	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Sessions:    sessions,
		UserID:      "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, ag.Run(ctx, first.ID, "one"))
	drain(t, ag.Run(ctx, second.ID, "two"))

	first = getTestSession(t, sessions, "user-1", first.ID)
	second = getTestSession(t, sessions, "user-1", second.ID)
	if first.Messages[1].Content != "one" {
		t.Fatalf("unexpected first session user message: %+v", first.Messages)
	}
	if second.Messages[1].Content != "two" {
		t.Fatalf("unexpected second session user message: %+v", second.Messages)
	}
}

func drain(t *testing.T, events iter.Seq2[llm.Event, error]) {
	t.Helper()
	for _, err := range events {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func createTestSession(t *testing.T, sessions session.Service, userID string) *session.Session {
	t.Helper()
	resp, err := sessions.Create(context.Background(), &session.CreateRequest{
		AppName: "yu",
		UserID:  userID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Session
}

func getTestSession(t *testing.T, sessions session.Service, userID, sessionID string) *session.Session {
	t.Helper()
	resp, err := sessions.Get(context.Background(), &session.GetRequest{
		AppName:   "yu",
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp.Session
}

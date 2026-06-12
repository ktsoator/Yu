package launcher

import (
	"context"
	"iter"
	"testing"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/session"
)

type fakeAgent struct{}

func (fakeAgent) Name() string { return "fake" }

func (fakeAgent) Run(ctx context.Context, ictx *agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		yield(&session.Event{
			InvocationID: ictx.InvocationID,
			SessionID:    ictx.Session.ID,
			Type:         session.EventMessage,
			Author:       "fake",
			Message: session.Message{
				Role:    session.RoleAssistant,
				Content: "hello",
			},
		}, nil)
	}
}

func (fakeAgent) SetModel(llm.Model)        {}
func (fakeAgent) SetThinking(bool)          {}
func (fakeAgent) Thinking() bool            { return false }
func (fakeAgent) SupportsThinking() bool    { return false }
func (fakeAgent) Description() string       { return "" }
func (fakeAgent) Instruction() string       { return "" }
func (fakeAgent) SetInstruction(string)     {}
func (fakeAgent) SetDescription(string)     {}
func (fakeAgent) SetName(string)            {}
func (fakeAgent) SetTools([]interface{})    {}
func (fakeAgent) SetToolsets([]interface{}) {}

type noopRenderer struct{}

func (noopRenderer) OnEvent(*session.Event) {}
func (noopRenderer) Finish()                {}

func TestTerminalExecuteOneShotCreatesSessionAndRunsAgent(t *testing.T) {
	sessions := session.NewInMemoryService()
	l := NewTerminal()

	err := l.Execute(context.Background(), &Config{
		AgentLoader: agent.NewSingleLoader(fakeAgent{}),
		Sessions:    sessions,
		AppName:     "yu",
		UserID:      "user-1",
		Renderer:    func() render.Renderer { return noopRenderer{} },
	}, []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := sessions.List(context.Background(), &session.ListRequest{
		AppName: "yu",
		UserID:  "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(listed.Sessions))
	}
	got := listed.Sessions[0]
	resp, err := sessions.Get(context.Background(), &session.GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: got.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Session.Events) != 2 {
		t.Fatalf("expected user and assistant events, got %+v", resp.Session.Events)
	}
	if resp.Session.Events[0].Message.Content != "hello" {
		t.Fatalf("unexpected user event: %+v", resp.Session.Events[0])
	}
	if resp.Session.Events[1].Message.Content != "hello" {
		t.Fatalf("unexpected assistant event: %+v", resp.Session.Events[1])
	}
}

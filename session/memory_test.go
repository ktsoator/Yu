package session

import (
	"context"
	"testing"
)

func TestInMemoryServiceAppendAndGet(t *testing.T) {
	ctx := context.Background()
	service := NewInMemoryService()

	created, err := service.Create(ctx, &CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	sess := created.Session
	if sess.ID == "" {
		t.Fatal("expected session ID")
	}

	appended, err := service.AppendMessage(ctx, &AppendMessageRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: sess.ID,
		Message: Message{
			Role:    RoleUser,
			Content: "hello",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended.Message.ID == "" {
		t.Fatal("expected message ID")
	}
	if appended.Message.CreatedAt.IsZero() {
		t.Fatal("expected message CreatedAt")
	}

	got, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Session.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got.Session.Messages))
	}
	if got.Session.Messages[0].Content != "hello" {
		t.Fatalf("unexpected content: %s", got.Session.Messages[0].Content)
	}
}

func TestInMemoryServiceReturnsClones(t *testing.T) {
	ctx := context.Background()
	service := NewInMemoryService()
	created, err := service.Create(ctx, &CreateRequest{
		AppName: "yu",
		UserID:  "user-1",
		State:   map[string]any{"initial": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess := created.Session
	if _, err := service.AppendMessage(ctx, &AppendMessageRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: sess.ID,
		Message: Message{
			Role:    RoleAssistant,
			Content: "hello",
			ToolCalls: []ToolCall{{
				ID:        "call-1",
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	got.Session.Messages[0].Content = "mutated"
	got.Session.Messages[0].ToolCalls[0].Name = "mutated"
	got.Session.State["x"] = "mutated"

	again, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Session.Messages[0].Content != "hello" {
		t.Fatalf("stored message was mutated: %q", again.Session.Messages[0].Content)
	}
	if again.Session.Messages[0].ToolCalls[0].Name != "read_file" {
		t.Fatalf("stored tool call was mutated: %q", again.Session.Messages[0].ToolCalls[0].Name)
	}
	if _, ok := again.Session.State["x"]; ok {
		t.Fatal("stored state was mutated")
	}
}

func TestInMemoryServiceListAndDelete(t *testing.T) {
	ctx := context.Background()
	service := NewInMemoryService()

	first, err := service.Create(ctx, &CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, &CreateRequest{AppName: "yu", UserID: "user-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, &CreateRequest{AppName: "yu", UserID: "user-2"}); err != nil {
		t.Fatal(err)
	}

	listed, err := service.List(ctx, &ListRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(listed.Sessions))
	}

	if err := service.Delete(ctx, &DeleteRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: first.Session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, &GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: first.Session.ID,
	}); err == nil {
		t.Fatal("expected deleted session to be missing")
	}
}

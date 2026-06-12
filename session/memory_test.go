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

	appended, err := service.AppendEvent(ctx, &AppendEventRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: sess.ID,
		Event: Event{
			Type:   EventMessage,
			Author: "user",
			Message: Message{
				Role:    RoleUser,
				Content: "hello",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended.Event.ID == "" {
		t.Fatal("expected event ID")
	}
	if appended.Event.SessionID != sess.ID {
		t.Fatalf("expected event session ID %s, got %s", sess.ID, appended.Event.SessionID)
	}
	if appended.Event.CreatedAt.IsZero() {
		t.Fatal("expected event CreatedAt")
	}

	got, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Session.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Session.Events))
	}
	if got.Session.Events[0].Message.Content != "hello" {
		t.Fatalf("unexpected content: %s", got.Session.Events[0].Message.Content)
	}
}

func TestInMemoryServiceRejectsPartialEvents(t *testing.T) {
	ctx := context.Background()
	service := NewInMemoryService()
	created, err := service.Create(ctx, &CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(ctx, &AppendEventRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: created.Session.ID,
		Event: Event{
			Type:    EventContentDelta,
			Partial: true,
			Message: Message{Role: RoleAssistant, Content: "frag"},
		},
	}); err == nil {
		t.Fatal("expected partial event append to fail")
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
	if _, err := service.AppendEvent(ctx, &AppendEventRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: sess.ID,
		Event: Event{
			Type:   EventMessage,
			Author: "yu",
			Message: Message{
				Role:    RoleAssistant,
				Content: "hello",
				ToolCalls: []ToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	got.Session.Events[0].Message.Content = "mutated"
	got.Session.Events[0].Message.ToolCalls[0].Name = "mutated"
	got.Session.State["x"] = "mutated"

	again, err := service.Get(ctx, &GetRequest{AppName: "yu", UserID: "user-1", SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Session.Events[0].Message.Content != "hello" {
		t.Fatalf("stored event was mutated: %q", again.Session.Events[0].Message.Content)
	}
	if again.Session.Events[0].Message.ToolCalls[0].Name != "read_file" {
		t.Fatalf("stored tool call was mutated: %q", again.Session.Events[0].Message.ToolCalls[0].Name)
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

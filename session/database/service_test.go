package database

import (
	"context"
	"testing"
	"time"

	"github.com/ktsoator/yu/session"
)

func TestServiceAppendAndGet(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t)

	created, err := service.Create(ctx, &session.CreateRequest{
		AppName: "yu",
		UserID:  "user-1",
		State:   map[string]any{"initial": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}

	appended, err := service.AppendEvent(ctx, &session.AppendEventRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: created.Session.ID,
		Event: session.Event{
			Type:   session.EventMessage,
			Author: "user",
			Message: session.Message{
				Role:    session.RoleUser,
				Content: "hello",
				ToolCalls: []session.ToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended.Event.ID == "" {
		t.Fatal("expected event ID")
	}
	if appended.Event.SessionID != created.Session.ID {
		t.Fatalf("expected event session ID %s, got %s", created.Session.ID, appended.Event.SessionID)
	}

	got, err := service.Get(ctx, &session.GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: created.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.State["initial"] != "value" {
		t.Fatalf("unexpected state: %+v", got.Session.State)
	}
	if len(got.Session.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Session.Events))
	}
	if got.Session.Events[0].Message.Content != "hello" {
		t.Fatalf("unexpected content: %q", got.Session.Events[0].Message.Content)
	}
	if got.Session.Events[0].Message.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected tool calls: %+v", got.Session.Events[0].Message.ToolCalls)
	}
}

func TestServiceRejectsPartialEvents(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t)
	created, err := service.Create(ctx, &session.CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendEvent(ctx, &session.AppendEventRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: created.Session.ID,
		Event: session.Event{
			Type:    session.EventContentDelta,
			Partial: true,
			Message: session.Message{Role: session.RoleAssistant, Content: "frag"},
		},
	}); err == nil {
		t.Fatal("expected partial event append to fail")
	}
}

func TestServiceListAndDelete(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t)

	first, err := service.Create(ctx, &session.CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, &session.CreateRequest{AppName: "yu", UserID: "user-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, &session.CreateRequest{AppName: "yu", UserID: "user-2"}); err != nil {
		t.Fatal(err)
	}

	listed, err := service.List(ctx, &session.ListRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(listed.Sessions))
	}

	if err := service.Delete(ctx, &session.DeleteRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: first.Session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, &session.GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: first.Session.ID,
	}); err == nil {
		t.Fatal("expected deleted session to be missing")
	}
}

func TestServicePreservesEventOrder(t *testing.T) {
	ctx := context.Background()
	service := newTestService(t)
	created, err := service.Create(ctx, &session.CreateRequest{AppName: "yu", UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{"one", "two", "three"} {
		if _, err := service.AppendEvent(ctx, &session.AppendEventRequest{
			AppName:   "yu",
			UserID:    "user-1",
			SessionID: created.Session.ID,
			Event: session.Event{
				Type:      session.EventMessage,
				Author:    "user",
				CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Message:   session.Message{Role: session.RoleUser, Content: input},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := service.Get(ctx, &session.GetRequest{
		AppName:   "yu",
		UserID:    "user-1",
		SessionID: created.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"one", "two", "three"} {
		if got.Session.Events[i].Message.Content != want {
			t.Fatalf("event %d = %q, want %q", i, got.Session.Events[i].Message.Content, want)
		}
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := Open(t.Context(), DriverSQLite, "file:"+newID("test")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service
}

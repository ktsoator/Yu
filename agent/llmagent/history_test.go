package llmagent

import (
	"testing"

	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
)

func assistantToolCallEvent(ids ...string) session.Event {
	calls := make([]session.ToolCall, 0, len(ids))
	for _, id := range ids {
		calls = append(calls, session.ToolCall{ID: id, Name: "do", Arguments: "{}"})
	}
	return session.Event{
		Type:    session.EventMessage,
		Message: session.Message{Role: session.RoleAssistant, ToolCalls: calls},
	}
}

func toolResultEventFor(id string) session.Event {
	return session.Event{
		Type:    session.EventToolResult,
		Message: session.Message{Role: session.RoleTool, ToolCallID: id, Content: "ok"},
	}
}

func userMessageEvent(text string) session.Event {
	return session.Event{
		Type:    session.EventMessage,
		Message: session.Message{Role: session.RoleUser, Content: text},
	}
}

// assertToolCallsAnswered pins the core agent-loop invariant: every assistant
// tool call in the reconstructed context has a matching tool-result message.
func assertToolCallsAnswered(t *testing.T, msgs []llm.Message) {
	t.Helper()
	answered := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == llm.Tool && m.ToolCallID != "" {
			answered[m.ToolCallID] = true
		}
	}
	for _, m := range msgs {
		if m.Role != llm.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if !answered[tc.ID] {
				t.Fatalf("dangling tool call %q has no result message", tc.ID)
			}
		}
	}
}

func TestToLLMMessagesRepairsFullyTornTurn(t *testing.T) {
	// Assistant requested two tools; history was torn before any result landed.
	events := []session.Event{
		userMessageEvent("hi"),
		assistantToolCallEvent("a", "b"),
	}

	msgs := toLLMMessages("inst", events)

	assertToolCallsAnswered(t, msgs)
	if got := countRole(msgs, llm.Tool); got != 2 {
		t.Fatalf("expected 2 synthetic tool results, got %d", got)
	}
	for _, m := range msgs {
		if m.Role == llm.Tool && m.Content != missingToolResult {
			t.Fatalf("synthetic result content = %q, want placeholder", m.Content)
		}
	}
}

func TestToLLMMessagesRepairsPartiallyTornTurn(t *testing.T) {
	// Tool a finished and was persisted; tool b was lost mid-turn.
	events := []session.Event{
		userMessageEvent("hi"),
		assistantToolCallEvent("a", "b"),
		toolResultEventFor("a"),
	}

	msgs := toLLMMessages("inst", events)

	assertToolCallsAnswered(t, msgs)
	if got := countRole(msgs, llm.Tool); got != 2 {
		t.Fatalf("expected 2 tool results (1 real, 1 synthetic), got %d", got)
	}
	// The real result must be preserved, not overwritten by a placeholder.
	if !hasToolResult(msgs, "a", "ok") {
		t.Fatalf("real tool result for %q was not preserved", "a")
	}
}

func TestToLLMMessagesLeavesHealthyHistoryUnchanged(t *testing.T) {
	events := []session.Event{
		userMessageEvent("hi"),
		assistantToolCallEvent("a"),
		toolResultEventFor("a"),
	}

	msgs := toLLMMessages("inst", events)

	assertToolCallsAnswered(t, msgs)
	if got := countRole(msgs, llm.Tool); got != 1 {
		t.Fatalf("healthy history should not gain synthetic results, got %d tool messages", got)
	}
}

func countRole(msgs []llm.Message, role llm.Role) int {
	n := 0
	for _, m := range msgs {
		if m.Role == role {
			n++
		}
	}
	return n
}

func hasToolResult(msgs []llm.Message, id, content string) bool {
	for _, m := range msgs {
		if m.Role == llm.Tool && m.ToolCallID == id && m.Content == content {
			return true
		}
	}
	return false
}

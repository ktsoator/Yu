package llmagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
)

// writeFakeTool is a non-read-only tool that records whether it ran.
type writeFakeTool struct{ ran *bool }

func (writeFakeTool) Name() string        { return "write_thing" }
func (writeFakeTool) Description() string { return "writes a thing" }
func (writeFakeTool) ReadOnly() bool      { return false }
func (writeFakeTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string"}},
	}
}

func (w writeFakeTool) Execute(_ tool.Context, _ json.RawMessage) (tool.Result, error) {
	if w.ran != nil {
		*w.ran = true
	}
	return tool.Result{Content: "wrote"}, nil
}

func toolCallReply(name string) llm.Message {
	return llm.Message{
		Role:      llm.Assistant,
		ToolCalls: []llm.ToolCall{{ID: "c1", Name: name, Arguments: `{"value":"x"}`}},
	}
}

func toolResultContent(events []*session.Event) string {
	for _, ev := range events {
		if ev.Type == session.EventToolResult {
			return ev.Message.Content
		}
	}
	return ""
}

func TestApprovalDeniedToolIsNotExecuted(t *testing.T) {
	ran := false
	model := &fakeModel{replies: []llm.Message{
		toolCallReply("write_thing"),
		{Role: llm.Assistant, Content: "ok"},
	}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "go",
		Tools:       []tool.Tool{writeFakeTool{ran: &ran}},
		Approve:     func(tool.Tool, string) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	events := collect(t, ag.Run(context.Background(), testICtx(userEvent("write"))))

	if ran {
		t.Fatal("denied tool must not execute")
	}
	if !hasPartial(events, session.EventToolApproval) {
		t.Fatal("expected streamed tool approval event")
	}
	if got := toolResultContent(events); !strings.Contains(got, "rejected") {
		t.Fatalf("rejection should be fed back as the tool result, got %q", got)
	}
}

func TestApprovalAllowedToolExecutes(t *testing.T) {
	ran := false
	model := &fakeModel{replies: []llm.Message{
		toolCallReply("write_thing"),
		{Role: llm.Assistant, Content: "ok"},
	}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "go",
		Tools:       []tool.Tool{writeFakeTool{ran: &ran}},
		Approve:     func(tool.Tool, string) (bool, error) { return true, nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	collect(t, ag.Run(context.Background(), testICtx(userEvent("write"))))

	if !ran {
		t.Fatal("approved tool should execute")
	}
}

func TestApprovalSkippedForReadOnlyTool(t *testing.T) {
	// fakeTool is read-only ("echo"); the approver must never be consulted, even
	// one that would deny everything.
	asked := false
	model := &fakeModel{replies: []llm.Message{
		{Role: llm.Assistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: `{"value":"hi"}`}}},
		{Role: llm.Assistant, Content: "done"},
	}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "go",
		Tools:       []tool.Tool{fakeTool{}},
		Approve: func(tool.Tool, string) (bool, error) {
			asked = true
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	events := collect(t, ag.Run(context.Background(), testICtx(userEvent("use echo"))))

	if asked {
		t.Fatal("read-only tools must not be gated by the approver")
	}
	if hasPartial(events, session.EventToolApproval) {
		t.Fatal("read-only tool should not emit an approval event")
	}
	if got := toolResultContent(events); got != "hi" {
		t.Fatalf("read-only tool should have run, got result %q", got)
	}
}

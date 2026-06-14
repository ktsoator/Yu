package llmagent

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/tool"
)

func TestRunInjectsEnvironmentContext(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{{Role: llm.Assistant, Content: "ok"}}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Environment: func(string) string { return "<env>TEST</env>" },
	})
	if err != nil {
		t.Fatal(err)
	}

	collect(t, ag.Run(context.Background(), testICtx(userEvent("hi"))))

	sys := model.seen[0][0]
	if sys.Role != llm.System {
		t.Fatalf("expected system message first, got %+v", sys)
	}
	if !strings.Contains(sys.Content, "be useful") {
		t.Fatalf("system prompt lost the instruction: %q", sys.Content)
	}
	if !strings.Contains(sys.Content, "<env>TEST</env>") {
		t.Fatalf("system prompt missing env block: %q", sys.Content)
	}
}

func TestRunAppendsToolNotes(t *testing.T) {
	noter, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "noter",
		Description: "does a thing",
		Prompt:      "use me carefully",
	}, func(tool.Context, struct{}) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}

	model := &fakeModel{replies: []llm.Message{{Role: llm.Assistant, Content: "ok"}}}
	ag, err := New(agent.Config{
		Name:        "yu",
		Model:       model,
		Instruction: "be useful",
		Tools:       []tool.Tool{noter},
	})
	if err != nil {
		t.Fatal(err)
	}

	collect(t, ag.Run(context.Background(), testICtx(userEvent("hi"))))

	sys := model.seen[0][0].Content
	if !strings.Contains(sys, "# Tool notes") || !strings.Contains(sys, "- noter: use me carefully") {
		t.Fatalf("system prompt missing tool notes:\n%s", sys)
	}
}

func TestRunWithoutEnvironmentKeepsInstructionOnly(t *testing.T) {
	model := &fakeModel{replies: []llm.Message{{Role: llm.Assistant, Content: "ok"}}}
	ag, err := New(agent.Config{Name: "yu", Model: model, Instruction: "be useful"})
	if err != nil {
		t.Fatal(err)
	}

	collect(t, ag.Run(context.Background(), testICtx(userEvent("hi"))))

	if got := model.seen[0][0].Content; got != "be useful" {
		t.Fatalf("expected plain instruction, got %q", got)
	}
}

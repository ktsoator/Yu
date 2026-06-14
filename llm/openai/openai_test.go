package openai

import (
	"testing"

	oai "github.com/openai/openai-go/v3"
)

func delta(index int64, id, name, args string) oai.ChatCompletionChunkChoiceDeltaToolCall {
	return oai.ChatCompletionChunkChoiceDeltaToolCall{
		Index: index,
		ID:    id,
		Function: oai.ChatCompletionChunkChoiceDeltaToolCallFunction{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestToolCallAccumulatorReassemblesFragments(t *testing.T) {
	acc := newToolCallAccumulator()

	acc.add([]oai.ChatCompletionChunkChoiceDeltaToolCall{delta(0, "c1", "write_file", "")})
	acc.add([]oai.ChatCompletionChunkChoiceDeltaToolCall{delta(0, "", "", `{"path":"a.go",`)})
	last := acc.add([]oai.ChatCompletionChunkChoiceDeltaToolCall{delta(0, "", "", `"content":"x"}`)})

	if len(last) != 1 {
		t.Fatalf("expected a progress event on the final fragment, got %d", len(last))
	}
	if last[0].ToolName != "write_file" || last[0].ToolArgs != `{"path":"a.go","content":"x"}` {
		t.Fatalf("unexpected progress event: %+v", last[0])
	}

	calls := acc.result()
	if len(calls) != 1 {
		t.Fatalf("expected 1 assembled call, got %d", len(calls))
	}
	if calls[0].ID != "c1" || calls[0].Name != "write_file" || calls[0].Arguments != `{"path":"a.go","content":"x"}` {
		t.Fatalf("unexpected assembled call: %+v", calls[0])
	}
}

func TestToolCallAccumulatorWaitsForName(t *testing.T) {
	acc := newToolCallAccumulator()
	ev := acc.add([]oai.ChatCompletionChunkChoiceDeltaToolCall{delta(0, "c1", "", `{"path":`)})
	if len(ev) != 0 {
		t.Fatalf("expected no event before the tool name is known, got %+v", ev)
	}
}

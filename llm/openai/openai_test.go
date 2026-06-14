package openai

import "testing"

func TestToolCallStatusCountsCompleteWriteFileLines(t *testing.T) {
	got := toolCallStatus("write_file", `{"path":"main.go","content":"package main\n\nfunc main() {}\n"}`)
	if got != "main.go 3 lines" {
		t.Fatalf("unexpected status: %q", got)
	}
}

func TestToolCallStatusCountsPartialEditFileLines(t *testing.T) {
	got := toolCallStatus("edit_file", `{"path":"main.go","new_string":"one\ntwo\nthr`)
	if got != "main.go 3 lines" {
		t.Fatalf("unexpected status: %q", got)
	}
}

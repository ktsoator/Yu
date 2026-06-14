package clirender

import (
	"strings"
	"testing"
)

func TestSummarizeArgs(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"path", `{"path":"cmd/yu/main.go"}`, "cmd/yu/main.go"},
		{"path preferred over body", `{"path":"a.go","content":"x"}`, "a.go"},
		{"command", `{"command":"go test ./..."}`, "go test ./..."},
		{"pattern when no path", `{"pattern":"func main"}`, "func main"},
		{"partial streamed json", `{"path":"cmd/yu/main.go","content":"abc`, "cmd/yu/main.go"},
		{"empty", ``, ""},
		{"no display key", `{"value":"x"}`, ""},
	}
	for _, c := range cases {
		if got := SummarizeArgs(c.in); got != c.want {
			t.Errorf("%s: SummarizeArgs(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRendererSummarizePrefersToolThenFallsBack(t *testing.T) {
	r := New(func(name, args string) string {
		if name == "grep" {
			return "func main in cmd/"
		}
		return ""
	})

	if got := r.summarize("grep", `{"pattern":"func main","path":"cmd/"}`); got != "func main in cmd/" {
		t.Fatalf("tool summary not used: %q", got)
	}
	// read_file has no custom summary, so the generic path summary applies.
	if got := r.summarize("read_file", `{"path":"a.go"}`); got != "a.go" {
		t.Fatalf("fallback summary wrong: %q", got)
	}
}

func TestSummarizeResult(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"short verbatim", "wrote 42 bytes to a.go", "wrote 42 bytes to a.go"},
		{"error shown", "error: old_string not found in a.go", "error: old_string not found in a.go"},
		{"no output", "", "(no output)"},
		{"trailing newline trimmed", "only line\n", "only line"},
		{"multiline counted", "a\nb\nc", "3 lines"},
	}
	for _, c := range cases {
		if got := summarizeResult(c.in); got != c.want {
			t.Errorf("%s: summarizeResult(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRendererSummarizeNilToolSummary(t *testing.T) {
	r := New(nil)
	if got := r.summarize("read_file", `{"path":"a.go"}`); got != "a.go" {
		t.Fatalf("nil summarizer should fall back, got %q", got)
	}
}

func TestSummarizeArgsTruncatesLongValue(t *testing.T) {
	in := `{"command":"` + strings.Repeat("a", 200) + `"}`
	got := SummarizeArgs(in)
	if n := len([]rune(got)); n > 80 {
		t.Fatalf("not truncated: %d runes", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("missing ellipsis: %q", got)
	}
}

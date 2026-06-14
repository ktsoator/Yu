package fstool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/yu/tool"
)

func writeTemp(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pat, path string
		want      bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.txt", false},
		{"cmd/**/*.go", "cmd/yu/main.go", true},
		{"cmd/**/*.go", "cmd/main.go", true},
		{"cmd/*.go", "cmd/main.go", true},
		{"cmd/*.go", "cmd/yu/main.go", false},
		{"a?c.go", "abc.go", true},
		{"a?c.go", "ac.go", false},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.pat)
		if err != nil {
			t.Fatalf("globToRegexp(%q): %v", c.pat, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q vs %q = %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestGlobFindsFilesByName(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.go", "")
	writeTemp(t, dir, "b.txt", "")
	writeTemp(t, dir, "sub/c.go", "")
	writeTemp(t, dir, ".git/should_skip.go", "")

	out, err := glob(tool.Context{WorkDir: dir}, globArgs{Pattern: "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, filepath.Join("sub", "c.go")) {
		t.Fatalf("glob missed expected files:\n%s", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Fatalf("glob matched a non-.go file:\n%s", out)
	}
	if strings.Contains(out, "should_skip") {
		t.Fatalf("glob walked into .git:\n%s", out)
	}
}

func TestGrepFindsMatches(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "f.txt", "alpha\nbeta\nalpha again\n")

	out, err := grep(tool.Context{WorkDir: dir}, grepArgs{Pattern: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "f.txt:1: alpha") {
		t.Fatalf("grep missed line 1:\n%s", out)
	}
	if !strings.Contains(out, "f.txt:3: alpha again") {
		t.Fatalf("grep missed line 3:\n%s", out)
	}
	if strings.Contains(out, "beta") {
		t.Fatalf("grep reported a non-matching line:\n%s", out)
	}
}

func TestGrepNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "f.txt", "nothing here\n")

	out, err := grep(tool.Context{WorkDir: dir}, grepArgs{Pattern: "absent"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "no matches" {
		t.Fatalf("expected \"no matches\", got %q", out)
	}
}

func TestWriteFileWithinWorkDir(t *testing.T) {
	dir := t.TempDir()

	if _, err := writeFile(tool.Context{WorkDir: dir}, writeFileArgs{Path: "sub/a.txt", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}
}

func TestWriteFileRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"../escape.txt", "/etc/passwd", "sub/../../escape.txt"} {
		if _, err := writeFile(tool.Context{WorkDir: dir}, writeFileArgs{Path: p, Content: "x"}); err == nil {
			t.Fatalf("expected rejection for path %q outside the work dir", p)
		}
	}
}

func TestEditFileReplacesUnique(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.go", "package x\n\nfunc foo() {}\n")

	if _, err := editFile(tool.Context{WorkDir: dir}, editFileArgs{
		Path: "a.go", OldString: "foo", NewString: "bar",
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	if !strings.Contains(string(got), "func bar()") {
		t.Fatalf("edit not applied: %s", got)
	}
}

func TestEditFileReplaceAll(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "x x x")

	out, err := editFile(tool.Context{WorkDir: dir}, editFileArgs{
		Path: "a.txt", OldString: "x", NewString: "y", ReplaceAll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "y y y" {
		t.Fatalf("got %q, want %q", got, "y y y")
	}
	if !strings.Contains(out, "3 occurrence") {
		t.Fatalf("summary = %q, want 3 occurrences", out)
	}
}

func TestEditFileAmbiguousWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "x x")

	if _, err := editFile(tool.Context{WorkDir: dir}, editFileArgs{
		Path: "a.txt", OldString: "x", NewString: "y",
	}); err == nil {
		t.Fatal("expected error for non-unique old_string")
	}
}

func TestEditFileErrors(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.txt", "hello")

	cases := map[string]editFileArgs{
		"not found":    {Path: "a.txt", OldString: "absent", NewString: "y"},
		"no change":    {Path: "a.txt", OldString: "hello", NewString: "hello"},
		"missing file": {Path: "nope.txt", OldString: "a", NewString: "b"},
		"empty old":    {Path: "a.txt", OldString: "", NewString: "b"},
	}
	for name, args := range cases {
		if _, err := editFile(tool.Context{WorkDir: dir}, args); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

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

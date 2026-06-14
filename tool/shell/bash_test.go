package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/yu/tool"
)

func ctxFor(dir string) tool.Context {
	return tool.Context{Context: context.Background(), WorkDir: dir}
}

func TestBashRunsCommand(t *testing.T) {
	out, err := runBash(ctxFor(t.TempDir()), bashArgs{Command: "echo hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("output = %q, want it to contain hello", out)
	}
}

func TestBashRunsInWorkDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runBash(ctxFor(dir), bashArgs{Command: "cat note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "data") {
		t.Fatalf("command did not run in the work dir: %q", out)
	}
}

func TestBashNonZeroExitShowsStatus(t *testing.T) {
	out, err := runBash(ctxFor(t.TempDir()), bashArgs{Command: "echo oops; exit 3"})
	if err != nil {
		t.Fatalf("a failed command should not be a Go error: %v", err)
	}
	if !strings.Contains(out, "oops") || !strings.Contains(out, "exit status 3") {
		t.Fatalf("expected output and exit status, got %q", out)
	}
}

func TestBashTimesOut(t *testing.T) {
	out, err := runBash(ctxFor(t.TempDir()), bashArgs{Command: "sleep 5", TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "timed out") {
		t.Fatalf("expected a timeout note, got %q", out)
	}
}

func TestBashTimeoutKillsChildProcesses(t *testing.T) {
	start := time.Now()
	out, err := runBash(ctxFor(t.TempDir()), bashArgs{Command: "sleep 5 & wait", TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout waited for child process to exit naturally after %s; output %q", elapsed, out)
	}
	if !strings.Contains(out, "timed out") {
		t.Fatalf("expected a timeout note, got %q", out)
	}
}

func TestBashStartsBackgroundCommand(t *testing.T) {
	start := time.Now()
	out, err := runBash(ctxFor(t.TempDir()), bashArgs{Command: "echo ready; sleep 0.1", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("background command did not return promptly after %s", elapsed)
	}
	if !strings.Contains(out, "started background command") || !strings.Contains(out, "log:") {
		t.Fatalf("unexpected background output: %q", out)
	}
	logPath := extractLogPath(t, out)
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected background log file to exist: %v", err)
	}
}

func TestBashSummaryMarksBackground(t *testing.T) {
	got := bashSummary(`{"command":"npm run dev","background":true}`)
	if got != "npm run dev · background" {
		t.Fatalf("summary = %q", got)
	}
}

func TestBashIsNotReadOnly(t *testing.T) {
	if NewBash().ReadOnly() {
		t.Fatal("bash must be non-read-only so it is always gated by approval")
	}
}

func extractLogPath(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "log: ") {
			return strings.TrimPrefix(line, "log: ")
		}
	}
	t.Fatalf("log path not found in output: %q", out)
	return ""
}

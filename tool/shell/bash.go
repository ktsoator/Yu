// Package shell provides a tool for running shell commands. Unlike the file
// tools it is never read-only, so the agent's approver always gates it.
package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/ktsoator/yu/tool"
)

const (
	defaultTimeout = 30 * time.Second
	maxTimeout     = 120 * time.Second
	// maxOutputBytes caps combined output so a chatty command can't blow up the
	// model context.
	maxOutputBytes = 30 * 1024
)

type bashArgs struct {
	Command        string `json:"command" description:"The shell command to run via /bin/sh -c."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" description:"Max seconds the command may run (default 30, max 120)."`
}

// NewBash returns a tool that runs a shell command in the working directory.
// It is not read-only: every call is gated by the agent's approver.
func NewBash() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "bash",
		Description: "Run a shell command and return its combined stdout and stderr.",
		ReadOnly:    false,
		Prompt:      "Runs via /bin/sh -c in the working directory. Prefer focused, non-interactive commands; it is not for long-running or interactive processes.",
	}, runBash)
	if err != nil {
		panic(err)
	}
	return t
}

func runBash(ctx tool.Context, in bashArgs) (string, error) {
	if in.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := defaultTimeout
	if in.TimeoutSeconds > 0 {
		timeout = min(time.Duration(in.TimeoutSeconds)*time.Second, maxTimeout)
	}
	runCtx, cancel := context.WithTimeout(ctx.Context, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", in.Command)
	cmd.Dir = ctx.WorkDir
	out, err := cmd.CombinedOutput()
	text := capOutput(string(out))

	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return appendNote(text, fmt.Sprintf("[timed out after %s]", timeout)), nil
	case err == nil:
		if text == "" {
			return "(no output)", nil
		}
		return text, nil
	default:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// Command ran but failed: show its output and exit code to the model
			// rather than hiding it behind a Go error.
			return appendNote(text, fmt.Sprintf("[exit status %d]", exit.ExitCode())), nil
		}
		// Could not start the command at all (e.g. shell missing).
		return "", err
	}
}

func capOutput(s string) string {
	if len(s) > maxOutputBytes {
		return s[:maxOutputBytes] + "\n...[output truncated]"
	}
	return s
}

func appendNote(text, note string) string {
	if text == "" {
		return note
	}
	return text + "\n" + note
}

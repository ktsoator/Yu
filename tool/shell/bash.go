// Package shell provides a tool for running shell commands. Unlike the file
// tools it is never read-only, so the agent's approver always gates it.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
	Background     bool   `json:"background,omitempty" description:"Start the command in the background and return immediately with a PID and log path. Use for long-running dev servers."`
}

// NewBash returns a tool that runs a shell command in the working directory.
// It is not read-only: every call is gated by the agent's approver.
func NewBash() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "bash",
		Description: "Run a shell command and return its combined stdout and stderr.",
		ReadOnly:    false,
		Prompt:      "Runs via /bin/sh -c in the working directory. Use background=true for long-running dev servers; otherwise prefer focused, non-interactive commands that finish.",
		Summary:     bashSummary,
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
	if in.Background {
		bg, err := startBackgroundShell(ctx, in.Command)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("started background command\npid: %d\nprocess group: -%d\nlog: %s\nstop: kill -TERM -%d", bg.PID, bg.PID, bg.LogPath, bg.PID), nil
	}

	timeout := defaultTimeout
	if in.TimeoutSeconds > 0 {
		timeout = min(time.Duration(in.TimeoutSeconds)*time.Second, maxTimeout)
	}
	out, err, timedOut := runShellCommand(ctx, in.Command, timeout)
	text := capOutput(string(out))

	switch {
	case timedOut:
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

type backgroundProcess struct {
	PID     int
	LogPath string
}

func startBackgroundShell(ctx tool.Context, command string) (backgroundProcess, error) {
	logFile, err := os.CreateTemp("", "yu-bash-*.log")
	if err != nil {
		return backgroundProcess{}, err
	}
	logPath := logFile.Name()
	cleanup := true
	defer func() {
		logFile.Close()
		if cleanup {
			os.Remove(logPath)
		}
	}()

	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = ctx.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return backgroundProcess{}, err
	}
	if err := cmd.Process.Release(); err != nil {
		killProcessGroup(cmd)
		return backgroundProcess{}, err
	}
	cleanup = false
	return backgroundProcess{PID: cmd.Process.Pid, LogPath: logPath}, nil
}

func runShellCommand(ctx tool.Context, command string, timeout time.Duration) ([]byte, error, bool) {
	runCtx, cancel := context.WithTimeout(ctx.Context, timeout)
	defer cancel()

	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = ctx.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, err, false
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return out.Bytes(), err, false
	case <-runCtx.Done():
		killProcessGroup(cmd)
		err := <-done
		return out.Bytes(), err, errors.Is(runCtx.Err(), context.DeadlineExceeded)
	}
}

func bashSummary(args string) string {
	command := jsonStringValue(args, "command")
	if command == "" {
		return ""
	}
	command = strings.ReplaceAll(command, "\n", " ")
	if len(command) > 80 {
		command = command[:79] + "…"
	}
	if jsonBoolValue(args, "background") {
		return command + " · background"
	}
	return command
}

func jsonStringValue(raw, field string) string {
	key := `"` + field + `"`
	i := strings.Index(raw, key)
	if i < 0 {
		return ""
	}
	i += len(key)
	for i < len(raw) && isJSONSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != ':' {
		return ""
	}
	i++
	for i < len(raw) && isJSONSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != '"' {
		return ""
	}
	i++
	var b strings.Builder
	escaped := false
	for ; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			if c == 'n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(c)
			}
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return b.String()
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func jsonBoolValue(raw, field string) bool {
	key := `"` + field + `"`
	i := strings.Index(raw, key)
	if i < 0 {
		return false
	}
	i += len(key)
	for i < len(raw) && (isJSONSpace(raw[i]) || raw[i] == ':') {
		i++
	}
	return strings.HasPrefix(raw[i:], "true")
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
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

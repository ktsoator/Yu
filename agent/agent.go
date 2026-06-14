package agent

import (
	"context"
	"iter"

	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
)

// ToolApprover decides whether a tool call may run. It is consulted only for
// non-read-only tools before execution; returning false rejects the call and
// the rejection is fed back to the model as that tool's result. A nil approver
// means every tool runs unattended.
type ToolApprover func(t tool.Tool, args string) (bool, error)

// Environment produces a dynamic context block (working directory, platform,
// date, git, ...) that is appended to the static Instruction at run time. It is
// evaluated once per invocation. A nil Environment adds no block, keeping the
// system prompt equal to the instruction.
type Environment func(workDir string) string

// Config describes an agent: what model it uses, how it should behave, and
// which tools it may call. Session handling lives in the runner, not here.
type Config struct {
	Name        string
	Description string
	Instruction string
	Model       llm.Model
	Tools       []tool.Tool
	// Approve gates non-read-only tool calls. Leave nil to run every tool
	// without asking (e.g. tests or non-interactive use).
	Approve ToolApprover
	// Environment supplies the dynamic context block appended to Instruction.
	// Leave nil for a purely static system prompt (e.g. tests).
	Environment Environment
}

// InvocationContext carries everything an agent needs for one invocation:
// who is asking, and the session history snapshot to reason over. The last
// event is the user input that triggered this invocation. Agents treat the
// snapshot as read-only; persistence is the runner's job.
type InvocationContext struct {
	InvocationID string
	AppName      string
	UserID       string
	Session      *session.Session
}

// Agent is the reasoning engine: given an invocation context it runs the
// model/tool loop and yields events. Partial events are streaming fragments;
// non-partial events are finished messages for the runner to persist.
type Agent interface {
	Name() string
	Run(ctx context.Context, ictx *InvocationContext) iter.Seq2[*session.Event, error]
	SetModel(m llm.Model)
	SetThinking(on bool)
	Thinking() bool
	SupportsThinking() bool
}

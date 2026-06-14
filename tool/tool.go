package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is a capability the agent can advertise to the model and run locally.
// A tool is one rich object: it describes itself, classifies its own safety,
// and executes.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	// ReadOnly reports whether the tool only observes state. Read-only tools
	// can be auto-approved; non-read-only tools are where confirmation hangs.
	ReadOnly() bool
	Execute(ctx Context, args json.RawMessage) (Result, error)
}

// Context carries invocation-scoped information into a tool execution.
type Context struct {
	context.Context

	AppName      string
	UserID       string
	SessionID    string
	InvocationID string
	WorkDir      string
}

// Result is what a tool produces. For now it is the model-facing text; it is
// kept as a struct so richer projections (e.g. a separate user-facing display
// distinct from the model-facing text) can land here without changing the Tool
// interface.
type Result struct {
	Content string `json:"content,omitempty"`
}

// Text returns the model-facing representation of the result.
func (r Result) Text() string { return r.Content }

// Registry maps tool names to tools for lookup during the agent loop.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry indexes tools by name.
func NewRegistry(tools []Tool) (*Registry, error) {
	reg := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if t == nil {
			return nil, fmt.Errorf("tool registry: nil tool")
		}
		name := t.Name()
		if name == "" {
			return nil, fmt.Errorf("tool registry: tool name is required")
		}
		if _, ok := reg.tools[name]; ok {
			return nil, fmt.Errorf("tool registry: duplicate tool %q", name)
		}
		reg.tools[name] = t
	}
	return reg, nil
}

// Get returns the tool with the given name.
func (r *Registry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

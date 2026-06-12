package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool describes a capability that can be advertised to the model.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
}

// Executable is a Tool that can be invoked locally by the agent loop.
type Executable interface {
	Tool
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

// ReadonlyContext carries invocation-scoped information for dynamic tool lookup.
type ReadonlyContext struct {
	context.Context

	AppName      string
	UserID       string
	SessionID    string
	InvocationID string
	WorkDir      string
}

// Result is the value a tool returns to the model.
type Result struct {
	Content string         `json:"content,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// Text returns the model-facing representation of the result.
func (r Result) Text() string {
	if r.Content != "" {
		return r.Content
	}
	if r.Data == nil {
		return ""
	}
	data, err := json.Marshal(r.Data)
	if err != nil {
		return fmt.Sprintf("error: encode tool result: %v", err)
	}
	return string(data)
}

// Toolset is a dynamic collection of tools.
type Toolset interface {
	Name() string
	Tools(ctx ReadonlyContext) ([]Executable, error)
}

// Registry maps tool names to tools for lookup during the agent loop.
type Registry struct {
	tools map[string]Executable
}

// NewRegistry indexes tools by name.
func NewRegistry(tools []Executable) (*Registry, error) {
	reg := &Registry{tools: make(map[string]Executable, len(tools))}
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
func (r *Registry) Get(name string) (Executable, bool) {
	if r == nil {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

// Tools returns all registered tools.
func (r *Registry) Tools() []Executable {
	if r == nil || len(r.tools) == 0 {
		return nil
	}
	out := make([]Executable, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

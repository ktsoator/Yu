package tool

import (
	"context"
	"encoding/json"
)

// Tool is a single capability the agent can invoke. Schema returns the JSON
// Schema describing the arguments object the model must produce; Execute runs
// the tool with those raw JSON arguments and returns text for the model.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry maps tool names to tools for lookup during the agent loop.
type Registry map[string]Tool

// NewRegistry indexes tools by name.
func NewRegistry(tools []Tool) Registry {
	reg := make(Registry, len(tools))
	for _, t := range tools {
		reg[t.Name()] = t
	}
	return reg
}

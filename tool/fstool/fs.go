package fstool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// maxFileBytes caps read_file output so a huge file can't blow up the context.
const maxFileBytes = 64 * 1024

// ReadFile reads a single file from disk and returns its contents.
type ReadFile struct{}

func NewReadFile() ReadFile { return ReadFile{} }

func (ReadFile) Name() string        { return "read_file" }
func (ReadFile) Description() string { return "Read the contents of a file at the given path." }

func (ReadFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read, relative to the current directory or absolute.",
			},
		},
		"required": []string{"path"},
	}
}

func (ReadFile) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if in.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return "", err
	}
	if len(data) > maxFileBytes {
		return string(data[:maxFileBytes]) + "\n...[truncated]", nil
	}
	return string(data), nil
}

// ListDir lists the entries of a directory.
type ListDir struct{}

func NewListDir() ListDir { return ListDir{} }

func (ListDir) Name() string        { return "list_dir" }
func (ListDir) Description() string { return "List the files and subdirectories in a directory." }

func (ListDir) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list. Defaults to the current directory.",
			},
		},
	}
}

func (ListDir) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	// Arguments may be empty/omitted; default to the current directory.
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	path := in.Path
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "(empty directory)", nil
	}
	return b.String(), nil
}

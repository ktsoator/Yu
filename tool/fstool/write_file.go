package fstool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ktsoator/yu/tool"
)

type writeFileArgs struct {
	Path    string `json:"path" description:"File path to write, relative to the working directory."`
	Content string `json:"content" description:"The full content to write to the file."`
}

// NewWriteFile returns a tool that creates or overwrites a file. It is not
// read-only, so the agent's approver gates it before execution.
func NewWriteFile() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content.",
		ReadOnly:    false,
		Summary:     writeFileSummary,
	}, writeFile)
	if err != nil {
		panic(err)
	}
	return t
}

func writeFile(ctx tool.Context, in writeFileArgs) (string, error) {
	if in.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	p, err := resolveInWorkDir(ctx, in.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(in.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

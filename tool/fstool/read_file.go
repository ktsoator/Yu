package fstool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ktsoator/yu/tool"
)

// maxFileBytes caps read_file output so a huge file can't blow up the context.
const maxFileBytes = 64 * 1024

type readFileArgs struct {
	Path string `json:"path" description:"Path to the file to read, relative to the current directory or absolute."`
}

// NewReadFile returns a tool that reads a single file from disk.
func NewReadFile() tool.Executable {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path.",
	}, readFile)
	if err != nil {
		panic(err)
	}
	return t
}

func readFile(ctx tool.Context, in readFileArgs) (string, error) {
	if in.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(resolvePath(ctx, in.Path))
	if err != nil {
		return "", err
	}
	if len(data) > maxFileBytes {
		return string(data[:maxFileBytes]) + "\n...[truncated]", nil
	}
	return string(data), nil
}

func resolvePath(ctx tool.Context, path string) string {
	if ctx.WorkDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ctx.WorkDir, path)
}

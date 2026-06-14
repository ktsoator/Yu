package fstool

import (
	"fmt"
	"os"
	"strings"

	"github.com/ktsoator/yu/tool"
)

type editFileArgs struct {
	Path       string `json:"path" description:"File to edit, relative to the working directory."`
	OldString  string `json:"old_string" description:"Exact text to replace. Must match the file exactly, including indentation."`
	NewString  string `json:"new_string" description:"Text to replace it with."`
	ReplaceAll bool   `json:"replace_all,omitempty" description:"Replace every occurrence instead of requiring old_string to be unique."`
}

// NewEditFile returns a tool that replaces an exact string in an existing file.
// It is not read-only, so the agent's approver gates it before execution.
func NewEditFile() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "edit_file",
		Description: "Replace an exact string in an existing file. old_string must be unique unless replace_all is set.",
		ReadOnly:    false,
		Prompt:      "old_string must match the file exactly, including whitespace, and be unique; otherwise the edit fails. Set replace_all to change every occurrence.",
		Summary:     editFileSummary,
	}, editFile)
	if err != nil {
		panic(err)
	}
	return t
}

func editFile(ctx tool.Context, in editFileArgs) (string, error) {
	if in.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if in.OldString == "" {
		return "", fmt.Errorf("old_string is required (use write_file to create a file)")
	}
	if in.OldString == in.NewString {
		return "", fmt.Errorf("no change: old_string and new_string are identical")
	}
	p, err := resolveInWorkDir(ctx, in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	content := string(data)

	n := strings.Count(content, in.OldString)
	switch {
	case n == 0:
		return "", fmt.Errorf("old_string not found in %s", in.Path)
	case n > 1 && !in.ReplaceAll:
		return "", fmt.Errorf("old_string is not unique in %s (%d matches): add surrounding context or set replace_all", in.Path, n)
	}

	updated := content
	if in.ReplaceAll {
		updated = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		updated = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d occurrence(s) in %s", n, in.Path), nil
}

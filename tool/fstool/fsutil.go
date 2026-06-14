package fstool

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ktsoator/yu/tool"
)

// resolveInWorkDir resolves path against the working directory and refuses to
// escape it. Write tools use this so a model can't reach outside the project
// through an absolute path or "..". With no working directory configured it is
// a no-op.
func resolveInWorkDir(ctx tool.Context, path string) (string, error) {
	if ctx.WorkDir == "" {
		return path, nil
	}
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(ctx.WorkDir, p)
	}
	p = filepath.Clean(p)
	rel, err := filepath.Rel(ctx.WorkDir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the working directory", path)
	}
	return p, nil
}

// skipDir reports directories that are almost never useful to walk and would
// otherwise dominate both output and walk time.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules":
		return true
	}
	return false
}

// relTo renders an absolute path relative to the working directory for display,
// falling back to the original path when that isn't possible.
func relTo(ctx tool.Context, path string) string {
	if ctx.WorkDir == "" {
		return path
	}
	if rel, err := filepath.Rel(ctx.WorkDir, path); err == nil {
		return rel
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

package fstool

import (
	"path/filepath"

	"github.com/ktsoator/yu/tool"
)

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

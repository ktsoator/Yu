package fstool

import (
	"os"
	"strings"

	"github.com/ktsoator/yu/tool"
)

type listDirArgs struct {
	Path string `json:"path,omitempty" description:"Directory path to list. Defaults to the current directory."`
}

// NewListDir returns a tool that lists the entries of a directory.
func NewListDir() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "list_dir",
		Description: "List the files and subdirectories in a directory.",
		ReadOnly:    true,
	}, listDir)
	if err != nil {
		panic(err)
	}
	return t
}

func listDir(ctx tool.Context, in listDirArgs) (string, error) {
	path := in.Path
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(resolvePath(ctx, path))
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

package fstool

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ktsoator/yu/tool"
)

const (
	// maxGrepMatches caps how many matching lines grep returns so a broad
	// pattern over a big tree can't blow up the model context.
	maxGrepMatches = 200
	// maxGrepLineLen caps the width of a single reported line.
	maxGrepLineLen = 300
)

type grepArgs struct {
	Pattern string `json:"pattern" description:"Regular expression (RE2 syntax) to search for in file contents."`
	Path    string `json:"path,omitempty" description:"File or directory to search. Defaults to the working directory."`
}

// NewGrep returns a read-only tool that searches file contents by regexp.
func NewGrep() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "grep",
		Description: "Search file contents for a regular expression. Returns matching lines as path:line: text.",
		ReadOnly:    true,
	}, grep)
	if err != nil {
		panic(err)
	}
	return t
}

func grep(ctx tool.Context, in grepArgs) (string, error) {
	if in.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	root := resolvePath(ctx, firstNonEmpty(in.Path, "."))

	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}

	var lines []string
	full := false
	search := func(path string) {
		if full {
			return
		}
		lines = grepFile(ctx, re, path, lines)
		if len(lines) >= maxGrepMatches {
			full = true
			lines = lines[:maxGrepMatches]
		}
	}

	if info.IsDir() {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries rather than abort
			}
			if d.IsDir() {
				if skipDir(d.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			search(path)
			if full {
				return fs.SkipAll
			}
			return nil
		})
	} else {
		search(root)
	}

	if len(lines) == 0 {
		return "no matches", nil
	}
	out := strings.Join(lines, "\n")
	if full {
		out += fmt.Sprintf("\n...[truncated at %d matches]", maxGrepMatches)
	}
	return out, nil
}

// grepFile scans one file and appends "path:line: text" for each matching line,
// stopping once the global cap is reached. Binary files are skipped.
func grepFile(ctx tool.Context, re *regexp.Regexp, path string, lines []string) []string {
	f, err := os.Open(path)
	if err != nil {
		return lines
	}
	defer f.Close()

	rel := relTo(ctx, path)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	num := 0
	for sc.Scan() {
		num++
		line := sc.Text()
		if strings.IndexByte(line, 0) >= 0 {
			return lines // looks binary; abandon this file
		}
		if re.MatchString(line) {
			lines = append(lines, fmt.Sprintf("%s:%d: %s", rel, num, truncLine(line)))
			if len(lines) >= maxGrepMatches {
				return lines
			}
		}
	}
	return lines
}

func truncLine(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) > maxGrepLineLen {
		return s[:maxGrepLineLen] + "..."
	}
	return s
}

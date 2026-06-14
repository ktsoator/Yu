package fstool

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ktsoator/yu/tool"
)

// maxGlobEntries caps how many paths glob returns.
const maxGlobEntries = 500

type globArgs struct {
	Pattern string `json:"pattern" description:"Glob pattern: '*' matches within a path segment, '**' matches across segments, '?' matches one character. A pattern with no '/' matches by file name anywhere (e.g. '*.go')."`
	Path    string `json:"path,omitempty" description:"Directory to search under. Defaults to the working directory."`
}

// NewGlob returns a read-only tool that finds files by path pattern.
func NewGlob() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "glob",
		Description: "Find files whose path matches a glob pattern. Returns matching file paths.",
		ReadOnly:    true,
	}, glob)
	if err != nil {
		panic(err)
	}
	return t
}

func glob(ctx tool.Context, in globArgs) (string, error) {
	if in.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	re, err := globToRegexp(in.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	// A pattern with no separator matches against the file name anywhere in the
	// tree, so "*.go" finds every Go file rather than only top-level ones.
	byName := !strings.Contains(in.Pattern, "/")
	root := resolvePath(ctx, firstNonEmpty(in.Path, "."))

	var hits []string
	truncated := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		rel := relTo(ctx, path)
		target := rel
		if byName {
			target = d.Name()
		}
		if re.MatchString(target) {
			hits = append(hits, rel)
			if len(hits) >= maxGlobEntries {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})

	if len(hits) == 0 {
		return "no files match", nil
	}
	sort.Strings(hits)
	out := strings.Join(hits, "\n")
	if truncated {
		out += fmt.Sprintf("\n...[truncated at %d files]", maxGlobEntries)
	}
	return out, nil
}

// globToRegexp translates a glob pattern into an anchored regexp. '*' stays
// within a path segment, '**' spans segments, '?' matches one non-separator.
func globToRegexp(pat string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; c {
		case '*':
			if i+1 < len(pat) && pat[i+1] == '*' {
				i++
				b.WriteString(".*")
				// Treat "**/" as zero-or-more leading segments.
				if i+1 < len(pat) && pat[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('$')
	return regexp.Compile(b.String())
}

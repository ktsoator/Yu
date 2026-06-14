package fstool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktsoator/yu/tool"
)

type applyPatchArgs struct {
	Patch string `json:"patch" description:"Patch text using Begin Patch / Add File / Update File / Delete File blocks."`
}

type patchOp struct {
	kind  string
	path  string
	hunks [][]patchLine
	lines []string
}

type patchLine struct {
	op   byte
	text string
}

// NewApplyPatch returns a write tool that applies structured patch text. It is
// useful for multi-line or multi-file edits where a diff is clearer than an
// exact old_string/new_string replacement.
func NewApplyPatch() tool.Tool {
	t, err := tool.NewFunction(tool.FunctionConfig{
		Name:        "apply_patch",
		Description: "Apply a Begin Patch style patch to one or more files.",
		ReadOnly:    false,
		Prompt:      "Use apply_patch for multi-line edits, repeated edits, or changes spanning multiple files. Use edit_file for small exact replacements and write_file for creating or replacing a whole file.",
		Summary:     applyPatchSummary,
	}, applyPatch)
	if err != nil {
		panic(err)
	}
	return t
}

func applyPatch(ctx tool.Context, in applyPatchArgs) (string, error) {
	if strings.TrimSpace(in.Patch) == "" {
		return "", fmt.Errorf("patch is required")
	}
	ops, err := parsePatch(in.Patch)
	if err != nil {
		return "", err
	}
	var changed []string
	for _, op := range ops {
		switch op.kind {
		case "add":
			if err := applyAddFile(ctx, op); err != nil {
				return "", err
			}
		case "update":
			if err := applyUpdateFile(ctx, op); err != nil {
				return "", err
			}
		case "delete":
			if err := applyDeleteFile(ctx, op); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unknown patch operation %q", op.kind)
		}
		changed = append(changed, op.path)
	}
	added, removed := patchLineStats(in.Patch)
	return fmt.Sprintf("applied patch to %d file(s) (+%d -%d): %s", len(changed), added, removed, strings.Join(changed, ", ")), nil
}

func applyAddFile(ctx tool.Context, op patchOp) error {
	p, err := resolveInWorkDir(ctx, op.path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("add file: %s already exists", op.path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(linesToContent(op.lines)), 0o644)
}

func applyUpdateFile(ctx tool.Context, op patchOp) error {
	p, err := resolveInWorkDir(ctx, op.path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	content := string(data)
	cursor := 0
	for _, hunk := range op.hunks {
		oldBlock, newBlock := hunkBlocks(hunk)
		if oldBlock == "" {
			return fmt.Errorf("update file: empty old block in %s", op.path)
		}
		idx := strings.Index(content[cursor:], oldBlock)
		if idx < 0 {
			return fmt.Errorf("update file: hunk not found in %s", op.path)
		}
		start := cursor + idx
		content = content[:start] + newBlock + content[start+len(oldBlock):]
		cursor = start + len(newBlock)
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

func applyDeleteFile(ctx tool.Context, op patchOp) error {
	p, err := resolveInWorkDir(ctx, op.path)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

func parsePatch(patch string) ([]patchOp, error) {
	lines := strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return nil, fmt.Errorf("patch must start with *** Begin Patch")
	}
	var ops []patchOp
	for i := 1; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			i++
		case line == "*** End Patch":
			return ops, nil
		case strings.HasPrefix(line, "*** Add File: "):
			op, next, err := parseAddFile(lines, i)
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			i = next
		case strings.HasPrefix(line, "*** Update File: "):
			op, next, err := parseUpdateFile(lines, i)
			if err != nil {
				return nil, err
			}
			ops = append(ops, op)
			i = next
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path == "" {
				return nil, fmt.Errorf("delete file path is required")
			}
			ops = append(ops, patchOp{kind: "delete", path: path})
			i++
		default:
			return nil, fmt.Errorf("unexpected patch line: %s", lines[i])
		}
	}
	return nil, fmt.Errorf("patch must end with *** End Patch")
}

func parseAddFile(lines []string, start int) (patchOp, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "*** Add File: "))
	if path == "" {
		return patchOp{}, start, fmt.Errorf("add file path is required")
	}
	op := patchOp{kind: "add", path: path}
	i := start + 1
	for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "*** ") {
		if lines[i] == "" && i == len(lines)-1 {
			break
		}
		if !strings.HasPrefix(lines[i], "+") {
			return patchOp{}, start, fmt.Errorf("add file %s: expected + line", path)
		}
		op.lines = append(op.lines, strings.TrimPrefix(lines[i], "+"))
		i++
	}
	return op, i, nil
}

func parseUpdateFile(lines []string, start int) (patchOp, int, error) {
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "*** Update File: "))
	if path == "" {
		return patchOp{}, start, fmt.Errorf("update file path is required")
	}
	op := patchOp{kind: "update", path: path}
	i := start + 1
	var current []patchLine
	for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "*** ") {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			if len(current) > 0 {
				op.hunks = append(op.hunks, current)
				current = nil
			}
			i++
			continue
		}
		if line == "" {
			current = append(current, patchLine{op: ' ', text: ""})
			i++
			continue
		}
		switch line[0] {
		case ' ', '-', '+':
			current = append(current, patchLine{op: line[0], text: line[1:]})
		default:
			return patchOp{}, start, fmt.Errorf("update file %s: expected context, - or + line", path)
		}
		i++
	}
	if len(current) > 0 {
		op.hunks = append(op.hunks, current)
	}
	if len(op.hunks) == 0 {
		return patchOp{}, start, fmt.Errorf("update file %s: no hunks", path)
	}
	return op, i, nil
}

func hunkBlocks(hunk []patchLine) (string, string) {
	var oldLines, newLines []string
	for _, l := range hunk {
		switch l.op {
		case ' ':
			oldLines = append(oldLines, l.text)
			newLines = append(newLines, l.text)
		case '-':
			oldLines = append(oldLines, l.text)
		case '+':
			newLines = append(newLines, l.text)
		}
	}
	return linesToContent(oldLines), linesToContent(newLines)
}

func linesToContent(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

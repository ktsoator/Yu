package fstool

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

func writeFileSummary(args string) string {
	path := jsonStringValue(args, "path")
	lines := jsonStringLineCount(args, "content")
	switch {
	case path != "" && lines > 0:
		return fmt.Sprintf("%s · %d %s", path, lines, lineWord(lines))
	case path != "":
		return path
	case lines > 0:
		return fmt.Sprintf("%d %s", lines, lineWord(lines))
	default:
		return ""
	}
}

func editFileSummary(args string) string {
	path := jsonStringValue(args, "path")
	oldLines := jsonStringLineCount(args, "old_string")
	newLines := jsonStringLineCount(args, "new_string")
	action := "replace 1 block"
	if gjson.Get(args, "replace_all").Bool() {
		action = "replace all"
	}

	var parts []string
	if path != "" {
		parts = append(parts, path)
	}
	parts = append(parts, action)
	if oldLines > 0 || newLines > 0 {
		parts = append(parts, fmt.Sprintf("+%d -%d %s", newLines, oldLines, lineWord(max(oldLines, newLines))))
	}
	return strings.Join(parts, " · ")
}

func jsonStringValue(raw, field string) string {
	if value := gjson.Get(raw, field); value.Exists() {
		return value.String()
	}
	value, _ := partialJSONStringValue(raw, field)
	return value
}

func jsonStringLineCount(raw, field string) int {
	if value := gjson.Get(raw, field); value.Exists() {
		return countLines(value.String())
	}
	value, ok := partialJSONStringValue(raw, field)
	if !ok || value == "" {
		return 0
	}
	return countEncodedLines(value)
}

func partialJSONStringValue(raw, field string) (string, bool) {
	key := `"` + field + `"`
	i := strings.Index(raw, key)
	if i < 0 {
		return "", false
	}
	i += len(key)
	for i < len(raw) && isJSONSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != ':' {
		return "", false
	}
	i++
	for i < len(raw) && isJSONSpace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != '"' {
		return "", false
	}
	i++

	var b strings.Builder
	escaped := false
	for ; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			b.WriteByte('\\')
			b.WriteByte(c)
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return b.String(), true
		default:
			b.WriteByte(c)
		}
	}
	if escaped {
		b.WriteByte('\\')
	}
	return b.String(), true
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

func countEncodedLines(s string) int {
	if s == "" {
		return 0
	}
	lines := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == 'n' {
			lines++
			i++
			continue
		}
		if s[i] == '\n' {
			lines++
		}
	}
	if !strings.HasSuffix(s, `\n`) && !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

func lineWord(n int) string {
	if n == 1 {
		return "line"
	}
	return "lines"
}

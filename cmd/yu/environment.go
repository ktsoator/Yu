package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// defaultEnvironment builds the dynamic <env> block appended to the agent's
// static instruction: where it is running, the platform, today's date, and the
// current git branch. Git is read straight from .git/HEAD to avoid spawning a
// subprocess.
func defaultEnvironment(workDir string) string {
	var b strings.Builder
	b.WriteString("<env>\n")
	if workDir != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", workDir)
	}
	fmt.Fprintf(&b, "Platform: %s\n", runtime.GOOS)
	fmt.Fprintf(&b, "Today's date: %s\n", time.Now().Format("2006-01-02"))
	if branch, ok := gitBranch(workDir); ok {
		fmt.Fprintf(&b, "Git repo: yes (branch: %s)\n", branch)
	} else {
		b.WriteString("Git repo: no\n")
	}
	b.WriteString("</env>")
	return b.String()
}

// gitBranch reads the current branch from workDir/.git/HEAD without running git.
// A symbolic ref ("ref: refs/heads/main") yields the branch name; a detached
// HEAD yields the short commit hash. It only inspects workDir itself, not parent
// directories.
func gitBranch(workDir string) (string, bool) {
	if workDir == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(workDir, ".git", "HEAD"))
	if err != nil {
		return "", false
	}
	head := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return ref, true
	}
	if len(head) >= 7 {
		return head[:7], true // detached HEAD
	}
	return "", false
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitBranchReadsHEAD(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	branch, ok := gitBranch(dir)
	if !ok || branch != "main" {
		t.Fatalf("gitBranch = %q, %v; want main, true", branch, ok)
	}
}

func TestGitBranchDetachedHead(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("0123456789abcdef\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	branch, ok := gitBranch(dir)
	if !ok || branch != "0123456" {
		t.Fatalf("gitBranch = %q, %v; want 0123456, true", branch, ok)
	}
}

func TestGitBranchNonRepo(t *testing.T) {
	if _, ok := gitBranch(t.TempDir()); ok {
		t.Fatal("expected a non-git directory to report false")
	}
}

func TestDefaultEnvironmentContents(t *testing.T) {
	block := defaultEnvironment(t.TempDir())
	for _, want := range []string{"<env>", "Platform:", "Today's date:", "Git repo:", "</env>"} {
		if !strings.Contains(block, want) {
			t.Fatalf("env block missing %q:\n%s", want, block)
		}
	}
}

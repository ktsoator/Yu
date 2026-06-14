package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestConfirmWorkspaceTrustDefaultsToYes(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	if !confirmWorkspaceTrust(scanner, "/tmp/project") {
		t.Fatal("empty input should trust the workspace")
	}
}

func TestConfirmWorkspaceTrustRejectsNo(t *testing.T) {
	for _, input := range []string{"2\n", "n\n", "no\n"} {
		scanner := bufio.NewScanner(strings.NewReader(input))
		if confirmWorkspaceTrust(scanner, "/tmp/project") {
			t.Fatalf("input %q should reject the workspace", input)
		}
	}
}

func TestClearTerminal(t *testing.T) {
	var b bytes.Buffer
	clearTerminalTo(&b)
	if got := b.String(); got != "\033[H\033[2J" {
		t.Fatalf("clear sequence = %q", got)
	}
}

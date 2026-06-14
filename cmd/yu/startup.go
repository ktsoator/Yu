package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func confirmWorkspaceTrust(scanner *bufio.Scanner, workDir string) bool {
	printWorkspaceTrustPrompt(workDir)
	if !scanner.Scan() {
		return false
	}
	input := strings.ToLower(strings.TrimSpace(scanner.Text()))
	switch input {
	case "", "1", "y", "yes":
		return true
	default:
		return false
	}
}

func printWorkspaceTrustPrompt(workDir string) {
	fmt.Println("\033[33m────────────────────────────────────────────────────────────────────────\033[0m")
	fmt.Println("\033[33mAccessing workspace:\033[0m")
	fmt.Println()
	fmt.Printf("\033[1m%s\033[0m\n", workDir)
	fmt.Println()
	fmt.Println("Quick safety check: Is this a project you created or one you trust?")
	fmt.Println("Yu will be able to read, edit, and execute files here.")
	fmt.Println()
	fmt.Println("\033[90mSecurity guide\033[0m")
	fmt.Println()
	fmt.Println("\033[36m›\033[0m 1. Yes, I trust this folder")
	fmt.Println("  2. No, exit")
	fmt.Println()
	fmt.Print("\033[90mEnter to confirm · type 2 to exit\033[0m ")
}

func printStartupModelHeader() {
	fmt.Println()
	fmt.Println("\033[33mSelect model:\033[0m")
}

func clearTerminal() {
	clearTerminalTo(os.Stdout)
}

func clearTerminalTo(w io.Writer) {
	fmt.Fprint(w, "\033[H\033[2J")
}

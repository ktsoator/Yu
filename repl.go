package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/llm"
)

type repl struct {
	ctx     context.Context
	scanner *bufio.Scanner
	models  []modelConfig
}

func newREPL(ctx context.Context, in io.Reader, models []modelConfig) *repl {
	return &repl{
		ctx:     ctx,
		scanner: bufio.NewScanner(in),
		models:  models,
	}
}

// Main REPL: slash commands are handled locally; normal input goes through
// the agent and streams chunks back into this terminal callback.
func (r *repl) run(ag agent.Agent) error {
	for {
		fmt.Print("\nyou › ")
		if !r.scanner.Scan() {
			break
		}
		input := strings.TrimSpace(r.scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/exit", "/quit":
			return nil
		case "/model":
			r.switchModel(ag)
			continue
		case "/think":
			toggleThinking(ag)
			continue
		}

		if err := r.send(ag, input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

func (r *repl) switchModel(ag agent.Agent) {
	mc := selectModel(r.models, r.scanner)
	model, err := buildModel(mc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	ag.SetModel(model)
	fmt.Printf("Switched to %s (thinking %s)\n", model.Name(), onOff(ag.Thinking()))
}

func toggleThinking(ag agent.Agent) {
	if !ag.SupportsThinking() {
		fmt.Println("Thinking is not supported by the current model configuration.")
		return
	}
	ag.SetThinking(!ag.Thinking())
	fmt.Printf("Thinking: %s\n", onOff(ag.Thinking()))
}

func (r *repl) send(ag agent.Agent, input string) error {
	// Track the transition from reasoning deltas to final content so the
	// terminal color is reset exactly once when the answer starts.
	var inReasoning, inContent bool
	_, err := ag.Send(r.ctx, input, func(ch llm.Chunk) {
		if ch.Reasoning != "" {
			if !inReasoning {
				fmt.Print("\033[90m[reasoning]\n")
				inReasoning = true
			}
			fmt.Print(ch.Reasoning)
		}
		if ch.Content != "" {
			if inReasoning && !inContent {
				fmt.Print("\033[0m\n") // close the gray reasoning block
			}
			inContent = true
			fmt.Print(ch.Content)
		}
	})
	if inReasoning && !inContent {
		fmt.Print("\033[0m") // reset color when there is reasoning but no content
	}
	fmt.Println()
	return err
}

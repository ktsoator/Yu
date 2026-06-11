package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/weblog"
)

// logViewerAddr is the address the /logs web viewer listens on.
const logViewerAddr = "127.0.0.1:8090"

type repl struct {
	ctx        context.Context
	scanner    *bufio.Scanner
	models     []modelConfig
	renderer   func() render.Renderer
	viewerOnce sync.Once
}

func newREPL(ctx context.Context, in io.Reader, models []modelConfig) *repl {
	return &repl{
		ctx:      ctx,
		scanner:  bufio.NewScanner(in),
		models:   models,
		renderer: func() render.Renderer { return clirender.New() },
	}
}

// Main REPL: slash commands are handled locally; normal input goes through
// the agent and streams chunks back into this terminal callback.
func (r *repl) run(agent agent.Agent) error {
	for {
		fmt.Print("\nYu › ")
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
			r.switchModel(agent)
			continue
		case "/think":
			toggleThinking(agent)
			continue
		case "/logs":
			r.startViewer()
			continue
		}

		if err := r.send(agent, input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

// startViewer launches the log web viewer once and prints its URL. Subsequent
// /logs calls just reprint the URL.
func (r *repl) startViewer() {
	url := "http://" + logViewerAddr
	r.viewerOnce.Do(func() {
		go func() {
			if err := weblog.Serve(logViewerAddr); err != nil {
				fmt.Fprintf(os.Stderr, "log viewer: %v\n", err)
			}
		}()
	})
	fmt.Printf("Log viewer: %s  (logs in %s)\n", url, weblog.Dir())
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

func (r *repl) send(agent agent.Agent, input string) error {
	renderer := r.renderer()
	for ev, err := range agent.Run(r.ctx, input) {
		if err != nil {
			renderer.Finish()
			return err
		}
		renderer.OnEvent(ev)
	}
	renderer.Finish()
	return nil
}

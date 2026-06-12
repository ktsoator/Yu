package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/llm/openai"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/runner"
	"github.com/ktsoator/yu/session"
)

type repl struct {
	ctx              context.Context
	scanner          *bufio.Scanner
	models           []config.Model
	appName          string
	userID           string
	agent            agent.Agent
	runner           *runner.Runner
	sessions         session.Service
	sessionID        string
	currentModelName string
	renderer         func() render.Renderer
}

type replConfig struct {
	Context   context.Context
	Scanner   *bufio.Scanner
	Models    []config.Model
	AppName   string
	UserID    string
	Agent     agent.Agent
	Runner    *runner.Runner
	Sessions  session.Service
	ModelName string
	Renderer  func() render.Renderer
}

func newREPL(cfg replConfig) *repl {
	renderer := cfg.Renderer
	if renderer == nil {
		renderer = func() render.Renderer { return clirender.New() }
	}
	return &repl{
		ctx:              cfg.Context,
		scanner:          cfg.Scanner,
		models:           cfg.Models,
		appName:          cfg.AppName,
		userID:           cfg.UserID,
		agent:            cfg.Agent,
		runner:           cfg.Runner,
		sessions:         cfg.Sessions,
		currentModelName: cfg.ModelName,
		renderer:         renderer,
	}
}

// Main REPL: slash commands are handled locally; normal input goes through
// the runner and streams events back into this terminal callback.
func (r *repl) run() error {
	for {
		input, ok := r.readInput()
		if !ok {
			break
		}
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if r.handleCommand(input) {
				continue
			}
			return nil
		}

		if err := r.send(input); err != nil {
			printError(err)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

func (r *repl) readInput() (string, bool) {
	fmt.Print(r.prompt())
	if !r.scanner.Scan() {
		return "", false
	}

	lines := []string{r.scanner.Text()}
	for hasContinuation(lines[len(lines)-1]) {
		lines[len(lines)-1] = strings.TrimSuffix(strings.TrimRight(lines[len(lines)-1], " \t"), "\\")
		fmt.Print("... ")
		if !r.scanner.Scan() {
			break
		}
		lines = append(lines, r.scanner.Text())
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), true
}

func hasContinuation(s string) bool {
	return strings.HasSuffix(strings.TrimRight(s, " \t"), "\\")
}

func (r *repl) prompt() string {
	return fmt.Sprintf("\n\033[1;36mYu\033[0m \033[90m%s %s\033[0m › ",
		shortModelName(r.currentModelName),
		"think:"+onOff(r.agent.Thinking()),
	)
}

func (r *repl) handleCommand(input string) bool {
	cmd, arg, _ := strings.Cut(input, " ")
	arg = strings.TrimSpace(arg)

	switch cmd {
	case "/exit", "/quit":
		return false
	case "/help":
		r.printHelp()
	case "/status":
		r.printStatus()
	case "/model":
		r.switchModel(arg)
	case "/think":
		r.setThinking(arg)
	case "/new":
		r.newSession()
	case "/sessions":
		r.printSessions()
	case "/history":
		r.printHistory()
	case "/session":
		r.switchSession(arg)
	default:
		fmt.Printf("Unknown command: %s. Try /help.\n", cmd)
	}
	return true
}

func (r *repl) send(input string) error {
	// Ctrl+C while a turn is running cancels just that turn: the signal
	// context aborts the in-flight model request and the REPL returns to the
	// prompt. Once stop() restores default handling, Ctrl+C at the prompt
	// exits the process as usual.
	ctx, stop := signal.NotifyContext(r.ctx, os.Interrupt)
	defer stop()

	if err := r.ensureSession(ctx); err != nil {
		return err
	}
	renderer := r.renderer()
	for ev, err := range r.runner.Run(ctx, r.userID, r.sessionID, input) {
		if err != nil {
			renderer.Finish()
			if errors.Is(err, context.Canceled) {
				fmt.Println("(interrupted)")
				return nil
			}
			return err
		}
		renderer.OnEvent(ev)
	}
	renderer.Finish()
	return nil
}

func (r *repl) ensureSession(ctx context.Context) error {
	if r.sessionID != "" {
		return nil
	}
	resp, err := r.sessions.Create(ctx, &session.CreateRequest{
		AppName: r.appName,
		UserID:  r.userID,
	})
	if err != nil {
		return err
	}
	r.sessionID = resp.Session.ID
	return nil
}

func (r *repl) printHistory() {
	if r.sessionID == "" {
		fmt.Println("No session yet. Send a message or use /new to create one.")
		return
	}
	resp, err := r.sessions.Get(r.ctx, &session.GetRequest{
		AppName:   r.appName,
		UserID:    r.userID,
		SessionID: r.sessionID,
	})
	if err != nil {
		printError(err)
		return
	}
	sess := resp.Session
	fmt.Printf("Session: %s  User: %s  Events: %d\n", sess.ID, sess.UserID, len(sess.Events))
	for i, ev := range sess.Events {
		fmt.Printf(" %2d. %-9s %s\n", i+1, historyRole(ev), historyText(ev))
		for _, tc := range ev.Message.ToolCalls {
			fmt.Printf("     ↳ tool %s %s\n", tc.Name, truncate(tc.Arguments, 160))
		}
	}
}

func (r *repl) newSession() {
	resp, err := r.sessions.Create(r.ctx, &session.CreateRequest{
		AppName: r.appName,
		UserID:  r.userID,
	})
	if err != nil {
		printError(err)
		return
	}
	r.sessionID = resp.Session.ID
	fmt.Printf("New session: %s\n", resp.Session.ID)
}

func (r *repl) switchSession(sessionID string) {
	if sessionID == "" {
		fmt.Println("Usage: /session <id>")
		return
	}
	resp, err := r.sessions.Get(r.ctx, &session.GetRequest{
		AppName:   r.appName,
		UserID:    r.userID,
		SessionID: sessionID,
	})
	if err != nil {
		printError(err)
		return
	}
	r.sessionID = resp.Session.ID
	fmt.Printf("Switched session: %s  Events: %d\n", resp.Session.ID, len(resp.Session.Events))
}

func (r *repl) printSessions() {
	resp, err := r.sessions.List(r.ctx, &session.ListRequest{
		AppName: r.appName,
		UserID:  r.userID,
	})
	if err != nil {
		printError(err)
		return
	}
	sessions := resp.Sessions
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	fmt.Printf("Sessions: %d\n", len(sessions))
	for _, sess := range sessions {
		marker := " "
		if sess.ID == r.sessionID {
			marker = "*"
		}
		fmt.Printf(" %s %s  events=%d  updated=%s\n", marker, sess.ID, len(sess.Events), sess.UpdatedAt.Format("15:04:05"))
	}
}

func historyRole(ev session.Event) string {
	if ev.Type == session.EventError {
		return "error"
	}
	msg := ev.Message
	if msg.Name != "" {
		return string(msg.Role) + ":" + msg.Name
	}
	return string(msg.Role)
}

func historyText(ev session.Event) string {
	if ev.Error != "" {
		return truncate(ev.Error, 220)
	}
	msg := ev.Message
	if msg.ToolCallID != "" && msg.Content != "" {
		return fmt.Sprintf("#%s %s", msg.ToolCallID, truncate(msg.Content, 180))
	}
	if msg.Content != "" {
		return truncate(msg.Content, 220)
	}
	if msg.Reasoning != "" {
		return "[reasoning] " + truncate(msg.Reasoning, 180)
	}
	if len(msg.ToolCalls) > 0 {
		return fmt.Sprintf("%d tool call(s)", len(msg.ToolCalls))
	}
	return "(empty)"
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func (r *repl) switchModel(spec string) {
	var mc config.Model
	if spec == "" {
		mc = selectModel(r.models, r.scanner)
	} else {
		var ok bool
		mc, ok = findModel(r.models, spec)
		if !ok {
			fmt.Printf("Model not found: %s\n", spec)
			r.printModels()
			return
		}
	}
	apiKey := os.Getenv(mc.APIKeyEnv)
	if apiKey == "" {
		printError(fmt.Errorf("missing API key: set %s in your environment or ~/.yu/.env", mc.APIKeyEnv))
		return
	}
	model := openai.New(openai.Config{
		APIKey:           apiKey,
		BaseURL:          mc.BaseURL,
		Model:            mc.Model,
		SupportsThinking: mc.SupportsThinking,
		ThinkingStyle:    mc.ThinkingStyle,
		ReasoningPath:    mc.ReasoningPath,
	})
	r.agent.SetModel(model)
	r.currentModelName = model.Name()
	fmt.Printf("Switched to %s (thinking %s)\n", model.Name(), onOff(r.agent.Thinking()))
}

func (r *repl) setThinking(spec string) {
	if !r.agent.SupportsThinking() {
		fmt.Println("Thinking is not supported by the current model configuration.")
		return
	}
	switch spec {
	case "", "toggle":
		r.agent.SetThinking(!r.agent.Thinking())
	case "on":
		r.agent.SetThinking(true)
	case "off":
		r.agent.SetThinking(false)
	default:
		fmt.Println("Usage: /think [on|off]")
		return
	}
	fmt.Printf("Thinking: %s\n", onOff(r.agent.Thinking()))
}

func (r *repl) printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /help                 show this help")
	fmt.Println("  /status               show current model, thinking mode, and session")
	fmt.Println("  /model [name|number]  switch model; omit the argument for a picker")
	fmt.Println("  /think [on|off]       toggle or set thinking mode")
	fmt.Println("  /new                  start a new session")
	fmt.Println("  /sessions             list sessions")
	fmt.Println("  /session <id>         switch session")
	fmt.Println("  /history              print current session history")
	fmt.Println("  /exit                 quit")
	fmt.Println()
	fmt.Println("End a line with \\ to continue input on the next line.")
}

func (r *repl) printStatus() {
	if r.sessionID == "" {
		fmt.Printf("Model: %s\n", r.currentModelName)
		fmt.Printf("Thinking: %s\n", onOff(r.agent.Thinking()))
		fmt.Println("Session: none yet")
		return
	}
	resp, err := r.sessions.Get(r.ctx, &session.GetRequest{
		AppName:   r.appName,
		UserID:    r.userID,
		SessionID: r.sessionID,
	})
	if err != nil {
		printError(err)
		return
	}
	fmt.Printf("Model: %s\n", r.currentModelName)
	fmt.Printf("Thinking: %s\n", onOff(r.agent.Thinking()))
	fmt.Printf("Session: %s  Events: %d\n", resp.Session.ID, len(resp.Session.Events))
}

func (r *repl) printModels() {
	fmt.Println("Models:")
	for i, m := range r.models {
		marker := " "
		if m.Model == r.currentModelName || m.Name == r.currentModelName {
			marker = "*"
		}
		fmt.Printf(" %s %d) %-10s %s\n", marker, i+1, m.Name, m.Model)
	}
}

func shortModelName(name string) string {
	if name == "" {
		return "?"
	}
	return truncateMiddle(name, 24)
}

func truncateMiddle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	separator := "..."
	available := max - len(separator)
	left := available / 2
	right := available - left
	return s[:left] + separator + s[len(s)-right:]
}

func printError(err error) {
	fmt.Fprintf(os.Stderr, "\033[31merror:\033[0m %v\n", err)
}

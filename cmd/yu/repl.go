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

	"github.com/ktsoator/yu"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/session"
)

type repl struct {
	ctx       context.Context
	scanner   *bufio.Scanner
	models    []config.Model
	app       *yu.App
	sessionID string
	renderer  func() render.Renderer
}

func newREPL(ctx context.Context, scanner *bufio.Scanner, models []config.Model, app *yu.App, sessionID string) *repl {
	return &repl{
		ctx:       ctx,
		scanner:   scanner,
		models:    models,
		app:       app,
		sessionID: sessionID,
		renderer:  func() render.Renderer { return clirender.New() },
	}
}

// Main REPL: slash commands are handled locally; normal input goes through
// the runner and streams events back into this terminal callback.
func (r *repl) run() error {
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
			r.switchModel()
			continue
		case "/think":
			r.toggleThinking()
			continue
		case "/new":
			r.newSession()
			continue
		case "/sessions":
			r.printSessions()
			continue
		case "/history":
			r.printHistory()
			continue
		case "/session":
			fmt.Println("Usage: /session <id>")
			continue
		}
		if id, ok := strings.CutPrefix(input, "/session "); ok {
			r.switchSession(strings.TrimSpace(id))
			continue
		}

		if err := r.send(input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

func (r *repl) send(input string) error {
	// Ctrl+C while a turn is running cancels just that turn: the signal
	// context aborts the in-flight model request and the REPL returns to the
	// prompt. Once stop() restores default handling, Ctrl+C at the prompt
	// exits the process as usual.
	ctx, stop := signal.NotifyContext(r.ctx, os.Interrupt)
	defer stop()

	renderer := r.renderer()
	for ev, err := range r.app.Runner.Run(ctx, yu.DefaultUserID, r.sessionID, input) {
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

func (r *repl) printHistory() {
	resp, err := r.app.Sessions.Get(r.ctx, &session.GetRequest{
		AppName:   r.app.AppName,
		UserID:    yu.DefaultUserID,
		SessionID: r.sessionID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	resp, err := r.app.Sessions.Create(r.ctx, &session.CreateRequest{
		AppName: r.app.AppName,
		UserID:  yu.DefaultUserID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	resp, err := r.app.Sessions.Get(r.ctx, &session.GetRequest{
		AppName:   r.app.AppName,
		UserID:    yu.DefaultUserID,
		SessionID: sessionID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	r.sessionID = resp.Session.ID
	fmt.Printf("Switched session: %s  Events: %d\n", resp.Session.ID, len(resp.Session.Events))
}

func (r *repl) printSessions() {
	resp, err := r.app.Sessions.List(r.ctx, &session.ListRequest{
		AppName: r.app.AppName,
		UserID:  yu.DefaultUserID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

func (r *repl) switchModel() {
	mc := selectModel(r.models, r.scanner)
	model, err := yu.BuildModel(mc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	r.app.Agent.SetModel(model)
	fmt.Printf("Switched to %s (thinking %s)\n", model.Name(), onOff(r.app.Agent.Thinking()))
}

func (r *repl) toggleThinking() {
	if !r.app.Agent.SupportsThinking() {
		fmt.Println("Thinking is not supported by the current model configuration.")
		return
	}
	r.app.Agent.SetThinking(!r.app.Agent.Thinking())
	fmt.Printf("Thinking: %s\n", onOff(r.app.Agent.Thinking()))
}

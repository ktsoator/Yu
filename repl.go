package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/session"
)

type repl struct {
	ctx       context.Context
	scanner   *bufio.Scanner
	models    []modelConfig
	sessions  session.Service
	sessionID string
	renderer  func() render.Renderer
}

func newREPL(ctx context.Context, in io.Reader, models []modelConfig, sessions session.Service, sessionID string) *repl {
	return &repl{
		ctx:       ctx,
		scanner:   bufio.NewScanner(in),
		models:    models,
		sessions:  sessions,
		sessionID: sessionID,
		renderer:  func() render.Renderer { return clirender.New() },
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

		if err := r.send(agent, input); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
	}
	if err := r.scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
	}
	return nil
}

func (r *repl) printHistory() {
	resp, err := r.sessions.Get(r.ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    defaultUserID,
		SessionID: r.sessionID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	sess := resp.Session
	fmt.Printf("Session: %s  User: %s  Messages: %d\n", sess.ID, sess.UserID, len(sess.Messages))
	for i, msg := range sess.Messages {
		fmt.Printf(" %2d. %-9s %s\n", i+1, historyRole(msg), historyText(msg))
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				fmt.Printf("     ↳ tool %s %s\n", tc.Name, truncate(tc.Arguments, 160))
			}
		}
	}
}

func (r *repl) newSession() {
	resp, err := r.sessions.Create(r.ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  defaultUserID,
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
	resp, err := r.sessions.Get(r.ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    defaultUserID,
		SessionID: sessionID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return
	}
	r.sessionID = resp.Session.ID
	fmt.Printf("Switched session: %s  Messages: %d\n", resp.Session.ID, len(resp.Session.Messages))
}

func (r *repl) printSessions() {
	resp, err := r.sessions.List(r.ctx, &session.ListRequest{
		AppName: appName,
		UserID:  defaultUserID,
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
		fmt.Printf(" %s %s  messages=%d  updated=%s\n", marker, sess.ID, len(sess.Messages), sess.UpdatedAt.Format("15:04:05"))
	}
}

func historyRole(msg session.Message) string {
	if msg.Name != "" {
		return string(msg.Role) + ":" + msg.Name
	}
	return string(msg.Role)
}

func historyText(msg session.Message) string {
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
	for ev, err := range agent.Run(r.ctx, r.sessionID, input) {
		if err != nil {
			renderer.Finish()
			return err
		}
		renderer.OnEvent(ev)
	}
	renderer.Finish()
	return nil
}

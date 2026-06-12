package launcher

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/render"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/runner"
	"github.com/ktsoator/yu/session"
)

const (
	defaultAppName = "yu"
	defaultUserID  = "local"
)

// Config controls a terminal launcher run.
type Config struct {
	AgentLoader agent.Loader
	Sessions    session.Service
	AppName     string
	UserID      string
	Input       io.Reader
	Output      io.Writer
	Renderer    func() render.Renderer
}

// Terminal launches an agent in a small terminal shell.
type Terminal struct{}

// NewTerminal returns a terminal launcher.
func NewTerminal() *Terminal {
	return &Terminal{}
}

// Execute runs one prompt when args are present, otherwise starts a small REPL.
func (l *Terminal) Execute(ctx context.Context, cfg *Config, args []string) error {
	if cfg == nil {
		return fmt.Errorf("launcher config is required")
	}
	if cfg.AgentLoader == nil {
		return fmt.Errorf("launcher agent loader is required")
	}
	a, err := cfg.AgentLoader.Load(ctx)
	if err != nil {
		return err
	}

	appName := cfg.AppName
	if appName == "" {
		appName = defaultAppName
	}
	userID := cfg.UserID
	if userID == "" {
		userID = defaultUserID
	}
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = session.NewInMemoryService()
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	renderer := cfg.Renderer
	if renderer == nil {
		renderer = func() render.Renderer { return clirender.New() }
	}

	r, err := runner.New(runner.Config{
		AppName:  appName,
		Agent:    a,
		Sessions: sessions,
	})
	if err != nil {
		return err
	}
	created, err := sessions.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		return err
	}

	state := &terminalState{
		ctx:       ctx,
		out:       out,
		appName:   appName,
		userID:    userID,
		sessionID: created.Session.ID,
		sessions:  sessions,
		runner:    r,
		renderer:  renderer,
	}
	if len(args) > 0 {
		input := strings.TrimSpace(strings.Join(args, " "))
		if input == "" {
			return fmt.Errorf("prompt is required")
		}
		return state.send(input)
	}
	return state.runInteractive(cfg.Input)
}

// CommandLineSyntax describes the launcher's tiny CLI surface.
func (l *Terminal) CommandLineSyntax() string {
	return "usage: <program> [prompt]\n\nWithout a prompt, starts an interactive terminal. Commands: /help, /new, /session, /exit."
}

type terminalState struct {
	ctx       context.Context
	out       io.Writer
	appName   string
	userID    string
	sessionID string
	sessions  session.Service
	runner    *runner.Runner
	renderer  func() render.Renderer
}

func (s *terminalState) runInteractive(input io.Reader) error {
	if input == nil {
		input = os.Stdin
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for {
		fmt.Fprintf(s.out, "\nYu %s › ", shortSessionID(s.sessionID))
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.handleCommand(line) {
				continue
			}
			return nil
		}
		if err := s.send(line); err != nil {
			fmt.Fprintf(s.out, "error: %v\n", err)
		}
	}
	return scanner.Err()
}

func (s *terminalState) handleCommand(line string) bool {
	cmd, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)

	switch cmd {
	case "/exit", "/quit":
		return false
	case "/help":
		fmt.Fprintln(s.out, "Commands: /help, /new, /session, /exit")
	case "/new":
		created, err := s.sessions.Create(s.ctx, &session.CreateRequest{
			AppName: s.appName,
			UserID:  s.userID,
		})
		if err != nil {
			fmt.Fprintf(s.out, "error: %v\n", err)
			return true
		}
		s.sessionID = created.Session.ID
		fmt.Fprintf(s.out, "New session: %s\n", s.sessionID)
	case "/session":
		if arg == "" {
			fmt.Fprintf(s.out, "Session: %s\n", s.sessionID)
			return true
		}
		if _, err := s.sessions.Get(s.ctx, &session.GetRequest{
			AppName:   s.appName,
			UserID:    s.userID,
			SessionID: arg,
		}); err != nil {
			fmt.Fprintf(s.out, "error: %v\n", err)
			return true
		}
		s.sessionID = arg
		fmt.Fprintf(s.out, "Switched session: %s\n", s.sessionID)
	default:
		fmt.Fprintf(s.out, "Unknown command: %s. Try /help.\n", cmd)
	}
	return true
}

func (s *terminalState) send(input string) error {
	ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt)
	defer stop()

	renderer := s.renderer()
	for ev, err := range s.runner.Run(ctx, s.userID, s.sessionID, input) {
		if err != nil {
			renderer.Finish()
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(s.out, "(interrupted)")
				return nil
			}
			return err
		}
		renderer.OnEvent(ev)
	}
	renderer.Finish()
	return nil
}

func shortSessionID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

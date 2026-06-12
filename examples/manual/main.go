package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/llm/openai"
	"github.com/ktsoator/yu/render/clirender"
	"github.com/ktsoator/yu/runner"
	"github.com/ktsoator/yu/session"
)

const (
	appName = "yu"
	userID  = "local"
)

func main() {
	ctx := context.Background()

	model := openai.New(openai.Config{
		APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-chat",
	})

	a, err := llmagent.New(agent.Config{
		Name:        "manual_agent",
		Model:       model,
		Description: "An agent that can use filesystem tools.",
		Instruction: "You are a helpful assistant. Use the available tools when useful.",
		// Tools: []tool.Executable{
		// 	fstool.NewReadFile(),
		// 	fstool.NewListDir(),
		// },
	})
	if err != nil {
		log.Fatal(err)
	}

	sessions := session.NewInMemoryService()
	created, err := sessions.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		log.Fatal(err)
	}

	r, err := runner.New(runner.Config{
		AppName:  appName,
		Agent:    a,
		Sessions: sessions,
	})
	if err != nil {
		log.Fatal(err)
	}

	state := &chatState{
		runner:    r,
		sessions:  sessions,
		sessionID: created.Session.ID,
	}
	if len(os.Args) > 1 {
		if err := state.send(ctx, strings.Join(os.Args[1:], " ")); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := state.repl(ctx); err != nil {
		log.Fatal(err)
	}
}

type chatState struct {
	runner    *runner.Runner
	sessions  session.Service
	sessionID string
}

func (s *chatState) repl(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	for {
		fmt.Printf("\nmanual %s › ", shortID(s.sessionID))
		if !scanner.Scan() {
			return scanner.Err()
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			if s.handleCommand(ctx, input) {
				continue
			}
			return nil
		}
		if err := s.send(ctx, input); err != nil {
			fmt.Printf("error: %v\n", err)
		}
	}
}

func (s *chatState) handleCommand(ctx context.Context, input string) bool {
	cmd, arg, _ := strings.Cut(input, " ")
	arg = strings.TrimSpace(arg)

	switch cmd {
	case "/exit", "/quit":
		return false
	case "/new":
		created, err := s.sessions.Create(ctx, &session.CreateRequest{
			AppName: appName,
			UserID:  userID,
		})
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return true
		}
		s.sessionID = created.Session.ID
		fmt.Printf("new session: %s\n", s.sessionID)
	case "/session":
		if arg == "" {
			fmt.Printf("session: %s\n", s.sessionID)
			return true
		}
		if _, err := s.sessions.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: arg,
		}); err != nil {
			fmt.Printf("error: %v\n", err)
			return true
		}
		s.sessionID = arg
		fmt.Printf("switched session: %s\n", s.sessionID)
	case "/help":
		fmt.Println("commands: /help, /new, /session [id], /exit")
	default:
		fmt.Printf("unknown command: %s\n", cmd)
	}
	return true
}

func (s *chatState) send(ctx context.Context, input string) error {
	turnCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	renderer := clirender.New()
	for ev, err := range s.runner.Run(turnCtx, userID, s.sessionID, input) {
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

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

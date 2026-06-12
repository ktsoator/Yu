package main

import (
	"context"
	"log"
	"os"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/launcher"
	"github.com/ktsoator/yu/llm/openai"
	"github.com/ktsoator/yu/session"
)

func main() {
	ctx := context.Background()

	model := openai.New(openai.Config{
		APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-v4-flash",
	})

	a, err := llmagent.New(agent.Config{
		Name:        "multi_tool_agent",
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

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(a),
		Sessions:    session.NewInMemoryService(),
	}

	l := launcher.NewTerminal()
	if err := l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/ktsoator/yu"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/session"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	envPath, err := config.EnvPath()
	if err != nil {
		return err
	}
	_ = godotenv.Load(envPath)

	configPath, err := config.ModelsPath()
	if err != nil {
		return err
	}
	models, err := config.LoadModels(configPath)
	if err != nil {
		return fmt.Errorf("load model config from %s: %w", configPath, err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	model, err := yu.BuildModel(selectModel(models, scanner))
	if err != nil {
		return err
	}
	sessions, closeSessions, err := yu.OpenSessionServiceFromEnv(ctx)
	if err != nil {
		return err
	}
	defer closeSessions()
	app, err := yu.New(yu.Config{Model: model, Sessions: sessions})
	if err != nil {
		return err
	}
	printAgentInfo(app, model.Name())

	initialSession, err := app.Sessions.Create(ctx, &session.CreateRequest{
		AppName: app.AppName,
		UserID:  yu.DefaultUserID,
	})
	if err != nil {
		return err
	}

	repl := newREPL(ctx, scanner, models, app, initialSession.Session.ID, model.Name())
	return repl.run()
}

func printAgentInfo(app *yu.App, modelName string) {
	names := make([]string, len(app.Tools))
	for i, t := range app.Tools {
		names[i] = t.Name()
	}
	fmt.Printf("Agent ready\n")
	fmt.Printf("  Model: %s\n", modelName)
	fmt.Printf("  Thinking: %s\n", onOff(app.Agent.Thinking()))
	fmt.Printf("  Tools: %s\n", strings.Join(names, ", "))
	fmt.Printf("  Commands: /help, /model, /think, /new, /sessions, /session <id>, /history, /exit\n")
}

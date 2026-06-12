package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/ktsoator/yu/session"
)

const (
	yuDirName      = ".yu"
	configFileName = "models.yaml"
	envFileName    = ".env"
	appName        = "yu"
	defaultUserID  = "local"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	envPath, err := yuPath(envFileName)
	if err != nil {
		return err
	}
	_ = godotenv.Load(envPath)

	// Load selectable model profiles up front. API keys are resolved from the
	// environment later, so ~/.yu/models.yaml can describe providers without secrets.
	configPath, err := yuPath(configFileName)
	if err != nil {
		return err
	}
	models, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load model config from %s: %w", configPath, err)
	}

	sessions := session.NewInMemoryService()
	initialSession, err := sessions.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  defaultUserID,
	})
	if err != nil {
		return err
	}

	repl := newREPL(ctx, os.Stdin, models, sessions, initialSession.Session.ID)
	agent, err := setupAgent(models, repl.scanner, sessions)
	if err != nil {
		return err
	}
	return repl.run(agent)
}

func yuPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, yuDirName, name), nil
}

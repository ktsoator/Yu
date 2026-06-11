package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/ktsoator/yu/weblog"
)

const (
	yuDirName      = ".yu"
	configFileName = "models.yaml"
	envFileName    = ".env"
	logDirName     = "logs"
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

	// Record every model round-trip to ~/.yu/logs for the /logs web viewer.
	logDir, err := yuPath(logDirName)
	if err != nil {
		return err
	}
	if err := weblog.Init(logDir); err != nil {
		return fmt.Errorf("init log dir %s: %w", logDir, err)
	}

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

	repl := newREPL(ctx, os.Stdin, models)
	agent, err := setupAgent(models, repl.scanner)
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

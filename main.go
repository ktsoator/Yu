package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

const configPath = "models.yaml"

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	_ = godotenv.Load()

	// Load selectable model profiles up front. API keys are resolved from the
	// environment later, so models.yaml can describe providers without secrets.
	models, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	repl := newREPL(ctx, os.Stdin, models)
	ag, err := setupAgent(models, repl.scanner)
	if err != nil {
		return err
	}
	return repl.run(ag)
}

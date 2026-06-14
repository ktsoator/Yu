package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/llm/openai"
	"github.com/ktsoator/yu/runner"
	sessiondatabase "github.com/ktsoator/yu/session/database"
	"github.com/ktsoator/yu/tool"
	"github.com/ktsoator/yu/tool/fstool"
)

const (
	appName = "yu"
	userID  = "local"

	sessionDriverEnv = "YU_SESSION_DRIVER"
	sessionDSNEnv    = "YU_SESSION_DSN"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Load local environment variables first so model API keys and optional
	// session database settings are available before anything is constructed.
	envPath, err := config.EnvPath()
	if err != nil {
		return err
	}
	_ = godotenv.Load(envPath)

	// Model profiles live in ~/.yu/models.yaml. The selected profile is turned
	// into an llm.Model below, while secrets still come from the environment.
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

	// Startup uses the first profile as the default model. Users can switch
	// later inside the REPL with /model <name|number>.
	mc := models[0]
	model := openai.New(openai.Config{
		APIKey:           os.Getenv(mc.APIKeyEnv),
		BaseURL:          mc.BaseURL,
		Model:            mc.Model,
		SupportsThinking: mc.SupportsThinking,
		ThinkingStyle:    mc.ThinkingStyle,
		ReasoningPath:    mc.ReasoningPath,
	})

	// The CLI uses database-backed sessions. Set YU_SESSION_DSN in ~/.yu/.env
	// or the shell; YU_SESSION_DRIVER defaults to postgres when omitted.
	sessions, err := sessiondatabase.Open(ctx, os.Getenv(sessionDriverEnv), os.Getenv(sessionDSNEnv))
	if err != nil {
		return err
	}
	defer sessions.Close()

	tools := []tool.Tool{
		fstool.NewReadFile(),
		fstool.NewListDir(),
	}
	ag, err := llmagent.New(agent.Config{
		Name:        appName,
		Model:       model,
		Description: "A concise coding assistant in a terminal.",
		Instruction: "You are a coding assistant in a terminal. Be concise. Use the available tools to read files and explore the project when it helps answer the user.",
		Tools:       tools,
	})
	if err != nil {
		return err
	}
	run, err := runner.New(runner.Config{
		AppName:  appName,
		Agent:    ag,
		Sessions: sessions,
	})
	if err != nil {
		return err
	}
	repl := newREPL(replConfig{
		Context:   ctx,
		Scanner:   scanner,
		Models:    models,
		AppName:   appName,
		UserID:    userID,
		Agent:     ag,
		Runner:    run,
		Sessions:  sessions,
		ModelName: model.Name(),
	})
	return repl.run()
}

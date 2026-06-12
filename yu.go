// Package yu assembles the application: it turns a model profile into a
// ready-to-run agent + runner + session service. Frontends (CLI, HTTP, ...)
// import this package and drive App.Runner; none of them should wire the
// internals themselves.
package yu

import (
	"context"
	"fmt"
	"os"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/llm/openai"
	"github.com/ktsoator/yu/runner"
	"github.com/ktsoator/yu/session"
	sessiondatabase "github.com/ktsoator/yu/session/database"
	"github.com/ktsoator/yu/tool"
	"github.com/ktsoator/yu/tool/fstool"
)

const (
	DefaultAppName   = "yu"
	DefaultUserID    = "local"
	SessionDriverEnv = "YU_SESSION_DRIVER"
	SessionDSNEnv    = "YU_SESSION_DSN"

	agentDescription = "A concise coding assistant in a terminal."
	agentInstruction = "You are a coding assistant in a terminal. Be concise. Use the available tools to read files and explore the project when it helps answer the user."
)

type Config struct {
	// Model is the LLM client the agent talks to. Required; build one from a
	// profile with BuildModel.
	Model llm.Model
	// AppName defaults to "yu".
	AppName string
	// Sessions defaults to a fresh in-memory service.
	Sessions session.Service
}

// App is one assembled Yu instance: a frontend talks to Runner for
// conversations, Agent for model/thinking switches, and Sessions for
// history queries.
type App struct {
	AppName  string
	Agent    agent.Agent
	Runner   *runner.Runner
	Sessions session.Service
	Tools    []tool.Executable
}

func New(cfg Config) (*App, error) {
	appName := cfg.AppName
	if appName == "" {
		appName = DefaultAppName
	}
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = session.NewInMemoryService()
	}

	if cfg.Model == nil {
		return nil, fmt.Errorf("app model is required")
	}
	tools := []tool.Executable{fstool.NewReadFile(), fstool.NewListDir()}
	ag, err := llmagent.New(agent.Config{
		Name:        appName,
		Model:       cfg.Model,
		Description: agentDescription,
		Instruction: agentInstruction,
		Tools:       tools,
	})
	if err != nil {
		return nil, err
	}
	run, err := runner.New(runner.Config{
		AppName:  appName,
		Agent:    ag,
		Sessions: sessions,
	})
	if err != nil {
		return nil, err
	}
	return &App{
		AppName:  appName,
		Agent:    ag,
		Runner:   run,
		Sessions: sessions,
		Tools:    tools,
	}, nil
}

// OpenSessionServiceFromEnv returns a database session service when
// YU_SESSION_DSN is set; otherwise it falls back to in-memory sessions.
func OpenSessionServiceFromEnv(ctx context.Context) (session.Service, func(), error) {
	driver := os.Getenv(SessionDriverEnv)
	dsn := os.Getenv(SessionDSNEnv)
	if dsn == "" {
		if driver != "" {
			return nil, nil, fmt.Errorf("%s is set but %s is empty", SessionDriverEnv, SessionDSNEnv)
		}
		return session.NewInMemoryService(), func() {}, nil
	}
	service, err := sessiondatabase.Open(ctx, driver, dsn)
	if err != nil {
		return nil, nil, err
	}
	return service, service.Close, nil
}

// BuildModel resolves the API key and constructs an openai-compatible client.
func BuildModel(mc config.Model) (llm.Model, error) {
	apiKey := os.Getenv(mc.APIKeyEnv)
	if apiKey == "" {
		return nil, fmt.Errorf("missing API key: set %s in your environment or ~/.yu/.env", mc.APIKeyEnv)
	}
	return openai.New(openai.Config{
		APIKey:           apiKey,
		BaseURL:          mc.BaseURL,
		Model:            mc.Model,
		SupportsThinking: mc.SupportsThinking,
		ThinkingStyle:    mc.ThinkingStyle,
		ReasoningPath:    mc.ReasoningPath,
	}), nil
}

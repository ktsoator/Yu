package main

import (
	"fmt"
	"os"

	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/llm/openai"
)

// buildModel resolves the API key and constructs an openai client.
func buildModel(mc modelConfig) (llm.Model, error) {
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

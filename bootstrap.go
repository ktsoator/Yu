package main

import (
	"bufio"
	"fmt"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/llm"
)

// setupAgent selects a model and builds the agent.
func setupAgent(models []modelConfig, scanner *bufio.Scanner) (agent.Agent, error) {
	mc := selectModel(models, scanner)
	model, err := buildModel(mc)
	if err != nil {
		return nil, err
	}
	agent, err := newAgent(model)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

func newAgent(model llm.Model) (agent.Agent, error) {
	ag, err := llmagent.New(agent.Config{
		Name:        "yu",
		Model:       model,
		Description: "A concise coding assistant in a terminal.",
		Instruction: "You are a coding assistant in a terminal. Be concise.",
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Agent ready\n")
	fmt.Printf("  Model: %s\n", model.Name())
	fmt.Printf("  Thinking: %s\n", onOff(ag.Thinking()))
	fmt.Printf("  Commands: /model, /think, /exit\n")
	return ag, nil
}

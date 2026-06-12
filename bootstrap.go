package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/agent/llmagent"
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
	"github.com/ktsoator/yu/tool/fstool"
)

// setupAgent selects a model and builds the agent.
func setupAgent(models []modelConfig, scanner *bufio.Scanner, sessions session.Service) (agent.Agent, error) {
	mc := selectModel(models, scanner)
	model, err := buildModel(mc)
	if err != nil {
		return nil, err
	}
	agent, err := newAgent(model, sessions)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

func newAgent(model llm.Model, sessions session.Service) (agent.Agent, error) {
	tools := []tool.Tool{fstool.NewReadFile(), fstool.NewListDir()}
	ag, err := llmagent.New(agent.Config{
		Name:        "yu",
		AppName:     appName,
		Model:       model,
		Description: "A concise coding assistant in a terminal.",
		Instruction: "You are a coding assistant in a terminal. Be concise. Use the available tools to read files and explore the project when it helps answer the user.",
		Tools:       tools,
		Sessions:    sessions,
		UserID:      defaultUserID,
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Agent ready\n")
	fmt.Printf("  Model: %s\n", model.Name())
	fmt.Printf("  Thinking: %s\n", onOff(ag.Thinking()))
	fmt.Printf("  Tools: %s\n", toolNames(tools))
	fmt.Printf("  Commands: /model, /think, /new, /sessions, /session <id>, /history, /exit\n")
	return ag, nil
}

func toolNames(tools []tool.Tool) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	return strings.Join(names, ", ")
}

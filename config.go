package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type modelConfig struct {
	Name             string `yaml:"name"`
	BaseURL          string `yaml:"base_url"`
	Model            string `yaml:"model"`
	APIKeyEnv        string `yaml:"api_key_env"`
	SupportsThinking bool   `yaml:"supports_thinking"`
	ThinkingStyle    string `yaml:"thinking_style"`
	// ReasoningPath lets compatible vendors expose reasoning deltas under
	// different raw JSON fields without needing separate client implementations.
	ReasoningPath string `yaml:"reasoning_path"`
}

func loadConfig(path string) ([]modelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Models []modelConfig `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("%s: no models defined", path)
	}
	return cfg.Models, nil
}

func selectModel(models []modelConfig, scanner *bufio.Scanner) modelConfig {
	// Empty input selects the first profile, so model config order defines the
	// default model.
	fmt.Println("Select a model:")
	for i, m := range models {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		fmt.Printf(" %s %d) %-10s %s\n", marker, i+1, m.Name, m.Model)
	}

	for {
		fmt.Print("model › ")
		if !scanner.Scan() {
			return models[0]
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return models[0]
		}
		if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(models) {
			return models[n-1]
		}
		fmt.Printf("Enter a number between 1 and %d.\n", len(models))
	}
}

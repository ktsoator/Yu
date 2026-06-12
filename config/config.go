// Package config loads Yu's model profiles from ~/.yu. API keys are never
// stored in the config file — each profile references an environment variable
// by name, so the file can describe providers without secrets.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	dirName    = ".yu"
	ModelsFile = "models.yaml"
	EnvFile    = ".env"
)

// Model is one selectable model profile from ~/.yu/models.yaml.
type Model struct {
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

// Dir returns the Yu config directory (~/.yu).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, dirName), nil
}

// Path returns the path of a file inside the Yu config directory.
func Path(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func ModelsPath() (string, error) { return Path(ModelsFile) }
func EnvPath() (string, error)    { return Path(EnvFile) }

func LoadModels(path string) ([]Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Models []Model `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("%s: no models defined", path)
	}
	return cfg.Models, nil
}

package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/ktsoator/yu/config"
)

func selectModel(models []config.Model, scanner *bufio.Scanner) config.Model {
	fmt.Println("\033[90m╭─ available models\033[0m")
	for i, m := range models {
		marker := " "
		if i == 0 {
			marker = "›"
		}
		fmt.Printf("\033[90m│\033[0m %s %d. \033[1m%-10s\033[0m \033[90m%s\033[0m\n", marker, i+1, m.Name, m.Model)
	}
	fmt.Println("\033[90m╰─ Enter to use the highlighted model\033[0m")

	for {
		fmt.Print("model › ")
		if !scanner.Scan() {
			return models[0]
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return models[0]
		}
		if model, ok := findModel(models, input); ok {
			return model
		}
		fmt.Printf("Enter a number between 1 and %d, or a model name.\n", len(models))
	}
}

func findModel(models []config.Model, spec string) (config.Model, bool) {
	if spec == "" {
		return config.Model{}, false
	}
	if n, err := strconv.Atoi(spec); err == nil && n >= 1 && n <= len(models) {
		return models[n-1], true
	}
	for _, m := range models {
		if m.Name == spec || m.Model == spec {
			return m, true
		}
	}
	return config.Model{}, false
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

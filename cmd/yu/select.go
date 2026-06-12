package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/ktsoator/yu/config"
)

func selectModel(models []config.Model, scanner *bufio.Scanner) config.Model {
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

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

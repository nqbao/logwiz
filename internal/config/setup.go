package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Setup runs an interactive first-time configuration wizard, prompting for
// base URL, API key, and model. Existing values are shown (the key is masked)
// and the user can accept or change each one. Results are persisted to
// ~/.logwiz/config.json.
func Setup() error {
	fc, err := loadFileConfig()
	if err != nil {
		return fmt.Errorf("reading existing config: %w", err)
	}

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("logwiz setup")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("Press Enter to accept the current/default value.")
	fmt.Println()

	// --- Base URL ---
	current := fc.OpenAIBaseURL
	if current == "" {
		current = DefaultBaseURL
	}
	fmt.Printf("OpenAI-compatible base URL [%s]: ", current)
	if !scanner.Scan() {
		return fmt.Errorf("setup cancelled")
	}
	if input := strings.TrimSpace(scanner.Text()); input != "" {
		fc.OpenAIBaseURL = strings.TrimRight(input, "/")
	} else {
		fc.OpenAIBaseURL = strings.TrimRight(current, "/")
	}

	// --- API Key ---
	masked := ""
	if fc.OpenAIAPIKey != "" {
		if len(fc.OpenAIAPIKey) > 4 {
			masked = "***" + fc.OpenAIAPIKey[len(fc.OpenAIAPIKey)-4:]
		} else {
			masked = "***"
		}
	}
	if masked != "" {
		fmt.Printf("API key (current: %s): ", masked)
	} else {
		fmt.Print("API key: ")
	}
	if !scanner.Scan() {
		return fmt.Errorf("setup cancelled")
	}
	if input := strings.TrimSpace(scanner.Text()); input != "" {
		fc.OpenAIAPIKey = input
	}

	// --- Model ---
	currentModel := fc.OpenAIModel
	if currentModel == "" {
		currentModel = DefaultModel
	}
	fmt.Printf("Model [%s]: ", currentModel)
	if !scanner.Scan() {
		return fmt.Errorf("setup cancelled")
	}
	if input := strings.TrimSpace(scanner.Text()); input != "" {
		fc.OpenAIModel = input
	} else {
		fc.OpenAIModel = currentModel
	}

	if err := saveFileConfig(fc); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("\nConfig saved to ~/.logwiz/config.json")
	return nil
}

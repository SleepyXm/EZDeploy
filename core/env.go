package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetupEnv interactively collects key/value pairs from stdin and writes
// them to a .env file in projectPath.
func SetupEnv(projectPath string) error {
	fmt.Println("\n[→] Enter environment variables (leave key blank when done):")

	scanner := bufio.NewScanner(os.Stdin)
	var lines []string

	for {
		fmt.Print("    Key: ")
		scanner.Scan()
		key := strings.TrimSpace(scanner.Text())
		if key == "" {
			break
		}

		fmt.Print("    Value: ")
		scanner.Scan()
		value := strings.TrimSpace(scanner.Text())
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	if len(lines) == 0 {
		fmt.Println("[!] No environment variables entered, skipping...")
		return nil
	}

	envPath := filepath.Join(projectPath, ".env")
	f, err := os.Create(envPath)
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	defer f.Close()

	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("write .env: %w", err)
		}
	}

	fmt.Println("\n[✓] .env file created")
	return nil
}

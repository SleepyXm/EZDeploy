package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SetupEnv(projectPath string) error {
	envOutput := filepath.Join(projectPath, ".env")

	fmt.Println("\n[→] Enter environment variables (leave key blank when done):\n")

	envValues := map[string]string{}

	for {
		key, err := input("    Key: ")
		if err != nil {
			return err
		}

		key = strings.TrimSpace(key)
		if key == "" {
			break
		}

		value, err := input("    Value: ")
		if err != nil {
			return err
		}

		envValues[key] = strings.TrimSpace(value)
	}

	if len(envValues) == 0 {
		fmt.Println("[!] No environment variables entered, skipping...")
		return nil
	}

	f, err := os.Create(envOutput)
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	defer f.Close()

	for key, value := range envValues {
		if _, err := fmt.Fprintf(f, "%s=%s\n", key, value); err != nil {
			return fmt.Errorf("write .env: %w", err)
		}
	}

	fmt.Println("\n[✓] .env file created")
	return nil
}

func input(prompt string) (string, error) {
	fmt.Print(prompt)

	var b strings.Builder
	buf := make([]byte, 1)

	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return "", err
		}

		if n == 0 {
			continue
		}

		ch := buf[0]

		switch ch {
		case '\n', '\r':
			fmt.Println()
			return b.String(), nil

		case 127, 8:
			current := b.String()
			if len(current) > 0 {
				b.Reset()
				b.WriteString(current[:len(current)-1])
				fmt.Print("\b \b")
			}

		default:
			b.WriteByte(ch)
			fmt.Print(string(ch))
		}
	}
}

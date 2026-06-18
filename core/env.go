package core

import (
	"EZDeploy/UI/pretty"
	"EZDeploy/walker"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func SetupEnv(projectPath string) error {
	envOutput := filepath.Join(projectPath, ".env")
	envValues := map[string]string{}

	report, err := walker.ScanDefault(projectPath)
	if err != nil {
		pretty.Printf("[!] Env discovery skipped: %v\n", err)
	} else {
		printEnvDiscoveryReport(report)

		for _, key := range report.UniqueEnvNames() {
			value, err := input(fmt.Sprintf("%s (blank to skip): ", key))
			if err != nil {
				return err
			}

			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}

			envValues[key] = value
		}
	}

	pretty.Println("\n[→] Add extra environment variables (leave key blank when done):\n")

	for {
		key, err := input("Key: ")
		if err != nil {
			return err
		}

		key = strings.TrimSpace(key)
		if key == "" {
			break
		}

		value, err := input("Value: ")
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

	keys := make([]string, 0, len(envValues))
	for key := range envValues {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		if _, err := fmt.Fprintf(f, "%s=%s\n", key, envValues[key]); err != nil {
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
			fmt.Print("\r\n")
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

func printEnvDiscoveryReport(report walker.Report) {
	if len(report.Languages) > 0 {
		pretty.Println("\n[→] Detected languages:")

		langs := make([]string, 0, len(report.Languages))
		for lang := range report.Languages {
			langs = append(langs, lang)
		}

		sort.Strings(langs)

		for _, lang := range langs {
			fmt.Printf("  - %s: %d files\n", lang, report.Languages[lang])
		}
	}

	if len(report.EnvHits) == 0 {
		pretty.Println("\n[→] No environment variables discovered automatically.")
		return
	}

	pretty.Println("\n[→] Discovered environment variables:")

	for _, hit := range report.EnvHits {
		pretty.Printf("  - %-30s %s:%d  [%s]\n", hit.Name, hit.Path, hit.Line, hit.Rule)
	}

	pretty.Println("\n[→] Enter values for discovered variables:")
}

package core

import (
	"EZDeploy/walker"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SetupEnv preserves existing values and writes newly discovered ones. Manual
// additions belong to first deploys; redeploys only ask for new scanned keys.
func SetupEnv(projectPath string, interactive, allowManualAdditions bool) error {
	envOutput := filepath.Join(projectPath, ".env")
	envValues := map[string]string{}
	existing, err := os.ReadFile(envOutput)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing .env: %w", err)
	}
	if err == nil {
		// Existing secrets receive the same protection as newly created files.
		if err := os.Chmod(envOutput, 0o600); err != nil {
			return fmt.Errorf("secure existing .env: %w", err)
		}
	}
	existingKeys := parseEnvKeys(existing)

	report, err := walker.ScanDefault(projectPath)
	if err != nil {
		if !interactive {
			return fmt.Errorf("environment discovery: %w", err)
		}
		fmt.Printf("[!] Env discovery skipped: %v\n", err)
	} else {
		var configured, discovered []string
		for _, key := range report.UniqueEnvNames() {
			if existingKeys[key] {
				configured = append(configured, key)
			} else {
				discovered = append(discovered, key)
			}
		}
		if interactive {
			printEnvDiscoveryReport(report, configured, discovered)
		}

		var missing []string
		for _, key := range discovered {
			if !interactive {
				missing = append(missing, key)
				continue
			}
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
		if len(missing) > 0 {
			return fmt.Errorf("missing environment values for %s", strings.Join(missing, ", "))
		}
	}
	if !interactive {
		fmt.Println("[✓] Environment validated")
		return nil
	}

	if allowManualAdditions {
		fmt.Printf("\n[→] Add extra environment variables (leave key blank when done):\n\n")
		for {
			key, err := input("Key: ")
			if err != nil {
				return err
			}
			key = strings.TrimSpace(key)
			if key == "" {
				break
			}
			if existingKeys[key] {
				fmt.Printf("[!] %s already exists; preserving its value\n", key)
				continue
			}
			value, err := input("Value: ")
			if err != nil {
				return err
			}
			envValues[key] = strings.TrimSpace(value)
		}
	}

	if len(envValues) == 0 {
		fmt.Println("[✓] Environment unchanged")
		return nil
	}

	flags := os.O_CREATE | os.O_WRONLY
	if len(existing) > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(envOutput, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open .env: %w", err)
	}
	defer f.Close()
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

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

func parseEnvKeys(data []byte) map[string]bool {
	keys := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if key, _, ok := strings.Cut(line, "="); ok && key != "" && !strings.HasPrefix(key, "#") {
			keys[strings.TrimSpace(key)] = true
		}
	}
	return keys
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

func printEnvDiscoveryReport(report walker.Report, configured, discovered []string) {
	if len(report.Languages) > 0 {
		fmt.Println("\n[→] Detected languages:")

		langs := make([]string, 0, len(report.Languages))
		for lang := range report.Languages {
			langs = append(langs, lang)
		}

		sort.Strings(langs)

		for _, lang := range langs {
			fmt.Printf("  - %s: %d files\n", lang, report.Languages[lang])
		}
	}

	if len(configured)+len(discovered) == 0 {
		fmt.Println("\n[→] No environment variables discovered automatically.")
		return
	}

	fmt.Println("\n[✓] Already configured environment variables:")
	if len(configured) == 0 {
		fmt.Println("  - none")
	} else {
		for _, key := range configured {
			fmt.Printf("  - %s\n", key)
		}
	}
	fmt.Println("\n[→] Newly discovered environment variables:")
	if len(discovered) == 0 {
		fmt.Println("  - none")
		return
	}
	for _, key := range discovered {
		fmt.Printf("  - %s\n", key)
	}
	fmt.Println("\n[→] Enter values for newly discovered variables:")
}

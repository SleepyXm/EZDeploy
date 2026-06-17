package core

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runIn is like run but executes the command in the given working directory.
func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// fileExists reports whether path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// goModBinaryName parses go.mod to extract the binary name from the module path.
// Falls back to "app" if the file is missing or unparseable.
func goModBinaryName(projectPath string) string {
	f, err := os.Open(filepath.Join(projectPath, "go.mod"))
	if err != nil {
		return "app"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "module" {
			parts := strings.Split(fields[1], "/")
			return strings.ToLower(parts[len(parts)-1])
		}
	}
	return "app"
}

// DownloadDeps auto-detects the project type and installs or builds dependencies.
func DownloadDeps(projectPath string) error {
	requirements := filepath.Join(projectPath, "requirements.txt")
	packageJSON := filepath.Join(projectPath, "package.json")
	goMod := filepath.Join(projectPath, "go.mod")

	switch {
	case fileExists(requirements):
		fmt.Println("[→] Python project found, installing dependencies...")
		if err := run("pip", "install", "--ignore-installed", "-r", requirements); err != nil {
			return fmt.Errorf("pip install: %w", err)
		}
		fmt.Println("[✓] Dependencies installed")

	case fileExists(goMod):
		binaryName := goModBinaryName(projectPath)
		binaryPath := filepath.Join(projectPath, binaryName)

		if fileExists(binaryPath) {
			fmt.Println("[✓] Pre-built binary found, skipping build...")
			if err := os.Chmod(binaryPath, 0o755); err != nil {
				return fmt.Errorf("chmod binary: %w", err)
			}
		} else {
			fmt.Printf("[→] Go project detected, building binary %s...\n", binaryName)
			if err := runIn(projectPath, "go", "build", "-buildvcs=false", "-o", binaryName, "."); err != nil {
				return fmt.Errorf("go build: %w", err)
			}
			fmt.Printf("[✓] Binary built: %s\n", binaryName)
		}

	case fileExists(packageJSON):
		fmt.Println("[→] Node project found, installing dependencies...")
		if err := runIn(projectPath, "npm", "install"); err != nil {
			return fmt.Errorf("npm install: %w", err)
		}
		fmt.Println("[✓] Dependencies installed")

	default:
		fmt.Println("[!] No dependency file found. Skipping...")
	}

	return nil
}

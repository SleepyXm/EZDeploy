package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileExists reports whether path exists on disk.
func FileExists(path string) bool {
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

// PrepareNativeService installs/builds only the service selected by the user.
// entry is relative to projectPath and is used to build nested Go cmd targets.
func PrepareNativeService(projectPath, runtime, entry string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "python":
		return "", installPythonDeps(projectPath)
	case "go":
		return buildGoService(projectPath, entry)
	case "node":
		return "", installNodeDeps(projectPath)
	default:
		return "", DownloadDeps(projectPath)
	}
}

// DownloadDeps auto-detects the project type and installs or builds dependencies.
func DownloadDeps(projectPath string) error {
	requirements := filepath.Join(projectPath, "requirements.txt")
	packageJSON := filepath.Join(projectPath, "package.json")
	goMod := filepath.Join(projectPath, "go.mod")

	switch {
	case FileExists(requirements):
		return installPythonDeps(projectPath)

	case FileExists(goMod):
		_, err := buildGoService(projectPath, "")
		return err

	case FileExists(packageJSON):
		return installNodeDeps(projectPath)

	default:
		fmt.Println("[!] No dependency file found. Skipping...")
	}

	return nil
}

func installPythonDeps(projectPath string) error {
	fmt.Println("[→] Python service selected, installing dependencies...")
	args := []string{"-m", "pip", "install", "--ignore-installed"}
	if FileExists(filepath.Join(projectPath, "requirements.txt")) {
		args = append(args, "-r", "requirements.txt")
	} else if FileExists(filepath.Join(projectPath, "pyproject.toml")) {
		args = append(args, ".")
	} else {
		fmt.Println("[!] No Python dependency manifest found. Skipping...")
		return nil
	}
	if err := Run(projectPath, "python3", args...); err != nil {
		return fmt.Errorf("python dependency install: %w", err)
	}
	fmt.Println("[✓] Dependencies installed")
	return nil
}

func installNodeDeps(projectPath string) error {
	fmt.Println("[→] Node service selected, installing dependencies...")
	if err := Run(projectPath, "npm", "install"); err != nil {
		return fmt.Errorf("npm install: %w", err)
	}
	fmt.Println("[✓] Dependencies installed")
	return nil
}

func buildGoService(projectPath, entry string) (string, error) {
	target := "."
	binaryName := goModBinaryName(projectPath)
	if entry = filepath.Clean(filepath.FromSlash(strings.TrimSpace(entry))); entry != "." && entry != "" {
		if filepath.IsAbs(entry) || entry == ".." || strings.HasPrefix(entry, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("Go service entry escapes project root: %s", entry)
		}
		if dir := filepath.Dir(entry); dir != "." {
			target = "./" + filepath.ToSlash(dir)
			binaryName = filepath.Base(dir)
		}
	}
	if !projectNamePattern.MatchString(binaryName) {
		return "", fmt.Errorf("cannot derive Go binary name from %q", entry)
	}

	// Every deployment rebuilds the selected target; an existing binary may
	// represent an older commit or another cmd/* service in the same module.
	fmt.Printf("[→] Go service selected, building %s from %s...\n", binaryName, target)
	if err := Run(projectPath, "go", "build", "-buildvcs=false", "-o", binaryName, target); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	if err := os.Chmod(filepath.Join(projectPath, binaryName), 0o755); err != nil {
		return "", fmt.Errorf("chmod binary: %w", err)
	}
	fmt.Printf("[✓] Binary built: %s\n", binaryName)
	return "./" + binaryName, nil
}

// DefaultStartCommand returns only conventional commands; ambiguous projects
// remain user-controlled through the deploy command's --start option.
func DefaultStartCommand(projectPath string) (string, error) {
	if FileExists(filepath.Join(projectPath, "go.mod")) {
		return "./" + goModBinaryName(projectPath), nil
	}
	if FileExists(filepath.Join(projectPath, "package.json")) {
		data, err := os.ReadFile(filepath.Join(projectPath, "package.json"))
		if err != nil {
			return "", err
		}
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			return "", fmt.Errorf("parse package.json: %w", err)
		}
		if strings.TrimSpace(pkg.Scripts["start"]) != "" {
			return "npm start", nil
		}
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, "main.py")); err == nil && strings.Contains(string(data), "FastAPI(") {
		return `python3 -m uvicorn main:app --host 127.0.0.1 --port "$PORT"`, nil
	}
	return "", nil
}

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const CloneDir = "/opt/EZDeploy/projects"

// CloneRepo clones a repository into CloneDir, or pulls the latest
// changes if it already exists. Returns the absolute path to the project.
func CloneRepo(repoURL string) (string, error) {
	repoName := strings.TrimSuffix(filepath.Base(strings.TrimRight(repoURL, "/")), ".git")
	dest := filepath.Join(CloneDir, repoName)

	if _, err := os.Stat(dest); err == nil {
		fmt.Println("[→] Repo already exists, pulling latest...")
		if err := run("git", "-C", dest, "pull"); err != nil {
			return "", fmt.Errorf("git pull: %w", err)
		}
	} else {
		fmt.Printf("[→] Cloning %s...\n", repoURL)
		if err := run("git", "clone", repoURL, dest); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}
	}

	fmt.Printf("[✓] Ready at %s\n", dest)
	return dest, nil
}

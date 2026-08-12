package core

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const CloneDir = "./projects"

var (
	projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	serviceNamePattern = regexp.MustCompile(`[^a-z0-9.-]+`)
)

// ManagedName is the canonical systemd and Docker container name.
func ManagedName(projectName string) string {
	name := strings.NewReplacer("_", "-", " ", "-").Replace(strings.ToLower(projectName))
	return "ezdeploy-" + name
}

// ManagedServiceName gives each process in a monorepo a stable system identity.
func ManagedServiceName(projectName, serviceName string, multiple bool) string {
	if !multiple {
		return ManagedName(projectName)
	}
	serviceName = serviceNamePattern.ReplaceAllString(strings.ToLower(serviceName), "-")
	serviceName = strings.Trim(serviceName, "-")
	if serviceName == "" {
		serviceName = "service"
	}
	return ManagedName(projectName + "-" + serviceName)
}

type CloneOptions struct {
	Branch string
	SSHKey string
}

// CloneRepoWithOptions supports public URLs and one explicit private-repo key.
// The key is passed to Git for this process only and is never copied or stored.
func CloneRepoWithOptions(repoURL string, options CloneOptions) (string, error) {
	repoURL = normalizeRepoURL(repoURL)
	options.SSHKey = strings.TrimSpace(options.SSHKey)
	projectName, err := ProjectNameFromRepoURL(repoURL)
	if err != nil {
		return "", err
	}
	if options.Branch != "" {
		if err := exec.Command("git", "check-ref-format", "--branch", options.Branch).Run(); err != nil {
			return "", fmt.Errorf("invalid branch %q", options.Branch)
		}
	}

	sshCommand := ""
	if options.SSHKey != "" {
		keyPath, err := validateSSHKey(options.SSHKey)
		if err != nil {
			return "", err
		}
		repoURL, err = sshRepoURL(repoURL)
		if err != nil {
			return "", err
		}
		sshCommand = fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=accept-new", shellQuote(keyPath))
	}

	// Installed copies are not guaranteed to ship with an empty projects directory.
	if err := os.MkdirAll(CloneDir, 0o755); err != nil {
		return "", fmt.Errorf("create projects directory: %w", err)
	}
	dest := filepath.Join(CloneDir, projectName)
	if info, err := os.Stat(dest); err == nil {
		if !info.IsDir() || !FileExists(filepath.Join(dest, ".git")) {
			return "", fmt.Errorf("%s exists but is not a git repository", dest)
		}
		origin, err := gitOutput(dest, "remote", "get-url", "origin")
		if err != nil || repoIdentity(origin) != repoIdentity(repoURL) {
			return "", fmt.Errorf("existing clone origin %q does not match %q", origin, repoURL)
		}
		fetchArgs := []string{"fetch", "origin"}
		if sshCommand != "" {
			// Existing HTTPS clones can use the SSH URL without rewriting .git/config.
			fetchArgs = append([]string{"-c", "remote.origin.url=" + repoURL}, fetchArgs...)
		}
		if err := runGit(sshCommand, dest, fetchArgs...); err != nil {
			return "", fmt.Errorf("git fetch: %w", err)
		}
		branch := options.Branch
		if branch == "" {
			branch, err = CurrentBranch(dest)
			if err != nil {
				return "", err
			}
		}
		if err := runGit(sshCommand, dest, "checkout", branch); err != nil {
			if err := runGit(sshCommand, dest, "checkout", "-b", branch, "--track", "origin/"+branch); err != nil {
				return "", fmt.Errorf("git checkout %s: %w", branch, err)
			}
		}
		if err := runGit(sshCommand, dest, "merge", "--ff-only", "origin/"+branch); err != nil {
			return "", fmt.Errorf("git fast-forward %s: %w", branch, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect clone destination: %w", err)
	} else {
		args := []string{"clone"}
		if options.Branch != "" {
			args = append(args, "--branch", options.Branch, "--single-branch")
		}
		args = append(args, repoURL, dest)
		if err := runGit(sshCommand, "", args...); err != nil {
			return "", fmt.Errorf("git clone: %w", err)
		}
	}

	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	fmt.Printf("[✓] Repository ready at %s\n", absDest)
	return absDest, nil
}

func ProjectNameFromRepoURL(repoURL string) (string, error) {
	name := strings.TrimSuffix(filepath.Base(strings.TrimRight(strings.TrimSpace(repoURL), "/")), ".git")
	// The name becomes both a directory and a systemd/Nginx identifier.
	if !projectNamePattern.MatchString(name) || len(name) > 100 {
		return "", fmt.Errorf("cannot derive project name from %q", repoURL)
	}
	return name, nil
}

func CurrentBranch(projectPath string) (string, error) {
	branch, err := gitOutput(projectPath, "branch", "--show-current")
	if err != nil || branch == "" {
		return "", fmt.Errorf("repository has no active branch")
	}
	return branch, nil
}

// CurrentRevision identifies the repository release stored in the registry.
func CurrentRevision(projectPath string) (string, error) {
	revision, err := gitOutput(projectPath, "rev-parse", "HEAD")
	if err != nil || revision == "" {
		return "", fmt.Errorf("repository has no current revision")
	}
	return revision, nil
}

func validateSSHKey(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(strings.ToLower(absPath), ".pub") {
		return "", fmt.Errorf("provide the private key, not %s", absPath)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("invalid SSH private key %s", absPath)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("SSH private key is too open; run chmod 600 %s", absPath)
	}
	return absPath, nil
}

func normalizeRepoURL(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if !strings.Contains(repoURL, "://") && !strings.HasPrefix(repoURL, "git@") {
		first := strings.SplitN(repoURL, "/", 2)[0]
		if strings.Contains(first, ".") {
			return "https://" + repoURL
		}
	}
	return repoURL
}

func sshRepoURL(repoURL string) (string, error) {
	if strings.HasPrefix(repoURL, "git@") || strings.HasPrefix(repoURL, "ssh://") {
		return repoURL, nil
	}
	parsed, err := url.Parse(repoURL)
	if err != nil || parsed.Hostname() == "" || parsed.Path == "" {
		return "", fmt.Errorf("cannot convert %q to an SSH repository URL", repoURL)
	}
	return "git@" + parsed.Hostname() + ":" + strings.TrimPrefix(parsed.Path, "/"), nil
}

func repoIdentity(repoURL string) string {
	repoURL = strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
	if strings.HasPrefix(repoURL, "git@") {
		return strings.Replace(strings.TrimPrefix(repoURL, "git@"), ":", "/", 1)
	}
	if parsed, err := url.Parse(repoURL); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname() + "/" + strings.Trim(parsed.Path, "/")
	}
	return repoURL
}

func runGit(sshCommand, dir string, args ...string) error {
	if dir != "" {
		absDir, _ := filepath.Abs(dir)
		args = append([]string{"-c", "safe.directory=" + absDir, "-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	cmd.Env = os.Environ()
	if sshCommand != "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND="+sshCommand)
	}
	return cmd.Run()
}

func gitOutput(dir string, args ...string) (string, error) {
	absDir, _ := filepath.Abs(dir)
	cmd := exec.Command("git", append([]string{"-c", "safe.directory=" + absDir, "-C", dir}, args...)...)
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

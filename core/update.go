package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PullAndRedeploy is the entry-point for user-triggered updates
// (`ezdeploy pull`). It:
//
//  1. Reads the project's full config from the registry (repo, branch, SSH key, runtime).
//  2. Fetches and fast-forwards the tracked branch using the same credentials
//     that were used at deploy time — private repos work without re-prompting.
//  3. Snapshots the running state before touching the service.
//  4. Redeploys via Docker or systemd depending on the project's runtime.
//  5. Verifies the new instance is healthy; rolls back to the snapshot if not.
func PullAndRedeploy(projectName string) error {
	reg, err := loadRegistry()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	project, ok := reg[projectName]
	if !ok {
		return fmt.Errorf("project %q not found in registry", projectName)
	}

	fmt.Printf("[→] Updating %s (branch: %s)...\n", projectName, project.Branch)

	// ── 1. Git pull ──────────────────────────────────────────────────────────
	// Re-use the SSH key stored at deploy time so private repos keep working.
	opts := CloneOptions{
		Branch: project.Branch,
		SSHKey: project.SSHKey, // empty string = public repo, falls back to HTTPS
	}
	if _, err := CloneRepoWithOptions(project.RepoURL, opts); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	fmt.Printf("[✓] Pulled latest %s\n", project.Branch)

	// ── 2. Redeploy by runtime ───────────────────────────────────────────────
	switch project.Runtime {
	case "docker":
		return pullRedeployDocker(projectName, project)
	default:
		// systemd-managed process (go, python, node, rust, …)
		return pullRedeploySystemd(projectName, project)
	}
}

// ── Docker update ────────────────────────────────────────────────────────────

func pullRedeployDocker(projectName string, project Project) error {
	// DeployDocker already handles build → stop-old → run-new → health-check →
	// rollback on failure. Hand off directly.
	return DeployDocker(DockerDeployment{
		ProjectName:   projectName,
		ProjectPath:   project.Path,
		Dockerfile:    project.Dockerfile,
		BuildContext:  project.DockerContext,
		HostPort:      project.Port,
		ContainerPort: project.ContainerPort,
	})
}

// ── Systemd update ───────────────────────────────────────────────────────────

func pullRedeploySystemd(projectName string, project Project) error {
	serviceName := project.ServiceName
	if serviceName == "" {
		serviceName = "ezdeploy-" + projectName
	}
	servicePath := project.Path
	if project.ServiceRoot != "" && project.ServiceRoot != "." {
		servicePath = filepath.Join(project.Path, filepath.FromSlash(project.ServiceRoot))
	}

	// Snapshot before touching the running service so we have something to
	// restore if the new code crashes on startup.
	rollback, err := snapshotForRollback(servicePath, projectName)
	if err != nil {
		// Non-fatal: warn and proceed without a rollback safety net.
		fmt.Printf("[!] Rollback snapshot skipped: %v\n", err)
		rollback = ""
	}
	if project.ServiceRuntime != "" {
		entry := strings.TrimPrefix(project.ServiceEntry, strings.TrimSuffix(project.ServiceRoot, "/")+"/")
		if _, err := PrepareNativeService(servicePath, project.ServiceRuntime, entry); err != nil {
			restoreSystemdSnapshot(projectName, servicePath, rollback, serviceName)
			return fmt.Errorf("prepare updated service: %w", err)
		}
	}

	fmt.Printf("[→] Restarting %s...\n", serviceName)
	if err := run("systemctl", "restart", serviceName); err != nil {
		restoreSystemdSnapshot(projectName, servicePath, rollback, serviceName)
		return fmt.Errorf("systemctl restart %s: %w", serviceName, err)
	}

	// Poll for up to 5 seconds — enough for most apps to either bind their
	// port or crash with a config error.
	if err := waitForSystemdActive(serviceName, 5*time.Second); err != nil {
		restoreSystemdSnapshot(projectName, servicePath, rollback, serviceName)
		return fmt.Errorf("service %s failed to start after update: %w", serviceName, err)
	}

	if rollback != "" {
		_ = os.RemoveAll(rollback)
	}
	fmt.Printf("[✓] %s updated and running\n", projectName)
	return nil
}

// waitForSystemdActive polls `systemctl is-active` until the service is
// active or the deadline passes.
func waitForSystemdActive(serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := run("systemctl", "is-active", "--quiet", serviceName); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return run("systemctl", "is-active", "--quiet", serviceName)
}

// ── Rollback helpers ─────────────────────────────────────────────────────────

func snapshotForRollback(projectPath, projectName string) (string, error) {
	dest := filepath.Join(
		filepath.Dir(projectPath),
		fmt.Sprintf(".%s-rollback-%d", projectName, time.Now().Unix()),
	)
	if err := copyDir(projectPath, dest); err != nil {
		return "", fmt.Errorf("snapshot %s → %s: %w", projectPath, dest, err)
	}
	return dest, nil
}

func restoreSystemdSnapshot(projectName, projectPath, snapshot, serviceName string) {
	if snapshot == "" {
		fmt.Printf("[!] No rollback snapshot available for %s\n", projectName)
		return
	}
	fmt.Printf("[→] Restoring rollback snapshot for %s...\n", projectName)
	_ = run("systemctl", "stop", serviceName)
	_ = os.RemoveAll(projectPath)
	if err := copyDir(snapshot, projectPath); err != nil {
		fmt.Printf("[!] Rollback copy failed: %v — manual intervention needed\n", err)
		return
	}
	_ = os.RemoveAll(snapshot)
	if err := run("systemctl", "start", serviceName); err != nil {
		fmt.Printf("[!] Rollback service start failed: %v — manual intervention needed\n", err)
		return
	}
	fmt.Printf("[✓] Rolled back %s to previous version\n", projectName)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

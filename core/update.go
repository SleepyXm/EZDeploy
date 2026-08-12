package core

import (
	"errors"
	"fmt"
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
	rollback, err := BeginDeploymentRollback(projectName, project.Path)
	if err != nil {
		return err
	}
	fail := func(deployErr error) error {
		return errors.Join(deployErr, rollback.Restore())
	}

	// ── 1. Git pull ──────────────────────────────────────────────────────────
	// Re-use the SSH key stored at deploy time so private repos keep working.
	opts := CloneOptions{
		Branch: project.Branch,
		SSHKey: project.SSHKey, // empty string = public repo, falls back to HTTPS
	}
	if _, err := CloneRepoWithOptions(project.RepoURL, opts); err != nil {
		return fail(fmt.Errorf("git pull: %w", err))
	}
	fmt.Printf("[✓] Pulled latest %s\n", project.Branch)

	// ── 2. Redeploy by runtime ───────────────────────────────────────────────
	switch project.Runtime {
	case "docker":
		err = pullRedeployDocker(projectName, project)
	default:
		err = pullRedeploySystemd(projectName, project)
	}
	if err != nil {
		return fail(err)
	}
	if project.Revision, err = CurrentRevision(project.Path); err != nil {
		return fail(err)
	}
	project.Status = "deployed"
	if err := RegisterProject(projectName, project); err != nil {
		return fail(err)
	}
	return nil
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
	services := project.ManagedServices(projectName)
	if len(services) == 0 {
		return fmt.Errorf("project %s has no registered services", projectName)
	}
	for _, service := range services {
		servicePath := filepath.Join(project.Path, filepath.FromSlash(service.Root))
		if service.Runtime != "" {
			entry := strings.TrimPrefix(service.Entry, strings.TrimSuffix(service.Root, "/")+"/")
			if _, err := PrepareNativeService(servicePath, service.Runtime, entry); err != nil {
				return fmt.Errorf("prepare %s: %w", service.Name, err)
			}
		}
	}
	for _, service := range services {
		fmt.Printf("[→] Restarting %s...\n", service.Unit)
		if err := Run("", "systemctl", "restart", service.Unit); err != nil {
			return fmt.Errorf("systemctl restart %s: %w", service.Unit, err)
		}
	}
	for _, service := range services {
		if err := WaitForSystemdActive(service.Unit, 5*time.Second); err != nil {
			return fmt.Errorf("service %s failed to start after update: %w", service.Name, err)
		}
	}
	fmt.Printf("[✓] %s updated and running\n", projectName)
	return nil
}

// WaitForSystemdActive polls `systemctl is-active` until the service is
// active or the deadline passes.
func WaitForSystemdActive(serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := Run("", "systemctl", "is-active", "--quiet", serviceName); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return Run("", "systemctl", "is-active", "--quiet", serviceName)
}

package services

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"EZDeploy/core"
)

// writeUnit creates one unit file; activation reloads systemd once for the batch.
func writeUnit(unitName, description, projectPath, startCommand string, port int) error {
	switch {
	case unitName == "":
		return fmt.Errorf("unitName is required")
	case projectPath == "":
		return fmt.Errorf("projectPath is required")
	case startCommand == "":
		return fmt.Errorf("startCommand is required")
	case port == 0:
		return fmt.Errorf("port is required")
	}
	serviceUser, uid, gid, err := deploymentUser()
	if err != nil {
		return err
	}
	// The application runs as the sudo caller, so its managed project must be writable by that account.
	if err := filepath.WalkDir(projectPath, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Lchown(path, uid, gid)
	}); err != nil {
		return fmt.Errorf("set project ownership: %w", err)
	}

	unitPath := filepath.Join(core.SystemdDir, unitName+".service")
	unit := renderUnit(description, projectPath, startCommand, port, serviceUser)

	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}

	return nil
}
func renderUnit(projectName, projectPath, startCommand string, port int, serviceUser string) string {
	envPath := filepath.Join(projectPath, ".env")
	// Percent signs are systemd specifiers; doubling preserves the user's command.
	startCommand = strings.ReplaceAll(startCommand, "%", "%%")
	return fmt.Sprintf(`[Unit]
Description=EZDeploy service for %s
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
Environment=PORT=%d
EnvironmentFile=-%s
ExecStart=/bin/sh -lc %q
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, projectName, serviceUser, projectPath, port, envPath, startCommand)
}

func deploymentUser() (string, int, int, error) {
	name := strings.TrimSpace(os.Getenv("SUDO_USER"))
	if name == "" || name == "root" {
		return "", 0, 0, fmt.Errorf("run EZDeploy with sudo from the account that should run the application")
	}
	account, err := user.Lookup(name)
	if err != nil {
		return "", 0, 0, fmt.Errorf("look up service user %q: %w", name, err)
	}
	uid, uidErr := strconv.Atoi(account.Uid)
	gid, gidErr := strconv.Atoi(account.Gid)
	if uidErr != nil || gidErr != nil {
		return "", 0, 0, fmt.Errorf("invalid service account IDs for %q", name)
	}
	return account.Username, uid, gid, nil
}

// Action is the single project-level systemd action path.
func Action(projectName, actionName string) error {
	status, ok := map[string]string{"start": "running", "stop": "stopped", "restart": "restarted", "reload": "reloaded"}[actionName]
	if !ok {
		return fmt.Errorf("unsupported service action %q", actionName)
	}
	reg, err := core.GetRegistry()
	if err != nil {
		return err
	}
	project, ok := reg[projectName]
	if !ok {
		return fmt.Errorf("%s not found in registry", projectName)
	}
	managed := project.ManagedServices(projectName)
	if len(managed) == 0 {
		return fmt.Errorf("%s has no systemd services", projectName)
	}
	if project.Runtime == "docker" {
		if actionName == "reload" {
			return fmt.Errorf("Docker services do not support reload")
		}
		if err := core.DockerAction(projectName, actionName); err != nil {
			return err
		}
		managed[0].Status = status
		return core.RegisterProject(projectName, core.Project{Services: managed, Status: status})
	}
	if err := control(managed, actionName); err != nil {
		return err
	}
	for i := range managed {
		managed[i].Status = status
	}
	return core.RegisterProject(projectName, core.Project{Services: managed, Status: status})
}

// ActivateProject is the only Docker/systemd switch used by a fresh deployment.
func ActivateProject(projectName, projectPath string, previous core.Project, docker *core.DockerDeployment, native []core.Service, rollback *core.DeploymentRollback) error {
	if docker != nil {
		old := previous.ManagedServices(projectName)
		if previous.Runtime != "docker" {
			if err := control(old, "stop"); err != nil {
				return err
			}
		}
		if err := core.DeployDocker(*docker); err != nil {
			return err
		}
		rollback.TrackDocker()
		for _, service := range old {
			if service.Unit != "" {
				if err := removeUnit(service.Unit); err != nil {
					return err
				}
			}
		}
		if previous.Runtime != "docker" && len(old) > 0 {
			return core.Run("", "systemctl", "daemon-reload")
		}
		return nil
	}
	if previous.Runtime == "docker" {
		if err := core.DockerAction(projectName, "stop"); err != nil {
			return err
		}
	}
	wanted := map[string]bool{}
	for _, service := range native {
		wanted[service.Unit] = true
		if err := rollback.TrackUnit(service.Unit); err != nil {
			return err
		}
		path := filepath.Join(projectPath, filepath.FromSlash(service.Root))
		if err := writeUnit(service.Unit, projectName+" / "+service.Name, path, service.StartCommand, service.Port); err != nil {
			return err
		}
	}
	for _, service := range previous.ManagedServices(projectName) {
		if service.Unit != "" && !wanted[service.Unit] {
			if err := removeUnit(service.Unit); err != nil {
				return err
			}
		}
	}
	if err := core.Run("", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	for _, service := range native {
		if err := core.Run("", "systemctl", "enable", service.Unit); err != nil {
			return err
		}
		if err := core.Run("", "systemctl", "restart", service.Unit); err != nil {
			return err
		}
		if err := waitForSystemdActive(service.Unit, 5*time.Second); err != nil {
			return fmt.Errorf("service %s failed to start: %w", service.Name, err)
		}
	}
	return nil
}

// waitForSystemdActive confirms that a restarted unit stays active before deployment continues.
func waitForSystemdActive(serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := core.Run("", "systemctl", "is-active", "--quiet", serviceName); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return core.Run("", "systemctl", "is-active", "--quiet", serviceName)
}

func control(managed []core.Service, actionName string) error {
	for _, service := range managed {
		if service.Unit == "" {
			return fmt.Errorf("service %s is not managed by systemd", service.Name)
		}
		if err := core.Run("", "systemctl", actionName, service.Unit); err != nil {
			return fmt.Errorf("systemctl %s %s: %w", actionName, service.Unit, err)
		}
	}
	return nil
}
func removeUnit(unitName string) error {
	_ = core.Run("", "systemctl", "disable", "--now", unitName)
	if err := os.Remove(filepath.Join(core.SystemdDir, unitName+".service")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

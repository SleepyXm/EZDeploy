package services

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"EZDeploy/core"
)

const systemdDir = "/etc/systemd/system"

// Create writes and enables the unit; deployment owns the registry update.
func Create(projectName, projectPath, startCommand string, port int) (string, error) {
	if projectName == "" {
		return "", fmt.Errorf("projectName is required")
	}
	if projectPath == "" {
		return "", fmt.Errorf("projectPath is required")
	}
	if startCommand == "" {
		return "", fmt.Errorf("startCommand is required")
	}
	if port == 0 {
		return "", fmt.Errorf("port is required")
	}
	serviceUser, uid, gid, err := deploymentUser()
	if err != nil {
		return "", err
	}
	// The application runs as the sudo caller, so its managed project must be writable by that account.
	if err := filepath.WalkDir(projectPath, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return os.Lchown(path, uid, gid)
	}); err != nil {
		return "", fmt.Errorf("set project ownership: %w", err)
	}

	name := core.ManagedName(projectName)
	unitPath := filepath.Join(systemdDir, name+".service")

	unit := renderUnit(projectName, projectPath, startCommand, port, serviceUser)

	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return "", fmt.Errorf("write systemd unit: %w", err)
	}

	if err := core.Run("", "systemctl", "daemon-reload"); err != nil {
		return "", fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	if err := core.Run("", "systemctl", "enable", name); err != nil {
		return "", fmt.Errorf("systemctl enable %s: %w", name, err)
	}

	fmt.Printf("[✓] Service created: %s\n", name)
	return name, nil
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
	name, err := registeredName(projectName)
	if err != nil {
		return err
	}

	if err := core.Run("", "systemctl", actionName, name); err != nil {
		return fmt.Errorf("systemctl %s %s: %w", actionName, name, err)
	}

	return core.RegisterProject(projectName, core.Project{
		ServiceName: name,
		Status:      status,
	})
}

func registeredName(projectName string) (string, error) {
	reg, err := core.GetRegistry()
	if err != nil {
		return "", err
	}

	p, ok := reg[projectName]
	if !ok {
		return "", fmt.Errorf("%s not found in registry", projectName)
	}

	if p.ServiceName != "" {
		return p.ServiceName, nil
	}

	return core.ManagedName(projectName), nil
}

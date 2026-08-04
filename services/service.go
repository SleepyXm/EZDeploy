package services

import (
	"EZDeploy/core"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const systemdDir = "/etc/systemd/system"

func Create(projectName, projectPath, startCommand string, port int) error {
	if projectName == "" {
		return fmt.Errorf("projectName is required")
	}
	if projectPath == "" {
		return fmt.Errorf("projectPath is required")
	}
	if startCommand == "" {
		return fmt.Errorf("startCommand is required")
	}
	if port == 0 {
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

	name := Name(projectName)
	unitPath := filepath.Join(systemdDir, name+".service")

	unit := renderUnit(projectName, projectPath, startCommand, port, serviceUser)

	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}

	if err := run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	if err := run("systemctl", "enable", name); err != nil {
		return fmt.Errorf("systemctl enable %s: %w", name, err)
	}

	if err := core.RegisterProject(projectName, core.Project{
		Path:         projectPath,
		Port:         port,
		ServiceName:  name,
		StartCommand: startCommand,
		Status:       "service_created",
	}); err != nil {
		return err
	}

	fmt.Printf("[✓] Service created: %s\n", name)
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

func Start(projectName string) error {
	return action(projectName, "start", "running")
}

func Stop(projectName string) error {
	return action(projectName, "stop", "stopped")
}

func Restart(projectName string) error {
	return action(projectName, "restart", "restarted")
}

func Reload(projectName string) error {
	return action(projectName, "reload", "reloaded")
}

func Status(projectName string) error {
	name, err := registeredName(projectName)
	if err != nil {
		return err
	}

	cmd := exec.Command("systemctl", "status", name, "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Logs(projectName string) error {
	name, err := registeredName(projectName)
	if err != nil {
		return err
	}

	cmd := exec.Command("journalctl", "-u", name, "-n", "80", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func action(projectName, actionName, status string) error {
	name, err := registeredName(projectName)
	if err != nil {
		return err
	}

	if err := run("systemctl", actionName, name); err != nil {
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

	return Name(projectName), nil
}

func Name(projectName string) string {
	name := strings.ToLower(projectName)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return "ezdeploy-" + name
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

package services

import (
	"EZDeploy/core"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	name := Name(projectName)
	envPath := filepath.Join(projectPath, ".env")
	unitPath := filepath.Join(systemdDir, name+".service")

	unit := fmt.Sprintf(`[Unit]
Description=EZDeploy service for %s
After=network.target

[Service]
Type=simple
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
`, projectName, projectPath, port, envPath, startCommand)

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

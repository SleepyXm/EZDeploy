package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DockerDeployment struct {
	ProjectName   string
	ProjectPath   string
	Dockerfile    string
	BuildContext  string
	HostPort      int
	ContainerPort int
	KeepPrevious  bool // the outer transaction removes the backup only after Nginx and registry succeed
}

// DeployDocker builds before touching the running container, then keeps the
// previous container available until the replacement is confirmed running.
func DeployDocker(deployment DockerDeployment) error {
	if !projectNamePattern.MatchString(deployment.ProjectName) {
		return fmt.Errorf("invalid project name %q", deployment.ProjectName)
	}
	if deployment.HostPort < 1 || deployment.HostPort > 65535 || deployment.ContainerPort < 1 || deployment.ContainerPort > 65535 {
		return fmt.Errorf("invalid Docker port mapping %d:%d", deployment.HostPort, deployment.ContainerPort)
	}
	dockerfile, err := projectFile(deployment.ProjectPath, deployment.Dockerfile, false)
	if err != nil {
		return err
	}
	contextPath, err := projectFile(deployment.ProjectPath, deployment.BuildContext, true)
	if err != nil {
		return err
	}

	containerName, imageName := dockerNames(deployment.ProjectName)
	fmt.Printf("[→] Building %s from %s...\n", imageName, deployment.Dockerfile)
	if err := runHeavy("", "Building Docker image", "docker", "build", "--file", dockerfile, "--tag", imageName, contextPath); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	oldExists, oldRunning, err := dockerContainerState(containerName)
	if err != nil {
		return err
	}
	backupName := containerName + "-previous"
	if oldExists {
		if backupExists, _, err := dockerContainerState(backupName); err != nil {
			return err
		} else if backupExists {
			if err := Run("", "docker", "rm", "--force", backupName); err != nil {
				return fmt.Errorf("remove stale Docker rollback container: %w", err)
			}
		}
		if err := Run("", "docker", "stop", containerName); err != nil {
			return fmt.Errorf("stop current Docker container: %w", err)
		}
		if err := Run("", "docker", "rename", containerName, backupName); err != nil {
			_ = Run("", "docker", "start", containerName)
			return fmt.Errorf("prepare Docker rollback container: %w", err)
		}
	}

	runArgs := dockerRunArgs(deployment, containerName, imageName)
	if err := Run("", "docker", runArgs...); err != nil {
		restoreDockerContainer(containerName, backupName, oldExists, oldRunning)
		return fmt.Errorf("docker run: %w", err)
	}
	_, running, err := dockerContainerState(containerName)
	if err != nil || !running {
		restoreDockerContainer(containerName, backupName, oldExists, oldRunning)
		if err != nil {
			return err
		}
		return fmt.Errorf("Docker container %s stopped during startup", containerName)
	}
	if oldExists && !deployment.KeepPrevious {
		_ = Run("", "docker", "rm", backupName)
	}
	fmt.Printf("[✓] Docker container %s is running on 127.0.0.1:%d\n", containerName, deployment.HostPort)
	return nil
}

func dockerRunArgs(deployment DockerDeployment, containerName, imageName string) []string {
	args := []string{
		"run", "--detach", "--name", containerName,
		"--restart", "unless-stopped",
		"--label", "com.ezdeploy.project=" + deployment.ProjectName,
		"--publish", fmt.Sprintf("127.0.0.1:%d:%d", deployment.HostPort, deployment.ContainerPort),
	}
	if envPath := filepath.Join(deployment.ProjectPath, ".env"); FileExists(envPath) {
		args = append(args, "--env-file", envPath)
	}
	// Explicit PORT wins even when the project's env file also defines it.
	args = append(args, "--env", fmt.Sprintf("PORT=%d", deployment.ContainerPort))
	return append(args, imageName)
}

func projectFile(root, relative string, directory bool) (string, error) {
	relative = strings.TrimSpace(relative)
	if relative == "" && directory {
		relative = "."
	}
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("project path must be relative: %q", relative)
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project path escapes repository: %q", relative)
	}
	rootPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(rootPath, clean))
	if err != nil {
		return "", fmt.Errorf("inspect project path %s: %w", relative, err)
	}
	contained, err := filepath.Rel(rootPath, path)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("project path escapes repository: %q", relative)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect project path %s: %w", relative, err)
	}
	if info.IsDir() != directory {
		return "", fmt.Errorf("unexpected project path type: %s", relative)
	}
	return path, nil
}

func dockerNames(projectName string) (string, string) {
	container := ManagedName(projectName)
	return container, "ezdeploy/" + strings.TrimPrefix(container, "ezdeploy-") + ":latest"
}

// dockerContainerState is the single inspection path for existence and state.
func dockerContainerState(name string) (bool, bool, error) {
	output, err := exec.Command("docker", "container", "inspect", "--format", "{{.State.Running}}", name).CombinedOutput()
	if err == nil {
		return true, strings.TrimSpace(string(output)) == "true", nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no such") {
		return false, false, nil
	}
	return false, false, fmt.Errorf("inspect Docker container %s: %w: %s", name, err, strings.TrimSpace(string(output)))
}

func restoreDockerContainer(name, backup string, hadBackup, wasRunning bool) {
	_ = Run("", "docker", "rm", "--force", name)
	if hadBackup {
		_ = Run("", "docker", "rename", backup, name)
		if wasRunning {
			_ = Run("", "docker", "start", name)
		}
	}
}

// DockerAction controls the one container owned by a project.
func DockerAction(projectName, action string) error {
	name, _ := dockerNames(projectName)
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("unsupported Docker action %q", action)
	}
	return Run("", "docker", action, name)
}

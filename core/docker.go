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
	if err := run("docker", "build", "--file", dockerfile, "--tag", imageName, contextPath); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}

	oldExists, err := dockerContainerExists(containerName)
	if err != nil {
		return err
	}
	backupName := containerName + "-previous"
	if oldExists {
		if backupExists, err := dockerContainerExists(backupName); err != nil {
			return err
		} else if backupExists {
			if err := run("docker", "rm", "--force", backupName); err != nil {
				return fmt.Errorf("remove stale Docker rollback container: %w", err)
			}
		}
		if err := run("docker", "stop", containerName); err != nil {
			return fmt.Errorf("stop current Docker container: %w", err)
		}
		if err := run("docker", "rename", containerName, backupName); err != nil {
			_ = run("docker", "start", containerName)
			return fmt.Errorf("prepare Docker rollback container: %w", err)
		}
	}

	runArgs := dockerRunArgs(deployment, containerName, imageName)
	if err := run("docker", runArgs...); err != nil {
		restoreDockerContainer(containerName, backupName, oldExists)
		return fmt.Errorf("docker run: %w", err)
	}
	running, err := dockerContainerRunning(containerName)
	if err != nil || !running {
		restoreDockerContainer(containerName, backupName, oldExists)
		if err != nil {
			return err
		}
		return fmt.Errorf("Docker container %s stopped during startup", containerName)
	}
	if oldExists {
		_ = run("docker", "rm", backupName)
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
	if envPath := filepath.Join(deployment.ProjectPath, ".env"); fileExists(envPath) {
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
	slug := strings.ToLower(strings.ReplaceAll(projectName, "_", "-"))
	return "ezdeploy-" + slug, "ezdeploy/" + slug + ":latest"
}

func dockerContainerExists(name string) (bool, error) {
	output, err := exec.Command("docker", "container", "inspect", name).CombinedOutput()
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no such") {
		return false, nil
	}
	return false, fmt.Errorf("inspect Docker container %s: %w: %s", name, err, strings.TrimSpace(string(output)))
}

func dockerContainerRunning(name string) (bool, error) {
	output, err := exec.Command("docker", "container", "inspect", "--format", "{{.State.Running}}", name).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect Docker container state: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

func restoreDockerContainer(name, backup string, hadBackup bool) {
	_ = run("docker", "rm", "--force", name)
	if hadBackup {
		_ = run("docker", "rename", backup, name)
		_ = run("docker", "start", name)
	}
}

func StopDocker(projectName string) error {
	name, _ := dockerNames(projectName)
	exists, err := dockerContainerExists(name)
	if err != nil || !exists {
		return err
	}
	return run("docker", "stop", name)
}

func StartDocker(projectName string) error {
	name, _ := dockerNames(projectName)
	return run("docker", "start", name)
}

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SystemdDir is shared by unit creation and rollback capture.
const SystemdDir = "/etc/systemd/system"

type fileState struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

// DeploymentRollback captures the project state that one deploy invocation may mutate.
type DeploymentRollback struct {
	projectName, projectPath, revision string
	previous                           Project
	nginx                              fileState
	files                              map[string]fileState
	units                              map[string]bool
	hadPath, hadLink                   bool
	docker                             bool
}

// TrackDocker marks a replacement container for removal during rollback.
func (r *DeploymentRollback) TrackDocker() { r.docker = true }

// BeginDeploymentRollback snapshots code, registry metadata, Nginx, and existing units.
func BeginDeploymentRollback(projectName, projectPath string) (*DeploymentRollback, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, err
	}
	r := &DeploymentRollback{projectName: projectName, projectPath: absPath, files: map[string]fileState{}, units: map[string]bool{}}
	reg, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	r.previous = reg[projectName]
	if err := r.TrackFile(registryPath); err != nil {
		return nil, err
	}
	r.nginx, err = captureFile(filepath.Join(nginxSitesAvailable, projectName))
	if err != nil {
		return nil, err
	}
	_, linkErr := os.Lstat(filepath.Join(nginxSitesEnabled, projectName))
	r.hadLink = linkErr == nil
	if linkErr != nil && !os.IsNotExist(linkErr) {
		return nil, linkErr
	}
	if info, statErr := os.Stat(absPath); statErr == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("project path is not a directory: %s", absPath)
		}
		r.hadPath = true
		r.revision, err = CurrentRevision(absPath)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s revision: %w", projectName, err)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	for _, service := range r.previous.ManagedServices(projectName) {
		if service.Unit != "" {
			if err := r.TrackUnit(service.Unit); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// TrackUnit makes units created or replaced during deployment reversible.
func (r *DeploymentRollback) TrackUnit(unitName string) error {
	r.units[unitName] = true
	return r.TrackFile(filepath.Join(SystemdDir, unitName+".service"))
}

// TrackFile preserves a deployment-owned file such as a service environment.
func (r *DeploymentRollback) TrackFile(path string) error {
	// A failed first deployment removes its clone; restoring files inside it would recreate debris.
	if !r.hadPath && (path == r.projectPath || strings.HasPrefix(path, r.projectPath+string(filepath.Separator))) {
		return nil
	}
	if _, exists := r.files[path]; exists {
		return nil
	}
	state, err := captureFile(path)
	if err == nil {
		r.files[path] = state
	}
	return err
}

// Restore returns one project to the code, units, Nginx, and registry state from before deployment.
func (r *DeploymentRollback) Restore() error {
	var failures []error
	add := func(err error) {
		if err != nil {
			failures = append(failures, err)
		}
	}
	for unit := range r.units {
		_ = Run("", "systemctl", "stop", unit)
	}
	if r.docker {
		_ = Run("", "docker", "rm", "--force", ManagedName(r.projectName))
	}
	if r.hadPath {
		if r.previous.Branch != "" {
			add(Run(r.projectPath, "git", "checkout", r.previous.Branch))
		}
		add(Run(r.projectPath, "git", "reset", "--hard", r.revision))
	} else {
		add(os.RemoveAll(r.projectPath))
	}
	for _, state := range r.files {
		add(restoreFile(state))
	}
	add(restoreFile(r.nginx))
	linkPath := filepath.Join(nginxSitesEnabled, r.projectName)
	if r.hadLink {
		if _, err := os.Lstat(linkPath); os.IsNotExist(err) {
			if err := os.Symlink(r.nginx.path, linkPath); err != nil {
				add(err)
			}
		}
	} else if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		add(err)
	}
	_ = Run("", "systemctl", "daemon-reload")
	for _, service := range r.previous.ManagedServices(r.projectName) {
		if service.Unit != "" {
			if err := Run("", "systemctl", "restart", service.Unit); err != nil {
				add(err)
			}
		}
	}
	if r.previous.Runtime == "docker" {
		if err := DeployDocker(DockerDeployment{
			ProjectName: r.projectName, ProjectPath: r.projectPath,
			Dockerfile: r.previous.Dockerfile, BuildContext: r.previous.DockerContext,
			HostPort: r.previous.Port, ContainerPort: r.previous.ContainerPort,
		}); err != nil {
			add(err)
		}
	}
	if r.nginx.exists {
		if err := Run("", "nginx", "-t"); err == nil {
			_ = Run("", "systemctl", "reload", "nginx")
		} else {
			add(err)
		}
	}
	return errors.Join(failures...)
}
func captureFile(path string) (fileState, error) {
	state := fileState{path: path}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.data, err = os.ReadFile(path)
	state.mode, state.exists = info.Mode(), err == nil
	return state, err
}
func restoreFile(state fileState) error {
	if !state.exists {
		if err := os.Remove(state.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(state.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(state.path, state.data, state.mode)
}

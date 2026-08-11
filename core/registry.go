package core

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	registryPath = "./registry.json"
	startingPort = 8000
)

// Project holds the metadata for a deployed project.
type Project struct {
	Path           string `json:"path"`
	Port           int    `json:"port"`
	Domain         string `json:"domain"`
	Email          string `json:"email,omitempty"`
	RepoURL        string `json:"repo_url"`
	Branch         string `json:"branch"`
	Status         string `json:"status"`
	ServiceName    string `json:"service_name"`
	StartCommand   string `json:"start_command"`
	Runtime        string `json:"runtime,omitempty"`
	Dockerfile     string `json:"dockerfile,omitempty"`
	DockerContext  string `json:"docker_context,omitempty"`
	ContainerPort  int    `json:"container_port,omitempty"`
	ServiceRoot    string `json:"service_root,omitempty"`
	ServiceEntry   string `json:"service_entry,omitempty"`
	ServiceRuntime string `json:"service_runtime,omitempty"`
	SigningKey     string `json:"signing_key,omitempty"` // Ed25519 private key seed (base64) — never logged
	SSHKey         string `json:"ssh_key,omitempty"`     // path to SSH private key for private repos
}

// Registry maps project names to their metadata.
type Registry map[string]Project

func loadRegistry() (Registry, error) {
	if !fileExists(registryPath) {
		return Registry{}, nil
	}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return reg, nil
}

func saveRegistry(reg Registry) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		return fmt.Errorf("write registry: %w", err)
	}
	return nil
}

// GetNextPort returns the lowest available port >= startingPort.
func GetNextPort() (int, error) {
	reg, err := loadRegistry()
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool, len(reg))
	for _, p := range reg {
		used[p.Port] = true
	}
	port := startingPort
	for used[port] {
		port++
	}
	return port, nil
}

// RegisterProject adds or updates a project entry in the registry.
func RegisterProject(projectName string, project Project) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	existing := reg[projectName]

	if project.Path != "" {
		existing.Path = project.Path
	}

	if project.Port != 0 {
		existing.Port = project.Port
	} else if existing.Port == 0 {
		port, err := GetNextPort()
		if err != nil {
			return err
		}
		existing.Port = port
	}

	if project.Domain != "" {
		existing.Domain = project.Domain
	}
	if project.Email != "" {
		existing.Email = project.Email
	}

	if project.RepoURL != "" {
		existing.RepoURL = project.RepoURL
	}

	if project.Branch != "" {
		existing.Branch = project.Branch
	}

	if project.Runtime != "" {
		existing.Runtime = project.Runtime
		existing.ServiceName = project.ServiceName
		existing.StartCommand = project.StartCommand
		existing.Dockerfile = project.Dockerfile
		existing.DockerContext = project.DockerContext
		existing.ContainerPort = project.ContainerPort
		existing.ServiceRoot = project.ServiceRoot
		existing.ServiceEntry = project.ServiceEntry
		existing.ServiceRuntime = project.ServiceRuntime
	} else {
		if project.ServiceName != "" {
			existing.ServiceName = project.ServiceName
		}
		if project.StartCommand != "" {
			existing.StartCommand = project.StartCommand
		}
	}

	if project.Status != "" {
		existing.Status = project.Status
	}

	reg[projectName] = existing

	if err := saveRegistry(reg); err != nil {
		return err
	}

	fmt.Printf("[✓] Registered %s on port %d\n", projectName, existing.Port)
	return nil
}

// GetRegistry returns the current registry state.
func GetRegistry() (Registry, error) {
	return loadRegistry()
}

func GetProjects() (Registry, error) {
	return GetRegistry()
}

// UnregisterProject removes a project from the registry.
// Prints a warning and returns nil if the project does not exist.
func UnregisterProject(projectName string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	if _, ok := reg[projectName]; !ok {
		fmt.Printf("[!] %s not found in registry\n", projectName)
		return nil
	}
	delete(reg, projectName)
	if err := saveRegistry(reg); err != nil {
		return err
	}
	fmt.Printf("[✓] Unregistered %s\n", projectName)
	return nil
}

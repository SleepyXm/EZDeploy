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
	Path           string    `json:"path"`
	Port           int       `json:"port"`
	Domain         string    `json:"domain"`
	Email          string    `json:"email,omitempty"`
	RepoURL        string    `json:"repo_url"`
	Branch         string    `json:"branch"`
	Status         string    `json:"status"`
	ServiceName    string    `json:"service_name"`
	StartCommand   string    `json:"start_command"`
	Runtime        string    `json:"runtime,omitempty"`
	Dockerfile     string    `json:"dockerfile,omitempty"`
	DockerContext  string    `json:"docker_context,omitempty"`
	ContainerPort  int       `json:"container_port,omitempty"`
	ServiceRoot    string    `json:"service_root,omitempty"`
	ServiceEntry   string    `json:"service_entry,omitempty"`
	ServiceRuntime string    `json:"service_runtime,omitempty"`
	Revision       string    `json:"revision,omitempty"`
	Services       []Service `json:"services,omitempty"`
	SigningKey     string    `json:"signing_key,omitempty"` // Ed25519 private key seed (base64) — never logged
	SSHKey         string    `json:"ssh_key,omitempty"`     // path to SSH private key for private repos
}

// Service is one independently managed process inside a repository project.
type Service struct {
	Name         string   `json:"name"`
	Root         string   `json:"root"`
	Entry        string   `json:"entry,omitempty"`
	Runtime      string   `json:"runtime"`
	StartCommand string   `json:"start_command,omitempty"`
	Unit         string   `json:"unit,omitempty"`
	Port         int      `json:"port"`
	Routes       []string `json:"routes,omitempty"`
	Status       string   `json:"status,omitempty"`
}

// ManagedServices reads new multi-service records and migrates legacy projects in memory.
func (p Project) ManagedServices(projectName string) []Service {
	if len(p.Services) > 0 {
		return append([]Service(nil), p.Services...)
	}
	if p.Runtime == "docker" {
		return []Service{{Name: projectName, Root: ".", Runtime: "docker", Port: p.Port, Status: p.Status}}
	}
	if p.ServiceName == "" && p.ServiceRoot == "" && p.ServiceEntry == "" {
		return nil
	}
	unit := p.ServiceName
	if unit == "" {
		unit = ManagedName(projectName)
	}
	return []Service{{
		Name: projectName, Root: p.ServiceRoot, Entry: p.ServiceEntry,
		Runtime: p.ServiceRuntime, StartCommand: p.StartCommand,
		Unit: unit, Port: p.Port, Status: p.Status,
	}}
}

// Registry maps project names to their metadata.
type Registry map[string]Project

func loadRegistry() (Registry, error) {
	if !FileExists(registryPath) {
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

// GetNextPorts reserves no state; it returns the lowest currently unused ports.
func GetNextPorts(count int) ([]int, error) {
	reg, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	used := map[int]bool{}
	for name, p := range reg {
		used[p.Port] = true
		for _, service := range p.ManagedServices(name) {
			used[service.Port] = true
		}
	}
	ports := make([]int, 0, count)
	for port := startingPort; len(ports) < count; port++ {
		if !used[port] {
			ports = append(ports, port)
		}
	}
	return ports, nil
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
		ports, err := GetNextPorts(1)
		if err != nil {
			return err
		}
		existing.Port = ports[0]
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
	if project.Revision != "" {
		existing.Revision = project.Revision
	}
	if project.Services != nil {
		existing.Services = append([]Service(nil), project.Services...)
		if len(existing.Services) == 1 {
			service := existing.Services[0]
			existing.Port, existing.ServiceName, existing.StartCommand = service.Port, service.Unit, service.StartCommand
			existing.ServiceRoot, existing.ServiceEntry, existing.ServiceRuntime = service.Root, service.Entry, service.Runtime
		} else {
			existing.ServiceName, existing.StartCommand = "", ""
			existing.ServiceRoot, existing.ServiceEntry, existing.ServiceRuntime = "", "", ""
		}
	}

	if project.Status != "" {
		existing.Status = project.Status
	}

	reg[projectName] = existing

	if err := saveRegistry(reg); err != nil {
		return err
	}

	fmt.Printf("[✓] Registered %s with %d service(s)\n", projectName, len(existing.ManagedServices(projectName)))
	return nil
}

// GetRegistry returns the current registry state.
func GetRegistry() (Registry, error) {
	return loadRegistry()
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

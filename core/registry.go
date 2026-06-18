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
	Path    string `json:"path"`
	Port    int    `json:"port"`
	Domain  string `json:"domain"`
	RepoURL string `json:"repo_url"`
	Branch  string `json:"branch"`
	Status  string `json:"status"`
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

	if project.RepoURL != "" {
		existing.RepoURL = project.RepoURL
	}

	if project.Branch != "" {
		existing.Branch = project.Branch
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

func EnsureProject(projectName, path, repoURL, branch string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	existing, ok := reg[projectName]
	if !ok {
		port, err := GetNextPort()
		if err != nil {
			return err
		}

		reg[projectName] = Project{
			Path:    path,
			Port:    port,
			Domain:  "",
			RepoURL: repoURL,
			Branch:  branch,
			Status:  "cloned",
		}

		if err := saveRegistry(reg); err != nil {
			return err
		}

		fmt.Printf("[✓] Added %s to registry on port %d\n", projectName, port)
		return nil
	}

	if existing.Path == "" {
		existing.Path = path
	}
	if existing.RepoURL == "" {
		existing.RepoURL = repoURL
	}
	if existing.Branch == "" {
		existing.Branch = branch
	}
	if existing.Status == "" {
		existing.Status = "cloned"
	}

	reg[projectName] = existing

	if err := saveRegistry(reg); err != nil {
		return err
	}

	fmt.Printf("[✓] Registry already has %s\n", projectName)
	return nil
}

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
func RegisterProject(projectName string, port int, domain, repoURL, branch string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	reg[projectName] = Project{
		Port:    port,
		Domain:  domain,
		RepoURL: repoURL,
		Branch:  branch,
		Status:  "running",
	}
	if err := saveRegistry(reg); err != nil {
		return err
	}
	fmt.Printf("[✓] Registered %s on port %d\n", projectName, port)
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

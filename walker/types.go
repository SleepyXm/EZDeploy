package walker

import "sort"

type EnvHit struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
}

// RouteHit records the literal route declaration found in source code.
type RouteHit struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Rule   string `json:"rule"`
}

// DockerfileInfo contains only build metadata that can be read without executing it.
type DockerfileInfo struct {
	Path         string `json:"path"`
	BaseImage    string `json:"base_image"`
	ExposedPorts []int  `json:"exposed_ports"`
}

// ServiceCandidate describes a backend entrypoint without treating detection
// as permission to deploy it. Evidence and confidence keep filename-based
// guesses visible to the user instead of silently turning them into services.
type ServiceCandidate struct {
	Name         string   `json:"name"`
	Runtime      string   `json:"runtime"`
	Root         string   `json:"root"`
	Entry        string   `json:"entry"`
	StartCommand string   `json:"start_command,omitempty"`
	Confidence   string   `json:"confidence"`
	Evidence     []string `json:"evidence"`
}

type Report struct {
	Root         string             `json:"root"`
	FilesScanned int                `json:"files_scanned"`
	Languages    map[string]int     `json:"languages"`
	EnvHits      []EnvHit           `json:"env_hits"`
	RouteHits    []RouteHit         `json:"route_hits"`
	Dockerfiles  []DockerfileInfo   `json:"dockerfiles"`
	Services     []ServiceCandidate `json:"services"`
}

func (r Report) UniqueRoutePaths() []string {
	seen := map[string]bool{}
	for _, hit := range r.RouteHits {
		seen[hit.Path] = true
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (r Report) UniqueEnvNames() []string {
	seen := map[string]struct{}{}

	for _, hit := range r.EnvHits {
		if hit.Name == "" {
			continue
		}

		seen[hit.Name] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

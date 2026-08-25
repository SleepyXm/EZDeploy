package walker

import (
	"sort"
	"strings"
)

type EnvHit struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
}

// RouteHit records the literal route declaration found in source code.
type RouteHit struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Rule      string `json:"rule"`
	MountRoot bool   `json:"-"` // Empty child paths belong to the exact root of a mounted group.
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
	return uniqueRoutePaths(r.RouteHits)
}

// RouteHitsUnder returns only routes declared inside a selected service root.
// This prevents one microservice from inheriting another service's Nginx paths.
func (r Report) RouteHitsUnder(root string) []RouteHit {
	root = strings.Trim(strings.TrimSpace(root), "/")
	if root == "" || root == "." {
		return append([]RouteHit(nil), r.RouteHits...)
	}
	prefix := root + "/"
	var hits []RouteHit
	for _, hit := range r.RouteHits {
		if hit.File == root || strings.HasPrefix(hit.File, prefix) {
			hits = append(hits, hit)
		}
	}
	return hits
}

// RouteHitsForService removes routes owned by nested detected services. This
// matters when a repository-root frontend contains separate backend folders.
func (r Report) RouteHitsForService(service ServiceCandidate) []RouteHit {
	hits := r.RouteHitsUnder(service.Root)
	filtered := hits[:0]
	for _, hit := range hits {
		ownedByNestedService := false
		for _, other := range r.Services {
			if other.Entry == service.Entry || other.Root == "." || other.Root == service.Root {
				continue
			}
			if pathUnderRoot(other.Root, service.Root) && pathUnderRoot(hit.File, other.Root) {
				ownedByNestedService = true
				break
			}
		}
		if !ownedByNestedService {
			filtered = append(filtered, hit)
		}
	}
	return filtered
}

func (r Report) UniqueRoutePathsForService(service ServiceCandidate) []string {
	return uniqueRoutePaths(r.RouteHitsForService(service))
}

func pathUnderRoot(path, root string) bool {
	root = strings.Trim(strings.TrimSpace(root), "/")
	if root == "" || root == "." {
		return true
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

func uniqueRoutePaths(hits []RouteHit) []string {
	seen := map[string]bool{}
	for _, hit := range hits {
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

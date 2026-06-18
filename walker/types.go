package walker

import "sort"

type EnvHit struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
}

type Report struct {
	Root         string         `json:"root"`
	FilesScanned int            `json:"files_scanned"`
	Languages    map[string]int `json:"languages"`
	EnvHits      []EnvHit       `json:"env_hits"`
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

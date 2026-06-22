package UI

import "strings"

// repoNameFromURL mirrors the repo-name derivation in core.CloneRepo so the
// TUI's project context matches the actual on-disk clone path.
func repoNameFromURL(repoURL string) string {
	trimmed := strings.TrimRight(repoURL, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// cloneDirJoin builds the on-disk path core.CloneRepo would have used.
// Mirrors core.CloneDir; kept as a literal here to avoid exporting internals.
func cloneDirJoin(repoName string) string {
	return strings.TrimRight(cloneDirConst, "/") + "/" + repoName
}

const cloneDirConst = "./projects"

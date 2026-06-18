package UI

import "EZDeploy/core"

func activeProjectFromRegistry() *Project {
	reg, err := core.GetProjects()
	if err != nil || len(reg) == 0 {
		return nil
	}

	for name, p := range reg {
		return &Project{
			Path:    p.Path,
			Name:    name,
			RepoURL: p.RepoURL,
		}
	}

	return nil
}

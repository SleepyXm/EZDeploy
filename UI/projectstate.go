package UI

import "EZDeploy/core"

func activeProjectFromRegistry() string {
	reg, err := core.GetRegistry()
	if err != nil || len(reg) == 0 {
		return ""
	}

	for name := range reg {
		return name
	}
	return ""
}

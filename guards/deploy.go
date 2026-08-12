// Package guards validates deployment choices before EZDeploy mutates the host.
package guards

import (
	"fmt"
	"strconv"
	"strings"

	"EZDeploy/core"
)

var singleProjectFlags = map[string]bool{
	"--domain": true, "--port": true, "--start": true, "--service": true,
	"--dockerfile": true, "--docker-context": true, "--container-port": true,
	"--allow-route": true,
}

// FirstValue returns the first supplied value that is not empty.
func FirstValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// DeployArguments separates repository operands from flags and rejects a batch
// whose shared flags would have different meanings for each project.
func DeployArguments(args []string) ([]string, []string, error) {
	var repositories []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repositories, args = append(repositories, args[0]), args[1:]
	}
	if len(repositories) == 0 {
		return nil, nil, fmt.Errorf("at least one repository is required")
	}
	if len(repositories) > 1 {
		for _, arg := range args {
			if singleProjectFlags[strings.SplitN(arg, "=", 2)[0]] {
				return nil, nil, fmt.Errorf("%s is project-specific and cannot be shared across a deployment batch", arg)
			}
		}
	}
	seen := map[string]bool{}
	for _, repository := range repositories {
		name, err := core.ProjectNameFromRepoURL(repository)
		if err != nil {
			return nil, nil, err
		}
		managedName := core.ManagedName(name)
		if seen[managedName] {
			return nil, nil, fmt.Errorf("duplicate managed project name %q in deployment batch", name)
		}
		seen[managedName] = true
	}
	return repositories, args, nil
}

// SelectionIndexes parses the displayed comma-separated service numbers.
func SelectionIndexes(value string, maximum int) ([]int, error) {
	parts, seen := strings.Split(strings.TrimSpace(value), ","), map[int]bool{}
	indexes := make([]int, 0, len(parts))
	for _, part := range parts {
		choice, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || choice < 0 || choice > maximum || seen[choice] {
			return nil, fmt.Errorf("invalid service selection %q", value)
		}
		seen[choice], indexes = true, append(indexes, choice)
	}
	if seen[0] && len(indexes) > 1 {
		return nil, fmt.Errorf("repository root cannot be combined with detected services")
	}
	return indexes, nil
}

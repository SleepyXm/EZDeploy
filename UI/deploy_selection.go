package UI

import (
	"bufio"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"EZDeploy/core"
	"EZDeploy/guards"
	"EZDeploy/walker"
)

// RuntimeSelection is the validated runtime choice consumed by deployment.
type RuntimeSelection struct {
	Mode, Dockerfile, DockerContext string
	ContainerPort                   int
}

// PrintServiceCandidates shows the evidence behind every detected backend.
func PrintServiceCandidates(services []walker.ServiceCandidate) {
	if len(services) == 0 {
		fmt.Println("[→] No backend service entrypoints detected.")
		return
	}
	fmt.Println("\nDetected backend services:")
	for index, service := range services {
		fmt.Printf("  %d. %s (%s, %s confidence)\n     Root: %s\n     Entry: %s\n", index+1, service.Name, service.Runtime, service.Confidence, service.Root, service.Entry)
		if service.StartCommand != "" {
			fmt.Printf("     Likely start: %s\n", service.StartCommand)
		}
		fmt.Printf("     Evidence: %s\n", strings.Join(service.Evidence, ", "))
	}
}

// SelectServices accepts stable service identifiers or displayed indexes.
func SelectServices(reader *bufio.Reader, services []walker.ServiceCandidate, saved, selector string, nonInteractive bool) ([]walker.ServiceCandidate, error) {
	selector = guards.FirstValue(strings.TrimSpace(selector), strings.TrimSpace(saved))
	if selector != "" {
		return selectNamedServices(services, selector)
	}
	switch len(services) {
	case 0:
		return []walker.ServiceCandidate{{Name: "repository root", Root: "."}}, nil
	case 1:
		fmt.Printf("[✓] Native service: %s (%s)\n", services[0].Name, services[0].Root)
		return services, nil
	}
	if nonInteractive {
		return nil, fmt.Errorf("multiple backend services found; use --service with comma-separated names, roots, or entry files")
	}
	fmt.Println("\nNative service selection:\n  0. Repository root (manual monorepo configuration)")
	for index, service := range services {
		fmt.Printf("  %d. %s (%s) — %s\n", index+1, service.Name, service.Runtime, service.Root)
	}
	value, err := ReadPrompt(reader, fmt.Sprintf("Select services [0-%d, comma-separated]: ", len(services)))
	if err != nil {
		return nil, err
	}
	indexes, err := guards.SelectionIndexes(value, len(services))
	if err != nil {
		return nil, err
	}
	if indexes[0] == 0 {
		return []walker.ServiceCandidate{{Name: "repository root", Root: "."}}, nil
	}
	selected := make([]walker.ServiceCandidate, len(indexes))
	for index, choice := range indexes {
		selected[index] = services[choice-1]
		fmt.Printf("[✓] Native service: %s (%s)\n", selected[index].Name, selected[index].Root)
	}
	return selected, nil
}
func selectNamedServices(services []walker.ServiceCandidate, selectors string) ([]walker.ServiceCandidate, error) {
	selected, seen := []walker.ServiceCandidate{}, map[string]bool{}
	for _, selector := range strings.Split(selectors, ",") {
		service, err := matchService(services, strings.TrimSpace(selector))
		if err != nil {
			return nil, err
		}
		key := service.Root + "\x00" + service.Entry
		if seen[key] {
			return nil, fmt.Errorf("service %q was selected more than once", selector)
		}
		seen[key], selected = true, append(selected, service)
	}
	return selected, nil
}
func matchService(services []walker.ServiceCandidate, selector string) (walker.ServiceCandidate, error) {
	if selector == "." || strings.EqualFold(selector, "root") {
		return walker.ServiceCandidate{Name: "repository root", Root: "."}, nil
	}
	match := -1
	for index, service := range services {
		if service.Name == selector || service.Root == selector || service.Entry == selector {
			if match >= 0 {
				return walker.ServiceCandidate{}, fmt.Errorf("service name %q is ambiguous; use its root or entry file", selector)
			}
			match = index
		}
	}
	if match < 0 {
		return walker.ServiceCandidate{}, fmt.Errorf("service %q was not discovered", selector)
	}
	return services[match], nil
}

// SelectRuntime resolves native versus Docker without executing either runtime.
func SelectRuntime(reader *bufio.Reader, report walker.Report, existing core.Project, mode, dockerfile, context string, port int, nonInteractive bool) (RuntimeSelection, error) {
	mode, dockerfile, context = strings.ToLower(strings.TrimSpace(mode)), cleanRelative(dockerfile), cleanRelative(context)
	requestedMode := mode
	files := append([]walker.DockerfileInfo(nil), report.Dockerfiles...)
	sort.SliceStable(files, func(i, j int) bool {
		left, right := strings.Contains(strings.ToLower(filepath.Base(files[i].Path)), "prod"), strings.Contains(strings.ToLower(filepath.Base(files[j].Path)), "prod")
		return left && !right || left == right && files[i].Path < files[j].Path
	})
	mode = guards.FirstValue(mode, existing.Runtime)
	if mode == "" && existing.ServiceName != "" {
		mode = "native" // Legacy registry entries predate the runtime field.
	}
	if dockerfile != "" {
		if requestedMode == "native" {
			return RuntimeSelection{}, fmt.Errorf("--runtime native cannot be combined with --dockerfile")
		}
		mode = "docker"
	}
	if mode == "" && len(files) == 0 {
		mode = "native"
	}
	if mode == "" && nonInteractive {
		if len(files) != 1 {
			return RuntimeSelection{}, fmt.Errorf("multiple Dockerfiles found; use --dockerfile or --runtime native")
		}
		mode, dockerfile = "docker", files[0].Path
	}
	if mode == "" {
		var err error
		dockerfile, err = chooseDockerfile(reader, files, true)
		if err != nil {
			return RuntimeSelection{}, err
		}
		if dockerfile == "" {
			mode = "native"
		} else {
			mode = "docker"
		}
	}
	switch mode {
	case "native":
		if context != "" || port != 0 {
			return RuntimeSelection{}, fmt.Errorf("Docker context and container port require --runtime docker")
		}
		return RuntimeSelection{Mode: mode}, nil
	case "docker":
		return selectDocker(reader, files, existing, dockerfile, context, port, nonInteractive)
	default:
		return RuntimeSelection{}, fmt.Errorf("runtime must be native or docker, got %q", mode)
	}
}
func selectDocker(reader *bufio.Reader, files []walker.DockerfileInfo, existing core.Project, dockerfile, context string, port int, nonInteractive bool) (RuntimeSelection, error) {
	if len(files) == 0 {
		return RuntimeSelection{}, fmt.Errorf("Docker runtime selected but no Dockerfile was discovered")
	}
	if existing.Runtime == "docker" {
		dockerfile = guards.FirstValue(dockerfile, existing.Dockerfile)
	}
	if dockerfile == "" {
		if len(files) == 1 {
			dockerfile = files[0].Path
		} else if nonInteractive {
			return RuntimeSelection{}, fmt.Errorf("multiple Dockerfiles found; use --dockerfile")
		} else {
			var err error
			if dockerfile, err = chooseDockerfile(reader, files, false); err != nil {
				return RuntimeSelection{}, err
			}
		}
	}
	selected, ok := findDockerfile(files, dockerfile)
	if !ok {
		return RuntimeSelection{}, fmt.Errorf("Dockerfile %q was not discovered in the repository", dockerfile)
	}
	if existing.Runtime == "docker" && existing.Dockerfile == selected.Path {
		context = guards.FirstValue(context, existing.DockerContext)
		if port == 0 {
			port = existing.ContainerPort
		}
	}
	context = guards.FirstValue(context, ".")
	if port == 0 && len(selected.ExposedPorts) == 1 {
		port = selected.ExposedPorts[0]
	}
	if port == 0 && nonInteractive {
		return RuntimeSelection{}, fmt.Errorf("container port is ambiguous; use --container-port")
	}
	if port == 0 {
		value, err := ReadPrompt(reader, "Container port: ")
		if err != nil {
			return RuntimeSelection{}, err
		}
		if port, err = strconv.Atoi(value); err != nil {
			return RuntimeSelection{}, fmt.Errorf("invalid container port %q", value)
		}
	}
	if port < 1 || port > 65535 {
		return RuntimeSelection{}, fmt.Errorf("invalid container port %d", port)
	}
	fmt.Printf("[✓] Docker runtime: %s (context %s, container port %d)\n", selected.Path, context, port)
	return RuntimeSelection{Mode: "docker", Dockerfile: selected.Path, DockerContext: context, ContainerPort: port}, nil
}
func chooseDockerfile(reader *bufio.Reader, files []walker.DockerfileInfo, allowNative bool) (string, error) {
	fmt.Println("\nDeployment runtime:")
	if allowNative {
		fmt.Println("  0. Native process + systemd + Nginx")
	}
	for index, file := range files {
		ports := make([]string, len(file.ExposedPorts))
		for i, port := range file.ExposedPorts {
			ports[i] = strconv.Itoa(port)
		}
		exposed := guards.FirstValue(strings.Join(ports, ","), "no EXPOSE")
		if len(ports) > 0 {
			exposed = "ports " + exposed
		}
		fmt.Printf("  %d. Docker: %s (%s, %s)\n", index+1, file.Path, guards.FirstValue(file.BaseImage, "unknown base"), exposed)
	}
	value, err := ReadPrompt(reader, "Select: ")
	if err != nil {
		return "", err
	}
	if allowNative && (value == "" || value == "0") {
		return "", nil
	}
	choice, err := strconv.Atoi(value)
	if err != nil || choice < 1 || choice > len(files) {
		return "", fmt.Errorf("invalid runtime selection %q", value)
	}
	return files[choice-1].Path, nil
}
func findDockerfile(files []walker.DockerfileInfo, path string) (walker.DockerfileInfo, bool) {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, file := range files {
		if file.Path == path {
			return file, true
		}
	}
	return walker.DockerfileInfo{}, false
}
func cleanRelative(path string) string {
	if path = strings.TrimSpace(path); path != "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return ""
}

// ReadPrompt is the shared line-input path for deployment choices.
func ReadPrompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	return strings.TrimSpace(value), err
}

package core

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// LogOperation records only controlled metadata; errors and user arguments may contain secrets.
func LogOperation(project, operation, status string) {
	_ = exec.Command("logger", "-t", "ezdeploy", operationLogMessage(project, operation, status)).Run()
}

func operationLogMessage(project, operation, status string) string {
	if !projectNamePattern.MatchString(project) {
		project = "unknown"
	}
	allowed := func(value string, values ...string) string {
		for _, candidate := range values {
			if value == candidate {
				return value
			}
		}
		return "unknown"
	}
	operation = allowed(operation, "deploy", "redeploy", "rollback")
	status = allowed(status, "started", "succeeded", "failed")
	return fmt.Sprintf("project=%s operation=%s status=%s", project, operation, status)
}

type LogOptions struct {
	Source, Service string
	Lines           int
	Follow          bool
}

// LogCommand selects existing providers without introducing a log store.
func LogCommand(projectName string, project Project, options LogOptions) (*exec.Cmd, error) {
	if options.Lines == 0 {
		options.Lines = 100
	}
	if options.Lines < 1 || options.Lines > 10000 {
		return nil, fmt.Errorf("lines must be between 1 and 10000")
	}
	args := []string{}
	switch options.Source {
	case "deployment":
		args = []string{"-t", "ezdeploy", "--grep", "project=" + regexp.QuoteMeta(projectName), "-n", strconv.Itoa(options.Lines)}
		if options.Follow {
			args = append(args, "--follow")
		}
		return attachedCommand("journalctl", args...), nil
	case "runtime":
		services := project.ManagedServices(projectName)
		if len(services) == 0 {
			return nil, fmt.Errorf("%s has no managed services", projectName)
		}
		selected := Service{}
		if options.Service == "" && len(services) > 1 {
			return nil, fmt.Errorf("--service is required; choose %s", serviceChoices(services))
		}
		for _, service := range services {
			if options.Service == "" || options.Service == service.Name || options.Service == service.Unit {
				selected = service
				break
			}
		}
		if selected.Name == "" {
			return nil, fmt.Errorf("service %q not found; choose %s", options.Service, serviceChoices(services))
		}
		if selected.Runtime == "docker" || project.Runtime == "docker" {
			args = []string{"logs", "--tail", strconv.Itoa(options.Lines)}
			if options.Follow {
				args = append(args, "--follow")
			}
			args = append(args, ManagedName(projectName))
			return attachedCommand("docker", args...), nil
		}
		if selected.Unit == "" {
			return nil, fmt.Errorf("service %s has no systemd unit", selected.Name)
		}
		args = []string{"-u", selected.Unit, "-n", strconv.Itoa(options.Lines)}
		if options.Follow {
			args = append(args, "--follow")
		}
		return attachedCommand("journalctl", args...), nil
	default:
		return nil, fmt.Errorf("source must be runtime or deployment")
	}
}

func attachedCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

func serviceChoices(services []Service) string {
	names := make([]string, len(services))
	for index, service := range services {
		names[index] = service.Name
	}
	return strings.Join(names, ", ")
}

package UI

import (
	"fmt"
	"os/exec"
	"strings"

	"EZDeploy/core"
	"EZDeploy/services"
)

// ToolStatus represents the validity state of a registered tool.
type ToolStatus int

const (
	StatusUnknown ToolStatus = iota
	StatusValid
	StatusInvalid
)

// FieldKind distinguishes how a Field should be collected from the user.
type FieldKind int

const (
	FieldText FieldKind = iota
	FieldMultiSelect
)

// Field describes a single piece of input a tool needs before it can run.
type Field struct {
	Key         string // map key passed into Run
	Label       string // shown to the user during step-by-step prompt
	Placeholder string
	Kind        FieldKind
	// Options lists the choices for FieldMultiSelect fields.
	Options []string
	// AutoFill, if non-nil, pulls this field's value from the active Project
	// instead of prompting the user. Returns ("", false) if not available.
	AutoFill func(project string) (string, bool)
}

// Tool wraps a core function as a runnable, validatable registry entry.
type Tool struct {
	Name        string
	Description string

	// RequiresProject means this tool cannot run until a project is active
	// (i.e. a repo has been cloned in this session).
	RequiresProject bool

	// Fields lists the inputs needed, in prompt order. Fields with a
	// successful AutoFill are skipped during step-by-step collection.
	Fields []Field

	// Validate reports whether the tool's prerequisites are met (e.g. binaries on PATH).
	Validate func() (ToolStatus, string)

	// Run executes the tool given fully-collected args.
	Run func(args map[string]string) error
}

// requireBinaries applies one availability check to every tool that needs it.
func requireBinaries(names ...string) func() (ToolStatus, string) {
	return func() (ToolStatus, string) {
		for _, name := range names {
			if _, err := exec.LookPath(name); err != nil {
				return StatusInvalid, name + " not found on PATH"
			}
		}
		return StatusValid, "ready"
	}
}

func alwaysAvailable() (ToolStatus, string) { return StatusValid, "always available" }

func fromProjectName(project string) (string, bool) {
	if project == "" {
		return "", false
	}
	return project, true
}

// serviceActionTool is the single UI definition shared by systemd actions.
func serviceActionTool(action string) Tool {
	return Tool{
		Name:            action + "Service",
		Description:     strings.ToLower(action) + " the active project service",
		RequiresProject: true,
		Fields: []Field{
			{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
		},
		Validate: requireBinaries("systemctl"),
		Run: func(args map[string]string) error {
			return services.Action(args["projectName"], strings.ToLower(action))
		},
	}
}

// Registry returns the full set of tools wired to core package functions.
func Registry() []Tool {
	return []Tool{
		{
			Name:            "Install",
			Description:     "Install language runtimes + infra components",
			RequiresProject: false,
			Fields: []Field{
				{
					Key:     "items",
					Label:   "Select components to install",
					Kind:    FieldMultiSelect,
					Options: []string{"go", "node", "python", "rust", "nginx", "certbot", "docker"},
				},
			},
			Validate: func() (ToolStatus, string) {
				os := core.GetOS()
				switch os.ID {
				case "ubuntu", "debian", "amzn", "fedora", "rhel", "centos":
					return StatusValid, "supported OS: " + os.ID
				default:
					return StatusInvalid, "unsupported OS: " + os.ID
				}
			},
			Run: func(args map[string]string) error {
				// Empty selection is valid — core.Install falls back to the
				// default nginx+certbot stack when nothing else is picked.
				return core.Install(splitCSV(args["items"]), true)
			},
		},
		{
			Name:            "GetOS",
			Description:     "Detect current OS and distro ID",
			RequiresProject: false,
			Validate:        alwaysAvailable,
			Run: func(args map[string]string) error {
				info := core.GetOS()
				fmt.Printf("System=%s ID=%s\n", info.System, info.ID)
				return nil
			},
		},
		{
			Name:            "GetNextPort",
			Description:     "Compute the next available port from the registry",
			RequiresProject: false,
			Validate:        alwaysAvailable,
			Run: func(args map[string]string) error {
				ports, err := core.GetNextPorts(1)
				if err != nil {
					return err
				}
				fmt.Printf("Next available port: %d\n", ports[0])
				return nil
			},
		},
		{
			Name:            "UnregisterProject",
			Description:     "Remove a project entry from the registry",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: alwaysAvailable,
			Run: func(args map[string]string) error {
				if args["projectName"] == "" {
					return fmt.Errorf("projectName is required")
				}
				return core.UnregisterProject(args["projectName"])
			},
		},
		serviceActionTool("Start"),
		serviceActionTool("Stop"),
		serviceActionTool("Restart"),
		serviceActionTool("Reload"),
		{
			Name:            "Metrics",
			Description:     "Show live project and system metrics",
			RequiresProject: false,
			Validate:        requireBinaries("systemctl", "top", "free", "df"),
			Run: func(args map[string]string) error {
				return services.Metrics()
			},
		},
	}
}

func splitCSV(s string) []string {
	values := strings.Split(s, ",")
	result := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

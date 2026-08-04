package UI

import (
	"fmt"
	"os/exec"

	"EZDeploy/core"
	"EZDeploy/services"
	"EZDeploy/walker"
)

// ToolStatus represents the validity state of a registered tool.
type ToolStatus int

const (
	StatusUnknown ToolStatus = iota
	StatusValid
	StatusInvalid
)

func (s ToolStatus) String() string {
	switch s {
	case StatusValid:
		return "valid"
	case StatusInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// FieldKind distinguishes how a Field should be collected from the user.
type FieldKind int

const (
	FieldText FieldKind = iota
	FieldMultiSelect
	FieldKeyValueList
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
	AutoFill func(p *Project) (string, bool)
}

// Tool wraps a core function as a runnable, validatable registry entry.
type Tool struct {
	Name        string
	Category    string
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

// Project holds the active project context once a repo has been cloned,
// so downstream tools can auto-fill instead of re-asking.
type Project struct {
	Path    string
	Name    string
	RepoURL string
}

// binAvailable checks whether a binary exists on PATH.
func binAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fromProjectPath(p *Project) (string, bool) {
	if p == nil || p.Path == "" {
		return "", false
	}
	return p.Path, true
}

func fromProjectName(p *Project) (string, bool) {
	if p == nil || p.Name == "" {
		return "", false
	}
	return p.Name, true
}

func fromProjectRepoURL(p *Project) (string, bool) {
	if p == nil || p.RepoURL == "" {
		return "", false
	}
	return p.RepoURL, true
}

// Registry returns the full set of tools wired to core package functions.
func Registry() []Tool {
	return []Tool{
		{
			Name:            "CloneRepo",
			Category:        "Project Management",
			Description:     "Clone or pull a repository — establishes the active project",
			RequiresProject: false,
			Fields: []Field{
				{Key: "repoURL", Label: "Repository URL", Placeholder: "https://github.com/user/repo.git"},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("git") {
					return StatusInvalid, "git not found on PATH"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				repoURL := args["repoURL"]
				if repoURL == "" {
					return fmt.Errorf("repoURL is required")
				}
				_, err := core.CloneRepo(repoURL)
				return err
			},
		},
		{
			Name:            "DownloadDeps",
			Category:        "Project Management",
			Description:     "Detect project type and install/build dependencies",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectPath", Label: "Project path", AutoFill: fromProjectPath},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("git") {
					return StatusInvalid, "git not found on PATH"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				path := args["projectPath"]
				if path == "" {
					return fmt.Errorf("projectPath is required")
				}
				return core.DownloadDeps(path)
			},
		},
		{
			Name:            "SetupEnv",
			Category:        "Project Management",
			Description:     "Interactively collect env vars and write .env",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectPath", Label: "Project path", AutoFill: fromProjectPath},
			},
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				path := args["projectPath"]
				if path == "" {
					return fmt.Errorf("projectPath is required")
				}
				return core.SetupEnv(path)
			},
		},
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
				items := args["items"]
				// Empty selection is valid — core.Install falls back to the
				// default nginx+certbot stack when nothing else is picked.
				return core.Install(splitCSV(items))
			},
		},
		{
			Name:            "CreateNginxConfig",
			Category:        "Service Management",
			Description:     "Write nginx server block, symlink, reload, provision SSL",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
				{Key: "projectPath", Label: "Project path", AutoFill: fromProjectPath},
				{Key: "port", Label: "Port", Placeholder: "8000"},
				{Key: "domain", Label: "Domain", Placeholder: "example.com"},
				{Key: "email", Label: "Email (for SSL)", Placeholder: "you@example.com"},
				{Key: "extraRoutes", Label: "Additional routes", Placeholder: "/health,/callback"},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("nginx") {
					return StatusInvalid, "nginx not found on PATH"
				}
				if !binAvailable("certbot") {
					return StatusInvalid, "certbot not found on PATH"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				port := 0
				fmt.Sscanf(args["port"], "%d", &port)

				if args["projectName"] == "" || args["projectPath"] == "" || port == 0 || args["domain"] == "" || args["email"] == "" {
					return fmt.Errorf("projectName, projectPath, port, domain, and email are required")
				}

				report, err := walker.ScanDefault(args["projectPath"])
				if err != nil {
					return err
				}
				routes := append(report.UniqueRoutePaths(), splitCSV(args["extraRoutes"])...)
				if err := core.CreateNginxConfig(args["projectName"], port, args["domain"], args["email"], routes, true); err != nil {
					return err
				}

				return core.RegisterProject(args["projectName"], core.Project{
					Port:   port,
					Domain: args["domain"],
					Email:  args["email"],
					Status: "deployed",
				})
			},
		},
		{
			Name:            "GetOS",
			Description:     "Detect current OS and distro ID",
			RequiresProject: false,
			Fields:          nil,
			Validate: func() (ToolStatus, string) {
				return StatusValid, "always available"
			},
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
			Fields:          nil,
			Validate: func() (ToolStatus, string) {
				return StatusValid, "always available"
			},
			Run: func(args map[string]string) error {
				port, err := core.GetNextPort()
				if err != nil {
					return err
				}
				fmt.Printf("Next available port: %d\n", port)
				return nil
			},
		},
		{
			Name:            "RegisterProject",
			Category:        "Project Management",
			Description:     "Add or update a project entry in the registry",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
				{Key: "port", Label: "Port", Placeholder: "8000"},
				{Key: "domain", Label: "Domain", Placeholder: "example.com"},
				{Key: "repoURL", Label: "Repo URL", AutoFill: fromProjectRepoURL},
				{Key: "branch", Label: "Branch", Placeholder: "main"},
				{Key: "serviceName", Label: "Service name", Placeholder: "ezdeploy-myapp"},
				{Key: "startCommand", Label: "Start command", Placeholder: "npm start"},
			},
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				port := 0
				fmt.Sscanf(args["port"], "%d", &port)
				if args["projectName"] == "" || port == 0 {
					return fmt.Errorf("projectName and port are required")
				}
				return core.RegisterProject(args["projectName"], core.Project{
					Port:         port,
					Domain:       args["domain"],
					RepoURL:      args["repoURL"],
					Branch:       args["branch"],
					ServiceName:  args["serviceName"],
					StartCommand: args["startCommand"],
					Status:       "registered",
				})
			},
		},
		{
			Name:            "UnregisterProject",
			Category:        "Project Management",
			Description:     "Remove a project entry from the registry",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				if args["projectName"] == "" {
					return fmt.Errorf("projectName is required")
				}
				return core.UnregisterProject(args["projectName"])
			},
		},
		{
			Name:            "CreateService",
			Description:     "Create a systemd service for the active project",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
				{Key: "projectPath", Label: "Project path", AutoFill: fromProjectPath},
				{Key: "port", Label: "Port", Placeholder: "9000"},
				{Key: "startCommand", Label: "Start command", Placeholder: "npm start"},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				port := 0
				fmt.Sscanf(args["port"], "%d", &port)

				if args["projectName"] == "" || args["projectPath"] == "" || port == 0 || args["startCommand"] == "" {
					return fmt.Errorf("projectName, projectPath, port, and startCommand are required")
				}

				return services.Create(
					args["projectName"],
					args["projectPath"],
					args["startCommand"],
					port,
				)
			},
		},
		{
			Name:            "StartService",
			Category:        "Service Management",
			Description:     "Start the active project service",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				return services.Start(args["projectName"])
			},
		},
		{
			Name:            "StopService",
			Category:        "Service Management",
			Description:     "Stop the active project service",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				return services.Stop(args["projectName"])
			},
		},
		{
			Name:            "RestartService",
			Category:        "Service Management",
			Description:     "Restart the active project service",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				return services.Restart(args["projectName"])
			},
		},
		{
			Name:            "ReloadService",
			Category:        "Service Management",
			Description:     "Reload the active project service",
			RequiresProject: true,
			Fields: []Field{
				{Key: "projectName", Label: "Project name", AutoFill: fromProjectName},
			},
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				return services.Reload(args["projectName"])
			},
		},

		{
			Name:            "Metrics",
			Category:        "Monitoring",
			Description:     "Show live project and system metrics",
			RequiresProject: false,
			Fields:          nil,
			Validate: func() (ToolStatus, string) {
				if !binAvailable("systemctl") {
					return StatusInvalid, "systemctl not found"
				}
				if !binAvailable("top") {
					return StatusInvalid, "top not found"
				}
				if !binAvailable("free") {
					return StatusInvalid, "free not found"
				}
				if !binAvailable("df") {
					return StatusInvalid, "df not found"
				}
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				return services.Metrics()
			},
		},
	}
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

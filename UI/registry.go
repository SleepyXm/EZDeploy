package UI

import (
	"fmt"
	"os/exec"

	"EZDeploy/core"
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

// Tool wraps a core function as a runnable, validatable registry entry.
type Tool struct {
	Name        string
	Description string
	// Validate reports whether the tool's prerequisites are met (e.g. binaries on PATH).
	Validate func() (ToolStatus, string)
	// Run executes the tool. Args are tool-specific and resolved by the caller.
	Run func(args map[string]string) error
}

// binAvailable checks whether a binary exists on PATH.
func binAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Registry returns the full set of tools wired to core package functions.
func Registry() []Tool {
	return []Tool{
		{
			Name:        "DownloadDeps",
			Description: "Detect project type and install/build dependencies",
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
			Name:        "SetupEnv",
			Description: "Interactively collect env vars and write .env",
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready (interactive, runs in current terminal)"
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
			Name:        "CloneRepo",
			Description: "Clone or pull a repository into the projects dir",
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
			Name:        "Install",
			Description: "Install language runtimes + nginx + certbot",
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
				langs := args["languages"]
				if langs == "" {
					return fmt.Errorf("languages is required (comma-separated)")
				}
				return core.Install(splitCSV(langs))
			},
		},
		{
			Name:        "CreateNginxConfig",
			Description: "Write nginx server block, symlink, reload, provision SSL",
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
				if args["projectName"] == "" || port == 0 || args["domain"] == "" || args["email"] == "" {
					return fmt.Errorf("projectName, port, domain, and email are required")
				}
				return core.CreateNginxConfig(args["projectName"], port, args["domain"], args["email"])
			},
		},
		{
			Name:        "GetOS",
			Description: "Detect current OS and distro ID",
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
			Name:        "GetNextPort",
			Description: "Compute the next available port from the registry",
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
			Name:        "RegisterProject",
			Description: "Add or update a project entry in the registry",
			Validate: func() (ToolStatus, string) {
				return StatusValid, "ready"
			},
			Run: func(args map[string]string) error {
				port := 0
				fmt.Sscanf(args["port"], "%d", &port)
				if args["projectName"] == "" || port == 0 {
					return fmt.Errorf("projectName and port are required")
				}
				return core.RegisterProject(args["projectName"], port, args["domain"], args["repoURL"], args["branch"])
			},
		},
		{
			Name:        "UnregisterProject",
			Description: "Remove a project entry from the registry",
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

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/UI"
	"EZDeploy/core"
	"EZDeploy/services"
	"EZDeploy/walker"
)

type routeList []string

type runtimeSelection struct {
	Mode          string
	Dockerfile    string
	DockerContext string
	ContainerPort int
}

func (routes *routeList) String() string { return strings.Join(*routes, ",") }
func (routes *routeList) Set(value string) error {
	if value = strings.TrimSpace(value); value == "" {
		return fmt.Errorf("route cannot be empty")
	}
	*routes = append(*routes, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ezdeploy:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		printUsage()
		return nil
	}
	root, err := installationRoot()
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}

	if len(args) == 0 {
		program := tea.NewProgram(UI.NewModel())
		_, err := program.Run()
		return err
	}
	if args[0] != "deploy" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	return deploy(args[1:])
}

func deploy(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDeployUsage()
		return nil
	}
	repoURL := args[0]
	flags := flag.NewFlagSet("deploy", flag.ContinueOnError)
	branch := flags.String("branch", "", "repository branch")
	sshKey := flags.String("ssh-key", "", "private repository SSH key")
	domain := flags.String("domain", "", "application domain")
	email := flags.String("email", "", "Let's Encrypt email")
	port := flags.Int("port", 0, "application port")
	start := flags.String("start", "", "application start command")
	runtimeMode := flags.String("runtime", "", "native or docker")
	dockerfile := flags.String("dockerfile", "", "production Dockerfile path")
	dockerContext := flags.String("docker-context", "", "Docker build context relative to the repository")
	containerPort := flags.Int("container-port", 0, "port exposed by the application container")
	nonInteractive := flags.Bool("non-interactive", false, "fail instead of prompting for missing values")
	noWhitelist := flags.Bool("no-route-whitelist", false, "proxy every application path")
	var extraRoutes routeList
	flags.Var(&extraRoutes, "allow-route", "additional route path; repeatable")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if os.Geteuid() != 0 || strings.TrimSpace(os.Getenv("SUDO_USER")) == "" {
		return fmt.Errorf("deploy must be run with sudo from the application account")
	}

	projectName, err := core.ProjectNameFromRepoURL(repoURL)
	if err != nil {
		return err
	}
	registry, err := core.GetRegistry()
	if err != nil {
		return err
	}
	existing := registry[projectName]
	// A redeploy stays on its registered branch unless the user overrides it.
	if *branch == "" && existing.Branch != "" {
		*branch = existing.Branch
	}
	projectPath, err := core.CloneRepoWithOptions(repoURL, core.CloneOptions{Branch: *branch, SSHKey: *sshKey})
	if err != nil {
		return err
	}

	report, err := walker.ScanDefault(projectPath)
	if err != nil {
		return fmt.Errorf("scan project: %w", err)
	}
	routes := append(report.UniqueRoutePaths(), extraRoutes...)
	if !*noWhitelist && len(routes) == 0 {
		return fmt.Errorf("no routes discovered; use --allow-route or --no-route-whitelist")
	}
	for _, hit := range report.RouteHits {
		fmt.Printf("  %-7s %-30s %s:%d\n", hit.Method, hit.Path, hit.File, hit.Line)
	}
	reader := bufio.NewReader(os.Stdin)
	runtime, err := selectRuntime(reader, report, existing, *runtimeMode, *dockerfile, *dockerContext, *containerPort, *nonInteractive)
	if err != nil {
		return err
	}
	tools := []string{"nginx", "certbot"}
	if runtime.Mode == "docker" {
		tools = append(tools, "docker")
	}
	if err := core.EnsureTools(tools...); err != nil {
		return err
	}

	if runtime.Mode == "native" {
		if err := core.DownloadDeps(projectPath); err != nil {
			return err
		}
	}
	if *nonInteractive {
		err = core.SetupEnvNonInteractive(projectPath)
	} else {
		err = core.SetupEnv(projectPath)
	}
	if err != nil {
		return err
	}

	if *port == 0 {
		if existing.Port != 0 {
			*port = existing.Port
		} else {
			*port, err = core.GetNextPort()
			if err != nil {
				return err
			}
		}
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("invalid port %d", *port)
	}

	if runtime.Mode == "native" {
		if *start == "" {
			*start = existing.StartCommand
		}
		if *start == "" {
			*start, err = core.DefaultStartCommand(projectPath)
			if err != nil {
				return err
			}
		}
		if *start == "" {
			if *nonInteractive {
				return fmt.Errorf("start command is required; use --start")
			}
			*start, err = readPrompt(reader, "Start command: ")
			if err != nil || *start == "" {
				return fmt.Errorf("start command is required")
			}
		}
	}
	if *domain == "" {
		*domain = existing.Domain
	}
	if *domain == "" {
		if *nonInteractive {
			return fmt.Errorf("domain is required; use --domain")
		}
		*domain, err = readPrompt(reader, "Domain: ")
	}
	if err != nil || *domain == "" {
		return fmt.Errorf("domain is required")
	}
	if *email == "" {
		*email = existing.Email
	}
	if *email == "" {
		if *nonInteractive {
			return fmt.Errorf("email is required; use --email")
		}
		*email, err = readPrompt(reader, "Email for HTTPS: ")
	}
	if err != nil || *email == "" {
		return fmt.Errorf("email is required for HTTPS")
	}
	if *branch == "" {
		*branch, err = core.CurrentBranch(projectPath)
		if err != nil {
			return err
		}
	}

	if runtime.Mode == "docker" {
		stoppedNative := existing.ServiceName != "" && existing.Runtime != "docker"
		if stoppedNative {
			if err := services.Stop(projectName); err != nil {
				return err
			}
		}
		err = core.DeployDocker(core.DockerDeployment{
			ProjectName: projectName, ProjectPath: projectPath,
			Dockerfile: runtime.Dockerfile, BuildContext: runtime.DockerContext,
			HostPort: *port, ContainerPort: runtime.ContainerPort,
		})
		if err != nil {
			if stoppedNative {
				_ = services.Start(projectName)
			}
			return err
		}
	} else {
		stoppedDocker := existing.Runtime == "docker"
		if stoppedDocker {
			if err := core.StopDocker(projectName); err != nil {
				return err
			}
		}
		if err := services.Create(projectName, projectPath, *start, *port); err != nil {
			if stoppedDocker {
				_ = core.StartDocker(projectName)
			}
			return err
		}
		// Restart also starts an inactive unit and guarantees updated code is loaded.
		if err := services.Restart(projectName); err != nil {
			if stoppedDocker {
				_ = core.StartDocker(projectName)
			}
			return err
		}
	}
	serviceName := ""
	if runtime.Mode == "native" {
		serviceName = services.Name(projectName)
	}
	record := core.Project{
		Path: projectPath, Port: *port, Domain: *domain, Email: *email, RepoURL: repoURL,
		Branch: *branch, ServiceName: serviceName,
		StartCommand: *start, Runtime: runtime.Mode,
		Dockerfile: runtime.Dockerfile, DockerContext: runtime.DockerContext,
		ContainerPort: runtime.ContainerPort,
	}
	record.Status = runtime.Mode + "_running"
	if err := core.RegisterProject(projectName, record); err != nil {
		return err
	}
	if err := core.CreateNginxConfig(projectName, *port, *domain, *email, routes, !*noWhitelist); err != nil {
		return err
	}
	record.Status = "deployed"
	if err := core.RegisterProject(projectName, record); err != nil {
		return err
	}

	fmt.Printf("[✓] %s deployed on %s (port %d)\n", projectName, *domain, *port)
	return nil
}

func selectRuntime(reader *bufio.Reader, report walker.Report, existing core.Project, mode, dockerfile, context string, port int, nonInteractive bool) (runtimeSelection, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	dockerfile = cleanRelative(dockerfile)
	context = cleanRelative(context)
	if mode == "native" && dockerfile != "" {
		return runtimeSelection{}, fmt.Errorf("--runtime native cannot be combined with --dockerfile")
	}
	if dockerfile != "" {
		mode = "docker"
	}
	if mode == "" {
		mode = existing.Runtime
		if mode == "" && existing.ServiceName != "" {
			mode = "native"
		}
	}

	files := append([]walker.DockerfileInfo(nil), report.Dockerfiles...)
	sort.SliceStable(files, func(i, j int) bool {
		left := strings.Contains(strings.ToLower(filepath.Base(files[i].Path)), "prod")
		right := strings.Contains(strings.ToLower(filepath.Base(files[j].Path)), "prod")
		return left && !right || left == right && files[i].Path < files[j].Path
	})
	if mode == "" && len(files) == 0 {
		mode = "native"
	}
	if mode == "" && nonInteractive {
		if len(files) != 1 {
			return runtimeSelection{}, fmt.Errorf("multiple Dockerfiles found; use --dockerfile or --runtime native")
		}
		mode, dockerfile = "docker", files[0].Path
	}
	if mode == "" {
		choice, err := chooseDockerfile(reader, files, true)
		if err != nil {
			return runtimeSelection{}, err
		}
		if choice == "" {
			mode = "native"
		} else {
			mode, dockerfile = "docker", choice
		}
	}
	if mode == "native" {
		if context != "" || port != 0 {
			return runtimeSelection{}, fmt.Errorf("Docker context and container port require --runtime docker")
		}
		return runtimeSelection{Mode: mode}, nil
	}
	if mode != "docker" {
		return runtimeSelection{}, fmt.Errorf("runtime must be native or docker, got %q", mode)
	}
	if len(files) == 0 {
		return runtimeSelection{}, fmt.Errorf("Docker runtime selected but no Dockerfile was discovered")
	}
	if dockerfile == "" && existing.Runtime == "docker" {
		dockerfile = existing.Dockerfile
	}
	if dockerfile == "" {
		if len(files) == 1 {
			dockerfile = files[0].Path
		} else if nonInteractive {
			return runtimeSelection{}, fmt.Errorf("multiple Dockerfiles found; use --dockerfile")
		} else {
			var err error
			dockerfile, err = chooseDockerfile(reader, files, false)
			if err != nil {
				return runtimeSelection{}, err
			}
		}
	}
	selected, ok := findDockerfile(files, dockerfile)
	if !ok {
		return runtimeSelection{}, fmt.Errorf("Dockerfile %q was not discovered in the repository", dockerfile)
	}
	if context == "" && existing.Runtime == "docker" && existing.Dockerfile == selected.Path {
		context = existing.DockerContext
	}
	if context == "" {
		context = "."
	}
	if port == 0 && existing.Runtime == "docker" && existing.Dockerfile == selected.Path {
		port = existing.ContainerPort
	}
	if port == 0 && len(selected.ExposedPorts) == 1 {
		port = selected.ExposedPorts[0]
	}
	if port == 0 && nonInteractive {
		return runtimeSelection{}, fmt.Errorf("container port is ambiguous; use --container-port")
	}
	if port == 0 {
		value, err := readPrompt(reader, "Container port: ")
		if err != nil {
			return runtimeSelection{}, err
		}
		port, err = strconv.Atoi(value)
		if err != nil {
			return runtimeSelection{}, fmt.Errorf("invalid container port %q", value)
		}
	}
	if port < 1 || port > 65535 {
		return runtimeSelection{}, fmt.Errorf("invalid container port %d", port)
	}
	fmt.Printf("[✓] Docker runtime: %s (context %s, container port %d)\n", selected.Path, context, port)
	return runtimeSelection{Mode: mode, Dockerfile: selected.Path, DockerContext: context, ContainerPort: port}, nil
}

func cleanRelative(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func chooseDockerfile(reader *bufio.Reader, files []walker.DockerfileInfo, allowNative bool) (string, error) {
	fmt.Println("\nDeployment runtime:")
	if allowNative {
		fmt.Println("  0. Native process + systemd + Nginx")
	}
	printDockerfiles(files)
	value, err := readPrompt(reader, "Select: ")
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

func printDockerfiles(files []walker.DockerfileInfo) {
	for index, file := range files {
		image := "unknown base"
		if file.BaseImage != "" {
			image = file.BaseImage
		}
		ports := "no EXPOSE"
		if len(file.ExposedPorts) > 0 {
			values := make([]string, len(file.ExposedPorts))
			for i, port := range file.ExposedPorts {
				values[i] = strconv.Itoa(port)
			}
			ports = "ports " + strings.Join(values, ",")
		}
		fmt.Printf("  %d. Docker: %s (%s, %s)\n", index+1, file.Path, image, ports)
	}
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

func installationRoot() (string, error) {
	if cwd, err := os.Getwd(); err == nil && fileExists(filepath.Join(cwd, "yamls", "walk.yml")) {
		return cwd, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if executable, err = filepath.EvalSymlinks(executable); err != nil {
		return "", err
	}
	root := filepath.Dir(executable)
	if !fileExists(filepath.Join(root, "yamls", "walk.yml")) {
		return "", fmt.Errorf("cannot find yamls/walk.yml beside EZDeploy; run from its installation directory")
	}
	return root, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readPrompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	value, err := reader.ReadString('\n')
	return strings.TrimSpace(value), err
}

func printUsage() {
	fmt.Println("Usage: ezdeploy | sudo ezdeploy deploy <repository> [options]")
}

func printDeployUsage() {
	fmt.Println(`Usage: sudo ezdeploy deploy <repository> [options]
  --branch <name>              repository branch
  --ssh-key <path>             private repository key
  --domain <host>              application domain
  --email <address>            Let's Encrypt email
  --port <number>              application port
  --start <command>            application start command
  --runtime <native|docker>    application runtime
  --dockerfile <path>          production Dockerfile path
  --docker-context <path>      Docker build context (default: repository root)
  --container-port <number>    application port inside the container
  --non-interactive            fail instead of prompting for missing values
  --allow-route <path>         additional allowed path; repeatable
  --no-route-whitelist         proxy every application path`)
}

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/UI"
	"EZDeploy/core"
	"EZDeploy/guards"
	"EZDeploy/services"
	"EZDeploy/walker"
)

type routeList []string

type deploymentOperation string

const (
	operationDeploy   deploymentOperation = "deploy"
	operationRedeploy deploymentOperation = "redeploy"
	operationRollback deploymentOperation = "rollback"
)

type deploymentResult struct{ name, prior, revision string }

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
	switch args[0] {
	case "deploy":
		return deploy(args[1:], operationDeploy)
	case "redeploy":
		return deploy(args[1:], operationRedeploy)
	case "releases":
		return releasesCommand(args[1:])
	case "rollback":
		return rollbackCommand(args[1:])
	case "logs":
		return logsCommand(args[1:])
	case "network":
		return networkCommand(args[1:])
	case "__system-install":
		if err := requireSudo("installation"); err != nil {
			return err
		}
		return core.Install(args[1:], true)
	case "__metrics":
		return services.Metrics()
	case "__service":
		if len(args) != 3 {
			return fmt.Errorf("invalid internal service action")
		}
		if err := requireSudo("service control"); err != nil {
			return err
		}
		name, _, err := core.ResolveProject(args[2])
		if err != nil {
			return err
		}
		return services.Action(name, args[1])
	case "__remove":
		if len(args) != 2 {
			return fmt.Errorf("invalid internal removal")
		}
		name, _, err := core.ResolveProject(args[1])
		if err != nil {
			return err
		}
		return core.UnregisterProject(name)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func deploy(args []string, operation deploymentOperation) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDeployUsage(operation)
		return nil
	}
	repositories, options, err := guards.DeployArguments(args)
	if err != nil {
		return err
	}
	if operation == operationRedeploy {
		for index, identifier := range repositories {
			_, project, err := core.ResolveProject(identifier)
			if err != nil {
				return err
			}
			repositories[index] = project.RepoURL
		}
	}
	if err := requireSudo(string(operation)); err != nil {
		return err
	}
	var rollbacks []*core.DeploymentRollback
	var results []deploymentResult
	for _, repository := range repositories {
		name, _ := core.ProjectNameFromRepoURL(repository)
		core.LogOperation(name, string(operation), "started")
		rollback, err := core.BeginDeploymentRollback(name, filepath.Join(core.CloneDir, name))
		if err == nil {
			rollbacks = append(rollbacks, rollback)
			var result deploymentResult
			result, err = deployOne(append([]string{repository}, options...), rollback, operation, nil)
			results = append(results, result)
		}
		if err != nil {
			return failDeployment(name, operation, err, rollbacks)
		}
	}
	return finishDeployments(results, rollbacks, operation)
}
func deployOne(args []string, rollback *core.DeploymentRollback, operation deploymentOperation, release *core.Release) (deploymentResult, error) {
	repoURL := args[0]
	flags := flag.NewFlagSet("deploy", flag.ContinueOnError)
	branch, sshKey := flags.String("branch", "", "repository branch"), flags.String("ssh-key", "", "private repository SSH key")
	domain, email := flags.String("domain", "", "application domain"), flags.String("email", "", "Let's Encrypt email")
	port, containerPort := flags.Int("port", 0, "application port"), flags.Int("container-port", 0, "port exposed by the application container")
	start, service := flags.String("start", "", "application start command"), flags.String("service", "", "native service name, root, or entry file")
	runtimeMode := flags.String("runtime", "", "native or docker")
	dockerfile, dockerContext := flags.String("dockerfile", "", "production Dockerfile path"), flags.String("docker-context", "", "Docker build context relative to the repository")
	nonInteractive, noWhitelist := flags.Bool("non-interactive", false, "fail instead of prompting for missing values"), flags.Bool("no-route-whitelist", false, "proxy every application path")
	tlsCert, tlsKey := flags.String("tls-cert", "", "existing wildcard TLS certificate"), flags.String("tls-key", "", "existing wildcard TLS key")
	var extraRoutes routeList
	flags.Var(&extraRoutes, "allow-route", "additional route path; repeatable")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return deploymentResult{}, nil
		}
		return deploymentResult{}, err
	}
	if flags.NArg() > 0 {
		return deploymentResult{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	projectName, err := core.ProjectNameFromRepoURL(repoURL)
	if err != nil {
		return deploymentResult{}, err
	}
	registry, err := core.GetRegistry()
	if err != nil {
		return deploymentResult{}, err
	}
	for name := range registry {
		if name != projectName && core.ManagedName(name) == core.ManagedName(projectName) {
			return deploymentResult{}, fmt.Errorf("project %q conflicts with registered project %q", projectName, name)
		}
	}
	existing, registered := registry[projectName]
	// Redeploy is deliberately the existing deployment path with a registry guard:
	// saved runtime, services, ports and proxy settings are reused after every rescan.
	if operation != operationDeploy && !registered {
		return deploymentResult{}, fmt.Errorf("%s is not deployed; run deploy first", repoURL)
	}
	// A redeploy stays on its registered branch unless the user overrides it.
	*branch = guards.FirstValue(*branch, existing.Branch)
	*sshKey = guards.FirstValue(*sshKey, existing.SSHKey)
	projectPath := existing.Path
	if operation == operationRollback {
		if release == nil {
			return deploymentResult{}, fmt.Errorf("rollback release is required")
		}
		if err := core.CheckoutRelease(projectPath, *release); err != nil {
			return deploymentResult{}, err
		}
	} else if projectPath, err = core.CloneRepoWithOptions(repoURL, core.CloneOptions{Branch: *branch, SSHKey: *sshKey}); err != nil {
		return deploymentResult{}, err
	}

	report, err := walker.ScanDefault(projectPath)
	if err != nil {
		return deploymentResult{}, fmt.Errorf("scan project: %w", err)
	}
	UI.PrintServiceCandidates(report.Services)
	reader := bufio.NewReader(os.Stdin)
	runtime, err := UI.SelectRuntime(reader, report, existing, *runtimeMode, *dockerfile, *dockerContext, *containerPort, *nonInteractive)
	if err != nil {
		return deploymentResult{}, err
	}
	var native []core.Service
	var targets []core.RouteTarget
	if runtime.Mode == "docker" {
		if strings.TrimSpace(*service) != "" {
			return deploymentResult{}, fmt.Errorf("--service applies only to native deployments")
		}
		routes := append(report.UniqueRoutePaths(), extraRoutes...)
		if !*noWhitelist && len(routes) == 0 {
			return deploymentResult{}, fmt.Errorf("no routes discovered; use --allow-route or --no-route-whitelist")
		}
		if err := core.EnsureTools("nginx", "docker"); err != nil {
			return deploymentResult{}, err
		}
		if err := rollback.TrackFile(filepath.Join(projectPath, ".env")); err != nil {
			return deploymentResult{}, err
		}
		if err := prepareEnvironment(projectPath, operation, !*nonInteractive); err != nil {
			return deploymentResult{}, err
		}
		for _, hit := range report.RouteHits {
			fmt.Printf("  %-7s %-30s %s:%d\n", hit.Method, hit.Path, hit.File, hit.Line)
		}
		if *port == 0 {
			*port = existing.Port
		}
		if *port == 0 {
			ports, err := core.GetNextPorts(1)
			if err != nil {
				return deploymentResult{}, err
			}
			*port = ports[0]
		}
		for _, route := range routes {
			targets = append(targets, core.RouteTarget{Path: route, Port: *port})
		}
	} else {
		var saved []string
		if existing.Runtime == "native" || existing.Runtime == "" && existing.ServiceName != "" {
			for _, service := range existing.ManagedServices(projectName) {
				saved = append(saved, guards.FirstValue(service.Entry, service.Root))
			}
		}
		selected, err := UI.SelectServices(reader, report.Services, strings.Join(saved, ","), *service, *nonInteractive)
		if err != nil {
			return deploymentResult{}, err
		}
		if len(selected) > 1 && (*port != 0 || *start != "" || len(extraRoutes) > 0 || *noWhitelist) {
			return deploymentResult{}, fmt.Errorf("--port, --start, --allow-route, and --no-route-whitelist require a single selected service")
		}
		toolSet := map[string]bool{"nginx": true}
		for _, service := range selected {
			if service.Runtime != "" {
				toolSet[service.Runtime] = true
			}
		}
		tools := make([]string, 0, len(toolSet))
		for tool := range toolSet {
			tools = append(tools, tool)
		}
		if err := core.EnsureTools(tools...); err != nil {
			return deploymentResult{}, err
		}

		previous := map[string]core.Service{}
		for _, service := range existing.ManagedServices(projectName) {
			previous[service.Root+"\x00"+service.Entry] = service
		}
		missing := 0
		for _, service := range selected {
			if previous[service.Root+"\x00"+service.Entry].Port == 0 {
				missing++
			}
		}
		available, err := core.GetNextPorts(missing)
		if err != nil {
			return deploymentResult{}, err
		}
		nextPort := 0
		unitNames := map[string]bool{}
		for _, candidate := range selected {
			path := filepath.Join(projectPath, filepath.FromSlash(candidate.Root))
			if err := rollback.TrackFile(filepath.Join(path, ".env")); err != nil {
				return deploymentResult{}, err
			}
			if err := prepareEnvironment(path, operation, !*nonInteractive); err != nil {
				return deploymentResult{}, err
			}
			prepared := ""
			if candidate.Runtime == "" {
				err = core.DownloadDeps(path)
			} else {
				entry := strings.TrimPrefix(candidate.Entry, strings.TrimSuffix(candidate.Root, "/")+"/")
				prepared, err = core.PrepareNativeService(path, candidate.Runtime, entry)
			}
			if err != nil {
				return deploymentResult{}, err
			}
			old := previous[candidate.Root+"\x00"+candidate.Entry]
			servicePort := old.Port
			if len(selected) == 1 && *port != 0 {
				servicePort = *port
			} else if servicePort == 0 {
				servicePort, nextPort = available[nextPort], nextPort+1
			}
			command := old.StartCommand
			if len(selected) == 1 {
				command = guards.FirstValue(*start, command)
			}
			command = guards.FirstValue(command, prepared, candidate.StartCommand)
			if command == "" {
				command, err = core.DefaultStartCommand(path)
			}
			if err != nil {
				return deploymentResult{}, err
			}
			if command == "" && !*nonInteractive {
				command, err = UI.ReadPrompt(reader, fmt.Sprintf("Start command for %s: ", candidate.Name))
			}
			if err != nil || command == "" {
				return deploymentResult{}, fmt.Errorf("start command is required for %s", candidate.Name)
			}
			routeHits, routes := report.RouteHits, report.UniqueRoutePaths()
			if candidate.Entry != "" {
				routeHits, routes = report.RouteHitsForService(candidate), report.UniqueRoutePathsForService(candidate)
			}
			if len(selected) == 1 {
				routes = append(routes, extraRoutes...)
			}
			if !*noWhitelist && len(routes) == 0 {
				return deploymentResult{}, fmt.Errorf("no routes discovered for %s; use --allow-route or --no-route-whitelist", candidate.Name)
			}
			for _, hit := range routeHits {
				fmt.Printf("  %-7s %-30s %s:%d\n", hit.Method, hit.Path, hit.File, hit.Line)
			}
			unit := core.ManagedServiceName(projectName, candidate.Name, len(selected) > 1)
			if unitNames[unit] {
				return deploymentResult{}, fmt.Errorf("detected services share the system name %q; select them separately", candidate.Name)
			}
			unitNames[unit] = true
			metadata := core.Service{Name: candidate.Name, Root: candidate.Root, Entry: candidate.Entry, Runtime: candidate.Runtime,
				StartCommand: command, Unit: unit, Port: servicePort, Routes: routes, Status: "running"}
			native = append(native, metadata)
			for _, route := range routes {
				targets = append(targets, core.RouteTarget{Path: route, Port: servicePort})
			}
		}
		*port, *start = native[0].Port, native[0].StartCommand
	}
	if *port < 1 || *port > 65535 {
		return deploymentResult{}, fmt.Errorf("invalid port %d", *port)
	}
	if *noWhitelist && len(targets) == 0 {
		targets = []core.RouteTarget{{Port: *port}}
	}
	*domain = guards.FirstValue(*domain, existing.Domain)
	if *domain == "" {
		if *nonInteractive {
			return deploymentResult{}, fmt.Errorf("domain is required; use --domain")
		}
		*domain, err = UI.ReadPrompt(reader, "Domain: ")
	}
	if err != nil || *domain == "" {
		return deploymentResult{}, fmt.Errorf("domain is required")
	}
	*tlsCert, *tlsKey = guards.FirstValue(*tlsCert, existing.TLSCert), guards.FirstValue(*tlsKey, existing.TLSKey)
	*email = guards.FirstValue(*email, existing.Email)
	if !strings.HasPrefix(*domain, "*.") && *email == "" {
		if *nonInteractive {
			return deploymentResult{}, fmt.Errorf("email is required; use --email")
		}
		*email, err = UI.ReadPrompt(reader, "Email for HTTPS: ")
	}
	if err != nil || !strings.HasPrefix(*domain, "*.") && *email == "" {
		return deploymentResult{}, fmt.Errorf("email is required for HTTPS")
	}
	if !strings.HasPrefix(*domain, "*.") {
		if err := core.EnsureTools("certbot"); err != nil {
			return deploymentResult{}, err
		}
	}
	if *branch == "" {
		*branch, err = core.CurrentBranch(projectPath)
		if err != nil {
			return deploymentResult{}, err
		}
	}

	var docker *core.DockerDeployment
	if runtime.Mode == "docker" {
		docker = &core.DockerDeployment{
			ProjectName: projectName, ProjectPath: projectPath,
			Dockerfile: runtime.Dockerfile, BuildContext: runtime.DockerContext,
			HostPort: *port, ContainerPort: runtime.ContainerPort,
			KeepPrevious: true,
		}
	}
	if err := services.ActivateProject(projectName, projectPath, existing, docker, native, rollback); err != nil {
		return deploymentResult{}, err
	}
	revision, err := core.CurrentRevision(projectPath)
	if err != nil {
		return deploymentResult{}, err
	}
	record := core.Project{
		Path: projectPath, Port: *port, Domain: *domain, Email: *email, RepoURL: repoURL,
		Branch: *branch, Revision: revision,
		SSHKey: *sshKey, TLSCert: *tlsCert, TLSKey: *tlsKey,
		StartCommand: *start, Runtime: runtime.Mode,
		Dockerfile: runtime.Dockerfile, DockerContext: runtime.DockerContext,
		ContainerPort: runtime.ContainerPort,
		Services:      []core.Service{},
	}
	if runtime.Mode == "native" {
		record.Services = native
	} else {
		record.Services = []core.Service{{Name: projectName, Root: ".", Runtime: "docker", Port: *port, Status: "running"}}
	}
	if err := core.CreateNginxConfig(projectName, *domain, *email, targets, !*noWhitelist, *tlsCert, *tlsKey); err != nil {
		return deploymentResult{}, err
	}
	record.Status = "deployed"
	if err := core.RegisterProject(projectName, record); err != nil {
		return deploymentResult{}, err
	}

	fmt.Printf("[✓] %s deployed on %s (port %d)\n", projectName, *domain, *port)
	return deploymentResult{name: projectName, prior: existing.Revision, revision: revision}, nil
}

func prepareEnvironment(path string, operation deploymentOperation, interactive bool) error {
	if operation == operationRollback {
		return core.ValidateEnv(path)
	}
	return core.SetupEnv(path, interactive, operation == operationDeploy)
}

func requireSudo(action string) error {
	if os.Geteuid() != 0 || strings.TrimSpace(os.Getenv("SUDO_USER")) == "" {
		return fmt.Errorf("%s must be run with sudo from the application account", action)
	}
	return nil
}

func failDeployment(name string, operation deploymentOperation, deployErr error, rollbacks []*core.DeploymentRollback) error {
	core.LogOperation(name, string(operation), "failed")
	for index := len(rollbacks) - 1; index >= 0; index-- {
		deployErr = errors.Join(deployErr, rollbacks[index].Restore())
	}
	return deployErr
}

func finishDeployments(results []deploymentResult, rollbacks []*core.DeploymentRollback, operation deploymentOperation) error {
	for _, result := range results {
		if _, err := core.RecordSuccessfulRelease(result.name, result.prior, result.revision, string(operation)); err != nil {
			return failDeployment(result.name, operation, err, rollbacks)
		}
	}
	for _, result := range results {
		core.LogOperation(result.name, string(operation), "succeeded")
	}
	for _, rollback := range rollbacks {
		rollback.Commit()
	}
	return nil
}
func installationRoot() (string, error) {
	if cwd, err := os.Getwd(); err == nil && core.FileExists(filepath.Join(cwd, "yamls", "walk.yml")) {
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
	if !core.FileExists(filepath.Join(root, "yamls", "walk.yml")) {
		return "", fmt.Errorf("cannot find yamls/walk.yml beside EZDeploy; run from its installation directory")
	}
	return root, nil
}
func printUsage() {
	fmt.Println(`Usage:
  ezdeploy
  sudo ezdeploy deploy|redeploy <repository...> [options]
  ezdeploy releases|network <project-or-repository>
  sudo ezdeploy rollback <project-or-repository> --release <release-id>
  sudo ezdeploy logs <project-or-repository> --source runtime|deployment [options]`)
}
func printDeployUsage(operation deploymentOperation) {
	fmt.Printf(`Usage: sudo ezdeploy %s <repository...> [options]
  --branch <name>              repository branch
  --ssh-key <path>             private repository key
  --domain <host>              application domain
  --email <address>            Let's Encrypt email
  --port <number>              application port
  --start <command>            application start command
  --service <name|path,...>    native service names, roots, or entry files
  --runtime <native|docker>    application runtime
  --dockerfile <path>          production Dockerfile path
  --docker-context <path>      Docker build context (default: repository root)
  --container-port <number>    application port inside the container
  --tls-cert <path>            existing certificate for a wildcard domain
  --tls-key <path>             existing private key for a wildcard domain
  --non-interactive            fail instead of prompting for missing values
  --allow-route <path>         additional allowed path; repeatable
  --no-route-whitelist         proxy every application path
`, operation)
}

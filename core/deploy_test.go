package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"EZDeploy/walker"
)

func TestCloneRepoFastForwardsExistingProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	// CloneDir is deliberately relative to the EZDeploy installation directory.
	t.Chdir(t.TempDir())

	origin := filepath.Join(t.TempDir(), "sample.git")
	seed := filepath.Join(t.TempDir(), "seed")
	testGit(t, "init", "--bare", origin)
	testGit(t, "init", "-b", "main", seed)
	testGit(t, "-C", seed, "config", "user.name", "EZDeploy Test")
	testGit(t, "-C", seed, "config", "user.email", "test@example.com")
	testGit(t, "-C", seed, "remote", "add", "origin", origin)

	versionFile := filepath.Join(seed, "version.txt")
	if err := os.WriteFile(versionFile, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, "-C", seed, "add", "version.txt")
	testGit(t, "-C", seed, "commit", "-m", "first")
	testGit(t, "-C", seed, "push", "-u", "origin", "main")

	projectPath, err := CloneRepoWithOptions(origin, CloneOptions{Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionFile, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, "-C", seed, "commit", "-am", "second")
	testGit(t, "-C", seed, "push")
	if _, err := CloneRepoWithOptions(origin, CloneOptions{Branch: "main"}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(projectPath, "version.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Fatalf("redeployed version = %q, want two", got)
	}
}

func testGit(t *testing.T, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestNonInteractiveEnvironmentValidation(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte(`os.Getenv("TOKEN")`), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(project, ".env")
	if err := os.WriteFile(envPath, []byte("TOKEN=kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(repoRoot)

	if err := SetupEnv(project, false, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil || string(data) != "TOKEN=kept\n" {
		t.Fatalf("existing environment changed: %q, %v", data, err)
	}
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf(".env mode = %v; want 0600", info.Mode().Perm())
	}

	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte(`os.Getenv("NEW_TOKEN")`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetupEnv(project, false, false); err == nil || !strings.Contains(err.Error(), "NEW_TOKEN") {
		t.Fatalf("missing key error = %v", err)
	}
}

func TestNewProjectRollbackDoesNotRecreateCloneFromTrackedEnv(t *testing.T) {
	project := t.TempDir()
	rollback := &DeploymentRollback{projectPath: project, files: map[string]fileState{}}
	if err := rollback.TrackFile(filepath.Join(project, ".env")); err != nil {
		t.Fatal(err)
	}
	if len(rollback.files) != 0 {
		t.Fatal("new project environment was captured outside the clone-level rollback")
	}
}

func TestAvailableMemoryIncludesFreeSwap(t *testing.T) {
	got := parseAvailableMemory("MemTotal: 1048576 kB\nMemAvailable: 300000 kB\nSwapFree: 250000 kB\n")
	if got != 550000<<10 {
		t.Fatalf("available memory = %d; want %d", got, int64(550000<<10))
	}
}

func TestRoutesFlowFromSourceIntoNginx(t *testing.T) {
	config, err := walker.LoadConfig(filepath.Join("..", "yamls", "walk.yml"))
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := walker.NewScanner(config)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	sources := map[string]string{
		"Dockerfile": `FROM node:22-alpine
EXPOSE 3000`,
		"deploy/prod.Dockerfile": `FROM golang:1.25 AS build
FROM alpine:3.22
EXPOSE 8080/tcp 8081`,
		"routes.go": `api := r.Group("/api")
api.GET("/users/:id", getUser)
auth := api.Group("/auth")
auth.POST("/login", login)
// api.DELETE("/commented", deleteUser)`,
		"routes.js": `app.use("/v1", router)
router.post("/items/:id", createItem)`,
		"routes.py": `router = APIRouter(prefix="/admin")
@router.get("/{id}")
def get_admin(): pass
@app.route("/login", methods=["GET", "POST"])
def login(): pass`,
	}
	for name, source := range sources {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/admin/{id}", "/api/auth/login", "/api/users/:id", "/login", "/v1/items/:id"}
	if got := report.UniqueRoutePaths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("routes = %#v, want %#v", got, want)
	}
	if len(report.Dockerfiles) != 2 {
		t.Fatalf("Dockerfiles = %#v, want two", report.Dockerfiles)
	}
	production := report.Dockerfiles[1]
	if production.Path != "deploy/prod.Dockerfile" ||
		production.BaseImage != "alpine:3.22" ||
		!reflect.DeepEqual(production.ExposedPorts, []int{8080, 8081}) {
		t.Fatalf("production Dockerfile metadata = %#v", production)
	}

	var targets []RouteTarget
	for _, route := range report.UniqueRoutePaths() {
		port := 8080
		if route == "/login" {
			port = 9090 // A second service can own a different route in the same server block.
		}
		targets = append(targets, RouteTarget{Path: route, Port: port})
	}
	nginx, _, err := renderNginxConfig("api.example.com", targets, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`location /api/ {`,
		`location ~ ^/admin/[^/]+/?$`,
		`location = /api/auth/login`,
		`location ~ ^/api/users/[^/]+/?$`,
		`location = /login`,
		`proxy_pass http://127.0.0.1:9090`,
		`location ~ ^/v1/items/[^/]+/?$`,
		`proxy_set_header X-Real-IP $remote_addr`,
		`proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for`,
		`proxy_set_header X-Forwarded-Proto $scheme`,
		"location / {\n        return 404;",
	} {
		if !strings.Contains(nginx, expected) {
			t.Errorf("generated nginx config is missing %q:\n%s", expected, nginx)
		}
	}
	if strings.Contains(nginx, "location = /api {") {
		t.Fatal("shared /api policy boundary became an undiscovered proxy endpoint")
	}
	if nginxBinary, err := exec.LookPath("nginx"); err == nil {
		nginxRoot := t.TempDir()
		fullConfig := "pid " + filepath.Join(nginxRoot, "nginx.pid") + ";\nerror_log stderr;\nevents {}\nhttp {\n" + nginx + "}\n"
		configPath := filepath.Join(nginxRoot, "nginx.conf")
		if err := os.WriteFile(configPath, []byte(fullConfig), 0o644); err != nil {
			t.Fatal(err)
		}
		// The production writer also runs nginx -t before replacing a live config.
		if output, err := exec.Command(nginxBinary, "-t", "-e", "stderr", "-p", nginxRoot, "-c", configPath).CombinedOutput(); err != nil {
			if strings.Contains(string(output), "Operation not permitted") {
				t.Skip("nginx parser is blocked by the execution sandbox")
			}
			t.Fatalf("nginx rejected generated config: %v\n%s", err, output)
		}
	}
}

func TestWalkerDiscoversMixedBackendServices(t *testing.T) {
	config, err := walker.LoadConfig(filepath.Join("..", "yamls", "walk.yml"))
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := walker.NewScanner(config)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	sources := map[string]string{
		"services/users/pyproject.toml": `[project]
name = "users"`,
		"services/users/main.py": `from fastapi import FastAPI
app = FastAPI()
@app.get("/users")
def users(): return []`,
		"services/payments/go.mod": `module example.com/payments
go 1.25`,
		"services/payments/cmd/api/main.go": `package main
import "net/http"
func main() {
  mux := http.NewServeMux()
  mux.HandleFunc("GET /payments", nil)
  http.ListenAndServe(":8080", mux)
}`,
		"services/payments/handlers/api_test.go": `package handlers
const testRoute = "GET /test-only"`,
		"package.json": `{
  "name": "finsec",
  "scripts": {"start": "node app/ui/index.js"},
  "dependencies": {"express": "latest"}
}`,
		"app/ui/index.ts": `const express = require("express")
const app = express()
app.get("/notifications", handler)
app.listen(process.env.PORT)`,
	}
	for name, source := range sources {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	report, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Services) != 3 {
		t.Fatalf("services = %#v, want three", report.Services)
	}
	byRuntime := map[string]walker.ServiceCandidate{}
	for _, service := range report.Services {
		byRuntime[service.Runtime] = service
	}
	want := map[string]struct {
		name, root, entry, start string
	}{
		"python": {"users", "services/users", "services/users/main.py", `.venv/bin/python -m uvicorn main:app --host 127.0.0.1 --port "$PORT"`},
		"go":     {"api", "services/payments", "services/payments/cmd/api/main.go", "go run ./cmd/api"},
		"node":   {"finsec", ".", "app/ui/index.ts", "npm start"},
	}
	for runtime, expected := range want {
		service, ok := byRuntime[runtime]
		if !ok {
			t.Errorf("missing %s service in %#v", runtime, report.Services)
			continue
		}
		if service.Name != expected.name || service.Root != expected.root ||
			service.Entry != expected.entry || service.StartCommand != expected.start ||
			service.Confidence != "high" {
			t.Errorf("%s service = %#v", runtime, service)
		}
	}
	routeWant := map[string][]string{
		"python": {"/users"},
		"go":     {"/payments"},
		"node":   {"/notifications"},
	}
	for runtime, expected := range routeWant {
		if got := report.UniqueRoutePathsForService(byRuntime[runtime]); !reflect.DeepEqual(got, expected) {
			t.Errorf("routes for %s = %#v, want %#v", runtime, got, expected)
		}
	}
}

func TestDockerRunIsLoopbackOnlyAndPreservesEnvironment(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("TOKEN=secret\nPORT=wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deployment := DockerDeployment{
		ProjectName: "api", ProjectPath: project,
		HostPort: 8000, ContainerPort: 8080,
	}
	got := dockerRunArgs(deployment, "ezdeploy-api", "ezdeploy/api:latest")
	want := []string{
		"run", "--detach", "--name", "ezdeploy-api",
		"--restart", "unless-stopped",
		"--label", "com.ezdeploy.project=api",
		"--publish", "127.0.0.1:8000:8080",
		"--env-file", filepath.Join(project, ".env"),
		"--env", "PORT=8080",
		"ezdeploy/api:latest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("docker run args = %#v, want %#v", got, want)
	}
}

func TestDockerPathsCannotEscapeProject(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "Dockerfile.prod"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := projectFile(project, "Dockerfile.prod", false); err != nil {
		t.Fatal(err)
	}
	if _, err := projectFile(project, "../Dockerfile", false); err == nil {
		t.Fatal("path traversal was accepted")
	}
	outside := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(outside, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "Dockerfile.link")); err != nil {
		t.Fatal(err)
	}
	if _, err := projectFile(project, "Dockerfile.link", false); err == nil {
		t.Fatal("symlink escaping the project was accepted")
	}
}

func TestRouteLocationParameters(t *testing.T) {
	tests := map[string]string{
		"/health":            "location = /health",
		"/health/":           "location = /health/",
		"/users/:id":         `location ~ ^/users/[^/]+/?$`,
		"/users/{id}":        `location ~ ^/users/[^/]+/?$`,
		"/files/<path:name>": `location ~ ^/files/.*/?$`,
	}
	for input, want := range tests {
		got, err := routeLocation(input)
		if err != nil {
			t.Fatalf("routeLocation(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("routeLocation(%q) = %q, want %q", input, got, want)
		}
	}
}

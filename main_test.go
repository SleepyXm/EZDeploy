package main

import (
	"bufio"
	"strings"
	"testing"

	"EZDeploy/UI"
	"EZDeploy/core"
	"EZDeploy/guards"
	"EZDeploy/walker"
)

func TestRuntimeSelection(t *testing.T) {
	report := walker.Report{Dockerfiles: []walker.DockerfileInfo{
		{Path: "Dockerfile", BaseImage: "node:22", ExposedPorts: []int{3000}},
		{Path: "deploy/Dockerfile.prod", BaseImage: "node:22-alpine", ExposedPorts: []int{8080}},
	}}
	tests := []struct {
		name, input, mode, file   string
		existing                  core.Project
		nonInteractive, wantError bool
		wantFile                  string
	}{
		{name: "interactive production choice", input: "1\n", wantFile: "deploy/Dockerfile.prod"},
		{name: "non-interactive ambiguity", nonInteractive: true, wantError: true},
		{name: "saved production choice", nonInteractive: true, wantFile: "deploy/Dockerfile.prod", existing: core.Project{
			Runtime: "docker", Dockerfile: "deploy/Dockerfile.prod", DockerContext: ".", ContainerPort: 8080,
		}},
		{name: "Dockerfile overrides saved native runtime", file: "deploy/Dockerfile.prod", nonInteractive: true, wantFile: "deploy/Dockerfile.prod", existing: core.Project{Runtime: "native"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := UI.SelectRuntime(bufio.NewReader(strings.NewReader(test.input)), report, test.existing, test.mode, test.file, "", 0, test.nonInteractive)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
			if err == nil && (got.Dockerfile != test.wantFile || got.DockerContext != "." || got.ContainerPort != 8080) {
				t.Fatalf("selection = %#v", got)
			}
		})
	}
}

func TestServiceSelectionTargetsDetectedRoot(t *testing.T) {
	services := []walker.ServiceCandidate{
		{Name: "finsec", Runtime: "node", Root: ".", Entry: "app/ui/index.ts", StartCommand: "npm start"},
		{Name: "backend", Runtime: "python", Root: "app/backend", Entry: "app/backend/main.py", StartCommand: "uvicorn main:app"},
		{Name: "go-backend", Runtime: "go", Root: "go-backend", Entry: "go-backend/main.go"},
	}

	selected, err := UI.SelectServices(bufio.NewReader(strings.NewReader("2,3\n")), services, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Name != "backend" || selected[1].Name != "go-backend" {
		t.Fatalf("selected service = %#v", selected)
	}
	if _, err := UI.SelectServices(bufio.NewReader(strings.NewReader("")), services, "", "", true); err == nil {
		t.Fatal("non-interactive deployment accepted ambiguous services")
	}
	saved, err := UI.SelectServices(bufio.NewReader(strings.NewReader("")), services, "app/backend/main.py,go-backend/main.go", "", true)
	if err != nil || len(saved) != 2 || saved[0].Root != "app/backend" || saved[1].Root != "go-backend" {
		t.Fatalf("saved selection = %#v, %v", saved, err)
	}
	if _, err := UI.SelectServices(bufio.NewReader(strings.NewReader("0,2\n")), services, "", "", false); err == nil {
		t.Fatal("repository root was combined with a detected service")
	}
	if _, _, err := guards.DeployArguments([]string{"github.com/acme/api", "github.com/acme/worker", "--domain=api.test"}); err == nil {
		t.Fatal("batch accepted a project-specific flag")
	}
	if _, _, err := guards.DeployArguments([]string{"github.com/acme/API", "github.com/acme/api"}); err == nil {
		t.Fatal("batch accepted colliding system names")
	}
}

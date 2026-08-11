package main

import (
	"bufio"
	"strings"
	"testing"

	"EZDeploy/core"
	"EZDeploy/walker"
)

func TestRuntimeSelection(t *testing.T) {
	report := walker.Report{Dockerfiles: []walker.DockerfileInfo{
		{Path: "Dockerfile", BaseImage: "node:22", ExposedPorts: []int{3000}},
		{Path: "deploy/Dockerfile.prod", BaseImage: "node:22-alpine", ExposedPorts: []int{8080}},
	}}
	tests := []struct {
		name, input               string
		existing                  core.Project
		nonInteractive, wantError bool
		wantFile                  string
	}{
		{name: "interactive production choice", input: "1\n", wantFile: "deploy/Dockerfile.prod"},
		{name: "non-interactive ambiguity", nonInteractive: true, wantError: true},
		{name: "saved production choice", nonInteractive: true, wantFile: "deploy/Dockerfile.prod", existing: core.Project{
			Runtime: "docker", Dockerfile: "deploy/Dockerfile.prod", DockerContext: ".", ContainerPort: 8080,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectRuntime(bufio.NewReader(strings.NewReader(test.input)), report, test.existing, "", "", "", 0, test.nonInteractive)
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

	selected, err := selectService(bufio.NewReader(strings.NewReader("2\n")), services, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name != "backend" || selected.Root != "app/backend" || selected.Runtime != "python" {
		t.Fatalf("selected service = %#v", selected)
	}

	if _, err := selectService(bufio.NewReader(strings.NewReader("")), services, "", "", true); err == nil {
		t.Fatal("non-interactive deployment accepted ambiguous services")
	}
	saved, err := selectService(bufio.NewReader(strings.NewReader("")), services, "app/backend/main.py", "", true)
	if err != nil || saved.Root != "app/backend" {
		t.Fatalf("saved selection = %#v, %v", saved, err)
	}
}

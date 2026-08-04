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

package services

import (
	"strings"
	"testing"
)

func TestSystemdUnitRunsAsDeploymentUser(t *testing.T) {
	unit := renderUnit("api", "/opt/ezdeploy/projects/api", "./api --stamp=%s", 8000, "ubuntu")
	for _, expected := range []string{
		"User=ubuntu",
		"WorkingDirectory=/opt/ezdeploy/projects/api",
		"Environment=PORT=8000",
		"ExecStart=/bin/sh -lc \"./api --stamp=%%s\"",
	} {
		if !strings.Contains(unit, expected) {
			t.Errorf("systemd unit is missing %q:\n%s", expected, unit)
		}
	}
}

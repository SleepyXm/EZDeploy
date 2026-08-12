package services

import (
	"EZDeploy/core"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func Metrics() error {
	fmt.Println("[→] Starting EZDeploy metrics...")

	for {
		registry, err := core.GetRegistry()
		if err != nil {
			return err
		}

		stats := systemStats{CPU: getCPUUsage(), RAM: getRAMUsage(), Disk: getDiskUsage()}
		renderMetrics(registry, stats)

		time.Sleep(5 * time.Second)
	}
}

type systemStats struct {
	CPU  string
	RAM  string
	Disk string
}

func renderMetrics(registry core.Registry, stats systemStats) {
	clear()

	var b strings.Builder

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[→] EZDeploy Metrics")
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "  %-20s %-12s %-8s %-12s %s\n", "PROJECT", "STATUS", "PORT", "MEMORY", "STARTED")
	fmt.Fprintf(&b, "  %-20s %-12s %-8s %-12s %s\n", "-------", "------", "----", "------", "-------")

	for name, info := range registry {
		serviceName := info.ServiceName
		if serviceName == "" {
			serviceName = core.ManagedName(name)
		}

		status := getServiceStatus(serviceName)
		memory := getServiceMemory(serviceName)
		started := getServiceUptime(serviceName)

		statusDisplay := "[✗] " + status
		if status == "active" {
			statusDisplay = "[✓] " + status
		}

		port := "?"
		if info.Port != 0 {
			port = strconv.Itoa(info.Port)
		}

		fmt.Fprintf(&b, "  %-20s %-12s %-8s %-12s %s\n", name, statusDisplay, port, memory, started)
	}

	fmt.Fprintf(&b, "\n  CPU:  %s\n", stats.CPU)
	fmt.Fprintf(&b, "  RAM:  %s\n", stats.RAM)
	fmt.Fprintf(&b, "  Disk: %s\n", stats.Disk)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "  refreshing every 5s — ctrl+c to exit")
	fmt.Fprintln(&b)

	terminalPrint(b.String())
}

func getServiceStatus(serviceName string) string {
	out, err := commandOutput("systemctl", "is-active", serviceName)
	if err != nil {
		return "unknown"
	}

	status := strings.TrimSpace(out)
	if status == "" {
		return "unknown"
	}

	return status
}

func getServiceUptime(serviceName string) string {
	value, ok := systemdProperty(serviceName, "ActiveEnterTimestamp")
	if !ok {
		return "unknown"
	}
	return value
}

func getServiceMemory(serviceName string) string {
	value, ok := systemdProperty(serviceName, "MemoryCurrent")
	if !ok {
		return "unknown"
	}
	bytes, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "unknown"
	}

	mb := bytes / 1024 / 1024
	return fmt.Sprintf("%.1fMB", mb)
}

func systemdProperty(serviceName, property string) (string, bool) {
	out, err := commandOutput("systemctl", "show", serviceName, "--property="+property)
	if err != nil {
		return "", false
	}
	_, value, ok := strings.Cut(strings.TrimSpace(out), "=")
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func getCPUUsage() string {
	out, err := commandOutput("top", "-bn1")
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "%Cpu") || strings.Contains(line, "Cpu(s)") {
			re := regexp.MustCompile(`(\d+\.?\d*)\s*id`)
			match := re.FindStringSubmatch(line)
			if len(match) < 2 {
				return "unknown"
			}

			idle, err := strconv.ParseFloat(match[1], 64)
			if err != nil {
				return "unknown"
			}

			usage := 100 - idle
			return fmt.Sprintf("%.1f%%", usage)
		}
	}

	return "unknown"
}

func getRAMUsage() string {
	out, err := commandOutput("free", "-h")
	if err != nil {
		return "unknown"
	}

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Mem:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return fmt.Sprintf("%s/%s", parts[2], parts[1])
			}
		}
	}

	return "unknown"
}

func getDiskUsage() string {
	out, err := commandOutput("df", "-h", "/")
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return "unknown"
	}

	parts := strings.Fields(lines[1])
	if len(parts) < 3 {
		return "unknown"
	}

	used := parts[2]
	total := parts[1]

	return fmt.Sprintf("%s/%s", used, total)
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func clear() {
	fmt.Print("\033[H\033[2J\r")
	_ = os.Stdout.Sync()
}

func terminalPrint(s string) {
	// Normalize existing CRLF first so we do not create \r\r\n.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\r\n")

	fmt.Print(s)
}

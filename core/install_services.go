package core

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// getPackageManager returns the system package manager based on the detected OS.
func getPackageManager() (string, bool) {
	switch GetOS().ID {
	case "ubuntu", "debian":
		return "apt", true
	case "amzn", "fedora", "rhel", "centos":
		return "dnf", true
	default:
		return "", false
	}
}

// Run is the single streamed command runner used by core and services.
func Run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const minimumHeavyMemory = 512 << 20

// runHeavy guards memory-intensive installs/builds and reports elapsed time while they run.
func runHeavy(dir, label, name string, args ...string) error {
	if available := availableMemory(); available > 0 && available < minimumHeavyMemory {
		return fmt.Errorf("%s requires at least %d MiB of available memory or swap; only %d MiB is available (free memory, enable swap, or resize the instance)", label, minimumHeavyMemory>>20, available>>20)
	}
	cmd, stopProgress := exec.Command(name, args...), startProgress(label)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	defer stopProgress()
	if err := cmd.Run(); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "signal: killed") {
			return fmt.Errorf("%s was killed by the operating system; memory exhaustion is likely: %w", label, err)
		}
		return err
	}
	return nil
}

func availableMemory() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	available := parseAvailableMemory(string(data))
	limit, limitOK := readMemoryNumber("/sys/fs/cgroup/memory.max")
	current, currentOK := readMemoryNumber("/sys/fs/cgroup/memory.current")
	if limitOK && currentOK && limit > current && (available == 0 || limit-current < available) {
		available = limit - current
	}
	return available
}

func parseAvailableMemory(data string) int64 {
	var available int64
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "MemAvailable:" && fields[0] != "SwapFree:") {
			continue
		}
		if value, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			available += value << 10
		}
	}
	return available
}

func readMemoryNumber(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return value, err == nil
}

func startProgress(label string) func() {
	info, err := os.Stderr.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return func() {}
	}
	done, stopped, started := make(chan struct{}), make(chan struct{}), time.Now()
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		frames, frame := []string{"\u280b", "\u2839", "\u283c", "\u2826"}, 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "[%s] %s (%s elapsed)\n", frames[frame%len(frames)], label, time.Since(started).Round(time.Second))
				frame++
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// EnsureTools installs only when a deploy needs a command that is not present.
func EnsureTools(items ...string) error {
	binaries := map[string][]string{
		"docker": {"docker"}, "nginx": {"nginx"}, "certbot": {"certbot"},
		"go": {"go"}, "python": {"python3"}, "node": {"node", "npm"},
	}
	var missing []string
	for _, item := range items {
		required, ok := binaries[item]
		if !ok {
			return fmt.Errorf("unknown install item %q", item)
		}
		unavailable := false
		for _, binary := range required {
			if _, err := exec.LookPath(binary); err != nil {
				unavailable = true
				break
			}
		}
		// Debian-family systems split ensurepip into python3-venv even when python3 is already installed.
		if !unavailable && item == "python" {
			unavailable = exec.Command("python3", "-c", "import ensurepip, venv").Run() != nil
		}
		if unavailable {
			missing = append(missing, item)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	fmt.Printf("[→] Installing missing server components: %s\n", strings.Join(missing, ", "))
	return Install(missing, false)
}

// Install installs the requested language runtimes and infrastructure
// components (nginx, certbot, docker). nginx and certbot are the default
// reverse-proxy/SSL stack: if neither is explicitly selected, both are
// installed automatically as a fallback. If either is explicitly selected,
// only the selected ones run — no implicit extras.
func Install(items []string, defaultProxy bool) error {
	pm, ok := getPackageManager()
	if !ok {
		return fmt.Errorf("[!] unsupported OS — cannot install dependencies")
	}
	fmt.Printf("[→] Found package manager: %s\n", pm)

	set := make(map[string]bool, len(items))
	for _, it := range items {
		set[strings.ToLower(strings.TrimSpace(it))] = true
	}

	packages := map[string][]string{
		"go": {"golang"}, "python": {"python3", "python3-pip"}, "node": {"nodejs", "npm"},
	}
	if pm == "apt" {
		packages["go"] = []string{"golang-go"}
		packages["python"] = []string{"python3", "python3-pip", "python3-venv"}
	}
	for _, runtime := range []string{"go", "python", "node"} {
		if !set[runtime] {
			continue
		}
		label := "Installing " + strings.ToUpper(runtime[:1]) + runtime[1:]
		fmt.Printf("[→] %s...\n", label)
		args := append([]string{"install"}, packages[runtime]...)
		if err := runHeavy("", label, pm, append(args, "-y")...); err != nil {
			return fmt.Errorf("%s install: %w", runtime, err)
		}
	}

	if set["rust"] {
		fmt.Println("[→] Installing Rust...")
		if err := Run("", "curl", "--proto", "=https", "--tlsv1.2", "-sSf",
			"https://sh.rustup.rs", "-o", "/tmp/rustup.sh"); err != nil {
			return fmt.Errorf("rustup download: %w", err)
		}
		if err := runHeavy("", "Installing Rust", "sh", "/tmp/rustup.sh", "-y"); err != nil {
			return fmt.Errorf("rustup install: %w", err)
		}
	}

	if set["docker"] {
		if err := installDocker(pm); err != nil {
			return err
		}
	}

	// nginx/certbot are the default reverse-proxy stack. If the user picked
	// neither explicitly, fall back to installing both.
	wantNginx := set["nginx"]
	wantCertbot := set["certbot"]
	if defaultProxy && !wantNginx && !wantCertbot {
		wantNginx = true
		wantCertbot = true
	}

	if wantNginx {
		if err := installNginx(pm); err != nil {
			return err
		}
	}

	if wantCertbot {
		if err := installCertbot(pm); err != nil {
			return err
		}
	}

	fmt.Println("[✓] Server ready")
	return nil
}

// installDocker installs Docker Engine and enables/starts the service.
func installDocker(pm string) error {
	fmt.Println("[→] Installing Docker...")
	pkg := "docker.io"
	if pm == "dnf" {
		pkg = "docker"
	}
	if err := runHeavy("", "Installing Docker", pm, "install", pkg, "-y"); err != nil {
		return fmt.Errorf("docker install: %w", err)
	}

	fmt.Println("[→] Enabling Docker service...")
	if err := enableService("docker"); err != nil {
		return err
	}

	fmt.Println("[✓] Docker installed")
	return nil
}

// installNginx installs nginx, enables/starts it, and prepares the
// sites-available/sites-enabled layout + nginx.conf patch.
func installNginx(pm string) error {
	fmt.Println("[→] Installing nginx...")
	if err := runHeavy("", "Installing Nginx", pm, "install", "nginx", "-y"); err != nil {
		return fmt.Errorf("nginx install: %w", err)
	}

	fmt.Println("[→] Configuring nginx service...")
	if err := enableService("nginx"); err != nil {
		return err
	}

	for _, dir := range []string{"/etc/nginx/sites-available", "/etc/nginx/sites-enabled"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	if err := patchNginxConf(); err != nil {
		return err
	}

	fmt.Println("[✓] nginx installed")
	return nil
}

// patchNginxConf ensures /etc/nginx/nginx.conf includes the sites-enabled directory.
func patchNginxConf() error {
	const path = "/etc/nginx/nginx.conf"

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read nginx.conf: %w", err)
	}

	content := string(data)
	if strings.Contains(content, "sites-enabled") {
		fmt.Println("[✓] nginx.conf already configured")
		return nil
	}

	const anchor = "include /etc/nginx/conf.d/*.conf;"
	patched := strings.Replace(
		content,
		anchor,
		anchor+"\n    include /etc/nginx/sites-enabled/*;",
		1,
	)

	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		return fmt.Errorf("write nginx.conf: %w", err)
	}

	fmt.Println("[✓] Added sites-enabled to nginx.conf")
	return nil
}

func installCertbot(pm string) error {
	fmt.Println("[→] Installing certbot...")
	if pm != "apt" && pm != "dnf" {
		return fmt.Errorf("certbot install: unsupported package manager %q", pm)
	}
	if err := runHeavy("", "Installing Certbot", pm, "install", "certbot", "python3-certbot-nginx", "-y"); err != nil {
		return fmt.Errorf("certbot %s install: %w", pm, err)
	}
	fmt.Println("[✓] certbot installed")
	return nil
}

func enableService(name string) error {
	for _, action := range []string{"enable", "start"} {
		if err := Run("", "systemctl", action, name); err != nil {
			return fmt.Errorf("systemctl %s %s: %w", action, name, err)
		}
	}
	return nil
}

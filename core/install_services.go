package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// run executes a command, streaming its output directly to stdout/stderr.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EnsureTools installs only when a deploy needs a command that is not present.
func EnsureTools(items ...string) error {
	binaries := map[string]string{
		"docker": "docker", "nginx": "nginx", "certbot": "certbot",
	}
	var missing []string
	for _, item := range items {
		binary := binaries[item]
		if binary == "" {
			return fmt.Errorf("unknown install item %q", item)
		}
		if _, err := exec.LookPath(binary); err != nil {
			missing = append(missing, item)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	fmt.Printf("[→] Installing missing server components: %s\n", strings.Join(missing, ", "))
	return install(missing, false)
}

// Install installs the requested language runtimes and infrastructure
// components (nginx, certbot, docker). nginx and certbot are the default
// reverse-proxy/SSL stack: if neither is explicitly selected, both are
// installed automatically as a fallback. If either is explicitly selected,
// only the selected ones run — no implicit extras.
func Install(items []string) error {
	return install(items, true)
}

func install(items []string, defaultProxy bool) error {
	pm, ok := getPackageManager()
	if !ok {
		return fmt.Errorf("[!] unsupported OS — cannot install dependencies")
	}
	fmt.Printf("[→] Found package manager: %s\n", pm)

	set := make(map[string]bool, len(items))
	for _, it := range items {
		set[strings.ToLower(strings.TrimSpace(it))] = true
	}

	if set["go"] {
		fmt.Println("[→] Installing Go...")
		pkg := "golang"
		if pm == "apt" {
			pkg = "golang-go"
		}
		if err := run(pm, "install", pkg, "-y"); err != nil {
			return fmt.Errorf("go install: %w", err)
		}
	}

	if set["python"] {
		fmt.Println("[→] Installing Python...")
		if err := run(pm, "install", "python3", "python3-pip", "-y"); err != nil {
			return fmt.Errorf("python install: %w", err)
		}
	}

	if set["node"] {
		fmt.Println("[→] Installing Node...")
		if err := run(pm, "install", "nodejs", "npm", "-y"); err != nil {
			return fmt.Errorf("node install: %w", err)
		}
	}

	if set["rust"] {
		fmt.Println("[→] Installing Rust...")
		if err := run("curl", "--proto", "=https", "--tlsv1.2", "-sSf",
			"https://sh.rustup.rs", "-o", "/tmp/rustup.sh"); err != nil {
			return fmt.Errorf("rustup download: %w", err)
		}
		if err := run("sh", "/tmp/rustup.sh", "-y"); err != nil {
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
	if err := run(pm, "install", pkg, "-y"); err != nil {
		return fmt.Errorf("docker install: %w", err)
	}

	fmt.Println("[→] Enabling Docker service...")
	if err := run("systemctl", "enable", "docker"); err != nil {
		return fmt.Errorf("systemctl enable docker: %w", err)
	}
	if err := run("systemctl", "start", "docker"); err != nil {
		return fmt.Errorf("systemctl start docker: %w", err)
	}

	fmt.Println("[✓] Docker installed")
	return nil
}

// installNginx installs nginx, enables/starts it, and prepares the
// sites-available/sites-enabled layout + nginx.conf patch.
func installNginx(pm string) error {
	fmt.Println("[→] Installing nginx...")
	if err := run(pm, "install", "nginx", "-y"); err != nil {
		return fmt.Errorf("nginx install: %w", err)
	}

	fmt.Println("[→] Configuring nginx service...")
	if err := run("systemctl", "enable", "nginx"); err != nil {
		return fmt.Errorf("systemctl enable nginx: %w", err)
	}
	if err := run("systemctl", "start", "nginx"); err != nil {
		return fmt.Errorf("systemctl start nginx: %w", err)
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

	switch pm {
	case "apt":
		// Simple, dependency-safe path for Ubuntu/Debian.
		if err := run(pm, "install", "certbot", "python3-certbot-nginx", "-y"); err != nil {
			return fmt.Errorf("certbot apt install: %w", err)
		}

	case "dnf":
		// Works only where these packages are available.
		// Amazon Linux may need extra repo handling later.
		if err := run(pm, "install", "certbot", "python3-certbot-nginx", "-y"); err != nil {
			return fmt.Errorf("certbot dnf install: %w", err)
		}

	default:
		return fmt.Errorf("certbot install: unsupported package manager %q", pm)
	}

	fmt.Println("[✓] certbot installed")
	return nil
}

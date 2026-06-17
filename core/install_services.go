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

// Install installs the requested language runtimes and sets up nginx + certbot.
func Install(languages []string) error {
	pm, ok := getPackageManager()
	if !ok {
		return fmt.Errorf("[!] unsupported OS — cannot install dependencies")
	}
	fmt.Printf("[→] Found package manager: %s\n", pm)

	// Normalise into a set for O(1) lookups
	langs := make(map[string]bool, len(languages))
	for _, l := range languages {
		langs[strings.ToLower(strings.TrimSpace(l))] = true
	}

	if langs["go"] {
		fmt.Println("[→] Installing Go...")
		pkg := "golang"
		if pm == "apt" {
			pkg = "golang-go"
		}
		if err := run(pm, "install", pkg, "-y"); err != nil {
			return fmt.Errorf("go install: %w", err)
		}
	}

	if langs["python"] {
		fmt.Println("[→] Installing Python...")
		if err := run(pm, "install", "python3", "python3-pip", "-y"); err != nil {
			return fmt.Errorf("python install: %w", err)
		}
	}

	if langs["node"] {
		fmt.Println("[→] Installing Node...")
		if err := run(pm, "install", "nodejs", "npm", "-y"); err != nil {
			return fmt.Errorf("node install: %w", err)
		}
	}

	if langs["rust"] {
		fmt.Println("[→] Installing Rust...")
		if err := run("curl", "--proto", "=https", "--tlsv1.2", "-sSf",
			"https://sh.rustup.rs", "-o", "/tmp/rustup.sh"); err != nil {
			return fmt.Errorf("rustup download: %w", err)
		}
		if err := run("sh", "/tmp/rustup.sh", "-y"); err != nil {
			return fmt.Errorf("rustup install: %w", err)
		}
	}

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

	fmt.Println("[→] Installing certbot...")
	if err := run("pip3", "install", "certbot", "certbot-nginx"); err != nil {
		return fmt.Errorf("certbot install: %w", err)
	}

	fmt.Println("[✓] Server ready")
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

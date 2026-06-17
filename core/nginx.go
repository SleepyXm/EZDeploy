package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	nginxSitesAvailable = "/etc/nginx/sites-available"
	nginxSitesEnabled   = "/etc/nginx/sites-enabled"
	webhookPort         = 9001
)

// locationBlock generates an nginx location block proxying to the given port.
// When streaming is true, WebSocket/SSE upgrade headers are included.
func locationBlock(paths []string, port int, streaming bool) string {
	var locType, pattern string

	if len(paths) == 1 && paths[0] == "/" {
		locType, pattern = "location", "/"
	} else {
		locType = "location ~"
		pattern = "^/(" + strings.Join(paths, "|") + ")"
	}

	block := fmt.Sprintf(`    %s %s {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Host $host;`, locType, pattern, port)

	if streaming {
		block += `
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_cache_bypass $http_upgrade;
        proxy_buffering off;`
	}

	block += "\n    }"
	return block
}

// CreateNginxConfig writes a server block for the project to sites-available,
// symlinks it into sites-enabled, tests, reloads nginx, and provisions SSL.
func CreateNginxConfig(projectName string, port int, domain, email string) error {
	configPath := filepath.Join(nginxSitesAvailable, projectName)
	symlinkPath := filepath.Join(nginxSitesEnabled, projectName)

	// Separate domain from optional path component (e.g. "example.com/app" → "example.com", "/app")
	domainPart, location := domain, "/"
	if idx := strings.IndexByte(domain, '/'); idx != -1 {
		domainPart = domain[:idx]
		location = domain[idx:]
	}

	config := fmt.Sprintf(`server {
    listen 80;
    server_name %s;
%s
%s
}
`,
		domainPart,
		locationBlock([]string{location}, port, true),
		locationBlock([]string{"/gh-webhook"}, webhookPort, false),
	)

	fmt.Printf("[→] Writing nginx config for %s...\n", projectName)
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write nginx config: %w", err)
	}

	if !fileExists(symlinkPath) {
		if err := os.Symlink(configPath, symlinkPath); err != nil {
			return fmt.Errorf("symlink sites-enabled: %w", err)
		}
		fmt.Println("[✓] Symlinked to sites-enabled")
	}

	fmt.Println("[→] Testing nginx config...")
	if err := run("nginx", "-t"); err != nil {
		return fmt.Errorf("nginx -t: %w", err)
	}

	fmt.Println("[→] Reloading nginx...")
	if err := run("systemctl", "reload", "nginx"); err != nil {
		return fmt.Errorf("systemctl reload nginx: %w", err)
	}

	fmt.Println("[✓] Nginx configured")
	return setupSSL(domainPart, email)
}

// setupSSL provisions a Let's Encrypt certificate for domain via certbot.
func setupSSL(domain, email string) error {
	fmt.Printf("[→] Setting up SSL for %s...\n", domain)
	if err := run("certbot", "--nginx",
		"-d", domain,
		"--non-interactive",
		"--agree-tos",
		"--redirect",
		"-m", email,
	); err != nil {
		return fmt.Errorf("certbot: %w", err)
	}
	fmt.Printf("[✓] SSL configured, %s is now HTTPS\n", domain)
	return nil
}

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	nginxSitesAvailable = "/etc/nginx/sites-available"
	nginxSitesEnabled   = "/etc/nginx/sites-enabled"
	webhookPort         = 9001
)

var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// proxyBlock keeps the existing streaming-compatible proxy settings in one place.
func proxyBlock(location string, port int, streaming bool) string {
	block := fmt.Sprintf(`    %s {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Host $host;`, location, port)

	if streaming {
		block += `
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_cache_bypass $http_upgrade;
        proxy_buffering off;`
	}
	return block + "\n    }"
}

func routeLocation(path string) (string, error) {
	path = normalizeRoutePath(path)
	if strings.ContainsAny(path, "\r\n\t ;#$\"'\\") {
		return "", fmt.Errorf("invalid route path %q", path)
	}

	dynamic := false
	// Dynamic routes accept either trailing-slash form without creating a double slash.
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for index, segment := range segments {
		switch {
		case strings.HasPrefix(segment, "*"), strings.HasPrefix(segment, "<path:"):
			dynamic = true
			segments[index] = ".*"
		case strings.HasPrefix(segment, ":"),
			strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"),
			strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">"):
			dynamic = true
			segments[index] = "[^/]+"
		default:
			segments[index] = regexp.QuoteMeta(segment)
		}
	}

	if !dynamic {
		return "location = " + path, nil
	}
	return "location ~ ^/" + strings.Join(segments, "/") + "/?$", nil
}

func normalizeRoutePath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func joinRoutePath(prefix, path string) string {
	if prefix == "/" {
		return normalizeRoutePath(path)
	}
	return normalizeRoutePath(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/"))
}

// CreateNginxConfig writes, validates, and enables a project server block.
// whitelist=false preserves the old unrestricted proxy behavior explicitly.
func CreateNginxConfig(projectName string, port int, domain, email string, routes []string, whitelist bool) error {
	if !projectNamePattern.MatchString(projectName) || len(projectName) > 100 {
		return fmt.Errorf("invalid project name %q", projectName)
	}
	configPath := filepath.Join(nginxSitesAvailable, projectName)
	symlinkPath := filepath.Join(nginxSitesEnabled, projectName)
	config, domainPart, err := renderNginxConfig(port, domain, routes, whitelist)
	if err != nil {
		return err
	}

	previous, readErr := os.ReadFile(configPath)
	hadPrevious := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read nginx config: %w", readErr)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write nginx config: %w", err)
	}

	createdLink := false
	if !fileExists(symlinkPath) {
		if err := os.Symlink(configPath, symlinkPath); err != nil {
			restoreNginxConfig(configPath, previous, hadPrevious)
			return fmt.Errorf("symlink sites-enabled: %w", err)
		}
		createdLink = true
	}

	// A rejected config never replaces the last known valid file.
	if err := run("nginx", "-t"); err != nil {
		restoreNginxConfig(configPath, previous, hadPrevious)
		if createdLink {
			_ = os.Remove(symlinkPath)
		}
		return fmt.Errorf("nginx -t: %w", err)
	}
	if err := run("systemctl", "reload", "nginx"); err != nil {
		return fmt.Errorf("systemctl reload nginx: %w", err)
	}

	fmt.Println("[✓] Nginx configured")
	return setupSSL(domainPart, email)
}

func renderNginxConfig(port int, domain string, routes []string, whitelist bool) (string, string, error) {
	if port < 1 || port > 65535 {
		return "", "", fmt.Errorf("invalid port %d", port)
	}
	domain = strings.TrimSpace(domain)
	domainPart, basePath := domain, "/"
	if idx := strings.IndexByte(domain, '/'); idx != -1 {
		domainPart, basePath = domain[:idx], domain[idx:]
	}
	if !serverNamePattern.MatchString(domainPart) {
		return "", "", fmt.Errorf("invalid domain %q", domainPart)
	}
	if _, err := routeLocation(basePath); err != nil {
		return "", "", fmt.Errorf("invalid domain base path: %w", err)
	}

	var appLocations string
	if whitelist {
		seen := map[string]bool{}
		for _, route := range routes {
			path := joinRoutePath(basePath, route)
			// EZDeploy owns this exact path for its webhook listener.
			if path == "/gh-webhook" {
				return "", "", fmt.Errorf("route /gh-webhook is reserved by EZDeploy")
			}
			seen[path] = true
		}
		if len(seen) == 0 {
			return "", "", fmt.Errorf("no routes discovered; add routes manually or disable whitelisting")
		}

		paths := make([]string, 0, len(seen))
		for path := range seen {
			paths = append(paths, path)
		}
		sort.Strings(paths)

		var blocks []string
		for _, path := range paths {
			location, err := routeLocation(path)
			if err != nil {
				return "", "", err
			}
			blocks = append(blocks, proxyBlock(location, port, true))
		}
		appLocations = strings.Join(blocks, "\n") + "\n    location / {\n        return 404;\n    }"
	} else {
		appLocations = proxyBlock("location "+normalizeRoutePath(basePath), port, true)
	}

	config := fmt.Sprintf(`server {
    listen 80;
    server_name %s;
%s
%s
}
`, domainPart, appLocations, proxyBlock("location = /gh-webhook", webhookPort, false))
	return config, domainPart, nil
}

func restoreNginxConfig(path string, previous []byte, existed bool) {
	if existed {
		_ = os.WriteFile(path, previous, 0o644)
	} else {
		_ = os.Remove(path)
	}
}

func setupSSL(domain, email string) error {
	fmt.Printf("[→] Setting up SSL for %s...\n", domain)
	if err := run("certbot", "--nginx", "-d", domain, "--non-interactive", "--agree-tos", "--redirect", "-m", email); err != nil {
		return fmt.Errorf("certbot: %w", err)
	}
	fmt.Printf("[✓] SSL configured, %s is now HTTPS\n", domain)
	return nil
}

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// PullConfig is written to ~/.ezdeploy/<project>.json after deploy.
// The token is a self-signed payload — the server verifies it with its
// private key, so this file contains no server-side secret.
type PullConfig struct {
	ProjectName string `json:"project_name"`
	Host        string `json:"host"`  // e.g. "https://myapp.com"
	Token       string `json:"token"` // <payloadB64>.<sigB64>
}

// RunPull is the entrypoint for `ezdeploy pull [--project <name>]`.
func RunPull(projectName string) error {
	cfg, err := loadPullConfig(projectName)
	if err != nil {
		return err
	}
	fmt.Printf("[→] Triggering pull for %s on %s...\n", cfg.ProjectName, cfg.Host)

	status, body, err := sendPullRequest(cfg)
	if err != nil {
		return fmt.Errorf("pull request failed: %w", err)
	}
	switch status {
	case http.StatusAccepted:
		fmt.Printf("[✓] Server accepted — %s is redeploying\n", cfg.ProjectName)
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("token rejected — run `ezdeploy token rotate %s` on the server to issue a new one", cfg.ProjectName)
	default:
		return fmt.Errorf("unexpected server response %d: %s", status, strings.TrimSpace(string(body)))
	}
}

func sendPullRequest(cfg PullConfig) (int, []byte, error) {
	payload, _ := json.Marshal(map[string]string{"token": cfg.Token})
	client := &http.Client{Timeout: 10 * time.Second}
	endpoint := strings.TrimRight(cfg.Host, "/") + "/ezdeploy-pull"
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, body, nil
}

// ── Local config helpers ──────────────────────────────────────────────────────

func configDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.ezdeploy"
}

func configPath(projectName string) string {
	return configDir() + "/" + projectName + ".json"
}

// SavePullConfig writes the pull config to ~/.ezdeploy/<project>.json.
// Called once on the server immediately after deploy, then printed/saved
// for the user to carry to their local machine.
func SavePullConfig(cfg PullConfig) error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(cfg.ProjectName), data, 0o600)
}

func loadPullConfig(projectName string) (PullConfig, error) {
	if projectName != "" {
		return readPullConfig(configPath(projectName))
	}
	entries, err := os.ReadDir(configDir())
	if err != nil {
		return PullConfig{}, fmt.Errorf("no pull configs found — deploy a project first")
	}
	var configs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			configs = append(configs, e.Name())
		}
	}
	switch len(configs) {
	case 0:
		return PullConfig{}, fmt.Errorf("no pull configs found — deploy a project first")
	case 1:
		return readPullConfig(configDir() + "/" + configs[0])
	default:
		names := make([]string, len(configs))
		for i, c := range configs {
			names[i] = strings.TrimSuffix(c, ".json")
		}
		return PullConfig{}, fmt.Errorf(
			"multiple projects found, specify one: ezdeploy pull --project <%s>",
			strings.Join(names, "|"),
		)
	}
}

func readPullConfig(path string) (PullConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PullConfig{}, fmt.Errorf("pull config not found at %s", path)
	}
	var cfg PullConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return PullConfig{}, fmt.Errorf("invalid pull config: %w", err)
	}
	return cfg, nil
}

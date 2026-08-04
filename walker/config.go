package walker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "yamls/walk.yml"

type Config struct {
	Name             string        `yaml:"name"`
	MaxFileSizeBytes int64         `yaml:"max_file_size_bytes"`
	Languages        []LanguageDef `yaml:"languages"`
	IgnoredDirs      []string      `yaml:"ignored_dirs"`
	IgnoredFiles     []string      `yaml:"ignored_files"`
	ScanFiles        []string      `yaml:"scan_files"`
	ScanExtensions   []string      `yaml:"scan_extensions"`
	Dockerfiles      []string      `yaml:"dockerfiles"`
	EnvRules         []RuleDef     `yaml:"env_rules"`
	RouteMethods     []string      `yaml:"route_methods"`
	PrefixRules      []RuleDef     `yaml:"prefix_rules"`
	RouteRules       []RuleDef     `yaml:"route_rules"`
}

type LanguageDef struct {
	Name       string   `yaml:"name"`
	Extensions []string `yaml:"extensions"`
}

type RuleDef struct {
	Name       string   `yaml:"name"`
	Language   string   `yaml:"language"`
	Languages  []string `yaml:"languages"`
	Extensions []string `yaml:"extensions"`
	Files      []string `yaml:"files"`
	Pattern    string   `yaml:"pattern"`
	Multi      bool     `yaml:"multi"`
}

func LoadDefaultConfig() (*Config, error) {
	return LoadConfig("")
}

func LoadConfig(path string) (*Config, error) {
	path = strings.TrimSpace(path)

	if path == "" {
		path = filepath.Join("yamls", "walk.yml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read walker config %s: %w", path, err)
	}

	return decodeConfig(data, path)
}

func decodeConfig(data []byte, source string) (*Config, error) {
	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("%s: missing config name", source)
	}

	if len(cfg.EnvRules) == 0 {
		return nil, fmt.Errorf("%s: no env_rules defined", source)
	}

	return &cfg, nil
}

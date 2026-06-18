package walker

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed walk.yml
var defaultRules []byte

type Config struct {
	Name             string        `yaml:"name"`
	MaxFileSizeBytes int64         `yaml:"max_file_size_bytes"`
	Languages        []LanguageDef `yaml:"languages"`
	IgnoredDirs      []string      `yaml:"ignored_dirs"`
	IgnoredFiles     []string      `yaml:"ignored_files"`
	ScanFiles        []string      `yaml:"scan_files"`
	ScanExtensions   []string      `yaml:"scan_extensions"`
	EnvRules         []RuleDef     `yaml:"env_rules"`
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
}

func LoadDefaultConfig() (*Config, error) {
	return decodeConfig(defaultRules, "embedded walk.yml")
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env rules: %w", err)
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

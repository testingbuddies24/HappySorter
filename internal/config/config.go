// Package config loads and persists HappySorter's runtime configuration.
//
// Schema and defaults mirror docs/ARCHITECTURE.md § 7 exactly.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Paths    PathsConfig    `yaml:"paths"`
	Scraping ScrapingConfig `yaml:"scraping"`
	Sources  []SourceConfig `yaml:"sources"`
	Rename   RenameConfig   `yaml:"rename"`
}

type ServerConfig struct {
	Port     int    `yaml:"port"`
	LogLevel string `yaml:"log_level"`
}

type PathsConfig struct {
	Watch           string `yaml:"watch"`
	Library         string `yaml:"library"`
	ReviewFilter    string `yaml:"review_filter"`
	ReviewUnmatched string `yaml:"review_unmatched"`
	ReviewDuplicate string `yaml:"review_duplicate"`
}

type ScrapingConfig struct {
	DefaultQPS     float64 `yaml:"default_qps"`
	TimeoutSeconds int     `yaml:"timeout_seconds"`
	ProxyURL       string  `yaml:"proxy_url"`
	CookiesDir     string  `yaml:"cookies_dir"`
}

type SourceConfig struct {
	Name     string  `yaml:"name"`
	Enabled  bool    `yaml:"enabled"`
	Priority int     `yaml:"priority"`
	QPS      float64 `yaml:"qps"`
}

type RenameConfig struct {
	FolderTemplate     string `yaml:"folder_template"`
	FileTemplate       string `yaml:"file_template"`
	UnknownPlaceholder string `yaml:"unknown_placeholder"`
}

// Default returns the shipped default configuration: studio-direct sources
// first (no Cloudflare), aggregators as fallback, everything disabled until
// the user opts in via the setup GUI.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port:     8080,
			LogLevel: "info",
		},
		Paths: PathsConfig{
			Watch:           "/watch",
			Library:         "/library",
			ReviewFilter:    "/library/review/_filter",
			ReviewUnmatched: "/library/review/_unmatched",
			ReviewDuplicate: "/library/review/_duplicate",
		},
		Scraping: ScrapingConfig{
			DefaultQPS:     1.0,
			TimeoutSeconds: 30,
			ProxyURL:       "",
			CookiesDir:     "/config/cookies",
		},
		Sources: []SourceConfig{
			{Name: "s1", Enabled: false, Priority: 1, QPS: 1.0},
			{Name: "ideapocket", Enabled: false, Priority: 2, QPS: 1.0},
			{Name: "javbus", Enabled: false, Priority: 3, QPS: 1.0},
			{Name: "javdb", Enabled: false, Priority: 4, QPS: 1.0},
			{Name: "javlibrary", Enabled: false, Priority: 5, QPS: 0.5},
		},
		Rename: RenameConfig{
			FolderTemplate:     "{code} ({year})",
			FileTemplate:       "{code} ({year})",
			UnknownPlaceholder: "Unknown",
		},
	}
}

// Load reads config from path, writing out Default() first if the file
// doesn't exist yet (first-run case for a fresh /config mount).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Default()
		if writeErr := save(path, cfg); writeErr != nil {
			return nil, fmt.Errorf("writing default config: %w", writeErr)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Scrape   ScrapeConfig   `yaml:"scrape"`
	Schedule ScheduleConfig `yaml:"schedule"`
	Brewers  []string       `yaml:"brewers"`
	Ntfy     NtfyConfig     `yaml:"ntfy"`
}

type ScrapeConfig struct {
	URL                string `yaml:"url"`
	DraftSection       string `yaml:"draft_section"`
	PageTimeoutSeconds int    `yaml:"page_timeout_seconds"`
}

type ScheduleConfig struct {
	Cron string `yaml:"cron"`
}

type NtfyConfig struct {
	Topic    string   `yaml:"topic"`
	BaseURL  string   `yaml:"base_url"`
	Priority string   `yaml:"priority"`
	Tags     []string `yaml:"tags"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply defaults
	if cfg.Ntfy.BaseURL == "" {
		cfg.Ntfy.BaseURL = "https://ntfy.sh"
	}
	if cfg.Scrape.PageTimeoutSeconds == 0 {
		cfg.Scrape.PageTimeoutSeconds = 30
	}
	if cfg.Scrape.DraftSection == "" {
		cfg.Scrape.DraftSection = "on draft"
	}
	if cfg.Ntfy.Priority == "" {
		cfg.Ntfy.Priority = "default"
	}

	return &cfg, nil
}

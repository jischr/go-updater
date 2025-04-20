package config

import (
	"time"
)

// Config holds all configuration for the application
type Config struct {
	RepoOwner     string        `mapstructure:"repo_owner"`
	RepoName      string        `mapstructure:"repo_name"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	BinaryPrefix  string        `mapstructure:"binary_prefix"`
	ProxyPort     int           `mapstructure:"proxy_port"`
	StartingPort  int           `mapstructure:"starting_port"`
	GithubAPIURL  string        `mapstructure:"github_api_url"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		RepoOwner:     "jischr",
		RepoName:      "simple-server",
		CheckInterval: 30 * time.Second,
		BinaryPrefix:  "simple-server",
		ProxyPort:     8080,
		StartingPort:  5000,
		GithubAPIURL:  "https://api.github.com/repos/%s/%s/releases/latest",
	}
}

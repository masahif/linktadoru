// Package config provides configuration management for the crawler.
// It defines configuration structures and default values for crawling parameters.
package config

import (
	"os"
	"time"
)

// BasicAuth contains HTTP Basic Authentication credentials
type BasicAuth struct {
	Username    string `mapstructure:"username" yaml:"username"`         // Username for basic auth
	Password    string `mapstructure:"password" yaml:"password"`         // Password for basic auth
	UsernameEnv string `mapstructure:"username_env" yaml:"username_env"` // Environment variable for username
	PasswordEnv string `mapstructure:"password_env" yaml:"password_env"` // Environment variable for password
}

// Auth contains authentication configuration
type Auth struct {
	Basic *BasicAuth `mapstructure:"basic" yaml:"basic"` // Basic authentication settings
}

// CrawlConfig holds crawler configuration
type CrawlConfig struct {
	// Basic crawling parameters
	SeedURLs       []string      `mapstructure:"seed_urls" yaml:"seed_urls"`             // Starting URLs for crawling
	Concurrency    int           `mapstructure:"concurrency" yaml:"concurrency"`         // Number of concurrent workers
	RequestDelay   time.Duration `mapstructure:"request_delay" yaml:"request_delay"`     // Delay between requests
	RequestTimeout time.Duration `mapstructure:"request_timeout" yaml:"request_timeout"` // HTTP request timeout
	UserAgent      string        `mapstructure:"user_agent" yaml:"user_agent"`           // HTTP User-Agent header
	RespectRobots  bool          `mapstructure:"respect_robots" yaml:"respect_robots"`   // Whether to respect robots.txt
	Limit          int           `mapstructure:"limit" yaml:"limit"`                     // Stop after N pages

	// Authentication
	Auth *Auth `mapstructure:"auth" yaml:"auth"` // Authentication configuration

	// URL filtering
	IncludePatterns []string `mapstructure:"include_patterns" yaml:"include_patterns"` // Regex patterns for URLs to include
	ExcludePatterns []string `mapstructure:"exclude_patterns" yaml:"exclude_patterns"` // Regex patterns for URLs to exclude

	// Database configuration
	DatabasePath string `mapstructure:"database_path" yaml:"database_path"` // Path to SQLite database file

}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *CrawlConfig {
	return &CrawlConfig{
		Concurrency:    10,
		RequestDelay:   1 * time.Second,
		RequestTimeout: 30 * time.Second,
		UserAgent:      "LinkTadoru/1.0",
		RespectRobots:  true,
		Limit:          0, // unlimited
		DatabasePath:   "./crawl.db",
	}
}

// Validate checks if the configuration is valid
func (c *CrawlConfig) Validate() error {
	// Note: SeedURLs are optional - crawler can resume from existing queue

	if c.Concurrency <= 0 {
		return ErrInvalidConcurrency
	}

	if c.RequestTimeout <= 0 {
		return ErrInvalidTimeout
	}

	// Enforce minimum delay of 100ms for proper queue coordination
	if c.RequestDelay < 100*time.Millisecond {
		c.RequestDelay = 100 * time.Millisecond
	}

	if c.DatabasePath == "" {
		return ErrEmptyDatabasePath
	}

	return nil
}

// GetBasicAuthCredentials returns the basic auth username and password,
// resolving environment variables if specified
func (c *CrawlConfig) GetBasicAuthCredentials() (username, password string) {
	if c.Auth == nil || c.Auth.Basic == nil {
		return "", ""
	}

	basic := c.Auth.Basic

	// Get username
	if basic.UsernameEnv != "" {
		username = os.Getenv(basic.UsernameEnv)
	} else {
		username = basic.Username
	}

	// Get password
	if basic.PasswordEnv != "" {
		password = os.Getenv(basic.PasswordEnv)
	} else {
		password = basic.Password
	}

	return username, password
}

// Package config provides configuration management for the crawler.
// It defines configuration structures and default values for crawling parameters.
package config

import (
	"time"
)

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

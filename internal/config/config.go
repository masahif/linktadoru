// Package config provides configuration management for the crawler.
// It defines configuration structures and default values for crawling parameters.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// BasicAuth contains HTTP Basic Authentication credentials
type BasicAuth struct {
	Username    string `mapstructure:"username" yaml:"username"`         // Username for basic auth
	Password    string `mapstructure:"password" yaml:"password"`         // Password for basic auth
	UsernameEnv string `mapstructure:"username_env" yaml:"username_env"` // Environment variable for username
	PasswordEnv string `mapstructure:"password_env" yaml:"password_env"` // Environment variable for password
}

// AuthType represents the type of authentication
type AuthType string

const (
	NoAuth         AuthType = ""
	BasicAuthType  AuthType = "basic"
	BearerAuthType AuthType = "bearer"
	APIKeyAuthType AuthType = "api-key"
)

// BearerAuth represents Bearer token authentication
type BearerAuth struct {
	Token    string `mapstructure:"token" yaml:"token"`         // Bearer token
	TokenEnv string `mapstructure:"token_env" yaml:"token_env"` // Environment variable for token
}

// APIKeyAuth represents API key authentication
type APIKeyAuth struct {
	Header    string `mapstructure:"header" yaml:"header"`         // Header name (e.g., "X-API-Key")
	Value     string `mapstructure:"value" yaml:"value"`           // Header value
	HeaderEnv string `mapstructure:"header_env" yaml:"header_env"` // Environment variable for header name
	ValueEnv  string `mapstructure:"value_env" yaml:"value_env"`   // Environment variable for header value
}

// Auth contains authentication configuration
type Auth struct {
	Type   AuthType    `mapstructure:"type" yaml:"type"`     // Authentication type
	Basic  *BasicAuth  `mapstructure:"basic" yaml:"basic"`   // Basic authentication settings
	Bearer *BearerAuth `mapstructure:"bearer" yaml:"bearer"` // Bearer authentication settings
	APIKey *APIKeyAuth `mapstructure:"apikey" yaml:"apikey"` // API key authentication settings
}

// CrawlConfig holds crawler configuration
type CrawlConfig struct {
	// Basic crawling parameters
	SeedURLs       []string      `mapstructure:"seed_urls" yaml:"seed_urls"`             // Starting URLs for crawling
	Concurrency    int           `mapstructure:"concurrency" yaml:"concurrency"`         // Number of concurrent workers
	RequestDelay   float64       `mapstructure:"request_delay" yaml:"request_delay"`     // Delay between requests
	RequestTimeout time.Duration `mapstructure:"request_timeout" yaml:"request_timeout"` // HTTP request timeout
	UserAgent      string        `mapstructure:"user_agent" yaml:"user_agent"`           // HTTP User-Agent header
	IgnoreRobotsTxt     bool          `mapstructure:"ignore_robots_txt" yaml:"ignore_robots_txt"`         // Whether to ignore robots.txt
	FollowExternalHosts bool          `mapstructure:"follow_external_hosts" yaml:"follow_external_hosts"` // Whether to crawl external hosts
	Limit               int           `mapstructure:"limit" yaml:"limit"`                                 // Stop after N pages

	// Authentication
	Auth *Auth `mapstructure:"auth" yaml:"auth"` // Authentication configuration

	// URL filtering
	IncludePatterns []string `mapstructure:"include_patterns" yaml:"include_patterns"` // Regex patterns for URLs to include
	ExcludePatterns []string `mapstructure:"exclude_patterns" yaml:"exclude_patterns"` // Regex patterns for URLs to exclude
	AllowedSchemes  []string `mapstructure:"allowed_schemes" yaml:"allowed_schemes"`   // Allowed URL schemes (e.g., https://, http://)

	// HTTP Headers
	Headers []string `mapstructure:"headers" yaml:"headers"` // Custom HTTP headers

	// Database configuration
	DatabasePath string `mapstructure:"database_path" yaml:"database_path"` // Path to SQLite database file

	// Logging configuration
	LogLevel      string `mapstructure:"log_level" yaml:"log_level"`           // Log level (debug, info, warn, error)
	LogFile       string `mapstructure:"log_file" yaml:"log_file"`             // Path to log file
	LogMaxSize    int    `mapstructure:"log_max_size" yaml:"log_max_size"`     // Max log file size in MB
	LogMaxBackups int    `mapstructure:"log_max_backups" yaml:"log_max_backups"` // Number of old log files to keep
	LogConsole    bool   `mapstructure:"log_console" yaml:"log_console"`       // Enable console output
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *CrawlConfig {
	return &CrawlConfig{
		Concurrency:         2,     // Reduced from 10 to 2
		RequestDelay:        0.1,   // 100ms in seconds // Reduced from 1s to 0.1s
		RequestTimeout:      30 * time.Second,
		UserAgent:           "LinkTadoru/1.0",
		IgnoreRobotsTxt:     false,
		FollowExternalHosts: false, // Default to same-host only for safety
		Limit:               0,     // unlimited
		DatabasePath:        "./linktadoru.db",
		AllowedSchemes:      []string{"https://", "http://"}, // Default allowed URL schemes
		// Logging defaults
		LogLevel:      "info",
		LogFile:       "",    // Empty means no file logging by default
		LogMaxSize:    100,   // 100MB
		LogMaxBackups: 5,
		LogConsole:    true,  // Console output enabled by default
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
	if c.RequestDelay < 0.1 {
		c.RequestDelay = 0.1 // 100ms in seconds
	}

	if c.DatabasePath == "" {
		return ErrEmptyDatabasePath
	}

	// Validate authentication configuration
	if err := c.validateAuth(); err != nil {
		return err
	}

	// Validate headers
	if err := c.validateHeaders(); err != nil {
		return err
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

// GetBearerToken returns the bearer token from config or environment
func (c *CrawlConfig) GetBearerToken() string {
	if c.Auth == nil || c.Auth.Bearer == nil {
		return ""
	}

	bearer := c.Auth.Bearer
	if bearer.TokenEnv != "" {
		return os.Getenv(bearer.TokenEnv)
	}
	return bearer.Token
}

// GetAPIKeyCredentials returns the API key header and value from config or environment
func (c *CrawlConfig) GetAPIKeyCredentials() (header, value string) {
	if c.Auth == nil || c.Auth.APIKey == nil {
		return "", ""
	}

	apikey := c.Auth.APIKey

	// Get header name
	if apikey.HeaderEnv != "" {
		header = os.Getenv(apikey.HeaderEnv)
	} else {
		header = apikey.Header
	}

	// Get header value
	if apikey.ValueEnv != "" {
		value = os.Getenv(apikey.ValueEnv)
	} else {
		value = apikey.Value
	}

	return header, value
}

// validateAuth validates authentication configuration
func (c *CrawlConfig) validateAuth() error {
	if c.Auth == nil {
		return nil // No auth is valid
	}

	// Check for multiple authentication types configured
	if err := c.validateSingleAuthType(); err != nil {
		return err
	}

	// Validate specific auth type configuration
	return c.validateAuthTypeConfiguration()
}

// validateSingleAuthType ensures only one auth type is configured
func (c *CrawlConfig) validateSingleAuthType() error {
	configuredAuthTypes := 0

	if c.isBasicAuthConfigured() {
		configuredAuthTypes++
	}
	if c.isBearerAuthConfigured() {
		configuredAuthTypes++
	}
	if c.isAPIKeyAuthConfigured() {
		configuredAuthTypes++
	}

	if configuredAuthTypes > 1 {
		return fmt.Errorf("multiple authentication types configured simultaneously - please use only one")
	}
	return nil
}

// isBasicAuthConfigured checks if basic auth is configured
func (c *CrawlConfig) isBasicAuthConfigured() bool {
	return c.Auth.Basic != nil && (c.Auth.Basic.Username != "" || c.Auth.Basic.Password != "" ||
		c.Auth.Basic.UsernameEnv != "" || c.Auth.Basic.PasswordEnv != "")
}

// isBearerAuthConfigured checks if bearer auth is configured
func (c *CrawlConfig) isBearerAuthConfigured() bool {
	return c.Auth.Bearer != nil && (c.Auth.Bearer.Token != "" || c.Auth.Bearer.TokenEnv != "")
}

// isAPIKeyAuthConfigured checks if API key auth is configured
func (c *CrawlConfig) isAPIKeyAuthConfigured() bool {
	return c.Auth.APIKey != nil && (c.Auth.APIKey.Header != "" || c.Auth.APIKey.Value != "" ||
		c.Auth.APIKey.HeaderEnv != "" || c.Auth.APIKey.ValueEnv != "")
}

// validateAuthTypeConfiguration validates the specific auth type configuration
func (c *CrawlConfig) validateAuthTypeConfiguration() error {
	switch c.Auth.Type {
	case NoAuth:
		return nil
	case BasicAuthType:
		return c.validateBasicAuth()
	case BearerAuthType:
		return c.validateBearerAuth()
	case APIKeyAuthType:
		return c.validateAPIKeyAuth()
	default:
		return fmt.Errorf("unsupported authentication type: %s", c.Auth.Type)
	}
}

// validateBasicAuth validates basic authentication configuration
func (c *CrawlConfig) validateBasicAuth() error {
	if c.Auth.Basic == nil {
		return fmt.Errorf("basic auth type specified but no basic auth configuration provided")
	}
	username, password := c.GetBasicAuthCredentials()
	if username == "" || password == "" {
		return fmt.Errorf("basic auth requires both username and password")
	}
	return nil
}

// validateBearerAuth validates bearer authentication configuration
func (c *CrawlConfig) validateBearerAuth() error {
	if c.Auth.Bearer == nil {
		return fmt.Errorf("bearer auth type specified but no bearer auth configuration provided")
	}
	token := c.GetBearerToken()
	if token == "" {
		return fmt.Errorf("bearer auth requires token")
	}
	return nil
}

// validateAPIKeyAuth validates API key authentication configuration
func (c *CrawlConfig) validateAPIKeyAuth() error {
	if c.Auth.APIKey == nil {
		return fmt.Errorf("api-key auth type specified but no api-key auth configuration provided")
	}
	header, value := c.GetAPIKeyCredentials()
	if header == "" || value == "" {
		return fmt.Errorf("api-key auth requires both header and value")
	}
	return nil
}

// validateHeaders validates HTTP headers format
func (c *CrawlConfig) validateHeaders() error {
	for _, header := range c.Headers {
		// Check if header has proper format "Name: Value"
		colonIndex := strings.Index(header, ":")
		if colonIndex <= 0 {
			return fmt.Errorf("invalid header format '%s': expected 'Name: Value'", header)
		}

		headerName := strings.TrimSpace(header[:colonIndex])
		headerValue := strings.TrimSpace(header[colonIndex+1:])

		if headerName == "" {
			return fmt.Errorf("invalid header format '%s': header name cannot be empty", header)
		}

		if headerValue == "" {
			return fmt.Errorf("invalid header format '%s': header value cannot be empty", header)
		}

		// Check for forbidden headers that should not be set manually
		forbiddenHeaders := []string{"host", "content-length", "connection"}
		for _, forbidden := range forbiddenHeaders {
			if strings.EqualFold(headerName, forbidden) {
				return fmt.Errorf("cannot set forbidden header '%s'", headerName)
			}
		}
	}

	return nil
}

// LoadHeadersFromEnv loads headers from environment variables with LT_HEADER_ prefix
// as specified in Issue #8: LT_HEADER_ACCEPT, LT_HEADER_X_CUSTOM, etc.
func (c *CrawlConfig) LoadHeadersFromEnv() {
	const headerPrefix = "LT_HEADER_"

	// Get all environment variables
	for _, env := range os.Environ() {
		// Parse key=value
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]

		// Check if it's a header environment variable
		if !strings.HasPrefix(key, headerPrefix) {
			continue
		}

		// Extract header name (convert LT_HEADER_ACCEPT to Accept)
		headerName := strings.TrimPrefix(key, headerPrefix)
		if headerName == "" {
			continue
		}

		// Convert X_CUSTOM to X-Custom, ACCEPT to Accept
		headerName = strings.ReplaceAll(headerName, "_", "-")
		headerName = strings.ToLower(headerName)

		// Capitalize first letter and letters after hyphens
		headerParts := strings.Split(headerName, "-")
		for i, part := range headerParts {
			if len(part) > 0 {
				headerParts[i] = strings.ToUpper(string(part[0])) + part[1:]
			}
		}
		headerName = strings.Join(headerParts, "-")

		// Add to headers list in "Name: Value" format
		headerEntry := fmt.Sprintf("%s: %s", headerName, value)

		// Check if header already exists and replace it
		found := false
		for i, existing := range c.Headers {
			if strings.HasPrefix(existing, headerName+":") {
				c.Headers[i] = headerEntry
				found = true
				break
			}
		}

		// If not found, append
		if !found {
			c.Headers = append(c.Headers, headerEntry)
		}
	}
}

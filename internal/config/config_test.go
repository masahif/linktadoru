package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Concurrency != 2 {
		t.Errorf("Expected concurrency 2, got %d", cfg.Concurrency)
	}

	if cfg.RequestDelay != 0.1 {
		t.Errorf("Expected request delay 0.1s, got %v", cfg.RequestDelay)
	}

	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("Expected request timeout 30s, got %v", cfg.RequestTimeout)
	}

	if cfg.UserAgent != "LinkTadoru/1.0" {
		t.Errorf("Expected user agent 'LinkTadoru/1.0', got %s", cfg.UserAgent)
	}

	if cfg.IgnoreRobots {
		t.Errorf("Expected ignore robots false, got %v", cfg.IgnoreRobots)
	}

	if cfg.Limit != 0 {
		t.Errorf("Expected limit 0, got %d", cfg.Limit)
	}

	if cfg.DatabasePath != "./crawl.db" {
		t.Errorf("Expected database path './crawl.db', got %s", cfg.DatabasePath)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *CrawlConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid concurrency",
			config: &CrawlConfig{
				Concurrency:    0,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout",
			config: &CrawlConfig{
				Concurrency:    10,
				RequestTimeout: 0,
				DatabasePath:   "./test.db",
			},
			wantErr: true,
		},
		{
			name: "empty database path",
			config: &CrawlConfig{
				Concurrency:    10,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "",
			},
			wantErr: true,
		},
		{
			name: "minimum delay enforcement",
			config: &CrawlConfig{
				Concurrency:    10,
				RequestDelay:   0.05, // 50ms in seconds
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check minimum delay enforcement
			if tt.name == "minimum delay enforcement" && tt.config.RequestDelay < 0.1 {
				t.Errorf("Expected minimum delay to be enforced, got %v", tt.config.RequestDelay)
			}
		})
	}
}

func TestGetBasicAuthCredentials(t *testing.T) {
	// Test case 1: No auth configuration
	cfg := DefaultConfig()
	username, password := cfg.GetBasicAuthCredentials()
	if username != "" || password != "" {
		t.Errorf("Expected empty credentials, got username='%s', password='%s'", username, password)
	}

	// Test case 2: Direct username/password
	cfg.Auth = &Auth{
		Basic: &BasicAuth{
			Username: "testuser",
			Password: "testpass",
		},
	}
	username, password = cfg.GetBasicAuthCredentials()
	if username != "testuser" || password != "testpass" {
		t.Errorf("Expected testuser/testpass, got username='%s', password='%s'", username, password)
	}

	// Test case 3: Environment variables (mock by setting values)
	t.Setenv("TEST_USERNAME", "envuser")
	t.Setenv("TEST_PASSWORD", "envpass")

	cfg.Auth = &Auth{
		Basic: &BasicAuth{
			UsernameEnv: "TEST_USERNAME",
			PasswordEnv: "TEST_PASSWORD",
		},
	}
	username, password = cfg.GetBasicAuthCredentials()
	if username != "envuser" || password != "envpass" {
		t.Errorf("Expected envuser/envpass, got username='%s', password='%s'", username, password)
	}

	// Test case 4: Environment variables take precedence
	cfg.Auth = &Auth{
		Basic: &BasicAuth{
			Username:    "directuser",
			Password:    "directpass",
			UsernameEnv: "TEST_USERNAME",
			PasswordEnv: "TEST_PASSWORD",
		},
	}
	username, password = cfg.GetBasicAuthCredentials()
	if username != "envuser" || password != "envpass" {
		t.Errorf("Expected env vars to take precedence, got username='%s', password='%s'", username, password)
	}
}

func TestGetBearerToken(t *testing.T) {
	// Test case 1: No auth configuration
	cfg := DefaultConfig()
	token := cfg.GetBearerToken()
	if token != "" {
		t.Errorf("Expected empty token, got '%s'", token)
	}

	// Test case 2: Direct token
	cfg.Auth = &Auth{
		Bearer: &BearerAuth{
			Token: "test-bearer-token-123",
		},
	}
	token = cfg.GetBearerToken()
	if token != "test-bearer-token-123" {
		t.Errorf("Expected 'test-bearer-token-123', got '%s'", token)
	}

	// Test case 3: Environment variable
	t.Setenv("TEST_BEARER_TOKEN", "env-bearer-token-456")
	cfg.Auth = &Auth{
		Bearer: &BearerAuth{
			TokenEnv: "TEST_BEARER_TOKEN",
		},
	}
	token = cfg.GetBearerToken()
	if token != "env-bearer-token-456" {
		t.Errorf("Expected 'env-bearer-token-456', got '%s'", token)
	}

	// Test case 4: Environment variable takes precedence
	cfg.Auth = &Auth{
		Bearer: &BearerAuth{
			Token:    "direct-token",
			TokenEnv: "TEST_BEARER_TOKEN",
		},
	}
	token = cfg.GetBearerToken()
	if token != "env-bearer-token-456" {
		t.Errorf("Expected environment variable to take precedence, got '%s'", token)
	}
}

func TestGetAPIKeyCredentials(t *testing.T) {
	// Test case 1: No auth configuration
	cfg := DefaultConfig()
	header, value := cfg.GetAPIKeyCredentials()
	if header != "" || value != "" {
		t.Errorf("Expected empty credentials, got header='%s', value='%s'", header, value)
	}

	// Test case 2: Direct header and value
	cfg.Auth = &Auth{
		APIKey: &APIKeyAuth{
			Header: "X-API-Key",
			Value:  "test-api-key-123",
		},
	}
	header, value = cfg.GetAPIKeyCredentials()
	if header != "X-API-Key" || value != "test-api-key-123" {
		t.Errorf("Expected 'X-API-Key'/'test-api-key-123', got header='%s', value='%s'", header, value)
	}

	// Test case 3: Environment variables
	t.Setenv("TEST_API_HEADER", "X-Custom-Key")
	t.Setenv("TEST_API_VALUE", "env-api-key-456")
	cfg.Auth = &Auth{
		APIKey: &APIKeyAuth{
			HeaderEnv: "TEST_API_HEADER",
			ValueEnv:  "TEST_API_VALUE",
		},
	}
	header, value = cfg.GetAPIKeyCredentials()
	if header != "X-Custom-Key" || value != "env-api-key-456" {
		t.Errorf("Expected 'X-Custom-Key'/'env-api-key-456', got header='%s', value='%s'", header, value)
	}

	// Test case 4: Mixed environment and direct
	cfg.Auth = &Auth{
		APIKey: &APIKeyAuth{
			Header:    "X-Direct-Header",
			Value:     "direct-value",
			HeaderEnv: "TEST_API_HEADER",
			ValueEnv:  "TEST_API_VALUE",
		},
	}
	header, value = cfg.GetAPIKeyCredentials()
	if header != "X-Custom-Key" || value != "env-api-key-456" {
		t.Errorf("Expected environment variables to take precedence, got header='%s', value='%s'", header, value)
	}
}

func TestAuthValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *CrawlConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no auth",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "valid basic auth",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: BasicAuthType,
					Basic: &BasicAuth{
						Username: "user",
						Password: "pass",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid bearer auth",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: BearerAuthType,
					Bearer: &BearerAuth{
						Token: "bearer-token-123",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid api key auth",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: APIKeyAuthType,
					APIKey: &APIKeyAuth{
						Header: "X-API-Key",
						Value:  "api-key-123",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "basic auth missing username",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: BasicAuthType,
					Basic: &BasicAuth{
						Password: "pass",
					},
				},
			},
			wantErr: true,
			errMsg:  "basic auth requires both username and password",
		},
		{
			name: "bearer auth missing token",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type:   BearerAuthType,
					Bearer: &BearerAuth{},
				},
			},
			wantErr: true,
			errMsg:  "bearer auth requires token",
		},
		{
			name: "api key auth missing header",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: APIKeyAuthType,
					APIKey: &APIKeyAuth{
						Value: "api-key-123",
					},
				},
			},
			wantErr: true,
			errMsg:  "api-key auth requires both header and value",
		},
		{
			name: "unsupported auth type",
			config: &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Auth: &Auth{
					Type: AuthType("unsupported"),
				},
			},
			wantErr: true,
			errMsg:  "unsupported authentication type: unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("Validate() error = %v, want error containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no headers",
			headers: []string{},
			wantErr: false,
		},
		{
			name:    "valid headers",
			headers: []string{"Accept: application/json", "X-Custom: value"},
			wantErr: false,
		},
		{
			name:    "invalid header format - no colon",
			headers: []string{"InvalidHeader"},
			wantErr: true,
			errMsg:  "invalid header format 'InvalidHeader': expected 'Name: Value'",
		},
		{
			name:    "invalid header format - colon at start",
			headers: []string{":Value"},
			wantErr: true,
			errMsg:  "invalid header format ':Value': expected 'Name: Value'",
		},
		{
			name:    "empty header name",
			headers: []string{" : Value"},
			wantErr: true,
			errMsg:  "invalid header format ' : Value': header name cannot be empty",
		},
		{
			name:    "empty header value",
			headers: []string{"Name: "},
			wantErr: true,
			errMsg:  "invalid header format 'Name: ': header value cannot be empty",
		},
		{
			name:    "forbidden header - host",
			headers: []string{"Host: example.com"},
			wantErr: true,
			errMsg:  "cannot set forbidden header 'Host'",
		},
		{
			name:    "forbidden header - content-length",
			headers: []string{"Content-Length: 100"},
			wantErr: true,
			errMsg:  "cannot set forbidden header 'Content-Length'",
		},
		{
			name:    "forbidden header - connection",
			headers: []string{"Connection: keep-alive"},
			wantErr: true,
			errMsg:  "cannot set forbidden header 'Connection'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &CrawlConfig{
				Concurrency:    2,
				RequestDelay:   0.1,
				RequestTimeout: 30 * time.Second,
				DatabasePath:   "./test.db",
				Headers:        tt.headers,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("Validate() error = %v, want error containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestLoadHeadersFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected []string
	}{
		{
			name:     "no header env vars",
			envVars:  map[string]string{},
			expected: []string{},
		},
		{
			name: "single header",
			envVars: map[string]string{
				"LT_HEADER_ACCEPT": "application/json",
			},
			expected: []string{"Accept: application/json"},
		},
		{
			name: "multiple headers",
			envVars: map[string]string{
				"LT_HEADER_ACCEPT":     "application/json",
				"LT_HEADER_X_CUSTOM":   "custom-value",
				"LT_HEADER_USER_AGENT": "TestBot/1.0",
			},
			expected: []string{
				"Accept: application/json",
				"X-Custom: custom-value",
				"User-Agent: TestBot/1.0",
			},
		},
		{
			name: "header with underscores conversion",
			envVars: map[string]string{
				"LT_HEADER_X_API_KEY": "secret123",
			},
			expected: []string{"X-Api-Key: secret123"},
		},
		{
			name: "non-header env vars ignored",
			envVars: map[string]string{
				"LT_CONCURRENCY":    "5",
				"LT_HEADER_ACCEPT":  "text/html",
				"SOME_OTHER_HEADER": "ignored",
			},
			expected: []string{"Accept: text/html"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear existing environment
			for _, env := range os.Environ() {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 && strings.HasPrefix(parts[0], "LT_HEADER_") {
					t.Setenv(parts[0], "")
				}
			}

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			cfg := DefaultConfig()
			cfg.LoadHeadersFromEnv()

			// Check if we got the expected headers
			if len(cfg.Headers) != len(tt.expected) {
				t.Errorf("Expected %d headers, got %d. Headers: %v",
					len(tt.expected), len(cfg.Headers), cfg.Headers)
				return
			}

			// Create maps for easier comparison (order doesn't matter)
			expectedMap := make(map[string]bool)
			for _, header := range tt.expected {
				expectedMap[header] = true
			}

			actualMap := make(map[string]bool)
			for _, header := range cfg.Headers {
				actualMap[header] = true
			}

			for expected := range expectedMap {
				if !actualMap[expected] {
					t.Errorf("Expected header '%s' not found in actual headers: %v", expected, cfg.Headers)
				}
			}

			for actual := range actualMap {
				if !expectedMap[actual] {
					t.Errorf("Unexpected header '%s' found in actual headers", actual)
				}
			}
		})
	}
}

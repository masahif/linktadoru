package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	
	if cfg.Concurrency != 10 {
		t.Errorf("Expected concurrency 10, got %d", cfg.Concurrency)
	}
	
	if cfg.RequestDelay != 1*time.Second {
		t.Errorf("Expected request delay 1s, got %v", cfg.RequestDelay)
	}
	
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("Expected request timeout 30s, got %v", cfg.RequestTimeout)
	}
	
	if cfg.UserAgent != "LinkTadoru/1.0" {
		t.Errorf("Expected user agent 'LinkTadoru/1.0', got %s", cfg.UserAgent)
	}
	
	if !cfg.RespectRobots {
		t.Errorf("Expected respect robots true, got %v", cfg.RespectRobots)
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
				RequestDelay:   50 * time.Millisecond,
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
			if tt.name == "minimum delay enforcement" && tt.config.RequestDelay < 100*time.Millisecond {
				t.Errorf("Expected minimum delay to be enforced, got %v", tt.config.RequestDelay)
			}
		})
	}
}
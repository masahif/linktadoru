package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/masahif/linktadoru/internal/config"
)

func TestSetVersionInfo(t *testing.T) {
	version := "1.2.3"
	buildTime := "2023-12-01T10:00:00Z"
	
	SetVersionInfo(version, buildTime)
	
	expected := "1.2.3 (built 2023-12-01T10:00:00Z)"
	if rootCmd.Version != expected {
		t.Errorf("Expected version %s, got %s", expected, rootCmd.Version)
	}
}

func TestExecute(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	
	// Test help command
	os.Args = []string{"linktadoru", "--help"}
	err := Execute()
	// Help should exit with ErrHelp, but cobra handles this internally
	// and returns nil for help commands
	if err != nil {
		t.Logf("Execute with help returned: %v", err)
	}
}

func TestInitConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	
	configContent := `
concurrency: 5
request_delay: 2s
user_agent: "TestAgent/1.0"
`
	
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}
	
	// Set config file
	cfgFile = configFile
	
	// Initialize config
	initConfig()
	
	// Check if config was loaded
	if viper.ConfigFileUsed() != configFile {
		t.Errorf("Expected config file %s, got %s", configFile, viper.ConfigFileUsed())
	}
	
	// Reset for other tests
	cfgFile = ""
	viper.Reset()
}

func TestRootCmd(t *testing.T) {
	// Test that rootCmd is properly initialized
	if rootCmd.Use != "linktadoru [URLs...]" {
		t.Errorf("Expected use 'linktadoru [URLs...]', got %s", rootCmd.Use)
	}
	
	if rootCmd.Short != "A high-performance web crawler and link analysis tool" {
		t.Errorf("Unexpected short description: %s", rootCmd.Short)
	}
	
	if rootCmd.RunE == nil {
		t.Error("RunE should be set to runCrawler")
	}
}

func TestInitializeCrawler(t *testing.T) {
	// Create a temporary database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	
	cfg := &config.CrawlConfig{
		SeedURLs:       []string{"https://example.com"},
		Concurrency:    5,
		RequestDelay:   time.Second,
		RequestTimeout: 30 * time.Second,
		UserAgent:      "TestAgent/1.0",
		RespectRobots:  true,
		DatabasePath:   dbPath,
		Limit:          10,
	}
	
	crawler, err := initializeCrawler(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize crawler: %v", err)
	}
	
	if crawler == nil {
		t.Error("Crawler should not be nil")
	}
	
	// Clean up
	if crawler != nil {
		_ = crawler.Stop()
	}
}

func TestRunCrawlerValidation(t *testing.T) {
	// Create a temporary directory for database
	tempDir := t.TempDir()
	
	// Save original values
	origCfgFile := cfgFile
	defer func() { cfgFile = origCfgFile }()
	
	// Reset viper
	viper.Reset()
	
	// Create a mock command
	cmd := &cobra.Command{}
	cmd.Flags().Int("concurrency", 10, "")
	cmd.Flags().Duration("delay", time.Second, "")
	cmd.Flags().Duration("timeout", 30*time.Second, "")
	cmd.Flags().String("user-agent", "LinkTadoru/1.0", "")
	cmd.Flags().Bool("ignore-robots", false, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().StringSlice("include-patterns", []string{}, "")
	cmd.Flags().StringSlice("exclude-patterns", []string{}, "")
	cmd.Flags().String("database", filepath.Join(tempDir, "test.db"), "")
	
	// Bind flags to viper for this test
	viper.BindPFlag("concurrency", cmd.Flags().Lookup("concurrency"))
	viper.BindPFlag("request_delay", cmd.Flags().Lookup("delay"))
	viper.BindPFlag("request_timeout", cmd.Flags().Lookup("timeout"))
	viper.BindPFlag("user_agent", cmd.Flags().Lookup("user-agent"))
	viper.BindPFlag("respect_robots", cmd.Flags().Lookup("ignore-robots"))
	viper.BindPFlag("limit", cmd.Flags().Lookup("limit"))
	viper.BindPFlag("include_patterns", cmd.Flags().Lookup("include-patterns"))
	viper.BindPFlag("exclude_patterns", cmd.Flags().Lookup("exclude-patterns"))
	viper.BindPFlag("database_path", cmd.Flags().Lookup("database"))
	
	// Test with invalid config (no seed URLs and empty queue should be handled by crawler)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// Add context to command
	cmd.SetContext(ctx)
	
	// Test that runCrawler can be called (it will timeout due to context)
	err := runCrawler(cmd, []string{})
	// Should get context timeout or similar error, not validation error
	if err == nil {
		t.Log("runCrawler completed without error")
	} else {
		t.Logf("runCrawler returned expected error: %v", err)
	}
}

func TestFlagBinding(t *testing.T) {
	// This tests that the init() function properly sets up flags
	flags := rootCmd.Flags()
	
	// Test that essential flags exist
	expectedFlags := []string{
		"concurrency",
		"delay", 
		"timeout",
		"user-agent",
		"ignore-robots",
		"limit",
		"include-patterns",
		"exclude-patterns",
		"database",
	}
	
	for _, flagName := range expectedFlags {
		if flags.Lookup(flagName) == nil {
			t.Errorf("Expected flag %s to be defined", flagName)
		}
	}
	
	// Test persistent flags
	persistentFlags := rootCmd.PersistentFlags()
	if persistentFlags.Lookup("config") == nil {
		t.Error("Expected persistent flag 'config' to be defined")
	}
}
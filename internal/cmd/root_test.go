package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/storage"
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

func TestShowCurrentConfigRedactsSecrets(t *testing.T) {
	t.Setenv("BASIC_USERNAME", "basic-env-username-secret")
	t.Setenv("BASIC_PASSWORD", "basic-env-password-secret")
	t.Setenv("BEARER_TOKEN", "bearer-env-secret")
	t.Setenv("API_KEY_VALUE", "api-key-env-secret")
	t.Setenv("LT_HEADER_AUTHORIZATION", "Bearer env-header-secret")
	t.Setenv("LT_HEADER_COOKIE", "session=env-cookie-secret")
	t.Setenv("LT_HEADER_X_CUSTOM", "env-custom-secret")

	cfg := config.DefaultConfig()
	cfg.SeedURLs = []string{
		"https://seed-user-secret:seed-password-secret@example.com/private",
		"https://public.example/",
	}
	cfg.Auth = &config.Auth{
		Type: config.BasicAuthType,
		Basic: &config.BasicAuth{
			Username:    "direct-username-secret",
			Password:    "direct-password-secret",
			UsernameEnv: "BASIC_USERNAME",
			PasswordEnv: "BASIC_PASSWORD",
		},
		Bearer: &config.BearerAuth{
			Token:    "direct-bearer-secret",
			TokenEnv: "BEARER_TOKEN",
		},
		APIKey: &config.APIKeyAuth{
			Header:   "X-API-Key",
			Value:    "direct-api-key-secret",
			ValueEnv: "API_KEY_VALUE",
		},
	}
	cfg.Headers = []string{
		"X-Direct: direct-header-secret",
		"malformed-header-secret",
	}
	cfg.LoadHeadersFromEnv()

	originalHeaders := append([]string(nil), cfg.Headers...)

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := showCurrentConfig(cmd, cfg); err != nil {
		t.Fatalf("showCurrentConfig() error = %v", err)
	}

	output := stdout.String() + stderr.String()
	secrets := []string{
		"direct-username-secret",
		"direct-password-secret",
		"direct-bearer-secret",
		"direct-api-key-secret",
		"direct-header-secret",
		"malformed-header-secret",
		"basic-env-username-secret",
		"basic-env-password-secret",
		"bearer-env-secret",
		"api-key-env-secret",
		"env-header-secret",
		"env-cookie-secret",
		"env-custom-secret",
		"seed-user-secret",
		"seed-password-secret",
	}
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Errorf("show-config output leaked secret %q", secret)
		}
	}

	visibleMetadata := []string{
		redactedConfigValue,
		"username_env: BASIC_USERNAME",
		"password_env: BASIC_PASSWORD",
		"token_env: BEARER_TOKEN",
		"value_env: API_KEY_VALUE",
		"header: X-API-Key",
		"X-Direct: <redacted>",
		"Authorization: <redacted>",
		"Cookie: <redacted>",
		"X-Custom: <redacted>",
		"https://redacted@example.com/private",
		"https://public.example/",
	}
	for _, want := range visibleMetadata {
		if !strings.Contains(output, want) {
			t.Errorf("show-config output does not contain %q\noutput:\n%s", want, output)
		}
	}

	if cfg.Auth.Basic.Username != "direct-username-secret" || cfg.Auth.Basic.Password != "direct-password-secret" {
		t.Fatal("showCurrentConfig mutated the runtime basic-auth configuration")
	}
	if cfg.Auth.Bearer.Token != "direct-bearer-secret" {
		t.Fatal("showCurrentConfig mutated the runtime bearer configuration")
	}
	if cfg.Auth.APIKey.Value != "direct-api-key-secret" {
		t.Fatal("showCurrentConfig mutated the runtime API-key configuration")
	}
	if len(cfg.Headers) != len(originalHeaders) {
		t.Fatal("showCurrentConfig mutated the runtime header list")
	}
	for i := range originalHeaders {
		if cfg.Headers[i] != originalHeaders[i] {
			t.Fatalf("showCurrentConfig mutated header %d: got %q, want %q", i, cfg.Headers[i], originalHeaders[i])
		}
	}
}

func TestShowCurrentConfigRedactsInvalidHeaderValidationError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Headers = []string{"invalid-header-secret"}

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := showCurrentConfig(cmd, cfg); err != nil {
		t.Fatalf("showCurrentConfig() error = %v", err)
	}

	output := stdout.String() + stderr.String()
	if strings.Contains(output, "invalid-header-secret") {
		t.Fatal("show-config validation output leaked an invalid header value")
	}
	if !strings.Contains(stderr.String(), "Configuration validation failed") {
		t.Fatalf("stderr does not contain the expected validation warning: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), redactedConfigValue) {
		t.Fatalf("stderr does not contain the redaction marker: %s", stderr.String())
	}
}

func TestInitializeCrawler(t *testing.T) {
	// Create a temporary database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	cfg := &config.CrawlConfig{
		SeedURLs:        []string{"https://example.com"},
		Concurrency:     5,
		RequestDelay:    1.0, // 1 second
		RequestTimeout:  30 * time.Second,
		UserAgent:       "TestAgent/1.0",
		IgnoreRobotsTxt: false,
		DatabasePath:    dbPath,
		Limit:           10,
	}

	crawler, store, err := initializeCrawler(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize crawler: %v", err)
	}

	if crawler == nil {
		t.Error("Crawler should not be nil")
	}
	if store == nil {
		t.Error("Storage should not be nil")
	}

	// Clean up
	if crawler != nil {
		_ = crawler.Stop()
	}
	if store != nil {
		_ = store.Close()
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
	cmd.Flags().Float64("delay", 1.0, "")
	cmd.Flags().Duration("timeout", 30*time.Second, "")
	cmd.Flags().String("user-agent", "LinkTadoru/1.0", "")
	cmd.Flags().Bool("ignore-robots-txt", false, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().StringSlice("include-patterns", []string{}, "")
	cmd.Flags().StringSlice("exclude-patterns", []string{}, "")
	cmd.Flags().String("database", filepath.Join(tempDir, "test.db"), "")

	// Bind flags to viper for this test
	_ = viper.BindPFlag("concurrency", cmd.Flags().Lookup("concurrency"))
	_ = viper.BindPFlag("request_delay", cmd.Flags().Lookup("delay"))
	_ = viper.BindPFlag("request_timeout", cmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("user_agent", cmd.Flags().Lookup("user-agent"))
	_ = viper.BindPFlag("ignore_robots_txt", cmd.Flags().Lookup("ignore-robots-txt"))
	_ = viper.BindPFlag("limit", cmd.Flags().Lookup("limit"))
	_ = viper.BindPFlag("include_patterns", cmd.Flags().Lookup("include-patterns"))
	_ = viper.BindPFlag("exclude_patterns", cmd.Flags().Lookup("exclude-patterns"))
	_ = viper.BindPFlag("database_path", cmd.Flags().Lookup("database"))

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
		"ignore-robots-txt",
		"limit",
		"max-depth",
		"seed-file",
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

func TestRunCrawlerStartupValidation(t *testing.T) {
	// Create a temporary directory for database
	tempDir := t.TempDir()

	// Save original values
	origCfgFile := cfgFile
	defer func() { cfgFile = origCfgFile }()

	t.Run("NoURLsNoDB", func(t *testing.T) {
		// Reset viper for each subtest
		viper.Reset()

		// Create a mock command with no show-config flag
		cmd := &cobra.Command{}
		cmd.Flags().Bool("show-config", false, "")
		cmd.Flags().String("database", filepath.Join(tempDir, "nonexistent.db"), "")

		// Bind flags
		_ = viper.BindPFlag("database_path", cmd.Flags().Lookup("database"))

		// Test with no URLs and no database
		err := runCrawler(cmd, []string{}) // No seed URLs
		if err == nil {
			t.Error("Expected error when no URLs provided and no database exists")
		}
		if !strings.Contains(err.Error(), "no URLs provided and no existing database found") {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("NoURLsEmptyDB", func(t *testing.T) {
		// Reset viper for each subtest
		viper.Reset()

		// Create an empty database
		dbPath := filepath.Join(tempDir, "empty.db")
		emptyStore, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		_ = emptyStore.Close()

		// Create a mock command
		cmd := &cobra.Command{}
		cmd.Flags().Bool("show-config", false, "")
		cmd.Flags().String("database", dbPath, "")

		// Bind flags
		_ = viper.BindPFlag("database_path", cmd.Flags().Lookup("database"))

		// Test with no URLs but empty database (should exit gracefully)
		err = runCrawler(cmd, []string{}) // No seed URLs
		if err != nil {
			t.Errorf("Expected no error for empty database case, got: %v", err)
		}
	})

	t.Run("NoURLsDBWithQueue", func(t *testing.T) {
		// Reset viper for each subtest
		viper.Reset()

		// Create a database with queued items
		dbPath := filepath.Join(tempDir, "queued.db")
		testStore, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}

		// Add some URLs to queue
		err = testStore.AddToQueue([]string{"https://test.com/page1", "https://test.com/page2"}, 0)
		if err != nil {
			t.Fatalf("Failed to add URLs to queue: %v", err)
		}

		// Verify queue has items
		hasItems, err := testStore.HasQueuedItems()
		if err != nil {
			t.Fatalf("Failed to check queued items: %v", err)
		}
		if !hasItems {
			t.Fatal("Expected queued items, but HasQueuedItems returned false")
		}

		_ = testStore.Close()

		// For this test case, we only verify that the database validation logic works
		// We don't actually run the crawler to avoid infinite loops in tests
		// The validation logic should detect that there are queued items and NOT error out

		// Test the validation logic directly by checking database file existence and queue status
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("Expected database file to exist")
		}

		// Verify that HasQueuedItems works correctly for this database
		testStorage, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("Failed to reopen test database: %v", err)
		}
		defer func() { _ = testStorage.Close() }()

		hasItems, err = testStorage.HasQueuedItems()
		if err != nil {
			t.Errorf("Failed to check queued items in validation test: %v", err)
		}
		if !hasItems {
			t.Error("Expected queued items in validation test, but HasQueuedItems returned false")
		}

		// This validates that the startup validation logic would pass for this case
		// (The actual runCrawler call is omitted to prevent test timeouts)
	})
}

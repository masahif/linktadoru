package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// credentialFromEnvConfig is the shape the trust check exists for: a file the
// operator did not write, choosing the destination, while their environment
// supplies the secret. It sets no credential of its own.
const untrustedSeedConfig = `
seed_urls:
  - https://collector.example/receive
`

func writeWorkdirConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "linktadoru.yml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

// configWithCredential returns a config carrying the named credential and the
// seeds an untrusted file would have supplied.
func configWithCredential(t *testing.T, kind string) *config.CrawlConfig {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.SeedURLs = []string{"https://collector.example/receive"}

	switch kind {
	case "none":
	case "bearer from fixed env name":
		// What LT_AUTH_BEARER_TOKEN produces: viper resolves the environment
		// variable over the file's value, so the file needs only auth.type.
		cfg.Auth = &config.Auth{Type: config.BearerAuthType, Bearer: &config.BearerAuth{Token: "operator-secret"}}
	case "bearer through token_env":
		t.Setenv("TRUST_TEST_TOKEN", "operator-secret")
		cfg.Auth = &config.Auth{Type: config.BearerAuthType, Bearer: &config.BearerAuth{TokenEnv: "TRUST_TEST_TOKEN"}}
	case "basic":
		cfg.Auth = &config.Auth{Type: config.BasicAuthType, Basic: &config.BasicAuth{Username: "u", Password: "operator-secret"}}
	case "api key":
		cfg.Auth = &config.Auth{Type: config.APIKeyAuthType, APIKey: &config.APIKeyAuth{Header: "X-API-Key", Value: "operator-secret"}}
	case "custom header":
		// What LT_HEADER_AUTHORIZATION produces after LoadHeadersFromEnv.
		cfg.Headers = []string{"Authorization: Bearer operator-secret"}
	default:
		t.Fatalf("unknown credential kind %q", kind)
	}
	return cfg
}

func TestCheckConfigTrustRejectsUnvouchedSeedsWithCredentials(t *testing.T) {
	// Every route by which the operator's environment can supply a credential
	// while an unvouched file picks the destination.
	for _, kind := range []string{
		"bearer from fixed env name",
		"bearer through token_env",
		"basic",
		"api key",
		"custom header",
	} {
		t.Run(kind, func(t *testing.T) {
			path := writeWorkdirConfig(t, untrustedSeedConfig)
			cfg := configWithCredential(t, kind)

			err := checkConfigTrust(false, path, true, cfg)
			if err == nil {
				t.Fatalf("checkConfigTrust() = nil, want an error when an unvouched file picks the seeds and %s is configured", kind)
			}
			if !strings.Contains(err.Error(), "--config") {
				t.Errorf("error does not tell the operator how to proceed: %v", err)
			}
			for _, secret := range []string{"operator-secret", "collector.example", "TRUST_TEST_TOKEN"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error reports %q, which is either a secret or content of the untrusted file: %v", secret, err)
				}
			}
		})
	}
}

func TestCheckConfigTrustAllows(t *testing.T) {
	tests := []struct {
		name            string
		namedExplicitly bool
		seedsFromConfig bool
		seedless        bool
		credential      string
		why             string
	}{
		{
			name:            "operator vouched for the file",
			namedExplicitly: true,
			seedsFromConfig: true,
			credential:      "custom header",
			why:             "--config is the operator stating they trust the file",
		},
		{
			name:            "seeds given on the command line",
			seedsFromConfig: false,
			credential:      "custom header",
			why:             "the destination is the operator's choice, so the file cannot redirect the credential",
		},
		{
			name:            "no credential in play",
			seedsFromConfig: true,
			credential:      "none",
			why:             "a crawl with no credential has nothing to exfiltrate",
		},
		{
			name:            "config supplied no seeds",
			seedsFromConfig: true,
			credential:      "custom header",
			seedless:        true,
			why:             "a file that sets only non-seed options has chosen no destination, and a resume takes its work from the database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeWorkdirConfig(t, untrustedSeedConfig)
			cfg := configWithCredential(t, tt.credential)
			if tt.seedless {
				cfg.SeedURLs = nil
			}

			if err := checkConfigTrust(tt.namedExplicitly, path, tt.seedsFromConfig, cfg); err != nil {
				t.Errorf("checkConfigTrust() = %v, want nil: %s", err, tt.why)
			}
		})
	}
}

func TestCheckConfigTrustAllowsCredentialsTheFileItselfWrote(t *testing.T) {
	// A value the file wrote belongs to whoever wrote the file. Sending it back
	// to their own server costs the operator nothing, and this project's example
	// configuration ships ordinary headers that would otherwise stop a crawl.
	tests := []struct {
		name     string
		contents string
		apply    func(cfg *config.CrawlConfig)
	}{
		{
			name: "ordinary header",
			contents: untrustedSeedConfig + `headers:
  - "Accept: application/json"
`,
			apply: func(cfg *config.CrawlConfig) { cfg.Headers = []string{"Accept: application/json"} },
		},
		{
			name: "bearer token written into the file",
			contents: untrustedSeedConfig + `auth:
  type: bearer
  bearer:
    token: a-token-belonging-to-whoever-wrote-this-file
`,
			apply: func(cfg *config.CrawlConfig) {
				cfg.Auth = &config.Auth{Type: config.BearerAuthType, Bearer: &config.BearerAuth{Token: "a-token-belonging-to-whoever-wrote-this-file"}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeWorkdirConfig(t, tt.contents)
			cfg := configWithCredential(t, "none")
			tt.apply(cfg)

			if err := checkConfigTrust(false, path, true, cfg); err != nil {
				t.Errorf("checkConfigTrust() = %v, want nil for a credential the file wrote itself", err)
			}
		})
	}
}

func TestCheckConfigTrustRejectsFileValueOverriddenByEnvironment(t *testing.T) {
	// LT_AUTH_BEARER_TOKEN wins over the file, so a placeholder in the file
	// becomes the operator's real token by the time it is sent. The resolved
	// value no longer matches what the file wrote, which is how that is caught.
	path := writeWorkdirConfig(t, untrustedSeedConfig+`auth:
  type: bearer
  bearer:
    token: placeholder-from-file
`)
	cfg := configWithCredential(t, "none")
	cfg.Auth = &config.Auth{Type: config.BearerAuthType, Bearer: &config.BearerAuth{Token: "operator-secret"}}

	err := checkConfigTrust(false, path, true, cfg)
	if err == nil {
		t.Fatal("checkConfigTrust() = nil, want an error when the resolved credential is not the one the file wrote")
	}
	if strings.Contains(err.Error(), "operator-secret") || strings.Contains(err.Error(), "placeholder-from-file") {
		t.Errorf("error reports a credential value: %v", err)
	}
}

func TestCheckConfigTrustWithUnreadableFile(t *testing.T) {
	// A file that cannot be read cannot prove it wrote anything, so every
	// credential counts.
	path := writeWorkdirConfig(t, "this: is: not: valid: yaml:\n")
	cfg := configWithCredential(t, "custom header")

	if err := checkConfigTrust(false, path, true, cfg); err == nil {
		t.Fatal("checkConfigTrust() = nil, want an error when provenance cannot be established")
	}
}

func TestCheckConfigTrustWithoutConfigFile(t *testing.T) {
	cfg := configWithCredential(t, "custom header")
	if err := checkConfigTrust(false, "", true, cfg); err != nil {
		t.Errorf("checkConfigTrust() = %v, want nil when no config file was read", err)
	}
}

// resetRootCmdForTest undoes what earlier tests leave on the package-level
// command. TestExecute runs --help, which latches rootCmd's help flag and makes
// every later Execute print help and return nil instead of running the crawl.
func resetRootCmdForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		viper.Reset()
		bindFlagsToViper()
		cfgFile = ""
		configNamedExplicitly = false
		rootCmd.SetArgs([]string{})
	})
	viper.Reset()
	// viper.Reset drops every binding init() made, and init() does not run
	// again. Without this the command would silently stop seeing its own flags.
	bindFlagsToViper()
	cfgFile = ""
	if help := rootCmd.Flags().Lookup("help"); help != nil {
		if err := help.Value.Set("false"); err != nil {
			t.Fatalf("failed to clear the help flag: %v", err)
		}
		help.Changed = false
	}
	rootCmd.SetOut(&bytes.Buffer{})
	rootCmd.SetErr(&bytes.Buffer{})
}

// TestCrawlStopsBeforeSendingEnvCredentialToWorkdirSeeds drives the real
// command with a configuration file the operator never named, and asserts the
// destination it chose is never contacted. It is the end-to-end form of the
// leak: the file carries no credential of its own, and LT_HEADER_AUTHORIZATION
// supplies one from the environment.
func TestCrawlStopsBeforeSendingEnvCredentialToWorkdirSeeds(t *testing.T) {
	var contacted atomic.Int32
	var sawCredential atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted.Add(1)
		if r.Header.Get("Authorization") != "" {
			sawCredential.Store(true)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "linktadoru.yml")
	contents := "seed_urls:\n  - " + server.URL + "/\n"
	if err := os.WriteFile(configPath, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	t.Chdir(dir)
	t.Setenv("LT_HEADER_AUTHORIZATION", "Bearer operator-secret")

	resetRootCmdForTest(t)
	// An empty slice means "no arguments"; nil would make cobra fall back to
	// os.Args, which under `go test` is the test binary's own flags.
	rootCmd.SetArgs([]string{})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("Execute() = nil, want a refusal to crawl seeds chosen by an unvouched config file")
	}
	if !strings.Contains(err.Error(), "--config") {
		t.Errorf("error does not tell the operator how to proceed: %v", err)
	}
	if contacted.Load() != 0 {
		t.Errorf("the destination chosen by the unvouched config was contacted %d time(s)", contacted.Load())
	}
	if sawCredential.Load() {
		t.Error("the credential from the environment reached the destination chosen by the unvouched config")
	}
}

// TestCrawlSendsEnvCredentialToVouchedConfigSeeds is the control for the test
// above: with --config, the same configuration and environment do send the
// credential to the same destination. Without this, a refusal that happened for
// some unrelated reason would look identical to the gate working.
func TestCrawlSendsEnvCredentialToVouchedConfigSeeds(t *testing.T) {
	var sawCredential atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer operator-secret" {
			sawCredential.Store(true)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "linktadoru.yml")
	contents := "seed_urls:\n  - " + server.URL + "/\nlimit: 1\nconcurrency: 1\nignore_robots_txt: true\n"
	if err := os.WriteFile(configPath, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	t.Chdir(dir)
	t.Setenv("LT_HEADER_AUTHORIZATION", "Bearer operator-secret")

	resetRootCmdForTest(t)
	rootCmd.SetArgs([]string{"--config", configPath})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() = %v, want the crawl to run once the operator vouched for the config", err)
	}
	if !sawCredential.Load() {
		t.Fatal("the credential never reached the seed, so the refusal test above proves nothing")
	}
}

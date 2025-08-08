// Package cmd provides the command-line interface for LinkTadoru.
// It handles command parsing, configuration loading, and crawler execution.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"
)

var (
	cfgFile   string
	version   string
	buildTime string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "linktadoru [URLs...]",
	Short: "A high-performance web crawler and link analysis tool",
	Long: `LinkTadoru is a high-performance web crawler and link analysis tool.
	
It discovers and analyzes website structures, extracts metadata,
and maps link relationships for comprehensive site analysis.`,
	Args: cobra.ArbitraryArgs,
	RunE: runCrawler,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersionInfo sets version information for the CLI
func SetVersionInfo(v, bt string) {
	version = v
	buildTime = bt
	rootCmd.Version = fmt.Sprintf("%s (built %s)", version, buildTime)
}

func init() {
	cobra.OnInitialize(initConfig)

	// Configuration file flag
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./linktadoru.yml)")

	// Configuration management flags
	rootCmd.Flags().Bool("show-config", false, "Display current configuration in YAML format and exit")

	// Basic crawling flags (updated defaults)
	rootCmd.Flags().IntP("concurrency", "c", 2, "Number of concurrent workers")
	rootCmd.Flags().Float64P("delay", "r", 0.1, "Delay between requests in seconds")
	rootCmd.Flags().DurationP("timeout", "t", 30*time.Second, "HTTP request timeout")
	rootCmd.Flags().StringP("user-agent", "u", "LinkTadoru/1.0", "HTTP User-Agent header")
	rootCmd.Flags().Bool("ignore-robots", false, "Ignore robots.txt rules")
	rootCmd.Flags().Bool("follow-external-hosts", false, "Allow crawling external hosts")
	rootCmd.Flags().IntP("limit", "l", 0, "Stop after N pages (0=unlimited)")

	// Authentication type flag
	rootCmd.Flags().String("auth-type", "", "Authentication type: 'basic', 'bearer', or 'api-key'")

	// Basic authentication flags
	rootCmd.Flags().String("auth-username", "", "Username for basic authentication")
	rootCmd.Flags().String("auth-password", "", "Password for basic authentication")

	// Bearer authentication flags
	rootCmd.Flags().String("auth-token", "", "Bearer token for authorization header")

	// API Key authentication flags
	rootCmd.Flags().String("auth-header", "", "API key header name (e.g., X-API-Key)")
	rootCmd.Flags().String("auth-value", "", "API key header value")

	// HTTP Headers flags
	rootCmd.Flags().StringSliceP("header", "H", []string{}, "Custom HTTP headers in 'Name: Value' format (use multiple times for multiple headers)")

	// URL filtering flags
	rootCmd.Flags().StringSlice("include-patterns", []string{}, "Regex patterns for URLs to include")
	rootCmd.Flags().StringSlice("exclude-patterns", []string{}, "Regex patterns for URLs to exclude")

	// Database flags
	rootCmd.Flags().StringP("database", "d", "./linktadoru.db", "Path to SQLite database file")

	// Bind basic flags to viper
	bindFlags := []struct {
		viperKey string
		flagName string
	}{
		{"concurrency", "concurrency"},
		{"request_delay", "delay"},
		{"request_timeout", "timeout"},
		{"user_agent", "user-agent"},
		{"ignore_robots", "ignore-robots"},
		{"follow_external_hosts", "follow-external-hosts"},
		{"limit", "limit"},
		{"include_patterns", "include-patterns"},
		{"exclude_patterns", "exclude-patterns"},
		{"database_path", "database"},
		{"headers", "header"},
		{"auth.type", "auth-type"},
		{"auth.basic.username", "auth-username"},
		{"auth.basic.password", "auth-password"},
		{"auth.bearer.token", "auth-token"},
		{"auth.apikey.header", "auth-header"},
		{"auth.apikey.value", "auth-value"},
	}

	for _, bind := range bindFlags {
		if err := viper.BindPFlag(bind.viperKey, rootCmd.Flags().Lookup(bind.flagName)); err != nil {
			// Log the error but continue - non-critical for operation
			fmt.Fprintf(os.Stderr, "Warning: failed to bind flag %s: %v\n", bind.flagName, err)
		}
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("linktadoru") // Changed from "config" to "linktadoru"
	}

	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix("LT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func generateUserAgent() string {
	if version != "" && version != "dev" {
		return fmt.Sprintf("LinkTadoru/%s", version)
	}
	return "LinkTadoru/dev"
}

func showCurrentConfig(cfg *config.CrawlConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate configuration before showing it
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Configuration validation failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Displaying configuration anyway...\n\n")
	}

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration to YAML: %w", err)
	}

	// Add header comment to the output
	fmt.Printf("# Current LinkTadoru Configuration\n")
	fmt.Printf("# Generated at: %s\n", time.Now().Format(time.RFC3339))
	fmt.Printf("# Configuration file search paths: ./linktadoru.yml\n")
	fmt.Printf("# Environment variables prefix: LT_\n\n")

	fmt.Print(string(yamlData))

	// Add footer with additional information
	fmt.Printf("\n# Configuration source priority:\n")
	fmt.Printf("# 1. Command-line arguments (highest priority)\n")
	fmt.Printf("# 2. Environment variables (LT_ prefix)\n")
	fmt.Printf("# 3. Configuration file (linktadoru.yml)\n")
	fmt.Printf("# 4. Default values (lowest priority)\n")

	return nil
}

func runCrawler(cmd *cobra.Command, args []string) error {
	// Load configuration
	// Handle --show-config flag first
	showConfig, _ := cmd.Flags().GetBool("show-config")

	cfg := config.DefaultConfig()

	// Set seed URLs from command line arguments
	cfg.SeedURLs = args

	// Override with viper values
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Load headers from environment variables (Issue #8 specification)
	cfg.LoadHeadersFromEnv()

	// Update User-Agent with dynamic version if not explicitly set
	if !cmd.Flags().Changed("user-agent") && cfg.UserAgent == "LinkTadoru/1.0" {
		cfg.UserAgent = generateUserAgent()
	}

	// Handle --show-config: display current configuration and exit
	if showConfig {
		return showCurrentConfig(cfg)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Validate startup conditions: prevent running without URLs and without existing database
	if len(cfg.SeedURLs) == 0 {
		// No seed URLs provided, check if database exists for resume
		if _, err := os.Stat(cfg.DatabasePath); os.IsNotExist(err) {
			return fmt.Errorf("no URLs provided and no existing database found at %s\nUsage: %s [URLs...] or ensure database exists for resume operation",
				cfg.DatabasePath, os.Args[0])
		}

		// Database exists, but let's check if it has any queued items
		// Create a temporary storage instance to check queue status
		tempStorage, err := storage.NewSQLiteStorage(cfg.DatabasePath)
		if err != nil {
			return fmt.Errorf("failed to open database %s: %w", cfg.DatabasePath, err)
		}

		// Check if queue has any items (queued or processing)
		hasWork, err := tempStorage.HasQueuedItems()
		if err != nil {
			if closeErr := tempStorage.Close(); closeErr != nil {
				return fmt.Errorf("failed to check queue status: %w (close error: %v)", err, closeErr)
			}
			return fmt.Errorf("failed to check queue status: %w", err)
		}
		if closeErr := tempStorage.Close(); closeErr != nil {
			return fmt.Errorf("failed to close temporary storage: %w", closeErr)
		}

		if !hasWork {
			fmt.Printf("No URLs provided and no queued items found in database %s\n", cfg.DatabasePath)
			fmt.Printf("Nothing to crawl. Exiting.\n")
			return nil
		}

		fmt.Printf("Resuming crawl from existing database: %s\n", cfg.DatabasePath)
	}

	// Create database directory if it doesn't exist
	dbDir := filepath.Dir(cfg.DatabasePath)
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	fmt.Printf("Starting crawler with configuration:\n")
	if len(cfg.SeedURLs) > 0 {
		fmt.Printf("  Seed URLs: %v\n", cfg.SeedURLs)
	} else {
		fmt.Printf("  Seed URLs: (none - resuming from existing queue)\n")
	}
	fmt.Printf("  Limit: %d\n", cfg.Limit)
	fmt.Printf("  Concurrency: %d\n", cfg.Concurrency)
	fmt.Printf("  Request Delay: %v\n", cfg.RequestDelay)
	fmt.Printf("  Database: %s\n", cfg.DatabasePath)
	fmt.Printf("  Ignore Robots: %t\n", cfg.IgnoreRobots)

	// Display auth status without exposing credentials
	if username, password := cfg.GetBasicAuthCredentials(); username != "" && password != "" {
		fmt.Printf("  Authentication: Basic (username: %s)\n", username)
	} else {
		fmt.Printf("  Authentication: None\n")
	}

	// Initialize and start the crawler
	crawler, err := initializeCrawler(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize crawler: %w", err)
	}
	defer func() { _ = crawler.Stop() }()

	// Start crawling
	return crawler.Start(cmd.Context(), cfg.SeedURLs)
}

// initializeCrawler creates and configures a crawler instance
func initializeCrawler(cfg *config.CrawlConfig) (crawler.Crawler, error) {
	// Initialize storage
	store, err := storage.NewSQLiteStorage(cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Pass the complete config directly to the crawler
	return crawler.NewCrawler(cfg, store)
}

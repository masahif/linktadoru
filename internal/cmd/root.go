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

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"
)

var (
	cfgFile   string
	cfg       *config.CrawlConfig
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")

	// Basic crawling flags
	rootCmd.Flags().IntP("concurrency", "c", 10, "Number of concurrent workers")
	rootCmd.Flags().DurationP("delay", "r", 1*time.Second, "Delay between requests")
	rootCmd.Flags().DurationP("timeout", "t", 30*time.Second, "HTTP request timeout")
	rootCmd.Flags().StringP("user-agent", "u", "LinkTadoru/1.0", "HTTP User-Agent header")
	rootCmd.Flags().Bool("ignore-robots", false, "Ignore robots.txt rules")
	rootCmd.Flags().IntP("limit", "l", 0, "Stop after N pages (0=unlimited)")

	// URL filtering flags
	rootCmd.Flags().StringSlice("include-patterns", []string{}, "Regex patterns for URLs to include")
	rootCmd.Flags().StringSlice("exclude-patterns", []string{}, "Regex patterns for URLs to exclude")

	// Database flags
	rootCmd.Flags().StringP("database", "d", "./crawl.db", "Path to SQLite database file")

	// Bind flags to viper
	_ = viper.BindPFlag("concurrency", rootCmd.Flags().Lookup("concurrency"))
	_ = viper.BindPFlag("request_delay", rootCmd.Flags().Lookup("delay"))
	_ = viper.BindPFlag("request_timeout", rootCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("user_agent", rootCmd.Flags().Lookup("user-agent"))
	_ = viper.BindPFlag("respect_robots", rootCmd.Flags().Lookup("ignore-robots"))
	_ = viper.BindPFlag("limit", rootCmd.Flags().Lookup("limit"))
	_ = viper.BindPFlag("include_patterns", rootCmd.Flags().Lookup("include-patterns"))
	_ = viper.BindPFlag("exclude_patterns", rootCmd.Flags().Lookup("exclude-patterns"))
	_ = viper.BindPFlag("database_path", rootCmd.Flags().Lookup("database"))
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
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix("LT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func runCrawler(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg = config.DefaultConfig()

	// Set seed URLs from command line arguments
	cfg.SeedURLs = args

	// Override with viper values
	if err := viper.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Handle the inverted logic for respect_robots flag
	if cmd.Flags().Changed("ignore-robots") {
		ignoreRobots, _ := cmd.Flags().GetBool("ignore-robots")
		cfg.RespectRobots = !ignoreRobots
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
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
	fmt.Printf("  Respect Robots: %t\n", cfg.RespectRobots)

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

	// Convert config to crawler config
	crawlConfig := &crawler.CrawlConfig{
		SeedURLs:        cfg.SeedURLs,
		Concurrency:     cfg.Concurrency,
		RequestDelay:    cfg.RequestDelay,
		RequestTimeout:  cfg.RequestTimeout,
		UserAgent:       cfg.UserAgent,
		RespectRobots:   cfg.RespectRobots,
		IncludePatterns: cfg.IncludePatterns,
		ExcludePatterns: cfg.ExcludePatterns,
		DatabasePath:    cfg.DatabasePath,
		Limit:           cfg.Limit,
	}

	return crawler.NewCrawler(crawlConfig, store)
}

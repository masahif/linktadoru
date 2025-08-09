package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config represents the logging configuration
type Config struct {
	Level      slog.Level
	FilePath   string
	MaxSize    int64 // MB
	MaxBackups int
	Console    bool
}

// DefaultConfig returns the default logging configuration
func DefaultConfig() *Config {
	return &Config{
		Level:      slog.LevelInfo,
		FilePath:   "",
		MaxSize:    100, // 100MB
		MaxBackups: 5,
		Console:    true,
	}
}

// ParseLevel converts a string log level to slog.Level
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger creates a new logger with the given configuration
func NewLogger(config Config) (*slog.Logger, error) {
	var writers []io.Writer

	// Console output
	if config.Console {
		writers = append(writers, os.Stdout)
	}

	// File output with rotation
	if config.FilePath != "" {
		// Ensure directory exists
		dir := filepath.Dir(config.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		fileWriter, err := NewRotatingFileWriter(
			config.FilePath,
			config.MaxSize*1024*1024, // MB to bytes
			config.MaxBackups,
		)
		if err != nil {
			return nil, err
		}
		writers = append(writers, fileWriter)
	}

	// If no writers configured, use os.Stdout as default
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: config.Level,
	})

	return slog.New(handler), nil
}

// SetDefault creates and sets a default logger with the given configuration
func SetDefault(config Config) error {
	logger, err := NewLogger(config)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	return nil
}
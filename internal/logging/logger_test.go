package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"debug level", "debug", slog.LevelDebug},
		{"info level", "info", slog.LevelInfo},
		{"warn level", "warn", slog.LevelWarn},
		{"warning level", "warning", slog.LevelWarn},
		{"error level", "error", slog.LevelError},
		{"uppercase DEBUG", "DEBUG", slog.LevelDebug},
		{"mixed case Info", "Info", slog.LevelInfo},
		{"invalid level", "invalid", slog.LevelInfo}, // defaults to info
		{"empty string", "", slog.LevelInfo},          // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != slog.LevelInfo {
		t.Errorf("Default level = %v, want %v", cfg.Level, slog.LevelInfo)
	}
	if cfg.FilePath != "" {
		t.Errorf("Default FilePath = %q, want empty", cfg.FilePath)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("Default MaxSize = %d, want 100", cfg.MaxSize)
	}
	if cfg.MaxBackups != 5 {
		t.Errorf("Default MaxBackups = %d, want 5", cfg.MaxBackups)
	}
	if !cfg.Console {
		t.Errorf("Default Console = %v, want true", cfg.Console)
	}
}

func TestNewLogger(t *testing.T) {
	t.Run("console only", func(t *testing.T) {
		config := Config{
			Level:   slog.LevelInfo,
			Console: true,
		}
		logger, err := NewLogger(config)
		if err != nil {
			t.Fatalf("NewLogger failed: %v", err)
		}
		if logger == nil {
			t.Fatal("NewLogger returned nil logger")
		}
	})

	t.Run("file output", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		config := Config{
			Level:      slog.LevelDebug,
			FilePath:   logFile,
			MaxSize:    10,
			MaxBackups: 3,
			Console:    false,
		}

		logger, err := NewLogger(config)
		if err != nil {
			t.Fatalf("NewLogger failed: %v", err)
		}
		if logger == nil {
			t.Fatal("NewLogger returned nil logger")
		}

		// Test that log file is created
		logger.Info("test message")
		
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Errorf("Log file was not created at %s", logFile)
		}
	})

	t.Run("both console and file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		config := Config{
			Level:      slog.LevelInfo,
			FilePath:   logFile,
			MaxSize:    10,
			MaxBackups: 3,
			Console:    true,
		}

		logger, err := NewLogger(config)
		if err != nil {
			t.Fatalf("NewLogger failed: %v", err)
		}
		if logger == nil {
			t.Fatal("NewLogger returned nil logger")
		}
	})

	t.Run("no outputs configured defaults to console", func(t *testing.T) {
		config := Config{
			Level:   slog.LevelInfo,
			Console: false,
			// No file path, console disabled - should default to console
		}

		logger, err := NewLogger(config)
		if err != nil {
			t.Fatalf("NewLogger failed: %v", err)
		}
		if logger == nil {
			t.Fatal("NewLogger returned nil logger")
		}
	})
}

func TestSetDefault(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	config := Config{
		Level:      slog.LevelDebug,
		FilePath:   logFile,
		MaxSize:    10,
		MaxBackups: 3,
		Console:    false,
	}

	err := SetDefault(config)
	if err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	// Test that the default logger works
	slog.Info("test message from default logger")

	// Check that log file was created
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logFile)
	}
}
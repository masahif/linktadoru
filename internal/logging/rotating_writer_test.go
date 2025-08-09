package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRotatingFileWriter(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	writer, err := NewRotatingFileWriter(logFile, 1024, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter failed: %v", err)
	}
	defer writer.Close()

	if writer.filePath != logFile {
		t.Errorf("FilePath = %q, want %q", writer.filePath, logFile)
	}
	if writer.maxSize != 1024 {
		t.Errorf("MaxSize = %d, want 1024", writer.maxSize)
	}
	if writer.maxBackups != 3 {
		t.Errorf("MaxBackups = %d, want 3", writer.maxBackups)
	}
}

func TestRotatingFileWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	writer, err := NewRotatingFileWriter(logFile, 100, 3) // 100 bytes max
	if err != nil {
		t.Fatalf("NewRotatingFileWriter failed: %v", err)
	}
	defer writer.Close()

	// Write some data
	data := []byte("This is a test log message\n")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	// Verify file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if string(content) != string(data) {
		t.Errorf("File content = %q, want %q", string(content), string(data))
	}
}

func TestRotatingFileWriter_Rotation(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Small max size to trigger rotation
	writer, err := NewRotatingFileWriter(logFile, 50, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter failed: %v", err)
	}
	defer writer.Close()

	// Write data that exceeds max size
	firstMsg := strings.Repeat("A", 30) + "\n"
	secondMsg := strings.Repeat("B", 30) + "\n" // This should trigger rotation

	// Write first message
	if _, err := writer.Write([]byte(firstMsg)); err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	// Write second message (should trigger rotation)
	if _, err := writer.Write([]byte(secondMsg)); err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	// Check that current log file contains second message
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if string(content) != secondMsg {
		t.Errorf("Current log content = %q, want %q", string(content), secondMsg)
	}

	// Check that backup file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	backupFound := false
	for _, file := range files {
		if strings.Contains(file.Name(), "test-") && strings.HasSuffix(file.Name(), ".1.log") {
			backupFound = true
			// Read backup and verify it contains first message
			backupPath := filepath.Join(tmpDir, file.Name())
			backupContent, err := os.ReadFile(backupPath)
			if err != nil {
				t.Fatalf("Failed to read backup file: %v", err)
			}
			if string(backupContent) != firstMsg {
				t.Errorf("Backup content = %q, want %q", string(backupContent), firstMsg)
			}
			break
		}
	}

	if !backupFound {
		t.Error("Backup file was not created")
	}
}

func TestRotatingFileWriter_MaxBackups(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Small max size and only 2 backups
	writer, err := NewRotatingFileWriter(logFile, 20, 2)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter failed: %v", err)
	}
	defer writer.Close()

	// Write multiple messages to trigger multiple rotations
	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("Message %d: %s\n", i, strings.Repeat("X", 15))
		if _, err := writer.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	// Count backup files
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	backupCount := 0
	for _, file := range files {
		if strings.Contains(file.Name(), "test-") && strings.Contains(file.Name(), ".log") {
			backupCount++
		}
	}

	// Should have at most maxBackups (2) backup files plus the current file
	if backupCount > 2 {
		t.Errorf("Found %d backup files, expected at most 2", backupCount)
	}
}

func TestRotatingFileWriter_BackupName(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "app.log")

	writer, err := NewRotatingFileWriter(logFile, 1024, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileWriter failed: %v", err)
	}
	defer writer.Close()

	backupName := writer.backupName(1)
	
	// Check that backup name contains the base name and index
	if !strings.Contains(backupName, "app-") {
		t.Errorf("Backup name %q doesn't contain base name", backupName)
	}
	if !strings.HasSuffix(backupName, ".1.log") {
		t.Errorf("Backup name %q doesn't have correct suffix", backupName)
	}
	
	// Check that it's in the same directory
	if filepath.Dir(backupName) != tmpDir {
		t.Errorf("Backup directory = %q, want %q", filepath.Dir(backupName), tmpDir)
	}
}
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingFileWriter implements a file writer with size-based rotation
type RotatingFileWriter struct {
	mu         sync.Mutex
	file       *os.File
	filePath   string
	maxSize    int64
	maxBackups int
	size       int64
}

// NewRotatingFileWriter creates a new rotating file writer
func NewRotatingFileWriter(filePath string, maxSize int64, maxBackups int) (*RotatingFileWriter, error) {
	w := &RotatingFileWriter{
		filePath:   filePath,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}

	// Open or create the file
	if err := w.openFile(); err != nil {
		return nil, err
	}

	// Get current file size
	info, err := w.file.Stat()
	if err != nil {
		_ = w.file.Close()
		return nil, err
	}
	w.size = info.Size()

	return w, nil
}

// Write implements io.Writer
func (w *RotatingFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation is needed
	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close closes the file
func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// openFile opens the log file for writing
func (w *RotatingFileWriter) openFile() error {
	file, err := os.OpenFile(w.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

// rotate performs the log rotation
func (w *RotatingFileWriter) rotate() error {
	// Close current file
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}

	// Rotate existing files
	for i := w.maxBackups - 1; i > 0; i-- {
		oldPath := w.backupName(i)
		newPath := w.backupName(i + 1)

		// Remove the oldest backup if it exists
		if i == w.maxBackups-1 {
			_ = os.Remove(newPath)
			continue
		}

		// Rename backup files
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}

	// Rename current file to .1
	_ = os.Rename(w.filePath, w.backupName(1))
	// If rename fails, we continue anyway as the file might not exist yet

	// Create new file
	if err := w.openFile(); err != nil {
		return err
	}

	w.size = 0
	return nil
}

// backupName generates the name for a backup file
func (w *RotatingFileWriter) backupName(index int) string {
	dir := filepath.Dir(w.filePath)
	base := filepath.Base(w.filePath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	timestamp := time.Now().Format("20060102")
	return filepath.Join(dir, fmt.Sprintf("%s-%s.%d%s", name, timestamp, index, ext))
}

// Ensure RotatingFileWriter implements io.WriteCloser
var _ io.WriteCloser = (*RotatingFileWriter)(nil)

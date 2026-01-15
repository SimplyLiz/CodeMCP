package slogutil

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// RotatingFile implements io.WriteCloser with size-based rotation.
// It rotates log files when they exceed maxSize bytes, keeping up to
// maxBackups rotated files (e.g., log.1, log.2, log.3).
type RotatingFile struct {
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
	size       int64
	mu         sync.Mutex
}

// OpenRotatingFile opens a file with rotation support.
// If maxSize is 0, rotation is disabled.
// If maxBackups is 0, old rotated files are deleted immediately.
func OpenRotatingFile(path string, maxSize int64, maxBackups int) (*RotatingFile, error) {
	rf := &RotatingFile{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}

	if err := rf.openFile(); err != nil {
		return nil, err
	}

	return rf, nil
}

// openFile opens or creates the log file and gets its current size
func (r *RotatingFile) openFile() error {
	// Ensure parent directory exists
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Get current file size
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}

	r.file = f
	r.size = info.Size()
	return nil
}

// Write implements io.Writer. It rotates the file if needed before writing.
func (r *RotatingFile) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if rotation is needed
	if r.maxSize > 0 && r.size+int64(len(p)) > r.maxSize {
		// Log rotation failed, but try to write anyway
		// This prevents log loss in case of rotation issues
		_ = r.rotate()
	}

	n, err = r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// Close implements io.Closer
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// rotate performs the rotation: log -> log.1 -> log.2 -> ...
func (r *RotatingFile) rotate() error {
	// Close current file
	if r.file != nil {
		if err := r.file.Close(); err != nil {
			return err
		}
	}

	// Rotate existing backup files
	// Delete oldest if we're at maxBackups
	for i := r.maxBackups; i >= 1; i-- {
		oldPath := r.backupPath(i)
		newPath := r.backupPath(i + 1)

		if i == r.maxBackups {
			// Delete the oldest backup
			_ = os.Remove(oldPath)
		} else {
			// Rename to next backup number
			if _, err := os.Stat(oldPath); err == nil {
				_ = os.Rename(oldPath, newPath)
			}
		}
	}

	// Rename current log to .1
	if r.maxBackups > 0 {
		// If rename fails, continue anyway - the old file may have already been removed
		_ = os.Rename(r.path, r.backupPath(1))
	} else {
		// No backups, just truncate
		_ = os.Remove(r.path)
	}

	// Open new log file
	r.size = 0
	return r.openFile()
}

// backupPath returns the path for a backup file (e.g., log.1, log.2)
func (r *RotatingFile) backupPath(n int) string {
	return fmt.Sprintf("%s.%d", r.path, n)
}

// ParseSize parses a size string like "10MB", "1GB", "500KB" into bytes.
// Supported suffixes: B, KB, MB, GB (case-insensitive)
// Returns 0 for empty or invalid strings.
func ParseSize(s string) int64 {
	if s == "" {
		return 0
	}

	s = strings.TrimSpace(strings.ToUpper(s))

	// Extract numeric part and suffix
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(B|KB|MB|GB)?$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	suffix := matches[2]
	if suffix == "" {
		suffix = "B"
	}

	var multiplier float64
	switch suffix {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0
	}

	return int64(value * multiplier)
}

// NewFileLoggerWithRotation creates a rotating file logger.
// Uses the provided maxSize (e.g., "10MB") and maxBackups settings.
// If maxSize is empty or invalid, falls back to regular file logger.
func NewFileLoggerWithRotation(path string, level slog.Level, maxSize string, maxBackups int) (*slog.Logger, io.Closer, error) {
	size := ParseSize(maxSize)
	if size <= 0 {
		// No rotation, use regular file logger
		return NewFileLogger(path, level)
	}

	rf, err := OpenRotatingFile(path, size, maxBackups)
	if err != nil {
		return nil, nil, err
	}

	return NewLogger(rf, level), rf, nil
}

//go:build !windows

package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockFile = "index.lock"

// Lock represents an exclusive lock on the index.
type Lock struct {
	path string
	file *os.File
}

// AcquireLock attempts to acquire an exclusive lock on the index.
// Returns an error if another process holds the lock.
func AcquireLock(ckbDir string) (*Lock, error) {
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("creating .ckb directory: %w", err)
	}

	path := filepath.Join(ckbDir, lockFile)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = file.Close()

		// Try to read existing lock info for better error message
		if content, readErr := os.ReadFile(path); readErr == nil && len(content) > 0 {
			pid := strings.TrimSpace(string(content))
			return nil, fmt.Errorf("index is locked by another process (PID %s). Another ckb command may be running", pid)
		}
		return nil, fmt.Errorf("index is locked by another process. Another ckb command may be running")
	}

	// Write our PID to the lock file
	if err := file.Truncate(0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("truncating lock file: %w", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("seeking lock file: %w", err)
	}

	if _, err := file.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("writing PID to lock file: %w", err)
	}

	return &Lock{path: path, file: file}, nil
}

// Release releases the lock and removes the lock file.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}

	// Release the flock
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)

	// Close the file
	_ = l.file.Close()

	// Remove the lock file (best effort)
	_ = os.Remove(l.path)
}

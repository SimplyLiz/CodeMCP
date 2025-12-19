package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// PIDFile manages the daemon PID file
type PIDFile struct {
	path string
}

// NewPIDFile creates a new PID file manager
func NewPIDFile(path string) *PIDFile {
	return &PIDFile{path: path}
}

// Acquire creates the PID file with the current process ID
func (p *PIDFile) Acquire() error {
	// Check if another daemon is running
	running, pid, err := p.IsRunning()
	if err != nil {
		return err
	}

	if running {
		return fmt.Errorf("daemon is already running (PID: %d)", pid)
	}

	// Remove stale PID file if it exists
	if err := p.removeStale(); err != nil {
		return err
	}

	// Write current PID
	pid = os.Getpid()
	content := fmt.Sprintf("%d\n", pid)

	if err := os.WriteFile(p.path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

// Release removes the PID file
func (p *PIDFile) Release() error {
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// IsRunning checks if a daemon is currently running
// Returns (running, pid, error)
func (p *PIDFile) IsRunning() (bool, int, error) {
	data, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Invalid PID file, treat as not running
		return false, 0, nil //nolint:nilerr // intentional: invalid PID treated as not running
	}

	// Check if process exists
	if processExists(pid) {
		return true, pid, nil
	}

	return false, pid, nil
}

// GetPID returns the PID from the file, or 0 if not found
func (p *PIDFile) GetPID() (int, error) {
	data, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	return strconv.Atoi(pidStr)
}

// removeStale removes a stale PID file if the process is not running
func (p *PIDFile) removeStale() error {
	running, _, err := p.IsRunning()
	if err != nil {
		return err
	}

	if !running {
		// Remove stale file
		if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// processExists checks if a process with the given PID exists
func processExists(pid int) bool {
	// On Unix, sending signal 0 checks if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 doesn't send anything but checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

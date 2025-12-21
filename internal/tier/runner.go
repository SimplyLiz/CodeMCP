package tier

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ExecRunner abstracts command execution for testability.
type ExecRunner interface {
	// LookPath checks if a binary exists in PATH.
	LookPath(name string) (string, error)

	// Run executes a command and returns its output.
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

// RealRunner implements ExecRunner using os/exec.
type RealRunner struct {
	// Timeout for each command execution.
	Timeout time.Duration
}

// NewRealRunner creates a runner with the given timeout.
func NewRealRunner(timeout time.Duration) *RealRunner {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &RealRunner{Timeout: timeout}
}

// LookPath checks if a binary exists in PATH.
func (r *RealRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// Run executes a command and returns its output.
func (r *RealRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// MockRunner implements ExecRunner for testing.
type MockRunner struct {
	mu       sync.Mutex
	lookPath map[string]string
	commands map[string]mockResult
}

type mockResult struct {
	stdout string
	stderr string
	err    error
}

// NewMockRunner creates a new mock runner.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		lookPath: make(map[string]string),
		commands: make(map[string]mockResult),
	}
}

// SetLookPath configures the mock to return a path for the given name.
func (m *MockRunner) SetLookPath(name, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookPath[name] = path
}

// SetCommand configures the mock result for a command.
func (m *MockRunner) SetCommand(name string, stdout, stderr string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[name] = mockResult{stdout: stdout, stderr: stderr, err: err}
}

// LookPath implements ExecRunner.
func (m *MockRunner) LookPath(name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if path, ok := m.lookPath[name]; ok {
		return path, nil
	}
	return "", exec.ErrNotFound
}

// Run implements ExecRunner.
func (m *MockRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Try exact match first
	key := name
	if result, ok := m.commands[key]; ok {
		return result.stdout, result.stderr, result.err
	}

	// Try with args
	key = name + " " + strings.Join(args, " ")
	if result, ok := m.commands[key]; ok {
		return result.stdout, result.stderr, result.err
	}

	return "", "", exec.ErrNotFound
}

// CachingRunner wraps an ExecRunner with caching.
type CachingRunner struct {
	runner ExecRunner

	mu        sync.RWMutex
	lookCache map[string]lookCacheEntry
	runCache  map[string]runCacheEntry
}

type lookCacheEntry struct {
	path string
	err  error
}

type runCacheEntry struct {
	stdout string
	stderr string
	err    error
}

// NewCachingRunner wraps a runner with caching.
func NewCachingRunner(runner ExecRunner) *CachingRunner {
	return &CachingRunner{
		runner:    runner,
		lookCache: make(map[string]lookCacheEntry),
		runCache:  make(map[string]runCacheEntry),
	}
}

// LookPath implements ExecRunner with caching.
func (c *CachingRunner) LookPath(name string) (string, error) {
	c.mu.RLock()
	if entry, ok := c.lookCache[name]; ok {
		c.mu.RUnlock()
		return entry.path, entry.err
	}
	c.mu.RUnlock()

	path, err := c.runner.LookPath(name)

	c.mu.Lock()
	c.lookCache[name] = lookCacheEntry{path: path, err: err}
	c.mu.Unlock()

	return path, err
}

// Run implements ExecRunner with caching.
func (c *CachingRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	key := name + "|" + strings.Join(args, "|")

	c.mu.RLock()
	if entry, ok := c.runCache[key]; ok {
		c.mu.RUnlock()
		return entry.stdout, entry.stderr, entry.err
	}
	c.mu.RUnlock()

	stdout, stderr, err := c.runner.Run(ctx, name, args...)

	c.mu.Lock()
	c.runCache[key] = runCacheEntry{stdout: stdout, stderr: stderr, err: err}
	c.mu.Unlock()

	return stdout, stderr, err
}

// Clear clears the cache.
func (c *CachingRunner) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookCache = make(map[string]lookCacheEntry)
	c.runCache = make(map[string]runCacheEntry)
}

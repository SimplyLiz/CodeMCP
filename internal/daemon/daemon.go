// Package daemon provides the CKB daemon mode for always-on service.
package daemon

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"ckb/internal/config"
	"ckb/internal/index"
	"ckb/internal/logging"
	"ckb/internal/paths"
	"ckb/internal/scheduler"
	"ckb/internal/version"
	"ckb/internal/watcher"
	"ckb/internal/webhooks"
)

// Daemon represents the CKB daemon process
type Daemon struct {
	config *config.DaemonConfig
	server *http.Server
	pid    *PIDFile
	logger *log.Logger

	// Components
	scheduler      *scheduler.Scheduler
	watcher        *watcher.Watcher
	webhookManager *webhooks.Manager
	refreshManager *RefreshManager
	structuredLog  *logging.Logger

	// Shutdown coordination
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// State
	startedAt time.Time
	mu        sync.RWMutex
}

// DaemonState represents the current daemon state
type DaemonState struct {
	PID          int           `json:"pid"`
	StartedAt    time.Time     `json:"startedAt"`
	Port         int           `json:"port"`
	Bind         string        `json:"bind"`
	Version      string        `json:"version"`
	Uptime       time.Duration `json:"uptime"`
	JobsRunning  int           `json:"jobsRunning"`
	JobsQueued   int           `json:"jobsQueued"`
	ReposWatched int           `json:"reposWatched"`
}

// New creates a new daemon instance
func New(cfg *config.DaemonConfig) (*Daemon, error) {
	// Setup logging
	logPath := cfg.LogFile
	if logPath == "" {
		var err error
		logPath, err = paths.GetDaemonLogPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get log path: %w", err)
		}
	}

	// Ensure daemon directory exists
	daemonDir, err := paths.EnsureDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("failed to create daemon directory: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New(logFile, "[ckb-daemon] ", log.LstdFlags|log.Lmicroseconds)

	// Create structured logger for components
	structuredLogger := logging.NewLogger(logging.Config{
		Level:  logging.InfoLevel,
		Format: logging.JSONFormat,
		Output: logFile,
	})

	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		config:        cfg,
		logger:        logger,
		structuredLog: structuredLogger,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize scheduler
	sched, err := scheduler.New(daemonDir, structuredLogger, scheduler.DefaultConfig())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}
	d.scheduler = sched

	// Initialize watcher
	watcherCfg := watcher.DefaultConfig()
	watcherCfg.Enabled = cfg.Watch.Enabled
	watcherCfg.DebounceMs = cfg.Watch.DebounceMs
	if len(cfg.Watch.IgnorePatterns) > 0 {
		watcherCfg.IgnorePatterns = cfg.Watch.IgnorePatterns
	}
	d.watcher = watcher.New(watcherCfg, structuredLogger, d.onWatcherChange)

	// Initialize webhook manager
	webhookMgr, err := webhooks.NewManager(daemonDir, structuredLogger, webhooks.DefaultConfig())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create webhook manager: %w", err)
	}
	d.webhookManager = webhookMgr

	// Initialize refresh manager
	d.refreshManager = NewRefreshManager(structuredLogger, logger, webhookMgr)

	return d, nil
}

// onWatcherChange handles file system change events
func (d *Daemon) onWatcherChange(repoPath string, events []watcher.Event) {
	d.logger.Printf("File changes detected in %s (%d events)", repoPath, len(events))

	// Skip if refresh already pending for this repo
	if d.refreshManager.HasPendingRefresh(repoPath) {
		d.logger.Printf("Refresh already pending for %s, skipping", repoPath)
		return
	}

	// Determine trigger type from events
	trigger, triggerInfo := d.detectTriggerFromEvents(events)

	// Queue incremental refresh in background
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.refreshManager.RunIncrementalRefreshWithTrigger(d.ctx, repoPath, trigger, triggerInfo)
	}()
}

// detectTriggerFromEvents determines the trigger type from watcher events
func (d *Daemon) detectTriggerFromEvents(events []watcher.Event) (index.RefreshTrigger, string) {
	for _, e := range events {
		if strings.HasSuffix(e.Path, "HEAD") {
			return index.TriggerHEAD, "branch or commit changed"
		}
		if strings.HasSuffix(e.Path, "index") {
			return index.TriggerIndex, "staged files changed"
		}
	}
	return index.TriggerStale, ""
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.logger.Printf("Starting CKB daemon v%s", version.Version)

	// Create and acquire PID file
	pidPath, err := paths.GetDaemonPIDPath()
	if err != nil {
		return fmt.Errorf("failed to get PID path: %w", err)
	}

	d.pid = NewPIDFile(pidPath)
	if err := d.pid.Acquire(); err != nil {
		return fmt.Errorf("failed to acquire PID file: %w", err)
	}

	d.startedAt = time.Now()

	// Start scheduler
	if err := d.scheduler.Start(); err != nil {
		d.logger.Printf("Failed to start scheduler: %v", err)
	} else {
		d.logger.Println("Scheduler started")
	}

	// Start watcher
	if err := d.watcher.Start(); err != nil {
		d.logger.Printf("Failed to start watcher: %v", err)
	} else {
		d.logger.Println("File watcher started")
	}

	// Start webhook manager
	if err := d.webhookManager.Start(); err != nil {
		d.logger.Printf("Failed to start webhook manager: %v", err)
	} else {
		d.logger.Println("Webhook manager started")
	}

	// Setup HTTP server
	d.server = d.setupServer()

	// Start HTTP server in goroutine
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		addr := fmt.Sprintf("%s:%d", d.config.Bind, d.config.Port)
		d.logger.Printf("HTTP server listening on %s", addr)

		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			d.logger.Printf("HTTP server error: %v", err)
		}
	}()

	d.logger.Printf("Daemon started successfully (PID: %d)", os.Getpid())
	return nil
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() error {
	d.logger.Println("Stopping daemon...")

	// Signal shutdown
	d.cancel()

	shutdownTimeout := 30 * time.Second

	// Stop webhook manager
	if d.webhookManager != nil {
		if err := d.webhookManager.Stop(shutdownTimeout); err != nil {
			d.logger.Printf("Webhook manager shutdown error: %v", err)
		}
	}

	// Stop watcher
	if d.watcher != nil {
		if err := d.watcher.Stop(); err != nil {
			d.logger.Printf("Watcher shutdown error: %v", err)
		}
	}

	// Stop scheduler
	if d.scheduler != nil {
		if err := d.scheduler.Stop(shutdownTimeout); err != nil {
			d.logger.Printf("Scheduler shutdown error: %v", err)
		}
	}

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if d.server != nil {
		if err := d.server.Shutdown(ctx); err != nil {
			d.logger.Printf("HTTP server shutdown error: %v", err)
		}
	}

	// Wait for goroutines to finish
	d.wg.Wait()

	// Release PID file
	if d.pid != nil {
		if err := d.pid.Release(); err != nil {
			d.logger.Printf("Failed to release PID file: %v", err)
		}
	}

	d.logger.Println("Daemon stopped")
	return nil
}

// Wait blocks until the daemon receives a shutdown signal
func (d *Daemon) Wait() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		d.logger.Printf("Received signal: %v", sig)
	case <-d.ctx.Done():
		d.logger.Println("Context cancelled")
	}
}

// State returns the current daemon state
func (d *Daemon) State() *DaemonState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state := &DaemonState{
		PID:       os.Getpid(),
		StartedAt: d.startedAt,
		Port:      d.config.Port,
		Bind:      d.config.Bind,
		Version:   version.Version,
		Uptime:    time.Since(d.startedAt),
	}

	// Add watcher stats
	if d.watcher != nil {
		state.ReposWatched = len(d.watcher.WatchedRepos())
	}

	return state
}

// IsRunning checks if the daemon is currently running
func IsRunning() (bool, int, error) {
	pidPath, err := paths.GetDaemonPIDPath()
	if err != nil {
		return false, 0, err
	}

	pid := &PIDFile{path: pidPath}
	return pid.IsRunning()
}

// StopRemote sends a stop signal to a running daemon
func StopRemote() error {
	pidPath, err := paths.GetDaemonPIDPath()
	if err != nil {
		return err
	}

	pid := &PIDFile{path: pidPath}
	running, processID, err := pid.IsRunning()
	if err != nil {
		return err
	}

	if !running {
		return fmt.Errorf("daemon is not running")
	}

	// Send SIGTERM to the daemon process
	process, err := os.FindProcess(processID)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	// Wait for process to exit (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for daemon to stop")
		case <-ticker.C:
			running, _, _ := pid.IsRunning()
			if !running {
				return nil
			}
		}
	}
}

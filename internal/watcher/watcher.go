// Package watcher provides file system watching for git changes.
package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ckb/internal/logging"
)

// EventType represents the type of file system event
type EventType int

const (
	EventCreate EventType = iota
	EventModify
	EventDelete
	EventRename
)

// Event represents a file system event
type Event struct {
	Type      EventType
	Path      string
	Timestamp time.Time
}

// String returns a string representation of the event type
func (e EventType) String() string {
	switch e {
	case EventCreate:
		return "create"
	case EventModify:
		return "modify"
	case EventDelete:
		return "delete"
	case EventRename:
		return "rename"
	default:
		return "unknown"
	}
}

// ChangeHandler is called when changes are detected
type ChangeHandler func(repoPath string, events []Event)

// Config contains watcher configuration
type Config struct {
	Enabled        bool          `json:"enabled" mapstructure:"enabled"`
	DebounceMs     int           `json:"debounceMs" mapstructure:"debounce_ms"`
	IgnorePatterns []string      `json:"ignorePatterns" mapstructure:"ignore_patterns"`
	Repos          []string      `json:"repos" mapstructure:"repos"` // repo IDs or "all"
	PollInterval   time.Duration `json:"-"`                          // for polling fallback
}

// DefaultConfig returns the default watcher configuration
func DefaultConfig() Config {
	return Config{
		Enabled:    true,
		DebounceMs: 5000,
		IgnorePatterns: []string{
			"*.log",
			"*.tmp",
			"node_modules/**",
			".git/objects/**",
			".git/logs/**",
			"vendor/**",
			"__pycache__/**",
			".ckb/**",
		},
		Repos:        []string{"all"},
		PollInterval: 2 * time.Second,
	}
}

// Watcher watches for file system changes in git repositories
type Watcher struct {
	config   Config
	logger   *logging.Logger
	handler  ChangeHandler
	repos    map[string]*repoWatcher // repoPath -> watcher

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup
}

// repoWatcher watches a single repository
type repoWatcher struct {
	repoPath   string
	gitDir     string
	debouncer  *Debouncer
	lastHead   string
	lastIndex  time.Time
	stopCh     chan struct{}
}

// New creates a new file system watcher
func New(config Config, logger *logging.Logger, handler ChangeHandler) *Watcher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		config:  config,
		logger:  logger,
		handler: handler,
		repos:   make(map[string]*repoWatcher),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start begins watching
func (w *Watcher) Start() error {
	if !w.config.Enabled {
		w.logger.Info("File watcher is disabled", nil)
		return nil
	}

	w.logger.Info("Starting file watcher", map[string]interface{}{
		"debounceMs": w.config.DebounceMs,
	})

	return nil
}

// Stop stops watching
func (w *Watcher) Stop() error {
	w.logger.Info("Stopping file watcher", nil)
	w.cancel()

	w.mu.Lock()
	for _, rw := range w.repos {
		close(rw.stopCh)
	}
	w.mu.Unlock()

	w.wg.Wait()
	w.logger.Info("File watcher stopped", nil)
	return nil
}

// WatchRepo starts watching a repository
func (w *Watcher) WatchRepo(repoPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.repos[repoPath]; exists {
		return nil // Already watching
	}

	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil // Not a git repo
	}

	rw := &repoWatcher{
		repoPath:  repoPath,
		gitDir:    gitDir,
		debouncer: NewDebouncer(time.Duration(w.config.DebounceMs) * time.Millisecond),
		stopCh:    make(chan struct{}),
	}

	// Read initial HEAD
	rw.lastHead = w.readHead(gitDir)
	rw.lastIndex = w.getIndexModTime(gitDir)

	w.repos[repoPath] = rw

	w.wg.Add(1)
	go w.watchRepo(rw)

	w.logger.Info("Watching repository", map[string]interface{}{
		"path": repoPath,
	})

	return nil
}

// UnwatchRepo stops watching a repository
func (w *Watcher) UnwatchRepo(repoPath string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if rw, exists := w.repos[repoPath]; exists {
		close(rw.stopCh)
		delete(w.repos, repoPath)
		w.logger.Info("Stopped watching repository", map[string]interface{}{
			"path": repoPath,
		})
	}
}

// watchRepo polls for changes in a repository
// Using polling instead of fsnotify for simplicity and cross-platform compatibility
func (w *Watcher) watchRepo(rw *repoWatcher) {
	defer w.wg.Done()

	pollInterval := w.config.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkRepoChanges(rw)
		case <-rw.stopCh:
			return
		case <-w.ctx.Done():
			return
		}
	}
}

// checkRepoChanges checks for git changes in a repository
func (w *Watcher) checkRepoChanges(rw *repoWatcher) {
	var events []Event

	// Check HEAD changes (branch switch, new commit)
	currentHead := w.readHead(rw.gitDir)
	if currentHead != "" && currentHead != rw.lastHead {
		events = append(events, Event{
			Type:      EventModify,
			Path:      filepath.Join(rw.gitDir, "HEAD"),
			Timestamp: time.Now(),
		})
		rw.lastHead = currentHead
	}

	// Check index changes (staged files)
	currentIndex := w.getIndexModTime(rw.gitDir)
	if !currentIndex.IsZero() && currentIndex.After(rw.lastIndex) {
		events = append(events, Event{
			Type:      EventModify,
			Path:      filepath.Join(rw.gitDir, "index"),
			Timestamp: time.Now(),
		})
		rw.lastIndex = currentIndex
	}

	if len(events) > 0 {
		// Debounce the events
		rw.debouncer.Trigger(func() {
			w.logger.Debug("Git changes detected", map[string]interface{}{
				"repoPath":   rw.repoPath,
				"eventCount": len(events),
			})
			if w.handler != nil {
				w.handler(rw.repoPath, events)
			}
		})
	}
}

// readHead reads the current HEAD reference
func (w *Watcher) readHead(gitDir string) string {
	headPath := filepath.Join(gitDir, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// getIndexModTime returns the modification time of the git index
func (w *Watcher) getIndexModTime(gitDir string) time.Time {
	indexPath := filepath.Join(gitDir, "index")
	info, err := os.Stat(indexPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// IsIgnored checks if a path matches ignore patterns
func (w *Watcher) IsIgnored(path string) bool {
	for _, pattern := range w.config.IgnorePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}

		// Handle ** patterns
		if strings.Contains(pattern, "**") {
			// Simple glob matching for **
			parts := strings.Split(pattern, "**")
			if len(parts) == 2 {
				if strings.HasPrefix(path, strings.TrimSuffix(parts[0], "/")) &&
					(parts[1] == "" || strings.HasSuffix(path, strings.TrimPrefix(parts[1], "/"))) {
					return true
				}
			}
		}
	}
	return false
}

// WatchedRepos returns the list of watched repository paths
func (w *Watcher) WatchedRepos() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	repos := make([]string, 0, len(w.repos))
	for path := range w.repos {
		repos = append(repos, path)
	}
	return repos
}

// Stats returns watcher statistics
func (w *Watcher) Stats() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return map[string]interface{}{
		"enabled":        w.config.Enabled,
		"watchedRepos":   len(w.repos),
		"debounceMs":     w.config.DebounceMs,
		"ignorePatterns": len(w.config.IgnorePatterns),
	}
}

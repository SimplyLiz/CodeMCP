package watcher

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventCreate, "create"},
		{EventModify, "modify"},
		{EventDelete, "delete"},
		{EventRename, "rename"},
		{EventType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.eventType.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Enabled should be true by default")
	}
	if config.DebounceMs != 5000 {
		t.Errorf("DebounceMs = %d, want 5000", config.DebounceMs)
	}
	if len(config.IgnorePatterns) == 0 {
		t.Error("IgnorePatterns should not be empty")
	}
	if len(config.Repos) != 1 || config.Repos[0] != "all" {
		t.Errorf("Repos = %v, want [\"all\"]", config.Repos)
	}
	if config.PollInterval != 2*time.Second {
		t.Errorf("PollInterval = %v, want 2s", config.PollInterval)
	}
}

func TestDefaultConfigIgnorePatterns(t *testing.T) {
	config := DefaultConfig()

	expectedPatterns := []string{
		"*.log",
		"*.tmp",
		"node_modules/**",
		".git/objects/**",
		".git/logs/**",
	}

	for _, expected := range expectedPatterns {
		found := false
		for _, pattern := range config.IgnorePatterns {
			if pattern == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("IgnorePatterns should contain %q", expected)
		}
	}
}

func TestNewWatcher(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()
	handler := func(repoPath string, events []Event) {}

	w := New(config, logger, handler)
	if w == nil {
		t.Fatal("New() returned nil")
	}
	if w.repos == nil {
		t.Error("repos map should be initialized")
	}
	if w.ctx == nil {
		t.Error("context should be initialized")
	}
	if w.cancel == nil {
		t.Error("cancel func should be initialized")
	}
}

func TestWatcherStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()
	config.DebounceMs = 1000

	w := New(config, logger, nil)
	stats := w.Stats()

	if stats["enabled"] != true {
		t.Errorf("stats[enabled] = %v, want true", stats["enabled"])
	}
	if stats["watchedRepos"] != 0 {
		t.Errorf("stats[watchedRepos] = %v, want 0", stats["watchedRepos"])
	}
	if stats["debounceMs"] != 1000 {
		t.Errorf("stats[debounceMs] = %v, want 1000", stats["debounceMs"])
	}
}

func TestWatcherWatchedRepos(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()

	w := New(config, logger, nil)
	repos := w.WatchedRepos()

	if len(repos) != 0 {
		t.Errorf("WatchedRepos() = %v, want empty", repos)
	}
}

func TestWatcherIsIgnored(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := Config{
		IgnorePatterns: []string{
			"*.log",
			"*.tmp",
			"node_modules/**",
			".git/**",
		},
	}

	w := New(config, logger, nil)

	tests := []struct {
		path    string
		ignored bool
	}{
		{"debug.log", true},
		{"temp.tmp", true},
		{"node_modules/package/index.js", true},
		{".git/config", true},
		{"main.go", false},
		{"src/app.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := w.IsIgnored(tt.path)
			if got != tt.ignored {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.path, got, tt.ignored)
			}
		})
	}
}

func TestWatcherStartDisabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := Config{Enabled: false}

	w := New(config, logger, nil)
	err := w.Start()
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}
}

func TestWatcherStartEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()

	w := New(config, logger, nil)
	err := w.Start()
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}
}

func TestWatcherStopWithoutStart(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()

	w := New(config, logger, nil)
	err := w.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestEventStructure(t *testing.T) {
	now := time.Now()
	event := Event{
		Type:      EventModify,
		Path:      "/path/to/file.go",
		Timestamp: now,
	}

	if event.Type != EventModify {
		t.Errorf("Type = %v, want %v", event.Type, EventModify)
	}
	if event.Path != "/path/to/file.go" {
		t.Errorf("Path = %q, want '/path/to/file.go'", event.Path)
	}
	if !event.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, now)
	}
}

func TestConfigStructure(t *testing.T) {
	config := Config{
		Enabled:        true,
		DebounceMs:     3000,
		IgnorePatterns: []string{"*.log"},
		Repos:          []string{"repo1", "repo2"},
		PollInterval:   5 * time.Second,
	}

	if !config.Enabled {
		t.Error("Enabled should be true")
	}
	if config.DebounceMs != 3000 {
		t.Errorf("DebounceMs = %d, want 3000", config.DebounceMs)
	}
	if len(config.IgnorePatterns) != 1 {
		t.Errorf("IgnorePatterns len = %d, want 1", len(config.IgnorePatterns))
	}
	if len(config.Repos) != 2 {
		t.Errorf("Repos len = %d, want 2", len(config.Repos))
	}
}

// Debouncer tests

func TestNewDebouncer(t *testing.T) {
	d := NewDebouncer(100 * time.Millisecond)
	if d == nil {
		t.Fatal("NewDebouncer() returned nil")
	}
	if d.delay != 100*time.Millisecond {
		t.Errorf("delay = %v, want 100ms", d.delay)
	}
}

func TestDebouncerTrigger(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	var called int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		d.Trigger(func() {
			mu.Lock()
			called++
			mu.Unlock()
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if called != 1 {
		t.Errorf("Function should be called once, got %d", called)
	}
	mu.Unlock()
}

func TestDebouncerCancel(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	var called bool
	var mu sync.Mutex

	d.Trigger(func() {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// Cancel before debounce completes
	d.Cancel()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if called {
		t.Error("Function should not be called after cancel")
	}
	mu.Unlock()
}

func TestDebouncerFlush(t *testing.T) {
	d := NewDebouncer(500 * time.Millisecond)

	var called bool
	var mu sync.Mutex

	d.Trigger(func() {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// Flush immediately
	d.Flush()

	mu.Lock()
	if !called {
		t.Error("Function should be called after flush")
	}
	mu.Unlock()
}

func TestDebouncerFlushNoPending(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	// Flush without any pending function
	d.Flush() // Should not panic
}

func TestDebouncerCancelNoPending(t *testing.T) {
	d := NewDebouncer(50 * time.Millisecond)

	// Cancel without any pending function
	d.Cancel() // Should not panic
}

// BatchDebouncer tests

func TestNewBatchDebouncer(t *testing.T) {
	emit := func(events []Event) {}
	b := NewBatchDebouncer(100*time.Millisecond, emit)

	if b == nil {
		t.Fatal("NewBatchDebouncer() returned nil")
	}
	if b.delay != 100*time.Millisecond {
		t.Errorf("delay = %v, want 100ms", b.delay)
	}
	if b.events == nil {
		t.Error("events should be initialized")
	}
}

func TestBatchDebouncerAdd(t *testing.T) {
	var received []Event
	var mu sync.Mutex

	emit := func(events []Event) {
		mu.Lock()
		received = events
		mu.Unlock()
	}

	b := NewBatchDebouncer(50*time.Millisecond, emit)

	// Add multiple events
	b.Add(Event{Type: EventCreate, Path: "file1.go"})
	b.Add(Event{Type: EventModify, Path: "file2.go"})
	b.Add(Event{Type: EventDelete, Path: "file3.go"})

	if b.EventCount() != 3 {
		t.Errorf("EventCount() = %d, want 3", b.EventCount())
	}

	// Wait for debounce
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != 3 {
		t.Errorf("Should have received 3 events, got %d", len(received))
	}
	mu.Unlock()
}

func TestBatchDebouncerCancel(t *testing.T) {
	var called bool
	var mu sync.Mutex

	emit := func(events []Event) {
		mu.Lock()
		called = true
		mu.Unlock()
	}

	b := NewBatchDebouncer(50*time.Millisecond, emit)
	b.Add(Event{Type: EventCreate, Path: "file.go"})
	b.Cancel()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if called {
		t.Error("Emit should not be called after cancel")
	}
	mu.Unlock()

	if b.EventCount() != 0 {
		t.Errorf("EventCount() = %d, want 0 after cancel", b.EventCount())
	}
}

func TestBatchDebouncerFlush(t *testing.T) {
	var received []Event
	var mu sync.Mutex

	emit := func(events []Event) {
		mu.Lock()
		received = events
		mu.Unlock()
	}

	b := NewBatchDebouncer(500*time.Millisecond, emit)
	b.Add(Event{Type: EventCreate, Path: "file.go"})
	b.Flush()

	mu.Lock()
	if len(received) != 1 {
		t.Errorf("Should have received 1 event, got %d", len(received))
	}
	mu.Unlock()

	if b.EventCount() != 0 {
		t.Errorf("EventCount() = %d, want 0 after flush", b.EventCount())
	}
}

func TestBatchDebouncerEventCount(t *testing.T) {
	b := NewBatchDebouncer(100*time.Millisecond, nil)

	if b.EventCount() != 0 {
		t.Errorf("EventCount() = %d, want 0", b.EventCount())
	}

	b.Add(Event{Type: EventCreate})
	if b.EventCount() != 1 {
		t.Errorf("EventCount() = %d, want 1", b.EventCount())
	}

	b.Add(Event{Type: EventModify})
	if b.EventCount() != 2 {
		t.Errorf("EventCount() = %d, want 2", b.EventCount())
	}
}

func TestBatchDebouncerNoEmitWithNoEvents(t *testing.T) {
	var called bool
	var mu sync.Mutex

	emit := func(events []Event) {
		mu.Lock()
		called = true
		mu.Unlock()
	}

	b := NewBatchDebouncer(10*time.Millisecond, emit)
	b.Flush() // Flush without adding events

	mu.Lock()
	if called {
		t.Error("Emit should not be called with no events")
	}
	mu.Unlock()
}

func TestWatchRepoNonGitDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()

	w := New(config, logger, nil)

	// Try to watch a non-git directory
	err := w.WatchRepo(t.TempDir())
	if err != nil {
		t.Errorf("WatchRepo() error = %v", err)
	}

	// Should not add to watched repos since it's not a git directory
	if len(w.WatchedRepos()) != 0 {
		t.Error("Should not watch non-git directory")
	}
}

func TestUnwatchRepoNotWatched(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultConfig()

	w := New(config, logger, nil)

	// Unwatching a non-watched repo should not panic
	w.UnwatchRepo("/nonexistent/path")
}

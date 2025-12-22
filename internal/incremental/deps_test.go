package incremental

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

func setupTestDB(t *testing.T) (*storage.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}

	return db, func() {
		db.Close() //nolint:errcheck
	}
}

func TestDependencyTracker_FileDeps(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled: true,
		Mode:    InvalidationLazy,
		Depth:   1,
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Test case: Update and retrieve file dependencies
	t.Run("update and get dependencies", func(t *testing.T) {
		refs := []Reference{
			{FromFile: "a.go", ToSymbolID: "sym1"},
			{FromFile: "a.go", ToSymbolID: "sym2"},
			{FromFile: "a.go", ToSymbolID: "sym3"},
		}

		symbolToFile := map[string]string{
			"sym1": "b.go",
			"sym2": "c.go",
			"sym3": "b.go", // Duplicate - should be deduplicated
		}

		err := db.WithTx(func(tx *sql.Tx) error {
			return tracker.UpdateFileDeps(tx, "a.go", refs, symbolToFile)
		})
		if err != nil {
			t.Fatalf("UpdateFileDeps failed: %v", err)
		}

		deps, err := tracker.GetDependencies("a.go")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}

		if len(deps) != 2 {
			t.Errorf("Expected 2 dependencies, got %d", len(deps))
		}

		// Check that b.go and c.go are in the dependencies
		depSet := make(map[string]bool)
		for _, d := range deps {
			depSet[d] = true
		}
		if !depSet["b.go"] {
			t.Error("Expected b.go in dependencies")
		}
		if !depSet["c.go"] {
			t.Error("Expected c.go in dependencies")
		}
	})

	t.Run("get dependents", func(t *testing.T) {
		dependents, err := tracker.GetDependents("b.go")
		if err != nil {
			t.Fatalf("GetDependents failed: %v", err)
		}

		if len(dependents) != 1 || dependents[0] != "a.go" {
			t.Errorf("Expected [a.go], got %v", dependents)
		}
	})

	t.Run("skip self-references", func(t *testing.T) {
		refs := []Reference{
			{FromFile: "x.go", ToSymbolID: "sym_self"},
		}

		symbolToFile := map[string]string{
			"sym_self": "x.go", // Self-reference
		}

		err := db.WithTx(func(tx *sql.Tx) error {
			return tracker.UpdateFileDeps(tx, "x.go", refs, symbolToFile)
		})
		if err != nil {
			t.Fatalf("UpdateFileDeps failed: %v", err)
		}

		deps, err := tracker.GetDependencies("x.go")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}

		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies (self-ref excluded), got %d", len(deps))
		}
	})

	t.Run("skip unknown symbols", func(t *testing.T) {
		refs := []Reference{
			{FromFile: "y.go", ToSymbolID: "unknown_sym"},
		}

		symbolToFile := map[string]string{
			// unknown_sym not in map - likely external
		}

		err := db.WithTx(func(tx *sql.Tx) error {
			return tracker.UpdateFileDeps(tx, "y.go", refs, symbolToFile)
		})
		if err != nil {
			t.Fatalf("UpdateFileDeps failed: %v", err)
		}

		deps, err := tracker.GetDependencies("y.go")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}

		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies (unknown excluded), got %d", len(deps))
		}
	})
}

func TestDependencyTracker_RescanQueue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled: true,
		Mode:    InvalidationLazy,
		Depth:   1,
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	t.Run("enqueue and dequeue", func(t *testing.T) {
		// Enqueue some files
		if err := tracker.EnqueueRescan("file1.go", RescanDepChange, 1); err != nil {
			t.Fatalf("EnqueueRescan failed: %v", err)
		}
		if err := tracker.EnqueueRescan("file2.go", RescanDepChange, 1); err != nil {
			t.Fatalf("EnqueueRescan failed: %v", err)
		}

		count := tracker.GetPendingRescanCount()
		if count != 2 {
			t.Errorf("Expected 2 pending rescans, got %d", count)
		}

		// Get next (FIFO order)
		entry, err := tracker.GetNextRescan()
		if err != nil {
			t.Fatalf("GetNextRescan failed: %v", err)
		}
		if entry == nil {
			t.Fatal("Expected entry, got nil")
		}
		if entry.FilePath != "file1.go" {
			t.Errorf("Expected file1.go, got %s", entry.FilePath)
		}
		if entry.Reason != RescanDepChange {
			t.Errorf("Expected dep_change, got %s", entry.Reason)
		}
		if entry.Depth != 1 {
			t.Errorf("Expected depth 1, got %d", entry.Depth)
		}

		// Dequeue
		if err := tracker.DequeueRescan("file1.go"); err != nil {
			t.Fatalf("DequeueRescan failed: %v", err)
		}

		count = tracker.GetPendingRescanCount()
		if count != 1 {
			t.Errorf("Expected 1 pending rescan after dequeue, got %d", count)
		}
	})

	t.Run("idempotent enqueue", func(t *testing.T) {
		// Clear queue
		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		// Enqueue same file twice
		if err := tracker.EnqueueRescan("dup.go", RescanDepChange, 1); err != nil {
			t.Fatalf("EnqueueRescan failed: %v", err)
		}
		if err := tracker.EnqueueRescan("dup.go", RescanDepChange, 2); err != nil {
			t.Fatalf("EnqueueRescan (dup) failed: %v", err)
		}

		count := tracker.GetPendingRescanCount()
		if count != 1 {
			t.Errorf("Expected 1 (idempotent), got %d", count)
		}
	})

	t.Run("empty queue returns nil", func(t *testing.T) {
		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		entry, err := tracker.GetNextRescan()
		if err != nil {
			t.Fatalf("GetNextRescan failed: %v", err)
		}
		if entry != nil {
			t.Error("Expected nil for empty queue")
		}
	})
}

func TestDependencyTracker_InvalidateDependents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled: true,
		Mode:    InvalidationLazy, // Lazy mode for testing
		Depth:   2,
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Setup dependency chain: a.go -> b.go -> c.go
	err := db.WithTx(func(tx *sql.Tx) error {
		if err := tracker.UpdateFileDeps(tx, "a.go", []Reference{
			{ToSymbolID: "sym_b"},
		}, map[string]string{"sym_b": "b.go"}); err != nil {
			return err
		}
		return tracker.UpdateFileDeps(tx, "b.go", []Reference{
			{ToSymbolID: "sym_c"},
		}, map[string]string{"sym_c": "c.go"})
	})
	if err != nil {
		t.Fatalf("Setup deps failed: %v", err)
	}

	// Clear queue before test
	if err := tracker.ClearRescanQueue(); err != nil {
		t.Fatalf("ClearRescanQueue failed: %v", err)
	}

	t.Run("lazy mode enqueues direct dependents only", func(t *testing.T) {
		enqueued, err := tracker.InvalidateDependents([]string{"b.go"})
		if err != nil {
			t.Fatalf("InvalidateDependents failed: %v", err)
		}

		// In lazy mode, only direct dependents (a.go) should be enqueued
		if enqueued != 1 {
			t.Errorf("Expected 1 enqueued (lazy mode), got %d", enqueued)
		}

		count := tracker.GetPendingRescanCount()
		if count != 1 {
			t.Errorf("Expected 1 pending rescan, got %d", count)
		}
	})

	t.Run("disabled mode does nothing", func(t *testing.T) {
		disabledConfig := &TransitiveConfig{
			Enabled: false,
		}
		disabledTracker := NewDependencyTracker(db, store, disabledConfig, logger)

		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		enqueued, err := disabledTracker.InvalidateDependents([]string{"b.go"})
		if err != nil {
			t.Fatalf("InvalidateDependents failed: %v", err)
		}
		if enqueued != 0 {
			t.Errorf("Expected 0 (disabled), got %d", enqueued)
		}
	})

	t.Run("none mode does nothing", func(t *testing.T) {
		noneConfig := &TransitiveConfig{
			Enabled: true,
			Mode:    InvalidationNone,
		}
		noneTracker := NewDependencyTracker(db, store, noneConfig, logger)

		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		enqueued, err := noneTracker.InvalidateDependents([]string{"b.go"})
		if err != nil {
			t.Fatalf("InvalidateDependents failed: %v", err)
		}
		if enqueued != 0 {
			t.Errorf("Expected 0 (none mode), got %d", enqueued)
		}
	})
}

func TestDependencyTracker_DrainRescanQueue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled:        true,
		Mode:           InvalidationEager,
		Depth:          1,
		MaxRescanFiles: 2, // Low limit for testing
		MaxRescanMs:    0, // No time limit
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	t.Run("drain with file budget", func(t *testing.T) {
		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		// Enqueue 3 files
		for _, f := range []string{"f1.go", "f2.go", "f3.go"} {
			if err := tracker.EnqueueRescan(f, RescanDepChange, 1); err != nil {
				t.Fatalf("EnqueueRescan failed: %v", err)
			}
		}

		rescanCalls := 0
		result, err := tracker.DrainRescanQueue(func(filePath string) error {
			rescanCalls++
			return nil
		})
		if err != nil {
			t.Fatalf("DrainRescanQueue failed: %v", err)
		}

		// Should process 2 files (budget limit)
		if result.FilesProcessed != 2 {
			t.Errorf("Expected 2 files processed, got %d", result.FilesProcessed)
		}
		if !result.BudgetExceeded {
			t.Error("Expected BudgetExceeded = true")
		}
		if result.QueueDrained {
			t.Error("Expected QueueDrained = false")
		}

		// 1 file should remain
		remaining := tracker.GetPendingRescanCount()
		if remaining != 1 {
			t.Errorf("Expected 1 remaining, got %d", remaining)
		}
	})

	t.Run("drain empty queue", func(t *testing.T) {
		if err := tracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		result, err := tracker.DrainRescanQueue(func(filePath string) error {
			t.Error("Should not be called for empty queue")
			return nil
		})
		if err != nil {
			t.Fatalf("DrainRescanQueue failed: %v", err)
		}

		if result.FilesProcessed != 0 {
			t.Errorf("Expected 0 files processed, got %d", result.FilesProcessed)
		}
		if !result.QueueDrained {
			t.Error("Expected QueueDrained = true for empty queue")
		}
		if result.BudgetExceeded {
			t.Error("Expected BudgetExceeded = false for empty queue")
		}
	})

	t.Run("drain with time budget", func(t *testing.T) {
		timeConfig := &TransitiveConfig{
			Enabled:        true,
			Mode:           InvalidationEager,
			Depth:          1,
			MaxRescanFiles: 100, // High limit
			MaxRescanMs:    10,  // 10ms limit
		}
		timeTracker := NewDependencyTracker(db, store, timeConfig, logger)

		if err := timeTracker.ClearRescanQueue(); err != nil {
			t.Fatalf("ClearRescanQueue failed: %v", err)
		}

		// Enqueue several files
		for i := 0; i < 10; i++ {
			if err := timeTracker.EnqueueRescan(filepath.Join("slow", "file"+string(rune('0'+i))+".go"), RescanDepChange, 1); err != nil {
				t.Fatalf("EnqueueRescan failed: %v", err)
			}
		}

		result, err := timeTracker.DrainRescanQueue(func(filePath string) error {
			time.Sleep(5 * time.Millisecond) // Slow rescan
			return nil
		})
		if err != nil {
			t.Fatalf("DrainRescanQueue failed: %v", err)
		}

		// Should hit time budget before processing all
		if result.FilesProcessed >= 10 {
			t.Errorf("Expected less than 10 files processed due to time budget, got %d", result.FilesProcessed)
		}
		if !result.BudgetExceeded {
			t.Error("Expected BudgetExceeded = true")
		}
	})
}

func TestDependencyTracker_BuildSymbolToFileMap(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{Enabled: true}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Insert some file_symbols entries
	_, err := db.Exec(`INSERT INTO file_symbols (file_path, symbol_id) VALUES ('a.go', 'sym1')`)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	_, err = db.Exec(`INSERT INTO file_symbols (file_path, symbol_id) VALUES ('b.go', 'sym2')`)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	symbolToFile, err := tracker.BuildSymbolToFileMap()
	if err != nil {
		t.Fatalf("BuildSymbolToFileMap failed: %v", err)
	}

	if symbolToFile["sym1"] != "a.go" {
		t.Errorf("Expected sym1 -> a.go, got %s", symbolToFile["sym1"])
	}
	if symbolToFile["sym2"] != "b.go" {
		t.Errorf("Expected sym2 -> b.go, got %s", symbolToFile["sym2"])
	}
}

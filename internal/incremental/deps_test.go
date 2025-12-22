package incremental

import (
	"database/sql"
	"fmt"
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

func TestDependencyTracker_ClearFileDeps(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{Enabled: true, Mode: InvalidationLazy, Depth: 1}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Setup some dependencies
	err := db.WithTx(func(tx *sql.Tx) error {
		return tracker.UpdateFileDeps(tx, "a.go", []Reference{
			{ToSymbolID: "sym_b"},
		}, map[string]string{"sym_b": "b.go"})
	})
	if err != nil {
		t.Fatalf("Setup deps failed: %v", err)
	}

	// Verify deps exist
	deps, _ := tracker.GetDependencies("a.go")
	if len(deps) != 1 {
		t.Fatalf("Expected 1 dependency before clear, got %d", len(deps))
	}

	// Clear all deps
	if err := tracker.ClearFileDeps(); err != nil {
		t.Fatalf("ClearFileDeps failed: %v", err)
	}

	// Verify deps are gone
	deps, _ = tracker.GetDependencies("a.go")
	if len(deps) != 0 {
		t.Errorf("Expected 0 dependencies after clear, got %d", len(deps))
	}
}

func TestDependencyTracker_IncrementAttempts(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{Enabled: true, Mode: InvalidationLazy, Depth: 1}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Enqueue a file
	if err := tracker.EnqueueRescan("test.go", RescanDepChange, 1); err != nil {
		t.Fatalf("EnqueueRescan failed: %v", err)
	}

	// Check initial attempts
	entry, _ := tracker.GetNextRescan()
	if entry.Attempts != 0 {
		t.Errorf("Expected 0 attempts initially, got %d", entry.Attempts)
	}

	// Increment attempts
	if err := tracker.IncrementAttempts("test.go"); err != nil {
		t.Fatalf("IncrementAttempts failed: %v", err)
	}

	// Check incremented
	entry, _ = tracker.GetNextRescan()
	if entry.Attempts != 1 {
		t.Errorf("Expected 1 attempt after increment, got %d", entry.Attempts)
	}

	// Increment again
	if err := tracker.IncrementAttempts("test.go"); err != nil {
		t.Fatalf("IncrementAttempts (2nd) failed: %v", err)
	}

	entry, _ = tracker.GetNextRescan()
	if entry.Attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", entry.Attempts)
	}
}

func TestDependencyTracker_EagerModeWithDepth(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled: true,
		Mode:    InvalidationEager, // Eager mode cascades
		Depth:   3,
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Setup dependency chain: a.go -> b.go -> c.go -> d.go
	err := db.WithTx(func(tx *sql.Tx) error {
		if err := tracker.UpdateFileDeps(tx, "a.go", []Reference{
			{ToSymbolID: "sym_b"},
		}, map[string]string{"sym_b": "b.go"}); err != nil {
			return err
		}
		if err := tracker.UpdateFileDeps(tx, "b.go", []Reference{
			{ToSymbolID: "sym_c"},
		}, map[string]string{"sym_c": "c.go"}); err != nil {
			return err
		}
		return tracker.UpdateFileDeps(tx, "c.go", []Reference{
			{ToSymbolID: "sym_d"},
		}, map[string]string{"sym_d": "d.go"})
	})
	if err != nil {
		t.Fatalf("Setup deps failed: %v", err)
	}

	// Clear queue
	if err := tracker.ClearRescanQueue(); err != nil {
		t.Fatalf("ClearRescanQueue failed: %v", err)
	}

	// Invalidate from c.go - in eager mode with depth 3, should cascade
	// c.go changed -> b.go depends on c.go -> a.go depends on b.go
	enqueued, err := tracker.InvalidateDependents([]string{"c.go"})
	if err != nil {
		t.Fatalf("InvalidateDependents failed: %v", err)
	}

	// Should enqueue b.go (direct dependent of c.go), then a.go (transitive)
	if enqueued != 2 {
		t.Errorf("Expected 2 enqueued (eager cascade), got %d", enqueued)
	}

	count := tracker.GetPendingRescanCount()
	if count != 2 {
		t.Errorf("Expected 2 pending rescans, got %d", count)
	}
}

func TestDependencyTracker_DrainWithRescanError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled:        true,
		Mode:           InvalidationEager,
		Depth:          1,
		MaxRescanFiles: 10,
		MaxRescanMs:    0,
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Clear and enqueue files
	if err := tracker.ClearRescanQueue(); err != nil {
		t.Fatalf("ClearRescanQueue failed: %v", err)
	}

	for _, f := range []string{"good1.go", "bad.go", "good2.go"} {
		if err := tracker.EnqueueRescan(f, RescanDepChange, 1); err != nil {
			t.Fatalf("EnqueueRescan failed: %v", err)
		}
	}

	// Rescan function that fails for bad.go
	callCount := 0
	result, err := tracker.DrainRescanQueue(func(filePath string) error {
		callCount++
		if filePath == "bad.go" {
			return fmt.Errorf("simulated rescan failure")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DrainRescanQueue failed: %v", err)
	}

	// Should have processed 2 successfully (good1.go, good2.go)
	if result.FilesProcessed != 2 {
		t.Errorf("Expected 2 files processed, got %d", result.FilesProcessed)
	}

	// bad.go should still be in queue with incremented attempts
	remaining := tracker.GetPendingRescanCount()
	if remaining != 1 {
		t.Errorf("Expected 1 remaining (bad.go), got %d", remaining)
	}

	entry, _ := tracker.GetNextRescan()
	if entry.FilePath != "bad.go" {
		t.Errorf("Expected bad.go in queue, got %s", entry.FilePath)
	}
	if entry.Attempts != 1 {
		t.Errorf("Expected 1 attempt for bad.go, got %d", entry.Attempts)
	}
}

func TestDependencyTracker_MultipleChangedFiles(t *testing.T) {
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

	// Setup: x.go -> a.go, y.go -> a.go, z.go -> b.go
	err := db.WithTx(func(tx *sql.Tx) error {
		if err := tracker.UpdateFileDeps(tx, "x.go", []Reference{
			{ToSymbolID: "sym_a"},
		}, map[string]string{"sym_a": "a.go"}); err != nil {
			return err
		}
		if err := tracker.UpdateFileDeps(tx, "y.go", []Reference{
			{ToSymbolID: "sym_a"},
		}, map[string]string{"sym_a": "a.go"}); err != nil {
			return err
		}
		return tracker.UpdateFileDeps(tx, "z.go", []Reference{
			{ToSymbolID: "sym_b"},
		}, map[string]string{"sym_b": "b.go"})
	})
	if err != nil {
		t.Fatalf("Setup deps failed: %v", err)
	}

	// Clear queue
	if err := tracker.ClearRescanQueue(); err != nil {
		t.Fatalf("ClearRescanQueue failed: %v", err)
	}

	// Change both a.go and b.go
	enqueued, err := tracker.InvalidateDependents([]string{"a.go", "b.go"})
	if err != nil {
		t.Fatalf("InvalidateDependents failed: %v", err)
	}

	// Should enqueue x.go, y.go (depend on a.go), z.go (depends on b.go)
	if enqueued != 3 {
		t.Errorf("Expected 3 enqueued, got %d", enqueued)
	}

	count := tracker.GetPendingRescanCount()
	if count != 3 {
		t.Errorf("Expected 3 pending rescans, got %d", count)
	}
}

func TestDependencyTracker_DepthLimitRespected(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)
	config := &TransitiveConfig{
		Enabled: true,
		Mode:    InvalidationEager,
		Depth:   1, // Only direct dependents
	}
	tracker := NewDependencyTracker(db, store, config, logger)

	// Setup: a.go -> b.go -> c.go
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

	// Clear queue
	if err := tracker.ClearRescanQueue(); err != nil {
		t.Fatalf("ClearRescanQueue failed: %v", err)
	}

	// Change c.go with depth limit 1
	enqueued, err := tracker.InvalidateDependents([]string{"c.go"})
	if err != nil {
		t.Fatalf("InvalidateDependents failed: %v", err)
	}

	// Should only enqueue b.go (direct dependent), NOT a.go (transitive)
	if enqueued != 1 {
		t.Errorf("Expected 1 enqueued (depth limit), got %d", enqueued)
	}

	entry, _ := tracker.GetNextRescan()
	if entry.FilePath != "b.go" {
		t.Errorf("Expected b.go, got %s", entry.FilePath)
	}
}

func TestDependencyTracker_NilConfig(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	store := NewStore(db, logger)

	// Create tracker with nil config - should use defaults
	tracker := NewDependencyTracker(db, store, nil, logger)

	// Should not panic and should use default config
	if tracker == nil {
		t.Fatal("Expected non-nil tracker")
	}

	// Verify it works with default config
	if err := tracker.EnqueueRescan("test.go", RescanDepChange, 1); err != nil {
		t.Fatalf("EnqueueRescan with nil config failed: %v", err)
	}

	count := tracker.GetPendingRescanCount()
	if count != 1 {
		t.Errorf("Expected 1 pending, got %d", count)
	}
}

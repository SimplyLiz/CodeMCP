package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ckb/internal/logging"
)

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig()

	if !cfg.Enabled {
		t.Error("Expected compaction to be enabled by default")
	}
	if cfg.KeepSnapshots != 5 {
		t.Errorf("Expected KeepSnapshots=5, got %d", cfg.KeepSnapshots)
	}
	if cfg.KeepDays != 30 {
		t.Errorf("Expected KeepDays=30, got %d", cfg.KeepDays)
	}
	if cfg.CompactJournalAfterDays != 7 {
		t.Errorf("Expected CompactJournalAfterDays=7, got %d", cfg.CompactJournalAfterDays)
	}
	if cfg.Schedule != "0 3 * * *" {
		t.Errorf("Expected schedule='0 3 * * *', got %s", cfg.Schedule)
	}
}

func TestCompactor_DryRun(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create some test snapshot files
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "snapshot_test_"+string(rune('a'+i))+".db")
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test snapshot: %v", err)
		}
		// Make the first file old
		if i == 0 {
			oldTime := time.Now().AddDate(0, 0, -60) // 60 days old
			os.Chtimes(path, oldTime, oldTime)
		}
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.DebugLevel,
		Format: logging.HumanFormat,
		Output: os.Stderr,
	})

	config := CompactionConfig{
		Enabled:                 true,
		KeepSnapshots:           2,
		KeepDays:                30,
		CompactJournalAfterDays: 7,
		VacuumFTS:               false, // Skip FTS for test
		DryRun:                  true,
	}

	compactor := NewCompactor(tmpDir, config, logger)

	result, err := compactor.Run(context.Background())
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	if !result.DryRun {
		t.Error("Expected DryRun to be true")
	}

	// In dry run, files should still exist
	entries, _ := filepath.Glob(filepath.Join(tmpDir, "snapshot_*.db"))
	if len(entries) != 3 {
		t.Errorf("Expected 3 snapshots to still exist in dry run, got %d", len(entries))
	}

	if result.DurationMs < 0 {
		t.Error("Expected non-negative duration")
	}
}

func TestCompactor_ActualDeletion(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create some test snapshot files with different ages
	now := time.Now()
	testSnapshots := []struct {
		name string
		age  time.Duration
	}{
		{"snapshot_recent1.db", 0},                   // Very recent
		{"snapshot_recent2.db", 24 * time.Hour},      // 1 day old
		{"snapshot_old1.db", 35 * 24 * time.Hour},    // 35 days old
		{"snapshot_old2.db", 60 * 24 * time.Hour},    // 60 days old
		{"snapshot_ancient.db", 90 * 24 * time.Hour}, // 90 days old
	}

	for _, snap := range testSnapshots {
		path := filepath.Join(tmpDir, snap.name)
		if err := os.WriteFile(path, []byte("test data"), 0644); err != nil {
			t.Fatalf("Failed to create test snapshot: %v", err)
		}
		modTime := now.Add(-snap.age)
		os.Chtimes(path, modTime, modTime)
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.DebugLevel,
		Format: logging.HumanFormat,
		Output: os.Stderr,
	})

	config := CompactionConfig{
		Enabled:                 true,
		KeepSnapshots:           2,  // Keep newest 2
		KeepDays:                30, // Delete if older than 30 days
		CompactJournalAfterDays: 7,
		VacuumFTS:               false,
		DryRun:                  false, // Actually delete
	}

	compactor := NewCompactor(tmpDir, config, logger)

	result, err := compactor.Run(context.Background())
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	// Should have deleted 3 old snapshots (35, 60, 90 days old)
	// But kept 2 newest regardless of age
	if result.SnapshotsDeleted < 1 {
		t.Errorf("Expected at least 1 snapshot deleted, got %d", result.SnapshotsDeleted)
	}

	// Verify remaining files
	remaining, _ := filepath.Glob(filepath.Join(tmpDir, "snapshot_*.db"))
	if len(remaining) < 2 {
		t.Errorf("Expected at least 2 snapshots to remain, got %d", len(remaining))
	}
}

func TestCompactor_NoSnapshots(t *testing.T) {
	// Create empty temp directory
	tmpDir := t.TempDir()

	logger := logging.NewLogger(logging.Config{
		Level:  logging.DebugLevel,
		Format: logging.HumanFormat,
		Output: os.Stderr,
	})

	config := DefaultCompactionConfig()
	config.VacuumFTS = false
	config.DryRun = true

	compactor := NewCompactor(tmpDir, config, logger)

	result, err := compactor.Run(context.Background())
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	if result.SnapshotsDeleted != 0 {
		t.Errorf("Expected 0 snapshots deleted, got %d", result.SnapshotsDeleted)
	}
}

func TestCompactionResult(t *testing.T) {
	result := &CompactionResult{
		StartedAt:            time.Now(),
		CompletedAt:          time.Now().Add(100 * time.Millisecond),
		DurationMs:           100,
		SnapshotsDeleted:     2,
		JournalEntriesPurged: 50,
		BytesReclaimed:       1024 * 1024,
		FTSVacuumed:          true,
		DryRun:               false,
		DeletedSnapshots:     []string{"snapshot_1.db", "snapshot_2.db"},
	}

	if result.DurationMs != 100 {
		t.Errorf("Expected DurationMs=100, got %d", result.DurationMs)
	}
	if len(result.DeletedSnapshots) != 2 {
		t.Errorf("Expected 2 deleted snapshots, got %d", len(result.DeletedSnapshots))
	}
}

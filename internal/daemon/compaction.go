// Package daemon provides compaction for database maintenance.
package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ckb/internal/logging"
	"ckb/internal/scheduler"
)

// CompactionConfig contains compaction settings
type CompactionConfig struct {
	// Enabled enables compaction
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// KeepSnapshots is the number of snapshots to keep
	KeepSnapshots int `json:"keepSnapshots" mapstructure:"keep_snapshots"`
	// KeepDays is the minimum age in days to keep snapshots
	KeepDays int `json:"keepDays" mapstructure:"keep_days"`
	// CompactJournalAfterDays is when to prune change journal entries
	CompactJournalAfterDays int `json:"compactJournalAfterDays" mapstructure:"compact_journal_after_days"`
	// Schedule is the cron expression for compaction
	Schedule string `json:"schedule" mapstructure:"schedule"`
	// VacuumFTS enables FTS table vacuuming
	VacuumFTS bool `json:"vacuumFts" mapstructure:"vacuum_fts"`
	// DryRun performs a dry run without deleting
	DryRun bool `json:"dryRun" mapstructure:"dry_run"`
}

// DefaultCompactionConfig returns default compaction settings
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Enabled:                 true,
		KeepSnapshots:           5,
		KeepDays:                30,
		CompactJournalAfterDays: 7,
		Schedule:                "0 3 * * *", // 3 AM daily
		VacuumFTS:               true,
		DryRun:                  false,
	}
}

// CompactionResult contains the results of a compaction run
type CompactionResult struct {
	StartedAt            time.Time `json:"startedAt"`
	CompletedAt          time.Time `json:"completedAt"`
	DurationMs           int64     `json:"durationMs"`
	SnapshotsDeleted     int       `json:"snapshotsDeleted"`
	JournalEntriesPurged int       `json:"journalEntriesPurged"`
	BytesReclaimed       int64     `json:"bytesReclaimed"`
	FTSVacuumed          bool      `json:"ftsVacuumed"`
	Errors               []string  `json:"errors,omitempty"`
	DryRun               bool      `json:"dryRun"`
	DeletedSnapshots     []string  `json:"deletedSnapshots,omitempty"`
}

// Compactor handles database compaction operations
type Compactor struct {
	config CompactionConfig
	logger *logging.Logger
	ckbDir string
}

// NewCompactor creates a new compactor
func NewCompactor(ckbDir string, config CompactionConfig, logger *logging.Logger) *Compactor {
	return &Compactor{
		config: config,
		logger: logger,
		ckbDir: ckbDir,
	}
}

// Run performs compaction according to configuration
func (c *Compactor) Run(ctx context.Context) (*CompactionResult, error) {
	result := &CompactionResult{
		StartedAt: time.Now(),
		DryRun:    c.config.DryRun,
	}

	c.logger.Info("Starting compaction", map[string]interface{}{
		"dryRun":        c.config.DryRun,
		"keepSnapshots": c.config.KeepSnapshots,
		"keepDays":      c.config.KeepDays,
	})

	// Step 1: Delete old snapshots
	deleted, snapshots, err := c.deleteOldSnapshots(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("snapshot deletion: %v", err))
	}
	result.SnapshotsDeleted = deleted
	result.DeletedSnapshots = snapshots

	// Step 2: Prune change journal
	pruned, err := c.pruneChangeJournal(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("journal pruning: %v", err))
	}
	result.JournalEntriesPurged = pruned

	// Step 3: Vacuum FTS tables
	if c.config.VacuumFTS {
		if err := c.vacuumFTS(ctx); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("FTS vacuum: %v", err))
		} else {
			result.FTSVacuumed = true
		}
	}

	// Step 4: Calculate reclaimed space
	result.BytesReclaimed = c.calculateReclaimedSpace()

	result.CompletedAt = time.Now()
	result.DurationMs = result.CompletedAt.Sub(result.StartedAt).Milliseconds()

	c.logger.Info("Compaction completed", map[string]interface{}{
		"durationMs":       result.DurationMs,
		"snapshotsDeleted": result.SnapshotsDeleted,
		"journalPurged":    result.JournalEntriesPurged,
		"bytesReclaimed":   result.BytesReclaimed,
		"errors":           len(result.Errors),
	})

	return result, nil
}

// deleteOldSnapshots removes snapshot databases older than the retention policy
func (c *Compactor) deleteOldSnapshots(ctx context.Context) (int, []string, error) {
	// Find snapshot databases (*.db files in the ckb directory)
	pattern := filepath.Join(c.ckbDir, "snapshot_*.db")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(matches) == 0 {
		return 0, nil, nil
	}

	// Sort by modification time (newest first)
	type snapshotInfo struct {
		path    string
		modTime time.Time
	}

	snapshots := make([]snapshotInfo, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		snapshots = append(snapshots, snapshotInfo{path: path, modTime: info.ModTime()})
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].modTime.After(snapshots[j].modTime)
	})

	// Determine which to delete
	cutoffTime := time.Now().AddDate(0, 0, -c.config.KeepDays)
	var toDelete []snapshotInfo

	for i, snap := range snapshots {
		// Keep the minimum number of snapshots regardless of age
		if i < c.config.KeepSnapshots {
			continue
		}

		// Delete if older than cutoff
		if snap.modTime.Before(cutoffTime) {
			toDelete = append(toDelete, snap)
		}
	}

	if len(toDelete) == 0 {
		return 0, nil, nil
	}

	deleted := 0
	var deletedNames []string

	for _, snap := range toDelete {
		if c.config.DryRun {
			c.logger.Info("Would delete snapshot (dry-run)", map[string]interface{}{
				"path": snap.path,
				"age":  time.Since(snap.modTime).String(),
			})
			deleted++
			deletedNames = append(deletedNames, filepath.Base(snap.path))
			continue
		}

		// Delete the snapshot and any associated files
		if err := c.deleteSnapshotFiles(snap.path); err != nil {
			c.logger.Error("Failed to delete snapshot", map[string]interface{}{
				"path":  snap.path,
				"error": err.Error(),
			})
			continue
		}

		deleted++
		deletedNames = append(deletedNames, filepath.Base(snap.path))
		c.logger.Info("Deleted snapshot", map[string]interface{}{
			"path": snap.path,
			"age":  time.Since(snap.modTime).String(),
		})
	}

	return deleted, deletedNames, nil
}

// deleteSnapshotFiles deletes a snapshot database and its associated files
func (c *Compactor) deleteSnapshotFiles(dbPath string) error {
	// Delete main database
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Delete WAL file if exists
	walPath := dbPath + "-wal"
	if err := os.Remove(walPath); err != nil && !os.IsNotExist(err) {
		// Non-fatal, just log
		c.logger.Debug("Could not delete WAL file", map[string]interface{}{
			"path": walPath,
		})
	}

	// Delete SHM file if exists
	shmPath := dbPath + "-shm"
	if err := os.Remove(shmPath); err != nil && !os.IsNotExist(err) {
		// Non-fatal, just log
		c.logger.Debug("Could not delete SHM file", map[string]interface{}{
			"path": shmPath,
		})
	}

	return nil
}

// pruneChangeJournal removes old change journal entries
func (c *Compactor) pruneChangeJournal(ctx context.Context) (int, error) {
	// Open the main database
	dbPath := filepath.Join(c.ckbDir, "ckb.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, nil // No database yet
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Check if change_journal table exists
	var tableName string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='change_journal'").Scan(&tableName)
	if err == sql.ErrNoRows {
		return 0, nil // Table doesn't exist
	}
	if err != nil {
		return 0, fmt.Errorf("failed to check for change_journal table: %w", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -c.config.CompactJournalAfterDays)
	cutoffStr := cutoffTime.Format(time.RFC3339)

	if c.config.DryRun {
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM change_journal WHERE timestamp < ?", cutoffStr).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count journal entries: %w", err)
		}
		return count, nil
	}

	result, err := db.ExecContext(ctx, "DELETE FROM change_journal WHERE timestamp < ?", cutoffStr)
	if err != nil {
		return 0, fmt.Errorf("failed to prune journal: %w", err)
	}

	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// vacuumFTS optimizes FTS tables
func (c *Compactor) vacuumFTS(ctx context.Context) error {
	dbPath := filepath.Join(c.ckbDir, "ckb.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if c.config.DryRun {
		c.logger.Info("Would vacuum FTS tables (dry-run)", nil)
		return nil
	}

	// Check if FTS table exists
	var tableName string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='symbols_fts'").Scan(&tableName)
	if err == sql.ErrNoRows {
		return nil // FTS not initialized
	}
	if err != nil {
		return fmt.Errorf("failed to check for FTS table: %w", err)
	}

	// Optimize FTS index
	_, err = db.ExecContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES('optimize')")
	if err != nil {
		return fmt.Errorf("failed to optimize FTS: %w", err)
	}

	c.logger.Info("FTS tables optimized", nil)
	return nil
}

// calculateReclaimedSpace estimates reclaimed disk space
func (c *Compactor) calculateReclaimedSpace() int64 {
	// This is a rough estimate based on typical database sizes
	// In a real implementation, you'd track the actual sizes before/after
	return 0 // Placeholder
}

// RegisterCompactionHandler registers the compaction handler with the scheduler
func RegisterCompactionHandler(sched *scheduler.Scheduler, compactor *Compactor) {
	sched.RegisterHandler(scheduler.TaskTypeCleanup, func(ctx context.Context, schedule *scheduler.Schedule) error {
		result, err := compactor.Run(ctx)
		if err != nil {
			return err
		}

		if len(result.Errors) > 0 {
			return fmt.Errorf("compaction completed with errors: %s", strings.Join(result.Errors, "; "))
		}

		return nil
	})
}

// CreateDefaultCompactionSchedule creates the default compaction schedule
func CreateDefaultCompactionSchedule(config CompactionConfig) (*scheduler.Schedule, error) {
	if !config.Enabled {
		return nil, nil
	}

	return scheduler.NewSchedule(
		scheduler.TaskTypeCleanup,
		"", // No specific target for compaction
		config.Schedule,
	)
}

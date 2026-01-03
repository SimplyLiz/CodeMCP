package incremental

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"ckb/internal/storage"
)

// DependencyTracker manages file-level dependencies and transitive invalidation
type DependencyTracker struct {
	db     *storage.DB
	store  *Store
	config *TransitiveConfig
	logger *slog.Logger
}

// NewDependencyTracker creates a new dependency tracker
func NewDependencyTracker(db *storage.DB, store *Store, config *TransitiveConfig, logger *slog.Logger) *DependencyTracker {
	if config == nil {
		defaultCfg := DefaultConfig()
		config = &defaultCfg.Transitive
	}
	return &DependencyTracker{
		db:     db,
		store:  store,
		config: config,
		logger: logger,
	}
}

// ============================================================================
// File Dependency Operations
// ============================================================================

// UpdateFileDeps updates file_deps for a changed file based on its references
// definingFiles maps referenced symbol IDs to their defining file paths
// Only stores dependencies to internal files (not external/stdlib)
func (t *DependencyTracker) UpdateFileDeps(tx *sql.Tx, dependentFile string, refs []Reference, symbolToFile map[string]string) error {
	// Delete old deps for this file
	if _, err := tx.Exec(`DELETE FROM file_deps WHERE dependent_file = ?`, dependentFile); err != nil {
		return fmt.Errorf("delete old file_deps: %w", err)
	}

	// Collect unique defining files
	definingFiles := make(map[string]bool)
	for _, ref := range refs {
		if defFile, ok := symbolToFile[ref.ToSymbolID]; ok {
			// Skip self-references
			if defFile != dependentFile {
				definingFiles[defFile] = true
			}
		}
		// Skip if symbol not found - likely external/stdlib
	}

	if len(definingFiles) == 0 {
		return nil
	}

	// Insert new deps
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO file_deps (dependent_file, defining_file) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare file_deps insert: %w", err)
	}
	defer stmt.Close() //nolint:errcheck

	for defFile := range definingFiles {
		if _, err := stmt.Exec(dependentFile, defFile); err != nil {
			return fmt.Errorf("insert file_dep: %w", err)
		}
	}

	return nil
}

// GetDependents returns all files that depend on the given file
func (t *DependencyTracker) GetDependents(definingFile string) ([]string, error) {
	rows, err := t.db.Query(`
		SELECT dependent_file FROM file_deps WHERE defining_file = ?
	`, definingFile)
	if err != nil {
		return nil, fmt.Errorf("query dependents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var dependents []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, fmt.Errorf("scan dependent: %w", err)
		}
		dependents = append(dependents, dep)
	}
	return dependents, rows.Err()
}

// GetDependencies returns all files that the given file depends on
func (t *DependencyTracker) GetDependencies(dependentFile string) ([]string, error) {
	rows, err := t.db.Query(`
		SELECT defining_file FROM file_deps WHERE dependent_file = ?
	`, dependentFile)
	if err != nil {
		return nil, fmt.Errorf("query dependencies: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deps []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

// ClearFileDeps removes all file dependencies (for full reindex)
func (t *DependencyTracker) ClearFileDeps() error {
	_, err := t.db.Exec(`DELETE FROM file_deps`)
	if err != nil {
		return fmt.Errorf("clear file_deps: %w", err)
	}
	return nil
}

// ============================================================================
// Rescan Queue Operations
// ============================================================================

// EnqueueRescan adds a file to the rescan queue (idempotent via PK)
func (t *DependencyTracker) EnqueueRescan(filePath string, reason RescanReason, depth int) error {
	_, err := t.db.Exec(`
		INSERT OR IGNORE INTO rescan_queue (file_path, reason, depth, enqueued_at, attempts)
		VALUES (?, ?, ?, ?, 0)
	`, filePath, string(reason), depth, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("enqueue rescan: %w", err)
	}
	return nil
}

// DequeueRescan removes a file from the rescan queue
func (t *DependencyTracker) DequeueRescan(filePath string) error {
	_, err := t.db.Exec(`DELETE FROM rescan_queue WHERE file_path = ?`, filePath)
	if err != nil {
		return fmt.Errorf("dequeue rescan: %w", err)
	}
	return nil
}

// GetNextRescan returns the next file to rescan (FIFO by enqueue time, then depth)
func (t *DependencyTracker) GetNextRescan() (*RescanQueueEntry, error) {
	row := t.db.QueryRow(`
		SELECT file_path, reason, depth, enqueued_at, attempts
		FROM rescan_queue
		ORDER BY enqueued_at, depth
		LIMIT 1
	`)

	var entry RescanQueueEntry
	var reason string
	var enqueuedAt int64

	err := row.Scan(&entry.FilePath, &reason, &entry.Depth, &enqueuedAt, &entry.Attempts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get next rescan: %w", err)
	}

	entry.Reason = RescanReason(reason)
	entry.EnqueuedAt = time.Unix(enqueuedAt, 0)
	return &entry, nil
}

// GetPendingRescanCount returns the number of files in the rescan queue
func (t *DependencyTracker) GetPendingRescanCount() int {
	var count int
	row := t.db.QueryRow(`SELECT COUNT(*) FROM rescan_queue`)
	if err := row.Scan(&count); err != nil {
		return 0
	}
	return count
}

// ClearRescanQueue removes all entries from the rescan queue
func (t *DependencyTracker) ClearRescanQueue() error {
	_, err := t.db.Exec(`DELETE FROM rescan_queue`)
	if err != nil {
		return fmt.Errorf("clear rescan_queue: %w", err)
	}
	return nil
}

// IncrementAttempts increments the attempt counter for a queued file
func (t *DependencyTracker) IncrementAttempts(filePath string) error {
	_, err := t.db.Exec(`
		UPDATE rescan_queue SET attempts = attempts + 1 WHERE file_path = ?
	`, filePath)
	if err != nil {
		return fmt.Errorf("increment attempts: %w", err)
	}
	return nil
}

// ============================================================================
// Transitive Invalidation
// ============================================================================

// InvalidateDependents enqueues files that depend on changedFiles for rescanning
// Uses BFS with depth limit from config
func (t *DependencyTracker) InvalidateDependents(changedFiles []string) (int, error) {
	if !t.config.Enabled || t.config.Mode == InvalidationNone {
		return 0, nil
	}

	enqueued := 0

	// BFS: process each changed file and enqueue its dependents
	for _, changedFile := range changedFiles {
		count, err := t.invalidateRecursive(changedFile, 1)
		if err != nil {
			return enqueued, err
		}
		enqueued += count
	}

	t.logger.Info("Enqueued dependents for rescan", map[string]interface{}{
		"changedFiles": len(changedFiles),
		"enqueued":     enqueued,
	})

	return enqueued, nil
}

// invalidateRecursive enqueues dependents at a given depth
func (t *DependencyTracker) invalidateRecursive(changedFile string, depth int) (int, error) {
	if depth > t.config.Depth {
		return 0, nil
	}

	dependents, err := t.GetDependents(changedFile)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, dep := range dependents {
		if err := t.EnqueueRescan(dep, RescanDepChange, depth); err != nil {
			return enqueued, err
		}
		enqueued++

		// Recurse for cascade (BFS level by level)
		if t.config.Mode == InvalidationEager {
			childCount, err := t.invalidateRecursive(dep, depth+1)
			if err != nil {
				return enqueued, err
			}
			enqueued += childCount
		}
	}

	return enqueued, nil
}

// DrainRescanQueue processes the rescan queue with budget limits
// Returns the number of files processed and whether the queue is fully drained
type DrainResult struct {
	FilesProcessed int
	QueueDrained   bool
	BudgetExceeded bool
	Duration       time.Duration
}

// DrainRescanQueue processes pending rescans with budget limits
// rescanFunc is called for each file to actually perform the rescan
func (t *DependencyTracker) DrainRescanQueue(rescanFunc func(filePath string) error) (*DrainResult, error) {
	if !t.config.Enabled || t.config.Mode == InvalidationNone {
		return &DrainResult{QueueDrained: true}, nil
	}

	result := &DrainResult{}
	startTime := time.Now()

	// Track files that failed during this drain run to avoid infinite loops
	failedThisRun := make(map[string]bool)

	for {
		// Check file budget
		if t.config.MaxRescanFiles > 0 && result.FilesProcessed >= t.config.MaxRescanFiles {
			result.BudgetExceeded = true
			t.logger.Info("Rescan file budget exceeded", map[string]interface{}{
				"processed": result.FilesProcessed,
				"budget":    t.config.MaxRescanFiles,
			})
			break
		}

		// Check time budget
		if t.config.MaxRescanMs > 0 {
			elapsed := time.Since(startTime)
			if elapsed.Milliseconds() >= int64(t.config.MaxRescanMs) {
				result.BudgetExceeded = true
				t.logger.Info("Rescan time budget exceeded", map[string]interface{}{
					"elapsed":  elapsed.String(),
					"budgetMs": t.config.MaxRescanMs,
				})
				break
			}
		}

		// Get next file to rescan (skip files that already failed this run)
		entry, err := t.getNextRescanExcluding(failedThisRun)
		if err != nil {
			return result, fmt.Errorf("get next rescan: %w", err)
		}

		if entry == nil {
			// Queue is drained (or all remaining files failed this run)
			result.QueueDrained = len(failedThisRun) == 0 || t.GetPendingRescanCount() == len(failedThisRun)
			break
		}

		// Rescan the file
		if err := rescanFunc(entry.FilePath); err != nil {
			// Log error but continue with other files
			t.logger.Warn("Rescan failed", map[string]interface{}{
				"file":  entry.FilePath,
				"error": err.Error(),
			})
			// Mark as failed this run to skip in subsequent iterations
			failedThisRun[entry.FilePath] = true
			// Increment attempts instead of removing
			if err := t.IncrementAttempts(entry.FilePath); err != nil {
				t.logger.Warn("Failed to increment attempts", map[string]interface{}{
					"file":  entry.FilePath,
					"error": err.Error(),
				})
			}
			continue
		}

		// Remove from queue on success
		if err := t.DequeueRescan(entry.FilePath); err != nil {
			return result, fmt.Errorf("dequeue after rescan: %w", err)
		}

		result.FilesProcessed++
	}

	result.Duration = time.Since(startTime)

	t.logger.Info("Rescan queue drain complete", map[string]interface{}{
		"processed":      result.FilesProcessed,
		"queueDrained":   result.QueueDrained,
		"budgetExceeded": result.BudgetExceeded,
		"duration":       result.Duration.String(),
	})

	return result, nil
}

// getNextRescanExcluding returns the next file to rescan, excluding specified files
func (t *DependencyTracker) getNextRescanExcluding(exclude map[string]bool) (*RescanQueueEntry, error) {
	rows, err := t.db.Query(`
		SELECT file_path, reason, depth, enqueued_at, attempts
		FROM rescan_queue
		ORDER BY enqueued_at, depth
	`)
	if err != nil {
		return nil, fmt.Errorf("query rescan queue: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var entry RescanQueueEntry
		var reason string
		var enqueuedAt int64

		if err := rows.Scan(&entry.FilePath, &reason, &entry.Depth, &enqueuedAt, &entry.Attempts); err != nil {
			return nil, fmt.Errorf("scan rescan entry: %w", err)
		}

		// Skip excluded files
		if exclude[entry.FilePath] {
			continue
		}

		entry.Reason = RescanReason(reason)
		entry.EnqueuedAt = time.Unix(enqueuedAt, 0)
		return &entry, nil
	}

	return nil, rows.Err()
}

// ============================================================================
// Symbol-to-File Resolution
// ============================================================================

// BuildSymbolToFileMap builds a map from symbol IDs to their defining file paths
// This is used to populate file_deps from references
func (t *DependencyTracker) BuildSymbolToFileMap() (map[string]string, error) {
	rows, err := t.db.Query(`SELECT symbol_id, file_path FROM file_symbols`)
	if err != nil {
		return nil, fmt.Errorf("query file_symbols: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	symbolToFile := make(map[string]string)
	for rows.Next() {
		var symbolID, filePath string
		if err := rows.Scan(&symbolID, &filePath); err != nil {
			return nil, fmt.Errorf("scan file_symbol: %w", err)
		}
		symbolToFile[symbolID] = filePath
	}

	return symbolToFile, rows.Err()
}

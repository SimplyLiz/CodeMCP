package incremental

import (
	"database/sql"
	"fmt"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

// IndexUpdater applies incremental updates to the database
type IndexUpdater struct {
	db         *storage.DB
	store      *Store
	depTracker *DependencyTracker
	config     *Config
	logger     *logging.Logger
}

// NewIndexUpdater creates a new incremental updater
func NewIndexUpdater(db *storage.DB, store *Store, logger *logging.Logger) *IndexUpdater {
	config := DefaultConfig()
	return &IndexUpdater{
		db:         db,
		store:      store,
		depTracker: NewDependencyTracker(db, store, &config.Transitive, logger),
		config:     config,
		logger:     logger,
	}
}

// SetConfig updates the configuration
func (u *IndexUpdater) SetConfig(config *Config) {
	u.config = config
	u.depTracker = NewDependencyTracker(u.db, u.store, &config.Transitive, u.logger)
}

// ApplyDelta applies symbol changes to the database
// V1.1 updates: indexed_files, file_symbols, callgraph
// V2.0 updates: file_deps for transitive invalidation
func (u *IndexUpdater) ApplyDelta(delta *SymbolDelta) error {
	// Build symbol-to-file map for dependency tracking
	symbolToFile, err := u.depTracker.BuildSymbolToFileMap()
	if err != nil {
		u.logger.Warn("Failed to build symbol-to-file map", map[string]interface{}{
			"error": err.Error(),
		})
		symbolToFile = make(map[string]string) // Continue with empty map
	}

	// Also add symbols from this delta (they may not be in DB yet)
	for _, fd := range delta.FileDeltas {
		for _, sym := range fd.Symbols {
			symbolToFile[sym.ID] = fd.Path
		}
	}

	return u.db.WithTx(func(tx *sql.Tx) error {
		for _, fileDelta := range delta.FileDeltas {
			if err := u.applyFileDelta(tx, fileDelta, symbolToFile); err != nil {
				return fmt.Errorf("failed to update %s: %w", fileDelta.Path, err)
			}
		}
		return nil
	})
}

// ApplyDeltaWithInvalidation applies delta and triggers transitive invalidation
// Returns the list of files that were enqueued for rescanning
func (u *IndexUpdater) ApplyDeltaWithInvalidation(delta *SymbolDelta) (int, error) {
	// Apply the delta first
	if err := u.ApplyDelta(delta); err != nil {
		return 0, err
	}

	// Collect changed files for invalidation
	var changedFiles []string
	for _, fd := range delta.FileDeltas {
		if fd.ChangeType != ChangeDeleted {
			changedFiles = append(changedFiles, fd.Path)
		}
	}

	// Trigger transitive invalidation
	enqueued, err := u.depTracker.InvalidateDependents(changedFiles)
	if err != nil {
		return enqueued, fmt.Errorf("invalidate dependents: %w", err)
	}

	return enqueued, nil
}

// applyFileDelta applies changes for a single file
// CRITICAL: Uses OldPath for deletions to handle renames correctly
// V2.0: symbolToFile maps symbols to their defining files for dependency tracking
func (u *IndexUpdater) applyFileDelta(tx *sql.Tx, delta FileDelta, symbolToFile map[string]string) error {
	switch delta.ChangeType {
	case ChangeDeleted:
		// Delete everything for this file
		return u.deleteFileData(tx, delta.Path)

	case ChangeAdded:
		// Just insert new data
		return u.insertFileData(tx, delta, symbolToFile)

	case ChangeModified:
		// Delete old data, insert new
		if err := u.deleteFileData(tx, delta.Path); err != nil {
			return err
		}
		return u.insertFileData(tx, delta, symbolToFile)

	case ChangeRenamed:
		// CRITICAL: Delete using OldPath, insert using Path
		if delta.OldPath == "" {
			return fmt.Errorf("rename without OldPath for %s", delta.Path)
		}
		if err := u.deleteFileData(tx, delta.OldPath); err != nil {
			return err
		}
		return u.insertFileData(tx, delta, symbolToFile)
	}

	return nil
}

// deleteFileData removes all data owned by a file
// This includes: file_symbols mapping, indexed_files entry, callgraph edges, and file_deps
func (u *IndexUpdater) deleteFileData(tx *sql.Tx, path string) error {
	// 1. Delete file_symbols mapping for this file
	_, err := tx.Exec(`DELETE FROM file_symbols WHERE file_path = ?`, path)
	if err != nil {
		return fmt.Errorf("delete file_symbols: %w", err)
	}

	// 2. Delete file tracking entry
	_, err = tx.Exec(`DELETE FROM indexed_files WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("delete indexed_files: %w", err)
	}

	// 3. Delete call edges owned by this file (v1.1: caller-owned edges invariant)
	_, err = tx.Exec(`DELETE FROM callgraph WHERE caller_file = ?`, path)
	if err != nil {
		return fmt.Errorf("delete callgraph: %w", err)
	}

	// 4. Delete file dependencies for this file (v2: transitive invalidation)
	_, err = tx.Exec(`DELETE FROM file_deps WHERE dependent_file = ?`, path)
	if err != nil {
		return fmt.Errorf("delete file_deps: %w", err)
	}

	u.logger.Debug("Deleted file data", map[string]interface{}{
		"path": path,
	})

	return nil
}

// insertFileData adds all data for a file from its FileDelta
// V2.0: symbolToFile is used to update file_deps for transitive invalidation
func (u *IndexUpdater) insertFileData(tx *sql.Tx, delta FileDelta, symbolToFile map[string]string) error {
	now := time.Now()

	// 1. Insert or replace file tracking entry
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO indexed_files (path, hash, mtime, indexed_at, scip_document_hash, symbol_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, delta.Path, delta.Hash, now.Unix(), now.Unix(), delta.SCIPDocumentHash, delta.SymbolCount)
	if err != nil {
		return fmt.Errorf("insert indexed_files: %w", err)
	}

	// 2. Insert file_symbols mappings
	if len(delta.Symbols) > 0 {
		stmt, err := tx.Prepare(`INSERT OR IGNORE INTO file_symbols (file_path, symbol_id) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare file_symbols insert: %w", err)
		}
		defer stmt.Close() //nolint:errcheck // Best effort cleanup

		for _, sym := range delta.Symbols {
			if _, err := stmt.Exec(delta.Path, sym.ID); err != nil {
				return fmt.Errorf("insert file_symbol for %s: %w", sym.ID, err)
			}
		}
	}

	// 3. Insert call edges (v1.1)
	if len(delta.CallEdges) > 0 {
		if err := u.insertCallEdges(tx, delta); err != nil {
			return fmt.Errorf("insert callgraph: %w", err)
		}
	}

	// 4. Update file_deps for transitive invalidation (v2)
	if len(delta.Refs) > 0 && symbolToFile != nil {
		if err := u.depTracker.UpdateFileDeps(tx, delta.Path, delta.Refs, symbolToFile); err != nil {
			// Log but don't fail - deps are best-effort
			u.logger.Warn("Failed to update file_deps", map[string]interface{}{
				"path":  delta.Path,
				"error": err.Error(),
			})
		}
	}

	u.logger.Debug("Inserted file data", map[string]interface{}{
		"path":        delta.Path,
		"symbolCount": len(delta.Symbols),
		"refCount":    len(delta.Refs),
		"callEdges":   len(delta.CallEdges),
	})

	return nil
}

// insertCallEdges inserts call edges for a file into the callgraph table
func (u *IndexUpdater) insertCallEdges(tx *sql.Tx, delta FileDelta) error {
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO callgraph
		(caller_id, callee_id, caller_file, call_line, call_col, call_end_col)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close() //nolint:errcheck // Best effort cleanup

	for _, edge := range delta.CallEdges {
		// Use sql.NullString for caller_id (may be empty for top-level calls)
		var callerID interface{}
		if edge.CallerID != "" {
			callerID = edge.CallerID
		}

		var endCol interface{}
		if edge.EndColumn > 0 {
			endCol = edge.EndColumn
		}

		if _, err := stmt.Exec(callerID, edge.CalleeID, edge.CallerFile,
			edge.Line, edge.Column, endCol); err != nil {
			return err
		}
	}
	return nil
}

// UpdateIndexState updates the index metadata after an incremental update
func (u *IndexUpdater) UpdateIndexState(filesUpdated int, commit string) error {
	if err := u.store.SetIndexStatePartial(filesUpdated); err != nil {
		return fmt.Errorf("failed to set index state: %w", err)
	}

	if commit != "" {
		if err := u.store.SetLastIndexedCommit(commit); err != nil {
			return fmt.Errorf("failed to set indexed commit: %w", err)
		}
	}

	return nil
}

// SetFullIndexComplete marks a full reindex as complete
// Call this after a full (non-incremental) index
func (u *IndexUpdater) SetFullIndexComplete(commit string) error {
	if err := u.store.SetIndexStateFull(); err != nil {
		return fmt.Errorf("failed to set full index state: %w", err)
	}

	if commit != "" {
		if err := u.store.SetLastIndexedCommit(commit); err != nil {
			return fmt.Errorf("failed to set indexed commit: %w", err)
		}
	}

	return nil
}

// PopulateFromFullIndex populates the file tracking tables from a full SCIP index
// This should be called after a full reindex to enable incremental updates
// v1.1: Also populates callgraph table for call edges
// v2.0: Also populates file_deps and clears rescan_queue
func (u *IndexUpdater) PopulateFromFullIndex(extractor *SCIPExtractor) error {
	index, err := extractor.LoadIndex()
	if err != nil {
		return fmt.Errorf("failed to load SCIP index: %w", err)
	}

	u.logger.Info("Populating incremental tracking from full index", map[string]interface{}{
		"documentCount": len(index.Documents),
	})

	// First pass: collect all file deltas to build symbol-to-file map
	var deltas []FileDelta
	for _, doc := range index.Documents {
		// Skip non-Go files
		if doc.Language != "go" && doc.Language != "" {
			continue
		}

		change := ChangedFile{
			Path:       doc.RelativePath,
			ChangeType: ChangeAdded,
		}
		delta := extractor.extractFileDelta(doc, change)
		deltas = append(deltas, delta)
	}

	// Build symbol-to-file map from all symbols
	symbolToFile := make(map[string]string)
	for _, delta := range deltas {
		for _, sym := range delta.Symbols {
			symbolToFile[sym.ID] = delta.Path
		}
	}

	return u.db.WithTx(func(tx *sql.Tx) error {
		// Clear existing data
		if _, err := tx.Exec(`DELETE FROM file_symbols`); err != nil {
			return fmt.Errorf("clear file_symbols: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM indexed_files`); err != nil {
			return fmt.Errorf("clear indexed_files: %w", err)
		}
		// v1.1: Also clear callgraph
		if _, err := tx.Exec(`DELETE FROM callgraph`); err != nil {
			return fmt.Errorf("clear callgraph: %w", err)
		}
		// v2.0: Also clear file_deps and rescan_queue
		if _, err := tx.Exec(`DELETE FROM file_deps`); err != nil {
			return fmt.Errorf("clear file_deps: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM rescan_queue`); err != nil {
			return fmt.Errorf("clear rescan_queue: %w", err)
		}

		now := time.Now()

		// Prepare statements
		fileStmt, err := tx.Prepare(`
			INSERT INTO indexed_files (path, hash, mtime, indexed_at, scip_document_hash, symbol_count)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare indexed_files insert: %w", err)
		}
		defer fileStmt.Close() //nolint:errcheck // Best effort cleanup

		symbolStmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO file_symbols (file_path, symbol_id) VALUES (?, ?)
		`)
		if err != nil {
			return fmt.Errorf("prepare file_symbols insert: %w", err)
		}
		defer symbolStmt.Close() //nolint:errcheck // Best effort cleanup

		totalCallEdges := 0
		totalDeps := 0

		// Process each document
		for _, delta := range deltas {
			// Insert file tracking
			if _, err := fileStmt.Exec(delta.Path, delta.Hash, now.Unix(), now.Unix(),
				delta.SCIPDocumentHash, delta.SymbolCount); err != nil {
				return fmt.Errorf("insert indexed_file for %s: %w", delta.Path, err)
			}

			// Insert symbol mappings
			for _, sym := range delta.Symbols {
				if _, err := symbolStmt.Exec(delta.Path, sym.ID); err != nil {
					return fmt.Errorf("insert file_symbol: %w", err)
				}
			}

			// v1.1: Insert call edges
			if len(delta.CallEdges) > 0 {
				if err := u.insertCallEdges(tx, delta); err != nil {
					return fmt.Errorf("insert callgraph for %s: %w", delta.Path, err)
				}
				totalCallEdges += len(delta.CallEdges)
			}

			// v2.0: Insert file dependencies
			if len(delta.Refs) > 0 {
				if err := u.depTracker.UpdateFileDeps(tx, delta.Path, delta.Refs, symbolToFile); err != nil {
					u.logger.Warn("Failed to update file_deps", map[string]interface{}{
						"path":  delta.Path,
						"error": err.Error(),
					})
				} else {
					// Count deps inserted (approximate)
					deps, _ := u.depTracker.GetDependencies(delta.Path)
					totalDeps += len(deps)
				}
			}
		}

		u.logger.Info("Full index populated", map[string]interface{}{
			"callEdges": totalCallEdges,
			"fileDeps":  totalDeps,
		})

		return nil
	})
}

// GetDependencyTracker returns the dependency tracker for external access
func (u *IndexUpdater) GetDependencyTracker() *DependencyTracker {
	return u.depTracker
}

// GetUpdateStats returns statistics about the current update
func (u *IndexUpdater) GetUpdateStats(delta *SymbolDelta) DeltaStats {
	return delta.Stats
}

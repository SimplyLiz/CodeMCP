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
	db     *storage.DB
	store  *Store
	logger *logging.Logger
}

// NewIndexUpdater creates a new incremental updater
func NewIndexUpdater(db *storage.DB, store *Store, logger *logging.Logger) *IndexUpdater {
	return &IndexUpdater{
		db:     db,
		store:  store,
		logger: logger,
	}
}

// ApplyDelta applies symbol changes to the database
// V1 updates: indexed_files, file_symbols
// V1 does NOT update: callgraph (deferred to v1.1)
func (u *IndexUpdater) ApplyDelta(delta *SymbolDelta) error {
	return u.db.WithTx(func(tx *sql.Tx) error {
		for _, fileDelta := range delta.FileDeltas {
			if err := u.applyFileDelta(tx, fileDelta); err != nil {
				return fmt.Errorf("failed to update %s: %w", fileDelta.Path, err)
			}
		}
		return nil
	})
}

// applyFileDelta applies changes for a single file
// CRITICAL: Uses OldPath for deletions to handle renames correctly
func (u *IndexUpdater) applyFileDelta(tx *sql.Tx, delta FileDelta) error {
	switch delta.ChangeType {
	case ChangeDeleted:
		// Delete everything for this file
		return u.deleteFileData(tx, delta.Path)

	case ChangeAdded:
		// Just insert new data
		return u.insertFileData(tx, delta)

	case ChangeModified:
		// Delete old data, insert new
		if err := u.deleteFileData(tx, delta.Path); err != nil {
			return err
		}
		return u.insertFileData(tx, delta)

	case ChangeRenamed:
		// CRITICAL: Delete using OldPath, insert using Path
		if delta.OldPath == "" {
			return fmt.Errorf("rename without OldPath for %s", delta.Path)
		}
		if err := u.deleteFileData(tx, delta.OldPath); err != nil {
			return err
		}
		return u.insertFileData(tx, delta)
	}

	return nil
}

// deleteFileData removes all data owned by a file
// This includes: file_symbols mapping and indexed_files entry
// Does NOT touch: callgraph (deferred to v1.1)
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

	u.logger.Debug("Deleted file data", map[string]interface{}{
		"path": path,
	})

	return nil
}

// insertFileData adds all data for a file from its FileDelta
func (u *IndexUpdater) insertFileData(tx *sql.Tx, delta FileDelta) error {
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

	u.logger.Debug("Inserted file data", map[string]interface{}{
		"path":        delta.Path,
		"symbolCount": len(delta.Symbols),
		"refCount":    len(delta.Refs),
	})

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
func (u *IndexUpdater) PopulateFromFullIndex(extractor *SCIPExtractor) error {
	index, err := extractor.LoadIndex()
	if err != nil {
		return fmt.Errorf("failed to load SCIP index: %w", err)
	}

	u.logger.Info("Populating incremental tracking from full index", map[string]interface{}{
		"documentCount": len(index.Documents),
	})

	return u.db.WithTx(func(tx *sql.Tx) error {
		// Clear existing data
		if _, err := tx.Exec(`DELETE FROM file_symbols`); err != nil {
			return fmt.Errorf("clear file_symbols: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM indexed_files`); err != nil {
			return fmt.Errorf("clear indexed_files: %w", err)
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

		// Process each document
		for _, doc := range index.Documents {
			// Skip non-Go files
			if doc.Language != "go" && doc.Language != "" {
				continue
			}

			// Compute file hash
			hash := ""
			if h, err := hashFile(extractor.repoRoot + "/" + doc.RelativePath); err == nil {
				hash = h
			}

			// Count definitions (symbols)
			symbolCount := 0
			var symbolIDs []string
			for _, occ := range doc.Occurrences {
				if occ.SymbolRoles&1 != 0 { // Definition role
					if !isLocalSymbol(occ.Symbol) {
						symbolCount++
						symbolIDs = append(symbolIDs, occ.Symbol)
					}
				}
			}

			// Insert file tracking
			docHash := computeDocHash(doc)
			if _, err := fileStmt.Exec(doc.RelativePath, hash, now.Unix(), now.Unix(), docHash, symbolCount); err != nil {
				return fmt.Errorf("insert indexed_file for %s: %w", doc.RelativePath, err)
			}

			// Insert symbol mappings
			for _, symID := range symbolIDs {
				if _, err := symbolStmt.Exec(doc.RelativePath, symID); err != nil {
					return fmt.Errorf("insert file_symbol: %w", err)
				}
			}
		}

		return nil
	})
}

// GetUpdateStats returns statistics about the current update
func (u *IndexUpdater) GetUpdateStats(delta *SymbolDelta) DeltaStats {
	return delta.Stats
}

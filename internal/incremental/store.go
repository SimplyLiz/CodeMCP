package incremental

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

// Store provides database operations for incremental indexing
type Store struct {
	db     *storage.DB
	logger *logging.Logger
}

// NewStore creates a new incremental store
func NewStore(db *storage.DB, logger *logging.Logger) *Store {
	return &Store{
		db:     db,
		logger: logger,
	}
}

// ============================================================================
// File State Operations
// ============================================================================

// GetFileState retrieves the indexed state of a file
func (s *Store) GetFileState(path string) (*IndexedFile, error) {
	row := s.db.QueryRow(`
		SELECT path, hash, mtime, indexed_at, scip_document_hash, symbol_count
		FROM indexed_files
		WHERE path = ?
	`, path)

	var f IndexedFile
	var indexedAt int64
	var scipHash sql.NullString

	err := row.Scan(&f.Path, &f.Hash, &f.Mtime, &indexedAt, &scipHash, &f.SymbolCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file state: %w", err)
	}

	f.IndexedAt = time.Unix(indexedAt, 0)
	if scipHash.Valid {
		f.SCIPDocumentHash = scipHash.String
	}

	return &f, nil
}

// GetAllFileStates retrieves all indexed file states
func (s *Store) GetAllFileStates() ([]IndexedFile, error) {
	rows, err := s.db.Query(`
		SELECT path, hash, mtime, indexed_at, scip_document_hash, symbol_count
		FROM indexed_files
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexed files: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Best effort cleanup

	var files []IndexedFile
	for rows.Next() {
		var f IndexedFile
		var indexedAt int64
		var scipHash sql.NullString

		if err := rows.Scan(&f.Path, &f.Hash, &f.Mtime, &indexedAt, &scipHash, &f.SymbolCount); err != nil {
			return nil, fmt.Errorf("failed to scan file state: %w", err)
		}

		f.IndexedAt = time.Unix(indexedAt, 0)
		if scipHash.Valid {
			f.SCIPDocumentHash = scipHash.String
		}

		files = append(files, f)
	}

	return files, rows.Err()
}

// SaveFileState saves or updates a file's indexed state
func (s *Store) SaveFileState(state *IndexedFile) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO indexed_files (path, hash, mtime, indexed_at, scip_document_hash, symbol_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, state.Path, state.Hash, state.Mtime, state.IndexedAt.Unix(), state.SCIPDocumentHash, state.SymbolCount)
	if err != nil {
		return fmt.Errorf("failed to save file state: %w", err)
	}
	return nil
}

// DeleteFileState removes a file's state (cascades to file_symbols)
func (s *Store) DeleteFileState(path string) error {
	// Delete file_symbols first (no cascade in SQLite without FK enforcement per statement)
	if _, err := s.db.Exec(`DELETE FROM file_symbols WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete file symbols: %w", err)
	}

	if _, err := s.db.Exec(`DELETE FROM indexed_files WHERE path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete file state: %w", err)
	}
	return nil
}

// GetTotalFileCount returns the total number of indexed files
func (s *Store) GetTotalFileCount() int {
	var count int
	row := s.db.QueryRow(`SELECT COUNT(*) FROM indexed_files`)
	if err := row.Scan(&count); err != nil {
		return 0
	}
	return count
}

// HasIndex returns true if there are any indexed files
func (s *Store) HasIndex() bool {
	return s.GetTotalFileCount() > 0
}

// ============================================================================
// Symbol Mapping Operations
// ============================================================================

// GetSymbolsForFile returns all symbol IDs for a file
func (s *Store) GetSymbolsForFile(path string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT symbol_id FROM file_symbols WHERE file_path = ?
	`, path)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbols for file: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Best effort cleanup

	var symbols []string
	for rows.Next() {
		var symbolID string
		if err := rows.Scan(&symbolID); err != nil {
			return nil, fmt.Errorf("failed to scan symbol ID: %w", err)
		}
		symbols = append(symbols, symbolID)
	}

	return symbols, rows.Err()
}

// SaveFileSymbols saves symbol mappings for a file
func (s *Store) SaveFileSymbols(path string, symbolIDs []string) error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		// Delete existing mappings
		if _, err := tx.Exec(`DELETE FROM file_symbols WHERE file_path = ?`, path); err != nil {
			return fmt.Errorf("failed to delete old file symbols: %w", err)
		}

		// Insert new mappings
		stmt, err := tx.Prepare(`INSERT INTO file_symbols (file_path, symbol_id) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("failed to prepare insert statement: %w", err)
		}
		defer stmt.Close() //nolint:errcheck // Best effort cleanup

		for _, symbolID := range symbolIDs {
			if _, err := stmt.Exec(path, symbolID); err != nil {
				return fmt.Errorf("failed to insert file symbol: %w", err)
			}
		}

		return nil
	})
}

// GetFilesForSymbol returns all files containing a symbol
func (s *Store) GetFilesForSymbol(symbolID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT file_path FROM file_symbols WHERE symbol_id = ?
	`, symbolID)
	if err != nil {
		return nil, fmt.Errorf("failed to query files for symbol: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Best effort cleanup

	var files []string
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return nil, fmt.Errorf("failed to scan file path: %w", err)
		}
		files = append(files, filePath)
	}

	return files, rows.Err()
}

// ============================================================================
// Index Meta Operations
// ============================================================================

// GetMeta retrieves an index metadata value
func (s *Store) GetMeta(key string) string {
	var value string
	row := s.db.QueryRow(`SELECT value FROM index_meta WHERE key = ?`, key)
	if err := row.Scan(&value); err != nil {
		return ""
	}
	return value
}

// SetMeta sets an index metadata value
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO index_meta (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		return fmt.Errorf("failed to set meta %s: %w", key, err)
	}
	return nil
}

// GetMetaInt retrieves an index metadata value as int64
func (s *Store) GetMetaInt(key string) int64 {
	value := s.GetMeta(key)
	if value == "" {
		return 0
	}
	i, _ := strconv.ParseInt(value, 10, 64)
	return i
}

// SetMetaInt sets an index metadata value as int64
func (s *Store) SetMetaInt(key string, value int64) error {
	return s.SetMeta(key, strconv.FormatInt(value, 10))
}

// GetLastIndexedCommit returns the commit hash of the last full/incremental index
func (s *Store) GetLastIndexedCommit() string {
	return s.GetMeta(MetaKeyIndexCommit)
}

// SetLastIndexedCommit saves the commit hash
func (s *Store) SetLastIndexedCommit(commit string) error {
	return s.SetMeta(MetaKeyIndexCommit, commit)
}

// GetSchemaVersion returns the stored schema version
func (s *Store) GetSchemaVersion() int {
	return int(s.GetMetaInt(MetaKeySchemaVersion))
}

// GetIndexState retrieves the full index state for display
func (s *Store) GetIndexState() IndexState {
	state := IndexState{}

	baseState := s.GetMeta(MetaKeyIndexState)
	if baseState == "" {
		baseState = "unknown"
	}

	state.LastFull = s.GetMetaInt(MetaKeyLastFull)
	state.LastIncremental = s.GetMetaInt(MetaKeyLastIncremental)
	state.FilesSinceFull = int(s.GetMetaInt(MetaKeyFilesSinceFull))
	state.Commit = s.GetLastIndexedCommit()
	state.State = baseState

	return state
}

// SetIndexStatePartial marks the index as partial (after incremental update)
func (s *Store) SetIndexStatePartial(filesUpdated int) error {
	now := time.Now().Unix()

	if err := s.SetMeta(MetaKeyIndexState, "partial"); err != nil {
		return err
	}
	if err := s.SetMetaInt(MetaKeyLastIncremental, now); err != nil {
		return err
	}

	// Increment files since full
	current := s.GetMetaInt(MetaKeyFilesSinceFull)
	return s.SetMetaInt(MetaKeyFilesSinceFull, current+int64(filesUpdated))
}

// SetIndexStateFull marks the index as full (after full reindex)
func (s *Store) SetIndexStateFull() error {
	now := time.Now().Unix()

	if err := s.SetMeta(MetaKeyIndexState, "full"); err != nil {
		return err
	}
	if err := s.SetMetaInt(MetaKeyLastFull, now); err != nil {
		return err
	}
	return s.SetMetaInt(MetaKeyFilesSinceFull, 0)
}

// ============================================================================
// Batch Operations (for use within transactions)
// ============================================================================

// DeleteFileDataTx removes all data for a file within a transaction
func (s *Store) DeleteFileDataTx(tx *sql.Tx, path string) error {
	// Delete file_symbols
	if _, err := tx.Exec(`DELETE FROM file_symbols WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete file symbols: %w", err)
	}

	// Delete indexed_files entry
	if _, err := tx.Exec(`DELETE FROM indexed_files WHERE path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete indexed file: %w", err)
	}

	return nil
}

// InsertFileDataTx inserts file data within a transaction
func (s *Store) InsertFileDataTx(tx *sql.Tx, state *IndexedFile, symbolIDs []string) error {
	// Insert file state
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO indexed_files (path, hash, mtime, indexed_at, scip_document_hash, symbol_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, state.Path, state.Hash, state.Mtime, state.IndexedAt.Unix(), state.SCIPDocumentHash, state.SymbolCount)
	if err != nil {
		return fmt.Errorf("failed to insert file state: %w", err)
	}

	// Insert symbol mappings
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO file_symbols (file_path, symbol_id) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare symbol insert: %w", err)
	}
	defer stmt.Close() //nolint:errcheck // Best effort cleanup

	for _, symbolID := range symbolIDs {
		if _, err := stmt.Exec(state.Path, symbolID); err != nil {
			return fmt.Errorf("failed to insert file symbol: %w", err)
		}
	}

	return nil
}

// ClearAllFileData removes all indexed file data (for full reindex)
func (s *Store) ClearAllFileData() error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM file_symbols`); err != nil {
			return fmt.Errorf("failed to clear file_symbols: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM indexed_files`); err != nil {
			return fmt.Errorf("failed to clear indexed_files: %w", err)
		}
		return nil
	})
}

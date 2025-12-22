// Package storage provides FTS5 full-text search support for symbols.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FTSConfig configures FTS5 behavior
type FTSConfig struct {
	// TriggerThreshold is the number of symbol changes before full rebuild
	TriggerThreshold int
	// RebuildTimeout is the maximum time for a full rebuild
	RebuildTimeout time.Duration
	// RebuildOnFullSync rebuilds FTS on full index sync
	RebuildOnFullSync bool
	// Enabled enables/disables FTS5
	Enabled bool
}

// DefaultFTSConfig returns default FTS configuration
func DefaultFTSConfig() FTSConfig {
	return FTSConfig{
		TriggerThreshold:  1000,
		RebuildTimeout:    5 * time.Minute,
		RebuildOnFullSync: true,
		Enabled:           true,
	}
}

// FTSManager manages FTS5 operations for symbol search
type FTSManager struct {
	db     *sql.DB
	config FTSConfig
}

// NewFTSManager creates a new FTS manager
func NewFTSManager(db *sql.DB, config FTSConfig) *FTSManager {
	return &FTSManager{
		db:     db,
		config: config,
	}
}

// SymbolFTSRecord represents a symbol for FTS indexing
type SymbolFTSRecord struct {
	ID            string
	Name          string
	Kind          string
	Documentation string
	Signature     string
	FilePath      string
	Language      string
}

// InitSchema creates the FTS5 table and triggers for symbols
func (m *FTSManager) InitSchema() error {
	// Create the base symbols_fts_content table first
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS symbols_fts_content (
			rowid INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			kind TEXT,
			documentation TEXT,
			signature TEXT,
			file_path TEXT,
			language TEXT,
			indexed_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create symbols_fts_content table: %w", err)
	}

	// Create indexes on content table
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_symbols_fts_content_id ON symbols_fts_content(id)",
		"CREATE INDEX IF NOT EXISTS idx_symbols_fts_content_kind ON symbols_fts_content(kind)",
		"CREATE INDEX IF NOT EXISTS idx_symbols_fts_content_language ON symbols_fts_content(language)",
	}
	for _, idx := range indexes {
		if _, execErr := m.db.Exec(idx); execErr != nil {
			return fmt.Errorf("failed to create index: %w", execErr)
		}
	}

	// Create FTS5 virtual table
	_, err = m.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
			name,
			documentation,
			signature,
			content='symbols_fts_content',
			content_rowid='rowid'
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create symbols_fts table: %w", err)
	}

	// Create triggers for automatic sync
	triggers := []string{
		// After INSERT trigger
		`CREATE TRIGGER IF NOT EXISTS symbols_fts_ai AFTER INSERT ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(rowid, name, documentation, signature)
			VALUES (new.rowid, new.name, new.documentation, new.signature);
		END`,

		// After UPDATE trigger
		`CREATE TRIGGER IF NOT EXISTS symbols_fts_au AFTER UPDATE ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
			VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
			INSERT INTO symbols_fts(rowid, name, documentation, signature)
			VALUES (new.rowid, new.name, new.documentation, new.signature);
		END`,

		// After DELETE trigger
		`CREATE TRIGGER IF NOT EXISTS symbols_fts_ad AFTER DELETE ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
			VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
		END`,
	}

	for _, trigger := range triggers {
		if _, err := m.db.Exec(trigger); err != nil {
			return fmt.Errorf("failed to create trigger: %w", err)
		}
	}

	return nil
}

// BulkInsert inserts multiple symbols into FTS in a single transaction
func (m *FTSManager) BulkInsert(ctx context.Context, symbols []SymbolFTSRecord) error {
	if len(symbols) == 0 {
		return nil
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Drop triggers for bulk operation
	triggerDrops := []string{
		"DROP TRIGGER IF EXISTS symbols_fts_ai",
		"DROP TRIGGER IF EXISTS symbols_fts_au",
		"DROP TRIGGER IF EXISTS symbols_fts_ad",
	}
	for _, drop := range triggerDrops {
		if _, dropErr := tx.ExecContext(ctx, drop); dropErr != nil {
			return fmt.Errorf("failed to drop trigger: %w", dropErr)
		}
	}

	// Clear existing content (triggers are dropped, so this won't affect FTS yet)
	if _, delErr := tx.ExecContext(ctx, "DELETE FROM symbols_fts_content"); delErr != nil {
		return fmt.Errorf("failed to clear content: %w", delErr)
	}

	// Prepare insert statement
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO symbols_fts_content (id, name, kind, documentation, signature, file_path, language)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all symbols
	for _, sym := range symbols {
		if _, err := stmt.ExecContext(ctx, sym.ID, sym.Name, sym.Kind, sym.Documentation, sym.Signature, sym.FilePath, sym.Language); err != nil {
			return fmt.Errorf("failed to insert symbol %s: %w", sym.ID, err)
		}
	}

	// Rebuild FTS index from content table
	if _, err := tx.ExecContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES('rebuild')"); err != nil {
		return fmt.Errorf("failed to rebuild FTS: %w", err)
	}

	// Re-create triggers
	triggerCreates := []string{
		`CREATE TRIGGER symbols_fts_ai AFTER INSERT ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(rowid, name, documentation, signature)
			VALUES (new.rowid, new.name, new.documentation, new.signature);
		END`,
		`CREATE TRIGGER symbols_fts_au AFTER UPDATE ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
			VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
			INSERT INTO symbols_fts(rowid, name, documentation, signature)
			VALUES (new.rowid, new.name, new.documentation, new.signature);
		END`,
		`CREATE TRIGGER symbols_fts_ad AFTER DELETE ON symbols_fts_content BEGIN
			INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
			VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
		END`,
	}
	for _, create := range triggerCreates {
		if _, err := tx.ExecContext(ctx, create); err != nil {
			return fmt.Errorf("failed to create trigger: %w", err)
		}
	}

	return tx.Commit()
}

// SearchResult represents an FTS search result
type FTSSearchResult struct {
	ID            string
	Name          string
	Kind          string
	Documentation string
	Signature     string
	FilePath      string
	Language      string
	Rank          float64 // BM25 ranking score
	MatchType     string  // "exact", "prefix", "substring"
}

// Search performs FTS5 search with ranking
func (m *FTSManager) Search(ctx context.Context, query string, limit int) ([]FTSSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	var results []FTSSearchResult

	// Normalize query
	query = strings.TrimSpace(query)
	if query == "" {
		return results, nil
	}

	// Try exact match first (highest ranking)
	exactResults, err := m.searchExact(ctx, query, limit)
	if err == nil && len(exactResults) > 0 {
		results = append(results, exactResults...)
	}

	// If not enough results, try prefix match
	if len(results) < limit {
		remaining := limit - len(results)
		prefixResults, err := m.searchPrefix(ctx, query, remaining)
		if err == nil {
			// Filter out duplicates
			seen := make(map[string]bool)
			for _, r := range results {
				seen[r.ID] = true
			}
			for _, r := range prefixResults {
				if !seen[r.ID] {
					results = append(results, r)
				}
			}
		}
	}

	// If still not enough, fall back to LIKE search
	if len(results) < limit {
		remaining := limit - len(results)
		likeResults, err := m.searchLike(ctx, query, remaining)
		if err == nil {
			seen := make(map[string]bool)
			for _, r := range results {
				seen[r.ID] = true
			}
			for _, r := range likeResults {
				if !seen[r.ID] {
					results = append(results, r)
				}
			}
		}
	}

	return results, nil
}

// searchExact performs exact phrase match
func (m *FTSManager) searchExact(ctx context.Context, query string, limit int) ([]FTSSearchResult, error) {
	// Use FTS5 phrase query with MATCH
	ftsQuery := fmt.Sprintf(`"%s"`, escapeFTS5Query(query))

	rows, err := m.db.QueryContext(ctx, `
		SELECT
			c.id, c.name, c.kind, c.documentation, c.signature, c.file_path, c.language,
			bm25(symbols_fts, 1.0, 0.5, 0.3) as rank
		FROM symbols_fts f
		JOIN symbols_fts_content c ON f.rowid = c.rowid
		WHERE symbols_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FTSSearchResult
	for rows.Next() {
		var r FTSSearchResult
		var doc, sig, filePath, language sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &doc, &sig, &filePath, &language, &r.Rank); err != nil {
			return nil, err
		}
		r.Documentation = doc.String
		r.Signature = sig.String
		r.FilePath = filePath.String
		r.Language = language.String
		r.MatchType = "exact"
		r.Rank = 1.0 // Boost exact matches
		results = append(results, r)
	}

	return results, rows.Err()
}

// searchPrefix performs prefix match
func (m *FTSManager) searchPrefix(ctx context.Context, query string, limit int) ([]FTSSearchResult, error) {
	// Use FTS5 prefix query
	ftsQuery := fmt.Sprintf(`%s*`, escapeFTS5Query(query))

	rows, err := m.db.QueryContext(ctx, `
		SELECT
			c.id, c.name, c.kind, c.documentation, c.signature, c.file_path, c.language,
			bm25(symbols_fts, 1.0, 0.5, 0.3) as rank
		FROM symbols_fts f
		JOIN symbols_fts_content c ON f.rowid = c.rowid
		WHERE symbols_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FTSSearchResult
	for rows.Next() {
		var r FTSSearchResult
		var doc, sig, filePath, language sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &doc, &sig, &filePath, &language, &r.Rank); err != nil {
			return nil, err
		}
		r.Documentation = doc.String
		r.Signature = sig.String
		r.FilePath = filePath.String
		r.Language = language.String
		r.MatchType = "prefix"
		r.Rank = 0.8 // Prefix matches ranked lower than exact
		results = append(results, r)
	}

	return results, rows.Err()
}

// searchLike performs fallback LIKE search for substring matches
func (m *FTSManager) searchLike(ctx context.Context, query string, limit int) ([]FTSSearchResult, error) {
	pattern := "%" + query + "%"

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, kind, documentation, signature, file_path, language
		FROM symbols_fts_content
		WHERE name LIKE ? OR documentation LIKE ? OR signature LIKE ?
		LIMIT ?
	`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FTSSearchResult
	for rows.Next() {
		var r FTSSearchResult
		var doc, sig, filePath, language sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &doc, &sig, &filePath, &language); err != nil {
			return nil, err
		}
		r.Documentation = doc.String
		r.Signature = sig.String
		r.FilePath = filePath.String
		r.Language = language.String
		r.MatchType = "substring"
		r.Rank = 0.5 // Substring matches ranked lowest
		results = append(results, r)
	}

	return results, rows.Err()
}

// Rebuild forces a complete rebuild of the FTS index
func (m *FTSManager) Rebuild(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES('rebuild')")
	return err
}

// Vacuum optimizes the FTS index
func (m *FTSManager) Vacuum(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES('optimize')")
	return err
}

// IntegrityCheck verifies FTS index integrity
func (m *FTSManager) IntegrityCheck(ctx context.Context) (bool, error) {
	rows, err := m.db.QueryContext(ctx, "INSERT INTO symbols_fts(symbols_fts) VALUES('integrity-check')")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	// If no rows returned or no error, integrity is good
	return true, nil
}

// Clear removes all data from FTS tables
func (m *FTSManager) Clear(ctx context.Context) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols_fts_content"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM symbols_fts"); err != nil {
		return err
	}

	return tx.Commit()
}

// GetStats returns FTS index statistics
func (m *FTSManager) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count indexed symbols
	var count int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols_fts_content").Scan(&count); err != nil {
		return nil, err
	}
	stats["indexed_symbols"] = count

	// Get FTS size estimate
	var pageCount, pageSize int
	if err := m.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err == nil {
		if err := m.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err == nil {
			stats["estimated_size_bytes"] = pageCount * pageSize
		}
	}

	return stats, nil
}

// escapeFTS5Query escapes special characters in FTS5 queries
func escapeFTS5Query(query string) string {
	// Escape special FTS5 characters: " * ( ) - OR AND NOT
	replacer := strings.NewReplacer(
		`"`, `""`,
		`*`, `\*`,
		`(`, `\(`,
		`)`, `\)`,
	)
	return replacer.Replace(query)
}

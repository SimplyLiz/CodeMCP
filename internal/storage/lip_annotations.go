package storage

import (
	"database/sql"
	"time"
)

// LIPAnnotation is a symbol-URI-bound annotation matching the LIP AnnotationEntry schema.
// Unlike module annotations (which are module-level), these attach to individual symbols
// identified by their LIP URI (e.g. lip://local/src/foo.go#MyFunc).
type LIPAnnotation struct {
	SymbolURI   string
	Key         string
	Value       string
	AuthorID    string
	Confidence  uint8
	TimestampMs int64
	ExpiresMs   int64 // 0 = never expires
}

// LIPAnnotationRepository manages LIP symbol annotations in SQLite.
type LIPAnnotationRepository struct {
	db *sql.DB
}

// NewLIPAnnotationRepository creates a new repository and ensures the table exists.
func NewLIPAnnotationRepository(db *sql.DB) *LIPAnnotationRepository {
	r := &LIPAnnotationRepository{db: db}
	_ = r.migrate()
	return r
}

func (r *LIPAnnotationRepository) migrate() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS lip_annotations (
			symbol_uri   TEXT NOT NULL,
			key          TEXT NOT NULL,
			value        TEXT NOT NULL,
			author_id    TEXT NOT NULL DEFAULT '',
			confidence   INTEGER NOT NULL DEFAULT 80,
			timestamp_ms INTEGER NOT NULL,
			expires_ms   INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (symbol_uri, key)
		)`)
	return err
}

// Set inserts or replaces an annotation.
func (r *LIPAnnotationRepository) Set(a *LIPAnnotation) error {
	now := time.Now().UnixMilli()
	if a.TimestampMs == 0 {
		a.TimestampMs = now
	}
	_, err := r.db.Exec(`
		INSERT INTO lip_annotations (symbol_uri, key, value, author_id, confidence, timestamp_ms, expires_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol_uri, key) DO UPDATE SET
			value        = excluded.value,
			author_id    = excluded.author_id,
			confidence   = excluded.confidence,
			timestamp_ms = excluded.timestamp_ms,
			expires_ms   = excluded.expires_ms`,
		a.SymbolURI, a.Key, a.Value, a.AuthorID, a.Confidence, a.TimestampMs, a.ExpiresMs)
	return err
}

// Get retrieves a single annotation by symbol_uri + key. Returns nil if not found or expired.
func (r *LIPAnnotationRepository) Get(symbolURI, key string) (*LIPAnnotation, error) {
	now := time.Now().UnixMilli()
	row := r.db.QueryRow(`
		SELECT symbol_uri, key, value, author_id, confidence, timestamp_ms, expires_ms
		FROM lip_annotations
		WHERE symbol_uri = ? AND key = ?
		  AND (expires_ms = 0 OR expires_ms > ?)`,
		symbolURI, key, now)
	return scanAnnotation(row)
}

// List retrieves all non-expired annotations for a symbol URI.
func (r *LIPAnnotationRepository) List(symbolURI string) ([]*LIPAnnotation, error) {
	now := time.Now().UnixMilli()
	rows, err := r.db.Query(`
		SELECT symbol_uri, key, value, author_id, confidence, timestamp_ms, expires_ms
		FROM lip_annotations
		WHERE symbol_uri = ?
		  AND (expires_ms = 0 OR expires_ms > ?)
		ORDER BY key`,
		symbolURI, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*LIPAnnotation
	for rows.Next() {
		a, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Delete removes an annotation by symbol_uri + key.
func (r *LIPAnnotationRepository) Delete(symbolURI, key string) error {
	_, err := r.db.Exec(`DELETE FROM lip_annotations WHERE symbol_uri = ? AND key = ?`, symbolURI, key)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAnnotation(s scanner) (*LIPAnnotation, error) {
	var a LIPAnnotation
	err := s.Scan(&a.SymbolURI, &a.Key, &a.Value, &a.AuthorID, &a.Confidence, &a.TimestampMs, &a.ExpiresMs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

package docs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/storage"
)

// Store provides database operations for doc-symbol linking.
type Store struct {
	db *storage.DB
}

// NewStore creates a new docs store.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// SaveDocument saves or updates a document and its references.
func (s *Store) SaveDocument(doc *Document) error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		// Delete existing document (cascade deletes references and modules)
		_, err := tx.Exec("DELETE FROM docs WHERE path = ?", doc.Path)
		if err != nil {
			return fmt.Errorf("failed to delete existing doc: %w", err)
		}

		// Insert document
		_, err = tx.Exec(`
			INSERT INTO docs (path, doc_type, title, hash, last_indexed)
			VALUES (?, ?, ?, ?, ?)
		`, doc.Path, string(doc.Type), doc.Title, doc.Hash, doc.LastIndexed.Unix())
		if err != nil {
			return fmt.Errorf("failed to insert doc: %w", err)
		}

		// Insert references
		for _, ref := range doc.References {
			candidatesJSON, _ := json.Marshal(ref.Candidates)
			_, err = tx.Exec(`
				INSERT INTO doc_references (
					doc_path, raw_text, normalized_text, symbol_id, symbol_name,
					line, col, context, detection_method, resolution,
					candidates, confidence, last_resolved
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				doc.Path, ref.RawText, ref.NormalizedText, ref.SymbolID, ref.SymbolName,
				ref.Line, ref.Column, ref.Context, string(ref.DetectionMethod), string(ref.Resolution),
				string(candidatesJSON), ref.Confidence, ref.LastResolved.Unix(),
			)
			if err != nil {
				return fmt.Errorf("failed to insert reference: %w", err)
			}
		}

		// Insert module links
		for _, modID := range doc.Modules {
			_, err = tx.Exec(`
				INSERT INTO doc_modules (doc_path, module_id, line)
				VALUES (?, ?, ?)
			`, doc.Path, modID, 0)
			if err != nil {
				return fmt.Errorf("failed to insert module link: %w", err)
			}
		}

		return nil
	})
}

// GetDocument retrieves a document with its references.
func (s *Store) GetDocument(path string) (*Document, error) {
	var doc Document
	var lastIndexed int64

	err := s.db.QueryRow(`
		SELECT path, doc_type, title, hash, last_indexed
		FROM docs WHERE path = ?
	`, path).Scan(&doc.Path, &doc.Type, &doc.Title, &doc.Hash, &lastIndexed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	doc.LastIndexed = time.Unix(lastIndexed, 0)

	// Get references
	refs, err := s.GetReferencesForDoc(path)
	if err != nil {
		return nil, err
	}
	doc.References = refs

	// Get modules
	rows, err := s.db.Query("SELECT module_id FROM doc_modules WHERE doc_path = ?", path)
	if err != nil {
		return nil, fmt.Errorf("failed to get modules: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var modID string
		if err := rows.Scan(&modID); err != nil {
			return nil, fmt.Errorf("failed to scan module: %w", err)
		}
		doc.Modules = append(doc.Modules, modID)
	}

	return &doc, nil
}

// GetReferencesForDoc retrieves all references in a document.
func (s *Store) GetReferencesForDoc(path string) ([]DocReference, error) {
	rows, err := s.db.Query(`
		SELECT id, doc_path, raw_text, normalized_text, symbol_id, symbol_name,
			   line, col, context, detection_method, resolution,
			   candidates, confidence, last_resolved
		FROM doc_references WHERE doc_path = ?
		ORDER BY line, col
	`, path)
	if err != nil {
		return nil, fmt.Errorf("failed to query references: %w", err)
	}
	defer rows.Close()

	return s.scanReferences(rows)
}

// GetDocsForSymbol finds all documents that reference a symbol.
func (s *Store) GetDocsForSymbol(symbolID string, limit int) ([]DocReference, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.Query(`
		SELECT id, doc_path, raw_text, normalized_text, symbol_id, symbol_name,
			   line, col, context, detection_method, resolution,
			   candidates, confidence, last_resolved
		FROM doc_references
		WHERE symbol_id = ?
		ORDER BY doc_path, line
		LIMIT ?
	`, symbolID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query docs for symbol: %w", err)
	}
	defer rows.Close()

	return s.scanReferences(rows)
}

// GetDocsForModule finds all documents linked to a module.
func (s *Store) GetDocsForModule(moduleID string) ([]Document, error) {
	rows, err := s.db.Query(`
		SELECT d.path, d.doc_type, d.title, d.hash, d.last_indexed
		FROM docs d
		JOIN doc_modules dm ON d.path = dm.doc_path
		WHERE dm.module_id = ?
		ORDER BY d.path
	`, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to query docs for module: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		var lastIndexed int64
		if err := rows.Scan(&doc.Path, &doc.Type, &doc.Title, &doc.Hash, &lastIndexed); err != nil {
			return nil, fmt.Errorf("failed to scan doc: %w", err)
		}
		doc.LastIndexed = time.Unix(lastIndexed, 0)
		docs = append(docs, doc)
	}

	return docs, nil
}

// GetAllDocuments retrieves all indexed documents (without references for efficiency).
func (s *Store) GetAllDocuments() ([]Document, error) {
	rows, err := s.db.Query(`
		SELECT path, doc_type, title, hash, last_indexed
		FROM docs ORDER BY path
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		var lastIndexed int64
		if err := rows.Scan(&doc.Path, &doc.Type, &doc.Title, &doc.Hash, &lastIndexed); err != nil {
			return nil, fmt.Errorf("failed to scan doc: %w", err)
		}
		doc.LastIndexed = time.Unix(lastIndexed, 0)
		docs = append(docs, doc)
	}

	return docs, nil
}

// GetDocumentHash retrieves just the hash of a document (for change detection).
func (s *Store) GetDocumentHash(path string) (string, error) {
	var hash string
	err := s.db.QueryRow("SELECT hash FROM docs WHERE path = ?", path).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return hash, nil
}

// DeleteDocument removes a document and its references.
func (s *Store) DeleteDocument(path string) error {
	_, err := s.db.Exec("DELETE FROM docs WHERE path = ?", path)
	return err
}

// scanReferences scans rows into DocReference structs.
func (s *Store) scanReferences(rows *sql.Rows) ([]DocReference, error) {
	var refs []DocReference
	for rows.Next() {
		var ref DocReference
		var symbolID sql.NullString
		var candidatesJSON sql.NullString
		var lastResolved int64

		err := rows.Scan(
			&ref.ID, &ref.DocPath, &ref.RawText, &ref.NormalizedText,
			&symbolID, &ref.SymbolName, &ref.Line, &ref.Column, &ref.Context,
			&ref.DetectionMethod, &ref.Resolution, &candidatesJSON,
			&ref.Confidence, &lastResolved,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reference: %w", err)
		}

		if symbolID.Valid {
			ref.SymbolID = &symbolID.String
		}
		if candidatesJSON.Valid && candidatesJSON.String != "" {
			_ = json.Unmarshal([]byte(candidatesJSON.String), &ref.Candidates)
		}
		ref.LastResolved = time.Unix(lastResolved, 0)

		refs = append(refs, ref)
	}

	return refs, nil
}

// Suffix Index Operations

// SaveSuffixes saves suffix entries for a symbol.
func (s *Store) SaveSuffixes(symbolID string, suffixes []string) error {
	return s.db.WithTx(func(tx *sql.Tx) error {
		// Delete existing suffixes for this symbol
		_, err := tx.Exec("DELETE FROM symbol_suffixes WHERE symbol_id = ?", symbolID)
		if err != nil {
			return err
		}

		// Insert new suffixes
		for _, suffix := range suffixes {
			segmentCount := countSegments(suffix)
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO symbol_suffixes (suffix, symbol_id, segment_count)
				VALUES (?, ?, ?)
			`, suffix, symbolID, segmentCount)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// ClearSuffixIndex clears the entire suffix index.
func (s *Store) ClearSuffixIndex() error {
	_, err := s.db.Exec("DELETE FROM symbol_suffixes")
	return err
}

// SuffixMatch finds all symbols matching a suffix.
func (s *Store) SuffixMatch(suffix string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT symbol_id FROM symbol_suffixes
		WHERE suffix = ?
	`, suffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		candidates = append(candidates, id)
	}

	return candidates, nil
}

// Meta Operations

// SetMeta sets a metadata value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO docs_meta (key, value) VALUES (?, ?)
	`, key, value)
	return err
}

// GetMeta gets a metadata value.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM docs_meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetSymbolIndexVersion returns the version of the symbol index used for suffix building.
func (s *Store) GetSymbolIndexVersion() (string, error) {
	return s.GetMeta("symbol_index_version")
}

// SetSymbolIndexVersion sets the version of the symbol index.
func (s *Store) SetSymbolIndexVersion(version string) error {
	return s.SetMeta("symbol_index_version", version)
}

// Stats Operations

// GetStats returns statistics about indexed docs.
func (s *Store) GetStats() (*IndexStats, error) {
	stats := &IndexStats{}

	// Count docs
	err := s.db.QueryRow("SELECT COUNT(*) FROM docs").Scan(&stats.DocsIndexed)
	if err != nil {
		return nil, err
	}

	// Count references by resolution status
	rows, err := s.db.Query(`
		SELECT resolution, COUNT(*) FROM doc_references GROUP BY resolution
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var resolution string
		var count int
		if err := rows.Scan(&resolution, &count); err != nil {
			return nil, err
		}

		stats.ReferencesFound += count
		switch ResolutionStatus(resolution) {
		case ResolutionExact, ResolutionSuffix:
			stats.Resolved += count
		case ResolutionAmbiguous:
			stats.Ambiguous += count
		case ResolutionMissing:
			stats.Missing += count
		case ResolutionIneligible:
			stats.Ineligible += count
		}
	}

	return stats, nil
}

// countSegments returns the number of dot-separated segments in a string.
func countSegments(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '.' {
			count++
		}
	}
	return count
}

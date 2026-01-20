package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// KeyStore provides persistence for API keys using SQLite
type KeyStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewKeyStore creates a new key store backed by SQLite
// The caller is responsible for ensuring the database has the required tables
func NewKeyStore(db *sql.DB, logger *slog.Logger) *KeyStore {
	return &KeyStore{
		db:     db,
		logger: logger,
	}
}

// InitSchema creates the required tables if they don't exist
func (s *KeyStore) InitSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			scopes TEXT NOT NULL,
			repo_patterns TEXT,
			rate_limit INTEGER,
			expires_at TEXT,
			created_at TEXT NOT NULL,
			created_by TEXT,
			last_used_at TEXT,
			revoked INTEGER NOT NULL DEFAULT 0,
			revoked_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(token_prefix);
		CREATE INDEX IF NOT EXISTS idx_api_keys_revoked ON api_keys(revoked);

		CREATE TABLE IF NOT EXISTS auth_audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			key_id TEXT,
			key_name TEXT,
			ip_address TEXT,
			user_agent TEXT,
			details TEXT,
			occurred_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_auth_audit_occurred ON auth_audit_log(occurred_at);
		CREATE INDEX IF NOT EXISTS idx_auth_audit_key ON auth_audit_log(key_id);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// Save persists a new API key
func (s *KeyStore) Save(key *APIKey) error {
	scopesJSON, err := json.Marshal(key.Scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}

	var repoPatternsJSON []byte
	if len(key.RepoPatterns) > 0 {
		repoPatternsJSON, err = json.Marshal(key.RepoPatterns)
		if err != nil {
			return fmt.Errorf("marshal repo patterns: %w", err)
		}
	}

	var expiresAt *string
	if key.ExpiresAt != nil {
		t := key.ExpiresAt.Format(time.RFC3339)
		expiresAt = &t
	}

	var lastUsedAt *string
	if key.LastUsedAt != nil {
		t := key.LastUsedAt.Format(time.RFC3339)
		lastUsedAt = &t
	}

	var revokedAt *string
	if key.RevokedAt != nil {
		t := key.RevokedAt.Format(time.RFC3339)
		revokedAt = &t
	}

	_, err = s.db.Exec(`
		INSERT INTO api_keys (
			id, name, token_hash, token_prefix, scopes, repo_patterns,
			rate_limit, expires_at, created_at, created_by, last_used_at,
			revoked, revoked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		key.ID,
		key.Name,
		key.TokenHash,
		key.TokenPrefix,
		string(scopesJSON),
		nullableString(repoPatternsJSON),
		key.RateLimit,
		expiresAt,
		key.CreatedAt.Format(time.RFC3339),
		nullableStringPtr(&key.CreatedBy),
		lastUsedAt,
		boolToInt(key.Revoked),
		revokedAt,
	)

	if err != nil {
		return fmt.Errorf("insert key: %w", err)
	}

	s.logger.Debug("API key saved",
		"key_id", key.ID,
		"key_name", key.Name,
	)

	return nil
}

// GetByID retrieves a key by its ID
func (s *KeyStore) GetByID(id string) (*APIKey, error) {
	row := s.db.QueryRow(`
		SELECT id, name, token_hash, token_prefix, scopes, repo_patterns,
			   rate_limit, expires_at, created_at, created_by, last_used_at,
			   revoked, revoked_at
		FROM api_keys
		WHERE id = ?
	`, id)

	return s.scanKey(row)
}

// GetByTokenPrefix retrieves keys matching a token prefix for fast lookup
// Returns all matching keys (there should typically be only one)
func (s *KeyStore) GetByTokenPrefix(prefix string) ([]*APIKey, error) {
	rows, err := s.db.Query(`
		SELECT id, name, token_hash, token_prefix, scopes, repo_patterns,
			   rate_limit, expires_at, created_at, created_by, last_used_at,
			   revoked, revoked_at
		FROM api_keys
		WHERE token_prefix = ?
	`, prefix)
	if err != nil {
		return nil, fmt.Errorf("query by prefix: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanKeys(rows)
}

// List returns all keys matching filters
func (s *KeyStore) List(includeRevoked bool) ([]*APIKey, error) {
	query := `
		SELECT id, name, token_hash, token_prefix, scopes, repo_patterns,
			   rate_limit, expires_at, created_at, created_by, last_used_at,
			   revoked, revoked_at
		FROM api_keys
	`
	if !includeRevoked {
		query += " WHERE revoked = 0"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanKeys(rows)
}

// Update updates an existing key
func (s *KeyStore) Update(key *APIKey) error {
	scopesJSON, err := json.Marshal(key.Scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}

	var repoPatternsJSON []byte
	if len(key.RepoPatterns) > 0 {
		repoPatternsJSON, err = json.Marshal(key.RepoPatterns)
		if err != nil {
			return fmt.Errorf("marshal repo patterns: %w", err)
		}
	}

	var expiresAt *string
	if key.ExpiresAt != nil {
		t := key.ExpiresAt.Format(time.RFC3339)
		expiresAt = &t
	}

	var lastUsedAt *string
	if key.LastUsedAt != nil {
		t := key.LastUsedAt.Format(time.RFC3339)
		lastUsedAt = &t
	}

	var revokedAt *string
	if key.RevokedAt != nil {
		t := key.RevokedAt.Format(time.RFC3339)
		revokedAt = &t
	}

	result, err := s.db.Exec(`
		UPDATE api_keys SET
			name = ?,
			token_hash = ?,
			token_prefix = ?,
			scopes = ?,
			repo_patterns = ?,
			rate_limit = ?,
			expires_at = ?,
			last_used_at = ?,
			revoked = ?,
			revoked_at = ?
		WHERE id = ?
	`,
		key.Name,
		key.TokenHash,
		key.TokenPrefix,
		string(scopesJSON),
		nullableString(repoPatternsJSON),
		key.RateLimit,
		expiresAt,
		lastUsedAt,
		boolToInt(key.Revoked),
		revokedAt,
		key.ID,
	)

	if err != nil {
		return fmt.Errorf("update key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrKeyNotFound
	}

	return nil
}

// UpdateLastUsed updates only the last_used_at timestamp
func (s *KeyStore) UpdateLastUsed(id string, lastUsed time.Time) error {
	_, err := s.db.Exec(`
		UPDATE api_keys SET last_used_at = ? WHERE id = ?
	`, lastUsed.Format(time.RFC3339), id)

	return err
}

// Delete permanently removes a key
func (s *KeyStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrKeyNotFound
	}

	return nil
}

// LogAuditEvent records an authentication-related event
func (s *KeyStore) LogAuditEvent(event AuditEvent) error {
	var detailsJSON []byte
	var err error
	if len(event.Details) > 0 {
		detailsJSON, err = json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}
	}

	_, err = s.db.Exec(`
		INSERT INTO auth_audit_log (event_type, key_id, key_name, ip_address, user_agent, details, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		event.EventType,
		nullableStringPtr(&event.KeyID),
		nullableStringPtr(&event.KeyName),
		nullableStringPtr(&event.IPAddress),
		nullableStringPtr(&event.UserAgent),
		nullableString(detailsJSON),
		event.OccurredAt.Format(time.RFC3339),
	)

	return err
}

// scanKey scans a single row into an APIKey
func (s *KeyStore) scanKey(row *sql.Row) (*APIKey, error) {
	var key APIKey
	var scopesJSON string
	var repoPatternsJSON sql.NullString
	var rateLimit sql.NullInt64
	var expiresAt, createdAt, createdBy, lastUsedAt, revokedAt sql.NullString
	var revoked int

	err := row.Scan(
		&key.ID,
		&key.Name,
		&key.TokenHash,
		&key.TokenPrefix,
		&scopesJSON,
		&repoPatternsJSON,
		&rateLimit,
		&expiresAt,
		&createdAt,
		&createdBy,
		&lastUsedAt,
		&revoked,
		&revokedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan key: %w", err)
	}

	// Parse JSON fields
	if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
		return nil, fmt.Errorf("unmarshal scopes: %w", err)
	}

	if repoPatternsJSON.Valid && repoPatternsJSON.String != "" {
		if err := json.Unmarshal([]byte(repoPatternsJSON.String), &key.RepoPatterns); err != nil {
			return nil, fmt.Errorf("unmarshal repo patterns: %w", err)
		}
	}

	// Parse optional fields
	if rateLimit.Valid {
		rl := int(rateLimit.Int64)
		key.RateLimit = &rl
	}

	if expiresAt.Valid {
		t, err := time.Parse(time.RFC3339, expiresAt.String)
		if err == nil {
			key.ExpiresAt = &t
		}
	}

	if createdAt.Valid {
		t, err := time.Parse(time.RFC3339, createdAt.String)
		if err == nil {
			key.CreatedAt = t
		}
	}

	if createdBy.Valid {
		key.CreatedBy = createdBy.String
	}

	if lastUsedAt.Valid {
		t, err := time.Parse(time.RFC3339, lastUsedAt.String)
		if err == nil {
			key.LastUsedAt = &t
		}
	}

	key.Revoked = revoked != 0

	if revokedAt.Valid {
		t, err := time.Parse(time.RFC3339, revokedAt.String)
		if err == nil {
			key.RevokedAt = &t
		}
	}

	return &key, nil
}

// scanKeys scans multiple rows into APIKeys
func (s *KeyStore) scanKeys(rows *sql.Rows) ([]*APIKey, error) {
	var keys []*APIKey

	for rows.Next() {
		var key APIKey
		var scopesJSON string
		var repoPatternsJSON sql.NullString
		var rateLimit sql.NullInt64
		var expiresAt, createdAt, createdBy, lastUsedAt, revokedAt sql.NullString
		var revoked int

		err := rows.Scan(
			&key.ID,
			&key.Name,
			&key.TokenHash,
			&key.TokenPrefix,
			&scopesJSON,
			&repoPatternsJSON,
			&rateLimit,
			&expiresAt,
			&createdAt,
			&createdBy,
			&lastUsedAt,
			&revoked,
			&revokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan key: %w", err)
		}

		// Parse JSON fields
		if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}

		if repoPatternsJSON.Valid && repoPatternsJSON.String != "" {
			if err := json.Unmarshal([]byte(repoPatternsJSON.String), &key.RepoPatterns); err != nil {
				return nil, fmt.Errorf("unmarshal repo patterns: %w", err)
			}
		}

		// Parse optional fields
		if rateLimit.Valid {
			rl := int(rateLimit.Int64)
			key.RateLimit = &rl
		}

		if expiresAt.Valid {
			t, err := time.Parse(time.RFC3339, expiresAt.String)
			if err == nil {
				key.ExpiresAt = &t
			}
		}

		if createdAt.Valid {
			t, err := time.Parse(time.RFC3339, createdAt.String)
			if err == nil {
				key.CreatedAt = t
			}
		}

		if createdBy.Valid {
			key.CreatedBy = createdBy.String
		}

		if lastUsedAt.Valid {
			t, err := time.Parse(time.RFC3339, lastUsedAt.String)
			if err == nil {
				key.LastUsedAt = &t
			}
		}

		key.Revoked = revoked != 0

		if revokedAt.Valid {
			t, err := time.Parse(time.RFC3339, revokedAt.String)
			if err == nil {
				key.RevokedAt = &t
			}
		}

		keys = append(keys, &key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return keys, nil
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableString(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func nullableStringPtr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

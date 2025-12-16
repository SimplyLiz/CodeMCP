package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CacheTier represents the type of cache
type CacheTier string

const (
	// QueryCache for query results (TTL 300s, key includes headCommit)
	QueryCache CacheTier = "query"
	// ViewCache for view results (TTL 3600s, key includes repoStateId)
	ViewCache CacheTier = "view"
	// NegativeCache for negative results (TTL 60s, key includes repoStateId)
	NegativeCache CacheTier = "negative"
)

// CacheEntry represents a generic cache entry
type CacheEntry struct {
	Key       string
	Value     string // JSON-encoded value
	ExpiresAt time.Time
	StateID   string
	CreatedAt time.Time
}

// NegativeCacheEntry represents an entry in the negative cache
type NegativeCacheEntry struct {
	Key          string
	ErrorType    string
	ErrorMessage string
	ExpiresAt    time.Time
	StateID      string
	CreatedAt    time.Time
}

// Cache provides methods for cache operations across all cache tiers
type Cache struct {
	db *DB
}

// NewCache creates a new cache instance
func NewCache(db *DB) *Cache {
	return &Cache{db: db}
}

// GetQueryCache retrieves a value from the query cache
// Returns nil if not found or expired
func (c *Cache) GetQueryCache(key string, headCommit string) (string, bool, error) {
	var valueJSON string
	var expiresAt string

	err := c.db.QueryRow(`
		SELECT value_json, expires_at
		FROM query_cache
		WHERE key = ? AND head_commit = ?
	`, key, headCommit).Scan(&valueJSON, &expiresAt)

	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("query cache lookup failed: %w", err)
	}

	// Check if expired
	expiresAtTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", false, fmt.Errorf("invalid expires_at format: %w", err)
	}

	if time.Now().After(expiresAtTime) {
		// Entry is expired, delete it
		c.db.Exec("DELETE FROM query_cache WHERE key = ?", key)
		return "", false, nil
	}

	return valueJSON, true, nil
}

// SetQueryCache stores a value in the query cache
func (c *Cache) SetQueryCache(key string, valueJSON string, headCommit string, stateID string, ttlSeconds int) error {
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO query_cache (key, value_json, expires_at, state_id, head_commit, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, key, valueJSON, expiresAt.Format(time.RFC3339), stateID, headCommit, now.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("failed to set query cache: %w", err)
	}

	return nil
}

// GetViewCache retrieves a value from the view cache
// Returns nil if not found or expired
func (c *Cache) GetViewCache(key string, stateID string) (string, bool, error) {
	var valueJSON string
	var expiresAt string

	err := c.db.QueryRow(`
		SELECT value_json, expires_at
		FROM view_cache
		WHERE key = ? AND state_id = ?
	`, key, stateID).Scan(&valueJSON, &expiresAt)

	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("view cache lookup failed: %w", err)
	}

	// Check if expired
	expiresAtTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", false, fmt.Errorf("invalid expires_at format: %w", err)
	}

	if time.Now().After(expiresAtTime) {
		// Entry is expired, delete it
		c.db.Exec("DELETE FROM view_cache WHERE key = ?", key)
		return "", false, nil
	}

	return valueJSON, true, nil
}

// SetViewCache stores a value in the view cache
func (c *Cache) SetViewCache(key string, valueJSON string, stateID string, ttlSeconds int) error {
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO view_cache (key, value_json, expires_at, state_id, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, key, valueJSON, expiresAt.Format(time.RFC3339), stateID, now.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("failed to set view cache: %w", err)
	}

	return nil
}

// GetNegativeCache retrieves an error from the negative cache
// Returns nil if not found or expired
func (c *Cache) GetNegativeCache(key string, stateID string) (*NegativeCacheEntry, error) {
	var entry NegativeCacheEntry
	var expiresAt, createdAt string

	err := c.db.QueryRow(`
		SELECT key, error_type, error_message, expires_at, state_id, created_at
		FROM negative_cache
		WHERE key = ? AND state_id = ?
	`, key, stateID).Scan(&entry.Key, &entry.ErrorType, &entry.ErrorMessage, &expiresAt, &entry.StateID, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("negative cache lookup failed: %w", err)
	}

	// Parse timestamps
	expiresAtTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid expires_at format: %w", err)
	}

	createdAtTime, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("invalid created_at format: %w", err)
	}

	entry.ExpiresAt = expiresAtTime
	entry.CreatedAt = createdAtTime

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		// Entry is expired, delete it
		c.db.Exec("DELETE FROM negative_cache WHERE key = ?", key)
		return nil, nil
	}

	return &entry, nil
}

// SetNegativeCache stores an error in the negative cache
func (c *Cache) SetNegativeCache(key string, errorType string, errorMessage string, stateID string, ttlSeconds int) error {
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO negative_cache (key, error_type, error_message, expires_at, state_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, key, errorType, errorMessage, expiresAt.Format(time.RFC3339), stateID, now.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("failed to set negative cache: %w", err)
	}

	return nil
}

// InvalidateQueryCache removes entries from query cache by key pattern
func (c *Cache) InvalidateQueryCache(keyPattern string) error {
	_, err := c.db.Exec("DELETE FROM query_cache WHERE key LIKE ?", keyPattern)
	if err != nil {
		return fmt.Errorf("failed to invalidate query cache: %w", err)
	}
	return nil
}

// InvalidateViewCache removes entries from view cache by key pattern
func (c *Cache) InvalidateViewCache(keyPattern string) error {
	_, err := c.db.Exec("DELETE FROM view_cache WHERE key LIKE ?", keyPattern)
	if err != nil {
		return fmt.Errorf("failed to invalidate view cache: %w", err)
	}
	return nil
}

// InvalidateNegativeCache removes entries from negative cache by key pattern
func (c *Cache) InvalidateNegativeCache(keyPattern string) error {
	_, err := c.db.Exec("DELETE FROM negative_cache WHERE key LIKE ?", keyPattern)
	if err != nil {
		return fmt.Errorf("failed to invalidate negative cache: %w", err)
	}
	return nil
}

// InvalidateAllQueryCache clears all entries from query cache
func (c *Cache) InvalidateAllQueryCache() error {
	_, err := c.db.Exec("DELETE FROM query_cache")
	if err != nil {
		return fmt.Errorf("failed to clear query cache: %w", err)
	}
	return nil
}

// InvalidateAllViewCache clears all entries from view cache
func (c *Cache) InvalidateAllViewCache() error {
	_, err := c.db.Exec("DELETE FROM view_cache")
	if err != nil {
		return fmt.Errorf("failed to clear view cache: %w", err)
	}
	return nil
}

// InvalidateAllNegativeCache clears all entries from negative cache
func (c *Cache) InvalidateAllNegativeCache() error {
	_, err := c.db.Exec("DELETE FROM negative_cache")
	if err != nil {
		return fmt.Errorf("failed to clear negative cache: %w", err)
	}
	return nil
}

// InvalidateByStateID removes all cache entries for a specific state ID
// This is triggered when the repository state changes (Section 9.3)
func (c *Cache) InvalidateByStateID(stateID string) error {
	// Invalidate query cache
	if _, err := c.db.Exec("DELETE FROM query_cache WHERE state_id = ?", stateID); err != nil {
		return fmt.Errorf("failed to invalidate query cache by state_id: %w", err)
	}

	// Invalidate view cache
	if _, err := c.db.Exec("DELETE FROM view_cache WHERE state_id = ?", stateID); err != nil {
		return fmt.Errorf("failed to invalidate view cache by state_id: %w", err)
	}

	// Invalidate negative cache
	if _, err := c.db.Exec("DELETE FROM negative_cache WHERE state_id = ?", stateID); err != nil {
		return fmt.Errorf("failed to invalidate negative cache by state_id: %w", err)
	}

	c.db.logger.Debug("Invalidated all caches for state", map[string]interface{}{
		"state_id": stateID,
	})

	return nil
}

// CleanupExpiredEntries removes all expired entries from all cache tables
// This should be called periodically
func (c *Cache) CleanupExpiredEntries() error {
	now := time.Now().Format(time.RFC3339)

	// Clean query cache
	if _, err := c.db.Exec("DELETE FROM query_cache WHERE expires_at < ?", now); err != nil {
		return fmt.Errorf("failed to cleanup query cache: %w", err)
	}

	// Clean view cache
	if _, err := c.db.Exec("DELETE FROM view_cache WHERE expires_at < ?", now); err != nil {
		return fmt.Errorf("failed to cleanup view cache: %w", err)
	}

	// Clean negative cache
	if _, err := c.db.Exec("DELETE FROM negative_cache WHERE expires_at < ?", now); err != nil {
		return fmt.Errorf("failed to cleanup negative cache: %w", err)
	}

	c.db.logger.Debug("Cleaned up expired cache entries", nil)

	return nil
}

// GetCacheStats returns statistics about cache usage
func (c *Cache) GetCacheStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Query cache stats
	var queryCount, querySizeBytes int
	err := c.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(LENGTH(value_json)), 0)
		FROM query_cache
	`).Scan(&queryCount, &querySizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get query cache stats: %w", err)
	}

	// View cache stats
	var viewCount, viewSizeBytes int
	err = c.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(LENGTH(value_json)), 0)
		FROM view_cache
	`).Scan(&viewCount, &viewSizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get view cache stats: %w", err)
	}

	// Negative cache stats
	var negativeCount int
	err = c.db.QueryRow(`
		SELECT COUNT(*)
		FROM negative_cache
	`).Scan(&negativeCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get negative cache stats: %w", err)
	}

	stats["query_cache"] = map[string]interface{}{
		"entries":    queryCount,
		"size_bytes": querySizeBytes,
	}
	stats["view_cache"] = map[string]interface{}{
		"entries":    viewCount,
		"size_bytes": viewSizeBytes,
	}
	stats["negative_cache"] = map[string]interface{}{
		"entries": negativeCount,
	}

	return stats, nil
}

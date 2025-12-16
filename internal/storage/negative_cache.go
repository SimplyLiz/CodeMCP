package storage

import (
	"fmt"
)

// NegativeCacheErrorType represents the type of error stored in negative cache
type NegativeCacheErrorType string

const (
	// SymbolNotFound - symbol lookup failed (TTL 60s)
	SymbolNotFound NegativeCacheErrorType = "symbol-not-found"

	// BackendUnavailable - backend service is down or unreachable (TTL 15s)
	BackendUnavailable NegativeCacheErrorType = "backend-unavailable"

	// WorkspaceNotReady - LSP workspace not initialized (TTL 10s, triggers warmup)
	WorkspaceNotReady NegativeCacheErrorType = "workspace-not-ready"

	// Timeout - query timed out (TTL 5s)
	Timeout NegativeCacheErrorType = "timeout"

	// IndexNotFound - SCIP index not found (TTL 60s)
	IndexNotFound NegativeCacheErrorType = "index-not-found"

	// ParseError - failed to parse file or query (TTL 60s)
	ParseError NegativeCacheErrorType = "parse-error"
)

// NegativeCachePolicy defines TTL and behavior for each error type
type NegativeCachePolicy struct {
	TTLSeconds    int
	TriggerWarmup bool
	Description   string
}

// negativeCachePolicies maps error types to their policies per Section 9.2
var negativeCachePolicies = map[NegativeCacheErrorType]NegativeCachePolicy{
	SymbolNotFound: {
		TTLSeconds:    60,
		TriggerWarmup: false,
		Description:   "Symbol lookup failed - likely doesn't exist in codebase",
	},
	BackendUnavailable: {
		TTLSeconds:    15,
		TriggerWarmup: false,
		Description:   "Backend service is down or unreachable - retry after short delay",
	},
	WorkspaceNotReady: {
		TTLSeconds:    10,
		TriggerWarmup: true,
		Description:   "LSP workspace not initialized - trigger warmup and retry",
	},
	Timeout: {
		TTLSeconds:    5,
		TriggerWarmup: false,
		Description:   "Query timed out - retry with shorter timeout",
	},
	IndexNotFound: {
		TTLSeconds:    60,
		TriggerWarmup: false,
		Description:   "SCIP index not found - user needs to generate index",
	},
	ParseError: {
		TTLSeconds:    60,
		TriggerWarmup: false,
		Description:   "Failed to parse file or query - likely syntax error",
	},
}

// GetNegativeCachePolicy returns the policy for a given error type
func GetNegativeCachePolicy(errorType NegativeCacheErrorType) (NegativeCachePolicy, error) {
	policy, ok := negativeCachePolicies[errorType]
	if !ok {
		return NegativeCachePolicy{}, fmt.Errorf("unknown negative cache error type: %s", errorType)
	}
	return policy, nil
}

// GetNegativeCacheTTL returns the TTL in seconds for a given error type
func GetNegativeCacheTTL(errorType NegativeCacheErrorType) int {
	policy, err := GetNegativeCachePolicy(errorType)
	if err != nil {
		// Default to 60 seconds for unknown error types
		return 60
	}
	return policy.TTLSeconds
}

// ShouldTriggerWarmup returns whether the error type should trigger a warmup action
func ShouldTriggerWarmup(errorType NegativeCacheErrorType) bool {
	policy, err := GetNegativeCachePolicy(errorType)
	if err != nil {
		return false
	}
	return policy.TriggerWarmup
}

// NegativeCacheManager provides high-level negative cache operations
type NegativeCacheManager struct {
	cache *Cache
}

// NewNegativeCacheManager creates a new negative cache manager
func NewNegativeCacheManager(cache *Cache) *NegativeCacheManager {
	return &NegativeCacheManager{cache: cache}
}

// CacheError stores an error in the negative cache with appropriate TTL
func (m *NegativeCacheManager) CacheError(key string, errorType NegativeCacheErrorType, errorMessage string, stateID string) error {
	ttl := GetNegativeCacheTTL(errorType)

	if err := m.cache.SetNegativeCache(key, string(errorType), errorMessage, stateID, ttl); err != nil {
		return fmt.Errorf("failed to cache error: %w", err)
	}

	// Check if warmup should be triggered
	if ShouldTriggerWarmup(errorType) {
		m.cache.db.logger.Info("Warmup triggered by negative cache", map[string]interface{}{
			"error_type": errorType,
			"key":        key,
		})
		// TODO: Trigger warmup action (implement in later phase)
		// For now, just log the event
	}

	return nil
}

// CheckError checks if an error is cached and returns it if found
// Returns nil if not cached or expired
func (m *NegativeCacheManager) CheckError(key string, stateID string) (*NegativeCacheEntry, error) {
	entry, err := m.cache.GetNegativeCache(key, stateID)
	if err != nil {
		return nil, fmt.Errorf("failed to check negative cache: %w", err)
	}

	if entry != nil {
		m.cache.db.logger.Debug("Negative cache hit", map[string]interface{}{
			"key":        key,
			"error_type": entry.ErrorType,
		})
	}

	return entry, nil
}

// InvalidateError removes a specific error from the cache
func (m *NegativeCacheManager) InvalidateError(key string) error {
	if err := m.cache.InvalidateNegativeCache(key); err != nil {
		return fmt.Errorf("failed to invalidate error: %w", err)
	}
	return nil
}

// InvalidateAllErrors clears all negative cache entries
func (m *NegativeCacheManager) InvalidateAllErrors() error {
	if err := m.cache.InvalidateAllNegativeCache(); err != nil {
		return fmt.Errorf("failed to invalidate all errors: %w", err)
	}
	return nil
}

// GetErrorStats returns statistics about negative cache entries by error type
func (m *NegativeCacheManager) GetErrorStats() (map[string]int, error) {
	rows, err := m.cache.db.Query(`
		SELECT error_type, COUNT(*) as count
		FROM negative_cache
		GROUP BY error_type
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get error stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var errorType string
		var count int
		if err := rows.Scan(&errorType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan error stats: %w", err)
		}
		stats[errorType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating error stats: %w", err)
	}

	return stats, nil
}

// CleanupExpiredErrors removes expired negative cache entries
func (m *NegativeCacheManager) CleanupExpiredErrors() error {
	return m.cache.CleanupExpiredEntries()
}

package compression

import "ckb/internal/config"

// BackendLimits defines hard limits for backend operations to prevent resource exhaustion
type BackendLimits struct {
	// MaxRefsPerQuery limits the total number of references processed in a single query
	MaxRefsPerQuery int

	// MaxSymbolsPerSearch limits the number of symbols returned by a search operation
	MaxSymbolsPerSearch int

	// MaxFilesScanned limits the number of files that can be scanned
	MaxFilesScanned int

	// MaxFileSizeBytes limits the size of individual files that can be processed
	MaxFileSizeBytes int

	// MaxUnionModeTimeMs is the maximum time allowed for union mode queries (slower but more complete)
	MaxUnionModeTimeMs int

	// MaxScipIndexSizeMb is a warning threshold for SCIP index size (not a hard limit)
	MaxScipIndexSizeMb int
}

// DefaultLimits returns the default backend limits with safe, conservative values
func DefaultLimits() *BackendLimits {
	return &BackendLimits{
		MaxRefsPerQuery:     10000,
		MaxSymbolsPerSearch: 1000,
		MaxFilesScanned:     5000,
		MaxFileSizeBytes:    1_000_000, // 1MB
		MaxUnionModeTimeMs:  60000,     // 60 seconds
		MaxScipIndexSizeMb:  500,       // Warn if SCIP index > 500MB
	}
}

// LoadFromConfig creates a BackendLimits from configuration, using defaults for missing values
func (l *BackendLimits) LoadFromConfig(cfg *config.Config) *BackendLimits {
	if cfg == nil {
		return DefaultLimits()
	}

	limits := &BackendLimits{
		MaxRefsPerQuery:    cfg.BackendLimits.MaxRefsPerQuery,
		MaxFilesScanned:    cfg.BackendLimits.MaxFilesScanned,
		MaxUnionModeTimeMs: cfg.BackendLimits.MaxUnionModeTimeMs,
	}

	// Apply defaults for zero values
	if limits.MaxRefsPerQuery == 0 {
		limits.MaxRefsPerQuery = 10000
	}
	if limits.MaxFilesScanned == 0 {
		limits.MaxFilesScanned = 5000
	}
	if limits.MaxUnionModeTimeMs == 0 {
		limits.MaxUnionModeTimeMs = 60000
	}

	// These are not in config yet, use defaults
	limits.MaxSymbolsPerSearch = 1000
	limits.MaxFileSizeBytes = 1_000_000
	limits.MaxScipIndexSizeMb = 500

	return limits
}

// NewLimitsFromConfig creates new BackendLimits from a config file
func NewLimitsFromConfig(cfg *config.Config) *BackendLimits {
	return DefaultLimits().LoadFromConfig(cfg)
}

// IsScipIndexTooLarge checks if a SCIP index size exceeds the warning threshold
func (l *BackendLimits) IsScipIndexTooLarge(sizeBytes int64) bool {
	sizeMb := sizeBytes / (1024 * 1024)
	return int(sizeMb) > l.MaxScipIndexSizeMb
}

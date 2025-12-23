package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cacheFileName is the name of the update check cache file
const cacheFileName = "update-check.json"

// CacheEntry stores the cached update check result
type CacheEntry struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Cache handles caching of update check results
type Cache struct {
	path string
}

// NewCache creates a new cache using the default location (~/.ckb/update-check.json)
func NewCache() *Cache {
	return &Cache{
		path: getCachePath(),
	}
}

// getCachePath returns the path to the cache file
func getCachePath() string {
	// Try to use ~/.ckb directory
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".ckb", cacheFileName)
}

// Get returns the cached entry and whether it needs refresh.
// Returns (nil, true) if cache doesn't exist or is corrupted.
// Returns (entry, true) if cache exists but is stale.
// Returns (entry, false) if cache is fresh.
func (c *Cache) Get() (*CacheEntry, bool) {
	if c.path == "" {
		return nil, true
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, true
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, true
	}

	// Check if cache is stale
	isStale := time.Since(entry.CheckedAt) > checkInterval

	return &entry, isStale
}

// Set updates the cache with the latest version
func (c *Cache) Set(latestVersion string) {
	if c.path == "" {
		return
	}

	entry := CacheEntry{
		LatestVersion: latestVersion,
		CheckedAt:     time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	// Write atomically by writing to temp file first
	tmpPath := c.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}

	// Rename temp file to actual cache file
	_ = os.Rename(tmpPath, c.path)
}

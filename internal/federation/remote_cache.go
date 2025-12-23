// Package federation provides cross-repository federation capabilities for CKB.
// This file provides a caching wrapper for remote client queries (Phase 5).
package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/logging"
)

// Cache TTL defaults
const (
	CacheTTLRepoList     = time.Hour
	CacheTTLMetadata     = time.Hour
	CacheTTLSymbolSearch = 15 * time.Minute
	CacheTTLFileSearch   = 15 * time.Minute
	// Refs and callgraph are not cached (always fresh)
)

// CachedRemoteClient wraps a RemoteClient with caching support.
type CachedRemoteClient struct {
	client *RemoteClient
	index  *Index
	logger *logging.Logger
}

// NewCachedRemoteClient creates a new cached remote client.
func NewCachedRemoteClient(client *RemoteClient, index *Index, logger *logging.Logger) *CachedRemoteClient {
	return &CachedRemoteClient{
		client: client,
		index:  index,
		logger: logger,
	}
}

// Client returns the underlying remote client.
func (c *CachedRemoteClient) Client() *RemoteClient {
	return c.client
}

// Server returns the remote server configuration.
func (c *CachedRemoteClient) Server() *RemoteServer {
	return c.client.Server()
}

// cacheKey generates a cache key from components.
func cacheKey(parts ...string) string {
	data := ""
	for _, p := range parts {
		data += p + ":"
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}

// getFromCache retrieves a cached value and unmarshals it.
func (c *CachedRemoteClient) getFromCache(repoID, key string, target interface{}) (bool, error) {
	data, found, err := c.index.GetRemoteCacheEntry(c.Server().Name, repoID, key)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if unmarshalErr := json.Unmarshal(data, target); unmarshalErr != nil {
		return false, nil //nolint:nilerr // intentional: unmarshal error = cache miss, not failure
	}
	return true, nil
}

// setInCache marshals a value and stores it in cache.
func (c *CachedRemoteClient) setInCache(repoID, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.index.SetRemoteCacheEntry(c.Server().Name, repoID, key, data, ttl)
}

// ListRepos lists repositories with caching.
func (c *CachedRemoteClient) ListRepos(ctx context.Context) ([]RemoteRepoInfo, error) {
	key := cacheKey("repos", "list")

	// Try cache first
	var cached []RemoteRepoInfo
	if found, _ := c.getFromCache("", key, &cached); found {
		if c.logger != nil {
			c.logger.Debug("Remote repos from cache", map[string]interface{}{
				"server": c.Server().Name,
				"count":  len(cached),
			})
		}
		return cached, nil
	}

	// Fetch from remote
	repos, err := c.client.ListRepos(ctx)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if cacheErr := c.setInCache("", key, repos, CacheTTLRepoList); cacheErr != nil && c.logger != nil {
		c.logger.Warn("Failed to cache repo list", map[string]interface{}{
			"server": c.Server().Name,
			"error":  cacheErr.Error(),
		})
	}

	// Update cached repo info in the index
	for _, repo := range repos {
		languages, _ := json.Marshal(repo.Languages)
		if upsertErr := c.index.UpsertRemoteRepo(&CachedRemoteRepo{
			ServerName:   c.Server().Name,
			RepoID:       repo.ID,
			Name:         repo.Name,
			Description:  repo.Description,
			Commit:       repo.Commit,
			Languages:    string(languages),
			SymbolCount:  repo.SymbolCount,
			FileCount:    repo.FileCount,
			SyncSeq:      repo.SyncSeq,
			IndexVersion: repo.IndexVersion,
			CachedAt:     time.Now(),
		}); upsertErr != nil && c.logger != nil {
			c.logger.Warn("Failed to cache repo info", map[string]interface{}{
				"server": c.Server().Name,
				"repo":   repo.ID,
				"error":  upsertErr.Error(),
			})
		}
	}

	return repos, nil
}

// GetRepoMeta gets repository metadata with caching.
func (c *CachedRemoteClient) GetRepoMeta(ctx context.Context, repoID string) (*RemoteRepoMeta, error) {
	key := cacheKey("meta", repoID)

	// Try cache first
	var cached RemoteRepoMeta
	if found, _ := c.getFromCache(repoID, key, &cached); found {
		if c.logger != nil {
			c.logger.Debug("Remote meta from cache", map[string]interface{}{
				"server": c.Server().Name,
				"repo":   repoID,
			})
		}
		return &cached, nil
	}

	// Fetch from remote
	meta, err := c.client.GetRepoMeta(ctx, repoID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if cacheErr := c.setInCache(repoID, key, meta, CacheTTLMetadata); cacheErr != nil && c.logger != nil {
		c.logger.Warn("Failed to cache repo meta", map[string]interface{}{
			"server": c.Server().Name,
			"repo":   repoID,
			"error":  cacheErr.Error(),
		})
	}

	return meta, nil
}

// SearchSymbols searches for symbols with caching.
func (c *CachedRemoteClient) SearchSymbols(ctx context.Context, repoID string, opts *RemoteSymbolSearchOptions) ([]RemoteSymbol, bool, error) {
	key := cacheKey("search", "symbols", repoID, opts.Query, opts.Language, opts.Kind, fmt.Sprintf("%d", opts.Limit))

	// Try cache first
	type cachedResult struct {
		Symbols   []RemoteSymbol `json:"symbols"`
		Truncated bool           `json:"truncated"`
	}
	var cached cachedResult
	if found, _ := c.getFromCache(repoID, key, &cached); found {
		if c.logger != nil {
			c.logger.Debug("Remote symbol search from cache", map[string]interface{}{
				"server": c.Server().Name,
				"repo":   repoID,
				"query":  opts.Query,
				"count":  len(cached.Symbols),
			})
		}
		return cached.Symbols, cached.Truncated, nil
	}

	// Fetch from remote
	symbols, truncated, err := c.client.SearchSymbols(ctx, repoID, opts)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	cacheErr := c.setInCache(repoID, key, cachedResult{Symbols: symbols, Truncated: truncated}, CacheTTLSymbolSearch)
	if cacheErr != nil && c.logger != nil {
		c.logger.Warn("Failed to cache symbol search", map[string]interface{}{
			"server": c.Server().Name,
			"repo":   repoID,
			"error":  cacheErr.Error(),
		})
	}

	return symbols, truncated, nil
}

// SearchFiles searches for files with caching.
func (c *CachedRemoteClient) SearchFiles(ctx context.Context, repoID, query string, limit int) ([]RemoteFile, bool, error) {
	key := cacheKey("search", "files", repoID, query, fmt.Sprintf("%d", limit))

	// Try cache first
	type cachedResult struct {
		Files     []RemoteFile `json:"files"`
		Truncated bool         `json:"truncated"`
	}
	var cached cachedResult
	if found, _ := c.getFromCache(repoID, key, &cached); found {
		if c.logger != nil {
			c.logger.Debug("Remote file search from cache", map[string]interface{}{
				"server": c.Server().Name,
				"repo":   repoID,
				"query":  query,
				"count":  len(cached.Files),
			})
		}
		return cached.Files, cached.Truncated, nil
	}

	// Fetch from remote
	files, truncated, err := c.client.SearchFiles(ctx, repoID, query, limit)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	cacheErr := c.setInCache(repoID, key, cachedResult{Files: files, Truncated: truncated}, CacheTTLFileSearch)
	if cacheErr != nil && c.logger != nil {
		c.logger.Warn("Failed to cache file search", map[string]interface{}{
			"server": c.Server().Name,
			"repo":   repoID,
			"error":  cacheErr.Error(),
		})
	}

	return files, truncated, nil
}

// GetSymbol gets a symbol (no caching - should be fast enough).
func (c *CachedRemoteClient) GetSymbol(ctx context.Context, repoID, symbolID string) (*RemoteSymbol, error) {
	return c.client.GetSymbol(ctx, repoID, symbolID)
}

// BatchGetSymbols gets multiple symbols (no caching).
func (c *CachedRemoteClient) BatchGetSymbols(ctx context.Context, repoID string, ids []string) ([]RemoteSymbol, []string, error) {
	return c.client.BatchGetSymbols(ctx, repoID, ids)
}

// ListSymbols lists symbols (no caching - use pagination).
func (c *CachedRemoteClient) ListSymbols(ctx context.Context, repoID string, opts *RemoteSymbolListOptions) ([]RemoteSymbol, string, int, error) {
	return c.client.ListSymbols(ctx, repoID, opts)
}

// ListRefs lists references (no caching - always fresh).
func (c *CachedRemoteClient) ListRefs(ctx context.Context, repoID string, opts *RemoteRefOptions) ([]RemoteRef, string, error) {
	return c.client.ListRefs(ctx, repoID, opts)
}

// ListCallGraph lists call graph edges (no caching - always fresh).
func (c *CachedRemoteClient) ListCallGraph(ctx context.Context, repoID string, opts *RemoteCallGraphOptions) ([]RemoteCallEdge, string, error) {
	return c.client.ListCallGraph(ctx, repoID, opts)
}

// ListFiles lists files (no caching - use pagination).
func (c *CachedRemoteClient) ListFiles(ctx context.Context, repoID string, limit int, cursor string) ([]RemoteFile, string, error) {
	return c.client.ListFiles(ctx, repoID, limit, cursor)
}

// Ping checks server connectivity.
func (c *CachedRemoteClient) Ping(ctx context.Context) error {
	return c.client.Ping(ctx)
}

// InvalidateCache invalidates all cached data for a repository.
func (c *CachedRemoteClient) InvalidateCache(repoID string) error {
	return c.index.ClearRemoteCache(c.Server().Name)
}

// InvalidateAll invalidates all cached data for this server.
func (c *CachedRemoteClient) InvalidateAll() error {
	return c.index.ClearRemoteCache(c.Server().Name)
}

// Package federation provides cross-repository federation capabilities for CKB.
// This file provides hybrid queries combining local and remote results (Phase 5).
package federation

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// HybridEngine provides hybrid queries across local federation and remote servers.
type HybridEngine struct {
	federation *Federation
	remotes    map[string]*CachedRemoteClient
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewHybridEngine creates a new hybrid query engine.
func NewHybridEngine(federation *Federation, logger *slog.Logger) *HybridEngine {
	return &HybridEngine{
		federation: federation,
		remotes:    make(map[string]*CachedRemoteClient),
		logger:     logger,
	}
}

// InitRemoteClients initializes clients for all enabled remote servers.
func (h *HybridEngine) InitRemoteClients() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	servers := h.federation.GetEnabledRemoteServers()
	for _, server := range servers {
		serverCopy := server // Copy to avoid closure issues
		client := NewRemoteClient(&serverCopy, h.federation.Index(), h.logger)
		h.remotes[server.Name] = NewCachedRemoteClient(client, h.federation.Index(), h.logger)
	}

	return nil
}

// GetRemoteClient returns the cached client for a server.
func (h *HybridEngine) GetRemoteClient(serverName string) *CachedRemoteClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.remotes[serverName]
}

// RefreshRemoteClients updates the remote client list based on current config.
func (h *HybridEngine) RefreshRemoteClients() {
	h.mu.Lock()
	defer h.mu.Unlock()

	servers := h.federation.GetEnabledRemoteServers()
	serverMap := make(map[string]bool)

	for _, server := range servers {
		serverMap[server.Name] = true

		// Add new servers
		if _, exists := h.remotes[server.Name]; !exists {
			serverCopy := server
			client := NewRemoteClient(&serverCopy, h.federation.Index(), h.logger)
			h.remotes[server.Name] = NewCachedRemoteClient(client, h.federation.Index(), h.logger)
		}
	}

	// Remove disabled servers
	for name := range h.remotes {
		if !serverMap[name] {
			delete(h.remotes, name)
		}
	}
}

// HybridSearchOptions contains options for hybrid symbol searches.
type HybridSearchOptions struct {
	Query        string
	Limit        int
	Language     string
	Kind         string
	IncludeLocal bool     // Include local federation repos
	Servers      []string // Specific servers to query (empty = all enabled)
	Timeout      time.Duration
}

// SearchSymbols searches for symbols across local and remote sources.
func (h *HybridEngine) SearchSymbols(ctx context.Context, opts HybridSearchOptions) (*HybridSearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.IncludeLocal {
		// Default to including local
		opts.IncludeLocal = true
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var wg sync.WaitGroup
	resultsCh := make(chan hybridSearchBatch, 10)
	errorsCh := make(chan QueryError, 10)

	// Query remote servers
	h.mu.RLock()
	remotes := make(map[string]*CachedRemoteClient)
	for name, client := range h.remotes {
		if len(opts.Servers) == 0 || contains(opts.Servers, name) {
			remotes[name] = client
		}
	}
	h.mu.RUnlock()

	for serverName, client := range remotes {
		wg.Add(1)
		go func(name string, c *CachedRemoteClient) {
			defer wg.Done()
			h.queryRemoteForSymbols(ctx, name, c, opts, resultsCh, errorsCh)
		}(serverName, client)
	}

	// Close channels when done
	go func() {
		wg.Wait()
		close(resultsCh)
		close(errorsCh)
	}()

	// Collect results
	var allResults []HybridSymbolResult
	var sources []QuerySource
	var errors []QueryError

	for batch := range resultsCh {
		allResults = append(allResults, batch.results...)
		sources = append(sources, batch.source)
	}

	for err := range errorsCh {
		errors = append(errors, err)
	}

	// Deduplicate and limit results
	if len(allResults) > opts.Limit {
		allResults = allResults[:opts.Limit]
	}

	return &HybridSearchResult{
		Results:   allResults,
		Sources:   sources,
		Errors:    errors,
		Truncated: len(allResults) >= opts.Limit,
	}, nil
}

type hybridSearchBatch struct {
	results []HybridSymbolResult
	source  QuerySource
}

func (h *HybridEngine) queryRemoteForSymbols(
	ctx context.Context,
	serverName string,
	client *CachedRemoteClient,
	opts HybridSearchOptions,
	results chan<- hybridSearchBatch,
	errors chan<- QueryError,
) {
	start := time.Now()
	server := client.Server()

	// Get repos from this server
	repos, err := client.ListRepos(ctx)
	if err != nil {
		errors <- QueryError{
			Source:  serverName,
			URL:     server.URL,
			Code:    "list_repos_failed",
			Message: err.Error(),
		}
		return
	}

	var allResults []HybridSymbolResult

	// Search each repo
	for _, repo := range repos {
		symbols, _, err := client.SearchSymbols(ctx, repo.ID, &RemoteSymbolSearchOptions{
			Query:    opts.Query,
			Limit:    opts.Limit / len(repos), // Distribute limit
			Language: opts.Language,
			Kind:     opts.Kind,
		})

		if err != nil {
			if h.logger != nil {
				h.logger.Warn("Failed to search remote repo",
					"server", serverName,
					"repo", repo.ID,
					"error", err.Error(),
				)
			}
			continue
		}

		for _, sym := range symbols {
			allResults = append(allResults, HybridSymbolResult{
				Symbol:     sym,
				Source:     serverName,
				RepoID:     repo.ID,
				ServerURL:  server.URL,
				ServerName: serverName,
			})
		}
	}

	results <- hybridSearchBatch{
		results: allResults,
		source: QuerySource{
			Name:        serverName,
			URL:         server.URL,
			Status:      "success",
			ResultCount: len(allResults),
			Latency:     time.Since(start),
		},
	}
}

// SyncRemote syncs metadata from a remote server.
func (h *HybridEngine) SyncRemote(ctx context.Context, serverName string) error {
	h.mu.RLock()
	client, ok := h.remotes[serverName]
	h.mu.RUnlock()

	if !ok {
		// Try to create client for this server
		server := h.federation.GetRemoteServer(serverName)
		if server == nil {
			return &RemoteError{Code: "server_not_found", Message: "Remote server not found: " + serverName}
		}
		rawClient := NewRemoteClient(server, h.federation.Index(), h.logger)
		client = NewCachedRemoteClient(rawClient, h.federation.Index(), h.logger)

		h.mu.Lock()
		h.remotes[serverName] = client
		h.mu.Unlock()
	}

	// Fetch repo list
	repos, err := client.ListRepos(ctx)
	if err != nil {
		if setErr := h.federation.SetRemoteServerError(serverName, err.Error()); setErr != nil && h.logger != nil {
			h.logger.Warn("Failed to set server error",
				"server", serverName,
				"error", setErr.Error(),
			)
		}
		return err
	}

	// Update cached repo info
	for _, repo := range repos {
		languages, _ := json.Marshal(repo.Languages)
		if upsertErr := h.federation.Index().UpsertRemoteRepo(&CachedRemoteRepo{
			ServerName:   serverName,
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
		}); upsertErr != nil && h.logger != nil {
			h.logger.Warn("Failed to cache repo info",
				"server", serverName,
				"repo", repo.ID,
				"error", upsertErr.Error(),
			)
		}
	}

	// Mark server as synced
	if setErr := h.federation.SetRemoteServerSynced(serverName); setErr != nil && h.logger != nil {
		h.logger.Warn("Failed to set server synced",
			"server", serverName,
			"error", setErr.Error(),
		)
	}

	if h.logger != nil {
		h.logger.Info("Synced remote server",
			"server", serverName,
			"repo_count", len(repos),
		)
	}

	return nil
}

// SyncAllRemotes syncs metadata from all enabled remote servers.
func (h *HybridEngine) SyncAllRemotes(ctx context.Context) []QueryError {
	var errors []QueryError
	var wg sync.WaitGroup
	var mu sync.Mutex

	servers := h.federation.GetEnabledRemoteServers()
	for _, server := range servers {
		wg.Add(1)
		go func(s RemoteServer) {
			defer wg.Done()
			if err := h.SyncRemote(ctx, s.Name); err != nil {
				mu.Lock()
				errors = append(errors, QueryError{
					Source:  s.Name,
					URL:     s.URL,
					Code:    "sync_failed",
					Message: err.Error(),
				})
				mu.Unlock()
			}
		}(server)
	}

	wg.Wait()
	return errors
}

// GetRemoteStatus returns status information for a remote server.
func (h *HybridEngine) GetRemoteStatus(ctx context.Context, serverName string) (*RemoteServerStatus, error) {
	server := h.federation.GetRemoteServer(serverName)
	if server == nil {
		return nil, &RemoteError{Code: "server_not_found", Message: "Remote server not found: " + serverName}
	}

	h.mu.RLock()
	client := h.remotes[serverName]
	h.mu.RUnlock()

	status := &RemoteServerStatus{
		Name:         server.Name,
		URL:          server.URL,
		Enabled:      server.Enabled,
		LastSyncedAt: server.LastSyncedAt,
		LastError:    server.LastError,
	}

	// Try to ping the server
	if client != nil {
		start := time.Now()
		if err := client.Ping(ctx); err != nil {
			status.Online = false
			status.PingError = err.Error()
		} else {
			status.Online = true
			status.Latency = time.Since(start)
		}
	}

	// Get cached repo count
	repos, err := h.federation.Index().GetRemoteRepos(serverName)
	if err == nil {
		status.CachedRepoCount = len(repos)
	}

	return status, nil
}

// RemoteServerStatus contains status information for a remote server.
type RemoteServerStatus struct {
	Name            string        `json:"name"`
	URL             string        `json:"url"`
	Enabled         bool          `json:"enabled"`
	Online          bool          `json:"online"`
	Latency         time.Duration `json:"latency,omitempty"`
	PingError       string        `json:"ping_error,omitempty"`
	LastSyncedAt    *time.Time    `json:"last_synced_at,omitempty"`
	LastError       string        `json:"last_error,omitempty"`
	CachedRepoCount int           `json:"cached_repo_count"`
}

// ListAllRepos lists repos from local federation and all remote servers.
func (h *HybridEngine) ListAllRepos(ctx context.Context) (*HybridRepoList, error) {
	result := &HybridRepoList{}

	// Local repos
	localRepos := h.federation.ListRepos()
	for _, repo := range localRepos {
		result.LocalRepos = append(result.LocalRepos, HybridRepoInfo{
			RepoID:     repo.RepoID,
			Path:       repo.Path,
			Tags:       repo.Tags,
			Source:     "local",
			ServerName: "",
		})
	}

	// Remote repos
	h.mu.RLock()
	remotes := make(map[string]*CachedRemoteClient)
	for name, client := range h.remotes {
		remotes[name] = client
	}
	h.mu.RUnlock()

	for serverName, client := range remotes {
		repos, err := client.ListRepos(ctx)
		if err != nil {
			result.Errors = append(result.Errors, QueryError{
				Source:  serverName,
				URL:     client.Server().URL,
				Code:    "list_repos_failed",
				Message: err.Error(),
			})
			continue
		}

		for _, repo := range repos {
			result.RemoteRepos = append(result.RemoteRepos, HybridRepoInfo{
				RepoID:      repo.ID,
				Name:        repo.Name,
				Description: repo.Description,
				Languages:   repo.Languages,
				SymbolCount: repo.SymbolCount,
				FileCount:   repo.FileCount,
				Source:      "remote",
				ServerName:  serverName,
				ServerURL:   client.Server().URL,
			})
		}
	}

	return result, nil
}

// HybridRepoList contains repos from local and remote sources.
type HybridRepoList struct {
	LocalRepos  []HybridRepoInfo `json:"local_repos"`
	RemoteRepos []HybridRepoInfo `json:"remote_repos"`
	Errors      []QueryError     `json:"errors,omitempty"`
}

// HybridRepoInfo represents a repo from either local or remote source.
type HybridRepoInfo struct {
	RepoID      string   `json:"repo_id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Path        string   `json:"path,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	SymbolCount int      `json:"symbol_count,omitempty"`
	FileCount   int      `json:"file_count,omitempty"`
	Source      string   `json:"source"` // "local" or "remote"
	ServerName  string   `json:"server_name,omitempty"`
	ServerURL   string   `json:"server_url,omitempty"`
}

// helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

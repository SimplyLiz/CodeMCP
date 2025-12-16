package query

import (
	"context"
	"runtime"
	"strconv"
	"time"
)

// StatusResponse is the response for getStatus.
type StatusResponse struct {
	CkbVersion      string          `json:"ckbVersion"`
	Healthy         bool            `json:"healthy"`
	RepoState       *RepoState      `json:"repoState"`
	Backends        []BackendStatus `json:"backends"`
	Cache           *CacheStatus    `json:"cache"`
	QueryDurationMs int64           `json:"queryDurationMs"`
}

// BackendStatus describes the status of a backend.
type BackendStatus struct {
	Id           string   `json:"id"`
	Available    bool     `json:"available"`
	Healthy      bool     `json:"healthy"`
	Capabilities []string `json:"capabilities"`
	Details      string   `json:"details,omitempty"`
	Warning      string   `json:"warning,omitempty"`
}

// CacheStatus describes the cache state.
type CacheStatus struct {
	QueriesCached int     `json:"queriesCached"`
	ViewsCached   int     `json:"viewsCached"`
	HitRate       float64 `json:"hitRate"`
	SizeBytes     int64   `json:"sizeBytes"`
}

// GetStatus returns the current system status.
func (e *Engine) GetStatus(ctx context.Context) (*StatusResponse, error) {
	startTime := time.Now()

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		e.logger.Warn("failed to get repo state", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Get backend statuses
	backendStatuses := e.getBackendStatuses(ctx)

	// Determine overall health
	healthy := true
	for _, bs := range backendStatuses {
		if bs.Available && !bs.Healthy {
			healthy = false
			break
		}
	}

	// Get cache status
	cacheStatus := e.getCacheStatus()

	return &StatusResponse{
		CkbVersion:      "0.1.0",
		Healthy:         healthy,
		RepoState:       repoState,
		Backends:        backendStatuses,
		Cache:           cacheStatus,
		QueryDurationMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// getBackendStatuses returns the status of all backends.
func (e *Engine) getBackendStatuses(ctx context.Context) []BackendStatus {
	statuses := make([]BackendStatus, 0)

	// SCIP backend
	scipStatus := BackendStatus{
		Id:           "scip",
		Capabilities: []string{"symbol-search", "find-references", "goto-definition"},
	}
	if e.scipAdapter != nil {
		scipStatus.Available = e.scipAdapter.IsAvailable()
		scipStatus.Healthy = scipStatus.Available
		if !scipStatus.Available {
			scipStatus.Details = "Index not found. Run SCIP indexer to create."
		} else {
			info := e.scipAdapter.GetIndexInfo()
			if info != nil {
				scipStatus.Details = "Symbols: " + strconv.Itoa(info.SymbolCount) + ", Documents: " + strconv.Itoa(info.DocumentCount)
			}
		}
	}
	statuses = append(statuses, scipStatus)

	// LSP backend
	lspStatus := BackendStatus{
		Id:           "lsp",
		Capabilities: []string{"symbol-search", "find-references", "goto-definition", "hover"},
	}
	if e.lspSupervisor != nil {
		stats := e.lspSupervisor.GetStats()
		if processCount, ok := stats["totalProcesses"].(int); ok {
			lspStatus.Available = processCount > 0
			lspStatus.Healthy = lspStatus.Available
			lspStatus.Details = formatServerCount(processCount)
		}
	}
	statuses = append(statuses, lspStatus)

	// Git backend
	gitStatus := BackendStatus{
		Id:           "git",
		Capabilities: []string{"blame", "history", "churn"},
	}
	if e.gitAdapter != nil {
		gitStatus.Available = e.gitAdapter.IsAvailable()
		gitStatus.Healthy = gitStatus.Available
		gitStatus.Details = "Ready"
	}
	statuses = append(statuses, gitStatus)

	return statuses
}

// getCacheStatus returns the current cache status.
func (e *Engine) getCacheStatus() *CacheStatus {
	if e.db == nil {
		return &CacheStatus{}
	}
	// Query basic stats from DB
	var queryCount int
	var sizeBytes int64
	row := e.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(LENGTH(value_json)), 0) FROM query_cache")
	if err := row.Scan(&queryCount, &sizeBytes); err != nil {
		return &CacheStatus{}
	}
	return &CacheStatus{
		QueriesCached: queryCount,
		SizeBytes:     sizeBytes,
	}
}

// formatServerCount formats the LSP server count message.
func formatServerCount(count int) string {
	if count == 0 {
		return "No servers running"
	}
	if count == 1 {
		return "1 server configured"
	}
	return strconv.Itoa(count) + " server(s) configured"
}

// SystemInfo contains system information for diagnostics.
type SystemInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
	NumCPU    int    `json:"numCpu"`
}

// GetSystemInfo returns system information.
func (e *Engine) GetSystemInfo() *SystemInfo {
	return &SystemInfo{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		NumCPU:    runtime.NumCPU(),
	}
}

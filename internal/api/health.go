package api

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"

	"ckb/internal/version"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// ReadyResponse represents the readiness check response
type ReadyResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Backends  map[string]bool   `json:"backends"`
	Details   map[string]string `json:"details,omitempty"`
}

// DetailedHealthResponse represents an enhanced health check response
type DetailedHealthResponse struct {
	Status    string              `json:"status"`
	Timestamp time.Time           `json:"timestamp"`
	Version   string              `json:"version"`
	Uptime    string              `json:"uptime,omitempty"`
	Repos     *RepoHealthInfo     `json:"repos,omitempty"`
	Storage   *StorageHealthInfo  `json:"storage,omitempty"`
	Journal   *JournalHealthInfo  `json:"journal,omitempty"`
	Memory    *MemoryHealthInfo   `json:"memory,omitempty"`
	Backends  []BackendHealthInfo `json:"backends,omitempty"`
	Warnings  []string            `json:"warnings,omitempty"`
}

// RepoHealthInfo contains repository health information
type RepoHealthInfo struct {
	TotalRepos      int    `json:"totalRepos"`
	IndexedRepos    int    `json:"indexedRepos"`
	StaleRepos      int    `json:"staleRepos"`
	LastIndexedAt   string `json:"lastIndexedAt,omitempty"`
	LastIndexCommit string `json:"lastIndexCommit,omitempty"`
}

// StorageHealthInfo contains storage health information
type StorageHealthInfo struct {
	DatabasePath      string `json:"databasePath"`
	DatabaseSizeBytes int64  `json:"databaseSizeBytes"`
	WalSizeBytes      int64  `json:"walSizeBytes"`
	SnapshotCount     int    `json:"snapshotCount"`
	FTSEnabled        bool   `json:"ftsEnabled"`
	FTSSymbolCount    int    `json:"ftsSymbolCount,omitempty"`
}

// JournalHealthInfo contains journal health information
type JournalHealthInfo struct {
	Enabled           bool   `json:"enabled"`
	EntryCount        int    `json:"entryCount"`
	OldestEntry       string `json:"oldestEntry,omitempty"`
	PendingCompaction bool   `json:"pendingCompaction"`
}

// MemoryHealthInfo contains memory usage information
type MemoryHealthInfo struct {
	AllocMB      float64 `json:"allocMb"`
	TotalAllocMB float64 `json:"totalAllocMb"`
	SysMB        float64 `json:"sysMb"`
	NumGC        uint32  `json:"numGc"`
	NumGoroutine int     `json:"numGoroutine"`
}

// BackendHealthInfo contains backend health information
type BackendHealthInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Available bool   `json:"available"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// handleHealth responds to health check requests (simple liveness check)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Version:   version.Version,
	}

	WriteJSON(w, response, http.StatusOK)
}

// handleReady responds to readiness check requests (checks backend availability)
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Actually check backend availability
	// For now, return a placeholder response
	backends := map[string]bool{
		"scip": true, // Placeholder
		"lsp":  true, // Placeholder
		"git":  true, // Placeholder
	}

	// Determine overall readiness
	ready := true
	for _, available := range backends {
		if !available {
			ready = false
			break
		}
	}

	status := "ready"
	statusCode := http.StatusOK
	if !ready {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := ReadyResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Backends:  backends,
	}

	WriteJSON(w, response, statusCode)
}

// handleHealthDetailed responds to detailed health check requests
func (s *Server) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	response := DetailedHealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Version:   version.Version,
	}

	var warnings []string

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	response.Memory = &MemoryHealthInfo{
		AllocMB:      float64(memStats.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(memStats.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(memStats.Sys) / 1024 / 1024,
		NumGC:        memStats.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
	}

	// Get storage info
	storage := s.getStorageHealthInfo(ctx)
	response.Storage = storage

	// Get backend health from status
	statusResp, err := s.engine.GetStatus(ctx)
	if err == nil {
		backends := make([]BackendHealthInfo, 0, len(statusResp.Backends))
		allHealthy := true
		for _, b := range statusResp.Backends {
			backends = append(backends, BackendHealthInfo{
				ID:        b.Id,
				Type:      b.Id,
				Available: b.Available,
				Healthy:   b.Healthy,
			})
			if !b.Healthy {
				allHealthy = false
			}
		}
		response.Backends = backends

		if !allHealthy {
			response.Status = "degraded"
			warnings = append(warnings, "One or more backends are unhealthy")
		}

		// Get repo info
		if statusResp.RepoState != nil {
			response.Repos = &RepoHealthInfo{
				TotalRepos:      1,
				IndexedRepos:    1,
				LastIndexCommit: statusResp.RepoState.HeadCommit,
			}
		}
	} else {
		response.Status = "degraded"
		warnings = append(warnings, "Could not fetch status: "+err.Error())
	}

	// Get journal info
	journal := s.getJournalHealthInfo(ctx)
	response.Journal = journal

	if len(warnings) > 0 {
		response.Warnings = warnings
	}

	statusCode := http.StatusOK
	if response.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	WriteJSON(w, response, statusCode)
}

// getStorageHealthInfo gathers storage health information
func (s *Server) getStorageHealthInfo(ctx context.Context) *StorageHealthInfo {
	info := &StorageHealthInfo{
		FTSEnabled: true, // Default assumption
	}

	// Try to find the database path
	ckbDir := ".ckb"
	if wd, err := os.Getwd(); err == nil {
		ckbDir = filepath.Join(wd, ".ckb")
	}

	dbPath := filepath.Join(ckbDir, "ckb.db")
	info.DatabasePath = dbPath

	// Get database size
	if stat, err := os.Stat(dbPath); err == nil {
		info.DatabaseSizeBytes = stat.Size()
	}

	// Get WAL size
	walPath := dbPath + "-wal"
	if stat, err := os.Stat(walPath); err == nil {
		info.WalSizeBytes = stat.Size()
	}

	// Count snapshots
	if matches, err := filepath.Glob(filepath.Join(ckbDir, "snapshot_*.db")); err == nil {
		info.SnapshotCount = len(matches)
	}

	// Get FTS symbol count
	db, err := sql.Open("sqlite", dbPath)
	if err == nil {
		defer func() { _ = db.Close() }()

		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM symbols_fts_content").Scan(&count)
		if err == nil {
			info.FTSSymbolCount = count
		} else {
			info.FTSEnabled = false
		}
	}

	return info
}

// getJournalHealthInfo gathers journal health information
func (s *Server) getJournalHealthInfo(ctx context.Context) *JournalHealthInfo {
	info := &JournalHealthInfo{
		Enabled: false,
	}

	ckbDir := ".ckb"
	if wd, err := os.Getwd(); err == nil {
		ckbDir = filepath.Join(wd, ".ckb")
	}

	dbPath := filepath.Join(ckbDir, "ckb.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return info
	}
	defer func() { _ = db.Close() }()

	// Check if change_journal table exists
	var tableName string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='change_journal'").Scan(&tableName)
	if err != nil {
		return info
	}

	info.Enabled = true

	// Get entry count
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM change_journal").Scan(&count); err == nil {
		info.EntryCount = count
	}

	// Get oldest entry
	var oldest string
	if err := db.QueryRowContext(ctx, "SELECT MIN(timestamp) FROM change_journal").Scan(&oldest); err == nil && oldest != "" {
		info.OldestEntry = oldest

		// Check if compaction is needed (entries older than 7 days)
		if t, err := time.Parse(time.RFC3339, oldest); err == nil {
			if time.Since(t) > 7*24*time.Hour {
				info.PendingCompaction = true
			}
		}
	}

	return info
}

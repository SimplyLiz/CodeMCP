package api

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"ckb/internal/logging"
)

// IndexRepoHandle holds an open connection to a repo's index database
type IndexRepoHandle struct {
	ID     string
	Config *IndexRepoConfig
	db     *sql.DB
	meta   *IndexRepoMetadata
	mu     sync.RWMutex
}

// IndexRepoManager manages repo handles for index serving
type IndexRepoManager struct {
	repos   map[string]*IndexRepoHandle
	config  *IndexServerConfig
	logger  *logging.Logger
	cursor  *CursorManager
	mu      sync.RWMutex
}

// NewIndexRepoManager creates a new repo manager with connections to all configured repos
func NewIndexRepoManager(config *IndexServerConfig, logger *logging.Logger) (*IndexRepoManager, error) {
	m := &IndexRepoManager{
		repos:  make(map[string]*IndexRepoHandle),
		config: config,
		logger: logger,
		cursor: NewCursorManager(config.CursorSecret),
	}

	// Open connections to all configured repos
	for _, repoConfig := range config.Repos {
		handle, err := m.openRepo(repoConfig)
		if err != nil {
			// Close any already-opened repos before returning
			m.Close()
			return nil, fmt.Errorf("failed to open repo %s: %w", repoConfig.ID, err)
		}
		m.repos[repoConfig.ID] = handle
		logger.Info("Opened index repo", map[string]interface{}{
			"repo_id": repoConfig.ID,
			"path":    repoConfig.Path,
		})
	}

	return m, nil
}

// openRepo opens a read-only connection to a repo's database
func (m *IndexRepoManager) openRepo(config IndexRepoConfig) (*IndexRepoHandle, error) {
	// Database path
	dbPath := filepath.Join(config.Path, ".ckb", "ckb.db")

	// Open in read-only mode
	connStr := fmt.Sprintf("file:%s?mode=ro", dbPath)
	conn, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set read-only pragmas
	pragmas := []string{
		"PRAGMA query_only=ON",       // Enforce read-only at SQLite level
		"PRAGMA busy_timeout=5000",   // Wait up to 5 seconds on lock
		"PRAGMA cache_size=-32000",   // 32MB cache (smaller for read-only)
		"PRAGMA temp_store=MEMORY",   // Use memory for temp tables
		"PRAGMA mmap_size=134217728", // 128MB mmap
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	// Verify connection works
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	handle := &IndexRepoHandle{
		ID:     config.ID,
		Config: &config,
		db:     conn,
	}

	// Load initial metadata
	if err := handle.refreshMeta(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	return handle, nil
}

// GetRepo returns a repo handle by ID
func (m *IndexRepoManager) GetRepo(id string) (*IndexRepoHandle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	handle, ok := m.repos[id]
	if !ok {
		return nil, fmt.Errorf("repo not found: %s", id)
	}
	return handle, nil
}

// ListRepos returns all repo handles
func (m *IndexRepoManager) ListRepos() []*IndexRepoHandle {
	m.mu.RLock()
	defer m.mu.RUnlock()

	handles := make([]*IndexRepoHandle, 0, len(m.repos))
	for _, h := range m.repos {
		handles = append(handles, h)
	}
	return handles
}

// RefreshMeta refreshes metadata for a specific repo
func (m *IndexRepoManager) RefreshMeta(id string) error {
	handle, err := m.GetRepo(id)
	if err != nil {
		return err
	}
	return handle.refreshMeta()
}

// RefreshAllMeta refreshes metadata for all repos
func (m *IndexRepoManager) RefreshAllMeta() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, handle := range m.repos {
		if err := handle.refreshMeta(); err != nil {
			m.logger.Warn("Failed to refresh metadata", map[string]interface{}{
				"repo_id": id,
				"error":   err.Error(),
			})
		}
	}
	return nil
}

// Close closes all repo connections
func (m *IndexRepoManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for id, handle := range m.repos {
		if err := handle.Close(); err != nil {
			m.logger.Error("Failed to close repo", map[string]interface{}{
				"repo_id": id,
				"error":   err.Error(),
			})
			lastErr = err
		}
	}
	m.repos = make(map[string]*IndexRepoHandle)
	return lastErr
}

// CursorManager returns the cursor manager for pagination
func (m *IndexRepoManager) CursorManager() *CursorManager {
	return m.cursor
}

// Config returns the server configuration
func (m *IndexRepoManager) Config() *IndexServerConfig {
	return m.config
}

// Close closes the repo's database connection
func (h *IndexRepoHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.db != nil {
		err := h.db.Close()
		h.db = nil
		return err
	}
	return nil
}

// DB returns the underlying database connection
func (h *IndexRepoHandle) DB() *sql.DB {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.db
}

// Meta returns the cached metadata
func (h *IndexRepoHandle) Meta() *IndexRepoMetadata {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.meta
}

// refreshMeta loads metadata from the database
func (h *IndexRepoHandle) refreshMeta() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	meta := &IndexRepoMetadata{
		IndexVersion:  "1.0",
		SchemaVersion: 8, // Current schema version
		IndexedAt:     time.Now(),
		Languages:     []string{},
	}

	// Get commit from index_meta
	var commit sql.NullString
	err := h.db.QueryRow(`SELECT value FROM index_meta WHERE key = 'commit'`).Scan(&commit)
	if err == nil && commit.Valid {
		meta.Commit = commit.String
	}

	// Get sync_seq from index_meta
	var syncSeq sql.NullInt64
	err = h.db.QueryRow(`SELECT value FROM index_meta WHERE key = 'sync_seq'`).Scan(&syncSeq)
	if err == nil && syncSeq.Valid {
		meta.SyncSeq = syncSeq.Int64
	}

	// Get indexed_at timestamp
	var indexedAt sql.NullString
	err = h.db.QueryRow(`SELECT value FROM index_meta WHERE key = 'indexed_at'`).Scan(&indexedAt)
	if err == nil && indexedAt.Valid {
		if t, parseErr := time.Parse(time.RFC3339, indexedAt.String); parseErr == nil {
			meta.IndexedAt = t
		}
	}

	// Get stats
	stats := IndexRepoStats{}

	// Count files
	err = h.db.QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&stats.Files)
	if err != nil {
		// Try alternative table name
		_ = h.db.QueryRow(`SELECT COUNT(DISTINCT file_path) FROM symbol_mappings WHERE state = 'active'`).Scan(&stats.Files)
	}

	// Count symbols
	err = h.db.QueryRow(`SELECT COUNT(*) FROM symbol_mappings WHERE state = 'active'`).Scan(&stats.Symbols)
	if err != nil {
		stats.Symbols = 0
	}

	// Count refs - try occurrences table first
	err = h.db.QueryRow(`SELECT COUNT(*) FROM occurrences`).Scan(&stats.Refs)
	if err != nil {
		stats.Refs = 0
	}

	// Count call edges
	err = h.db.QueryRow(`SELECT COUNT(*) FROM callgraph`).Scan(&stats.CallEdges)
	if err != nil {
		stats.CallEdges = 0
	}

	meta.Stats = stats

	// Get languages from symbol_mappings
	rows, err := h.db.Query(`SELECT DISTINCT language FROM symbol_mappings WHERE state = 'active' AND language IS NOT NULL AND language != ''`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var lang string
			if err := rows.Scan(&lang); err == nil {
				meta.Languages = append(meta.Languages, lang)
			}
		}
	}

	h.meta = meta
	return nil
}

// ToRepoInfo converts handle metadata to API response format
func (h *IndexRepoHandle) ToRepoInfo() IndexRepoInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	name := h.Config.Name
	if name == "" {
		name = h.ID
	}

	info := IndexRepoInfo{
		ID:           h.ID,
		Name:         name,
		Description:  h.Config.Description,
		Languages:    h.meta.Languages,
		Commit:       h.meta.Commit,
		IndexVersion: h.meta.IndexVersion,
		SyncSeq:      h.meta.SyncSeq,
		IndexedAt:    h.meta.IndexedAt.Unix(),
		SymbolCount:  h.meta.Stats.Symbols,
		FileCount:    h.meta.Stats.Files,
	}

	if info.Languages == nil {
		info.Languages = []string{}
	}

	return info
}

// ToMetaResponse converts handle metadata to full API metadata response
func (h *IndexRepoHandle) ToMetaResponse(privacy IndexPrivacyConfig, maxPageSize int) IndexRepoMetaResponse {
	h.mu.RLock()
	defer h.mu.RUnlock()

	name := h.Config.Name
	if name == "" {
		name = h.ID
	}

	return IndexRepoMetaResponse{
		ID:                     h.ID,
		Name:                   name,
		Description:            h.Config.Description,
		Commit:                 h.meta.Commit,
		IndexVersion:           h.meta.IndexVersion,
		SyncSeq:                h.meta.SyncSeq,
		SyncLogRetentionSeqMin: 0, // Not implemented yet
		SchemaVersion:          h.meta.SchemaVersion,
		IndexedAt:              h.meta.IndexedAt.Unix(),
		Languages:              h.meta.Languages,
		Stats:                  h.meta.Stats,
		Capabilities: IndexCapabilities{
			SyncSeq:     true,
			Search:      true,
			BatchGet:    true,
			Compression: []string{}, // Not implemented yet
			Redaction:   true,
			MaxPageSize: maxPageSize,
		},
		Privacy: IndexPrivacyInfo{
			PathsExposed:      privacy.ExposePaths,
			DocsExposed:       privacy.ExposeDocs,
			SignaturesExposed: privacy.ExposeSignatures,
		},
	}
}

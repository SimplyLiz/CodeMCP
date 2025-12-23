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
	repos     map[string]*IndexRepoHandle
	config    *IndexServerConfig
	logger    *logging.Logger
	cursor    *CursorManager
	storage   *IndexStorage  // For uploaded repos (Phase 2)
	processor *SCIPProcessor // For processing uploads (Phase 2)
	mu        sync.RWMutex
}

// NewIndexRepoManager creates a new repo manager with connections to all configured repos
func NewIndexRepoManager(config *IndexServerConfig, logger *logging.Logger) (*IndexRepoManager, error) {
	m := &IndexRepoManager{
		repos:  make(map[string]*IndexRepoHandle),
		config: config,
		logger: logger,
		cursor: NewCursorManager(config.CursorSecret),
	}

	// Initialize storage for uploaded repos (Phase 2)
	if config.DataDir != "" {
		storage, err := NewIndexStorage(config.DataDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize storage: %w", err)
		}
		m.storage = storage
		m.processor = NewSCIPProcessor(storage, logger)

		// Load any existing uploaded repos from storage
		uploadedRepos, err := storage.ListRepos()
		if err != nil {
			logger.Warn("Failed to list uploaded repos", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			for _, repoID := range uploadedRepos {
				meta, err := storage.LoadMeta(repoID)
				if err != nil {
					logger.Warn("Failed to load uploaded repo metadata", map[string]interface{}{
						"repo_id": repoID,
						"error":   err.Error(),
					})
					continue
				}
				repoConfig := IndexRepoConfig{
					ID:          repoID,
					Name:        meta.Name,
					Description: meta.Description,
					Path:        storage.RepoPath(repoID),
					Source:      RepoSourceUploaded,
				}
				handle, err := m.openUploadedRepo(repoConfig)
				if err != nil {
					logger.Warn("Failed to open uploaded repo", map[string]interface{}{
						"repo_id": repoID,
						"error":   err.Error(),
					})
					continue
				}
				m.repos[repoID] = handle
				logger.Info("Loaded uploaded repo", map[string]interface{}{
					"repo_id": repoID,
				})
			}
		}
	}

	// Open connections to all configured repos
	for _, repoConfig := range config.Repos {
		repoConfig.Source = RepoSourceConfig // Mark as config-based
		handle, err := m.openRepo(repoConfig)
		if err != nil {
			// Close any already-opened repos before returning
			_ = m.Close()
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

// GetRepoCommit returns the current indexed commit for a repo
func (m *IndexRepoManager) GetRepoCommit(id string) (string, error) {
	handle, err := m.GetRepo(id)
	if err != nil {
		return "", err
	}
	meta := handle.Meta()
	if meta == nil {
		return "", nil
	}
	return meta.Commit, nil
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
		defer func() { _ = rows.Close() }()
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

// --- Phase 2: Dynamic Repo Management ---

// openUploadedRepo opens a connection to an uploaded repo's database
// Uploaded repos store the database directly in the repo directory (not .ckb/)
func (m *IndexRepoManager) openUploadedRepo(config IndexRepoConfig) (*IndexRepoHandle, error) {
	// Database path for uploaded repos is directly in repo path
	dbPath := filepath.Join(config.Path, "ckb.db")

	// Open in read-only mode for queries (writes happen during upload processing)
	connStr := fmt.Sprintf("file:%s?mode=ro", dbPath)
	conn, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set read-only pragmas
	pragmas := []string{
		"PRAGMA query_only=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-32000",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA mmap_size=134217728",
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

// CreateUploadedRepo creates a new repo in storage for upload
func (m *IndexRepoManager) CreateUploadedRepo(id, name, description string) error {
	if m.storage == nil {
		return fmt.Errorf("storage not initialized (data_dir not configured)")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already exists
	if _, exists := m.repos[id]; exists {
		return fmt.Errorf("repo already exists: %s", id)
	}

	// Create in storage
	if err := m.storage.CreateRepo(id, name, description); err != nil {
		return err
	}

	m.logger.Info("Created uploaded repo", map[string]interface{}{
		"repo_id": id,
	})

	return nil
}

// RemoveRepo removes an uploaded repo
func (m *IndexRepoManager) RemoveRepo(id string) error {
	if m.storage == nil {
		return fmt.Errorf("storage not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close handle if open
	if handle, exists := m.repos[id]; exists {
		if err := handle.Close(); err != nil {
			m.logger.Warn("Failed to close repo before removal", map[string]interface{}{
				"repo_id": id,
				"error":   err.Error(),
			})
		}
		delete(m.repos, id)
	}

	// Delete from storage
	if err := m.storage.DeleteRepo(id); err != nil {
		return err
	}

	m.logger.Info("Removed repo", map[string]interface{}{
		"repo_id": id,
	})

	return nil
}

// ReloadRepo closes and reopens a repo's database connection
// This is needed after an upload to pick up new data
func (m *IndexRepoManager) ReloadRepo(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	handle, exists := m.repos[id]
	if !exists {
		// Repo might be new - try to open it
		if m.storage == nil {
			return fmt.Errorf("repo not found: %s", id)
		}

		meta, err := m.storage.LoadMeta(id)
		if err != nil {
			return fmt.Errorf("repo not found: %s", id)
		}

		config := IndexRepoConfig{
			ID:          id,
			Name:        meta.Name,
			Description: meta.Description,
			Path:        m.storage.RepoPath(id),
			Source:      RepoSourceUploaded,
		}

		newHandle, err := m.openUploadedRepo(config)
		if err != nil {
			return fmt.Errorf("failed to open repo: %w", err)
		}

		m.repos[id] = newHandle
		return nil
	}

	// Close existing connection
	if err := handle.Close(); err != nil {
		m.logger.Warn("Failed to close repo during reload", map[string]interface{}{
			"repo_id": id,
			"error":   err.Error(),
		})
	}

	// Reopen based on source
	var newHandle *IndexRepoHandle
	var err error

	if handle.Config.Source == RepoSourceUploaded {
		newHandle, err = m.openUploadedRepo(*handle.Config)
	} else {
		newHandle, err = m.openRepo(*handle.Config)
	}

	if err != nil {
		delete(m.repos, id)
		return fmt.Errorf("failed to reopen repo: %w", err)
	}

	m.repos[id] = newHandle
	return nil
}

// IsUploadedRepo checks if a repo is uploaded (vs configured)
func (m *IndexRepoManager) IsUploadedRepo(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	handle, exists := m.repos[id]
	if !exists {
		return false
	}
	return handle.Config.Source == RepoSourceUploaded
}

// Storage returns the storage manager for uploaded repos
func (m *IndexRepoManager) Storage() *IndexStorage {
	return m.storage
}

// Processor returns the SCIP processor for uploads
func (m *IndexRepoManager) Processor() *SCIPProcessor {
	return m.processor
}

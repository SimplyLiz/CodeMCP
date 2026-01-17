package mcp

import (
	"fmt"
	"time"

	"ckb/internal/config"
	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/query"
	"ckb/internal/repos"
	"ckb/internal/storage"
)

// toolListRepos lists all registered repositories
func (s *MCPServer) toolListRepos(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing listRepos")

	if !s.IsMultiRepoMode() {
		return nil, &MCPError{
			Code:    InvalidRequest,
			Message: "Multi-repo mode not enabled. Start MCP server with a registry.",
		}
	}

	registry, err := repos.LoadRegistry()
	if err != nil {
		return nil, errors.NewOperationError("load registry", err)
	}

	activeRepo, _ := s.GetActiveRepo()

	type repoInfo struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		State     string `json:"state"`
		IsDefault bool   `json:"is_default"`
		IsActive  bool   `json:"is_active"`
		IsLoaded  bool   `json:"is_loaded"`
	}

	var repoList []repoInfo
	for _, entry := range registry.List() {
		state := registry.ValidateState(entry.Name)

		s.mu.RLock()
		_, isLoaded := s.engines[entry.Path]
		s.mu.RUnlock()

		repoList = append(repoList, repoInfo{
			Name:      entry.Name,
			Path:      entry.Path,
			State:     string(state),
			IsDefault: entry.Name == registry.Default,
			IsActive:  entry.Name == activeRepo,
			IsLoaded:  isLoaded,
		})
	}

	return OperationalResponse(map[string]interface{}{
		"repos":      repoList,
		"activeRepo": activeRepo,
		"default":    registry.Default,
	}), nil
}

// toolSwitchRepo switches to a different repository
func (s *MCPServer) toolSwitchRepo(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing switchRepo",
		"params", params,
	)

	if !s.IsMultiRepoMode() {
		return nil, &MCPError{
			Code:    InvalidRequest,
			Message: "Multi-repo mode not enabled. Start MCP server with a registry.",
		}
	}

	name, ok := params["name"].(string)
	if !ok || name == "" {
		return nil, &MCPError{
			Code:    InvalidParams,
			Message: "name parameter is required",
		}
	}

	registry, err := repos.LoadRegistry()
	if err != nil {
		return nil, errors.NewOperationError("load registry", err)
	}

	entry, state, err := registry.Get(name)
	if err != nil {
		return nil, &MCPError{
			Code:    InvalidParams,
			Message: fmt.Sprintf("Repository not found: %s", name),
		}
	}

	switch state {
	case repos.RepoStateMissing:
		return nil, &MCPError{
			Code:    InvalidParams,
			Message: fmt.Sprintf("Path does not exist: %s", entry.Path),
			Data:    map[string]string{"hint": fmt.Sprintf("Run: ckb repo remove %s", name)},
		}
	case repos.RepoStateUninitialized:
		return nil, &MCPError{
			Code:    InvalidParams,
			Message: fmt.Sprintf("Repository not initialized: %s", entry.Path),
			Data:    map[string]string{"hint": fmt.Sprintf("Run: cd %s && ckb init", entry.Path)},
		}
	}

	// Load or switch engine
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already loaded
	if existingEntry, ok := s.engines[entry.Path]; ok {
		existingEntry.lastUsed = time.Now()
		s.activeRepo = name
		s.activeRepoPath = entry.Path
		s.logger.Info("Switched to existing engine",
			"repo", name,
			"path", entry.Path,
		)
		return OperationalResponse(map[string]interface{}{
			"success":    true,
			"activeRepo": name,
			"path":       entry.Path,
		}), nil
	}

	// Need to create new engine - check if we're at max
	if len(s.engines) >= maxEngines {
		s.evictLRULocked()
	}

	// Create new engine
	engine, err := s.createEngineForRepo(entry.Path)
	if err != nil {
		return nil, errors.NewOperationError("create engine for "+name, err)
	}

	s.engines[entry.Path] = &engineEntry{
		engine:   engine,
		repoPath: entry.Path,
		repoName: name,
		loadedAt: time.Now(),
		lastUsed: time.Now(),
	}
	s.activeRepo = name
	s.activeRepoPath = entry.Path

	// Update last used in registry
	_ = registry.TouchLastUsed(name)

	s.logger.Info("Created new engine and switched",
		"repo", name,
		"path", entry.Path,
		"totalLoaded", len(s.engines),
	)

	return OperationalResponse(map[string]interface{}{
		"success":    true,
		"activeRepo": name,
		"path":       entry.Path,
	}), nil
}

// toolGetActiveRepo returns information about the currently active repository
func (s *MCPServer) toolGetActiveRepo(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing getActiveRepo")

	if !s.IsMultiRepoMode() {
		return nil, &MCPError{
			Code:    InvalidRequest,
			Message: "Multi-repo mode not enabled. Start MCP server with a registry.",
		}
	}

	name, path := s.GetActiveRepo()

	if name == "" {
		return OperationalResponse(map[string]interface{}{
			"name":  nil,
			"state": "none",
			"error": "No active repository. Call switchRepo first or set a default.",
		}), nil
	}

	registry, err := repos.LoadRegistry()
	if err != nil {
		return nil, errors.NewOperationError("load registry", err)
	}

	state := registry.ValidateState(name)

	return OperationalResponse(map[string]interface{}{
		"name":  name,
		"path":  path,
		"state": string(state),
	}), nil
}

// evictLRULocked evicts the least recently used engine (must be called with mu held)
func (s *MCPServer) evictLRULocked() {
	var victim string
	var oldest time.Time

	for path, entry := range s.engines {
		// Never evict active repo
		if path == s.activeRepoPath {
			continue
		}
		if victim == "" || entry.lastUsed.Before(oldest) {
			victim = path
			oldest = entry.lastUsed
		}
	}

	if victim != "" {
		entry := s.engines[victim]
		s.logger.Info("Evicting LRU engine",
			"repo", entry.repoName,
			"path", entry.repoPath,
			"lastUsed", entry.lastUsed,
		)
		// Wait for any in-flight operations
		entry.activeOps.Wait()
		// Close the engine
		if entry.engine != nil {
			_ = entry.engine.Close()
		}
		delete(s.engines, victim)
	}
}

// createEngineForRepo creates a new query engine for a repository
func (s *MCPServer) createEngineForRepo(repoPath string) (*query.Engine, error) {
	// Load config from repo
	cfg, err := config.LoadConfig(repoPath)
	if err != nil {
		// Use default config
		cfg = config.DefaultConfig()
	}

	// Open storage for this repo
	db, err := storage.Open(repoPath, s.logger)
	if err != nil {
		return nil, errors.NewOperationError("open database", err)
	}

	// Create engine
	engine, err := query.NewEngine(repoPath, db, s.logger, cfg)
	if err != nil {
		_ = db.Close()
		return nil, errors.NewOperationError("create engine", err)
	}

	return engine, nil
}

// CloseAllEngines closes all loaded engines (for graceful shutdown)
func (s *MCPServer) CloseAllEngines() {
	s.mu.Lock()
	entries := make([]*engineEntry, 0, len(s.engines))
	for _, entry := range s.engines {
		entries = append(entries, entry)
	}
	s.engines = make(map[string]*engineEntry)
	s.mu.Unlock()

	// Close outside lock
	for _, entry := range entries {
		entry.activeOps.Wait()
		if entry.engine != nil {
			_ = entry.engine.Close()
		}
	}
}

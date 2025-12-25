package mcp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"ckb/internal/logging"
	"ckb/internal/query"
	"ckb/internal/repos"
)

const maxEngines = 5

// engineEntry holds an engine and its metadata
type engineEntry struct {
	engine    *query.Engine
	repoPath  string
	repoName  string
	loadedAt  time.Time
	lastUsed  time.Time
	activeOps sync.WaitGroup
}

// MCPServer represents the MCP server
type MCPServer struct {
	stdin     io.Reader
	stdout    io.Writer
	scanner   *bufio.Scanner
	logger    *logging.Logger
	version   string
	tools     map[string]ToolHandler
	resources map[string]ResourceHandler

	// Legacy single-engine mode
	legacyEngine *query.Engine

	// Multi-repo mode
	engines        map[string]*engineEntry // keyed by normalized path
	activeRepo     string                  // current repo name
	activeRepoPath string                  // current repo path
	registry       *repos.Registry
	mu             sync.RWMutex

	// Preset configuration (for tools/list pagination)
	activePreset string // current preset (core, review, refactor, etc.)
	toolsetHash  string // hash of current tool definitions (for cursor invalidation)
	expanded     bool   // true if expandToolset has been called this session
}

// NewMCPServer creates a new MCP server in legacy single-engine mode
func NewMCPServer(version string, engine *query.Engine, logger *logging.Logger) *MCPServer {
	server := &MCPServer{
		stdin:        os.Stdin,
		stdout:       os.Stdout,
		logger:       logger,
		version:      version,
		legacyEngine: engine,
		tools:        make(map[string]ToolHandler),
		resources:    make(map[string]ResourceHandler),
		activePreset: DefaultPreset,
	}

	// Register all tools
	server.RegisterTools()

	// Compute initial toolset hash
	server.updateToolsetHash()

	// Wire up metrics persistence if database is available
	if engine != nil && engine.DB() != nil {
		SetMetricsDB(engine.DB())
	}

	return server
}

// NewMCPServerWithRegistry creates a new MCP server with multi-repo support
func NewMCPServerWithRegistry(version string, registry *repos.Registry, logger *logging.Logger) *MCPServer {
	server := &MCPServer{
		stdin:        os.Stdin,
		stdout:       os.Stdout,
		logger:       logger,
		version:      version,
		registry:     registry,
		engines:      make(map[string]*engineEntry),
		tools:        make(map[string]ToolHandler),
		resources:    make(map[string]ResourceHandler),
		activePreset: DefaultPreset,
	}

	// Register all tools
	server.RegisterTools()

	// Compute initial toolset hash
	server.updateToolsetHash()

	return server
}

// engine returns the current engine (for backward compatibility with tool handlers)
func (s *MCPServer) engine() *query.Engine {
	// Legacy mode
	if s.legacyEngine != nil {
		return s.legacyEngine
	}

	// Multi-repo mode
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.activeRepoPath == "" {
		return nil
	}

	entry, ok := s.engines[s.activeRepoPath]
	if !ok {
		return nil
	}

	return entry.engine
}

// GetEngine returns the current engine or an error if none is active
func (s *MCPServer) GetEngine() (*query.Engine, error) {
	engine := s.engine()
	if engine == nil {
		return nil, fmt.Errorf("no active repository. Call listRepos to see available repos, then switchRepo")
	}
	return engine, nil
}

// IsMultiRepoMode returns true if the server is in multi-repo mode
func (s *MCPServer) IsMultiRepoMode() bool {
	return s.registry != nil
}

// GetActiveRepo returns the current active repo name and path
func (s *MCPServer) GetActiveRepo() (name string, path string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeRepo, s.activeRepoPath
}

// SetActiveRepo sets the initial active repo (used during startup)
func (s *MCPServer) SetActiveRepo(name, path string, engine *query.Engine) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeRepo = name
	s.activeRepoPath = path

	if engine != nil {
		s.engines[path] = &engineEntry{
			engine:   engine,
			repoPath: path,
			repoName: name,
			loadedAt: time.Now(),
			lastUsed: time.Now(),
		}
		// Wire up metrics persistence for multi-repo mode
		if engine.DB() != nil {
			SetMetricsDB(engine.DB())
		}
	}
}

// Start starts the MCP server and begins processing messages
func (s *MCPServer) Start() error {
	s.logger.Info("MCP server starting", map[string]interface{}{
		"version": s.version,
	})

	// Main message loop
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				s.logger.Info("MCP server shutting down (EOF)", nil)
				return nil
			}
			s.logger.Error("Error reading message", map[string]interface{}{
				"error": err.Error(),
			})

			// Try to send error response if we can extract an ID
			if msg != nil && msg.Id != nil {
				_ = s.writeError(msg.Id, ParseError, fmt.Sprintf("Failed to parse message: %v", err))
			}
			continue
		}

		// Process the message
		response := s.handleMessage(msg)

		// Write response if one was generated (notifications don't generate responses)
		if response != nil {
			if err := s.writeMessage(response); err != nil {
				s.logger.Error("Error writing response", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}
}

// SetStdin sets the input stream (for testing)
func (s *MCPServer) SetStdin(r io.Reader) {
	s.stdin = r
	s.scanner = nil // Reset scanner so it will be recreated with new reader
}

// SetStdout sets the output stream (for testing)
func (s *MCPServer) SetStdout(w io.Writer) {
	s.stdout = w
}

// SetPreset sets the active preset and updates the toolset hash
func (s *MCPServer) SetPreset(preset string) error {
	if !IsValidPreset(preset) {
		return fmt.Errorf("invalid preset: %s (valid: %v)", preset, ValidPresets())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.activePreset = preset
	s.updateToolsetHashLocked()

	s.logger.Info("Preset changed", map[string]interface{}{
		"preset": preset,
	})

	return nil
}

// GetActivePreset returns the current active preset
func (s *MCPServer) GetActivePreset() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.activePreset == "" {
		return DefaultPreset
	}
	return s.activePreset
}

// GetToolsetHash returns the current toolset hash (for cursor validation)
func (s *MCPServer) GetToolsetHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolsetHash
}

// GetFilteredTools returns tools filtered by the active preset, ordered core-first
func (s *MCPServer) GetFilteredTools() []Tool {
	s.mu.RLock()
	preset := s.activePreset
	s.mu.RUnlock()

	if preset == "" {
		preset = DefaultPreset
	}

	allTools := s.GetToolDefinitions()
	return FilterAndOrderTools(allTools, preset)
}

// GetPresetStats returns statistics about the current preset
func (s *MCPServer) GetPresetStats() (preset string, exposedCount int, totalCount int) {
	preset = s.GetActivePreset()
	allTools := s.GetToolDefinitions()
	filteredTools := s.GetFilteredTools()
	return preset, len(filteredTools), len(allTools)
}

// EstimateActiveTokens returns estimated tokens for the active preset's tools/list response
func (s *MCPServer) EstimateActiveTokens() int {
	tools := s.GetFilteredTools()
	return EstimateTokens(MeasureJSONSize(tools))
}

// EstimateFullTokens returns estimated tokens for the full preset (all tools)
func (s *MCPServer) EstimateFullTokens() int {
	allTools := s.GetToolDefinitions()
	return EstimateTokens(MeasureJSONSize(allTools))
}

// updateToolsetHash recomputes the toolset hash (call with lock held or during init)
func (s *MCPServer) updateToolsetHash() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateToolsetHashLocked()
}

// updateToolsetHashLocked recomputes the toolset hash (caller must hold lock)
func (s *MCPServer) updateToolsetHashLocked() {
	allTools := s.GetToolDefinitions()
	filteredTools := FilterAndOrderTools(allTools, s.activePreset)
	s.toolsetHash = ComputeToolsetHash(filteredTools)
}

// IsExpanded returns true if expandToolset has been called this session
func (s *MCPServer) IsExpanded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.expanded
}

// MarkExpanded marks the session as expanded (rate limit: one expansion per session)
func (s *MCPServer) MarkExpanded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expanded = true
}

// SendNotification sends a JSON-RPC notification to the client
func (s *MCPServer) SendNotification(method string, params interface{}) error {
	msg := &MCPMessage{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
	}
	return s.writeMessage(msg)
}

package mcp

import (
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// rootsRequestTimeout is the timeout for roots/list requests to the client
const rootsRequestTimeout = 10 * time.Second

// Root represents a filesystem root provided by the MCP client
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// Path returns the filesystem path for this root (converts file:// URI to path)
func (r *Root) Path() string {
	if !strings.HasPrefix(r.URI, "file://") {
		return r.URI // Return as-is if not a file URI
	}

	// Parse the file URI
	u, err := url.Parse(r.URI)
	if err != nil {
		return strings.TrimPrefix(r.URI, "file://")
	}

	// filepath.FromSlash handles OS-specific path conversion
	return filepath.FromSlash(u.Path)
}

// ClientCapabilities represents capabilities reported by the MCP client
type ClientCapabilities struct {
	Roots *RootsCapability `json:"roots,omitempty"`
}

// RootsCapability indicates the client supports the roots feature
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// rootsManager handles MCP roots from the client
type rootsManager struct {
	mu                 sync.RWMutex
	roots              []Root
	clientSupported    bool
	listChangedEnabled bool
	requestID          atomic.Int64
	pendingRequests    sync.Map // map[int64]chan *MCPMessage
}

// newRootsManager creates a new roots manager
func newRootsManager() *rootsManager {
	return &rootsManager{}
}

// SetClientSupported marks whether the client supports roots
func (rm *rootsManager) SetClientSupported(supported bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.clientSupported = supported
}

// IsClientSupported returns whether the client supports roots
func (rm *rootsManager) IsClientSupported() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.clientSupported
}

// SetListChangedEnabled sets whether the client supports listChanged notifications
func (rm *rootsManager) SetListChangedEnabled(enabled bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.listChangedEnabled = enabled
}

// IsListChangedEnabled returns whether the client supports listChanged notifications
func (rm *rootsManager) IsListChangedEnabled() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.listChangedEnabled
}

// CancelPendingRequest cancels a pending request (for timeout/cleanup)
func (rm *rootsManager) CancelPendingRequest(id int64) bool {
	if ch, ok := rm.pendingRequests.LoadAndDelete(id); ok {
		close(ch.(chan *MCPMessage))
		return true
	}
	return false
}

// CancelAllPending cancels all pending requests (for shutdown cleanup)
func (rm *rootsManager) CancelAllPending() {
	rm.pendingRequests.Range(func(key, value any) bool {
		rm.pendingRequests.Delete(key)
		close(value.(chan *MCPMessage))
		return true
	})
}

// SetRoots updates the stored roots
func (rm *rootsManager) SetRoots(roots []Root) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.roots = roots
}

// GetRoots returns the current roots
func (rm *rootsManager) GetRoots() []Root {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.roots == nil {
		return nil
	}

	// Return a copy to avoid races
	result := make([]Root, len(rm.roots))
	copy(result, rm.roots)
	return result
}

// GetPaths returns the filesystem paths for all roots
func (rm *rootsManager) GetPaths() []string {
	roots := rm.GetRoots()
	if roots == nil {
		return nil
	}

	paths := make([]string, 0, len(roots))
	for _, root := range roots {
		if path := root.Path(); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// NextRequestID generates a unique request ID for server-to-client requests
func (rm *rootsManager) NextRequestID() int64 {
	return rm.requestID.Add(1)
}

// RegisterPendingRequest registers a pending request and returns a channel for the response
func (rm *rootsManager) RegisterPendingRequest(id int64) chan *MCPMessage {
	ch := make(chan *MCPMessage, 1)
	rm.pendingRequests.Store(id, ch)
	return ch
}

// ResolvePendingRequest resolves a pending request with the response
func (rm *rootsManager) ResolvePendingRequest(id int64, msg *MCPMessage) bool {
	if ch, ok := rm.pendingRequests.LoadAndDelete(id); ok {
		ch.(chan *MCPMessage) <- msg
		return true
	}
	return false
}

// parseClientCapabilities extracts client capabilities from initialize params
func parseClientCapabilities(params map[string]interface{}) *ClientCapabilities {
	caps := &ClientCapabilities{}

	capabilitiesRaw, ok := params["capabilities"].(map[string]interface{})
	if !ok {
		return caps
	}

	rootsRaw, ok := capabilitiesRaw["roots"].(map[string]interface{})
	if ok {
		caps.Roots = &RootsCapability{}
		if listChanged, ok := rootsRaw["listChanged"].(bool); ok {
			caps.Roots.ListChanged = listChanged
		}
	}

	return caps
}

// isValidRootURI validates a root URI per MCP spec requirements
// - Must be a file:// URI (file:/// for local paths)
// - Must be an absolute path
// - Must not contain path traversal sequences
// - Must not have a hostname (security: reject file://hostname/path)
func isValidRootURI(uri string) bool {
	if !strings.HasPrefix(uri, "file://") {
		return false
	}

	u, err := url.Parse(uri)
	if err != nil {
		return false
	}

	// Reject file URIs with a hostname (e.g., file://hostname/path)
	// Valid local file URIs use file:/// (empty host)
	if u.Host != "" {
		return false
	}

	// Reject path traversal attempts
	if strings.Contains(u.Path, "..") {
		return false
	}

	// Must be absolute path
	return filepath.IsAbs(u.Path)
}

// parseRootsResponse parses a roots/list response
func parseRootsResponse(result interface{}) []Root {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}

	rootsRaw, ok := resultMap["roots"].([]interface{})
	if !ok {
		return nil
	}

	roots := make([]Root, 0, len(rootsRaw))
	for _, r := range rootsRaw {
		rootMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		root := Root{}
		if uri, ok := rootMap["uri"].(string); ok {
			root.URI = uri
		}
		if name, ok := rootMap["name"].(string); ok {
			root.Name = name
		}

		// Validate URI per MCP spec (must be file:// URI, absolute, no traversal)
		if root.URI != "" && isValidRootURI(root.URI) {
			roots = append(roots, root)
		}
	}

	return roots
}

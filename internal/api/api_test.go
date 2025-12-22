package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/query"
	"ckb/internal/storage"
)

// newTestServer creates a server for testing
func newTestServer(t *testing.T) *Server {
	t.Helper()

	// Create minimal config
	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: false,
			},
			Scip: config.ScipConfig{
				Enabled: false,
			},
			Lsp: config.LspConfig{
				Enabled: false,
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	// Create in-memory database
	db, err := storage.Open(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create query engine with minimal setup
	engine, err := query.NewEngine(".", db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	// Create server with auth disabled for testing
	serverConfig := DefaultServerConfig()
	serverConfig.Auth.Enabled = false
	server, err := NewServer(":0", engine, logger, serverConfig)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	return server
}

func TestHealthEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%v'", response["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// May return 200 or 503 depending on backend state
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have status field
	if _, ok := response["status"]; !ok {
		t.Error("Response should have 'status' field")
	}
}

func TestRootEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["name"] != "CKB HTTP API" {
		t.Errorf("Expected name 'CKB HTTP API', got '%v'", response["name"])
	}

	if _, ok := response["endpoints"]; !ok {
		t.Error("Response should have 'endpoints' field")
	}
}

func TestSearchEndpoint(t *testing.T) {
	server := newTestServer(t)

	// Test without query parameter
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing query, got %d", w.Code)
	}

	// Test with query parameter (will return empty results since no backends)
	req = httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have results array (even if empty)
	if _, ok := response["results"]; !ok {
		t.Error("Response should have 'results' field")
	}
}

func TestSymbolEndpoint(t *testing.T) {
	server := newTestServer(t)

	// Test symbol lookup (will fail since no backends)
	req := httptest.NewRequest(http.MethodGet, "/symbol/test-symbol-id", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Should return 404 or error since symbol doesn't exist
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 404 or 500 for nonexistent symbol, got %d", w.Code)
	}
}

func TestRefsEndpoint(t *testing.T) {
	server := newTestServer(t)

	// Test refs lookup (will fail since no backends)
	req := httptest.NewRequest(http.MethodGet, "/refs/test-symbol-id", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// May return 404 (not found), 200 (empty results), or 500 (no backends)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 404, 200, or 500, got %d", w.Code)
	}
}

func TestArchitectureEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/architecture", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Should return 200 (may have empty modules)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have modules array
	if _, ok := response["modules"]; !ok {
		t.Error("Response should have 'modules' field")
	}
}

func TestStatusEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have backends info
	if _, ok := response["backends"]; !ok {
		t.Error("Response should have 'backends' field")
	}
}

func TestDoctorEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/doctor", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have checks array
	if _, ok := response["checks"]; !ok {
		t.Error("Response should have 'checks' field")
	}
}

func TestOpenAPIEndpoint(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse OpenAPI spec: %v", err)
	}

	// Check required OpenAPI fields
	if response["openapi"] == nil {
		t.Error("OpenAPI spec should have 'openapi' version field")
	}
	if response["info"] == nil {
		t.Error("OpenAPI spec should have 'info' field")
	}
	if response["paths"] == nil {
		t.Error("OpenAPI spec should have 'paths' field")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)

	// Test POST on GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST on /health, got %d", w.Code)
	}
}

func TestNotFound(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for nonexistent path, got %d", w.Code)
	}
}

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCursorEncoding(t *testing.T) {
	cm := NewCursorManager("test-secret")

	tests := []struct {
		name   string
		cursor CursorData
	}{
		{
			name: "symbol cursor",
			cursor: CursorData{
				Entity: "symbol",
				LastPK: "12345",
			},
		},
		{
			name: "ref cursor",
			cursor: CursorData{
				Entity: "ref",
				LastPK: "67890",
			},
		},
		{
			name: "search cursor with offset",
			cursor: CursorData{
				Entity: "symbol",
				Offset: 100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded, err := cm.Encode(tt.cursor)
			if err != nil {
				t.Fatalf("Failed to encode cursor: %v", err)
			}

			// Should have two parts separated by dot
			if !strings.Contains(encoded, ".") {
				t.Errorf("Encoded cursor should contain '.': %s", encoded)
			}

			// Decode
			decoded, err := cm.Decode(encoded)
			if err != nil {
				t.Fatalf("Failed to decode cursor: %v", err)
			}

			// Verify values
			if decoded.Entity != tt.cursor.Entity {
				t.Errorf("Entity mismatch: got %s, want %s", decoded.Entity, tt.cursor.Entity)
			}
			if decoded.LastPK != tt.cursor.LastPK {
				t.Errorf("LastPK mismatch: got %s, want %s", decoded.LastPK, tt.cursor.LastPK)
			}
			if decoded.Offset != tt.cursor.Offset {
				t.Errorf("Offset mismatch: got %d, want %d", decoded.Offset, tt.cursor.Offset)
			}
		})
	}
}

func TestCursorTampering(t *testing.T) {
	cm := NewCursorManager("test-secret")

	cursor := CursorData{
		Entity: "symbol",
		LastPK: "12345",
	}

	encoded, _ := cm.Encode(cursor)

	// Tamper with the payload (change a character)
	tampered := "x" + encoded[1:]

	_, err := cm.Decode(tampered)
	if err == nil {
		t.Error("Expected error for tampered cursor")
	}
}

func TestCursorEmptyString(t *testing.T) {
	cm := NewCursorManager("test-secret")

	decoded, err := cm.Decode("")
	if err != nil {
		t.Errorf("Empty cursor should not error: %v", err)
	}
	if decoded != nil {
		t.Error("Empty cursor should return nil")
	}
}

func TestCursorEntityValidation(t *testing.T) {
	cursor := &CursorData{
		Entity: "symbol",
		LastPK: "123",
	}

	// Valid entity
	if err := cursor.ValidateEntity("symbol"); err != nil {
		t.Errorf("Valid entity should not error: %v", err)
	}

	// Invalid entity
	if err := cursor.ValidateEntity("ref"); err == nil {
		t.Error("Expected error for mismatched entity")
	}

	// Nil cursor is always valid
	var nilCursor *CursorData
	if err := nilCursor.ValidateEntity("symbol"); err != nil {
		t.Errorf("Nil cursor should be valid: %v", err)
	}
}

func TestRedaction(t *testing.T) {
	tests := []struct {
		name     string
		config   IndexPrivacyConfig
		symbol   IndexSymbol
		wantPath string
		wantDocs string
		wantSig  string
	}{
		{
			name: "no redaction",
			config: IndexPrivacyConfig{
				ExposePaths:      true,
				ExposeDocs:       true,
				ExposeSignatures: true,
			},
			symbol: IndexSymbol{
				FilePath:      "/src/main.go",
				Documentation: "Some docs",
				Signature:     "func main()",
			},
			wantPath: "/src/main.go",
			wantDocs: "Some docs",
			wantSig:  "func main()",
		},
		{
			name: "redact paths",
			config: IndexPrivacyConfig{
				ExposePaths:      false,
				ExposeDocs:       true,
				ExposeSignatures: true,
			},
			symbol: IndexSymbol{
				FilePath:      "/src/main.go",
				Documentation: "Some docs",
				Signature:     "func main()",
			},
			wantPath: "",
			wantDocs: "Some docs",
			wantSig:  "func main()",
		},
		{
			name: "redact all",
			config: IndexPrivacyConfig{
				ExposePaths:      false,
				ExposeDocs:       false,
				ExposeSignatures: false,
			},
			symbol: IndexSymbol{
				FilePath:      "/src/main.go",
				Documentation: "Some docs",
				Signature:     "func main()",
			},
			wantPath: "",
			wantDocs: "",
			wantSig:  "",
		},
		{
			name: "strip prefix",
			config: IndexPrivacyConfig{
				ExposePaths:      true,
				ExposeDocs:       true,
				ExposeSignatures: true,
				PathPrefixStrip:  "/home/build",
			},
			symbol: IndexSymbol{
				FilePath:      "/home/build/src/main.go",
				Documentation: "Some docs",
				Signature:     "func main()",
			},
			wantPath: "src/main.go",
			wantDocs: "Some docs",
			wantSig:  "func main()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redactor := NewRedactor(&tt.config)
			result := redactor.RedactSymbol(tt.symbol)

			if result.FilePath != tt.wantPath {
				t.Errorf("FilePath: got %q, want %q", result.FilePath, tt.wantPath)
			}
			if result.Documentation != tt.wantDocs {
				t.Errorf("Documentation: got %q, want %q", result.Documentation, tt.wantDocs)
			}
			if result.Signature != tt.wantSig {
				t.Errorf("Signature: got %q, want %q", result.Signature, tt.wantSig)
			}
		})
	}
}

func TestIndexServerDisabled(t *testing.T) {
	// Create a minimal test server without index manager
	server := &Server{
		indexManager: nil,
	}

	tests := []struct {
		name    string
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"ListRepos", "/index/repos", server.HandleIndexListRepos},
		{"GetMeta", "/index/repos/test/meta", server.HandleIndexGetMeta},
		{"ListSymbols", "/index/repos/test/symbols", server.HandleIndexListSymbols},
		{"ListRefs", "/index/repos/test/refs", server.HandleIndexListRefs},
		{"ListCallgraph", "/index/repos/test/callgraph", server.HandleIndexListCallgraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("Expected status 503, got %d", w.Code)
			}

			var resp IndexResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if resp.Error == nil {
				t.Error("Expected error in response")
			} else if resp.Error.Code != "index_server_disabled" {
				t.Errorf("Expected error code 'index_server_disabled', got %s", resp.Error.Code)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *IndexServerConfig
		wantErr bool
	}{
		{
			name: "disabled config - no validation",
			config: &IndexServerConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "enabled but no repos",
			config: &IndexServerConfig{
				Enabled:     true,
				MaxPageSize: 1000,
			},
			wantErr: true,
		},
		{
			name: "invalid max page size",
			config: &IndexServerConfig{
				Enabled:     false,
				MaxPageSize: -1,
			},
			wantErr: false, // Not validated when disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractRepoID(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		suffix string
		want   string
	}{
		{"/index/repos/myorg/myrepo/meta", "/index/repos/", "/meta", "myorg/myrepo"},
		{"/index/repos/simple/symbols", "/index/repos/", "/symbols", "simple"},
		{"/index/repos/", "/index/repos/", "/meta", ""},
		{"/other/path", "/index/repos/", "/meta", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractRepoID(tt.path, tt.prefix, tt.suffix)
			if got != tt.want {
				t.Errorf("extractRepoID(%q, %q, %q) = %q, want %q",
					tt.path, tt.prefix, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestNewSymbolCursor(t *testing.T) {
	cursor := NewSymbolCursor("pk123")
	if cursor.Entity != "symbol" {
		t.Errorf("Expected entity 'symbol', got %s", cursor.Entity)
	}
	if cursor.LastPK != "pk123" {
		t.Errorf("Expected LastPK 'pk123', got %s", cursor.LastPK)
	}
}

func TestNewRefCursor(t *testing.T) {
	cursor := NewRefCursor("pk456")
	if cursor.Entity != "ref" {
		t.Errorf("Expected entity 'ref', got %s", cursor.Entity)
	}
	if cursor.LastPK != "pk456" {
		t.Errorf("Expected LastPK 'pk456', got %s", cursor.LastPK)
	}
}

func TestNewCallgraphCursor(t *testing.T) {
	cursor := NewCallgraphCursor("pk789")
	if cursor.Entity != "callgraph" {
		t.Errorf("Expected entity 'callgraph', got %s", cursor.Entity)
	}
	if cursor.LastPK != "pk789" {
		t.Errorf("Expected LastPK 'pk789', got %s", cursor.LastPK)
	}
}

func TestNewSearchCursor(t *testing.T) {
	cursor := NewSearchCursor("symbol", 100)
	if cursor.Entity != "symbol" {
		t.Errorf("Expected entity 'symbol', got %s", cursor.Entity)
	}
	if cursor.Offset != 100 {
		t.Errorf("Expected Offset 100, got %d", cursor.Offset)
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"component.tsx", "typescriptreact"},
		{"script.js", "javascript"},
		{"component.jsx", "javascriptreact"},
		{"app.py", "python"},
		{"lib.rs", "rust"},
		{"App.java", "java"},
		{"Main.kt", "kotlin"},
		{"main.c", "c"},
		{"main.cpp", "cpp"},
		{"Program.cs", "csharp"},
		{"app.rb", "ruby"},
		{"index.php", "php"},
		{"lib.dart", "dart"},
		{"unknown.xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIndexResponseCreation(t *testing.T) {
	// Test basic response
	data := map[string]string{"key": "value"}
	resp := NewIndexResponse(data)

	if resp.Data == nil {
		t.Error("Expected data in response")
	}
	if resp.Meta == nil {
		t.Error("Expected meta in response")
	}
	if resp.Meta.Timestamp == 0 {
		t.Error("Expected timestamp to be set")
	}
	if resp.Error != nil {
		t.Error("Expected no error in response")
	}
}

func TestIndexErrorResponse(t *testing.T) {
	resp := NewIndexErrorResponse("test_code", "Test message")

	if resp.Data != nil {
		t.Error("Expected no data in error response")
	}
	if resp.Error == nil {
		t.Error("Expected error in response")
	}
	if resp.Error.Code != "test_code" {
		t.Errorf("Expected code 'test_code', got %s", resp.Error.Code)
	}
	if resp.Error.Message != "Test message" {
		t.Errorf("Expected message 'Test message', got %s", resp.Error.Message)
	}
}

// =============================================================================
// Integration tests with real database
// =============================================================================

// testIndexDB creates an in-memory SQLite database with test data
func setupTestIndexDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create schema matching what index_queries.go expects
	schema := `
		CREATE TABLE symbol_mappings (
			stable_id TEXT PRIMARY KEY,
			state TEXT NOT NULL CHECK(state IN ('active', 'deleted', 'unknown')),
			backend_stable_id TEXT,
			fingerprint_json TEXT NOT NULL,
			location_json TEXT NOT NULL,
			definition_version_id TEXT,
			definition_version_semantics TEXT,
			last_verified_at TEXT NOT NULL,
			last_verified_state_id TEXT NOT NULL,
			deleted_at TEXT,
			deleted_in_state_id TEXT
		);

		CREATE TABLE callgraph (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			caller_id TEXT,
			callee_id TEXT NOT NULL,
			caller_file TEXT NOT NULL,
			call_line INTEGER NOT NULL,
			call_col INTEGER NOT NULL,
			call_end_col INTEGER,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE indexed_files (
			path TEXT PRIMARY KEY,
			hash TEXT NOT NULL,
			symbol_count INTEGER DEFAULT 0,
			indexed_at TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'current'
		);

		CREATE INDEX idx_callgraph_caller ON callgraph(caller_id);
		CREATE INDEX idx_callgraph_callee ON callgraph(callee_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Insert test data
	testData := `
		INSERT INTO symbol_mappings (stable_id, state, fingerprint_json, location_json, last_verified_at, last_verified_state_id)
		VALUES
			('sym:func:main', 'active', '{"name":"main","kind":"function","qualifiedContainer":""}', '{"path":"cmd/app/main.go","line":10,"column":1}', datetime('now'), 'state1'),
			('sym:func:helper', 'active', '{"name":"Helper","kind":"function","qualifiedContainer":"pkg"}', '{"path":"internal/pkg/helper.go","line":5,"column":1}', datetime('now'), 'state1'),
			('sym:type:Config', 'active', '{"name":"Config","kind":"struct","qualifiedContainer":"config"}', '{"path":"internal/config/config.go","line":15,"column":1}', datetime('now'), 'state1'),
			('sym:method:Start', 'active', '{"name":"Start","kind":"method","qualifiedContainer":"Server"}', '{"path":"internal/server/server.go","line":25,"column":1}', datetime('now'), 'state1'),
			('sym:deleted:old', 'deleted', '{"name":"OldFunc","kind":"function","qualifiedContainer":""}', '{"path":"old.go","line":1,"column":1}', datetime('now'), 'state1');

		INSERT INTO callgraph (caller_id, callee_id, caller_file, call_line, call_col, call_end_col)
		VALUES
			('sym:func:main', 'sym:func:helper', 'cmd/app/main.go', 15, 5, 20),
			('sym:func:main', 'sym:method:Start', 'cmd/app/main.go', 20, 5, 25),
			('sym:method:Start', 'sym:func:helper', 'internal/server/server.go', 30, 10, 15);

		INSERT INTO indexed_files (path, hash, symbol_count, indexed_at)
		VALUES
			('cmd/app/main.go', 'hash1', 5, datetime('now')),
			('internal/pkg/helper.go', 'hash2', 3, datetime('now')),
			('internal/config/config.go', 'hash3', 10, datetime('now')),
			('internal/server/server.go', 'hash4', 8, datetime('now'));
	`
	if _, err := db.Exec(testData); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	return db
}

// createTestServer creates a server with a test index manager
func createTestServer(t *testing.T, db *sql.DB) *Server {
	t.Helper()

	config := &IndexServerConfig{
		Enabled:      true,
		MaxPageSize:  100,
		CursorSecret: "test-secret",
		Repos: []IndexRepoConfig{
			{
				ID:   "test-repo",
				Name: "Test Repository",
				Path: "/test/path",
			},
		},
		DefaultPrivacy: IndexPrivacyConfig{
			ExposePaths:      true,
			ExposeDocs:       true,
			ExposeSignatures: true,
		},
	}

	// Create a mock handle with the test database
	handle := &IndexRepoHandle{
		ID: "test-repo",
		Config: &IndexRepoConfig{
			ID:   "test-repo",
			Name: "Test Repository",
			Path: "/test/path",
		},
		db: db,
		meta: &IndexRepoMetadata{
			Stats: IndexRepoStats{
				Symbols: 4,
				Files:   4,
			},
		},
	}

	manager := &IndexRepoManager{
		repos:  map[string]*IndexRepoHandle{"test-repo": handle},
		config: config,
		cursor: NewCursorManager("test-secret"),
	}

	return &Server{
		indexManager: manager,
	}
}

func TestHandleIndexListReposIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListRepos(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	// Check that we got the repos
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	repos, ok := data["repos"].([]interface{})
	if !ok {
		t.Fatalf("Expected repos to be array, got %T", data["repos"])
	}

	if len(repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(repos))
	}
}

func TestHandleIndexGetMetaIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/meta", nil)
	w := httptest.NewRecorder()

	server.HandleIndexGetMeta(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}
}

func TestHandleIndexGetMetaNotFound(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/nonexistent/meta", nil)
	w := httptest.NewRecorder()

	server.HandleIndexGetMeta(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHandleIndexListFilesIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/files", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	files, ok := data["files"].([]interface{})
	if !ok {
		t.Fatalf("Expected files to be array, got %T", data["files"])
	}

	if len(files) != 4 {
		t.Errorf("Expected 4 files, got %d", len(files))
	}
}

func TestHandleIndexListSymbolsIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/symbols?limit=10", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	symbols, ok := data["symbols"].([]interface{})
	if !ok {
		t.Fatalf("Expected symbols to be array, got %T", data["symbols"])
	}

	// Should have 4 active symbols (excluding deleted)
	if len(symbols) != 4 {
		t.Errorf("Expected 4 symbols, got %d", len(symbols))
	}
}

func TestHandleIndexListSymbolsWithFilter(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	// Filter by kind
	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/symbols?kind=function", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	symbols, ok := data["symbols"].([]interface{})
	if !ok {
		t.Fatalf("Expected symbols to be array, got %T", data["symbols"])
	}

	// Should have 2 functions (main, Helper)
	if len(symbols) != 2 {
		t.Errorf("Expected 2 function symbols, got %d", len(symbols))
	}
}

func TestHandleIndexGetSymbolIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/symbols/sym:func:main", nil)
	w := httptest.NewRecorder()

	server.HandleIndexGetSymbol(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	symbol, ok := data["symbol"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected symbol to be map, got %T", data["symbol"])
	}

	if symbol["id"] != "sym:func:main" {
		t.Errorf("Expected symbol id 'sym:func:main', got %v", symbol["id"])
	}
}

func TestHandleIndexGetSymbolNotFound(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/symbols/sym:nonexistent", nil)
	w := httptest.NewRecorder()

	server.HandleIndexGetSymbol(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHandleIndexBatchGetSymbolsIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	body := `{"ids": ["sym:func:main", "sym:func:helper", "sym:nonexistent"]}`
	req := httptest.NewRequest(http.MethodPost, "/index/repos/test-repo/symbols:batchGet", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.HandleIndexBatchGetSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	symbols, ok := data["symbols"].([]interface{})
	if !ok {
		t.Fatalf("Expected symbols to be array, got %T", data["symbols"])
	}

	// Should have 2 found symbols (nonexistent is skipped)
	if len(symbols) != 2 {
		t.Errorf("Expected 2 symbols, got %d", len(symbols))
	}
}

func TestHandleIndexListCallgraphIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/callgraph", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListCallgraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	edges, ok := data["edges"].([]interface{})
	if !ok {
		t.Fatalf("Expected edges to be array, got %T", data["edges"])
	}

	if len(edges) != 3 {
		t.Errorf("Expected 3 call edges, got %d", len(edges))
	}
}

func TestHandleIndexListCallgraphWithCallerFilter(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/callgraph?caller_id=sym:func:main", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListCallgraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	edges, ok := data["edges"].([]interface{})
	if !ok {
		t.Fatalf("Expected edges to be array, got %T", data["edges"])
	}

	// main calls 2 functions
	if len(edges) != 2 {
		t.Errorf("Expected 2 edges from main, got %d", len(edges))
	}
}

func TestHandleIndexSearchSymbolsIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/search/symbols?q=Helper", nil)
	w := httptest.NewRecorder()

	server.HandleIndexSearchSymbols(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	symbols, ok := data["symbols"].([]interface{})
	if !ok {
		t.Fatalf("Expected symbols to be array, got %T", data["symbols"])
	}

	if len(symbols) != 1 {
		t.Errorf("Expected 1 symbol matching 'Helper', got %d", len(symbols))
	}
}

func TestHandleIndexSearchSymbolsMissingQuery(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/search/symbols", nil)
	w := httptest.NewRecorder()

	server.HandleIndexSearchSymbols(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleIndexSearchFilesIntegration(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/search/files?q=server", nil)
	w := httptest.NewRecorder()

	server.HandleIndexSearchFiles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Unexpected error: %v", resp.Error)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	files, ok := data["files"].([]interface{})
	if !ok {
		t.Fatalf("Expected files to be array, got %T", data["files"])
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file matching 'server', got %d", len(files))
	}
}

func TestHandleIndexListRefsPagination(t *testing.T) {
	db := setupTestIndexDB(t)
	defer func() { _ = db.Close() }()

	server := createTestServer(t, db)

	// First request with limit=2
	req := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/callgraph?limit=2", nil)
	w := httptest.NewRecorder()

	server.HandleIndexListCallgraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, _ := resp.Data.(map[string]interface{})
	edges, _ := data["edges"].([]interface{})

	if len(edges) != 2 {
		t.Errorf("Expected 2 edges, got %d", len(edges))
	}

	// Check that cursor is returned for more results (cursor is in Meta, not Data)
	if resp.Meta == nil || resp.Meta.Cursor == "" {
		t.Error("Expected cursor in Meta for pagination")
	}
	cursor := resp.Meta.Cursor

	// Second request with cursor
	req2 := httptest.NewRequest(http.MethodGet, "/index/repos/test-repo/callgraph?limit=2&cursor="+cursor, nil)
	w2 := httptest.NewRecorder()

	server.HandleIndexListCallgraph(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp2 IndexResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data2, _ := resp2.Data.(map[string]interface{})
	edges2, _ := data2["edges"].([]interface{})

	if len(edges2) != 1 {
		t.Errorf("Expected 1 remaining edge, got %d", len(edges2))
	}
}

// === Upload Handler Integration Tests ===

// createTestServerWithStorage creates a server with actual storage for upload testing
func createTestServerWithStorage(t *testing.T, tmpDir string) *Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create actual storage
	storage, err := NewIndexStorage(tmpDir, logger)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	config := &IndexServerConfig{
		Enabled:         true,
		MaxPageSize:     100,
		CursorSecret:    "test-secret",
		AllowCreateRepo: true,
		DataDir:         tmpDir,
		DefaultPrivacy: IndexPrivacyConfig{
			ExposePaths:      true,
			ExposeDocs:       true,
			ExposeSignatures: true,
		},
	}

	manager := &IndexRepoManager{
		repos:   make(map[string]*IndexRepoHandle),
		config:  config,
		logger:  logger,
		storage: storage,
		cursor:  NewCursorManager(config.CursorSecret),
	}

	serverConfig := ServerConfig{
		IndexServer: config,
	}

	return &Server{
		indexManager: manager,
		config:       serverConfig,
		logger:       logger,
	}
}

func TestHandleIndexCreateRepoIntegration(t *testing.T) {
	// Create temp directory for storage
	tmpDir, err := os.MkdirTemp("", "ckb-create-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	reqBody := `{"id": "test/new-repo", "name": "New Test Repo", "description": "A test repository"}`
	req := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.HandleIndexCreateRepo(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	if id, _ := data["id"].(string); id != "test/new-repo" {
		t.Errorf("Expected id 'test/new-repo', got %q", id)
	}
	if status, _ := data["status"].(string); status != "created" {
		t.Errorf("Expected status 'created', got %q", status)
	}

	// Verify repo was actually created in storage
	if !server.indexManager.Storage().RepoExists("test/new-repo") {
		t.Error("Repo was not created in storage")
	}
}

func TestHandleIndexCreateRepoConflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-conflict-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	// Create repo first time
	reqBody := `{"id": "test/repo", "name": "Test Repo"}`
	req1 := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.HandleIndexCreateRepo(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("First create failed: %s", w1.Body.String())
	}

	// Reload the repo so GetRepo succeeds
	if err := server.indexManager.ReloadRepo("test/repo"); err != nil {
		// Ignore error if no database yet - just mark it in the map
		server.indexManager.repos["test/repo"] = &IndexRepoHandle{ID: "test/repo"}
	}

	// Try to create again - should conflict
	req2 := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.HandleIndexCreateRepo(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("Expected status 409 Conflict, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleIndexCreateRepoMissingID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-missing-id-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	reqBody := `{"name": "No ID Repo"}`
	req := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.HandleIndexCreateRepo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexCreateRepoInvalidID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-invalid-id-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	// Try invalid IDs
	invalidIDs := []string{
		"/invalid",
		"invalid/",
		"has spaces",
		"has@special",
	}

	for _, invalidID := range invalidIDs {
		reqBody := fmt.Sprintf(`{"id": %q}`, invalidID)
		req := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.HandleIndexCreateRepo(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for ID %q, got %d: %s", invalidID, w.Code, w.Body.String())
		}
	}
}

func TestHandleIndexCreateRepoDisabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-disabled-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.AllowCreateRepo = false

	reqBody := `{"id": "test/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/index/repos", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.HandleIndexCreateRepo(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexDeleteRepoIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delete-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	// First create a repo
	if err := server.indexManager.CreateUploadedRepo("to-delete/repo", "Delete Me", ""); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	// Mark as uploaded in memory (with Config.Source = RepoSourceUploaded)
	server.indexManager.repos["to-delete/repo"] = &IndexRepoHandle{
		ID: "to-delete/repo",
		Config: &IndexRepoConfig{
			ID:     "to-delete/repo",
			Source: RepoSourceUploaded,
		},
	}

	// Now delete it
	req := httptest.NewRequest(http.MethodDelete, "/index/repos/to-delete/repo", nil)
	w := httptest.NewRecorder()

	server.HandleIndexDeleteRepo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map, got %T", resp.Data)
	}

	if status, _ := data["status"].(string); status != "deleted" {
		t.Errorf("Expected status 'deleted', got %q", status)
	}

	// Verify repo was removed from storage
	if server.indexManager.Storage().RepoExists("to-delete/repo") {
		t.Error("Repo still exists in storage after deletion")
	}
}

func TestHandleIndexDeleteRepoNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delete-notfound-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	req := httptest.NewRequest(http.MethodDelete, "/index/repos/nonexistent/repo", nil)
	w := httptest.NewRecorder()

	server.HandleIndexDeleteRepo(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexDeleteConfigRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delete-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)

	// Add a config-based repo (not uploaded - Source = RepoSourceConfig)
	server.indexManager.repos["config/repo"] = &IndexRepoHandle{
		ID: "config/repo",
		Config: &IndexRepoConfig{
			ID:     "config/repo",
			Source: RepoSourceConfig, // This is a config-based repo
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/index/repos/config/repo", nil)
	w := httptest.NewRecorder()

	server.HandleIndexDeleteRepo(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 for config-based repo, got %d: %s", w.Code, w.Body.String())
	}
}

// === Delta Upload Handler Integration Tests ===

func TestHandleIndexDeltaUploadDisabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delta-disabled-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.EnableDeltaUpload = false

	req := httptest.NewRequest(http.MethodPost, "/index/repos/test/repo/upload/delta", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-CKB-Base-Commit", "abc123")
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 when delta disabled, got %d: %s", w.Code, w.Body.String())
	}

	var resp IndexResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil && resp.Error != nil {
		if resp.Error.Code != "delta_disabled" {
			t.Errorf("Expected error code 'delta_disabled', got %q", resp.Error.Code)
		}
	}
}

func TestHandleIndexDeltaUploadRepoNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delta-notfound-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.EnableDeltaUpload = true

	req := httptest.NewRequest(http.MethodPost, "/index/repos/nonexistent/repo/upload/delta", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-CKB-Base-Commit", "abc123")
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for nonexistent repo, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexDeltaUploadMissingRepoID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delta-noid-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.EnableDeltaUpload = true

	// Path without repo ID
	req := httptest.NewRequest(http.MethodPost, "/index/repos//upload/delta", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing repo ID, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexDeltaUploadMissingBaseCommit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delta-nocommit-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.EnableDeltaUpload = true

	// Create a repo first
	if err := server.indexManager.CreateUploadedRepo("test/repo", "Test Repo", ""); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	server.indexManager.repos["test/repo"] = &IndexRepoHandle{
		ID: "test/repo",
		Config: &IndexRepoConfig{
			ID:     "test/repo",
			Source: RepoSourceUploaded,
		},
	}

	// Request without X-CKB-Base-Commit header
	req := httptest.NewRequest(http.MethodPost, "/index/repos/test/repo/upload/delta", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing base commit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleIndexDeltaUploadCommitMismatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-delta-mismatch-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	server := createTestServerWithStorage(t, tmpDir)
	server.config.IndexServer.EnableDeltaUpload = true

	// Create a repo with a known commit
	if err := server.indexManager.CreateUploadedRepo("test/repo", "Test Repo", ""); err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}

	// Create a mock handle with a commit set via meta (not db)
	server.indexManager.repos["test/repo"] = &IndexRepoHandle{
		ID: "test/repo",
		Config: &IndexRepoConfig{
			ID:     "test/repo",
			Source: RepoSourceUploaded,
		},
		meta: &IndexRepoMetadata{
			Commit: "current-commit-123", // This is what GetRepoCommit returns
		},
	}

	// Request with mismatched base commit
	req := httptest.NewRequest(http.MethodPost, "/index/repos/test/repo/upload/delta", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-CKB-Base-Commit", "wrong-commit-456")
	req.Header.Set("X-CKB-Target-Commit", "new-commit-789")
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409 for commit mismatch, got %d: %s", w.Code, w.Body.String())
	}

	// Check the response contains the current commit
	var deltaResp DeltaErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &deltaResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if deltaResp.Code != "base_commit_mismatch" {
		t.Errorf("Expected code 'base_commit_mismatch', got %q", deltaResp.Code)
	}
	if deltaResp.CurrentCommit != "current-commit-123" {
		t.Errorf("Expected current commit 'current-commit-123', got %q", deltaResp.CurrentCommit)
	}
}

func TestHandleIndexDeltaUploadNoIndexManager(t *testing.T) {
	server := &Server{
		indexManager: nil, // No index manager
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodPost, "/index/repos/test/repo/upload/delta", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	server.HandleIndexDeltaUpload(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when index server disabled, got %d: %s", w.Code, w.Body.String())
	}
}

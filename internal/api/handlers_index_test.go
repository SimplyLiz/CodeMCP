package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

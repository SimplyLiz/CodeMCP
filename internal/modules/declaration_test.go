package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModulesFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-modules-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a MODULES.toml file
	modulesContent := `
version = 1

[[module]]
name = "api"
path = "internal/api"
responsibility = "HTTP API handlers"
owner = "@api-team"
tags = ["core", "api"]

[[module]]
name = "query"
path = "internal/query"
responsibility = "Query engine"
owner = "@platform-team"

[module.boundaries]
exports = ["Engine", "Query"]
internal = ["internal/query/private"]
allowed_dependencies = ["internal/storage"]
`

	modulesPath := filepath.Join(tempDir, "MODULES.toml")
	if err := os.WriteFile(modulesPath, []byte(modulesContent), 0644); err != nil {
		t.Fatalf("Failed to write MODULES.toml: %v", err)
	}

	// Parse the file
	modulesFile, err := ParseModulesFile(modulesPath)
	if err != nil {
		t.Fatalf("Failed to parse MODULES.toml: %v", err)
	}

	// Verify version
	if modulesFile.Version != 1 {
		t.Errorf("Expected version 1, got %d", modulesFile.Version)
	}

	// Verify module count
	if len(modulesFile.Modules) != 2 {
		t.Errorf("Expected 2 modules, got %d", len(modulesFile.Modules))
	}

	// Verify first module
	api := modulesFile.Modules[0]
	if api.Name != "api" {
		t.Errorf("Expected name 'api', got '%s'", api.Name)
	}
	if api.Path != "internal/api" {
		t.Errorf("Expected path 'internal/api', got '%s'", api.Path)
	}
	if api.Responsibility != "HTTP API handlers" {
		t.Errorf("Expected responsibility 'HTTP API handlers', got '%s'", api.Responsibility)
	}
	if api.Owner != "@api-team" {
		t.Errorf("Expected owner '@api-team', got '%s'", api.Owner)
	}
	if len(api.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(api.Tags))
	}

	// Verify second module with boundaries
	query := modulesFile.Modules[1]
	if query.Name != "query" {
		t.Errorf("Expected name 'query', got '%s'", query.Name)
	}
	if query.Boundaries == nil {
		t.Error("Expected boundaries to be set")
	} else {
		if len(query.Boundaries.Exports) != 2 {
			t.Errorf("Expected 2 exports, got %d", len(query.Boundaries.Exports))
		}
		if len(query.Boundaries.Internal) != 1 {
			t.Errorf("Expected 1 internal path, got %d", len(query.Boundaries.Internal))
		}
		if len(query.Boundaries.AllowedDependencies) != 1 {
			t.Errorf("Expected 1 allowed dependency, got %d", len(query.Boundaries.AllowedDependencies))
		}
	}
}

func TestLoadDeclaredModules(t *testing.T) {
	// Create a temp directory with module structure
	tempDir, err := os.MkdirTemp("", "ckb-modules-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create module directories
	apiDir := filepath.Join(tempDir, "internal", "api")
	queryDir := filepath.Join(tempDir, "internal", "query")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatalf("Failed to create api dir: %v", err)
	}
	if err := os.MkdirAll(queryDir, 0755); err != nil {
		t.Fatalf("Failed to create query dir: %v", err)
	}

	// Create a Go file in api dir to help with language detection
	if err := os.WriteFile(filepath.Join(apiDir, "handler.go"), []byte("package api"), 0644); err != nil {
		t.Fatalf("Failed to write handler.go: %v", err)
	}

	// Create a MODULES.toml file
	modulesContent := `
version = 1

[[module]]
name = "api"
path = "internal/api"
responsibility = "HTTP API handlers"

[[module]]
path = "internal/query"
`

	modulesPath := filepath.Join(tempDir, "MODULES.toml")
	if err := os.WriteFile(modulesPath, []byte(modulesContent), 0644); err != nil {
		t.Fatalf("Failed to write MODULES.toml: %v", err)
	}

	// Load declared modules
	modules, err := LoadDeclaredModules(tempDir, "", "test-state")
	if err != nil {
		t.Fatalf("Failed to load declared modules: %v", err)
	}

	// Verify module count
	if len(modules) != 2 {
		t.Errorf("Expected 2 modules, got %d", len(modules))
	}

	// Verify first module has a stable ID
	if modules[0].ID == "" {
		t.Error("Expected module ID to be generated")
	}
	if modules[0].Name != "api" {
		t.Errorf("Expected name 'api', got '%s'", modules[0].Name)
	}

	// Verify second module uses path as name
	if modules[1].Name != "query" {
		t.Errorf("Expected name 'query' (from path), got '%s'", modules[1].Name)
	}

	// Verify state ID is set
	if modules[0].StateId != "test-state" {
		t.Errorf("Expected state ID 'test-state', got '%s'", modules[0].StateId)
	}
}

func TestLoadDeclaredModulesNoFile(t *testing.T) {
	// Create a temp directory without MODULES.toml
	tempDir, err := os.MkdirTemp("", "ckb-modules-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Load declared modules
	modules, err := LoadDeclaredModules(tempDir, "", "test-state")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return nil when no file exists
	if modules != nil {
		t.Errorf("Expected nil modules, got %v", modules)
	}
}

func TestGenerateStableModuleID(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/api", "ckb:mod:"},
		{"internal/query", "ckb:mod:"},
		{"src/components", "ckb:mod:"},
	}

	for _, tt := range tests {
		id := GenerateStableModuleID("", tt.path)
		if id[:8] != tt.expected {
			t.Errorf("Expected ID to start with '%s', got '%s'", tt.expected, id[:8])
		}
	}

	// Verify same path produces same ID
	id1 := GenerateStableModuleID("", "internal/api")
	id2 := GenerateStableModuleID("", "internal/api")
	if id1 != id2 {
		t.Errorf("Expected stable ID, got %s != %s", id1, id2)
	}

	// Verify different paths produce different IDs
	id3 := GenerateStableModuleID("", "internal/query")
	if id1 == id3 {
		t.Errorf("Expected different IDs for different paths, got %s == %s", id1, id3)
	}
}

func TestParseModuleID(t *testing.T) {
	tests := []struct {
		input   string
		prefix  string
		hash    string
		isValid bool
	}{
		{"ckb:mod:abc123", "ckb:mod", "abc123", true},
		{"ckb:mod:1234567890abcdef", "ckb:mod", "1234567890abcdef", true},
		{"invalid", "", "", false},
		{"ckb:sym:abc123", "", "", false},
		{"ckb:mod:", "", "", false},
	}

	for _, tt := range tests {
		prefix, hash, isValid := ParseModuleID(tt.input)
		if isValid != tt.isValid {
			t.Errorf("ParseModuleID(%s): expected isValid=%v, got %v", tt.input, tt.isValid, isValid)
		}
		if isValid {
			if prefix != tt.prefix {
				t.Errorf("ParseModuleID(%s): expected prefix=%s, got %s", tt.input, tt.prefix, prefix)
			}
			if hash != tt.hash {
				t.Errorf("ParseModuleID(%s): expected hash=%s, got %s", tt.input, tt.hash, hash)
			}
		}
	}
}

func TestIsValidModuleID(t *testing.T) {
	if !IsValidModuleID("ckb:mod:abc123") {
		t.Error("Expected 'ckb:mod:abc123' to be valid")
	}
	if IsValidModuleID("invalid") {
		t.Error("Expected 'invalid' to be invalid")
	}
	if IsValidModuleID("ckb:sym:abc123") {
		t.Error("Expected 'ckb:sym:abc123' to be invalid for module ID")
	}
}

func TestWriteAndReadModulesFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-modules-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a modules file
	original := &ModulesFile{
		Version: 1,
		Modules: []ModuleDeclaration{
			{
				Name:           "test",
				Path:           "internal/test",
				Responsibility: "Test module",
				Owner:          "@test-team",
				Tags:           []string{"test"},
			},
		},
	}

	filePath := filepath.Join(tempDir, "MODULES.toml")
	if err := WriteModulesFile(filePath, original); err != nil {
		t.Fatalf("Failed to write modules file: %v", err)
	}

	// Read it back
	parsed, err := ParseModulesFile(filePath)
	if err != nil {
		t.Fatalf("Failed to parse written file: %v", err)
	}

	// Verify content
	if parsed.Version != original.Version {
		t.Errorf("Version mismatch: %d != %d", parsed.Version, original.Version)
	}
	if len(parsed.Modules) != len(original.Modules) {
		t.Errorf("Module count mismatch: %d != %d", len(parsed.Modules), len(original.Modules))
	}
	if parsed.Modules[0].Name != original.Modules[0].Name {
		t.Errorf("Name mismatch: %s != %s", parsed.Modules[0].Name, original.Modules[0].Name)
	}
}

func TestExtractMetadataFromDeclarations(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-modules-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a MODULES.toml file with metadata
	modulesContent := `
version = 1

[[module]]
id = "ckb:mod:custom123"
name = "api"
path = "internal/api"
responsibility = "HTTP API handlers"
owner = "@api-team"
tags = ["core", "api"]

[module.boundaries]
exports = ["Handler"]
`

	modulesPath := filepath.Join(tempDir, "MODULES.toml")
	if err := os.WriteFile(modulesPath, []byte(modulesContent), 0644); err != nil {
		t.Fatalf("Failed to write MODULES.toml: %v", err)
	}

	// Extract metadata
	metadata, err := ExtractMetadataFromDeclarations(modulesPath)
	if err != nil {
		t.Fatalf("Failed to extract metadata: %v", err)
	}

	// Verify metadata was extracted
	if len(metadata) != 1 {
		t.Errorf("Expected 1 metadata entry, got %d", len(metadata))
	}

	// Check for the custom ID entry
	entry, ok := metadata["ckb:mod:custom123"]
	if !ok {
		t.Error("Expected metadata for 'ckb:mod:custom123'")
	} else {
		if entry.Responsibility != "HTTP API handlers" {
			t.Errorf("Expected responsibility 'HTTP API handlers', got '%s'", entry.Responsibility)
		}
		if entry.Owner != "@api-team" {
			t.Errorf("Expected owner '@api-team', got '%s'", entry.Owner)
		}
		if len(entry.Tags) != 2 {
			t.Errorf("Expected 2 tags, got %d", len(entry.Tags))
		}
		if entry.Boundaries == nil {
			t.Error("Expected boundaries to be set")
		} else if len(entry.Boundaries.Exports) != 1 {
			t.Errorf("Expected 1 export, got %d", len(entry.Boundaries.Exports))
		}
	}
}

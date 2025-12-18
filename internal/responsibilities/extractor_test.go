package responsibilities

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFromModule_WithREADME(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a module directory with README
	moduleDir := filepath.Join(tempDir, "internal", "api")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("Failed to create module dir: %v", err)
	}

	// Create a README - bullet points come immediately before first paragraph ends
	readmeContent := `# API Module

- User authentication
- Data management
- Rate limiting

This module provides REST API endpoints for the application.
`
	if err := os.WriteFile(filepath.Join(moduleDir, "README.md"), []byte(readmeContent), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}

	// Extract responsibility
	extractor := NewExtractor(tempDir)
	resp, err := extractor.ExtractFromModule("internal/api")
	if err != nil {
		t.Fatalf("ExtractFromModule failed: %v", err)
	}

	// Verify
	if resp.TargetID != "internal/api" {
		t.Errorf("Expected targetId 'internal/api', got '%s'", resp.TargetID)
	}

	if resp.TargetType != "module" {
		t.Errorf("Expected targetType 'module', got '%s'", resp.TargetType)
	}

	if resp.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	if resp.Source != "declared" {
		t.Errorf("Expected source 'declared', got '%s'", resp.Source)
	}

	if resp.Confidence < 0.8 {
		t.Errorf("Expected confidence >= 0.8 for README source, got %f", resp.Confidence)
	}

	// Capabilities are extracted from bullet points before the paragraph
	if len(resp.Capabilities) != 3 {
		t.Errorf("Expected 3 capabilities from README, got %d", len(resp.Capabilities))
	}
}

func TestExtractFromModule_WithGoDoc(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-godoc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a module directory with Go doc comment
	moduleDir := filepath.Join(tempDir, "internal", "storage")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("Failed to create module dir: %v", err)
	}

	// Create a Go file with package doc comment
	goContent := `// Package storage provides database operations for the application.
// It handles connections, migrations, and query execution.
package storage

type DB struct {}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "db.go"), []byte(goContent), 0644); err != nil {
		t.Fatalf("Failed to write Go file: %v", err)
	}

	// Extract responsibility
	extractor := NewExtractor(tempDir)
	resp, err := extractor.ExtractFromModule("internal/storage")
	if err != nil {
		t.Fatalf("ExtractFromModule failed: %v", err)
	}

	// Verify
	if resp.Summary == "" {
		t.Error("Expected non-empty summary from Go doc comment")
	}

	if resp.Source != "declared" {
		t.Errorf("Expected source 'declared', got '%s'", resp.Source)
	}
}

func TestExtractFromModule_Inferred(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-inferred-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a module directory with only exports (no docs)
	moduleDir := filepath.Join(tempDir, "internal", "utils")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("Failed to create module dir: %v", err)
	}

	// Create a Go file with exports but no package doc
	goContent := `package utils

func FormatDate(t time.Time) string { return "" }
func ParseJSON(data []byte) interface{} { return nil }
type Helper struct {}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "utils.go"), []byte(goContent), 0644); err != nil {
		t.Fatalf("Failed to write Go file: %v", err)
	}

	// Extract responsibility
	extractor := NewExtractor(tempDir)
	resp, err := extractor.ExtractFromModule("internal/utils")
	if err != nil {
		t.Fatalf("ExtractFromModule failed: %v", err)
	}

	// Verify
	if resp.Source != "inferred" {
		t.Errorf("Expected source 'inferred', got '%s'", resp.Source)
	}

	if resp.Confidence > 0.7 {
		t.Errorf("Expected confidence <= 0.7 for inferred source, got %f", resp.Confidence)
	}
}

func TestExtractFromFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file with doc comment
	fileContent := `// handler.go provides HTTP request handlers for user endpoints.
// It includes authentication middleware and response formatting.
package api

func HandleUser(w http.ResponseWriter, r *http.Request) {}
`
	filePath := filepath.Join(tempDir, "handler.go")
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Extract responsibility
	extractor := NewExtractor(tempDir)
	resp, err := extractor.ExtractFromFile("handler.go")
	if err != nil {
		t.Fatalf("ExtractFromFile failed: %v", err)
	}

	// Verify
	if resp.TargetID != "handler.go" {
		t.Errorf("Expected targetId 'handler.go', got '%s'", resp.TargetID)
	}

	if resp.TargetType != "file" {
		t.Errorf("Expected targetType 'file', got '%s'", resp.TargetType)
	}

	if resp.Summary == "" {
		t.Error("Expected non-empty summary from file doc comment")
	}
}

func TestExtractFromFile_InferredFromName(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-file-infer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create files with no doc comments
	testCases := []struct {
		filename string
		contains string
	}{
		{"user_handler.go", "handler"},
		{"config.go", "Configuration"},
		{"utils.go", "Utility"},
		{"user_test.go", "tests"},
		{"service.go", "Service"},
		{"types.go", "types"},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(tempDir, tc.filename)
		if err := os.WriteFile(filePath, []byte("package main\n"), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		extractor := NewExtractor(tempDir)
		resp, err := extractor.ExtractFromFile(tc.filename)
		if err != nil {
			t.Fatalf("ExtractFromFile(%s) failed: %v", tc.filename, err)
		}

		if resp.Source != "inferred" {
			t.Errorf("%s: expected source 'inferred', got '%s'", tc.filename, resp.Source)
		}
	}
}

func TestInferFromFileName(t *testing.T) {
	extractor := &Extractor{}

	tests := []struct {
		filename string
		contains string
	}{
		{"handler.go", "handler"},
		{"user_service.go", "service"},
		{"config.json", "Configuration"},
		{"utils.go", "Utility"},
		{"helper.go", "Utility"},
		{"types.go", "types"},
		{"model.go", "model"},
		{"foo_test.go", "tests"},
	}

	for _, tt := range tests {
		result := extractor.inferFromFileName(tt.filename)
		if result == "" {
			t.Errorf("inferFromFileName(%q) returned empty string", tt.filename)
		}
	}
}

func TestParseReadme(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-resp-readme-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a README with bullet points before first paragraph ends
	readmeContent := `# My Package

- Fast data parsing
- Multiple format support
- Streaming capabilities

This package provides a comprehensive solution for data processing.
`
	readmePath := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}

	extractor := NewExtractor(tempDir)
	summary, capabilities := extractor.parseReadme(readmePath)

	// Should extract the first paragraph
	if summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Should not include the badge line
	if len(summary) > 200 {
		t.Errorf("Summary too long (should be trimmed): %d characters", len(summary))
	}

	// Should extract bullet points as capabilities
	if len(capabilities) != 3 {
		t.Errorf("Expected 3 capabilities, got %d", len(capabilities))
	}
}

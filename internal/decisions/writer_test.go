package decisions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateADR(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-writer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create writer
	writer := NewWriter(tempDir, "docs/decisions")

	// Create an ADR
	adr := &ArchitecturalDecision{
		ID:              "ADR-001",
		Title:           "Use PostgreSQL for persistence",
		Status:          string(StatusProposed),
		Date:            time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Author:          "John Doe",
		Context:         "We need a database for storing user data.",
		Decision:        "We will use PostgreSQL.",
		Consequences:    []string{"Better JSON support", "Team training needed"},
		AffectedModules: []string{"internal/storage", "internal/api"},
		Alternatives:    []string{"MySQL", "MongoDB"},
	}

	// Create the ADR
	relPath, err := writer.CreateADR(adr)
	if err != nil {
		t.Fatalf("CreateADR failed: %v", err)
	}

	// Verify file was created
	fullPath := filepath.Join(tempDir, relPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("ADR file was not created: %s", fullPath)
	}

	// Verify file path format
	if !strings.HasPrefix(relPath, "docs/decisions/") {
		t.Errorf("Expected path to start with 'docs/decisions/', got '%s'", relPath)
	}

	if !strings.HasSuffix(relPath, ".md") {
		t.Errorf("Expected path to end with '.md', got '%s'", relPath)
	}

	// Read and verify content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read ADR file: %v", err)
	}

	contentStr := string(content)

	// Verify key sections exist
	expectedSections := []string{
		"# ADR-001: Use PostgreSQL for persistence",
		"**Status:** proposed",
		"**Date:** 2024-01-15",
		"**Author:** John Doe",
		"## Context",
		"We need a database for storing user data.",
		"## Decision",
		"We will use PostgreSQL.",
		"## Consequences",
		"- Better JSON support",
		"- Team training needed",
		"## Affected Modules",
		"- internal/storage",
		"- internal/api",
		"## Alternatives Considered",
		"- MySQL",
		"- MongoDB",
	}

	for _, section := range expectedSections {
		if !strings.Contains(contentStr, section) {
			t.Errorf("Expected ADR to contain '%s'", section)
		}
	}
}

func TestCreateADR_AlreadyExists(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-exists-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create writer
	writer := NewWriter(tempDir, "docs/decisions")

	// Create an ADR
	adr := NewADR(1, "Test Decision")
	adr.Context = "Test context"
	adr.Decision = "Test decision"
	adr.Consequences = []string{"Test consequence"}

	// Create the ADR first time
	_, err = writer.CreateADR(adr)
	if err != nil {
		t.Fatalf("First CreateADR failed: %v", err)
	}

	// Try to create again with same ID
	adr2 := NewADR(1, "Test Decision")
	adr2.Context = "Test context"
	adr2.Decision = "Test decision"
	adr2.Consequences = []string{"Test consequence"}

	_, err = writer.CreateADR(adr2)
	if err == nil {
		t.Error("Expected error when creating duplicate ADR")
	}
}

func TestUpdateADR(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-update-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create writer
	writer := NewWriter(tempDir, "docs/decisions")

	// Create an ADR
	adr := NewADR(1, "Test Decision")
	adr.Context = "Original context"
	adr.Decision = "Original decision"
	adr.Consequences = []string{"Original consequence"}

	// Create the ADR
	relPath, err := writer.CreateADR(adr)
	if err != nil {
		t.Fatalf("CreateADR failed: %v", err)
	}

	// Update the ADR
	adr.Status = string(StatusAccepted)
	adr.Context = "Updated context"
	adr.Consequences = []string{"Updated consequence 1", "Updated consequence 2"}

	err = writer.UpdateADR(adr)
	if err != nil {
		t.Fatalf("UpdateADR failed: %v", err)
	}

	// Read and verify updated content
	fullPath := filepath.Join(tempDir, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read updated ADR: %v", err)
	}

	contentStr := string(content)

	// Verify updates
	if !strings.Contains(contentStr, "**Status:** accepted") {
		t.Error("Expected status to be updated to 'accepted'")
	}

	if !strings.Contains(contentStr, "Updated context") {
		t.Error("Expected context to be updated")
	}

	if !strings.Contains(contentStr, "Updated consequence 1") {
		t.Error("Expected consequences to be updated")
	}
}

func TestGenerateFilename(t *testing.T) {
	tests := []struct {
		id       string
		title    string
		expected string
	}{
		{"ADR-001", "Use PostgreSQL", "adr-001-use-postgresql.md"},
		{"ADR-002", "API Framework Choice", "adr-002-api-framework-choice.md"},
		{"ADR-003", "Handle Special!@#$Characters", "adr-003-handle-specialcharacters.md"},
		{"ADR-004", "Very Long Title That Should Be Truncated To Fit Within Maximum Length Limit", "adr-004-very-long-title-that-should-be-truncated-to-fit-wi.md"},
	}

	for _, tt := range tests {
		result := generateFilename(tt.id, tt.title)
		if result != tt.expected {
			t.Errorf("generateFilename(%q, %q) = %q, expected %q", tt.id, tt.title, result, tt.expected)
		}
	}
}

func TestNewADR(t *testing.T) {
	adr := NewADR(42, "Test Title")

	if adr.ID != "ADR-042" {
		t.Errorf("Expected ID 'ADR-042', got '%s'", adr.ID)
	}

	if adr.Title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got '%s'", adr.Title)
	}

	if adr.Status != string(StatusProposed) {
		t.Errorf("Expected status 'proposed', got '%s'", adr.Status)
	}

	if adr.Date.IsZero() {
		t.Error("Expected date to be set")
	}
}

func TestGetDefaultOutputDir(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-default-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// No directories exist - should return default
	result := GetDefaultOutputDir(tempDir)
	if result != "docs/decisions" {
		t.Errorf("Expected 'docs/decisions', got '%s'", result)
	}

	// Create docs/adr - should return that
	adrDir := filepath.Join(tempDir, "docs", "adr")
	if err := os.MkdirAll(adrDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	result = GetDefaultOutputDir(tempDir)
	if result != "docs/adr" {
		t.Errorf("Expected 'docs/adr', got '%s'", result)
	}

	// Create docs/decisions - should prefer that
	decisionsDir := filepath.Join(tempDir, "docs", "decisions")
	if err := os.MkdirAll(decisionsDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	result = GetDefaultOutputDir(tempDir)
	if result != "docs/decisions" {
		t.Errorf("Expected 'docs/decisions', got '%s'", result)
	}
}

func TestEnsureOutputDir(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-ensure-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	writer := NewWriter(tempDir, "docs/decisions")

	outputDir, err := writer.EnsureOutputDir()
	if err != nil {
		t.Fatalf("EnsureOutputDir failed: %v", err)
	}

	if outputDir != "docs/decisions" {
		t.Errorf("Expected 'docs/decisions', got '%s'", outputDir)
	}

	// Verify directory was created
	fullPath := filepath.Join(tempDir, "docs", "decisions")
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		t.Error("Directory was not created")
	}
	if !info.IsDir() {
		t.Error("Path is not a directory")
	}
}

func TestIsValidStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{"proposed", true},
		{"accepted", true},
		{"deprecated", true},
		{"superseded", true},
		{"invalid", false},
		{"PROPOSED", false}, // case-sensitive
		{"", false},
	}

	for _, tt := range tests {
		result := IsValidStatus(tt.status)
		if result != tt.expected {
			t.Errorf("IsValidStatus(%q) = %v, expected %v", tt.status, result, tt.expected)
		}
	}
}

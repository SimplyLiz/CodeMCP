package decisions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseADRFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create docs/decisions directory
	adrDir := filepath.Join(tempDir, "docs", "decisions")
	if err := os.MkdirAll(adrDir, 0755); err != nil {
		t.Fatalf("Failed to create ADR dir: %v", err)
	}

	// Create an ADR file
	adrContent := `# ADR-001: Use PostgreSQL for persistence

**Status:** accepted

**Date:** 2024-01-15

**Author:** John Doe

## Context

We need a database for storing user data. The team has experience with both MySQL and PostgreSQL.

## Decision

We will use PostgreSQL because it offers better JSON support and advanced features.

## Consequences

- Better JSON query performance
- Team needs PostgreSQL training
- Migration from current SQLite required

## Affected Modules

- internal/storage
- internal/api

## Alternatives Considered

- MySQL - widely used but less JSON support
- MongoDB - document database, different paradigm
`

	adrPath := filepath.Join(adrDir, "adr-001-use-postgresql.md")
	if err := os.WriteFile(adrPath, []byte(adrContent), 0644); err != nil {
		t.Fatalf("Failed to write ADR file: %v", err)
	}

	// Parse the file
	parser := NewParser(tempDir)
	adr, err := parser.ParseFile("docs/decisions/adr-001-use-postgresql.md")
	if err != nil {
		t.Fatalf("Failed to parse ADR: %v", err)
	}

	// Verify parsed values
	if adr.ID != "ADR-001" {
		t.Errorf("Expected ID 'ADR-001', got '%s'", adr.ID)
	}

	if adr.Title != "Use PostgreSQL for persistence" {
		t.Errorf("Expected title 'Use PostgreSQL for persistence', got '%s'", adr.Title)
	}

	if adr.Status != "accepted" {
		t.Errorf("Expected status 'accepted', got '%s'", adr.Status)
	}

	if adr.Author != "John Doe" {
		t.Errorf("Expected author 'John Doe', got '%s'", adr.Author)
	}

	expectedDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !adr.Date.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, adr.Date)
	}

	if len(adr.Consequences) != 3 {
		t.Errorf("Expected 3 consequences, got %d", len(adr.Consequences))
	}

	if len(adr.AffectedModules) != 2 {
		t.Errorf("Expected 2 affected modules, got %d", len(adr.AffectedModules))
	}

	if len(adr.Alternatives) != 2 {
		t.Errorf("Expected 2 alternatives, got %d", len(adr.Alternatives))
	}
}

func TestParseDirectory(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create docs/decisions directory
	adrDir := filepath.Join(tempDir, "docs", "decisions")
	if err := os.MkdirAll(adrDir, 0755); err != nil {
		t.Fatalf("Failed to create ADR dir: %v", err)
	}

	// Create multiple ADR files
	adrs := []struct {
		filename string
		content  string
	}{
		{
			"adr-001-database.md",
			`# ADR-001: Database Choice

**Status:** accepted
**Date:** 2024-01-01

## Context
Need a database.

## Decision
Use PostgreSQL.

## Consequences
- Good performance
`,
		},
		{
			"adr-002-api-framework.md",
			`# ADR-002: API Framework

**Status:** proposed
**Date:** 2024-01-15

## Context
Need an API framework.

## Decision
Use Gin.

## Consequences
- Fast routing
`,
		},
		{
			"readme.md", // Should be ignored
			`# README

This is not an ADR.
`,
		},
	}

	for _, adr := range adrs {
		path := filepath.Join(adrDir, adr.filename)
		if err := os.WriteFile(path, []byte(adr.content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", adr.filename, err)
		}
	}

	// Parse directory
	parser := NewParser(tempDir)
	parsedADRs, err := parser.ParseDirectory("docs/decisions")
	if err != nil {
		t.Fatalf("Failed to parse directory: %v", err)
	}

	// Should find 2 ADRs (not the readme)
	if len(parsedADRs) != 2 {
		t.Errorf("Expected 2 ADRs, got %d", len(parsedADRs))
	}

	// Verify IDs
	ids := make(map[string]bool)
	for _, adr := range parsedADRs {
		ids[adr.ID] = true
	}

	if !ids["ADR-001"] {
		t.Error("Expected to find ADR-001")
	}
	if !ids["ADR-002"] {
		t.Error("Expected to find ADR-002")
	}
}

func TestFindADRDirectories(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-dirs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create various ADR directories
	dirs := []string{
		"docs/decisions",
		"docs/adr",
		"adr",
	}

	for _, dir := range dirs {
		path := filepath.Join(tempDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	// Find directories
	parser := NewParser(tempDir)
	found := parser.FindADRDirectories()

	if len(found) != 3 {
		t.Errorf("Expected 3 ADR directories, got %d: %v", len(found), found)
	}
}

func TestGetNextADRNumber(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-adr-num-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create docs/decisions directory
	adrDir := filepath.Join(tempDir, "docs", "decisions")
	if err := os.MkdirAll(adrDir, 0755); err != nil {
		t.Fatalf("Failed to create ADR dir: %v", err)
	}

	parser := NewParser(tempDir)

	// No ADRs exist, should return 1
	num, err := parser.GetNextADRNumber()
	if err != nil {
		t.Fatalf("GetNextADRNumber failed: %v", err)
	}
	if num != 1 {
		t.Errorf("Expected next number 1, got %d", num)
	}

	// Create some ADRs
	adrs := []string{
		"adr-001-first.md",
		"adr-003-third.md", // Note: skipped 002
		"adr-005-fifth.md",
	}

	for i, filename := range adrs {
		var content []byte
		if i == 0 {
			content = []byte(`# ADR-001: First

**Status:** proposed
**Date:** 2024-01-01

## Context
Test.

## Decision
Test.

## Consequences
- Test
`)
		} else if i == 1 {
			content = []byte(`# ADR-003: Third

**Status:** proposed
**Date:** 2024-01-01

## Context
Test.

## Decision
Test.

## Consequences
- Test
`)
		} else {
			content = []byte(`# ADR-005: Fifth

**Status:** proposed
**Date:** 2024-01-01

## Context
Test.

## Decision
Test.

## Consequences
- Test
`)
		}
		path := filepath.Join(adrDir, filename)
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	// Should return 6 (max is 5)
	num, err = parser.GetNextADRNumber()
	if err != nil {
		t.Fatalf("GetNextADRNumber failed: %v", err)
	}
	if num != 6 {
		t.Errorf("Expected next number 6, got %d", num)
	}
}

func TestNormalizeADRID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ADR-1", "ADR-001"},
		{"ADR1", "ADR-001"},
		{"ADR 1", "ADR-001"},
		{"ADR-12", "ADR-012"},
		{"ADR-123", "ADR-123"},
		{"adr-001", "ADR-001"},
	}

	for _, tt := range tests {
		result := normalizeADRID(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeADRID(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsADRFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"adr-001-test.md", true},
		{"ADR-001-test.md", true},
		{"adr001-test.md", true},
		{"001-test.md", true},
		{"0001-test.md", true},
		{"readme.md", false},
		{"test.md", false},
		{"index.md", false},
	}

	for _, tt := range tests {
		result := isADRFile(tt.filename)
		if result != tt.expected {
			t.Errorf("isADRFile(%q) = %v, expected %v", tt.filename, result, tt.expected)
		}
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		hasError bool
	}{
		{"2024-01-15", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"January 15, 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"Jan 15, 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"2024/01/15", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"invalid", time.Time{}, true},
	}

	for _, tt := range tests {
		result, err := parseDate(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("parseDate(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseDate(%q) unexpected error: %v", tt.input, err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("parseDate(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		}
	}
}

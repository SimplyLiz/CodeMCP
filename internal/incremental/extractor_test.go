package incremental

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/backends/scip"
	"ckb/internal/logging"
)

func TestIsLocalSymbol(t *testing.T) {
	tests := []struct {
		symbolID string
		expected bool
	}{
		{"local 0", true},
		{"local 123", true},
		{"scip-go gomod example.com/foo 1.0 pkg.Func().", false},
		{"", false},
		{"loca", false},  // Too short to be "local "
		{"local", false}, // Missing space
	}

	for _, tc := range tests {
		t.Run(tc.symbolID, func(t *testing.T) {
			result := isLocalSymbol(tc.symbolID)
			if result != tc.expected {
				t.Errorf("isLocalSymbol(%q) = %v, want %v", tc.symbolID, result, tc.expected)
			}
		})
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		name        string
		symbolID    string
		displayName string
		expected    string
	}{
		{
			name:        "uses displayName if provided",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Func().",
			displayName: "Func",
			expected:    "Func",
		},
		{
			name:        "extracts from symbolID without displayName",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Func().",
			displayName: "",
			expected:    "Func",
		},
		{
			name:        "handles method names",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Type.Method().",
			displayName: "",
			expected:    "Method",
		},
		{
			name:        "handles type names",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Type.",
			displayName: "",
			expected:    "Type",
		},
		{
			name:        "handles short symbolID",
			symbolID:    "short",
			displayName: "",
			expected:    "short",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractSymbolName(tc.symbolID, tc.displayName)
			if result != tc.expected {
				t.Errorf("extractSymbolName(%q, %q) = %q, want %q",
					tc.symbolID, tc.displayName, result, tc.expected)
			}
		})
	}
}

func TestMapSymbolKind(t *testing.T) {
	tests := []struct {
		kind     int32
		expected string
	}{
		{0, "unknown"},
		{5, "class"},
		{6, "method"},
		{8, "field"},
		{12, "function"},
		{13, "variable"},
		{14, "constant"},
		{23, "struct"},
		{999, "unknown"}, // Unknown value
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := mapSymbolKind(tc.kind)
			if result != tc.expected {
				t.Errorf("mapSymbolKind(%d) = %q, want %q", tc.kind, result, tc.expected)
			}
		})
	}
}

func TestNewSCIPExtractor(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Test with relative path
	ext := NewSCIPExtractor("/repo", ".scip/index.scip", logger)
	if ext == nil {
		t.Fatal("expected non-nil extractor")
	}
	if ext.repoRoot != "/repo" {
		t.Errorf("expected repoRoot '/repo', got %q", ext.repoRoot)
	}
	expectedPath := filepath.Join("/repo", ".scip/index.scip")
	if ext.indexPath != expectedPath {
		t.Errorf("expected indexPath %q, got %q", expectedPath, ext.indexPath)
	}
}

func TestNewSCIPExtractor_AbsolutePath(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Test with absolute path
	ext := NewSCIPExtractor("/repo", "/custom/path/index.scip", logger)
	if ext.indexPath != "/custom/path/index.scip" {
		t.Errorf("expected absolute path to be preserved, got %q", ext.indexPath)
	}
}

func TestHashFile_Extractor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.go")
	content := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	hash1, err := hashFile(testFile)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}
	if hash1 == "" {
		t.Error("expected non-empty hash")
	}

	// Same content, same hash
	hash2, err := hashFile(testFile)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}
	if hash1 != hash2 {
		t.Error("expected same hash for same file")
	}

	// Different content, different hash
	if err := os.WriteFile(testFile, []byte("different content"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}
	hash3, err := hashFile(testFile)
	if err != nil {
		t.Fatalf("hashFile failed: %v", err)
	}
	if hash1 == hash3 {
		t.Error("expected different hash for different content")
	}
}

func TestHashFile_Extractor_NonExistent(t *testing.T) {
	_, err := hashFile("/nonexistent/file.go")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestComputeDocHash(t *testing.T) {
	doc1 := &scip.Document{
		RelativePath: "main.go",
		Occurrences: []*scip.Occurrence{
			{Symbol: "pkg.Foo", Range: []int32{10, 0, 10, 3}, SymbolRoles: 1},
		},
		Symbols: []*scip.SymbolInformation{
			{Symbol: "pkg.Foo", Kind: 12},
		},
	}

	hash1 := computeDocHash(doc1)
	if hash1 == "" {
		t.Error("expected non-empty hash")
	}
	if len(hash1) != 16 {
		t.Errorf("expected 16-char hash, got %d chars", len(hash1))
	}

	// Same document, same hash
	hash2 := computeDocHash(doc1)
	if hash1 != hash2 {
		t.Error("expected same hash for same document")
	}

	// Different document, different hash
	doc2 := &scip.Document{
		RelativePath: "main.go",
		Occurrences: []*scip.Occurrence{
			{Symbol: "pkg.Bar", Range: []int32{10, 0, 10, 3}, SymbolRoles: 1},
		},
	}
	hash3 := computeDocHash(doc2)
	if hash1 == hash3 {
		t.Error("expected different hash for different document")
	}

	// Different path, different hash
	doc3 := &scip.Document{
		RelativePath: "other.go",
		Occurrences: []*scip.Occurrence{
			{Symbol: "pkg.Foo", Range: []int32{10, 0, 10, 3}, SymbolRoles: 1},
		},
	}
	hash4 := computeDocHash(doc3)
	if hash1 == hash4 {
		t.Error("expected different hash for different path")
	}
}

func TestExtractFileDelta(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor(tmpDir, ".scip/index.scip", logger)

	// Create a mock SCIP document
	doc := &scip.Document{
		RelativePath: "main.go",
		Language:     "go",
		Occurrences: []*scip.Occurrence{
			// Definition
			{Symbol: "scip-go gomod example 1.0 main.main().", Range: []int32{5, 5, 5, 9}, SymbolRoles: 1},
			// Reference
			{Symbol: "scip-go gomod fmt 1.0 fmt.Println().", Range: []int32{6, 1, 6, 8}, SymbolRoles: 0},
			// Local symbol (should be skipped)
			{Symbol: "local 0", Range: []int32{7, 1, 7, 5}, SymbolRoles: 1},
		},
		Symbols: []*scip.SymbolInformation{
			{Symbol: "scip-go gomod example 1.0 main.main().", DisplayName: "main", Kind: 12},
		},
	}

	change := ChangedFile{
		Path:       "main.go",
		ChangeType: ChangeAdded,
	}

	delta := ext.extractFileDelta(doc, change)

	if delta.Path != "main.go" {
		t.Errorf("expected path 'main.go', got %q", delta.Path)
	}
	if delta.ChangeType != ChangeAdded {
		t.Errorf("expected change type 'added', got %q", delta.ChangeType)
	}

	// Should have 1 symbol (local symbol filtered out)
	if len(delta.Symbols) != 1 {
		t.Errorf("expected 1 symbol, got %d", len(delta.Symbols))
	}
	if len(delta.Symbols) > 0 {
		sym := delta.Symbols[0]
		if sym.Name != "main" {
			t.Errorf("expected symbol name 'main', got %q", sym.Name)
		}
		if sym.Kind != "function" {
			t.Errorf("expected kind 'function', got %q", sym.Kind)
		}
		if sym.StartLine != 6 { // SCIP is 0-indexed, we use 1-indexed
			t.Errorf("expected StartLine 6, got %d", sym.StartLine)
		}
	}

	// Should have 1 reference (local symbols filtered out)
	if len(delta.Refs) != 1 {
		t.Errorf("expected 1 reference, got %d", len(delta.Refs))
	}
}

func TestExtractFileDelta_Rename(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file at the new path
	testFile := filepath.Join(tmpDir, "new.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor(tmpDir, ".scip/index.scip", logger)

	doc := &scip.Document{
		RelativePath: "new.go",
		Language:     "go",
		Occurrences:  []*scip.Occurrence{},
	}

	change := ChangedFile{
		Path:       "new.go",
		OldPath:    "old.go",
		ChangeType: ChangeRenamed,
	}

	delta := ext.extractFileDelta(doc, change)

	if delta.Path != "new.go" {
		t.Errorf("expected path 'new.go', got %q", delta.Path)
	}
	if delta.OldPath != "old.go" {
		t.Errorf("expected oldPath 'old.go', got %q", delta.OldPath)
	}
	if delta.ChangeType != ChangeRenamed {
		t.Errorf("expected change type 'renamed', got %q", delta.ChangeType)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.IndexPath != ".scip/index.scip" {
		t.Errorf("expected IndexPath '.scip/index.scip', got %q", config.IndexPath)
	}
	if config.IncrementalThreshold != 50 {
		t.Errorf("expected IncrementalThreshold 50, got %d", config.IncrementalThreshold)
	}
	if config.IndexTests {
		t.Error("expected IndexTests=false by default")
	}
}

func TestMapSymbolKind_AllValues(t *testing.T) {
	// Test all documented kind values
	kinds := map[int32]string{
		0:  "unknown",
		1:  "file",
		2:  "module",
		3:  "namespace",
		4:  "package",
		5:  "class",
		6:  "method",
		7:  "property",
		8:  "field",
		9:  "constructor",
		10: "enum",
		11: "interface",
		12: "function",
		13: "variable",
		14: "constant",
		15: "string",
		16: "number",
		17: "boolean",
		18: "array",
		19: "object",
		20: "key",
		21: "null",
		22: "enum_member",
		23: "struct",
		24: "event",
		25: "operator",
		26: "type_parameter",
	}

	for kind, expected := range kinds {
		result := mapSymbolKind(kind)
		if result != expected {
			t.Errorf("mapSymbolKind(%d) = %q, want %q", kind, result, expected)
		}
	}
}

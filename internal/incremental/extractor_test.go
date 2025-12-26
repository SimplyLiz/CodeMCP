package incremental

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/backends/scip"
	"ckb/internal/logging"
	"ckb/internal/project"
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

// v1.1 Callgraph Tests

func TestIsFunctionSymbol(t *testing.T) {
	tests := []struct {
		symbolID string
		expected bool
	}{
		// Functions should match
		{"scip-go gomod example.com/foo 1.0 pkg.Func().", true},
		{"scip-go gomod example.com/foo 1.0 main.main().", true},
		// Methods should match
		{"scip-go gomod example.com/foo 1.0 pkg.Type.Method().", true},
		// Types should NOT match
		{"scip-go gomod example.com/foo 1.0 pkg.Type.", false},
		// Variables should NOT match
		{"scip-go gomod example.com/foo 1.0 pkg.Variable.", false},
		// Empty string
		{"", false},
		// Has "()" but not "()." pattern
		{"has()something.", false},
		// Has "()." pattern
		{"has().something", true},
	}

	for _, tc := range tests {
		t.Run(tc.symbolID, func(t *testing.T) {
			result := isFunctionSymbol(tc.symbolID)
			if result != tc.expected {
				t.Errorf("isFunctionSymbol(%q) = %v, want %v", tc.symbolID, result, tc.expected)
			}
		})
	}
}

func TestIsCallableKind(t *testing.T) {
	tests := []struct {
		kind     int32
		expected bool
	}{
		{6, true},   // Method
		{9, true},   // Constructor
		{12, true},  // Function
		{5, false},  // Class
		{8, false},  // Field
		{13, false}, // Variable
		{23, false}, // Struct
		{0, false},  // Unknown
	}

	for _, tc := range tests {
		t.Run(mapSymbolKind(tc.kind), func(t *testing.T) {
			result := isCallableKind(tc.kind)
			if result != tc.expected {
				t.Errorf("isCallableKind(%d) = %v, want %v", tc.kind, result, tc.expected)
			}
		})
	}
}

func TestIsCallable(t *testing.T) {
	// Test with SymbolInformation available (Tier 1)
	symbolInfo := map[string]*scip.SymbolInformation{
		"scip-go gomod example 1.0 pkg.Func().":   {Symbol: "scip-go gomod example 1.0 pkg.Func().", Kind: 12},
		"scip-go gomod example 1.0 pkg.Method().": {Symbol: "scip-go gomod example 1.0 pkg.Method().", Kind: 6},
		"scip-go gomod example 1.0 pkg.Type.":     {Symbol: "scip-go gomod example 1.0 pkg.Type.", Kind: 5},
		"scip-go gomod example 1.0 pkg.NoKind().": {Symbol: "scip-go gomod example 1.0 pkg.NoKind().", Kind: 0}, // Kind=0 means unknown
	}

	tests := []struct {
		name     string
		symbolID string
		expected bool
	}{
		{"function with Kind", "scip-go gomod example 1.0 pkg.Func().", true},
		{"method with Kind", "scip-go gomod example 1.0 pkg.Method().", true},
		{"class type", "scip-go gomod example 1.0 pkg.Type.", false},
		{"unknown kind falls back to heuristic", "scip-go gomod example 1.0 pkg.NoKind().", true},
		{"not in symbolInfo uses heuristic", "scip-go gomod example 1.0 other.Func().", true},
		{"not in symbolInfo non-func", "scip-go gomod example 1.0 other.Variable.", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isCallable(tc.symbolID, symbolInfo)
			if result != tc.expected {
				t.Errorf("isCallable(%q) = %v, want %v", tc.symbolID, result, tc.expected)
			}
		})
	}
}

func TestResolveCallerSymbol(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor("/repo", ".scip/index.scip", logger)

	symbols := []Symbol{
		{ID: "scip-go gomod example 1.0 pkg.init().", Name: "init", Kind: "function", StartLine: 5, EndLine: 10},
		{ID: "scip-go gomod example 1.0 pkg.main().", Name: "main", Kind: "function", StartLine: 12, EndLine: 30},
		{ID: "scip-go gomod example 1.0 pkg.helper().", Name: "helper", Kind: "function", StartLine: 32, EndLine: 40},
		{ID: "scip-go gomod example 1.0 pkg.Type.", Name: "Type", Kind: "struct", StartLine: 1, EndLine: 3}, // Not a callable
	}

	tests := []struct {
		name     string
		callLine int
		expected string
	}{
		{"call in init", 7, "scip-go gomod example 1.0 pkg.init()."},
		{"call in main start", 12, "scip-go gomod example 1.0 pkg.main()."},
		{"call in main middle", 20, "scip-go gomod example 1.0 pkg.main()."},
		{"call in main end", 30, "scip-go gomod example 1.0 pkg.main()."},
		{"call in helper", 35, "scip-go gomod example 1.0 pkg.helper()."},
		{"call before any function", 1, ""},  // Should return empty (top-level or struct def line)
		{"call after all functions", 50, ""}, // Should return empty
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ext.resolveCallerSymbol(symbols, tc.callLine)
			if result != tc.expected {
				t.Errorf("resolveCallerSymbol(..., %d) = %q, want %q", tc.callLine, result, tc.expected)
			}
		})
	}
}

func TestResolveCallerSymbol_InferredEndLine(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor("/repo", ".scip/index.scip", logger)

	// Symbols without explicit EndLine (EndLine == StartLine or 0)
	symbols := []Symbol{
		{ID: "scip-go gomod example 1.0 pkg.first().", Name: "first", Kind: "function", StartLine: 10, EndLine: 10},
		{ID: "scip-go gomod example 1.0 pkg.second().", Name: "second", Kind: "function", StartLine: 50, EndLine: 50},
	}

	tests := []struct {
		name     string
		callLine int
		expected string
	}{
		// First function's inferred end is second's start - 1 = 49
		{"call in first function", 25, "scip-go gomod example 1.0 pkg.first()."},
		{"call at inferred boundary", 49, "scip-go gomod example 1.0 pkg.first()."},
		// Second function uses default 500 lines
		{"call in second function", 100, "scip-go gomod example 1.0 pkg.second()."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ext.resolveCallerSymbol(symbols, tc.callLine)
			if result != tc.expected {
				t.Errorf("resolveCallerSymbol(..., %d) = %q, want %q", tc.callLine, result, tc.expected)
			}
		})
	}
}

func TestExtractFileDelta_CallEdges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor(tmpDir, ".scip/index.scip", logger)

	// Create a document with a function definition and a call
	doc := &scip.Document{
		RelativePath: "main.go",
		Language:     "go",
		Occurrences: []*scip.Occurrence{
			// Definition of main.main
			{Symbol: "scip-go gomod example 1.0 main.main().", Range: []int32{5, 5, 5, 9}, SymbolRoles: 1},
			// Call to fmt.Println from within main
			{Symbol: "scip-go gomod fmt 1.0 fmt.Println().", Range: []int32{7, 1, 7, 12}, SymbolRoles: 0},
			// Reference to a non-callable (should not create call edge)
			{Symbol: "scip-go gomod example 1.0 pkg.Variable.", Range: []int32{8, 1, 8, 8}, SymbolRoles: 0},
		},
		Symbols: []*scip.SymbolInformation{
			{Symbol: "scip-go gomod example 1.0 main.main().", DisplayName: "main", Kind: 12},
			{Symbol: "scip-go gomod fmt 1.0 fmt.Println().", DisplayName: "Println", Kind: 12},
		},
	}

	change := ChangedFile{
		Path:       "main.go",
		ChangeType: ChangeAdded,
	}

	delta := ext.extractFileDelta(doc, change)

	// Should have 1 call edge (to fmt.Println)
	if len(delta.CallEdges) != 1 {
		t.Fatalf("expected 1 call edge, got %d", len(delta.CallEdges))
	}

	edge := delta.CallEdges[0]
	if edge.CalleeID != "scip-go gomod fmt 1.0 fmt.Println()." {
		t.Errorf("expected callee 'fmt.Println', got %q", edge.CalleeID)
	}
	if edge.CallerFile != "main.go" {
		t.Errorf("expected caller file 'main.go', got %q", edge.CallerFile)
	}
	if edge.Line != 8 { // SCIP 0-indexed, we use 1-indexed
		t.Errorf("expected line 8, got %d", edge.Line)
	}
	// CallerID should be resolved to main.main since the call is on line 8, within main (lines 6-end)
	if edge.CallerID != "scip-go gomod example 1.0 main.main()." {
		t.Errorf("expected caller 'main.main', got %q", edge.CallerID)
	}
}

// Multi-language support tests (v7.6)

func TestIsIndexerInstalled(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor("/repo", ".scip/index.scip", logger)

	tests := []struct {
		name   string
		lang   project.Language
		exists bool // Whether the test expects the indexer to exist (may vary by environment)
	}{
		// These tests check that IsIndexerInstalled doesn't panic
		// Actual availability depends on the test environment
		{"Go", project.LangGo, false}, // scip-go typically not in test env
		{"TypeScript", project.LangTypeScript, false},
		{"Python", project.LangPython, false},
		{"Dart", project.LangDart, false},
		{"Rust", project.LangRust, false},
		{"Java", project.LangJava, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := project.GetIndexerConfig(tt.lang)
			if config == nil {
				t.Skipf("no indexer config for %s", tt.lang)
			}

			// Should not panic
			_ = ext.IsIndexerInstalled(config)
		})
	}
}

func TestRunIndexer_CreatesOutputDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-runindexer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a nested output path that doesn't exist
	outputPath := filepath.Join(tmpDir, "nested", "deep", "index.scip")

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor(tmpDir, outputPath, logger)

	config := project.GetIndexerConfig(project.LangGo)
	if config == nil {
		t.Skip("no Go indexer config")
	}

	// RunIndexer should create the output directory
	// (It will fail because scip-go isn't installed, but the directory should be created first)
	_ = ext.RunIndexer(config)

	// Check that the directory was created
	outputDir := filepath.Dir(outputPath)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Errorf("expected output directory %s to be created", outputDir)
	}
}

func TestRunSCIPGo_Deprecated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extractor-runscipgo-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	ext := NewSCIPExtractor(tmpDir, ".scip/index.scip", logger)

	// RunSCIPGo should call RunIndexer internally
	// It will fail because scip-go isn't installed, but it shouldn't panic
	err = ext.RunSCIPGo()

	// We expect an error (indexer not found or similar)
	if err == nil {
		t.Skip("scip-go is installed, test passes")
	}

	// Error should be about the indexer failing, not a nil pointer or similar
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestGetIndexerConfigForAllLanguages(t *testing.T) {
	// Verify that GetIndexerConfig returns correct configs for supported languages
	supportedLangs := []project.Language{
		project.LangGo,
		project.LangTypeScript,
		project.LangJavaScript,
		project.LangPython,
		project.LangDart,
		project.LangRust,
	}

	for _, lang := range supportedLangs {
		t.Run(string(lang), func(t *testing.T) {
			config := project.GetIndexerConfig(lang)
			if config == nil {
				t.Errorf("expected non-nil config for %s", lang)
				return
			}

			if config.Cmd == "" {
				t.Errorf("expected non-empty Cmd for %s", lang)
			}

			if !config.SupportsIncremental {
				t.Errorf("expected SupportsIncremental=true for %s", lang)
			}
		})
	}

	// Verify unsupported languages
	unsupportedLangs := []project.Language{
		project.LangJava,
		project.LangKotlin,
		project.LangCpp,
		project.LangRuby,
		project.LangCSharp,
		project.LangPHP,
	}

	for _, lang := range unsupportedLangs {
		t.Run(string(lang)+"_no_incremental", func(t *testing.T) {
			config := project.GetIndexerConfig(lang)
			if config == nil {
				t.Skipf("no config for %s", lang)
			}

			if config.SupportsIncremental {
				t.Errorf("expected SupportsIncremental=false for %s", lang)
			}
		})
	}
}

func TestIndexerConfigBuildCommand(t *testing.T) {
	tests := []struct {
		name       string
		lang       project.Language
		outputPath string
		wantCmd    string
	}{
		{
			name:       "Go with output",
			lang:       project.LangGo,
			outputPath: "/tmp/index.scip",
			wantCmd:    "scip-go",
		},
		{
			name:       "TypeScript with output",
			lang:       project.LangTypeScript,
			outputPath: "/tmp/index.scip",
			wantCmd:    "scip-typescript",
		},
		{
			name:       "Rust (fixed output)",
			lang:       project.LangRust,
			outputPath: "/tmp/index.scip",
			wantCmd:    "rust-analyzer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := project.GetIndexerConfig(tt.lang)
			if config == nil {
				t.Skipf("no config for %s", tt.lang)
			}

			cmd := config.BuildCommand(tt.outputPath)
			if cmd == nil {
				t.Fatal("expected non-nil command")
			}

			// Check command name
			if len(cmd.Args) == 0 || cmd.Args[0] != tt.wantCmd {
				t.Errorf("expected command %s, got %v", tt.wantCmd, cmd.Args)
			}
		})
	}
}

package suggest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity string
		rank     int
	}{
		{"critical", 4},
		{"high", 3},
		{"medium", 2},
		{"low", 1},
		{"unknown", 0},
		{"", 0},
	}

	for _, tc := range tests {
		got := severityRank(tc.severity)
		if got != tc.rank {
			t.Errorf("severityRank(%q) = %d, want %d", tc.severity, got, tc.rank)
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	suggestions := []Suggestion{
		{Type: SuggestExtractFunction, Severity: "low", Target: "a.go"},
		{Type: SuggestSplitFile, Severity: "medium", Target: "b.go"},
		{Type: SuggestReduceCoupling, Severity: "high", Target: "c.go"},
		{Type: SuggestRemoveDeadCode, Severity: "critical", Target: "d.go"},
	}

	tests := []struct {
		minSeverity string
		wantCount   int
	}{
		{"low", 4},
		{"medium", 3},
		{"high", 2},
		{"critical", 1},
	}

	for _, tc := range tests {
		filtered := filterBySeverity(suggestions, tc.minSeverity)
		if len(filtered) != tc.wantCount {
			t.Errorf("filterBySeverity(%q): got %d, want %d", tc.minSeverity, len(filtered), tc.wantCount)
		}
	}
}

func TestDedup(t *testing.T) {
	suggestions := []Suggestion{
		{Type: SuggestExtractFunction, Target: "a.go:Foo"},
		{Type: SuggestExtractFunction, Target: "a.go:Foo"},  // duplicate
		{Type: SuggestSimplifyFunction, Target: "a.go:Foo"}, // same target, different type
		{Type: SuggestExtractFunction, Target: "b.go:Bar"},
	}

	result := dedup(suggestions)
	if len(result) != 3 {
		t.Errorf("expected 3 unique suggestions, got %d", len(result))
	}
}

func TestBuildSummary(t *testing.T) {
	suggestions := []Suggestion{
		{Type: SuggestExtractFunction, Severity: "high"},
		{Type: SuggestExtractFunction, Severity: "medium"},
		{Type: SuggestSimplifyFunction, Severity: "high"},
	}

	summary := buildSummary(suggestions)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.BySeverity["high"] != 2 {
		t.Errorf("expected 2 high severity, got %d", summary.BySeverity["high"])
	}
	if summary.BySeverity["medium"] != 1 {
		t.Errorf("expected 1 medium severity, got %d", summary.BySeverity["medium"])
	}
	if summary.ByType[string(SuggestExtractFunction)] != 2 {
		t.Errorf("expected 2 extract_function, got %d", summary.ByType[string(SuggestExtractFunction)])
	}
	if summary.ByType[string(SuggestSimplifyFunction)] != 1 {
		t.Errorf("expected 1 simplify_function, got %d", summary.ByType[string(SuggestSimplifyFunction)])
	}
}

func TestBuildSummary_Empty(t *testing.T) {
	summary := buildSummary(nil)
	if summary == nil {
		t.Fatal("expected non-nil summary even for nil input")
	}
	if len(summary.BySeverity) != 0 {
		t.Error("expected empty BySeverity map")
	}
}

func TestComplexitySeverity(t *testing.T) {
	tests := []struct {
		value    int
		expected string
	}{
		{5, "low"},
		{10, "low"},
		{11, "medium"},
		{20, "medium"},
		{21, "high"},
		{30, "high"},
		{31, "critical"},
	}

	for _, tc := range tests {
		got := complexitySeverity(tc.value)
		if got != tc.expected {
			t.Errorf("complexitySeverity(%d) = %q, want %q", tc.value, got, tc.expected)
		}
	}
}

func TestComplexityEffort(t *testing.T) {
	tests := []struct {
		lines    int
		expected string
	}{
		{50, "small"},
		{100, "small"},
		{101, "medium"},
		{200, "medium"},
		{201, "large"},
	}

	for _, tc := range tests {
		got := complexityEffort(tc.lines)
		if got != tc.expected {
			t.Errorf("complexityEffort(%d) = %q, want %q", tc.lines, got, tc.expected)
		}
	}
}

func TestCouplingSeverity(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{1, "low"},
		{2, "low"},
		{3, "medium"},
		{4, "medium"},
		{5, "high"},
		{10, "high"},
	}

	for _, tc := range tests {
		got := couplingSeverity(tc.count)
		if got != tc.expected {
			t.Errorf("couplingSeverity(%d) = %q, want %q", tc.count, got, tc.expected)
		}
	}
}

func TestDeadCodeSeverity(t *testing.T) {
	tests := []struct {
		confidence float64
		expected   string
	}{
		{0.5, "low"},
		{0.7, "medium"},
		{0.89, "medium"},
		{0.9, "high"},
		{1.0, "high"},
	}

	for _, tc := range tests {
		got := deadCodeSeverity(tc.confidence)
		if got != tc.expected {
			t.Errorf("deadCodeSeverity(%f) = %q, want %q", tc.confidence, got, tc.expected)
		}
	}
}

func TestHasTestFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a Go source file and its test
	createFile(t, tmpDir, "pkg/handler.go", "package pkg")
	createFile(t, tmpDir, "pkg/handler_test.go", "package pkg")

	// Create a JS file without a test
	createFile(t, tmpDir, "src/utils.js", "// utils")

	// Create a Python file with a test
	createFile(t, tmpDir, "lib/parser.py", "# parser")
	createFile(t, tmpDir, "lib/test_parser.py", "# test")

	tests := []struct {
		path     string
		expected bool
	}{
		{"pkg/handler.go", true},
		{"src/utils.js", false},
		{"lib/parser.py", true},
		{"nonexistent.go", false},
	}

	for _, tc := range tests {
		got := hasTestFile(tmpDir, tc.path)
		if got != tc.expected {
			t.Errorf("hasTestFile(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

func TestListSourceFiles(t *testing.T) {
	tmpDir := t.TempDir()

	createFile(t, tmpDir, "main.go", "package main")
	createFile(t, tmpDir, "lib/utils.py", "# utils")
	createFile(t, tmpDir, "data.json", "{}")
	createFile(t, tmpDir, "README.md", "# readme")

	a := &Analyzer{repoRoot: tmpDir}
	files := a.listSourceFiles("")

	hasGo := false
	hasPy := false
	hasJSON := false
	for _, f := range files {
		if f == "main.go" {
			hasGo = true
		}
		if f == "lib/utils.py" {
			hasPy = true
		}
		if f == "data.json" {
			hasJSON = true
		}
	}

	if !hasGo {
		t.Error("expected main.go in source files")
	}
	if !hasPy {
		t.Error("expected lib/utils.py in source files")
	}
	if hasJSON {
		t.Error("data.json should not be in source files")
	}
}

func TestListSourceFiles_WithScope(t *testing.T) {
	tmpDir := t.TempDir()

	createFile(t, tmpDir, "root.go", "package root")
	createFile(t, tmpDir, "sub/inner.go", "package sub")

	a := &Analyzer{repoRoot: tmpDir}
	files := a.listSourceFiles("sub")

	if len(files) != 1 {
		t.Fatalf("expected 1 file in sub scope, got %d", len(files))
	}
	if files[0] != "sub/inner.go" {
		t.Errorf("expected sub/inner.go, got %s", files[0])
	}
}

// createFile is a test helper that creates a file with content.
func createFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", relPath, err)
	}
}

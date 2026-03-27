package compliance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSourceFilesBasic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some source files.
	dirs := []string{
		"pkg",
		"internal/foo",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	sourceFiles := []string{
		"main.go",
		"pkg/handler.go",
		"pkg/utils.ts",
		"internal/foo/bar.py",
	}
	for _, f := range sourceFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("// code"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	// Create a non-source file that should be excluded.
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := findSourceFiles(tmpDir, "")
	if err != nil {
		t.Fatalf("findSourceFiles error: %v", err)
	}

	if len(files) != len(sourceFiles) {
		t.Errorf("expected %d source files, got %d: %v", len(sourceFiles), len(files), files)
	}

	// All returned paths should be relative.
	for _, f := range files {
		if filepath.IsAbs(f) {
			t.Errorf("expected relative path, got absolute: %s", f)
		}
	}
}

func TestFindSourceFilesScope(t *testing.T) {
	tmpDir := t.TempDir()

	dirs := []string{"pkg", "cmd"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	allFiles := []string{"pkg/a.go", "pkg/b.go", "cmd/main.go"}
	for _, f := range allFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("// code"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Scope to "pkg" should only return files under pkg/.
	files, err := findSourceFiles(tmpDir, "pkg")
	if err != nil {
		t.Fatalf("findSourceFiles error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files in scope=pkg, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if f != "pkg/a.go" && f != "pkg/b.go" {
			t.Errorf("unexpected file in pkg scope: %s", f)
		}
	}
}

func TestFindSourceFilesSkipsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be skipped.
	skipDirs := []string{"node_modules", "vendor", ".git", "dist", "build"}
	for _, d := range skipDirs {
		dir := filepath.Join(tmpDir, d)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Put a source file inside each — it should NOT be found.
		if err := os.WriteFile(filepath.Join(dir, "hidden.go"), []byte("// code"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// One legitimate file.
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("// code"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := findSourceFiles(tmpDir, "")
	if err != nil {
		t.Fatalf("findSourceFiles error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (skipped dirs), got %d: %v", len(files), files)
	}
}

func TestFindSourceFilesEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty directory should return empty, not error.
	files, err := findSourceFiles(tmpDir, "")
	if err != nil {
		t.Fatalf("findSourceFiles on empty dir should not error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestIsSourceExt(t *testing.T) {
	yes := []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".java", ".kt", ".rs", ".rb", ".c", ".cpp", ".h", ".hpp", ".cs", ".swift", ".dart", ".scala"}
	no := []string{".md", ".txt", ".json", ".yaml", ".toml", ".xml", ".html", ".css", ".sql", ".sh", ""}

	for _, ext := range yes {
		if !isSourceExt(ext) {
			t.Errorf("isSourceExt(%q) = false, want true", ext)
		}
	}
	for _, ext := range no {
		if isSourceExt(ext) {
			t.Errorf("isSourceExt(%q) = true, want false", ext)
		}
	}
}

func TestSeverityOrder(t *testing.T) {
	// error < warning < info < unknown
	if severityOrder("error") >= severityOrder("warning") {
		t.Error("error should sort before warning")
	}
	if severityOrder("warning") >= severityOrder("info") {
		t.Error("warning should sort before info")
	}
	if severityOrder("info") >= severityOrder("other") {
		t.Error("info should sort before unknown")
	}
}

func TestMatchesCheckFilter(t *testing.T) {
	tests := []struct {
		checkID     string
		frameworkID string
		filters     []string
		want        bool
	}{
		{"pii-in-logs", "gdpr", []string{"pii-in-logs"}, true},
		{"pii-in-logs", "gdpr", []string{"gdpr/pii-in-logs"}, true},
		{"pii-in-logs", "gdpr", []string{"other-check"}, false},
		{"pii-in-logs", "gdpr", []string{"iso27001/pii-in-logs"}, false},
	}

	for _, tt := range tests {
		got := matchesCheckFilter(tt.checkID, tt.frameworkID, tt.filters)
		if got != tt.want {
			t.Errorf("matchesCheckFilter(%q, %q, %v) = %v, want %v",
				tt.checkID, tt.frameworkID, tt.filters, got, tt.want)
		}
	}
}

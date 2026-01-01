package diff

import (
	"testing"
)

func TestParseGitDiff_Empty(t *testing.T) {
	parser := NewGitDiffParser()
	result, err := parser.Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestParseGitDiff_SingleFile(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 1234567..abcdefg 100644
--- a/foo.go
+++ b/foo.go
@@ -1,5 +1,6 @@
 package main

 func main() {
+    fmt.Println("hello")
     fmt.Println("world")
 }
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	file := result.Files[0]
	if file.OldPath != "foo.go" {
		t.Errorf("expected OldPath 'foo.go', got '%s'", file.OldPath)
	}
	if file.NewPath != "foo.go" {
		t.Errorf("expected NewPath 'foo.go', got '%s'", file.NewPath)
	}
	if file.IsNew {
		t.Error("expected IsNew to be false")
	}
	if file.Deleted {
		t.Error("expected Deleted to be false")
	}

	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}

	hunk := file.Hunks[0]
	if hunk.OldStart != 1 {
		t.Errorf("expected OldStart 1, got %d", hunk.OldStart)
	}
	if hunk.NewStart != 1 {
		t.Errorf("expected NewStart 1, got %d", hunk.NewStart)
	}
	if len(hunk.Added) != 1 {
		t.Errorf("expected 1 added line, got %d", len(hunk.Added))
	}
	if len(hunk.Removed) != 0 {
		t.Errorf("expected 0 removed lines, got %d", len(hunk.Removed))
	}
	// Added line should be line 4 in new file
	if len(hunk.Added) > 0 && hunk.Added[0] != 4 {
		t.Errorf("expected added line 4, got %d", hunk.Added[0])
	}
}

func TestParseGitDiff_NewFile(t *testing.T) {
	diff := `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func hello() {}
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	file := result.Files[0]
	if !file.IsNew {
		t.Error("expected IsNew to be true")
	}
	if file.NewPath != "new.go" {
		t.Errorf("expected NewPath 'new.go', got '%s'", file.NewPath)
	}
}

func TestParseGitDiff_DeletedFile(t *testing.T) {
	diff := `diff --git a/old.go b/old.go
deleted file mode 100644
index 1234567..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func goodbye() {}
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	file := result.Files[0]
	if !file.Deleted {
		t.Error("expected Deleted to be true")
	}
	if file.OldPath != "old.go" {
		t.Errorf("expected OldPath 'old.go', got '%s'", file.OldPath)
	}
}

func TestParseGitDiff_RenamedFile(t *testing.T) {
	diff := `diff --git a/old_name.go b/new_name.go
similarity index 95%
rename from old_name.go
rename to new_name.go
index 1234567..abcdefg 100644
--- a/old_name.go
+++ b/new_name.go
@@ -1,3 +1,3 @@
 package main

-func oldFunc() {}
+func newFunc() {}
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	file := result.Files[0]
	if !file.Renamed {
		t.Error("expected Renamed to be true")
	}
	if file.OldPath != "old_name.go" {
		t.Errorf("expected OldPath 'old_name.go', got '%s'", file.OldPath)
	}
	if file.NewPath != "new_name.go" {
		t.Errorf("expected NewPath 'new_name.go', got '%s'", file.NewPath)
	}
}

func TestParseGitDiff_MultipleFiles(t *testing.T) {
	diff := `diff --git a/file1.go b/file1.go
index 1234567..abcdefg 100644
--- a/file1.go
+++ b/file1.go
@@ -1,2 +1,3 @@
 package main
+// comment
 func a() {}
diff --git a/file2.go b/file2.go
index 1234567..abcdefg 100644
--- a/file2.go
+++ b/file2.go
@@ -1,2 +1,3 @@
 package main
+// another comment
 func b() {}
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}

	if result.Files[0].NewPath != "file1.go" {
		t.Errorf("expected first file 'file1.go', got '%s'", result.Files[0].NewPath)
	}
	if result.Files[1].NewPath != "file2.go" {
		t.Errorf("expected second file 'file2.go', got '%s'", result.Files[1].NewPath)
	}
}

func TestParseGitDiff_MultipleHunks(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
index 1234567..abcdefg 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package main
+// import added

 import "fmt"
@@ -10,3 +11,4 @@ func main() {
 func helper() {
     fmt.Println("help")
+    // more help
 }
`

	parser := NewGitDiffParser()
	result, err := parser.Parse(diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	file := result.Files[0]
	if len(file.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(file.Hunks))
	}

	// First hunk
	if file.Hunks[0].OldStart != 1 {
		t.Errorf("expected first hunk OldStart 1, got %d", file.Hunks[0].OldStart)
	}
	if file.Hunks[0].NewStart != 1 {
		t.Errorf("expected first hunk NewStart 1, got %d", file.Hunks[0].NewStart)
	}

	// Second hunk
	if file.Hunks[1].OldStart != 10 {
		t.Errorf("expected second hunk OldStart 10, got %d", file.Hunks[1].OldStart)
	}
	if file.Hunks[1].NewStart != 11 {
		t.Errorf("expected second hunk NewStart 11, got %d", file.Hunks[1].NewStart)
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"internal/foo/bar.go", true},
		{"vendor/github.com/pkg/foo.go", false},
		{"node_modules/package/index.js", false},
		{".git/config", false},
		{"go.sum", false},
		{"package-lock.json", false},
		{"foo.pb.go", false},
		{"generated_types.go", true}, // Only _generated.go is skipped
		{"types_generated.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsSourceFile(tt.path)
			if result != tt.expected {
				t.Errorf("IsSourceFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a/foo.go", "foo.go"},
		{"b/foo.go", "foo.go"},
		{"foo.go", "foo.go"},
		{"/dev/null", "/dev/null"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanPath(tt.input)
			if result != tt.expected {
				t.Errorf("cleanPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

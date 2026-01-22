package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInferredDirectories(t *testing.T) {
	// Create a temporary test directory structure
	tempDir, err := os.MkdirTemp("", "ckb-inferred-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test directory structure:
	// tempDir/
	//   src/
	//     components/
	//       index.ts
	//       Button.tsx
	//       Modal.tsx
	//     hooks/
	//       useAuth.ts
	//       useData.ts
	//   tests/
	//     test1.ts
	//     test2.ts

	dirs := []string{
		"src/components",
		"src/hooks",
		"tests",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tempDir, d), 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", d, err)
		}
	}

	// Create files
	files := map[string]string{
		"src/components/index.ts":   "export * from './Button';",
		"src/components/Button.tsx": "export const Button = () => {};",
		"src/components/Modal.tsx":  "export const Modal = () => {};",
		"src/hooks/useAuth.ts":      "export const useAuth = () => {};",
		"src/hooks/useData.ts":      "export const useData = () => {};",
		"src/hooks/useQuery.ts":     "export const useQuery = () => {};",
		"tests/test1.ts":            "test('works', () => {});",
		"tests/test2.ts":            "test('also works', () => {});",
		"tests/test3.ts":            "test('three works', () => {});",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, path), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	// Run detection
	opts := DefaultInferOptions()
	dirs2, err := DetectInferredDirectories(tempDir, opts)
	if err != nil {
		t.Fatalf("DetectInferredDirectories failed: %v", err)
	}

	// Should detect at least src/components (has index file + semantic name + files)
	foundComponents := false
	foundHooks := false
	foundTests := false

	for _, d := range dirs2 {
		t.Logf("Found directory: %s (score: %.1f, hasIndex: %v, fileCount: %d, isSemantic: %v)",
			d.Path, d.Score, d.HasIndexFile, d.FileCount, d.IsSemantic)
		switch d.Path {
		case "src/components":
			foundComponents = true
			if !d.HasIndexFile {
				t.Errorf("src/components should have HasIndexFile=true")
			}
			if !d.IsSemantic {
				t.Errorf("src/components should have IsSemantic=true")
			}
		case "src/hooks":
			foundHooks = true
			if !d.IsSemantic {
				t.Errorf("src/hooks should have IsSemantic=true")
			}
		case "tests":
			foundTests = true
			if !d.IsSemantic {
				t.Errorf("tests should have IsSemantic=true")
			}
		}
	}

	if !foundComponents {
		t.Errorf("Should have found src/components")
	}
	if !foundHooks {
		t.Errorf("Should have found src/hooks")
	}
	if !foundTests {
		t.Errorf("Should have found tests")
	}
}

func TestScoreDirectory(t *testing.T) {
	// Create a temporary test directory with various files
	tempDir, err := os.MkdirTemp("", "ckb-score-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testCases := []struct {
		name           string
		files          []string
		expectedIndex  bool
		expectedSemant bool
		minScore       float64
	}{
		{
			name:           "components with index",
			files:          []string{"index.ts", "Button.tsx", "Modal.tsx"},
			expectedIndex:  true,
			expectedSemant: true,
			minScore:       5.0, // index(3) + semantic(2)
		},
		{
			name:           "utils with multiple files",
			files:          []string{"format.ts", "parse.ts", "validate.ts"},
			expectedIndex:  false,
			expectedSemant: true,
			minScore:       4.0, // semantic(2) + 3+ files(2)
		},
		{
			name:          "random with few files",
			files:         []string{"foo.ts"},
			expectedIndex: false,
			minScore:      0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create subdirectory
			subDir := filepath.Join(tempDir, tc.name)
			if err := os.MkdirAll(subDir, 0755); err != nil {
				t.Fatalf("Failed to create dir: %v", err)
			}
			defer os.RemoveAll(subDir)

			// Create files
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(subDir, f), []byte("content"), 0644); err != nil {
					t.Fatalf("Failed to create file: %v", err)
				}
			}

			// Score the directory
			dir := scoreDirectory(subDir, tc.name, 1)

			if dir.HasIndexFile != tc.expectedIndex {
				t.Errorf("HasIndexFile: expected %v, got %v", tc.expectedIndex, dir.HasIndexFile)
			}

			if dir.Score < tc.minScore {
				t.Errorf("Score: expected >= %.1f, got %.1f", tc.minScore, dir.Score)
			}
		})
	}
}

func TestSemanticDirectoryNames(t *testing.T) {
	expectedSemantic := []string{
		"src", "lib", "components", "hooks", "utils", "services",
		"models", "controllers", "views", "pages", "routes",
		"tests", "internal", "pkg", "cmd",
	}

	for _, name := range expectedSemantic {
		if !SemanticDirectoryNames[name] {
			t.Errorf("Expected %q to be a semantic directory name", name)
		}
	}
}

func TestIndexFilePatterns(t *testing.T) {
	// Verify key index patterns exist
	if patterns, ok := IndexFilePatterns[LanguageTypeScript]; !ok || len(patterns) == 0 {
		t.Error("TypeScript should have index file patterns")
	}

	if patterns, ok := IndexFilePatterns[LanguagePython]; !ok || len(patterns) == 0 {
		t.Error("Python should have index file patterns (__init__.py)")
	}

	if patterns, ok := IndexFilePatterns[LanguageRust]; !ok || len(patterns) == 0 {
		t.Error("Rust should have index file patterns (mod.rs)")
	}
}

func TestIntermediateDirectories(t *testing.T) {
	// Create a deep nested structure where intermediate directories
	// don't have source files and may not meet scoring threshold
	//
	// web/
	//   src/
	//     myfeature/           <- non-semantic, no files (would be filtered without fix)
	//       components/
	//         common/
	//           Button.tsx
	//           Modal.tsx
	//           Card.tsx

	tempDir, err := os.MkdirTemp("", "ckb-intermediate-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create deep directory structure
	deepDir := "web/src/myfeature/components/common"
	if err := os.MkdirAll(filepath.Join(tempDir, deepDir), 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	// Only add files at the deepest level
	files := []string{
		"web/src/myfeature/components/common/Button.tsx",
		"web/src/myfeature/components/common/Modal.tsx",
		"web/src/myfeature/components/common/Card.tsx",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tempDir, f), []byte("export default {}"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", f, err)
		}
	}

	// Run detection
	opts := DefaultInferOptions()
	opts.MaxDepth = 6 // Allow deeper traversal
	dirs, err := DetectInferredDirectories(tempDir, opts)
	if err != nil {
		t.Fatalf("DetectInferredDirectories failed: %v", err)
	}

	// Build a map for easy lookup
	dirMap := make(map[string]*InferredDirectory)
	for _, d := range dirs {
		dirMap[d.Path] = d
		t.Logf("Found: %s (fileCount=%d, isIntermediate=%v, score=%.1f)",
			d.Path, d.FileCount, d.IsIntermediate, d.Score)
	}

	// Verify all intermediate directories are present
	expectedIntermediates := []string{
		"web",
		"web/src",
		"web/src/myfeature",
		"web/src/myfeature/components",
	}

	for _, path := range expectedIntermediates {
		dir, found := dirMap[path]
		if !found {
			t.Errorf("Missing intermediate directory: %s", path)
			continue
		}
		// These should have no direct files (or be marked intermediate)
		// Note: some may score high enough naturally (e.g., "src", "components" are semantic)
		if dir.FileCount > 0 {
			t.Errorf("%s should have fileCount=0, got %d", path, dir.FileCount)
		}
	}

	// Verify the leaf directory with files is present and NOT intermediate
	leafPath := "web/src/myfeature/components/common"
	leaf, found := dirMap[leafPath]
	if !found {
		t.Errorf("Missing leaf directory: %s", leafPath)
	} else {
		if leaf.FileCount != 3 {
			t.Errorf("%s should have fileCount=3, got %d", leafPath, leaf.FileCount)
		}
		if leaf.IsIntermediate {
			t.Errorf("%s should NOT be marked as intermediate", leafPath)
		}
	}

	// Specifically check that "myfeature" (non-semantic) is present
	// This is the key case - it has no files and isn't a semantic name
	myfeature, found := dirMap["web/src/myfeature"]
	if !found {
		t.Errorf("Missing non-semantic intermediate: web/src/myfeature")
	} else {
		if !myfeature.IsIntermediate {
			// It should be marked intermediate since it was added to complete hierarchy
			// (unless it scored high enough on its own, which it shouldn't)
			if myfeature.Score < 2.0 {
				t.Errorf("web/src/myfeature should be marked IsIntermediate=true")
			}
		}
	}
}

func TestFillIntermediateDirectories(t *testing.T) {
	// Unit test for fillIntermediateDirectories function directly
	input := []*InferredDirectory{
		{Path: "a/b/c/d", FileCount: 5, Score: 5.0},
		{Path: "x/y/z", FileCount: 3, Score: 4.0},
	}

	result := fillIntermediateDirectories(input)

	// Build map for lookup
	resultMap := make(map[string]*InferredDirectory)
	for _, d := range result {
		resultMap[d.Path] = d
	}

	// Check all intermediates are present
	expectedPaths := []string{
		"a", "a/b", "a/b/c", "a/b/c/d",
		"x", "x/y", "x/y/z",
	}

	for _, path := range expectedPaths {
		if _, found := resultMap[path]; !found {
			t.Errorf("Missing path: %s", path)
		}
	}

	// Check intermediate flags
	intermediates := []string{"a", "a/b", "a/b/c", "x", "x/y"}
	for _, path := range intermediates {
		if dir, found := resultMap[path]; found {
			if !dir.IsIntermediate {
				t.Errorf("%s should have IsIntermediate=true", path)
			}
		}
	}

	// Check non-intermediates
	nonIntermediates := []string{"a/b/c/d", "x/y/z"}
	for _, path := range nonIntermediates {
		if dir, found := resultMap[path]; found {
			if dir.IsIntermediate {
				t.Errorf("%s should have IsIntermediate=false", path)
			}
		}
	}
}

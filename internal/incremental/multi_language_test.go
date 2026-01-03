package incremental

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"ckb/internal/project"
	"ckb/internal/storage"
)

// TestFixturePaths verifies that test fixtures exist for all supported languages.
func TestFixturePaths(t *testing.T) {
	// Get the project root (relative to this test file)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	fixtures := []struct {
		lang     string
		manifest string
		sources  []string
	}{
		{
			lang:     "dart",
			manifest: "pubspec.yaml",
			sources:  []string{"lib/main.dart", "lib/utils.dart"},
		},
		{
			lang:     "typescript",
			manifest: "package.json",
			sources:  []string{"src/index.ts", "src/utils.ts"},
		},
		{
			lang:     "python",
			manifest: "pyproject.toml",
			sources:  []string{"src/main.py", "src/utils.py", "src/__init__.py"},
		},
	}

	for _, f := range fixtures {
		t.Run(f.lang, func(t *testing.T) {
			fixtureDir := filepath.Join(projectRoot, "testdata", "incremental", f.lang)

			// Check fixture directory exists
			if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
				t.Fatalf("fixture directory does not exist: %s", fixtureDir)
			}

			// Check manifest exists
			manifestPath := filepath.Join(fixtureDir, f.manifest)
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				t.Errorf("manifest file does not exist: %s", manifestPath)
			}

			// Check source files exist
			for _, src := range f.sources {
				srcPath := filepath.Join(fixtureDir, src)
				if _, err := os.Stat(srcPath); os.IsNotExist(err) {
					t.Errorf("source file does not exist: %s", srcPath)
				}
			}
		})
	}
}

// TestLanguageDetectionForFixtures verifies that fixtures are correctly detected.
func TestLanguageDetectionForFixtures(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	tests := []struct {
		lang     string
		expected project.Language
	}{
		{"dart", project.LangDart},
		{"typescript", project.LangTypeScript},
		{"python", project.LangPython},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			fixtureDir := filepath.Join(projectRoot, "testdata", "incremental", tt.lang)

			detected, _, _ := project.DetectAllLanguages(fixtureDir)
			if detected != tt.expected {
				t.Errorf("DetectLanguage() = %v, want %v", detected, tt.expected)
			}
		})
	}
}

// TestIncrementalIndexerCreation tests that we can create indexers for all fixture languages.
func TestIncrementalIndexerCreation(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	languages := []struct {
		name string
		lang project.Language
	}{
		{"dart", project.LangDart},
		{"typescript", project.LangTypeScript},
		{"python", project.LangPython},
	}

	for _, tt := range languages {
		t.Run(tt.name, func(t *testing.T) {
			fixtureDir := filepath.Join(projectRoot, "testdata", "incremental", tt.name)

			// Create temp .ckb directory
			ckbDir := filepath.Join(fixtureDir, ".ckb")
			if err := os.MkdirAll(ckbDir, 0755); err != nil {
				t.Fatalf("failed to create .ckb dir: %v", err)
			}
			defer os.RemoveAll(ckbDir)

			// Create logger and database
			logger := logging.NewLogger(logging.Config{
				Format: logging.HumanFormat,
				Level:  logging.ErrorLevel,
			})

			db, err := storage.Open(fixtureDir, logger)
			if err != nil {
				t.Fatalf("failed to open database: %v", err)
			}
			defer db.Close()

			// Create incremental indexer
			config := DefaultConfig()
			indexer := NewIncrementalIndexer(fixtureDir, db, config, logger)

			if indexer == nil {
				t.Fatal("NewIncrementalIndexer returned nil")
			}

			// Check CanUseIncremental - should work if indexer is installed
			canUse, reason := indexer.CanUseIncremental(tt.lang)
			t.Logf("CanUseIncremental(%s) = %v, reason: %s", tt.name, canUse, reason)

			// If can't use, reason should explain why (indexer not installed)
			if !canUse && reason == "" {
				t.Error("expected non-empty reason when CanUseIncremental returns false")
			}
		})
	}
}

// TestIndexerConfigForFixtureLanguages verifies indexer configs exist for fixture languages.
func TestIndexerConfigForFixtureLanguages(t *testing.T) {
	languages := []struct {
		lang                project.Language
		expectedCmd         string
		supportsIncremental bool
	}{
		{project.LangDart, "dart", true},
		{project.LangTypeScript, "scip-typescript", true},
		{project.LangPython, "scip-python", true},
	}

	for _, tt := range languages {
		t.Run(string(tt.lang), func(t *testing.T) {
			config := project.GetIndexerConfig(tt.lang)
			if config == nil {
				t.Fatalf("no indexer config for %s", tt.lang)
			}

			if config.Cmd != tt.expectedCmd {
				t.Errorf("Cmd = %q, want %q", config.Cmd, tt.expectedCmd)
			}

			if config.SupportsIncremental != tt.supportsIncremental {
				t.Errorf("SupportsIncremental = %v, want %v",
					config.SupportsIncremental, tt.supportsIncremental)
			}
		})
	}
}

// TestFixtureFilesHaveContent verifies fixture files are not empty.
func TestFixtureFilesHaveContent(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	files := []string{
		"testdata/incremental/dart/lib/main.dart",
		"testdata/incremental/dart/lib/utils.dart",
		"testdata/incremental/typescript/src/index.ts",
		"testdata/incremental/typescript/src/utils.ts",
		"testdata/incremental/python/src/main.py",
		"testdata/incremental/python/src/utils.py",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			fullPath := filepath.Join(projectRoot, file)
			info, err := os.Stat(fullPath)
			if err != nil {
				t.Fatalf("failed to stat file: %v", err)
			}

			if info.Size() == 0 {
				t.Error("file is empty")
			}

			// Check file has at least some expected content
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}

			// Each file should define at least one function
			if len(content) < 50 {
				t.Errorf("file content seems too short: %d bytes", len(content))
			}
		})
	}
}

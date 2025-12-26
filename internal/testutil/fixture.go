// Package testutil provides testing utilities for golden tests.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// FixtureContext holds information about a loaded fixture.
type FixtureContext struct {
	// Language is the fixture language (e.g., "go", "typescript")
	Language string

	// Root is the absolute path to the fixture directory
	Root string

	// SCIPPath is the path to the SCIP index file
	SCIPPath string

	// ExpectedDir is the path to the expected/ directory
	ExpectedDir string
}

// LoadFixture loads a language fixture, failing the test on error.
// The lang parameter should be one of: "go", "typescript", "python", "rust"
func LoadFixture(t *testing.T, lang string) *FixtureContext {
	t.Helper()

	root := getFixturesRoot(t)
	fixtureDir := filepath.Join(root, lang)

	// Verify fixture exists
	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Fatalf("Fixture directory not found: %s", fixtureDir)
	}

	scipPath := filepath.Join(fixtureDir, ".scip", "index.scip")
	if _, err := os.Stat(scipPath); os.IsNotExist(err) {
		t.Fatalf("SCIP index not found: %s\nRun: cd %s && scip-go --output=.scip/index.scip ./...", scipPath, fixtureDir)
	}

	expectedDir := filepath.Join(fixtureDir, "expected")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		if err := os.MkdirAll(expectedDir, 0o755); err != nil {
			t.Fatalf("Failed to create expected directory: %v", err)
		}
	}

	return &FixtureContext{
		Language:    lang,
		Root:        fixtureDir,
		SCIPPath:    scipPath,
		ExpectedDir: expectedDir,
	}
}

// ExpectedPath returns the path to a golden file within the fixture.
// The name should not include the .json extension.
func (f *FixtureContext) ExpectedPath(name string) string {
	return filepath.Join(f.ExpectedDir, name+".json")
}

// getFixturesRoot returns the absolute path to testdata/fixtures/.
func getFixturesRoot(t *testing.T) string {
	t.Helper()

	// Get the directory of this source file
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Failed to get caller information")
	}

	// Navigate from internal/testutil to project root
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	fixturesRoot := filepath.Join(projectRoot, "testdata", "fixtures")

	if _, err := os.Stat(fixturesRoot); os.IsNotExist(err) {
		t.Fatalf("Fixtures root not found: %s", fixturesRoot)
	}

	return fixturesRoot
}

// AvailableLanguages returns the list of available fixture languages.
func AvailableLanguages(t *testing.T) []string {
	t.Helper()

	root := getFixturesRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("Failed to read fixtures directory: %v", err)
	}

	var langs []string
	for _, entry := range entries {
		if entry.IsDir() && !isHiddenDir(entry.Name()) {
			// Check if it has a SCIP index
			scipPath := filepath.Join(root, entry.Name(), ".scip", "index.scip")
			if _, err := os.Stat(scipPath); err == nil {
				langs = append(langs, entry.Name())
			}
		}
	}

	return langs
}

func isHiddenDir(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

package testutil

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var (
	// updateGolden controls whether golden files should be updated.
	// Use: go test ./... -run TestGolden -update
	updateGolden = flag.Bool("update", false, "update golden files")

	// goldenLang filters which fixture languages to test.
	// Use: go test ./... -run TestGolden -goldenLang=go,ts
	goldenLang = flag.String("goldenLang", "", "filter languages (comma-separated: go,ts,python)")
)

// ShouldUpdate returns true if golden files should be updated.
func ShouldUpdate() bool {
	return *updateGolden
}

// ShouldTestLang returns true if the given language should be tested.
func ShouldTestLang(lang string) bool {
	if *goldenLang == "" {
		return true
	}

	langs := strings.Split(*goldenLang, ",")
	for _, l := range langs {
		l = strings.TrimSpace(l)
		// Support both "go" and "golang", "ts" and "typescript"
		if l == lang || l == shortLang(lang) || longLang(l) == lang {
			return true
		}
	}
	return false
}

func shortLang(lang string) string {
	switch lang {
	case "typescript":
		return "ts"
	case "python":
		return "py"
	case "golang":
		return "go"
	default:
		return lang
	}
}

func longLang(short string) string {
	switch short {
	case "ts":
		return "typescript"
	case "py":
		return "python"
	default:
		return short
	}
}

// CompareGolden compares got against the golden file, failing with a diff on mismatch.
// Automatically normalizes the data before comparison.
// If -update flag is set, updates the golden file instead of comparing.
func CompareGolden(t *testing.T, fixture *FixtureContext, name string, got any) {
	t.Helper()

	normalized := MarshalNormalized(t, fixture, got)
	goldenPath := fixture.ExpectedPath(name)

	if *updateGolden {
		UpdateGolden(t, fixture, name, normalized)
		t.Logf("Updated golden: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Golden file missing: %s\n\nGot:\n%s\n\nRun with -update to create:\n  go test ./... -run %s -update",
				goldenPath, string(normalized), t.Name())
		}
		t.Fatalf("Failed to read golden file: %v", err)
	}

	if !bytes.Equal(normalized, expected) {
		diff := unifiedDiff(string(expected), string(normalized), goldenPath)
		t.Fatalf("Golden mismatch for %s:\n%s\n\nRun with -update to refresh:\n  go test ./... -run %s -update",
			name, diff, t.Name())
	}
}

// UpdateGolden writes normalized data to the golden file.
// Creates parent directories if they don't exist.
func UpdateGolden(t *testing.T, fixture *FixtureContext, name string, data []byte) {
	t.Helper()

	goldenPath := fixture.ExpectedPath(name)

	// Create parent directories
	if err := os.MkdirAll(fixture.ExpectedDir, 0o755); err != nil {
		t.Fatalf("Failed to create expected directory: %v", err)
	}

	if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write golden file: %v", err)
	}
}

// unifiedDiff produces a simple unified diff between two strings.
// This is a simplified implementation - for production use, consider
// using a proper diff library.
func unifiedDiff(expected, got, path string) string {
	var buf bytes.Buffer

	expectedLines := strings.Split(expected, "\n")
	gotLines := strings.Split(got, "\n")

	fmt.Fprintf(&buf, "--- %s (expected)\n", path)
	fmt.Fprintf(&buf, "+++ %s (got)\n", path)

	// Simple line-by-line diff
	maxLines := len(expectedLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}

	inHunk := false
	hunkStart := 0
	var hunkLines []string

	flushHunk := func() {
		if len(hunkLines) > 0 {
			fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", hunkStart+1, len(hunkLines), hunkStart+1, len(hunkLines))
			for _, line := range hunkLines {
				buf.WriteString(line)
				buf.WriteString("\n")
			}
			hunkLines = nil
		}
	}

	for i := 0; i < maxLines; i++ {
		var expLine, gotLine string
		if i < len(expectedLines) {
			expLine = expectedLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}

		if expLine == gotLine {
			if inHunk {
				// Context line in hunk
				hunkLines = append(hunkLines, " "+expLine)
				if len(hunkLines) > 6 {
					flushHunk()
					inHunk = false
				}
			}
		} else {
			if !inHunk {
				inHunk = true
				hunkStart = i
				// Add context before
				for j := max(0, i-3); j < i; j++ {
					if j < len(expectedLines) {
						hunkLines = append(hunkLines, " "+expectedLines[j])
					}
				}
			}

			if i < len(expectedLines) && expLine != "" {
				hunkLines = append(hunkLines, "-"+expLine)
			}
			if i < len(gotLines) && gotLine != "" {
				hunkLines = append(hunkLines, "+"+gotLine)
			}
		}
	}

	flushHunk()

	return buf.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// AssertGoldenSlice is a helper for testing slices with golden comparison.
// It normalizes and compares a slice result.
func AssertGoldenSlice(t *testing.T, fixture *FixtureContext, name string, got any) {
	t.Helper()

	// Convert to generic slice for normalization
	slice := SliceToMaps(t, got)
	CompareGolden(t, fixture, name, slice)
}

// AssertGoldenStruct is a helper for testing structs with golden comparison.
// It normalizes and compares a struct result.
func AssertGoldenStruct(t *testing.T, fixture *FixtureContext, name string, got any) {
	t.Helper()

	// Convert to generic map for normalization
	m := StructToMap(t, got)
	CompareGolden(t, fixture, name, m)
}

// ForEachLanguage runs a test function for each available fixture language.
// Respects the -goldenLang flag and -short flag.
func ForEachLanguage(t *testing.T, fn func(t *testing.T, fixture *FixtureContext)) {
	t.Helper()

	langs := AvailableLanguages(t)
	if len(langs) == 0 {
		t.Skip("No fixtures available")
	}

	// In short mode, only test the first language (usually Go)
	if testing.Short() && len(langs) > 1 {
		langs = langs[:1]
	}

	for _, lang := range langs {
		if !ShouldTestLang(lang) {
			continue
		}

		t.Run(lang, func(t *testing.T) {
			fixture := LoadFixture(t, lang)
			fn(t, fixture)
		})
	}
}

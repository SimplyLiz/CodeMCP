package incremental

import (
	"testing"
)

func TestParseGitDiffNUL(t *testing.T) {
	// Create a detector with config that includes test files
	d := &ChangeDetector{
		config: &Config{
			IndexTests: true, // Include _test.go files for testing
		},
	}

	tests := []struct {
		name     string
		input    []byte
		expected []ChangedFile
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: nil,
		},
		{
			name:  "single added file",
			input: []byte("A\x00main.go\x00"),
			expected: []ChangedFile{
				{Path: "main.go", ChangeType: ChangeAdded},
			},
		},
		{
			name:  "single modified file",
			input: []byte("M\x00internal/foo.go\x00"),
			expected: []ChangedFile{
				{Path: "internal/foo.go", ChangeType: ChangeModified},
			},
		},
		{
			name:  "single deleted file",
			input: []byte("D\x00old.go\x00"),
			expected: []ChangedFile{
				{Path: "old.go", ChangeType: ChangeDeleted},
			},
		},
		{
			name:  "rename go to go",
			input: []byte("R100\x00old.go\x00new.go\x00"),
			expected: []ChangedFile{
				{Path: "new.go", OldPath: "old.go", ChangeType: ChangeRenamed},
			},
		},
		{
			name:  "rename go to non-go (treated as delete)",
			input: []byte("R100\x00main.go\x00main.txt\x00"),
			expected: []ChangedFile{
				{Path: "main.go", ChangeType: ChangeDeleted},
			},
		},
		{
			name:  "rename non-go to go (treated as add)",
			input: []byte("R100\x00main.txt\x00main.go\x00"),
			expected: []ChangedFile{
				{Path: "main.go", ChangeType: ChangeAdded},
			},
		},
		{
			name:     "rename non-go to non-go (ignored)",
			input:    []byte("R100\x00old.txt\x00new.txt\x00"),
			expected: nil,
		},
		{
			name:  "copy creates new file",
			input: []byte("C100\x00original.go\x00copy.go\x00"),
			expected: []ChangedFile{
				{Path: "copy.go", ChangeType: ChangeAdded},
			},
		},
		{
			name:  "multiple changes",
			input: []byte("A\x00new.go\x00M\x00modified.go\x00D\x00deleted.go\x00"),
			expected: []ChangedFile{
				{Path: "new.go", ChangeType: ChangeAdded},
				{Path: "modified.go", ChangeType: ChangeModified},
				{Path: "deleted.go", ChangeType: ChangeDeleted},
			},
		},
		{
			name:  "path with spaces",
			input: []byte("A\x00path with spaces/file.go\x00"),
			expected: []ChangedFile{
				{Path: "path with spaces/file.go", ChangeType: ChangeAdded},
			},
		},
		{
			name:     "non-go file ignored",
			input:    []byte("A\x00readme.md\x00"),
			expected: nil,
		},
		{
			name:  "mixed go and non-go",
			input: []byte("A\x00main.go\x00M\x00readme.md\x00D\x00old.go\x00"),
			expected: []ChangedFile{
				{Path: "main.go", ChangeType: ChangeAdded},
				{Path: "old.go", ChangeType: ChangeDeleted},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := d.parseGitDiffNUL(tc.input)

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d changes, got %d", len(tc.expected), len(result))
				return
			}

			for i, exp := range tc.expected {
				got := result[i]
				if got.Path != exp.Path {
					t.Errorf("change %d: expected path %q, got %q", i, exp.Path, got.Path)
				}
				if got.OldPath != exp.OldPath {
					t.Errorf("change %d: expected oldPath %q, got %q", i, exp.OldPath, got.OldPath)
				}
				if got.ChangeType != exp.ChangeType {
					t.Errorf("change %d: expected type %q, got %q", i, exp.ChangeType, got.ChangeType)
				}
			}
		})
	}
}

func TestDeduplicateChanges(t *testing.T) {
	d := &ChangeDetector{}

	tests := []struct {
		name     string
		input    []ChangedFile
		expected []ChangedFile
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name: "no duplicates",
			input: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeAdded},
				{Path: "b.go", ChangeType: ChangeModified},
			},
			expected: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeAdded},
				{Path: "b.go", ChangeType: ChangeModified},
			},
		},
		{
			name: "duplicate keeps last",
			input: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeAdded},
				{Path: "a.go", ChangeType: ChangeModified},
			},
			expected: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeModified},
			},
		},
		{
			name: "multiple duplicates",
			input: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeAdded},
				{Path: "b.go", ChangeType: ChangeAdded},
				{Path: "a.go", ChangeType: ChangeModified},
				{Path: "b.go", ChangeType: ChangeDeleted},
			},
			expected: []ChangedFile{
				{Path: "a.go", ChangeType: ChangeModified},
				{Path: "b.go", ChangeType: ChangeDeleted},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := d.deduplicateChanges(tc.input)

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d changes, got %d", len(tc.expected), len(result))
				return
			}

			for i, exp := range tc.expected {
				got := result[i]
				if got.Path != exp.Path || got.ChangeType != exp.ChangeType {
					t.Errorf("change %d: expected {%s, %s}, got {%s, %s}",
						i, exp.Path, exp.ChangeType, got.Path, got.ChangeType)
				}
			}
		})
	}
}

func TestIsGoFile(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		indexTests bool
		excludes   []string
		expected   bool
	}{
		{"go file", "main.go", false, nil, true},
		{"non-go file", "main.txt", false, nil, false},
		{"test file with tests disabled", "main_test.go", false, nil, false},
		{"test file with tests enabled", "main_test.go", true, nil, true},
		{"excluded pattern", "vendor/foo.go", false, []string{"vendor"}, false},
		{"nested path", "internal/pkg/file.go", false, nil, true},
		{"excluded nested", "vendor/pkg/file.go", false, []string{"vendor"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &ChangeDetector{
				config: &Config{
					IndexTests: tc.indexTests,
					Excludes:   tc.excludes,
				},
			}
			result := d.isGoFile(tc.path)
			if result != tc.expected {
				t.Errorf("isGoFile(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		excludes []string
		expected bool
	}{
		{"no excludes", "main.go", nil, false},
		{"exact match", "vendor", []string{"vendor"}, true},
		{"directory prefix", "vendor/foo/bar.go", []string{"vendor"}, true},
		{"glob match", "test_data.go", []string{"test_*.go"}, true},
		{"no match", "main.go", []string{"vendor", "testdata"}, false},
		{"multiple excludes first matches", "vendor/x.go", []string{"vendor", "testdata"}, true},
		{"multiple excludes second matches", "testdata/x.go", []string{"vendor", "testdata"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &ChangeDetector{
				config: &Config{
					Excludes: tc.excludes,
				},
			}
			result := d.isExcluded(tc.path)
			if result != tc.expected {
				t.Errorf("isExcluded(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

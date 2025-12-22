package docs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBacktickPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		// Should match
		{"`UserService.Authenticate`", []string{"`UserService.Authenticate`"}},
		{"`pkg.Symbol`", []string{"`pkg.Symbol`"}},
		{"`internal/auth.Handler`", []string{"`internal/auth.Handler`"}},
		{"`crate::module::Type`", []string{"`crate::module::Type`"}},
		{"`Class#method`", []string{"`Class#method`"}},
		{"See `Foo.Bar` and `Baz.Qux`", []string{"`Foo.Bar`", "`Baz.Qux`"}},
		{"`a.b.c.d`", []string{"`a.b.c.d`"}},

		// Should not match (single segment)
		{"`Symbol`", nil},
		{"`foo`", nil},
		{"`123`", nil},

		// Should not match (invalid)
		{"Symbol.Name", nil}, // No backticks
		{"``", nil},          // Empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := backtickPattern.FindAllString(tt.input, -1)
			if len(matches) != len(tt.expected) {
				t.Errorf("expected %d matches, got %d: %v", len(tt.expected), len(matches), matches)
				return
			}
			for i, m := range matches {
				if m != tt.expected[i] {
					t.Errorf("match %d: expected %q, got %q", i, tt.expected[i], m)
				}
			}
		})
	}
}

func TestSymbolDirectivePattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"<!-- ckb:symbol UserService.Authenticate -->", []string{"UserService.Authenticate"}},
		{"<!-- ckb:symbol internal/auth.Handler -->", []string{"internal/auth.Handler"}},
		{"<!--ckb:symbol NoSpaces-->", []string{"NoSpaces"}},
		{"text <!-- ckb:symbol Foo --> more", []string{"Foo"}},

		// Should not match
		{"<!-- ckb:module foo -->", nil},
		{"<!-- other -->", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := symbolDirectivePattern.FindAllStringSubmatch(tt.input, -1)
			if len(matches) != len(tt.expected) {
				t.Errorf("expected %d matches, got %d", len(tt.expected), len(matches))
				return
			}
			for i, m := range matches {
				if m[1] != tt.expected[i] {
					t.Errorf("match %d: expected %q, got %q", i, tt.expected[i], m[1])
				}
			}
		})
	}
}

func TestModuleDirectivePattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"<!-- ckb:module internal/auth -->", []string{"internal/auth"}},
		{"<!-- ckb:module api -->", []string{"api"}},
		{"<!--ckb:module foo/bar-->", []string{"foo/bar"}},

		// Should not match
		{"<!-- ckb:symbol Foo -->", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := moduleDirectivePattern.FindAllStringSubmatch(tt.input, -1)
			if len(matches) != len(tt.expected) {
				t.Errorf("expected %d matches, got %d", len(tt.expected), len(matches))
				return
			}
			for i, m := range matches {
				if m[1] != tt.expected[i] {
					t.Errorf("match %d: expected %q, got %q", i, tt.expected[i], m[1])
				}
			}
		})
	}
}

func TestFenceTracking(t *testing.T) {
	// Create a temp file with fenced content
	content := `# Test Doc

` + "```" + `go
// Code inside fence
func Foo.Bar() {}
` + "```" + `

Outside fence: ` + "`Baz.Qux`" + `
`

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(dir)
	result := scanner.ScanFile(path)

	if result.Error != nil {
		t.Fatalf("scan error: %v", result.Error)
	}

	// Should find backticks both inside and outside fence (v1 behavior)
	// The fence itself contains no valid backtick patterns
	if len(result.Mentions) != 1 {
		t.Errorf("expected 1 mention, got %d: %v", len(result.Mentions), result.Mentions)
	}

	if len(result.Mentions) > 0 && result.Mentions[0].RawText != "`Baz.Qux`" {
		t.Errorf("expected mention to be `Baz.Qux`, got %q", result.Mentions[0].RawText)
	}
}

func TestMixedFenceDelimiters(t *testing.T) {
	// Test that ~~~ fence is not closed by ```
	content := `# Test

~~~python
code here
` + "```" + `
still in fence
~~~

Outside: ` + "`Foo.Bar`" + `
`

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(dir)
	result := scanner.ScanFile(path)

	if result.Error != nil {
		t.Fatalf("scan error: %v", result.Error)
	}

	if len(result.Mentions) != 1 {
		t.Errorf("expected 1 mention, got %d", len(result.Mentions))
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"`UserService.Authenticate`", "UserService.Authenticate"},
		{"UserService.Authenticate", "UserService.Authenticate"},
		{"crate::module::Type", "crate.module.Type"},
		{"Class#method", "Class.method"},
		{"package/subpkg.Func", "package.subpkg.Func"},
		{".leading.dots.", "leading.dots"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Normalize(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCountSegments(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"Symbol", 1},
		{"Foo.Bar", 2},
		{"a.b.c", 3},
		{"internal.auth.UserService.Authenticate", 4},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CountSegments(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestDetectDocType(t *testing.T) {
	scanner := NewScanner("/repo")

	tests := []struct {
		path     string
		expected DocType
	}{
		{"/repo/docs/guide.md", DocTypeMarkdown},
		{"/repo/README.md", DocTypeMarkdown},
		{"/repo/adr/ADR-001.md", DocTypeADR},
		{"/repo/decisions/0001-use-go.md", DocTypeADR},
		{"/repo/docs/adr-template.md", DocTypeADR},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := scanner.detectDocType(tt.path)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestScanFile(t *testing.T) {
	content := `# Authentication Module

<!-- ckb:module internal/auth -->

This module handles user authentication.

## Usage

Use ` + "`UserService.Authenticate`" + ` to validate credentials.
See also ` + "`Session.Create`" + ` for session management.

<!-- ckb:symbol TokenValidator.Validate -->

## Internal

The ` + "`internal.helper.Hash`" + ` function is used internally.
`

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(dir)
	result := scanner.ScanFile(path)

	if result.Error != nil {
		t.Fatalf("scan error: %v", result.Error)
	}

	// Check document
	if result.Doc.Title != "Authentication Module" {
		t.Errorf("expected title 'Authentication Module', got %q", result.Doc.Title)
	}
	if result.Doc.Type != DocTypeMarkdown {
		t.Errorf("expected type markdown, got %s", result.Doc.Type)
	}
	if result.Doc.Hash == "" {
		t.Error("expected non-empty hash")
	}

	// Check mentions
	expectedMentions := 4 // 3 backticks + 1 directive
	if len(result.Mentions) != expectedMentions {
		t.Errorf("expected %d mentions, got %d: %v", expectedMentions, len(result.Mentions), result.Mentions)
	}

	// Check modules
	if len(result.Modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(result.Modules))
	}
	if len(result.Modules) > 0 && result.Modules[0].ModuleID != "internal/auth" {
		t.Errorf("expected module 'internal/auth', got %q", result.Modules[0].ModuleID)
	}
}

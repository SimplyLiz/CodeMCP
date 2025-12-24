package lsp

import (
	"testing"
)

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"file:///path/to/file.go:10:5", "file.go"},
		{"file:///src/main.ts:1:1", "main.ts"},
		{"file:///Users/test/project/module/handler.py:50:10", "handler.py"},
		{"file://relative/path.js:5:2", "path.js"},
		{"not-a-file-uri", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := extractSymbolName(tt.id)
			if got != tt.want {
				t.Errorf("extractSymbolName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestParseLocationFromMap(t *testing.T) {
	t.Run("valid location", func(t *testing.T) {
		locMap := map[string]interface{}{
			"uri": "file:///path/to/file.go",
			"range": map[string]interface{}{
				"start": map[string]interface{}{
					"line":      float64(9),  // 0-indexed
					"character": float64(4),
				},
				"end": map[string]interface{}{
					"line":      float64(9),
					"character": float64(14),
				},
			},
		}

		loc, err := parseLocationFromMap(locMap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// CKB uses 1-indexed lines/columns
		if loc.Path != "/path/to/file.go" {
			t.Errorf("Path = %q, want /path/to/file.go", loc.Path)
		}
		if loc.Line != 10 {
			t.Errorf("Line = %d, want 10", loc.Line)
		}
		if loc.Column != 5 {
			t.Errorf("Column = %d, want 5", loc.Column)
		}
		if loc.EndLine != 10 {
			t.Errorf("EndLine = %d, want 10", loc.EndLine)
		}
		if loc.EndColumn != 15 {
			t.Errorf("EndColumn = %d, want 15", loc.EndColumn)
		}
	})

	t.Run("missing range", func(t *testing.T) {
		locMap := map[string]interface{}{
			"uri": "file:///path/to/file.go",
		}

		_, err := parseLocationFromMap(locMap)
		if err == nil {
			t.Error("expected error for missing range")
		}
	})

	t.Run("missing start/end", func(t *testing.T) {
		locMap := map[string]interface{}{
			"uri":   "file:///path/to/file.go",
			"range": map[string]interface{}{},
		}

		_, err := parseLocationFromMap(locMap)
		if err == nil {
			t.Error("expected error for missing start/end")
		}
	})
}

func TestParseLocation(t *testing.T) {
	t.Run("single location map", func(t *testing.T) {
		result := map[string]interface{}{
			"uri": "file:///path/to/file.go",
			"range": map[string]interface{}{
				"start": map[string]interface{}{
					"line":      float64(0),
					"character": float64(0),
				},
				"end": map[string]interface{}{
					"line":      float64(0),
					"character": float64(10),
				},
			},
		}

		loc, err := parseLocation(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc == nil {
			t.Fatal("expected non-nil location")
		}
		if loc.Line != 1 {
			t.Errorf("Line = %d, want 1", loc.Line)
		}
	})

	t.Run("location array", func(t *testing.T) {
		result := []interface{}{
			map[string]interface{}{
				"uri": "file:///first.go",
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      float64(5),
						"character": float64(0),
					},
					"end": map[string]interface{}{
						"line":      float64(5),
						"character": float64(10),
					},
				},
			},
		}

		loc, err := parseLocation(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc.Line != 6 {
			t.Errorf("Line = %d, want 6", loc.Line)
		}
	})

	t.Run("empty array returns error", func(t *testing.T) {
		result := []interface{}{}

		_, err := parseLocation(result)
		if err == nil {
			t.Error("expected error for empty array")
		}
	})

	t.Run("unexpected format returns error", func(t *testing.T) {
		result := "not a location"

		_, err := parseLocation(result)
		if err == nil {
			t.Error("expected error for unexpected format")
		}
	})
}

func TestExtractDocumentation(t *testing.T) {
	t.Run("markdown content", func(t *testing.T) {
		result := map[string]interface{}{
			"contents": map[string]interface{}{
				"kind":  "markdown",
				"value": "Documentation here",
			},
		}

		doc := extractDocumentation(result)
		if doc != "Documentation here" {
			t.Errorf("extractDocumentation() = %q, want 'Documentation here'", doc)
		}
	})

	t.Run("string content", func(t *testing.T) {
		result := map[string]interface{}{
			"contents": "Plain text docs",
		}

		doc := extractDocumentation(result)
		if doc != "Plain text docs" {
			t.Errorf("extractDocumentation() = %q, want 'Plain text docs'", doc)
		}
	})

	t.Run("missing contents returns empty", func(t *testing.T) {
		result := map[string]interface{}{}

		doc := extractDocumentation(result)
		if doc != "" {
			t.Errorf("extractDocumentation() = %q, want empty", doc)
		}
	})

	t.Run("nil returns empty", func(t *testing.T) {
		doc := extractDocumentation(nil)
		if doc != "" {
			t.Errorf("extractDocumentation(nil) = %q, want empty", doc)
		}
	})
}

func TestSymbolKindToString(t *testing.T) {
	tests := []struct {
		kind int
		want string
	}{
		{1, "file"},
		{2, "module"},
		{3, "namespace"},
		{4, "package"},
		{5, "class"},
		{6, "method"},
		{7, "property"},
		{8, "field"},
		{9, "constructor"},
		{10, "enum"},
		{11, "interface"},
		{12, "function"},
		{13, "variable"},
		{14, "constant"},
		{15, "string"},
		{16, "number"},
		{17, "boolean"},
		{18, "array"},
		// Unknown kinds return "symbol" as default
		{19, "symbol"},
		{0, "symbol"},
		{99, "symbol"},
		{-1, "symbol"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := symbolKindToString(tt.kind)
			if got != tt.want {
				t.Errorf("symbolKindToString(%d) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestParseReferences(t *testing.T) {
	t.Run("valid references", func(t *testing.T) {
		result := []interface{}{
			map[string]interface{}{
				"uri": "file:///ref1.go",
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      float64(10),
						"character": float64(5),
					},
					"end": map[string]interface{}{
						"line":      float64(10),
						"character": float64(15),
					},
				},
			},
			map[string]interface{}{
				"uri": "file:///ref2.go",
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      float64(20),
						"character": float64(0),
					},
					"end": map[string]interface{}{
						"line":      float64(20),
						"character": float64(10),
					},
				},
			},
		}

		refs, err := parseReferences(result, "/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 2 {
			t.Errorf("got %d refs, want 2", len(refs))
		}
		if refs[0].Kind != "reference" {
			t.Errorf("Kind = %q, want reference", refs[0].Kind)
		}
	})

	t.Run("skips invalid entries", func(t *testing.T) {
		result := []interface{}{
			"not a map",
			map[string]interface{}{
				"uri": "file:///valid.go",
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      float64(0),
						"character": float64(0),
					},
					"end": map[string]interface{}{
						"line":      float64(0),
						"character": float64(5),
					},
				},
			},
		}

		refs, err := parseReferences(result, "/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 1 {
			t.Errorf("got %d refs, want 1 (should skip invalid)", len(refs))
		}
	})

	t.Run("non-array returns error", func(t *testing.T) {
		result := "not an array"

		_, err := parseReferences(result, "/repo")
		if err == nil {
			t.Error("expected error for non-array input")
		}
	})
}

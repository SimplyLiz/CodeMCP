package query

import (
	"testing"
)

func TestInferLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"src/app.ts", "typescript"},
		{"src/app.tsx", "typescript"},
		{"lib/index.js", "javascript"},
		{"lib/index.jsx", "javascript"},
		{"script.py", "python"},
		{"lib.rs", "rust"},
		{"Main.java", "java"},
		{"Main.kt", "kotlin"},
		{"unknown.xyz", ""},
		{"noext", ""},
	}

	for _, tc := range tests {
		got := inferLanguage(tc.path)
		if got != tc.expected {
			t.Errorf("inferLanguage(%q) = %q, want %q", tc.path, got, tc.expected)
		}
	}
}

func TestGenerateGoSignature(t *testing.T) {
	tests := []struct {
		name    string
		params  []ExtractParameter
		returns []ExtractReturn
		want    string
	}{
		{
			name: "no params no returns",
			want: "func extracted()",
		},
		{
			name:   "params with types",
			params: []ExtractParameter{{Name: "x", Type: "int"}, {Name: "y", Type: "string"}},
			want:   "func extracted(x int, y string)",
		},
		{
			name:    "single return",
			returns: []ExtractReturn{{Name: "err", Type: "error"}},
			want:    "func extracted() error",
		},
		{
			name:    "multiple returns",
			returns: []ExtractReturn{{Name: "result", Type: "int"}, {Name: "err", Type: "error"}},
			want:    "func extracted() (int, error)",
		},
		{
			name:    "params and returns",
			params:  []ExtractParameter{{Name: "ctx", Type: "context.Context"}},
			returns: []ExtractReturn{{Name: "err", Type: "error"}},
			want:    "func extracted(ctx context.Context) error",
		},
		{
			name:   "params without types",
			params: []ExtractParameter{{Name: "a"}, {Name: "b"}},
			want:   "func extracted(a, b)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := generateGoSignature(tc.params, tc.returns)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateJSSignature(t *testing.T) {
	params := []ExtractParameter{{Name: "data"}, {Name: "options"}}
	returns := []ExtractReturn{{Name: "result"}}

	got := generateJSSignature(params, returns)
	expected := "function extracted(data, options)"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestGeneratePySignature(t *testing.T) {
	tests := []struct {
		name    string
		params  []ExtractParameter
		returns []ExtractReturn
		want    string
	}{
		{
			name: "no params",
			want: "def extracted()",
		},
		{
			name:   "typed params",
			params: []ExtractParameter{{Name: "x", Type: "int"}, {Name: "y", Type: "str"}},
			want:   "def extracted(x: int, y: str)",
		},
		{
			name:    "with returns",
			params:  []ExtractParameter{{Name: "data"}},
			returns: []ExtractReturn{{Name: "result"}, {Name: "count"}},
			want:    "def extracted(data) -> (result, count)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := generatePySignature(tc.params, tc.returns)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenerateSignature_LanguageDispatch(t *testing.T) {
	params := []ExtractParameter{{Name: "x", Type: "int"}}
	returns := []ExtractReturn{{Name: "err", Type: "error"}}

	goSig := generateSignature("go", params, returns)
	if goSig != "func extracted(x int) error" {
		t.Errorf("Go signature: %q", goSig)
	}

	jsSig := generateSignature("javascript", params, returns)
	if jsSig != "function extracted(x)" {
		t.Errorf("JS signature: %q", jsSig)
	}

	pySig := generateSignature("python", params, returns)
	if pySig != "def extracted(x: int) -> (err)" {
		t.Errorf("Python signature: %q", pySig)
	}

	// Unknown language falls back to Go
	unknownSig := generateSignature("ruby", params, returns)
	if unknownSig != goSig {
		t.Errorf("unknown language should fall back to Go, got %q", unknownSig)
	}
}

func TestGetPrepareExtractDetail_NilTarget(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	detail := engine.getPrepareExtractDetail(nil)
	if detail != nil {
		t.Error("expected nil for nil target")
	}
}

func TestGetPrepareExtractDetail_EmptyPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	detail := engine.getPrepareExtractDetail(&PrepareChangeTarget{Path: ""})
	if detail != nil {
		t.Error("expected nil for empty path")
	}
}

func TestGetPrepareExtractDetail_WithFile(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "handler.go", `package main

func handler() {
	x := 1
	y := x + 2
	println(y)
}
`)

	target := &PrepareChangeTarget{Path: "handler.go"}
	detail := engine.getPrepareExtractDetail(target)

	if detail == nil {
		t.Fatal("expected non-nil ExtractDetail")
	}
	if detail.BoundaryAnalysis == nil {
		t.Fatal("expected non-nil BoundaryAnalysis")
	}
	if detail.BoundaryAnalysis.Language != "go" {
		t.Errorf("expected language go, got %s", detail.BoundaryAnalysis.Language)
	}
	if detail.BoundaryAnalysis.Lines <= 0 {
		t.Error("expected positive line count")
	}
}

func TestGetPrepareExtractDetail_NonexistentFile(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	target := &PrepareChangeTarget{Path: "nonexistent.go"}
	detail := engine.getPrepareExtractDetail(target)
	if detail != nil {
		t.Error("expected nil for nonexistent file")
	}
}

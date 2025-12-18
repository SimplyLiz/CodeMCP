package modules

import (
	"testing"
)

func TestImportEdgeCreation(t *testing.T) {
	edge := NewImportEdge("src/main.ts", "src/utils.ts", LocalFile, 0.9, "./utils")

	if edge.From != "src/main.ts" {
		t.Errorf("Expected From 'src/main.ts', got '%s'", edge.From)
	}

	if edge.To != "src/utils.ts" {
		t.Errorf("Expected To 'src/utils.ts', got '%s'", edge.To)
	}

	if edge.Kind != LocalFile {
		t.Errorf("Expected Kind LocalFile, got '%s'", edge.Kind)
	}

	if edge.Confidence != 0.9 {
		t.Errorf("Expected Confidence 0.9, got %f", edge.Confidence)
	}

	if edge.RawImport != "./utils" {
		t.Errorf("Expected RawImport './utils', got '%s'", edge.RawImport)
	}
}

func TestImportEdgeIsLocal(t *testing.T) {
	testCases := []struct {
		kind     ImportEdgeKind
		expected bool
	}{
		{LocalFile, true},
		{LocalModule, true},
		{WorkspacePackage, false},
		{ExternalDependency, false},
		{Stdlib, false},
		{Unknown, false},
	}

	for _, tc := range testCases {
		edge := &ImportEdge{Kind: tc.kind}
		if edge.IsLocal() != tc.expected {
			t.Errorf("ImportEdge with kind %s: IsLocal() = %v, expected %v", tc.kind, edge.IsLocal(), tc.expected)
		}
	}
}

func TestImportEdgeIsExternal(t *testing.T) {
	testCases := []struct {
		kind     ImportEdgeKind
		expected bool
	}{
		{LocalFile, false},
		{LocalModule, false},
		{WorkspacePackage, false},
		{ExternalDependency, true},
		{Stdlib, false},
		{Unknown, false},
	}

	for _, tc := range testCases {
		edge := &ImportEdge{Kind: tc.kind}
		if edge.IsExternal() != tc.expected {
			t.Errorf("ImportEdge with kind %s: IsExternal() = %v, expected %v", tc.kind, edge.IsExternal(), tc.expected)
		}
	}
}

func TestImportEdgeIsStdlib(t *testing.T) {
	testCases := []struct {
		kind     ImportEdgeKind
		expected bool
	}{
		{LocalFile, false},
		{LocalModule, false},
		{WorkspacePackage, false},
		{ExternalDependency, false},
		{Stdlib, true},
		{Unknown, false},
	}

	for _, tc := range testCases {
		edge := &ImportEdge{Kind: tc.kind}
		if edge.IsStdlib() != tc.expected {
			t.Errorf("ImportEdge with kind %s: IsStdlib() = %v, expected %v", tc.kind, edge.IsStdlib(), tc.expected)
		}
	}
}

func TestImportEdgeKindValues(t *testing.T) {
	// Verify the string values match expected constants
	if LocalFile != "local-file" {
		t.Errorf("LocalFile should be 'local-file', got '%s'", LocalFile)
	}
	if LocalModule != "local-module" {
		t.Errorf("LocalModule should be 'local-module', got '%s'", LocalModule)
	}
	if WorkspacePackage != "workspace-package" {
		t.Errorf("WorkspacePackage should be 'workspace-package', got '%s'", WorkspacePackage)
	}
	if ExternalDependency != "external-dependency" {
		t.Errorf("ExternalDependency should be 'external-dependency', got '%s'", ExternalDependency)
	}
	if Stdlib != "stdlib" {
		t.Errorf("Stdlib should be 'stdlib', got '%s'", Stdlib)
	}
	if Unknown != "unknown" {
		t.Errorf("Unknown should be 'unknown', got '%s'", Unknown)
	}
}

func TestImportEdgeWithLine(t *testing.T) {
	edge := NewImportEdge("src/main.ts", "lodash", ExternalDependency, 1.0, "lodash")
	edge.Line = 5

	if edge.Line != 5 {
		t.Errorf("Expected Line 5, got %d", edge.Line)
	}
}

func TestImportEdgeMultipleCases(t *testing.T) {
	testCases := []struct {
		name       string
		from       string
		to         string
		kind       ImportEdgeKind
		confidence float64
		rawImport  string
		expectLocal    bool
		expectExternal bool
		expectStdlib   bool
	}{
		{
			name:       "relative import",
			from:       "src/components/Button.tsx",
			to:         "src/components/Icon.tsx",
			kind:       LocalFile,
			confidence: 0.95,
			rawImport:  "./Icon",
			expectLocal:    true,
			expectExternal: false,
			expectStdlib:   false,
		},
		{
			name:       "npm package",
			from:       "src/index.ts",
			to:         "react",
			kind:       ExternalDependency,
			confidence: 1.0,
			rawImport:  "react",
			expectLocal:    false,
			expectExternal: true,
			expectStdlib:   false,
		},
		{
			name:       "node stdlib",
			from:       "src/server.ts",
			to:         "fs",
			kind:       Stdlib,
			confidence: 1.0,
			rawImport:  "node:fs",
			expectLocal:    false,
			expectExternal: false,
			expectStdlib:   true,
		},
		{
			name:       "workspace package",
			from:       "packages/app/src/index.ts",
			to:         "packages/shared/src/utils",
			kind:       WorkspacePackage,
			confidence: 0.85,
			rawImport:  "@company/shared",
			expectLocal:    false,
			expectExternal: false,
			expectStdlib:   false,
		},
		{
			name:       "local module import",
			from:       "internal/api/handler.go",
			to:         "internal/query/engine.go",
			kind:       LocalModule,
			confidence: 0.9,
			rawImport:  "ckb/internal/query",
			expectLocal:    true,
			expectExternal: false,
			expectStdlib:   false,
		},
		{
			name:       "unknown import",
			from:       "src/main.ts",
			to:         "???",
			kind:       Unknown,
			confidence: 0.0,
			rawImport:  "something-weird",
			expectLocal:    false,
			expectExternal: false,
			expectStdlib:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			edge := NewImportEdge(tc.from, tc.to, tc.kind, tc.confidence, tc.rawImport)

			if edge.From != tc.from {
				t.Errorf("From: expected '%s', got '%s'", tc.from, edge.From)
			}
			if edge.To != tc.to {
				t.Errorf("To: expected '%s', got '%s'", tc.to, edge.To)
			}
			if edge.Kind != tc.kind {
				t.Errorf("Kind: expected '%s', got '%s'", tc.kind, edge.Kind)
			}
			if edge.Confidence != tc.confidence {
				t.Errorf("Confidence: expected %f, got %f", tc.confidence, edge.Confidence)
			}
			if edge.RawImport != tc.rawImport {
				t.Errorf("RawImport: expected '%s', got '%s'", tc.rawImport, edge.RawImport)
			}
			if edge.IsLocal() != tc.expectLocal {
				t.Errorf("IsLocal(): expected %v, got %v", tc.expectLocal, edge.IsLocal())
			}
			if edge.IsExternal() != tc.expectExternal {
				t.Errorf("IsExternal(): expected %v, got %v", tc.expectExternal, edge.IsExternal())
			}
			if edge.IsStdlib() != tc.expectStdlib {
				t.Errorf("IsStdlib(): expected %v, got %v", tc.expectStdlib, edge.IsStdlib())
			}
		})
	}
}

// BenchmarkImportEdgeCreation benchmarks creating import edges
func BenchmarkImportEdgeCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		edge := NewImportEdge("src/main.ts", "src/utils.ts", LocalFile, 0.9, "./utils")
		_ = edge
	}
}

// BenchmarkImportEdgeClassification benchmarks import classification
func BenchmarkImportEdgeClassification(b *testing.B) {
	edge := NewImportEdge("src/main.ts", "src/utils.ts", LocalFile, 0.9, "./utils")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = edge.IsLocal()
		_ = edge.IsExternal()
		_ = edge.IsStdlib()
	}
}

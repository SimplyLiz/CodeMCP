package incremental

import (
	"testing"
)

func TestIsLocalSymbol(t *testing.T) {
	tests := []struct {
		symbolID string
		expected bool
	}{
		{"local 0", true},
		{"local 123", true},
		{"scip-go gomod example.com/foo 1.0 pkg.Func().", false},
		{"", false},
		{"loca", false},  // Too short to be "local "
		{"local", false}, // Missing space
	}

	for _, tc := range tests {
		t.Run(tc.symbolID, func(t *testing.T) {
			result := isLocalSymbol(tc.symbolID)
			if result != tc.expected {
				t.Errorf("isLocalSymbol(%q) = %v, want %v", tc.symbolID, result, tc.expected)
			}
		})
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		name        string
		symbolID    string
		displayName string
		expected    string
	}{
		{
			name:        "uses displayName if provided",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Func().",
			displayName: "Func",
			expected:    "Func",
		},
		{
			name:        "extracts from symbolID without displayName",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Func().",
			displayName: "",
			expected:    "Func",
		},
		{
			name:        "handles method names",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Type.Method().",
			displayName: "",
			expected:    "Method",
		},
		{
			name:        "handles type names",
			symbolID:    "scip-go gomod example.com/foo 1.0 pkg.Type.",
			displayName: "",
			expected:    "Type",
		},
		{
			name:        "handles short symbolID",
			symbolID:    "short",
			displayName: "",
			expected:    "short",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractSymbolName(tc.symbolID, tc.displayName)
			if result != tc.expected {
				t.Errorf("extractSymbolName(%q, %q) = %q, want %q",
					tc.symbolID, tc.displayName, result, tc.expected)
			}
		})
	}
}

func TestMapSymbolKind(t *testing.T) {
	tests := []struct {
		kind     int32
		expected string
	}{
		{0, "unknown"},
		{5, "class"},
		{6, "method"},
		{8, "field"},
		{12, "function"},
		{13, "variable"},
		{14, "constant"},
		{23, "struct"},
		{999, "unknown"}, // Unknown value
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := mapSymbolKind(tc.kind)
			if result != tc.expected {
				t.Errorf("mapSymbolKind(%d) = %q, want %q", tc.kind, result, tc.expected)
			}
		})
	}
}

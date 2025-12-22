package docs

import (
	"testing"
)

func TestGenerateSuffixes(t *testing.T) {
	tests := []struct {
		canonical string
		expected  []string
	}{
		{
			"UserService.Authenticate",
			[]string{"UserService.Authenticate"},
		},
		{
			// Input should already be normalized (dots only)
			"internal.auth.UserService.Authenticate",
			[]string{
				"UserService.Authenticate",
				"auth.UserService.Authenticate",
				"internal.auth.UserService.Authenticate",
			},
		},
		{
			"a.b.c.d",
			[]string{"c.d", "b.c.d", "a.b.c.d"},
		},
		{
			"Single",
			nil, // No 2+ segment suffixes possible
		},
	}

	for _, tt := range tests {
		t.Run(tt.canonical, func(t *testing.T) {
			result := GenerateSuffixes(tt.canonical)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d suffixes, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("suffix %d: expected %q, got %q", i, tt.expected[i], s)
				}
			}
		})
	}
}

func TestParseCanonicalName(t *testing.T) {
	tests := []struct {
		scip     string
		expected string
	}{
		{
			// Slashes normalized to dots
			"scip-go gomod github.com/foo/ckb 1.0.0 internal/auth.UserService.Authenticate().",
			"internal.auth.UserService.Authenticate",
		},
		{
			"scip-go gomod github.com/foo/ckb 1.0.0 pkg.Function().",
			"pkg.Function",
		},
		{
			"scip-go gomod github.com/foo/ckb 1.0.0 pkg.Variable.",
			"pkg.Variable",
		},
		{
			// Backticks removed, slashes normalized
			"scip-go gomod ckb 1.0.0 `ckb/internal/query`.Engine#Start().",
			"ckb.internal.query.Engine.Start",
		},
		{
			// Hash normalized to dot
			"scip-go gomod ckb 1.0.0 pkg.Type#Method().",
			"pkg.Type.Method",
		},
		{
			// Fallback for non-standard format
			"invalid",
			"invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.scip, func(t *testing.T) {
			result := ParseCanonicalName(tt.scip)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractDisplayName(t *testing.T) {
	tests := []struct {
		scip     string
		expected string
	}{
		{
			"scip-go gomod github.com/foo/ckb 1.0.0 internal/auth.UserService.Authenticate().",
			"UserService.Authenticate",
		},
		{
			"scip-go gomod github.com/foo/ckb 1.0.0 pkg.Function().",
			"pkg.Function",
		},
		{
			"scip-go gomod github.com/foo/ckb 1.0.0 a.b.c.d.Method().",
			"d.Method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.scip, func(t *testing.T) {
			result := ExtractDisplayName(tt.scip)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// MockSymbolIndex implements SymbolIndex for testing.
type MockSymbolIndex struct {
	symbols map[string]string // canonicalName -> symbolID
	names   map[string]string // symbolID -> displayName
}

func NewMockSymbolIndex() *MockSymbolIndex {
	return &MockSymbolIndex{
		symbols: make(map[string]string),
		names:   make(map[string]string),
	}
}

func (m *MockSymbolIndex) AddSymbol(id, canonical, display string) {
	m.symbols[canonical] = id
	m.names[id] = display
}

func (m *MockSymbolIndex) ExactMatch(canonicalName string) (string, bool) {
	id, ok := m.symbols[canonicalName]
	return id, ok
}

func (m *MockSymbolIndex) GetDisplayName(symbolID string) string {
	return m.names[symbolID]
}

func (m *MockSymbolIndex) Exists(symbolID string) bool {
	_, ok := m.names[symbolID]
	return ok
}

func (m *MockSymbolIndex) IsLanguageIndexed(hint string) bool {
	return true
}

func TestResolverIneligible(t *testing.T) {
	index := NewMockSymbolIndex()
	// No store needed for ineligible test
	resolver := &Resolver{
		symbolIndex: index,
		store:       nil,
		config:      DefaultResolverConfig(),
	}

	result := resolver.Resolve("`SingleSegment`")

	if result.Status != ResolutionIneligible {
		t.Errorf("expected ResolutionIneligible, got %s", result.Status)
	}
}

func TestResolverExactMatch(t *testing.T) {
	index := NewMockSymbolIndex()
	index.AddSymbol("sym:123", "UserService.Authenticate", "UserService.Authenticate")

	resolver := &Resolver{
		symbolIndex: index,
		store:       nil,
		config:      DefaultResolverConfig(),
	}

	result := resolver.Resolve("`UserService.Authenticate`")

	if result.Status != ResolutionExact {
		t.Errorf("expected ResolutionExact, got %s", result.Status)
	}
	if result.SymbolID != "sym:123" {
		t.Errorf("expected sym:123, got %s", result.SymbolID)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", result.Confidence)
	}
}

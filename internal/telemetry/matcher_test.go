package telemetry

import (
	"testing"
)

func TestNamesMatch(t *testing.T) {
	tests := []struct {
		name          string
		indexName     string
		telemetryName string
		expected      bool
	}{
		{"exact match", "Function", "Function", true},
		{"different names", "Foo", "Bar", false},
		{"method receiver", "(*Foo).Bar", "Bar", true},
		{"struct method", "Foo.Bar", "Bar", true},
		{"package prefix telemetry", "Function", "pkg.Function", true},
		{"package prefix index", "pkg.Function", "Function", true}, // index name with prefix matches bare name
		{"complex method receiver", "(*internal/pkg.Type).Method", "Method", true},
		{"empty index", "", "Foo", false},
		{"empty telemetry", "Foo", "", false},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := namesMatch(tt.indexName, tt.telemetryName)
			if result != tt.expected {
				t.Errorf("namesMatch(%q, %q) = %v, want %v",
					tt.indexName, tt.telemetryName, result, tt.expected)
			}
		})
	}
}

func TestFindUniqueByName(t *testing.T) {
	t.Run("finds unique match", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Foo"},
			{ID: "sym2", Name: "Bar"},
			{ID: "sym3", Name: "Baz"},
		}

		result := findUniqueByName(symbols, "Bar")
		if result == nil {
			t.Fatal("expected to find symbol")
		}
		if result.ID != "sym2" {
			t.Errorf("expected sym2, got %s", result.ID)
		}
	})

	t.Run("returns nil for no match", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Foo"},
			{ID: "sym2", Name: "Bar"},
		}

		result := findUniqueByName(symbols, "Qux")
		if result != nil {
			t.Error("expected nil for no match")
		}
	})

	t.Run("returns nil for multiple matches", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Foo"},
			{ID: "sym2", Name: "Foo"}, // duplicate name
		}

		result := findUniqueByName(symbols, "Foo")
		if result != nil {
			t.Error("expected nil for ambiguous match")
		}
	})

	t.Run("handles empty list", func(t *testing.T) {
		result := findUniqueByName(nil, "Foo")
		if result != nil {
			t.Error("expected nil for empty list")
		}
	})

	t.Run("matches method name", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "(*Type).Method"},
		}

		result := findUniqueByName(symbols, "Method")
		if result == nil {
			t.Fatal("expected to find method")
		}
		if result.ID != "sym1" {
			t.Errorf("expected sym1, got %s", result.ID)
		}
	})
}

func TestFilterByName(t *testing.T) {
	t.Run("filters matching symbols", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Foo"},
			{ID: "sym2", Name: "Bar"},
			{ID: "sym3", Name: "Foo"},
		}

		result := filterByName(symbols, "Foo")
		if len(result) != 2 {
			t.Errorf("expected 2 matches, got %d", len(result))
		}
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Foo"},
			{ID: "sym2", Name: "Bar"},
		}

		result := filterByName(symbols, "Qux")
		if len(result) != 0 {
			t.Errorf("expected 0 matches, got %d", len(result))
		}
	})

	t.Run("handles empty list", func(t *testing.T) {
		result := filterByName(nil, "Foo")
		if len(result) != 0 {
			t.Errorf("expected 0 matches, got %d", len(result))
		}
	})

	t.Run("filters with method matching", func(t *testing.T) {
		symbols := []*IndexedSymbol{
			{ID: "sym1", Name: "Type.Process"},
			{ID: "sym2", Name: "(*Other).Process"},
			{ID: "sym3", Name: "Handle"},
		}

		result := filterByName(symbols, "Process")
		if len(result) != 2 {
			t.Errorf("expected 2 matches for Process, got %d", len(result))
		}
	})
}

// MockSymbolIndex implements SymbolIndex for testing
type MockSymbolIndex struct {
	byLocation  map[string]*IndexedSymbol
	byFile      map[string][]*IndexedSymbol
	byNamespace map[string][]*IndexedSymbol
	byName      map[string][]*IndexedSymbol
}

func NewMockSymbolIndex() *MockSymbolIndex {
	return &MockSymbolIndex{
		byLocation:  make(map[string]*IndexedSymbol),
		byFile:      make(map[string][]*IndexedSymbol),
		byNamespace: make(map[string][]*IndexedSymbol),
		byName:      make(map[string][]*IndexedSymbol),
	}
}

func (m *MockSymbolIndex) FindByLocation(filePath string, line int) *IndexedSymbol {
	key := filePath + ":" + string(rune(line))
	return m.byLocation[key]
}

func (m *MockSymbolIndex) FindByFile(filePath string) []*IndexedSymbol {
	return m.byFile[filePath]
}

func (m *MockSymbolIndex) FindByNamespace(namespace string) []*IndexedSymbol {
	return m.byNamespace[namespace]
}

func (m *MockSymbolIndex) FindByName(name string) []*IndexedSymbol {
	return m.byName[name]
}

func TestNewMatcher(t *testing.T) {
	index := NewMockSymbolIndex()
	matcher := NewMatcher(index)

	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}
}

func TestMatcherMatch(t *testing.T) {
	t.Run("unmatched with no data", func(t *testing.T) {
		index := NewMockSymbolIndex()
		matcher := NewMatcher(index)

		call := &CallAggregate{
			FunctionName: "Process",
			FilePath:     "unknown.go",
			Namespace:    "pkg",
		}

		result := matcher.Match(call)
		if result.Quality != MatchUnmatched {
			t.Errorf("expected MatchUnmatched, got %v", result.Quality)
		}
	})

	t.Run("weak match by name only", func(t *testing.T) {
		index := NewMockSymbolIndex()
		index.byName["Process"] = []*IndexedSymbol{
			{ID: "sym1", Name: "Process"},
		}
		matcher := NewMatcher(index)

		call := &CallAggregate{
			FunctionName: "Process",
		}

		result := matcher.Match(call)
		if result.Quality != MatchWeak {
			t.Errorf("expected MatchWeak, got %v", result.Quality)
		}
		if result.SymbolID != "sym1" {
			t.Errorf("expected sym1, got %s", result.SymbolID)
		}
	})

	t.Run("unmatched with multiple global matches", func(t *testing.T) {
		index := NewMockSymbolIndex()
		index.byName["Process"] = []*IndexedSymbol{
			{ID: "sym1", Name: "Process"},
			{ID: "sym2", Name: "Process"},
		}
		matcher := NewMatcher(index)

		call := &CallAggregate{
			FunctionName: "Process",
		}

		result := matcher.Match(call)
		// Multiple matches = unmatched
		if result.Quality != MatchUnmatched {
			t.Errorf("expected MatchUnmatched for ambiguous, got %v", result.Quality)
		}
	})
}

func TestMatcherMatchBatch(t *testing.T) {
	index := NewMockSymbolIndex()
	index.byName["Foo"] = []*IndexedSymbol{{ID: "sym1", Name: "Foo"}}
	matcher := NewMatcher(index)

	calls := []*CallAggregate{
		{FunctionName: "Foo"},
		{FunctionName: "Bar"},
	}

	results := matcher.MatchBatch(calls)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[calls[0]].SymbolID != "sym1" {
		t.Error("expected Foo to match sym1")
	}
	if results[calls[1]].Quality != MatchUnmatched {
		t.Error("expected Bar to be unmatched")
	}
}

func TestSCIPSymbolIndex(t *testing.T) {
	t.Run("NewSCIPSymbolIndex", func(t *testing.T) {
		idx := NewSCIPSymbolIndex()
		if idx == nil {
			t.Fatal("expected non-nil index")
		}
		if idx.SymbolCount() != 0 {
			t.Error("expected empty index")
		}
	})

	t.Run("AddSymbol and lookup", func(t *testing.T) {
		idx := NewSCIPSymbolIndex()
		sym := &IndexedSymbol{
			ID:        "sym1",
			Name:      "Process",
			File:      "handler.go",
			Line:      42,
			Namespace: "internal/api",
		}
		idx.AddSymbol(sym)

		if idx.SymbolCount() != 1 {
			t.Errorf("expected 1 symbol, got %d", idx.SymbolCount())
		}

		// Test FindByFile
		byFile := idx.FindByFile("handler.go")
		if len(byFile) != 1 {
			t.Errorf("expected 1 symbol in file, got %d", len(byFile))
		}

		// Test FindByNamespace
		byNs := idx.FindByNamespace("internal/api")
		if len(byNs) != 1 {
			t.Errorf("expected 1 symbol in namespace, got %d", len(byNs))
		}

		// Test FindByName
		byName := idx.FindByName("Process")
		if len(byName) != 1 {
			t.Errorf("expected 1 symbol by name, got %d", len(byName))
		}

		// Test FindByLocation
		byLoc := idx.FindByLocation("handler.go", 42)
		if byLoc == nil {
			t.Error("expected to find symbol at location")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		idx := NewSCIPSymbolIndex()
		idx.AddSymbol(&IndexedSymbol{ID: "sym1", Name: "Foo", File: "foo.go"})
		idx.AddSymbol(&IndexedSymbol{ID: "sym2", Name: "Bar", File: "bar.go"})

		if idx.SymbolCount() != 2 {
			t.Errorf("expected 2 symbols before clear, got %d", idx.SymbolCount())
		}

		idx.Clear()

		if idx.SymbolCount() != 0 {
			t.Errorf("expected 0 symbols after clear, got %d", idx.SymbolCount())
		}
	})
}

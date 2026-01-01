package query

import (
	"strings"
	"testing"
)

func TestParseScope(t *testing.T) {
	tests := []struct {
		scope    string
		expected []string
	}{
		{"", nil},
		{"internal/query", []string{"internal/query"}},
		{"cmd/ckb", []string{"cmd/ckb"}},
	}

	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			result := parseScope(tt.scope)
			if tt.expected == nil && result != nil {
				t.Errorf("parseScope(%q) = %v, want nil", tt.scope, result)
				return
			}
			if tt.expected != nil {
				if len(result) != len(tt.expected) {
					t.Errorf("parseScope(%q) = %v, want %v", tt.scope, result, tt.expected)
					return
				}
				for i := range result {
					if result[i] != tt.expected[i] {
						t.Errorf("parseScope(%q)[%d] = %q, want %q", tt.scope, i, result[i], tt.expected[i])
					}
				}
			}
		})
	}
}

func TestGenerateSearchCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		opts     SearchSymbolsOptions
		wantHash bool
	}{
		{
			name:     "simple query",
			opts:     SearchSymbolsOptions{Query: "Engine", Limit: 20},
			wantHash: true,
		},
		{
			name:     "with scope",
			opts:     SearchSymbolsOptions{Query: "Engine", Scope: "internal/query", Limit: 20},
			wantHash: true,
		},
		{
			name:     "with kinds",
			opts:     SearchSymbolsOptions{Query: "Engine", Kinds: []string{"function", "class"}, Limit: 20},
			wantHash: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSearchCacheKey(tt.opts)
			if !strings.HasPrefix(result, "search:") {
				t.Errorf("generateSearchCacheKey() = %q, expected prefix 'search:'", result)
			}
			if tt.wantHash && len(result) < 10 {
				t.Errorf("generateSearchCacheKey() = %q, expected longer hash", result)
			}
		})
	}

	// Same options should produce same key
	t.Run("deterministic", func(t *testing.T) {
		opts := SearchSymbolsOptions{Query: "Engine", Limit: 20}
		key1 := generateSearchCacheKey(opts)
		key2 := generateSearchCacheKey(opts)
		if key1 != key2 {
			t.Errorf("generateSearchCacheKey not deterministic: %q != %q", key1, key2)
		}
	})

	// Kind order should not matter
	t.Run("kind order independent", func(t *testing.T) {
		opts1 := SearchSymbolsOptions{Query: "Engine", Kinds: []string{"function", "class"}, Limit: 20}
		opts2 := SearchSymbolsOptions{Query: "Engine", Kinds: []string{"class", "function"}, Limit: 20}
		key1 := generateSearchCacheKey(opts1)
		key2 := generateSearchCacheKey(opts2)
		if key1 != key2 {
			t.Errorf("generateSearchCacheKey should be kind-order independent: %q != %q", key1, key2)
		}
	})
}

func TestNewRankingV52(t *testing.T) {
	signals := map[string]interface{}{
		"matchType": "exact",
		"kind":      "function",
	}

	ranking := NewRankingV52(85.0, signals)

	if ranking == nil {
		t.Fatal("NewRankingV52 returned nil")
	}
	if ranking.Score != 85.0 {
		t.Errorf("Score = %f, want 85.0", ranking.Score)
	}
	if ranking.PolicyVersion != "5.2" {
		t.Errorf("PolicyVersion = %q, want '5.2'", ranking.PolicyVersion)
	}
	if ranking.Signals["matchType"] != "exact" {
		t.Errorf("Signals[matchType] = %v, want 'exact'", ranking.Signals["matchType"])
	}
}

func TestDeduplicateReferences(t *testing.T) {
	tests := []struct {
		name     string
		refs     []ReferenceInfo
		expected int
	}{
		{
			name:     "empty",
			refs:     []ReferenceInfo{},
			expected: 0,
		},
		{
			name: "no duplicates",
			refs: []ReferenceInfo{
				{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 5}},
				{Location: &LocationInfo{FileId: "b.go", StartLine: 20, StartColumn: 10}},
			},
			expected: 2,
		},
		{
			name: "with duplicates",
			refs: []ReferenceInfo{
				{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 5}},
				{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 5}},
				{Location: &LocationInfo{FileId: "b.go", StartLine: 20, StartColumn: 10}},
			},
			expected: 2,
		},
		{
			name: "nil location skipped",
			refs: []ReferenceInfo{
				{Location: nil},
				{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 5}},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateReferences(tt.refs)
			if len(result) != tt.expected {
				t.Errorf("deduplicateReferences() returned %d refs, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestSortReferences(t *testing.T) {
	refs := []ReferenceInfo{
		{Location: &LocationInfo{FileId: "b.go", StartLine: 10, StartColumn: 5}},
		{Location: &LocationInfo{FileId: "a.go", StartLine: 20, StartColumn: 10}},
		{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 15}},
		{Location: &LocationInfo{FileId: "a.go", StartLine: 10, StartColumn: 5}},
	}

	sortReferences(refs)

	// Should be sorted by file, then line, then column
	expected := []struct {
		file   string
		line   int
		column int
	}{
		{"a.go", 10, 5},
		{"a.go", 10, 15},
		{"a.go", 20, 10},
		{"b.go", 10, 5},
	}

	for i, exp := range expected {
		if refs[i].Location.FileId != exp.file {
			t.Errorf("refs[%d].FileId = %q, want %q", i, refs[i].Location.FileId, exp.file)
		}
		if refs[i].Location.StartLine != exp.line {
			t.Errorf("refs[%d].StartLine = %d, want %d", i, refs[i].Location.StartLine, exp.line)
		}
		if refs[i].Location.StartColumn != exp.column {
			t.Errorf("refs[%d].StartColumn = %d, want %d", i, refs[i].Location.StartColumn, exp.column)
		}
	}
}

func TestGenerateTreesitterSymbolId(t *testing.T) {
	tests := []struct {
		path string
		name string
		kind string
		line int
	}{
		{"internal/query/engine.go", "Engine", "struct", 42},
		{"cmd/ckb/main.go", "main", "function", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateTreesitterSymbolId(tt.path, tt.name, tt.kind, tt.line)
			if !strings.HasPrefix(id, "ts-") {
				t.Errorf("generateTreesitterSymbolId() = %q, expected prefix 'ts-'", id)
			}
			if len(id) < 10 {
				t.Errorf("generateTreesitterSymbolId() = %q, expected longer ID", id)
			}
		})
	}

	// Same inputs should produce same ID
	t.Run("deterministic", func(t *testing.T) {
		id1 := generateTreesitterSymbolId("a.go", "Foo", "function", 10)
		id2 := generateTreesitterSymbolId("a.go", "Foo", "function", 10)
		if id1 != id2 {
			t.Errorf("generateTreesitterSymbolId not deterministic: %q != %q", id1, id2)
		}
	})

	// Different inputs should produce different IDs
	t.Run("different inputs", func(t *testing.T) {
		id1 := generateTreesitterSymbolId("a.go", "Foo", "function", 10)
		id2 := generateTreesitterSymbolId("a.go", "Bar", "function", 10)
		if id1 == id2 {
			t.Errorf("generateTreesitterSymbolId should differ for different names")
		}
	})
}

func TestInferVisibility(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected string
	}{
		{"", "function", "unknown"},
		{"Engine", "struct", "public"},
		{"engine", "struct", "internal"},
		{"GetUser", "method", "public"},
		{"getUser", "method", "internal"},
		{"_helper", "function", "internal"},
		{"__private", "function", "private"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.kind, func(t *testing.T) {
			result := inferVisibility(tt.name, tt.kind)
			if result != tt.expected {
				t.Errorf("inferVisibility(%q, %q) = %q, want %q", tt.name, tt.kind, result, tt.expected)
			}
		})
	}
}

func TestRankSearchResults(t *testing.T) {
	tests := []struct {
		name         string
		results      []SearchResultItem
		query        string
		wantScored   bool
		wantRankings bool
	}{
		{
			name:         "empty results",
			results:      []SearchResultItem{},
			query:        "Engine",
			wantScored:   true,
			wantRankings: true,
		},
		{
			name: "exact match",
			results: []SearchResultItem{
				{Name: "Engine", Kind: "class"},
			},
			query:        "Engine",
			wantScored:   true,
			wantRankings: true,
		},
		{
			name: "partial match",
			results: []SearchResultItem{
				{Name: "EngineFactory", Kind: "class"},
			},
			query:        "Engine",
			wantScored:   true,
			wantRankings: true,
		},
		{
			name: "contains match",
			results: []SearchResultItem{
				{Name: "QueryEngine", Kind: "class"},
			},
			query:        "Engine",
			wantScored:   true,
			wantRankings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]SearchResultItem, len(tt.results))
			copy(results, tt.results)

			rankSearchResults(results, tt.query)

			for i, r := range results {
				if tt.wantScored && r.Score == 0 && len(tt.results) > 0 {
					t.Errorf("results[%d].Score = 0, expected non-zero score", i)
				}
				if tt.wantRankings && r.Ranking == nil && len(tt.results) > 0 {
					t.Errorf("results[%d].Ranking = nil, expected ranking", i)
				}
			}
		})
	}

	// Exact match should score higher than partial
	t.Run("exact scores higher", func(t *testing.T) {
		results := []SearchResultItem{
			{Name: "EngineFactory", Kind: "class"},
			{Name: "Engine", Kind: "class"},
		}
		rankSearchResults(results, "Engine")

		// Find scores
		var exactScore, partialScore float64
		for _, r := range results {
			if r.Name == "Engine" {
				exactScore = r.Score
			} else {
				partialScore = r.Score
			}
		}

		if exactScore <= partialScore {
			t.Errorf("exact match score (%f) should be higher than partial (%f)", exactScore, partialScore)
		}
	})
}

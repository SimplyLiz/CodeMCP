package query

import (
	"strings"
	"testing"
)

// applyPostEnrichmentFilters mirrors the filtering logic from SearchSymbols
// (symbols.go lines 519-541) so we can unit-test it without a full Engine.
func applyPostEnrichmentFilters(results []SearchResultItem, opts SearchSymbolsOptions) []SearchResultItem {
	if opts.MinLines <= 0 && opts.MinComplexity <= 0 && len(opts.ExcludePatterns) == 0 {
		return results
	}
	var filtered []SearchResultItem
	for _, r := range results {
		if opts.MinLines > 0 && r.Lines > 0 && r.Lines < opts.MinLines {
			continue
		}
		if opts.MinComplexity > 0 && r.Cyclomatic < opts.MinComplexity {
			continue
		}
		excluded := false
		for _, p := range opts.ExcludePatterns {
			if strings.Contains(r.Name, p) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func TestSearchSymbolsOptions_ExcludePatterns(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "Engine#field", Kind: "field", StableId: "s1"},
		{Name: "Engine", Kind: "class", StableId: "s2"},
		{Name: "doWork", Kind: "function", StableId: "s3"},
		{Name: "Config#timeout", Kind: "field", StableId: "s4"},
	}

	opts := SearchSymbolsOptions{
		ExcludePatterns: []string{"#"},
	}

	got := applyPostEnrichmentFilters(input, opts)

	if len(got) != 2 {
		t.Fatalf("expected 2 results after excluding '#', got %d", len(got))
	}
	for _, r := range got {
		if strings.Contains(r.Name, "#") {
			t.Errorf("result %q should have been excluded (contains '#')", r.Name)
		}
	}
	if got[0].Name != "Engine" {
		t.Errorf("expected first result 'Engine', got %q", got[0].Name)
	}
	if got[1].Name != "doWork" {
		t.Errorf("expected second result 'doWork', got %q", got[1].Name)
	}
}

func TestSearchSymbolsOptions_ExcludePatterns_Multiple(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "Engine#field", Kind: "field", StableId: "s1"},
		{Name: "Foo.()", Kind: "method", StableId: "s2"},
		{Name: "doWork", Kind: "function", StableId: "s3"},
	}

	opts := SearchSymbolsOptions{
		ExcludePatterns: []string{"#", ".()"},
	}

	got := applyPostEnrichmentFilters(input, opts)

	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Name != "doWork" {
		t.Errorf("expected 'doWork', got %q", got[0].Name)
	}
}

func TestSearchSymbolsOptions_MinLines(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "small", Lines: 5, StableId: "s1"},
		{Name: "large", Lines: 50, StableId: "s2"},
		{Name: "unknown", Lines: 0, StableId: "s3"}, // no body data yet
	}

	opts := SearchSymbolsOptions{
		MinLines: 30,
	}

	got := applyPostEnrichmentFilters(input, opts)

	// Lines=5 is filtered out (below threshold).
	// Lines=50 passes.
	// Lines=0 passes (condition is: Lines > 0 && Lines < MinLines).
	if len(got) != 2 {
		t.Fatalf("expected 2 results with minLines=30, got %d: %+v", len(got), got)
	}

	names := map[string]bool{}
	for _, r := range got {
		names[r.Name] = true
	}
	if !names["large"] {
		t.Error("'large' (Lines=50) should pass minLines=30")
	}
	if !names["unknown"] {
		t.Error("'unknown' (Lines=0) should pass because Lines==0 means no body data")
	}
	if names["small"] {
		t.Error("'small' (Lines=5) should be filtered by minLines=30")
	}
}

func TestSearchSymbolsOptions_MinComplexity(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "simple", Cyclomatic: 1, StableId: "s1"},
		{Name: "complex", Cyclomatic: 15, StableId: "s2"},
		{Name: "nometric", Cyclomatic: 0, StableId: "s3"}, // no complexity data
	}

	opts := SearchSymbolsOptions{
		MinComplexity: 5,
	}

	got := applyPostEnrichmentFilters(input, opts)

	// Cyclomatic=1 is below 5 -> filtered.
	// Cyclomatic=15 passes.
	// Cyclomatic=0 is below 5 -> filtered (unlike Lines, there's no >0 guard).
	if len(got) != 1 {
		t.Fatalf("expected 1 result with minComplexity=5, got %d: %+v", len(got), got)
	}
	if got[0].Name != "complex" {
		t.Errorf("expected 'complex', got %q", got[0].Name)
	}
}

func TestSearchSymbolsOptions_CombinedFilters(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "Engine#field", Lines: 100, Cyclomatic: 10, StableId: "s1"}, // excluded by pattern
		{Name: "tinyFunc", Lines: 3, Cyclomatic: 10, StableId: "s2"},       // excluded by minLines
		{Name: "simpleFunc", Lines: 50, Cyclomatic: 1, StableId: "s3"},     // excluded by minComplexity
		{Name: "bigFunc", Lines: 80, Cyclomatic: 12, StableId: "s4"},       // passes all
	}

	opts := SearchSymbolsOptions{
		MinLines:        10,
		MinComplexity:   5,
		ExcludePatterns: []string{"#"},
	}

	got := applyPostEnrichmentFilters(input, opts)

	if len(got) != 1 {
		t.Fatalf("expected 1 result with combined filters, got %d: %+v", len(got), got)
	}
	if got[0].Name != "bigFunc" {
		t.Errorf("expected 'bigFunc', got %q", got[0].Name)
	}
}

func TestSearchSymbolsOptions_NoFilters(t *testing.T) {
	t.Parallel()

	input := []SearchResultItem{
		{Name: "a", StableId: "s1"},
		{Name: "b", StableId: "s2"},
	}

	opts := SearchSymbolsOptions{}

	got := applyPostEnrichmentFilters(input, opts)

	if len(got) != 2 {
		t.Fatalf("expected 2 results with no filters, got %d", len(got))
	}
}

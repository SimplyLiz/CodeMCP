package storage

import (
	"context"
	"testing"
)

func TestListAllReturnsAllSymbols(t *testing.T) {
	db, cleanup := setupTestFTSDB(t)
	defer cleanup()

	manager := NewFTSManager(db, DefaultFTSConfig())
	if err := manager.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	symbols := []SymbolFTSRecord{
		{ID: "s1", Name: "Alpha", Kind: "function", Documentation: "does alpha", FilePath: "alpha.go", Language: "go"},
		{ID: "s2", Name: "Beta", Kind: "class", Documentation: "does beta", FilePath: "beta.go", Language: "go"},
		{ID: "s3", Name: "Gamma", Kind: "method", Signature: "func Gamma() error", FilePath: "gamma.go", Language: "go"},
		{ID: "s4", Name: "Delta", Kind: "function", Documentation: "does delta", FilePath: "delta.go", Language: "go"},
		{ID: "s5", Name: "Epsilon", Kind: "variable", FilePath: "epsilon.go", Language: "go"},
	}

	ctx := context.Background()
	if err := manager.BulkInsert(ctx, symbols); err != nil {
		t.Fatalf("bulk insert failed: %v", err)
	}

	// Empty query with high limit should return all symbols.
	results, err := manager.Search(ctx, "", 10)
	if err != nil {
		t.Fatalf("Search(\"\", 10) error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results for empty query, got %d", len(results))
	}

	// Verify results are ordered by name (listAll uses ORDER BY name).
	expectedOrder := []string{"Alpha", "Beta", "Delta", "Epsilon", "Gamma"}
	for i, name := range expectedOrder {
		if i < len(results) && results[i].Name != name {
			t.Errorf("result[%d].Name = %q, want %q", i, results[i].Name, name)
		}
	}

	// All results should have MatchType "list".
	for i, r := range results {
		if r.MatchType != "list" {
			t.Errorf("result[%d].MatchType = %q, want \"list\"", i, r.MatchType)
		}
	}
}

func TestListAllRespectsLimit(t *testing.T) {
	db, cleanup := setupTestFTSDB(t)
	defer cleanup()

	manager := NewFTSManager(db, DefaultFTSConfig())
	if err := manager.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	symbols := []SymbolFTSRecord{
		{ID: "s1", Name: "Alpha", Kind: "function", FilePath: "a.go", Language: "go"},
		{ID: "s2", Name: "Beta", Kind: "function", FilePath: "b.go", Language: "go"},
		{ID: "s3", Name: "Gamma", Kind: "function", FilePath: "c.go", Language: "go"},
		{ID: "s4", Name: "Delta", Kind: "function", FilePath: "d.go", Language: "go"},
		{ID: "s5", Name: "Epsilon", Kind: "function", FilePath: "e.go", Language: "go"},
	}

	ctx := context.Background()
	if err := manager.BulkInsert(ctx, symbols); err != nil {
		t.Fatalf("bulk insert failed: %v", err)
	}

	results, err := manager.Search(ctx, "", 3)
	if err != nil {
		t.Fatalf("Search(\"\", 3) error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

func TestSearchWithQueryStillWorks(t *testing.T) {
	db, cleanup := setupTestFTSDB(t)
	defer cleanup()

	manager := NewFTSManager(db, DefaultFTSConfig())
	if err := manager.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	symbols := []SymbolFTSRecord{
		{ID: "s1", Name: "FuncAlpha", Kind: "function", Documentation: "a function", FilePath: "a.go", Language: "go"},
		{ID: "s2", Name: "ClassBeta", Kind: "class", Documentation: "a class", FilePath: "b.go", Language: "go"},
		{ID: "s3", Name: "FuncGamma", Kind: "function", Documentation: "another function", FilePath: "c.go", Language: "go"},
	}

	ctx := context.Background()
	if err := manager.BulkInsert(ctx, symbols); err != nil {
		t.Fatalf("bulk insert failed: %v", err)
	}

	// Non-empty query should do normal FTS search, not listAll.
	results, err := manager.Search(ctx, "Func", 10)
	if err != nil {
		t.Fatalf("Search(\"Func\", 10) error: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 results for \"Func\" query, got %d", len(results))
	}

	// Should not include ClassBeta (no "Func" in name/doc/sig).
	for _, r := range results {
		if r.Name == "ClassBeta" {
			t.Error("ClassBeta should not appear in results for query \"Func\"")
		}
	}

	// Results should NOT have MatchType "list".
	for _, r := range results {
		if r.MatchType == "list" {
			t.Errorf("non-empty query should not produce MatchType \"list\", got it for %s", r.Name)
		}
	}
}

func TestFTSEmptyDatabase(t *testing.T) {
	db, cleanup := setupTestFTSDB(t)
	defer cleanup()

	manager := NewFTSManager(db, DefaultFTSConfig())
	if err := manager.InitSchema(); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	ctx := context.Background()

	// Empty query on empty database should return empty slice, not error.
	results, err := manager.Search(ctx, "", 10)
	if err != nil {
		t.Fatalf("Search on empty DB should not error, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(results))
	}

	// Non-empty query on empty database should also return empty, not error.
	results, err = manager.Search(ctx, "anything", 10)
	if err != nil {
		t.Fatalf("Search(\"anything\") on empty DB should not error, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(results))
	}
}

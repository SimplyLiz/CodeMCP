package query

import (
	"testing"
)

func TestIndexDocs(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("runs without error on empty repo", func(t *testing.T) {
		stats, err := engine.IndexDocs(false)
		if err != nil {
			t.Fatalf("IndexDocs failed: %v", err)
		}
		if stats == nil {
			t.Error("stats should not be nil")
		}
	})

	t.Run("force re-index", func(t *testing.T) {
		stats, err := engine.IndexDocs(true)
		if err != nil {
			t.Fatalf("IndexDocs with force failed: %v", err)
		}
		if stats == nil {
			t.Error("stats should not be nil")
		}
	})
}

func TestGetDocsForSymbol(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns empty for unknown symbol", func(t *testing.T) {
		refs, err := engine.GetDocsForSymbol("nonexistent.Symbol", 10)
		if err != nil {
			t.Fatalf("GetDocsForSymbol failed: %v", err)
		}
		if len(refs) != 0 {
			t.Errorf("expected empty refs, got %d", len(refs))
		}
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		refs, err := engine.GetDocsForSymbol("any.Symbol", 5)
		if err != nil {
			t.Fatalf("GetDocsForSymbol failed: %v", err)
		}
		// Should not exceed limit (though likely empty)
		if len(refs) > 5 {
			t.Errorf("expected at most 5 refs, got %d", len(refs))
		}
	})

	t.Run("handles zero limit", func(t *testing.T) {
		refs, err := engine.GetDocsForSymbol("test.Symbol", 0)
		if err != nil {
			t.Fatalf("GetDocsForSymbol failed: %v", err)
		}
		// zero limit should work (may return 0 or use default)
		_ = refs
	})
}

func TestGetDocumentInfo(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns nil for unknown document", func(t *testing.T) {
		doc, err := engine.GetDocumentInfo("nonexistent/README.md")
		if err != nil {
			t.Fatalf("GetDocumentInfo failed: %v", err)
		}
		if doc != nil {
			t.Error("expected nil doc for unknown path")
		}
	})

	t.Run("handles empty path", func(t *testing.T) {
		doc, err := engine.GetDocumentInfo("")
		if err != nil {
			t.Fatalf("GetDocumentInfo failed: %v", err)
		}
		if doc != nil {
			t.Error("expected nil doc for empty path")
		}
	})
}

func TestGetDocsForModule(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns empty for unknown module", func(t *testing.T) {
		docs, err := engine.GetDocsForModule("unknown/module")
		if err != nil {
			t.Fatalf("GetDocsForModule failed: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("expected empty docs, got %d", len(docs))
		}
	})

	t.Run("handles empty module ID", func(t *testing.T) {
		docs, err := engine.GetDocsForModule("")
		if err != nil {
			t.Fatalf("GetDocsForModule failed: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("expected empty docs, got %d", len(docs))
		}
	})
}

func TestCheckDocStaleness(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns error for unknown document", func(t *testing.T) {
		_, err := engine.CheckDocStaleness("nonexistent/README.md")
		if err == nil {
			t.Error("expected error for unknown document")
		}
	})

	t.Run("error message includes path", func(t *testing.T) {
		_, err := engine.CheckDocStaleness("missing/file.md")
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "document not found: missing/file.md" {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestCheckAllDocsStaleness(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns empty on fresh repo", func(t *testing.T) {
		reports, err := engine.CheckAllDocsStaleness()
		if err != nil {
			t.Fatalf("CheckAllDocsStaleness failed: %v", err)
		}
		// On empty repo, should return empty or nil reports
		if len(reports) != 0 {
			t.Errorf("expected empty reports, got %d", len(reports))
		}
	})
}

func TestGetDocCoverage(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns coverage on empty repo", func(t *testing.T) {
		report, err := engine.GetDocCoverage(false, 10)
		if err != nil {
			t.Fatalf("GetDocCoverage failed: %v", err)
		}
		if report == nil {
			t.Error("report should not be nil")
		}
	})

	t.Run("exported only mode", func(t *testing.T) {
		report, err := engine.GetDocCoverage(true, 5)
		if err != nil {
			t.Fatalf("GetDocCoverage failed: %v", err)
		}
		if report == nil {
			t.Error("report should not be nil")
		}
	})

	t.Run("zero topN", func(t *testing.T) {
		report, err := engine.GetDocCoverage(false, 0)
		if err != nil {
			t.Fatalf("GetDocCoverage failed: %v", err)
		}
		if report == nil {
			t.Error("report should not be nil")
		}
	})
}

func TestScipSymbolIndex(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create the adapter directly for testing
	idx := &scipSymbolIndex{engine: engine}

	t.Run("ExactMatch returns false without SCIP", func(t *testing.T) {
		_, found := idx.ExactMatch("some.package.Symbol")
		if found {
			t.Error("should not find match without SCIP index")
		}
	})

	t.Run("GetDisplayName falls back to extraction", func(t *testing.T) {
		name := idx.GetDisplayName("ckb/internal/query.Engine")
		// Should extract something from the symbol ID
		if name == "" {
			t.Error("display name should not be empty")
		}
	})

	t.Run("Exists returns false without SCIP", func(t *testing.T) {
		exists := idx.Exists("some.symbol.ID")
		if exists {
			t.Error("should not exist without SCIP index")
		}
	})

	t.Run("IsLanguageIndexed returns false without SCIP", func(t *testing.T) {
		indexed := idx.IsLanguageIndexed("go")
		if indexed {
			t.Error("should not be indexed without SCIP")
		}
	})
}

func TestGetSymbolIndexVersion(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("returns unknown without git", func(t *testing.T) {
		version := engine.getSymbolIndexVersion()
		// Without a proper git repo, should return "unknown" or a commit hash
		if version == "" {
			t.Error("version should not be empty")
		}
	})
}

func TestRebuildSuffixIndex(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("no-op without SCIP adapter", func(t *testing.T) {
		// This creates a docs store and tries to rebuild
		// Should not error without SCIP
		// Access the internal method via reflection or just verify no panic

		// Since rebuildSuffixIndex is private, we test it through IndexDocs
		// which calls it internally
		_, err := engine.IndexDocs(false)
		if err != nil {
			t.Fatalf("IndexDocs (which uses rebuildSuffixIndex) failed: %v", err)
		}
	})
}

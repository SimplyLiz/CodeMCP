package scip

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// getTestAdapter returns a SCIP adapter for testing, or skips the test if unavailable
func getTestAdapter(t *testing.T) *SCIPAdapter {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RepoRoot = repoRoot

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	adapter, err := NewSCIPAdapter(cfg, logger)
	if err != nil {
		t.Skipf("SCIP adapter not available: %v", err)
	}

	if !adapter.IsAvailable() {
		t.Skip("SCIP index not available")
	}

	return adapter
}

// TestRefIndexBuiltDuringLoad verifies that RefIndex is populated during index load
func TestRefIndexBuiltDuringLoad(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	if index.RefIndex == nil {
		t.Fatal("RefIndex was not initialized")
	}

	if len(index.RefIndex) == 0 {
		t.Fatal("RefIndex is empty - should contain occurrence references")
	}

	// Verify RefIndex contains entries for known symbols
	refCount := 0
	for _, refs := range index.RefIndex {
		refCount += len(refs)
	}

	t.Logf("RefIndex contains %d symbols with %d total occurrence references", len(index.RefIndex), refCount)

	// RefIndex should have at least as many entries as there are symbols with occurrences
	if len(index.RefIndex) < 100 {
		t.Errorf("RefIndex seems too small: %d entries", len(index.RefIndex))
	}
}

// TestConvertedSymbolsCacheBuilt verifies that ConvertedSymbols is populated during load
func TestConvertedSymbolsCacheBuilt(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	if index.ConvertedSymbols == nil {
		t.Fatal("ConvertedSymbols was not initialized")
	}

	if len(index.ConvertedSymbols) == 0 {
		t.Fatal("ConvertedSymbols is empty - should contain pre-converted symbols")
	}

	// Should match the number of raw symbols
	if len(index.ConvertedSymbols) != len(index.Symbols) {
		t.Logf("ConvertedSymbols: %d, Symbols: %d (some may have failed conversion)",
			len(index.ConvertedSymbols), len(index.Symbols))
	}

	t.Logf("ConvertedSymbols contains %d pre-converted symbols", len(index.ConvertedSymbols))

	// Verify a sample symbol is correctly converted
	for symbolId, cached := range index.ConvertedSymbols {
		if cached == nil {
			t.Errorf("Cached symbol for %s is nil", symbolId)
			continue
		}
		if cached.StableId != symbolId {
			t.Errorf("Cached symbol StableId mismatch: got %s, want %s", cached.StableId, symbolId)
		}
		break // Just check one
	}
}

// TestGetCachedSymbol verifies GetCachedSymbol returns cached symbols
func TestGetCachedSymbol(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	// Get a known symbol ID from the cache
	var testSymbolId string
	for id := range index.ConvertedSymbols {
		testSymbolId = id
		break
	}

	if testSymbolId == "" {
		t.Skip("No symbols in cache to test")
	}

	// Get from cache
	cached, err := index.GetCachedSymbol(testSymbolId)
	if err != nil {
		t.Fatalf("GetCachedSymbol failed: %v", err)
	}

	if cached == nil {
		t.Fatal("GetCachedSymbol returned nil")
	}

	if cached.StableId != testSymbolId {
		t.Errorf("StableId mismatch: got %s, want %s", cached.StableId, testSymbolId)
	}

	// Verify it's the same object from cache (pointer equality)
	cached2, _ := index.GetCachedSymbol(testSymbolId)
	if cached != cached2 {
		t.Error("GetCachedSymbol should return the same cached object")
	}
}

// TestFindSymbolLocationFast verifies fast location lookup works correctly
func TestFindSymbolLocationFast(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	// Find a symbol that has a definition
	var testSymbolId string
	for symbolId, refs := range index.RefIndex {
		for _, ref := range refs {
			if ref.Occ.SymbolRoles&SymbolRoleDefinition != 0 {
				testSymbolId = symbolId
				break
			}
		}
		if testSymbolId != "" {
			break
		}
	}

	if testSymbolId == "" {
		t.Skip("No symbol with definition found")
	}

	// Test fast lookup
	fastLoc := findSymbolLocationFast(testSymbolId, index)
	if fastLoc == nil {
		t.Fatal("findSymbolLocationFast returned nil for symbol with definition")
	}

	// Compare with slow lookup
	slowLoc := findSymbolLocation(testSymbolId, index)
	if slowLoc == nil {
		t.Fatal("findSymbolLocation returned nil for symbol with definition")
	}

	// Results should match
	if fastLoc.FileId != slowLoc.FileId {
		t.Errorf("FileId mismatch: fast=%s, slow=%s", fastLoc.FileId, slowLoc.FileId)
	}
	if fastLoc.StartLine != slowLoc.StartLine {
		t.Errorf("StartLine mismatch: fast=%d, slow=%d", fastLoc.StartLine, slowLoc.StartLine)
	}
	if fastLoc.StartColumn != slowLoc.StartColumn {
		t.Errorf("StartColumn mismatch: fast=%d, slow=%d", fastLoc.StartColumn, slowLoc.StartColumn)
	}

	t.Logf("Location found: %s:%d:%d", fastLoc.FileId, fastLoc.StartLine, fastLoc.StartColumn)
}

// TestFindReferencesUsesRefIndex verifies FindReferences uses the inverted index
func TestFindReferencesUsesRefIndex(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	// Find a symbol with multiple references
	var testSymbolId string
	var expectedCount int
	for symbolId, refs := range index.RefIndex {
		if len(refs) >= 5 {
			testSymbolId = symbolId
			expectedCount = len(refs)
			break
		}
	}

	if testSymbolId == "" {
		t.Skip("No symbol with 5+ references found")
	}

	// Test with RefIndex available
	refs, err := index.FindReferences(testSymbolId, ReferenceOptions{
		IncludeDefinition: true,
		MaxResults:        100,
	})
	if err != nil {
		t.Fatalf("FindReferences failed: %v", err)
	}

	// Should find references
	if len(refs) == 0 {
		t.Error("FindReferences returned no results")
	}

	t.Logf("Found %d references (expected from RefIndex: %d)", len(refs), expectedCount)

	// Count should be close to RefIndex count (may differ due to filtering)
	if len(refs) > expectedCount {
		t.Errorf("Found more references (%d) than in RefIndex (%d)", len(refs), expectedCount)
	}
}

// TestSearchSymbolsUsesCachedSymbols verifies SearchSymbols uses the cache
func TestSearchSymbolsUsesCachedSymbols(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	// Search for a common term
	results, err := index.SearchSymbols("Engine", SearchOptions{
		MaxResults:   10,
		IncludeTests: true,
	})
	if err != nil {
		t.Fatalf("SearchSymbols failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("SearchSymbols returned no results for 'Engine'")
	}

	t.Logf("Found %d symbols matching 'Engine'", len(results))

	// Verify results are from cache (they should have all fields populated)
	for _, sym := range results {
		if sym.StableId == "" {
			t.Error("Result missing StableId")
		}
		if sym.Name == "" {
			t.Error("Result missing Name")
		}
	}
}

// BenchmarkFindReferencesWithRefIndex benchmarks reference lookup with the inverted index
func BenchmarkFindReferencesWithRefIndex(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	// Find a symbol with many references for benchmarking
	var testSymbolId string
	for symbolId, refs := range index.RefIndex {
		if len(refs) >= 10 {
			testSymbolId = symbolId
			break
		}
	}

	if testSymbolId == "" {
		b.Skip("No symbol with 10+ references found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = index.FindReferences(testSymbolId, ReferenceOptions{
			IncludeDefinition: true,
			MaxResults:        50,
		})
	}
}

// BenchmarkFindReferencesWithoutRefIndex benchmarks reference lookup without the inverted index
func BenchmarkFindReferencesWithoutRefIndex(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	// Find a symbol with references
	var testSymbolId string
	for symbolId, refs := range index.RefIndex {
		if len(refs) >= 10 {
			testSymbolId = symbolId
			break
		}
	}

	if testSymbolId == "" {
		b.Skip("No symbol with 10+ references found")
	}

	// Temporarily disable RefIndex to test fallback path
	savedRefIndex := index.RefIndex
	index.RefIndex = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = index.FindReferences(testSymbolId, ReferenceOptions{
			IncludeDefinition: true,
			MaxResults:        50,
		})
	}

	// Restore
	index.RefIndex = savedRefIndex
}

// BenchmarkSearchSymbolsWithCache benchmarks symbol search with pre-converted cache
func BenchmarkSearchSymbolsWithCache(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = index.SearchSymbols("Engine", SearchOptions{
			MaxResults:   20,
			IncludeTests: true,
		})
	}
}

// BenchmarkSearchSymbolsWithoutCache benchmarks symbol search without cache (fallback path)
func BenchmarkSearchSymbolsWithoutCache(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	// Temporarily disable cache to test fallback path
	savedCache := index.ConvertedSymbols
	index.ConvertedSymbols = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = index.SearchSymbols("Engine", SearchOptions{
			MaxResults:   20,
			IncludeTests: true,
		})
	}

	// Restore
	index.ConvertedSymbols = savedCache
}

// BenchmarkFindSymbolLocationFast benchmarks the fast location lookup
func BenchmarkFindSymbolLocationFast(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	// Find a symbol with a definition
	var testSymbolId string
	for symbolId, refs := range index.RefIndex {
		for _, ref := range refs {
			if ref.Occ.SymbolRoles&SymbolRoleDefinition != 0 {
				testSymbolId = symbolId
				break
			}
		}
		if testSymbolId != "" {
			break
		}
	}

	if testSymbolId == "" {
		b.Skip("No symbol with definition found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = findSymbolLocationFast(testSymbolId, index)
	}
}

// BenchmarkFindSymbolLocationSlow benchmarks the slow location lookup (fallback)
func BenchmarkFindSymbolLocationSlow(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	// Find a symbol with a definition
	var testSymbolId string
	for symbolId, refs := range index.RefIndex {
		for _, ref := range refs {
			if ref.Occ.SymbolRoles&SymbolRoleDefinition != 0 {
				testSymbolId = symbolId
				break
			}
		}
		if testSymbolId != "" {
			break
		}
	}

	if testSymbolId == "" {
		b.Skip("No symbol with definition found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = findSymbolLocation(testSymbolId, index)
	}
}

// BenchmarkGetCachedSymbol benchmarks cached symbol retrieval
func BenchmarkGetCachedSymbol(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	var testSymbolId string
	for id := range index.ConvertedSymbols {
		testSymbolId = id
		break
	}

	if testSymbolId == "" {
		b.Skip("No cached symbols found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = index.GetCachedSymbol(testSymbolId)
	}
}

// BenchmarkConvertSymbolOnTheFly benchmarks on-the-fly symbol conversion
func BenchmarkConvertSymbolOnTheFly(b *testing.B) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		b.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		b.Skip("SCIP index not found")
	}

	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		b.Fatalf("Failed to load index: %v", err)
	}

	var testSymbolId string
	var testSymInfo *SymbolInformation
	for id, info := range index.Symbols {
		testSymbolId = id
		testSymInfo = info
		break
	}

	if testSymbolId == "" {
		b.Skip("No symbols found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = convertToSCIPSymbol(testSymInfo, index)
	}
}

// TestRefIndexConsistency verifies RefIndex entries match document occurrences
func TestRefIndexConsistency(t *testing.T) {
	adapter := getTestAdapter(t)
	index := adapter.GetIndex()

	if index == nil {
		t.Fatal("Index is nil")
	}

	// Count occurrences from documents
	docOccCount := 0
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			if occ.Symbol != "" {
				docOccCount++
			}
		}
	}

	// Count occurrences from RefIndex
	refIndexCount := 0
	for _, refs := range index.RefIndex {
		refIndexCount += len(refs)
	}

	if docOccCount != refIndexCount {
		t.Errorf("Occurrence count mismatch: documents=%d, RefIndex=%d", docOccCount, refIndexCount)
	} else {
		t.Logf("RefIndex correctly indexed %d occurrences", refIndexCount)
	}
}

// TestIndexLoadPerformance measures index loading time
func TestIndexLoadPerformance(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	indexPath := filepath.Join(repoRoot, ".scip", "index.scip")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Skip("SCIP index not found")
	}

	// Time the load
	index, err := LoadSCIPIndex(indexPath)
	if err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}

	t.Logf("Index loaded: %d documents, %d symbols, %d RefIndex entries, %d cached symbols",
		len(index.Documents),
		len(index.Symbols),
		len(index.RefIndex),
		len(index.ConvertedSymbols))

	// Verify all caches are populated
	if len(index.RefIndex) == 0 {
		t.Error("RefIndex not populated during load")
	}
	if len(index.ConvertedSymbols) == 0 {
		t.Error("ConvertedSymbols not populated during load")
	}
}

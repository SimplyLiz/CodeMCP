package query

import (
	"testing"

	"ckb/internal/backends/git"
)

// =============================================================================
// v5.2 Performance Targets
// =============================================================================
// Cheap tools: P95 < 300ms
// Heavy tools: P95 < 2000ms
//
// These benchmarks test helper functions and classification logic.
// Full tool benchmarks require engine setup (see engine_bench_test.go).
// =============================================================================

// =============================================================================
// Phase 2: summarizeDiff Benchmarks
// =============================================================================

func BenchmarkClassifyFileRiskLevel(b *testing.B) {
	stat := git.DiffStats{
		FilePath:  "internal/query/engine.go",
		Additions: 50,
		Deletions: 30,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyFileRiskLevel(stat, "core")
	}
}

func BenchmarkSuggestTestPath(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		suggestTestPath("internal/query/engine.go", "go")
	}
}

func BenchmarkBuildDiffSummary(b *testing.B) {
	files := make([]DiffFileChange, 20)
	for i := 0; i < 20; i++ {
		files[i] = DiffFileChange{
			FilePath:   "file.go",
			ChangeType: "modified",
			Additions:  50,
			Deletions:  20,
		}
	}
	commits := make([]DiffCommitInfo, 10)
	for i := 0; i < 10; i++ {
		commits[i] = DiffCommitInfo{Hash: "abc123"}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildDiffSummary(files, nil, nil, commits)
	}
}

func BenchmarkComputeDiffConfidence(b *testing.B) {
	basis := []ConfidenceBasisItem{
		{Backend: "git", Status: "available"},
		{Backend: "scip", Status: "available"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeDiffConfidence(basis, nil)
	}
}

// =============================================================================
// Phase 3: getHotspots Benchmarks
// =============================================================================

func BenchmarkClassifyRecency(b *testing.B) {
	timestamp := "2024-01-15T10:30:00Z"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyRecency(timestamp)
	}
}

func BenchmarkClassifyHotspotRisk(b *testing.B) {
	churn := git.ChurnMetrics{
		ChangeCount:    25,
		AuthorCount:    3,
		AverageChanges: 15.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyHotspotRisk(churn, "core")
	}
}

// =============================================================================
// Phase 4: explainPath Benchmarks
// =============================================================================

func BenchmarkClassifyPathRole(b *testing.B) {
	paths := []string{
		"internal/query/engine.go",
		"internal/query/engine_test.go",
		"config/settings.yaml",
		"cmd/server/main.go",
		"vendor/github.com/foo/bar.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyPathRole(paths[i%len(paths)])
	}
}

func BenchmarkComputePathConfidence(b *testing.B) {
	basis := []ClassificationBasis{
		{Type: "naming", Signal: "test pattern", Confidence: 0.95},
		{Type: "location", Signal: "internal dir", Confidence: 0.85},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computePathConfidence(basis)
	}
}

// =============================================================================
// Phase 4: listKeyConcepts Benchmarks
// =============================================================================

func BenchmarkExtractConcept(b *testing.B) {
	names := []string{
		"UserService",
		"CacheManager",
		"AuthHandler",
		"ConfigProvider",
		"DatabaseClient",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractConcept(names[i%len(names)])
	}
}

func BenchmarkSplitCamelCase(b *testing.B) {
	names := []string{
		"UserService",
		"HTTPHandler",
		"XMLParser",
		"getUser",
		"SimpleWord",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		splitCamelCase(names[i%len(names)])
	}
}

func BenchmarkCategorizeConceptV52(b *testing.B) {
	concepts := []string{
		"Cache",
		"User",
		"Factory",
		"Queue",
		"Order",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		categorizeConceptV52(concepts[i%len(concepts)])
	}
}

func BenchmarkTitleCase(b *testing.B) {
	words := []string{"hello", "world", "test", "user", "cache"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		titleCase(words[i%len(words)])
	}
}

// =============================================================================
// Phase 4: recentlyRelevant Benchmarks
// =============================================================================

func BenchmarkComputeRecencyScore(b *testing.B) {
	timestamp := "2024-01-15T10:30:00Z"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeRecencyScore(timestamp)
	}
}

// =============================================================================
// Shared Helper Benchmarks
// =============================================================================

func BenchmarkClassifyFileRole(b *testing.B) {
	paths := []string{
		"internal/query/engine.go",
		"internal/query/engine_test.go",
		"config.json",
		"cmd/main.go",
		"vendor/lib.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyFileRole(paths[i%len(paths)])
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	paths := []string{
		"file.go",
		"file.ts",
		"file.py",
		"file.rs",
		"file.java",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectLanguage(paths[i%len(paths)])
	}
}

// =============================================================================
// Composite Benchmarks (simulate tool processing)
// =============================================================================

// BenchmarkDiffProcessingPipeline simulates summarizeDiff's file processing
func BenchmarkDiffProcessingPipeline(b *testing.B) {
	stats := make([]git.DiffStats, 50)
	for i := 0; i < 50; i++ {
		stats[i] = git.DiffStats{
			FilePath:  "internal/query/file.go",
			Additions: 30,
			Deletions: 10,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, stat := range stats {
			lang := detectLanguage(stat.FilePath)
			role := classifyFileRole(stat.FilePath)
			classifyFileRiskLevel(stat, role)
			suggestTestPath(stat.FilePath, lang)
		}
	}
}

// BenchmarkHotspotProcessingPipeline simulates getHotspots ranking
func BenchmarkHotspotProcessingPipeline(b *testing.B) {
	metrics := make([]git.ChurnMetrics, 50)
	for i := 0; i < 50; i++ {
		metrics[i] = git.ChurnMetrics{
			FilePath:     "internal/query/file.go",
			ChangeCount:  15,
			AuthorCount:  2,
			LastModified: "2024-01-15T10:30:00Z",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, m := range metrics {
			role := classifyFileRole(m.FilePath)
			classifyRecency(m.LastModified)
			classifyHotspotRisk(m, role)
		}
	}
}

// BenchmarkConceptExtractionPipeline simulates listKeyConcepts processing
func BenchmarkConceptExtractionPipeline(b *testing.B) {
	names := []string{
		"UserService", "CacheManager", "AuthHandler",
		"ConfigProvider", "DatabaseClient", "HTTPServer",
		"QueryEngine", "StorageAdapter", "ErrorHandler",
		"LogManager",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range names {
			concept := extractConcept(name)
			if concept != "" {
				categorizeConceptV52(concept)
			}
		}
	}
}

// BenchmarkPathClassificationPipeline simulates explainPath processing
func BenchmarkPathClassificationPipeline(b *testing.B) {
	paths := []string{
		"internal/query/engine.go",
		"internal/query/engine_test.go",
		"config/settings.yaml",
		"cmd/server/main.go",
		"vendor/github.com/foo/bar.go",
		"docs/README.md",
		"internal/api/handler.go",
		"legacy/old_code.go",
		"src/index.ts",
		"pkg/utils/helper.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			_, _, basis := classifyPathRole(path)
			computePathConfidence(basis)
		}
	}
}

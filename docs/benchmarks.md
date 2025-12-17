# CKB Performance Benchmarks

Performance benchmarks for v5.2 navigation tools, measured against spec latency targets.

## v5.2 Latency Targets

| Budget | Target | Tools |
|--------|--------|-------|
| Cheap | P95 < 300ms | searchSymbols, explainFile, listEntrypoints, explainPath, getSymbol, explainSymbol |
| Heavy | P95 < 2000ms | traceUsage, getArchitecture, getHotspots, summarizeDiff, recentlyRelevant, listKeyConcepts, analyzeImpact, getCallGraph, findReferences, justifySymbol |

## Benchmark Results

**Environment:** Apple M4 Pro, Go 1.23, macOS

### Helper Function Benchmarks

These measure the in-memory processing logic (excludes I/O).

| Function | Time | Allocs | Description |
|----------|------|--------|-------------|
| `classifyFileRiskLevel` | 1.0 ns | 0 | Risk classification for diff files |
| `classifyHotspotRisk` | 0.77 ns | 0 | Churn-based risk assessment |
| `computeDiffConfidence` | 2.2 ns | 0 | Confidence calculation |
| `computePathConfidence` | 1.0 ns | 0 | Path confidence from basis |
| `detectLanguage` | 7.3 ns | 0 | Language from file extension |
| `suggestTestPath` | 19 ns | 1 | Test file path generation |
| `titleCase` | 29 ns | 1 | Simple title casing |
| `classifyRecency` | 43 ns | 0 | Timestamp recency classification |
| `computeRecencyScore` | 44 ns | 0 | Recency scoring |
| `classifyFileRole` | 78 ns | 0 | File role from path patterns |
| `splitCamelCase` | 116 ns | 5 | CamelCase word splitting |
| `classifyPathRole` | 297 ns | 1 | Full path role classification |
| `categorizeConceptV52` | 561 ns | 5 | Concept categorization |
| `buildDiffSummary` | 674 ns | 24 | Diff summary text generation |
| `extractConcept` | 903 ns | 14 | Concept extraction from names |

### Pipeline Benchmarks

Simulated tool processing pipelines (multiple items, no I/O).

| Pipeline | Items | Time | Budget | Headroom |
|----------|-------|------|--------|----------|
| PathClassification | 10 paths | 3.0 µs | 300ms | 99.999% |
| DiffProcessing | 50 files | 8.9 µs | 2000ms | 99.999% |
| HotspotProcessing | 50 items | 10.1 µs | 2000ms | 99.999% |
| ConceptExtraction | 10 names | 14.9 µs | 2000ms | 99.999% |

### Analysis

**In-memory processing is negligible** - all helper functions complete in nanoseconds to microseconds, leaving >99% of the latency budget for I/O operations:

- Git commands (commit history, diff stats, churn metrics)
- SCIP index queries (symbol search, references, call graph)
- File system reads (file contents, directory walks)

**Bottleneck identification:** Real-world latency is dominated by:
1. SCIP index lookups (especially for large codebases)
2. Git history queries (especially for repositories with long history)
3. File system operations (directory traversal in listKeyConcepts fallback)

## Running Benchmarks

```bash
# Run all benchmarks
go test ./internal/query/... -bench=. -benchmem -run=^$

# Run specific benchmark
go test ./internal/query/... -bench=BenchmarkClassifyPathRole -benchmem -run=^$

# Run with CPU profiling
go test ./internal/query/... -bench=BenchmarkDiffProcessingPipeline -cpuprofile=cpu.prof -run=^$
```

## Historical Results

| Date | Commit | Environment | Notes |
|------|--------|-------------|-------|
| 2024-12-17 | Initial | Apple M4 Pro | Baseline benchmarks |

# CKB Performance Benchmarks

Performance benchmarks for CKB tools, measured against spec latency targets.

## Latency Targets

### v5.2 Navigation Tools

| Budget | Target | Tools |
|--------|--------|-------|
| Cheap | P95 < 300ms | searchSymbols, explainFile, listEntrypoints, explainPath, getSymbol, explainSymbol |
| Heavy | P95 < 2000ms | traceUsage, getArchitecture, getHotspots, summarizeDiff, recentlyRelevant, listKeyConcepts, analyzeImpact, getCallGraph, findReferences, justifySymbol |

### v6.0 Architectural Memory Tools

| Budget | Target | Tools |
|--------|--------|-------|
| Cheap | P95 < 300ms | getModuleResponsibilities, getOwnership, recordDecision, getDecisions, annotateModule |
| Heavy | P95 < 2000ms | getArchitecture, getHotspots |
| Heavy | P95 < 30000ms | refreshArchitecture |

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

### v6.0 Hotspot Benchmarks

| Function | Time | Allocs | Description |
|----------|------|--------|-------------|
| `CalculateInstability` | 0.25 ns | 0 | Martin's instability metric |
| `ComputeCompositeScore` | 0.26 ns | 0 | Weighted hotspot score |
| `NormalizeChurnScore` | 0.25 ns | 0 | Churn normalization |
| `NormalizeCouplingScore` | 0.25 ns | 0 | Coupling normalization |
| `NormalizeComplexityScore` | 0.26 ns | 0 | Complexity normalization |
| `CalculateTrend` | 295 ns | 1 | Trend analysis (30 snapshots) |

| Pipeline | Items | Time | Budget | Headroom |
|----------|-------|------|--------|----------|
| HotspotScoring | 100 files | 69 ns | 2000ms | 99.999% |
| TrendAnalysis | 50 files × 10 snapshots | 5.7 µs | 2000ms | 99.999% |

### v6.0 Ownership Benchmarks

| Function | Time | Allocs | Description |
|----------|------|--------|-------------|
| `normalizeAuthorKey` | 10 ns | 0 | Author key normalization |
| `BlameOwnershipToOwners` | 47 ns | 1 | Convert blame to owners |
| `CodeownersToOwners` | 56 ns | 1 | Convert CODEOWNERS to owners |
| `isBot` | 743 ns | 0 | Bot detection (regex) |
| `matchPattern` | 1.9 µs | 59 | Glob pattern matching |
| `GetOwnersForPath` | 51 µs | 1580 | Resolve owners for path |

| Pipeline | Items | Time | Budget | Headroom |
|----------|-------|------|--------|----------|
| OwnershipResolution | 100 files × 50 rules | 9.2 ms | 300ms | 96.9% |

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
# Run all query benchmarks
go test ./internal/query/... -bench=. -benchmem -run=^$

# Run all v6.0 benchmarks
go test ./internal/hotspots/... ./internal/ownership/... -bench=. -benchmem -run=^$

# Run specific benchmark
go test ./internal/query/... -bench=BenchmarkClassifyPathRole -benchmem -run=^$

# Run with CPU profiling
go test ./internal/query/... -bench=BenchmarkDiffProcessingPipeline -cpuprofile=cpu.prof -run=^$
```

## Historical Results

| Date | Commit | Environment | Notes |
|------|--------|-------------|-------|
| 2024-12-17 | Initial | Apple M4 Pro | v5.2 baseline benchmarks |
| 2024-12-18 | Phase 5 | Apple M4 Pro | v6.0 hotspot and ownership benchmarks |

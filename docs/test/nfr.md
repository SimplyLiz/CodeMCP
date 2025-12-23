# NFR Benchmark Suite

Non-Functional Requirements (NFR) test infrastructure for CKB performance validation.

## Quick Reference

```bash
# Run deterministic NFR tests (CI gate, always works)
go test -v -run TestNFRScenarios ./internal/mcp/...

# Run all benchmarks with memory stats
go test -bench=. -benchmem ./...

# Run MCP benchmarks with latency tracking
go test -bench=BenchmarkWideResult ./internal/mcp/... -count=5

# Compare before/after with benchstat
go test -bench=. ./internal/mcp/... -count=5 > before.txt
# ... make changes ...
go test -bench=. ./internal/mcp/... -count=5 > after.txt
benchstat before.txt after.txt
```

---

## Deterministic NFR Tests (CI Gates)

### TestNFRScenarios (`internal/mcp/wide_result_budget_test.go`)

These tests use synthetic fixtures and always run (no SCIP index required). They fail CI on >10% token budget regression.

| Scenario | Tool | Tier | Baseline (bytes) | Fixture Size |
|----------|------|------|------------------|--------------|
| searchSymbols_small | searchSymbols | small | 3,600 | 20 symbols |
| searchSymbols_medium | searchSymbols | medium | 18,000 | 100 symbols |
| searchSymbols_large | searchSymbols | large | 91,000 | 500 symbols |
| findReferences_small | findReferences | small | 4,500 | 50 refs |
| findReferences_medium | findReferences | medium | 45,000 | 500 refs |
| findReferences_large | findReferences | large | 450,000 | 5,000 refs |
| getCallGraph_shallow | getCallGraph | shallow | 900 | depth=2, branching=3 |
| getCallGraph_deep | getCallGraph | deep | 16,000 | depth=4, branching=5 |
| getHotspots_small | getHotspots | small | 900 | 10 hotspots |
| getHotspots_large | getHotspots | large | 17,000 | 200 hotspots |
| analyzeImpact_small | analyzeImpact | small | 2,000 | 10 impact nodes |
| analyzeImpact_large | analyzeImpact | large | 18,000 | 100 impact nodes |
| getArchitecture_small | getArchitecture | small | 1,500 | 5 modules |
| getArchitecture_large | getArchitecture | large | 8,000 | 30 modules |
| traceUsage_small | traceUsage | small | 800 | 5 paths |
| traceUsage_large | traceUsage | large | 7,800 | 50 paths |

### Token Baselines (`nfrTokenBaselines`)

```go
var nfrTokenBaselines = map[string]map[string]int{
    "searchSymbols":    {"small": 3600, "medium": 18000, "large": 91000},
    "findReferences":   {"small": 4500, "medium": 45000, "large": 450000},
    "getCallGraph":     {"shallow": 900, "deep": 16000},
    "getHotspots":      {"small": 900, "large": 17000},
    "analyzeImpact":    {"small": 2000, "large": 18000},
    "getArchitecture":  {"small": 1500, "large": 8000},
    "traceUsage":       {"small": 800, "large": 7800},
}
```

### Synthetic Fixtures (`internal/mcp/testdata/fixtures.go`)

Fixture generators for deterministic testing:

| Generator | Output | Usage |
|-----------|--------|-------|
| `GenerateSymbols(n)` | `[]SymbolFixture` | searchSymbols scenarios |
| `GenerateReferences(n)` | `[]ReferenceFixture` | findReferences scenarios |
| `GenerateHotspots(n)` | `[]HotspotFixture` | getHotspots scenarios |
| `GenerateCallGraph(root, depth, branching)` | `[]CallGraphNodeFixture` | getCallGraph scenarios |
| `GenerateImpactNodes(n, maxDepth)` | `[]ImpactNodeFixture` | analyzeImpact scenarios |
| `GenerateModules(n)` | `[]ModuleFixture` | getArchitecture scenarios |
| `GenerateUsagePaths(n, maxDepth)` | `[]UsagePathFixture` | traceUsage scenarios |

Preset fixture sets:
- `SmallFixtures()` - 20 symbols, 50 refs, 10 hotspots, 10 impact nodes, 5 modules, 5 paths
- `MediumFixtures()` - 100 symbols, 500 refs, 50 hotspots, 40 impact nodes, 15 modules, 20 paths
- `LargeFixtures()` - 500 symbols, 5000 refs, 200 hotspots, 100 impact nodes, 30 modules, 50 paths

---

## Design Decisions

| Metric | CI Gate? | Tracking Method | Rationale |
|--------|----------|-----------------|-----------|
| Bytes/tokens | ✅ Yes | `TestNFRScenarios` | Deterministic, reproducible |
| Latency | ❌ No | `b.ReportMetric` in benchmarks | Too flaky for CI (shared runners, cold caches) |

**Why not latency as CI gate?**
- CI runners have variable performance
- Cold cache vs warm cache differences
- Network/disk I/O variance
- Better to track trends via `benchstat` than hard-fail

---

## Integration Tests (Require SCIP Index)

### TestWideResultTokenBudgetsIntegration (`internal/mcp/wide_result_budget_test.go`)

Tests with real SCIP index data. Skipped if no index available.

| Test | Budget (bytes) | Purpose |
|------|----------------|---------|
| getCallGraph | 15,000 | Call graph with real symbols |
| getHotspots | 10,000 | Real file hotspot data |
| findReferences | 12,000 | Real reference lookups |
| analyzeImpact | 16,000 | Real impact analysis |

---

## Benchmark Catalog

### SCIP Backend (`internal/backends/scip/performance_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkNavigation/FindDefinition | Symbol definition lookup |
| BenchmarkNavigation/FindReferences | Reference discovery |
| BenchmarkNavigation/GetHover | Hover info retrieval |
| BenchmarkSymbolSearch/ExactMatch | Exact symbol search |
| BenchmarkSymbolSearch/PrefixMatch | Prefix-based search |
| BenchmarkSymbolSearch/FuzzyMatch | Fuzzy symbol matching |
| BenchmarkSymbolSearch/WithKindFilter | Filtered symbol search |
| BenchmarkConcurrentAccess | Concurrent read operations |
| BenchmarkIndexLoading | Index load performance |
| BenchmarkMemoryUsage | Memory consumption tracking |
| BenchmarkParallelSymbolSearch | Parallel search throughput |

### MCP Token Budget (`internal/mcp/token_budget_test.go`, `token_budget_bench_test.go`)

| Test/Benchmark | Purpose |
|----------------|---------|
| TestTokenBudgetEnforcement | Budget limit enforcement |
| TestTokenBudgetPrioritization | Priority-based truncation |
| TestTokenBudgetWithNestedStructures | Complex structure handling |
| BenchmarkTokenCounting | Token counting speed |
| BenchmarkBudgetEnforcement | Enforcement overhead |
| BenchmarkPrioritizedTruncation | Truncation algorithm |

### Wide-Result Benchmarks (`internal/mcp/wide_result_bench_test.go`)

| Benchmark | Metrics Reported | Purpose |
|-----------|------------------|---------|
| BenchmarkWideResultSize/getCallGraph_depth1 | bytes/op, est_tokens/op, latency_ms/op | Shallow call graph |
| BenchmarkWideResultSize/getCallGraph_depth2 | bytes/op, est_tokens/op, latency_ms/op | Deep call graph |
| BenchmarkWideResultSize/findReferences_limit50 | bytes/op, est_tokens/op, latency_ms/op | Small ref set |
| BenchmarkWideResultSize/findReferences_limit100 | bytes/op, est_tokens/op, latency_ms/op | Large ref set |
| BenchmarkWideResultSize/analyzeImpact_depth2 | bytes/op, est_tokens/op, latency_ms/op | Impact analysis |
| BenchmarkWideResultSize/getHotspots_limit20 | bytes/op, est_tokens/op, latency_ms/op | Small hotspot set |
| BenchmarkWideResultSize/getHotspots_limit50 | bytes/op, est_tokens/op, latency_ms/op | Large hotspot set |
| BenchmarkWideResultSize/searchSymbols_limit20 | bytes/op, est_tokens/op, latency_ms/op | Small symbol search |
| BenchmarkWideResultSize/searchSymbols_limit50 | bytes/op, est_tokens/op, latency_ms/op | Large symbol search |
| BenchmarkWideResultSize/getArchitecture_depth2 | bytes/op, est_tokens/op, latency_ms/op | Architecture retrieval |
| BenchmarkWideResultWithFrontier/getCallGraph_normal | bytes/op, est_tokens/op, latency_ms/op | Baseline for frontier comparison |

### Wide-Result Metrics (`internal/mcp/wide_result_metrics_test.go`)

| Test | Purpose |
|------|---------|
| TestMetricsRecording | Metrics capture accuracy |
| TestMetricsAggregation | Aggregation correctness |
| TestMetricsPersistence | SQLite persistence |
| TestMetricsExport | JSON export format |

### Ownership Analysis (`internal/ownership/ownership_bench_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkCodeownersMatch/SimplePattern | Simple pattern matching |
| BenchmarkCodeownersMatch/GlobPattern | Glob pattern matching |
| BenchmarkCodeownersMatch/DeepPath | Deep path resolution |
| BenchmarkBlameAnalysis/SmallFile | Small file blame |
| BenchmarkBlameAnalysis/LargeFile | Large file blame |
| BenchmarkOwnershipMerge | Owner merge algorithm |
| BenchmarkOwnershipCache | Cache efficiency |

### Query Navigation (`internal/query/navigation_bench_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkFindDefinition | Definition lookup |
| BenchmarkFindReferences/Small | Small reference sets |
| BenchmarkFindReferences/Large | Large reference sets |
| BenchmarkGetCallGraph/Depth1 | Shallow call graphs |
| BenchmarkGetCallGraph/Depth3 | Deep call graphs |
| BenchmarkTraceUsage | Usage path tracing |
| BenchmarkAnalyzeImpact | Impact analysis |
| BenchmarkSearchSymbols/Exact | Exact search |
| BenchmarkSearchSymbols/Fuzzy | Fuzzy search |
| BenchmarkSearchSymbols/Filtered | Filtered search |
| BenchmarkGetArchitecture | Architecture retrieval |
| BenchmarkGetModuleOverview | Module overview |
| BenchmarkExplainSymbol | Symbol explanation |
| BenchmarkExplainFile | File explanation |
| BenchmarkListKeyConcepts | Concept extraction |
| BenchmarkGetHotspots | Hotspot calculation |

### Query Extended (`internal/query/query_extended_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkComplexQuery | Complex query handling |
| BenchmarkQueryCaching | Query cache efficiency |
| BenchmarkParallelQueries | Concurrent query load |

### Hotspots (`internal/hotspots/persistence_bench_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkHotspotSave | Save performance |
| BenchmarkHotspotLoad | Load performance |
| BenchmarkHotspotQuery/ByChurn | Churn-based query |
| BenchmarkHotspotQuery/ByRecency | Recency-based query |
| BenchmarkHotspotQuery/ByScore | Score-based query |
| BenchmarkHotspotBatchInsert | Batch insertion |
| BenchmarkHotspotIncremental | Incremental update |
| BenchmarkHotspotPruning | Data pruning |

### Complexity Analysis (`internal/complexity/analyzer_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkCyclomaticComplexity | Cyclomatic calculation |
| BenchmarkCognitiveComplexity | Cognitive calculation |
| BenchmarkFileComplexity/Small | Small file analysis |
| BenchmarkFileComplexity/Large | Large file analysis |
| BenchmarkParseAndAnalyze | Full parse+analyze |
| BenchmarkTreeSitterParsing | Parser performance |

### Storage/Cache (`internal/storage/negative_cache_test.go`)

| Benchmark | Purpose |
|-----------|---------|
| BenchmarkNegativeCacheHit | Cache hit path |
| BenchmarkNegativeCacheMiss | Cache miss path |

### Miscellaneous

| Location | Benchmark | Purpose |
|----------|-----------|---------|
| `internal/backends/limiter_test.go` | BenchmarkRateLimiter | Rate limit overhead |
| `internal/graph/ppr_test.go` | BenchmarkPPR | PageRank computation |

---

## Summary

**Total:** ~70 benchmarks + 16 deterministic NFR scenarios across 15 test files

| Category | Files | Tests/Benchmarks |
|----------|-------|------------------|
| NFR Scenarios (CI gate) | 1 | 16 |
| MCP Token/Wide-Result | 4 | ~15 |
| SCIP Backend | 1 | 11 |
| Query Navigation | 2 | ~19 |
| Ownership | 1 | 7 |
| Hotspots | 1 | 8 |
| Complexity | 1 | 6 |
| Storage/Cache | 1 | 2 |
| Misc | 2 | 2 |

Coverage areas:
- **Index performance** - SCIP backend operations
- **Token optimization** - MCP budgets, wide-result handling
- **Query latency** - Navigation, search, call graphs
- **Persistence** - Hotspots, metrics, cache
- **Code analysis** - Complexity, ownership

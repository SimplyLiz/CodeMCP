# Unit Test Implementation Working Document

**Status:** In Progress
**Goal:** Increase test coverage from 34.7% to 50%+
**Estimated Tests:** 115
**Expected Coverage Gain:** 20-27%

---

## Target Files

| Priority | File | Lines | Functions | Current Tests | Gap |
|----------|------|-------|-----------|---------------|-----|
| 1 | `internal/query/symbols.go` | 925 | 15 | Helper functions only | GetSymbol, SearchSymbols, FindReferences |
| 2 | `internal/query/navigation.go` | 3,774 | 52 | Helpers + benchmarks | ExplainSymbol, GetCallGraph, TraceUsage, etc. |
| 3 | `internal/mcp/tool_impls.go` | 1,782 | 33 | None dedicated | All 33 tool implementations |

---

## Test Patterns

### Table-Driven Tests with Parallel Execution

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name        string
        input       InputType
        expected    OutputType
        expectError bool
    }{
        {"case 1", input1, expected1, false},
        {"case 2", input2, expected2, true},
    }
    for _, tt := range tests {
        tt := tt // capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            // test body
        })
    }
}
```

### Setup/Teardown Pattern

```go
func testEngine(t *testing.T) (*Engine, func()) {
    t.Helper()
    tmpDir, _ := os.MkdirTemp("", "ckb-test-*")
    // setup...
    cleanup := func() {
        os.RemoveAll(tmpDir)
    }
    return engine, cleanup
}
```

---

## Phase 1: Test Infrastructure & symbols.go

**Status:** [x] In Progress (17 tests added)
**File to Create:** `internal/query/test_helpers_test.go`
**File to Modify:** `internal/query/symbols_test.go`
**Tests:** 25 (17 completed)
**Coverage Gain:** query package at 28.7%

### Task 1.1: Create Mock Infrastructure

**File:** `internal/query/test_helpers_test.go`

- [x] Create `mockSCIPAdapter` struct
  - Configurable `symbolResult`, `searchResult`, `refsResult`
  - Error injection via `err` field
  - Call tracking via `callCounts` map
- [ ] Create `mockResolver` struct (deferred - using real engine)
  - Configurable `resolved` symbol
  - Error injection
- [x] Create `mockQueryCache` struct
  - Get/Set methods
  - Hit/miss tracking
- [ ] Create `mockTreesitterExtractor` struct (deferred)

### Task 1.2: GetSymbol Tests

**File:** `internal/query/symbols_test.go`

| Status | Test Name | Description | Lines Covered |
|--------|-----------|-------------|---------------|
| [ ] | `TestGetSymbol_ValidSymbolId` | Happy path - returns symbol when found | 82-246 |
| [x] | `TestGetSymbol_DefaultRepoStateMode` | Defaults to "head" when empty | 82-84 |
| [ ] | `TestGetSymbol_SCIPFallback_RawId` | Falls back to SCIP for raw IDs | 97-142 |
| [x] | `TestGetSymbol_SymbolNotFound` | Returns error with drilldowns | 144-156 |
| [ ] | `TestGetSymbol_DeletedSymbol` | Returns deleted response | 159-166 |
| [ ] | `TestGetSymbol_RedirectedSymbol` | Returns redirect metadata | 169-173 |
| [ ] | `TestGetSymbol_IdentityFallback` | Uses identity when SCIP unavailable | 222-246 |
| [x] | `TestGetSymbol_ProvenanceBuilding` | Provenance includes backends | 249 |

### Task 1.3: SearchSymbols Tests

**File:** `internal/query/symbols_test.go`

| Status | Test Name | Description | Lines Covered |
|--------|-----------|-------------|---------------|
| [x] | `TestSearchSymbols_DefaultLimit` | Uses default limit of 20 | 334-337 |
| [ ] | `TestSearchSymbols_CacheHit` | Returns cached response | 347-367 |
| [ ] | `TestSearchSymbols_CacheMiss` | Tracks cache miss stats | 363-367 |
| [ ] | `TestSearchSymbols_FTSFirst` | Uses FTS5 before SCIP | 374-424 |
| [ ] | `TestSearchSymbols_SCIPFallback` | Uses SCIP when FTS empty | 427-467 |
| [ ] | `TestSearchSymbols_TreesitterFallback` | Uses treesitter when SCIP unavailable | 468-486 |
| [x] | `TestSearchSymbols_WithKinds` | Filters by specified kinds | 378-390 |
| [x] | `TestSearchSymbols_WithScope` | Filters by scope prefix | 391-393 |
| [x] | `TestSearchSymbols_TruncationInfo` | Sets truncation when exceeding limit | 518-527 |
| [ ] | `TestSearchSymbols_PPRReranking` | Applies PPR re-ranking | 508-515 |
| [ ] | `TestSearchSymbols_CacheStore` | Stores response in cache | 553-561 |
| [x] | `TestSearchSymbols_EmptyQuery` | Handles empty query | - |
| [x] | `TestSearchSymbols_ProvenanceBuilding` | Builds provenance correctly | - |

### Task 1.4: FindReferences Tests

**File:** `internal/query/symbols_test.go`

| Status | Test Name | Description | Lines Covered |
|--------|-----------|-------------|---------------|
| [x] | `TestFindReferences_DefaultLimit` | Uses default limit of 100 | 676-678 |
| [ ] | `TestFindReferences_ResolvedSymbolId` | Uses resolved ID from resolver | 686-695 |
| [ ] | `TestFindReferences_RawSymbolIdFallback` | Falls back to raw ID | 692-694 |
| [x] | `TestFindReferences_SymbolNotFound` | Returns error when not found | 738-744 |
| [ ] | `TestFindReferences_Deduplication` | Removes duplicate references | 747 |
| [ ] | `TestFindReferences_Sorting` | Sorts by file, line, column | 750 |
| [x] | `TestFindReferences_ProvenanceBuilding` | Builds provenance correctly | - |
| [x] | `TestFindReferences_WithScope` | Filters by scope | - |
| [x] | `TestFindReferences_IncludeTests` | Handles include tests flag | - |

---

## Phase 2: navigation.go Tests

**Status:** [ ] Not Started
**File to Create:** `internal/query/navigation_extended_test.go`
**Tests:** 35
**Coverage Gain:** +8-10%

### Task 2.1: ExplainSymbol Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestExplainSymbol_BuildsFactsFromGetSymbol` | Extracts symbol info correctly |
| [ ] | `TestExplainSymbol_CollectsCallees` | Populates callees from SCIP |
| [ ] | `TestExplainSymbol_CollectsCallers` | Builds callers from references |
| [ ] | `TestExplainSymbol_GitHistory` | Populates history from git |
| [ ] | `TestExplainSymbol_AnnotationContext` | Includes ADR annotations |
| [ ] | `TestExplainSymbol_SummaryBuilding` | Builds complete summary |

### Task 2.2: GetCallGraph Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestGetCallGraph_DefaultDepth` | Defaults to depth 1 |
| [ ] | `TestGetCallGraph_DepthCapping` | Caps depth at 4 |
| [ ] | `TestGetCallGraph_DefaultDirection` | Defaults to "both" |
| [ ] | `TestGetCallGraph_RootNode` | Always includes root node |
| [ ] | `TestGetCallGraph_CallerDirection` | Callers only mode |
| [ ] | `TestGetCallGraph_CalleeDirection` | Callees only mode |
| [ ] | `TestGetCallGraph_TransitiveNodes` | Adds nodes for deeper levels |
| [ ] | `TestGetCallGraph_FallbackToReferences` | Uses refs when SCIP unavailable |
| [ ] | `TestGetCallGraph_Warnings` | Adds warnings when SCIP unavailable |

### Task 2.3: TraceUsage Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestTraceUsage_DefaultMaxPaths` | Defaults to 10 paths |
| [ ] | `TestTraceUsage_DefaultMaxDepth` | Defaults to 5 depth |
| [ ] | `TestTraceUsage_PathFromEntrypoint` | Finds path from entry to target |
| [ ] | `TestTraceUsage_FallbackToCallers` | Uses direct callers as fallback |
| [ ] | `TestTraceUsage_SCIPUnavailable` | Returns limitations when unavailable |

### Task 2.4: SummarizeDiff Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestSummarizeDiff_DefaultTimeWindow` | Defaults to last 30 days |
| [ ] | `TestSummarizeDiff_CommitRangeSelector` | Handles commit range |
| [ ] | `TestSummarizeDiff_SingleCommitSelector` | Handles single commit |
| [ ] | `TestSummarizeDiff_TimeWindowSelector` | Handles time window |
| [ ] | `TestSummarizeDiff_FileCapAt50` | Caps files at 50 |
| [ ] | `TestSummarizeDiff_SymbolDetection` | Detects affected symbols |

### Task 2.5: GetHotspots Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestGetHotspots_DefaultLimit` | Defaults to 20 |
| [ ] | `TestGetHotspots_LimitCap` | Caps at 50 |
| [ ] | `TestGetHotspots_ScopeFiltering` | Filters by scope prefix |
| [ ] | `TestGetHotspots_RankingCalculation` | Applies recency multipliers |

### Task 2.6: ListEntrypoints Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestListEntrypoints_MainFunctionDetection` | Finds main in /cmd/ |
| [ ] | `TestListEntrypoints_HandlerPatterns` | Finds HTTP handlers |
| [ ] | `TestListEntrypoints_Deduplication` | Removes duplicates |
| [ ] | `TestListEntrypoints_ModuleFilter` | Filters by module |

### Task 2.7: JustifySymbol Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestJustifySymbol_Integration` | End-to-end test |

---

## Phase 3: tool_impls.go Tests

**Status:** [ ] Not Started
**File to Create:** `internal/mcp/tool_impls_test.go`
**Tests:** 40
**Coverage Gain:** +5-7%

### Task 3.1: Create Test Helper

```go
func callTool(t *testing.T, server *MCPServer, name string, params map[string]interface{}) *envelope.Response {
    t.Helper()
    // Use existing sendRequest pattern from mcp_test.go
}
```

### Task 3.2: Navigation Tool Tests (12 tests)

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolGetSymbol_MissingSymbolId` | Error when symbolId missing |
| [ ] | `TestToolGetSymbol_InvalidRepoStateMode` | Error on invalid mode |
| [ ] | `TestToolSearchSymbols_MissingQuery` | Error when query missing |
| [ ] | `TestToolSearchSymbols_InvalidKindsType` | Error on wrong type |
| [ ] | `TestToolFindReferences_MissingSymbolId` | Error when missing |
| [ ] | `TestToolGetCallGraph_MissingSymbolId` | Error when missing |
| [ ] | `TestToolGetCallGraph_InvalidDirection` | Error on invalid direction |
| [ ] | `TestToolGetCallGraph_DepthBounds` | Validates depth limits |
| [ ] | `TestToolExplainSymbol_MissingSymbolId` | Error when missing |
| [ ] | `TestToolExplainFile_MissingFilePath` | Error when missing |
| [ ] | `TestToolTraceUsage_MissingSymbolId` | Error when missing |
| [ ] | `TestToolListEntrypoints_LimitBounds` | Validates limit |

### Task 3.3: Analysis Tool Tests (10 tests)

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolAnalyzeImpact_MissingSymbolId` | Error when missing |
| [ ] | `TestToolAnalyzeImpact_InvalidTelemetryPeriod` | Error on invalid period |
| [ ] | `TestToolSummarizeDiff_SelectorValidation` | Validates selectors |
| [ ] | `TestToolSummarizePr_OptionalBranch` | Handles optional branch |
| [ ] | `TestToolGetHotspots_TimeWindowParsing` | Parses time window |
| [ ] | `TestToolGetHotspots_LimitBounds` | Validates limit |
| [ ] | `TestToolListKeyConcepts_LimitBounds` | Validates limit |
| [ ] | `TestToolGetArchitecture_DepthBounds` | Validates depth |
| [ ] | `TestToolGetModuleOverview_OptionalPath` | Handles optional path |
| [ ] | `TestToolGetFileComplexity_MissingPath` | Error when missing |

### Task 3.4: System Tool Tests (6 tests)

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolGetStatus_NoParams` | Works with no params |
| [ ] | `TestToolDoctor_NoParams` | Works with no params |
| [ ] | `TestToolExpandToolset_MissingPreset` | Error when missing |
| [ ] | `TestToolExpandToolset_ReasonMinLength` | Validates reason length |
| [ ] | `TestToolExpandToolset_InvalidPreset` | Error on invalid preset |
| [ ] | `TestToolGetWideResultMetrics_NoParams` | Works with no params |

### Task 3.5: Ownership Tool Tests (6 tests)

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolGetOwnership_MissingPath` | Error when missing |
| [ ] | `TestToolGetOwnershipDrift_ThresholdBounds` | Validates threshold |
| [ ] | `TestToolGetModuleResponsibilities_OptionalModule` | Handles optional |
| [ ] | `TestToolRecordDecision_RequiredFields` | Validates all required |
| [ ] | `TestToolGetDecisions_OptionalFilters` | Handles optional filters |
| [ ] | `TestToolAnnotateModule_ArrayParsing` | Parses array params |

### Task 3.6: Job Tool Tests (6 tests)

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolGetJobStatus_MissingJobId` | Error when missing |
| [ ] | `TestToolListJobs_StatusFiltering` | Filters by status |
| [ ] | `TestToolListJobs_TypeFiltering` | Filters by type |
| [ ] | `TestToolCancelJob_MissingJobId` | Error when missing |
| [ ] | `TestToolRefreshArchitecture_Flags` | Handles boolean flags |
| [ ] | `TestToolRefreshArchitecture_AsyncMode` | Handles async mode |

---

## Phase 4: Edge Cases & Integration

**Status:** [ ] Not Started
**Tests:** 15
**Coverage Gain:** +2-3%

### Task 4.1: Cache Behavior Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestSearchSymbols_CacheKeyDeterminism` | Same options = same key |
| [ ] | `TestSearchSymbols_CacheKeyKindOrdering` | Kinds sorted before hash |
| [ ] | `TestSearchSymbols_CacheInvalidationOnCommit` | Invalidates on new commit |

### Task 4.2: Error Propagation Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestGetSymbol_RepoStateError` | Wraps error correctly |
| [ ] | `TestFindReferences_BackendError` | Propagates SCIP errors |
| [ ] | `TestSummarizeDiff_GitUnavailable` | Returns meaningful error |
| [ ] | `TestGetHotspots_NoGitHistory` | Handles no history |
| [ ] | `TestTraceUsage_NoEntrypoints` | Handles no entrypoints |
| [ ] | `TestExplainSymbol_PartialFailure` | Continues on partial fail |

### Task 4.3: Treesitter Fallback Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestSearchWithTreesitter_DirectoryExtraction` | Extracts from dir |
| [ ] | `TestSearchWithTreesitter_KindFiltering` | Filters by kind |
| [ ] | `TestSearchWithTreesitter_SymbolIdGeneration` | Generates stable IDs |

### Task 4.4: Response Formatting Tests

| Status | Test Name | Description |
|--------|-----------|-------------|
| [ ] | `TestToolResponse_HasProvenance` | All responses have provenance |
| [ ] | `TestToolResponse_TruncationInfo` | Truncation properly signaled |
| [ ] | `TestToolResponse_DrilldownsPresent` | Drilldowns included |

---

## Files Summary

| Action | File | Phase |
|--------|------|-------|
| Create | `internal/query/test_helpers_test.go` | 1 |
| Modify | `internal/query/symbols_test.go` | 1 |
| Create | `internal/query/navigation_extended_test.go` | 2 |
| Create | `internal/mcp/tool_impls_test.go` | 3 |

---

## Progress Tracking

| Phase | Tests | Status | Coverage Before | Coverage After |
|-------|-------|--------|-----------------|----------------|
| 1 | 25 | [ ] Not Started | 34.7% | TBD |
| 2 | 35 | [ ] Not Started | TBD | TBD |
| 3 | 40 | [ ] Not Started | TBD | TBD |
| 4 | 15 | [ ] Not Started | TBD | TBD |

---

## Notes

- All tests use `t.Parallel()` for faster execution
- Follow existing patterns in `orchestrator_test.go` and `mcp_test.go`
- Use in-memory database: `storage.Open(":memory:", logger)`
- No testify - standard Go testing only

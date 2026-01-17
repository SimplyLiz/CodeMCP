# CKB v8.x Roadmap

This document consolidates the implementation plan for CKB versions 8.0 and 8.2.

**Theme:** Reliability, clarity, and compound operations for AI workflows.

---

## Version Overview

| Version | Focus | Status |
|---------|-------|--------|
| **8.0** | Foundation + Compound Operations + Streaming: reliability, error clarity, compound tools, SSE streaming | Complete |

**Key Principle:** Compound tools coexist with granular tools. Granular tools remain for specific queries; compound tools optimize AI workflows by reducing tool calls.

---

## v8.0: Foundation + Compound Operations

**Goal:** Every response is trustworthy, every error is actionable. Reduce AI tool calls by 60-70%.

### Completed

| Feature | Description | PR |
|---------|-------------|-----|
| ConfidenceFactor type | Structured explanation of confidence scores | #73 |
| CacheInfo type | Cache hit/miss transparency in responses | #73, #74 |
| Confidence wiring | `FromProvenance()` generates factors automatically | #73 |
| Cache wiring | Cache info populated when serving cached responses | #73 |
| Breaking change detection | `compareAPI` tool for API compatibility | #64 |
| Affected tests | `getAffectedTests` MCP tool for test coverage mapping | #63 |
| Static dead code | `findDeadCode` tool for unused symbol detection | #62 |
| Change impact analysis | Diff-based impact with test mapping | #55, #56 |
| Golden test suite | Multi-language fixtures for regression testing | #59 |
| Enhanced `getStatus` | Health tiers (available/degraded/unavailable), remediation, suggestions | #75 |
| `reindex` tool | Trigger index refresh via MCP, scope parameter (full/incremental) | #75 |
| Structured error codes | 6 new codes with constructors and remediation | #75, #76 |
| Streaming design doc | SSE design document for v8.2 | #75 |
| **`explore` tool** | Comprehensive area exploration (file/dir/module) | #77 |
| **`understand` tool** | Symbol deep-dive with ambiguity handling | #77 |
| **`prepareChange` tool** | Pre-change impact + risk assessment | #77 |
| **`batchGet` tool** | Retrieve multiple symbols by ID (max 50) | #77 |
| **`batchSearch` tool** | Multiple symbol searches in one call (max 10) | #77 |
| **Error audit** | Replace raw `fmt.Errorf` with `CkbError` in MCP handlers | #79 |

### Enhanced getStatus Spec

The `getStatus` tool should return:

```json
{
  "backends": {
    "scip": {
      "status": "available",
      "latencyMs": 12
    },
    "git": {
      "status": "available"
    },
    "lsp": {
      "status": "unavailable",
      "reason": "No LSP server configured",
      "remediation": "Configure LSP server in .ckb/config.json"
    }
  },
  "index": {
    "fresh": false,
    "commitsBehind": 3,
    "lastIndexed": "2h ago",
    "symbolCount": 4521,
    "fileCount": 156
  },
  "overallHealth": "degraded",
  "suggestions": [
    "Run 'ckb index' to refresh stale index",
    "Configure LSP for enhanced code intelligence"
  ]
}
```

Health tiers:
- `available` — Backend working normally
- `degraded` — Backend available but with warnings
- `unavailable` — Backend not available, includes remediation

### reindex Tool Spec

```json
// Input
{
  "scope": "full",      // "full" | "incremental"
  "async": false        // Return immediately if true
}

// Output
{
  "status": "action_required",  // "skipped" | "action_required" | "started" | "completed"
  "message": "Index is 3 commits behind. Run 'ckb index' to refresh.",
  "jobId": "..."               // If async=true
}
```

### Error Code Taxonomy

| Code | When | Remediation |
|------|------|-------------|
| `AMBIGUOUS_QUERY` | Multiple symbols match query | Narrow with scope, kind, or more specific name |
| `PARTIAL_RESULT` | Some backends failed | Result incomplete; check backend health |
| `INVALID_PARAMETER` | Bad input | Check parameter format |
| `RESOURCE_NOT_FOUND` | Symbol/file doesn't exist | Verify ID or path |
| `PRECONDITION_FAILED` | Required condition not met | Check index freshness, backend availability |
| `OPERATION_FAILED` | General failure | Check logs, retry |

---

## Compound Operations (Merged into v8.0)

**Goal:** Reduce AI tool calls by 60-70% with smart aggregation.

> **Note:** These features were originally planned for v8.1 but have been merged into v8.0.

### Tools

#### `explore` — Area Exploration

Replaces: `explainFile` → `searchSymbols` → `getCallGraph` → `getHotspots`

```json
// Input
{
  "target": "internal/query",  // file, directory, or module path
  "depth": "standard",         // "shallow" | "standard" | "deep"
  "focus": "structure"         // "structure" | "dependencies" | "changes"
}

// Output
{
  "overview": { /* module overview */ },
  "keySymbols": [ /* top 20 by importance */ ],
  "dependencies": { /* imports/exports */ },
  "recentChanges": [ /* if git available */ ],
  "hotspots": [ /* if git available */ ],
  "suggestions": [ /* drilldown hints */ ],
  "health": { /* backend status */ }
}
```

Implementation:
1. Determine target type (file vs directory vs module)
2. Run sub-queries in parallel:
   - `ExplainFile` or `GetModuleOverview`
   - `SearchSymbols` (scoped to target)
   - `GetCallGraph` (if depth >= standard)
   - `GetHotspots` (if git available)
3. Rank symbols: exported > internal, referenced > orphan
4. Truncate to response budget (~100KB)
5. Generate drilldown suggestions

#### `understand` — Symbol Deep-Dive

Replaces: `searchSymbols` → `getSymbol` → `explainSymbol` → `findReferences` → `getCallGraph`

```json
// Input
{
  "query": "HandleRequest",
  "includeReferences": true,
  "includeCallGraph": true,
  "maxReferences": 50
}

// Output
{
  "symbol": { /* full symbol detail */ },
  "explanation": "...",
  "references": [ /* grouped by file */ ],
  "callers": [ /* symbols that call this */ ],
  "callees": [ /* symbols this calls */ ],
  "relatedTests": [ /* test files using this */ ],
  "ambiguity": {
    "matchCount": 3,
    "topMatches": [ /* if multiple matches */ ],
    "hint": "Add scope to disambiguate"
  }
}
```

Implementation:
1. Search for symbol, check match count
2. If exact match → proceed
3. If multiple matches → return ambiguity info with top 3-5
4. Parallel fetch: explanation, references, call graph
5. Group references by file
6. Identify test files via naming convention + import analysis

#### `prepareChange` — Pre-Change Analysis

Replaces: `analyzeImpact` + `getAffectedTests` + `analyzeCoupling` + risk calculation

```json
// Input
{
  "target": "ckb:repo:sym:abc123",  // symbol ID or file path
  "changeType": "modify"            // "modify" | "rename" | "delete" | "extract"
}

// Output
{
  "target": { /* symbol detail */ },
  "directDependents": [ /* immediate callers */ ],
  "transitiveImpact": {
    "totalCallers": 47,
    "moduleSpread": 8,
    "maxDepth": 3
  },
  "relatedTests": [ /* tests to run */ ],
  "coChangeFiles": [ /* historically changed together */ ],
  "riskAssessment": {
    "level": "high",        // "low" | "medium" | "high" | "critical"
    "score": 0.78,
    "factors": [
      "High module spread (8 modules)",
      "Low test coverage",
      "Hot file (changed 12 times in 30 days)"
    ],
    "suggestions": [
      "Consider splitting into smaller changes",
      "Add tests before modifying"
    ]
  }
}
```

### Batch Operations

#### `batchGet` — Multiple Symbols by ID

```json
// Input
{ "symbolIds": ["ckb:...", "ckb:...", ...] }  // max 50

// Output
{
  "results": { "ckb:...": { /* symbol */ }, ... },
  "errors": { "ckb:...": { "code": "...", "message": "..." }, ... }
}
```

#### `batchSearch` — Multiple Searches

```json
// Input
{
  "queries": [
    { "query": "Handler", "kind": "function" },
    { "query": "Service", "kind": "class" }
  ]
}

// Output
{
  "results": [
    { "query": "Handler", "symbols": [...] },
    { "query": "Service", "symbols": [...] }
  ]
}
```

### Files Created

| File | Purpose |
|------|---------|
| `internal/query/compound.go` | All compound operations: `Explore()`, `Understand()`, `PrepareChange()`, `BatchGet()`, `BatchSearch()` |
| `internal/query/compound_test.go` | Tests for compound tools |
| `internal/mcp/tool_impls_compound.go` | MCP handlers for compound tools |

---

## Streaming (Merged into v8.0)

**Goal:** Real-time feedback for long-running operations.

> **Note:** These features were originally planned for v8.2 but have been merged into v8.0.

### Completed

| Feature | Description | PR |
|---------|-------------|-----|
| Streaming infrastructure | `internal/streaming/` package with Stream, Chunker, MCP integration | #78 |
| `findReferences` streaming | Stream references in chunks with progress updates | #78 |
| `searchSymbols` streaming | Stream symbol search results | #78 |
| `getStatus` streaming info | Added streaming capabilities to status response | #78 |

### SSE Streaming Protocol

For streamable tools, add `stream: true` to opt-in:

```json
// Request
{
  "name": "findReferences",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123",
    "stream": true,
    "chunkSize": 20
  }
}

// Initial response
{
  "streamId": "abc123",
  "streaming": true,
  "meta": { "chunkSize": 20 }
}

// MCP notifications follow:
// ckb/streamMeta, ckb/streamChunk, ckb/streamProgress, ckb/streamComplete
```

### Event Types

| Event | Purpose |
|-------|---------|
| `meta` | Stream metadata (total count, chunk size, backends) |
| `chunk` | Batch of items with sequence number |
| `progress` | Phase updates with percentage |
| `done` | Stream complete with summary |
| `error` | Error with code and remediation |

### Streamable Tools

| Operation | Status |
|-----------|--------|
| `findReferences` | Implemented |
| `searchSymbols` | Implemented |
| `explore` (deep) | Planned |
| `prepareChange` | Planned |

### Files Created

| File | Purpose |
|------|---------|
| `internal/streaming/stream.go` | Core Stream type with event sending, heartbeat |
| `internal/streaming/chunker.go` | Generic chunking by count and byte size |
| `internal/streaming/mcp.go` | MCP notification writer for streams |
| `internal/mcp/streaming.go` | StreamingHandler type, registry, wrapForStreaming |
| `internal/mcp/tool_impls_streaming.go` | Streaming implementations for tools |

---

## Success Metrics

### v8.0

| Metric | Target |
|--------|--------|
| Error remediation coverage | 100% of errors include remediation |
| Confidence factors in responses | 100% of tool responses |
| Cache visibility | 100% of cached responses show cache info |
| Tool call reduction | 60-70% fewer calls for common workflows |
| Compound op response time | <2s p95 |
| Ambiguity handling | 100% of multi-match queries return disambiguation |
| Time-to-first-result | <500ms for streamable operations |
| Client compatibility | Works with Claude Code, Cursor |

---

## Implementation Order

```
v8.0 (Complete)
├── ✅ Enhanced getStatus with health tiers (#75)
├── ✅ reindex tool (#75)
├── ✅ New error codes (#75, #76)
├── ✅ Streaming design doc (#75)
├── ✅ explore tool (#77)
├── ✅ understand tool (#77)
├── ✅ prepareChange tool (#77)
├── ✅ batchGet / batchSearch (#77)
├── ✅ SSE streaming infrastructure (#78)
├── ✅ findReferences streaming (#78)
├── ✅ searchSymbols streaming (#78)
└── ✅ Error audit across tool handlers (#79)
```

---

## Related Documents

- `docs/ideas.md` — Feature ideas with value/effort matrix
- `docs/featureplans/change-impact-analysis.md` — Detailed impact analysis spec
- `docs/plan/incremental-multi-language.md` — Multi-language indexing plan
- `docs/backlog/index-refresh-improvements.md` — Index refresh enhancements

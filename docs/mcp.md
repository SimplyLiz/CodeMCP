# CKB MCP Tools Reference (v5.2)

CKB exposes AI-native code navigation capabilities via the Model Context Protocol (MCP), enabling AI assistants like Claude Code to discover, understand, and navigate codebases.

> **v5.2 Theme:** Discovery, orientation, and flow comprehension
> **Non-goal:** Code mutation, refactoring, enforcement, or policy

## Quick Setup

```bash
# Add CKB to Claude Code (project-level)
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify
claude mcp list
```

---

## Platform Contracts

### Backend Budget Classification

Tools are classified by performance budget to ensure predictable behavior.

#### Cheap Tools
`findSymbols` · `explainFile` · `listEntrypoints` · `explainPath` · `getSymbol` · `searchSymbols` · `explainSymbol`

| Constraint | Rule |
|------------|------|
| Allowed backends | Symbol index, lightweight metadata, file system |
| Forbidden | Callgraph expansion, git history > 50 commits, deep traversal |
| Traversal limit | Max 1 hop in dependency/call graphs |
| Max latency | P95 < 300ms |
| Max result size | 50 items |

#### Heavy Tools
`traceUsage` · `getArchitectureMap` · `getHotspots` · `summarizeDiff` · `recentlyRelevant` · `listKeyConcepts` · `analyzeImpact` · `getCallGraph` · `findReferences` · `justifySymbol`

| Constraint | Rule |
|------------|------|
| Allowed backends | Multi-backend joins, bounded graph traversal (depth ≤ 5), bounded git queries (≤ 1000 commits) |
| Max latency | P95 < 2000ms |
| Must include | `limitations` field explaining any truncation or missing data |

---

### Confidence Computation Rules

Confidence scores are derived, not arbitrary:

| Condition | Confidence cap |
|-----------|----------------|
| Full static analysis (SCIP/LSP) coverage | 1.0 |
| Partial static analysis coverage | 0.89 |
| Heuristics only (naming, patterns, location) | 0.79 |
| Key backend missing | 0.69 |
| Multiple backends missing or speculative | 0.39 |

**Composition rule:** Use `min(signal_caps)` as ceiling when combining signals.

**Required fields in responses:**
```typescript
confidence: number
confidenceBasis: Array<{
  backend: "scip" | "lsp" | "git" | "naming" | "location" | "pattern"
  status: "available" | "partial" | "missing"
  heuristic?: string
}>
```

---

### Time Window Defaults

| Tool | Default window |
|------|----------------|
| `getHotspots` | 30 days |
| `summarizeDiff` | 30 days |
| `recentlyRelevant` | 7 days |

All temporal tools accept explicit `timeWindow` parameter to override.

---

## Tools Overview

### Discovery & Search
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [findSymbols](#findsymbols) | Cheap | Fast symbol discovery | v5.2 |
| [searchSymbols](#searchsymbols) | Cheap | Search with filtering | v5.1 ✓ |
| [getSymbol](#getsymbol) | Cheap | Get symbol details | v5.1 ✓ |

### Flow & Runtime Orientation
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [traceUsage](#traceusage) | Heavy | Show how something is reached | v5.2 |
| [listEntrypoints](#listentrypoints) | Cheap | List system entrypoints | v5.2 |
| [getCallGraph](#getcallgraph) | Heavy | Caller/callee graph | v5.1 ✓ |

### File-Level Navigation
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [explainFile](#explainfile) | Cheap | File orientation | v5.2 |
| [explainPath](#explainpath) | Cheap | Path role explanation | v5.2 |

### Change Awareness
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [summarizeDiff](#summarizediff) | Heavy | Compress diffs into intent | v5.2 |

### System-Level Orientation
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [getArchitectureMap](#getarchitecturemap) | Heavy | Architectural overview | v5.2 |
| [getHotspots](#gethotspots) | Heavy | Highlight volatile areas | v5.2 |
| [listKeyConcepts](#listkeyconcepts) | Heavy | Main codebase concepts | v5.2 |
| [recentlyRelevant](#recentlyrelevant) | Heavy | What matters now? | v5.2 |
| [getModuleOverview](#getmoduleoverview) | Heavy | Module statistics | v5.1 ✓ |

### Symbol Analysis
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [explainSymbol](#explainsymbol) | Cheap | AI-friendly symbol explanation | v5.1 ✓ |
| [justifySymbol](#justifysymbol) | Heavy | Keep/remove verdict | v5.1 ✓ |
| [findReferences](#findreferences) | Heavy | Find all usages | v5.1 ✓ |
| [analyzeImpact](#analyzeimpact) | Heavy | Change risk analysis | v5.1 ✓ |

### System
| Tool | Budget | Purpose | Status |
|------|--------|---------|--------|
| [getStatus](#getstatus) | Cheap | System health | v5.1 ✓ |
| [doctor](#doctor) | Cheap | Diagnostics | v5.1 ✓ |

---

## Discovery & Search

### findSymbols

Fast, explicit symbol discovery without side effects.

**Budget:** Cheap | **Status:** v5.2

**Why it exists:** Avoids overloading `explainSymbol` as a search tool. Gives agents a cheap "list candidates" step.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `query` | string | Yes | - | Search query |
| `kinds` | string[] | No | all | Filter by symbol kinds |
| `scope` | string | No | - | Module to search within |
| `limit` | number | No | 50 | Max results |

**Ranking signals:** `matchType` (exact/partial/fuzzy), `kind`, `scope`

**Example:**
```json
{ "query": "auth", "kinds": ["function", "class"], "limit": 20 }
```

**Drilldowns:** `explainSymbol`, `getCallGraph`

---

### searchSymbols

Search for symbols by name with optional filtering.

**Budget:** Cheap | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `query` | string | Yes | - | Search query (substring, case-insensitive) |
| `scope` | string | No | - | Module ID to limit scope |
| `kinds` | string[] | No | - | Symbol kinds: `function`, `method`, `class`, `interface`, `variable`, `constant` |
| `limit` | number | No | 20 | Maximum results |

**Example:**
```json
{ "query": "Handler", "kinds": ["function", "method"], "limit": 50 }
```

---

### getSymbol

Get detailed metadata for a symbol by stable ID.

**Budget:** Cheap | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID |
| `repoStateMode` | string | No | `head` | `head` or `full` (include uncommitted) |

---

## Flow & Runtime Orientation

### traceUsage

Show how something is reached, not just who calls whom.

**Budget:** Heavy | **Status:** v5.2

**Why it exists:** Call graphs are structural. Agents need causal paths: Route → controller → service → DB.

**Boundary with getCallGraph:**
| Tool | Scope | Direction | Output |
|------|-------|-----------|--------|
| `getCallGraph` | Local neighborhood | Outward/inward | Structural adjacency |
| `traceUsage` | From entrypoints to target | Inward | Ranked paths with `pathType` |

> **Rule:** `traceUsage` returns **paths**, not **neighbors**.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Target symbol to trace |
| `maxPaths` | number | No | 10 | Max paths to return |
| `pathTypes` | string[] | No | all | Filter by path type |

**Path types:** `api` | `cli` | `job` | `event` | `test` | `unknown`

**Ranking signals:** `pathType`, `pathLength`, `confidence`

**Start nodes:** Paths start from `listEntrypoints` output, test files, framework configs, or CLI mains.

**Fallback:** If no entrypoints found, returns paths from nearest callers with `pathType: "unknown"` and includes `limitations`.

---

### listEntrypoints

Explicit list of system entrypoints.

**Budget:** Cheap | **Status:** v5.2

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `types` | string[] | No | all | Filter by entrypoint type |
| `limit` | number | No | 30 | Max results |

**Entrypoint types:** `api` | `cli` | `job` | `event`

**Detection basis:** `naming` | `framework-config` | `static-call`

**Ranking signals:** `type`, `detectionBasis`, `fanOut`

> Detection basis is always surfaced separately—never merged into confidence.

---

### getCallGraph

Get caller/callee relationships for a symbol.

**Budget:** Heavy | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Root symbol |
| `direction` | string | No | `both` | `callers`, `callees`, or `both` |
| `depth` | number | No | 1 | Traversal depth (1-4) |

---

## File-Level Navigation

### explainFile

Lightweight orientation when a file is the starting point.

**Budget:** Cheap | **Status:** v5.2

**Why it exists:** Too big for `explainSymbol`, too small for `getModuleOverview`.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `filePath` | string | Yes | Path to the file |

**Output includes:**
- File role summary
- Top defined symbols (max 15)
- Key imports/exports
- Local hotspots

**Drilldowns:** `explainSymbol`, `getModuleOverview`

---

### explainPath

Explain why a path exists and what role it plays.

**Budget:** Cheap | **Status:** v5.2

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `filePath` | string | Yes | Path to explain |
| `contextHint` | string | No | Optional context (e.g., "from traceUsage") |

**Role classifications:** `core` | `glue` | `legacy` | `test-only` | `config` | `unknown`

**Classification basis:** `naming` | `location` | `usage` | `history`

> All responses include `limitations`: "intent inferred from static signals; actual purpose may differ."

---

## Change Awareness

### summarizeDiff

Compress diffs into "what changed, what might break."

**Budget:** Heavy | **Status:** v5.2

**Input (exactly one required):**
```typescript
commitRange?: { base: string; head: string }
prId?: string
commit?: string
timeWindow?: { start: ISO8601; end: ISO8601 }
```

**Output includes:**
- Files/symbols touched
- Behavior-relevant changes
- Risk signals (API change, signature change)
- Suggested tests to run
- Migration steps (procedural only)

**Allowed vs Not Allowed:**
| Allowed | Not allowed |
|---------|-------------|
| Risk flags | Proposed code changes |
| Affected surfaces | Refactor plans |
| Suggested drilldowns | Style enforcement |
| Suggested tests | Rewrites or code suggestions |

**Default time window:** 30 days if no selector specified.

---

## System-Level Orientation

### getArchitectureMap

Small, conservative architectural overview.

**Budget:** Heavy | **Status:** v5.2 (enhances v5.1 `getArchitecture`)

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `depth` | number | No | 2 | Dependency depth |
| `includeExternalDeps` | boolean | No | false | Include external deps |
| `refresh` | boolean | No | false | Force cache refresh |

**Hard caps:**
| Constraint | Limit |
|------------|-------|
| Max nodes | 15–20 modules |
| Max edges | 50 |

**Pruning rule:** Keep edges with `strength ≥ 0.3`, then top 50 by strength, lexical tiebreaker.

---

### getHotspots

Highlight areas that deserve attention.

**Budget:** Heavy | **Status:** v5.2

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `timeWindow` | object | No | 30 days | Time period to analyze |
| `scope` | string | No | - | Module to focus on |
| `limit` | number | No | 20 | Max results |

**Ranking signals:** `churn`, `coupling`, `recency`

---

### listKeyConcepts

What are the main ideas in this codebase?

**Budget:** Heavy | **Status:** v5.2

**Why it exists:** Not architecture—semantic clustering. Helps onboarding agents understand domain vocabulary.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `limit` | number | No | 12 | Max concepts |

**Output per concept:**
```typescript
{
  label: string           // e.g., "Auth", "Billing", "Sync"
  evidence: string[]      // top 3–5 modules/symbols
  basis: "naming" | "cluster" | "entrypoints"
  confidence: number
  confidenceBasis: ConfidenceBasis[]
}
```

**Hard caps:** Max 12 concepts, 3–5 evidence items each.

**Ranking signals:** `evidenceCount`, `basis`, `spread`

---

### recentlyRelevant

What should I care about now?

**Budget:** Heavy | **Status:** v5.2

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `timeWindow` | object | No | 7 days | Time period |
| `moduleFilter` | string | No | - | Module to focus on |

**Ranking signals:** `churn`, `fanIn`, `hasOpenChanges`

**Open changes:** Unmerged PRs touching the symbol/file/module (if PR backend available).

---

### getModuleOverview

High-level overview of a module.

**Budget:** Heavy | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | No | Module directory path |
| `name` | string | No | Friendly module name |

---

## Symbol Analysis

### explainSymbol

AI-friendly explanation of a symbol including usage, history, and summary.

**Budget:** Cheap | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |

**Output includes:**
- Symbol metadata (name, kind, signature, location)
- Usage statistics (callerCount, referenceCount, testCoverage)
- Git history (lastModified, recentCommits, authors)
- Related symbols

---

### justifySymbol

Keep/investigate/remove verdict for dead code detection.

**Budget:** Heavy | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |

**Verdicts:**
- `keep` - Actively used, well-tested
- `investigate` - Low usage, may need review
- `remove` - Appears to be dead code

---

### findReferences

Find all references to a symbol.

**Budget:** Heavy | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID |
| `scope` | string | No | - | Module to search |
| `includeTests` | boolean | No | false | Include test refs |
| `limit` | number | No | 100 | Max references |

---

### analyzeImpact

Analyze blast radius of changing a symbol.

**Budget:** Heavy | **Status:** v5.1 ✓

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID |
| `depth` | number | No | 2 | Transitive depth |

**Output includes:**
- Direct impact (distance 1)
- Transitive impact (distance 2+)
- Risk score with factors

---

## System Tools

### getStatus

Get CKB system health.

**Budget:** Cheap | **Status:** v5.1 ✓

**Output includes:**
```typescript
{
  status: "healthy" | "degraded" | "unhealthy"
  backends: BackendStatus[]
  cache: CacheStats
  repoState: RepoState
  health: {
    index: {
      completeness: number      // 0.0–1.0
      staleness: "fresh" | "stale" | "very-stale"
      lastRefresh: ISO8601
    }
  }
}
```

---

### doctor

Run diagnostic checks and get suggested fixes.

**Budget:** Cheap | **Status:** v5.1 ✓

---

## Navigation Presets

Predefined exploration modes that configure verbosity, depth, and ranking:

| Preset | Focus |
|--------|-------|
| `onboarding` | Broad, high-level, concept-first |
| `bug-investigation` | Trace-focused, recent changes emphasized |
| `refactor-safety` | Coupling and hotspot aware |
| `review` | Diff-centric, risk signals prominent |

---

## Recommended Workflows

### Understanding a New Codebase
```
1. getStatus()           → Verify CKB is healthy
2. listKeyConcepts()     → Understand domain vocabulary
3. getArchitectureMap()  → See module structure
4. listEntrypoints()     → Find where execution starts
5. findSymbols()         → Discover relevant code
```

### Investigating a Bug
```
1. findSymbols("ErrorType")  → Find related code
2. traceUsage(symbolId)      → How is it reached?
3. getCallGraph(symbolId)    → What does it call?
4. recentlyRelevant()        → What changed recently?
```

### Before Making Changes
```
1. findSymbols("Target")      → Find the symbol
2. explainSymbol(symbolId)    → Understand it
3. findReferences(symbolId)   → Find all usages
4. analyzeImpact(symbolId)    → Assess risk
5. getHotspots()              → Check volatility
```

### Code Review
```
1. summarizeDiff(prId)       → What changed?
2. getHotspots()             → Check affected areas
3. traceUsage(symbolId)      → Verify paths still work
```

### Dead Code Detection
```
1. findSymbols(query)        → Find candidates
2. justifySymbol(symbolId)   → Get verdict
3. explainFile(filePath)     → Understand context
```

---

## Error Codes

| Code | Description | Fix |
|------|-------------|-----|
| `SYMBOL_NOT_FOUND` | Invalid symbol ID | Use findSymbols() first |
| `BACKEND_UNAVAILABLE` | Backend not running | Check getStatus() |
| `INDEX_STALE` | SCIP needs refresh | Run scip-go |
| `QUERY_TIMEOUT` | Query too slow | Add scope/limit |
| `BUDGET_EXCEEDED` | Tool violated budget | Use cheaper alternative |

---

## What CKB Does NOT Do

- ❌ Code mutation or refactoring
- ❌ Test generation
- ❌ Fix suggestions (code)
- ❌ Enforcement / policy checks
- ❌ Lint-style judgments

*Navigation and comprehension only.*

---

## Implementation Status

### v5.1 (Implemented)
- `searchSymbols`, `getSymbol`, `findReferences`
- `explainSymbol`, `justifySymbol`
- `getCallGraph`, `getModuleOverview`, `analyzeImpact`
- `getStatus`, `doctor`

### v5.2 MVP (Phase 1)
- `findSymbols` - Fast discovery
- `explainFile` - File orientation
- `traceUsage` - Causal paths

### v5.2.1 (Phase 2)
- `listEntrypoints`
- `summarizeDiff`

### v5.2.2 (Phase 3)
- `getHotspots`
- `getArchitectureMap`

### v5.2.3 (Phase 4)
- `explainPath`
- `listKeyConcepts`
- `recentlyRelevant`

# CKB v5.2 — AI-Native Navigation Expansion

*Discovery, orientation, and flow comprehension*

---

**Theme:** Discovery, orientation, and flow comprehension  
**Non-goal:** Code mutation, refactoring, enforcement, or policy

v5.2 builds on v5.1's symbol-centric navigation and expands outward to answer:

- *"Where do I start?"*
- *"How does this get triggered?"*
- *"What changed recently and why does it matter?"*
- *"Which parts of the system matter most right now?"*

---

## 0. Platform Contracts

> **Schema continuity:** All v5.2 tools return `AINavigationResponse` (v5.1) with tool-specific `facts`/`evidence` types. Platform contracts in §0 are additive to v5.1 response semantics.

### 0.1 Backend Budget Contract

To prevent scope creep and ensure predictable performance, tools are classified by backend budget.

#### Cheap Tools

`findSymbols` · `explainFile` · `listEntrypoints` · `explainPath`

| Constraint | Rule |
|------------|------|
| Allowed backends | Symbol index, lightweight metadata, file system |
| Forbidden | Callgraph expansion, git history scans > 50 commits, deep traversal |
| Traversal limit | **Max 1 hop** in dependency or call graphs; no workspace-wide aggregates (except from precomputed indexes) |
| Max latency | P95 < 300ms |
| Max result size | 50 items |

#### Heavy Tools

`traceUsage` · `getArchitectureMap` · `getHotspots` · `summarizeDiff` · `recentlyRelevant` · `listKeyConcepts`

| Constraint | Rule |
|------------|------|
| Allowed backends | Multi-backend joins, bounded graph traversal (depth ≤ 5), bounded git queries (≤ 1000 commits) |
| Max latency | P95 < 2000ms |
| Must include | `limitations` field explaining any truncation or missing data |

> **Enforcement:** Any tool violating its budget class must be reclassified or optimized before release.

---

### 0.2 Confidence Computation Rules

Confidence scores must be **derived**, not arbitrary. The following rules ensure a clear trust gradient:

| Condition | Confidence cap |
|-----------|----------------|
| Backed by static analysis (SCIP/LSP) with full coverage | 1.0 |
| Static analysis with partial coverage | 0.89 |
| Heuristics only (naming, patterns, location) | 0.79 |
| Key backend missing (e.g., no SCIP for call counts) | 0.69 |
| Multiple backends missing or speculative inference | 0.39 |

**Composition rule:** When combining signals, use `min(signal_caps)` as the ceiling.

**Required fields:**

```typescript
confidence: number
confidenceBasis: Array<{
  backend: string      // e.g., "scip", "lsp", "git", "naming"
  status: "available" | "partial" | "missing"
  heuristic?: string   // e.g., "naming", "location", "pattern"
}>
```

**Examples:**
- `[{ backend: "scip", status: "available" }]` → cap 1.0
- `[{ backend: "scip", status: "partial" }, { backend: "git", status: "available" }]` → cap 0.89
- `[{ backend: "naming", status: "available", heuristic: "naming" }]` → cap 0.79

---

### 0.3 Ranking Policy v5.2

All ranked outputs must be **auditable and deterministic**.

#### Required fields per ranked item

```typescript
ranking: {
  score: number
  signals: Record<string, number | boolean | string>
  policyVersion: "5.2"
}
```

> **Note:** `position` is implicit from array index (0-based). Do not duplicate.

#### Signals by tool

| Tool | Required signals (key → type) |
|------|-------------------------------|
| `findSymbols` | `matchType`: `"exact"\|"partial"\|"fuzzy"`, `kind`: string, `scope`: string |
| `traceUsage` | `pathType`: string, `pathLength`: number, `confidence`: number |
| `listEntrypoints` | `type`: string, `detectionBasis`: string, `fanOut`: number |
| `getHotspots` | `churn`: number, `coupling`: number, `recency`: number |
| `listKeyConcepts` | `evidenceCount`: number, `basis`: string, `spread`: number |
| `recentlyRelevant` | `churn`: number, `fanIn`: number, `hasOpenChanges`: boolean |

> **Audit guarantee:** Given the same codebase state and query, rankings must be reproducible. Tiebreaker: symbol/file ID lexical sort.

---

### 0.4 Time Window Semantics

Default time windows (unless explicitly specified):

| Tool | Default window |
|------|----------------|
| `getHotspots` | 30 days |
| `summarizeDiff` | 30 days |
| `recentlyRelevant` | 7 days |

All temporal tools must accept an explicit `timeWindow` parameter to override defaults.

---

## 1. Discovery & Search

### 1.1 findSymbols

**Purpose:** Fast, explicit symbol discovery without side effects

**Budget:** Cheap

#### Why it exists

- Avoids overloading `explainSymbol` as a search tool
- Reduces ambiguity loops
- Gives agents a cheap "list candidates" step

> **Contract:** This tool must be cheap by design — no heavy provenance computation, no deep backend usage. Exploratory by nature.

#### Typical questions

- "Show me auth-related services"
- "What functions handle login?"
- "List top matches for 'refreshToken'"

#### Core output

- Ranked candidates with `matchScore`
- Minimal identity facts
- Stable pointers
- Clear "why it matched" signals
- `ranking.signals`: `matchType`, `kind`, `scope`

#### Drilldowns

- `explainSymbol`
- `getCallGraph`

---

## 2. Flow & Runtime Orientation

### 2.1 traceUsage

**Purpose:** Show how something is reached, not just who calls whom

**Budget:** Heavy

#### Why it exists

- Call graphs are structural
- Agents often need causal paths:
  - Route → controller → service → DB
  - Job → handler → worker → side effects

#### Boundary with getCallGraph

| Tool | Scope | Direction | Output |
|------|-------|-----------|--------|
| `getCallGraph` | Local neighborhood around one symbol | Outward (callees) or inward (callers) | Structural adjacency, depth-limited |
| `traceUsage` | From entrypoints/tests to target | Inward (how is this reached?) | Ranked paths with `pathType` |

> **Rule:** `traceUsage` must never return "just top callers" — that duplicates `getCallGraph`. It must return **paths**, not **neighbors**.

#### Start nodes

Paths may start from:

- `listEntrypoints` output
- Test files (per v5.1 test heuristics)
- Framework config roots
- CLI mains

#### Fallback behavior

If no entrypoints found:

- Return paths from nearest callers with `pathType: "unknown"`
- Include `limitations`: `"Entrypoint set unavailable; showing nearest callers"`

#### Typical questions

- "How does this function get invoked?"
- "Which API route leads here?"
- "How does this test hit production code?"

#### Core output

- Small number of ranked paths (max 10)
- Each path is a chain of symbols/files
- **Required per path:**
  - `pathType`: `api` | `cli` | `job` | `event` | `test` | `unknown`
  - `confidence` + `confidenceBasis`
  - `limitations`
- `ranking.signals`: `pathType`, `pathLength`, `confidence`

#### Drilldowns

- `getCallGraph`
- `explainSymbol`

---

### 2.2 listEntrypoints

**Purpose:** Explicit list of system entrypoints

**Budget:** Cheap

#### Why it exists

- Entrypoints are already inferred heuristically
- Making them explicit reduces guesswork

#### Typical questions

- "What are the main API handlers?"
- "Where does execution start?"
- "Which jobs or CLIs exist?"

#### Core output

- Ranked entrypoints (max 30)
- Entrypoint type: `api` | `cli` | `job` | `event`
- `detectionBasis`: `naming` | `framework-config` | `static-call`
- "Why detected" signals
- `ranking.signals`: `type`, `detectionBasis`, `fanOut`

> **Trust boundary:** Detection basis must always be surfaced separately — never merged into a single confidence score. Entrypoint misidentification erodes trust fast.

---

## 3. File-Level Navigation

### 3.1 explainFile

**Purpose:** Lightweight orientation when a file is the starting point

**Budget:** Cheap

#### Why it exists

- Too big for `explainSymbol`
- Too small for `getModuleOverview`

#### Typical questions

- "What is this file responsible for?"
- "Which symbols matter here?"
- "Why is this file changing so often?"

#### Core output

- File role summary
- Top defined symbols (max 15)
- Key imports/exports
- Local hotspots

#### Drilldowns

- `explainSymbol`
- `getModuleOverview`

---

### 3.2 explainPath *(new)*

**Purpose:** Explain why a path exists and what role it plays

**Budget:** Cheap

#### Why it exists

- Pairs with `traceUsage` for deeper understanding
- Helps agents understand file placement and intent

#### Input

```typescript
filePath: string          // required
contextHint?: string      // optional, e.g., "I saw this in a traceUsage path"
```

#### Typical questions

- "Why does this file exist here?"
- "Is this core, glue, legacy, or test-only?"
- "How does this fit into the module structure?"

#### Core output

- Path rationale
- Role classification: `core` | `glue` | `legacy` | `test-only` | `config` | `unknown`
- `classificationBasis`: `naming` | `location` | `usage` | `history`
- Module/flow context
- `confidence` + `confidenceBasis`

> **Required disclaimer:** All responses must include `limitations` noting "intent inferred from static signals; actual purpose may differ."

---

## 4. Change Awareness

### 4.1 summarizeDiff

**Purpose:** Compress diffs into "what changed, what might break"

**Budget:** Heavy

#### Why it exists

- Agents reason better from intent than raw diffs
- Supports PR/commit comprehension

#### Input (one of)

```typescript
// Exactly one selector required
commitRange?: { base: string; head: string }
prId?: string
commit?: string
timeWindow?: { start: ISO8601; end: ISO8601 }
```

#### Typical questions

- "What did this PR change?"
- "What should I be careful about here?"
- "Which tests should be run?"

#### Core output

- Files/symbols touched
- Behavior-relevant changes
- Risk signals (API change, signature change)
- Suggested drilldowns
- Suggested tests to run (command optional)
- Migration steps (only if purely procedural)

#### Allowed vs Not Allowed

| Allowed | Not allowed |
|---------|-------------|
| Risk flags | Proposed code changes |
| Affected surfaces | Refactor plans |
| Suggested drilldowns | Style enforcement |
| Suggested tests to run | Rewrites |
| Procedural migration steps | Code suggestions |

> **Constraint:** Descriptive and advisory only — no code generation, no refactoring suggestions.

#### Time window

Default: last 30 days if no selector specified. Explicit selector takes precedence.

---

## 5. System-Level Orientation

### 5.1 getHotspots

**Purpose:** Highlight areas that deserve attention

**Budget:** Heavy

#### Why it exists

- Agents need prioritization signals
- Humans ask: "what's risky right now?"

#### Typical questions

- "What parts of the code are volatile?"
- "Where do bugs likely cluster?"
- "What should I review first?"

#### Core output

- Ranked files/modules/symbols (max 20)
- Why they're hotspots (churn, coupling)
- Time-window aware (default: 30 days)
- `ranking.signals`: `churn`, `coupling`, `recency`

---

### 5.2 getArchitectureMap

**Purpose:** Small, conservative architectural overview

**Budget:** Heavy

#### Why it exists

- Humans build mental maps
- LLMs need a compressed version

#### Typical questions

- "How is this system structured?"
- "Which modules depend on which?"
- "Are there obvious clusters?"

#### Core output

- Module graph (small!)
- Strong dependency edges
- Inferred clusters
- Explicit limitations

#### Hard caps (required)

| Constraint | Limit |
|------------|-------|
| Max nodes | 15–20 modules |
| Max edges | 50 |
| Limitations | Always surfaced in response |

#### Deterministic pruning rule

1. Keep edges with `strength ≥ 0.3`
2. If still > 50 edges, keep top 50 by `strength`
3. Tiebreaker: `sourceModuleId + targetModuleId` lexical sort

> **Guardrail:** Without caps and deterministic pruning, this tool risks becoming "a bad diagram in JSON form."

---

### 5.3 listKeyConcepts *(new)*

**Purpose:** What are the main ideas in this codebase?

**Budget:** Heavy

#### Why it exists

- Not architecture — semantic clustering
- Helps onboarding agents understand domain vocabulary

#### Typical questions

- "What are the main concepts in this repo?"
- "What domains does this codebase cover?"

#### Core output (max 8–12 concepts)

Each concept includes:

```typescript
{
  label: string                    // e.g., "Auth", "Billing", "Sync"
  evidence: string[]               // top 3–5 modules/symbols
  basis: "naming" | "cluster" | "entrypoints"
  confidence: number
  confidenceBasis: ConfidenceBasis[]
}
```

- `ranking.signals`: `evidenceCount`, `basis`, `spread`

#### Hard caps

| Constraint | Limit |
|------------|-------|
| Max concepts | 12 |
| Evidence per concept | 3–5 items |

---

### 5.4 recentlyRelevant *(new)*

**Purpose:** What should I care about now?

**Budget:** Heavy

#### Why it exists

- Complements `getHotspots` with a temporal lens
- Answers "what matters *right now*?"

#### Inputs

```typescript
timeWindow?: { start: ISO8601; end: ISO8601 }  // default: 7 days
moduleFilter?: string
```

#### Core output

- Symbols/files with:
  - Recent churn
  - High fan-in
  - Open changes (see definition below)
- `ranking.signals`: `churn`, `fanIn`, `hasOpenChanges`

#### Open changes definition

| Source | Definition |
|--------|------------|
| PR backend available | Unmerged PRs touching the symbol/file/module |
| PR backend unavailable | Omit `hasOpenChanges`; add `limitations`: `"PR integration unavailable"` |

---

## 6. Cross-Cutting Enhancements

*(No new tool)*

### 6.1 Navigation Presets

Predefined "exploration modes":

- **onboarding** — broad, high-level, concept-first
- **bug-investigation** — trace-focused, recent changes emphasized
- **refactor-safety** — coupling and hotspot aware
- **review** — diff-centric, risk signals prominent

These pre-configure verbosity, depth, and ranking — no new backend logic.

---

### 6.2 Confidence & Limitation Surfacing

Every new tool:

- Must surface `confidence` + `confidenceBasis`
- Must list missing backend coverage in `limitations`
- Must explain heuristic use

This preserves trust as scope grows.

---

## What v5.2 Explicitly Does Not Add

To keep the feature coherent:

- ❌ Refactoring or code mutation
- ❌ Test generation
- ❌ Fix suggestions (code)
- ❌ Enforcement / policy checks
- ❌ Lint-style judgments

*Those belong to a separate product surface if ever added.*

---

## Suggested Implementation Order

*(Pragmatic — max value with minimal effort)*

### Phase 1 — v5.2 MVP (must-have)

1. **findSymbols** — unblocks everything
2. **explainFile** — most common starting point
3. **traceUsage** — killer feature (more backend work)

### Phase 2 — v5.2.1 (trust & flow)

4. **listEntrypoints**
5. **summarizeDiff**

### Phase 3 — v5.2.2 (strategic)

6. **getHotspots**
7. **getArchitectureMap**

### Phase 4 — v5.2.3 (extensions)

8. **explainPath**
9. **listKeyConcepts**
10. **recentlyRelevant**

---

## One-Sentence Positioning

> **CKB v5.2 expands AI-native code navigation from "what is this symbol?" to "how does this system work, where should I look, and what matters right now?"**

---

## Next Steps

Options for next iteration:

- Design exact schemas for 2–3 of these tools
- Cut this down to a minimal v5.2 MVP
- Stress-test against real agent prompts ("what would Claude/GPT ask first?")
- Develop "CKB Navigation Layer" narrative for positioning

---

## Appendix A: Review Summary

### What v5.2 Gets Right

| Strength | Why it matters |
|----------|----------------|
| Clear expansion axis | Symbol → path → flow → system → change (no scope creep) |
| `findSymbols` is foundational | Removes pathological overuse of `explainSymbol` as search |
| `traceUsage` fills blind spot | Causal paths, not just structural graphs |
| `explainFile` is pragmatic | Solves real "middle ground" pain point |
| `summarizeDiff` without mutation | Intent compression, not review automation |

### Feedback Incorporated (Draft 2)

| Feedback | Resolution |
|----------|------------|
| Missing confidence normalization | Added shared confidence scale |
| `getArchitectureMap` needs caps | Added hard caps table (15–20 nodes, edge pruning) |
| `listEntrypoints` needs contract | Added `detectionBasis` field requirement |
| No time window semantics | Added defaults table for all temporal tools |
| Suggested `explainPath` | Added as 3.2 |
| Suggested `listKeyConcepts` | Added as 5.3 |
| Suggested `recentlyRelevant` | Added as 5.4 |

### Feedback Incorporated (Draft 3)

| Feedback | Resolution |
|----------|------------|
| Define "cheap" vs "heavy" | Added Backend Budget Contract (Section 0.1) |
| Overlap between `traceUsage` and `getCallGraph` | Added explicit boundary table in 2.1 |
| Confidence needs computation rules | Added Confidence Computation Rules (Section 0.2) |
| Ranking policy versioning | Added Ranking Policy v5.2 (Section 0.3) with signals per tool |
| `summarizeDiff` "no suggestions" too strict | Refined to allowed/not-allowed table |
| `listKeyConcepts` needs caps | Added max 12 concepts, evidence structure |
| `explainPath` needs input spec + disclaimer | Added input schema and required disclaimer |

### Feedback Incorporated (Draft 4)

| Feedback | Resolution |
|----------|------------|
| Confidence caps conflict (heuristics = partial static) | Adjusted: heuristics-only now caps at 0.79 |
| Ranking signals should be structured | Changed to `Record<string, number \| boolean \| string>` |
| Cheap tools need traversal limit | Added "max 1 hop" rule to budget contract |
| `traceUsage` needs start nodes + fallback | Added start nodes list and fallback behavior |
| `summarizeDiff` input underspecified | Added explicit input selectors (commitRange, prId, commit, timeWindow) |
| `recentlyRelevant` "openDiffs" undefined | Added open changes definition with PR backend fallback |
| `getArchitectureMap` pruning not deterministic | Added pruning rule: strength ≥ 0.3, top 50, lexical tiebreaker |
| v5.2 should declare v5.1 response reuse | Added schema continuity statement at top of §0 |

---

## Appendix B: Type Definitions

```typescript
// Confidence basis (structured, not string tags)
interface ConfidenceBasis {
  backend: "scip" | "lsp" | "git" | "naming" | "location" | "pattern"
  status: "available" | "partial" | "missing"
  heuristic?: string
}

// Ranking (v5.2)
interface Ranking {
  score: number
  signals: Record<string, number | boolean | string>
  policyVersion: "5.2"
}

// Time window
interface TimeWindow {
  start: string  // ISO8601
  end: string    // ISO8601
}

// summarizeDiff input (exactly one required)
type SummarizeDiffInput = 
  | { commitRange: { base: string; head: string } }
  | { prId: string }
  | { commit: string }
  | { timeWindow: TimeWindow }
```

---

## Appendix C: Remediation Plan (Based on Effectiveness Assessment)

Testing revealed that while CKB's foundations are solid, several gaps must be closed before v5.2 tools can deliver on their promises. This appendix maps assessment findings to concrete remediation work.

### Assessment Summary

| Area | Status | Finding |
|------|--------|---------|
| `searchSymbols` | ✅ Working | Fast (43–122ms), relevant results with kind, location, visibility |
| `explainSymbol` | ✅ Working | Rich output: docs, usage stats (42 refs), git history, flags |
| SCIP backend | ✅ Healthy | Core symbol search/definition lookup works |
| Git backend | ✅ Healthy | Blame/history available |
| Index freshness | ⚠️ Degraded | Completeness: 0.3, reason: "index-stale" |
| Call graph | ❌ Incomplete | Returns only root node; "callee analysis not yet implemented" |
| Architecture | ❌ Sparse | Returns 1 module, 0 symbol count, empty dependency graph |
| LSP backend | ⚠️ Unavailable | "No servers running" — optional but limits live analysis |
| Cache | ⚠️ Unused | 0% hit rate, 0 queries cached |

---

### Remediation Plan

#### P0 — Must fix before v5.2 MVP

These blockers directly prevent MVP tools from working.

| Issue | Impact | Fix | Owner | Target |
|-------|--------|-----|-------|--------|
| **Stale index** | All tools report low confidence | Implement automatic index refresh on git changes; add `ckb refresh` command; surface staleness in `getStatus` prominently | Backend | v5.2-alpha |
| **Call graph incomplete** | `traceUsage` cannot function; `getCallGraph` returns stub | Complete callee analysis in SCIP backend; implement bounded BFS for caller chains | Backend | v5.2-alpha |
| **Architecture empty** | `getArchitectureMap` unusable | Implement module detection from file structure + import graph; populate dependency edges from SCIP references | Backend | v5.2-alpha |

#### P1 — Required for v5.2.1

These affect trust and completeness but don't block MVP.

| Issue | Impact | Fix | Owner | Target |
|-------|--------|-----|-------|--------|
| **calleeCount always 0** | `explainSymbol` output incomplete | Wire callee analysis output to symbol stats | Backend | v5.2.1 |
| **Cache unused** | Repeated queries hit backend; latency risk for cheap tools | Implement LRU cache with TTL; add cache-hit signal to response metadata | Backend | v5.2.1 |
| **LSP unavailable** | No live type info; limits confidence ceiling | Document LSP setup; add `ckb lsp start` helper; graceful degradation already in place | Docs + Backend | v5.2.1 |

#### P2 — Quality of life

| Issue | Impact | Fix | Owner | Target |
|-------|--------|-----|-------|--------|
| **Index completeness opaque** | Users don't know why completeness is low | Add `indexHealth` breakdown: files indexed, symbols parsed, errors encountered | Backend | v5.2.2 |
| **No incremental indexing** | Full reindex required after changes | Implement file-level incremental SCIP updates | Backend | v5.2.2+ |

---

### Gating Criteria for v5.2 Release

Before declaring v5.2 MVP ready:

| Criterion | Threshold | Measurement |
|-----------|-----------|-------------|
| Index completeness | ≥ 0.9 on test repos | `getStatus().completeness` |
| Call graph depth | ≥ 3 hops for callers and callees | Manual test on 10 symbols |
| Architecture coverage | ≥ 80% of directories mapped to modules | `getArchitectureMap().modules.length` vs actual |
| `traceUsage` paths | Returns ≥ 1 path for 80% of non-leaf symbols | Automated test suite |
| P95 latency (cheap tools) | < 300ms | Load test |
| P95 latency (heavy tools) | < 2000ms | Load test |

---

### Revised Implementation Order (Post-Remediation)

Given the assessment, the implementation order shifts to:

#### Phase 0 — Backend Remediation (new)

1. **Index refresh automation** — unblocks everything
2. **Call graph completion** — required for `traceUsage`
3. **Module detection** — required for `getArchitectureMap`

#### Phase 1 — v5.2 MVP (unchanged, but gated on Phase 0)

1. `findSymbols`
2. `explainFile`
3. `traceUsage`

#### Phase 2+ — As previously specified

---

### Confidence Impact

Until remediation is complete, v5.2 tools must cap confidence as follows:

| Condition | Confidence cap |
|-----------|----------------|
| Index stale (`completeness < 0.7`) | 0.69 max |
| Call graph incomplete | `traceUsage` capped at 0.39; `getCallGraph` returns `limitations` |
| Architecture sparse | `getArchitectureMap` capped at 0.39; returns `limitations` |

These caps are automatically lifted when the underlying backend issues are resolved.

---

### Monitoring

Add the following to `getStatus` output for ongoing health tracking:

```typescript
health: {
  index: {
    completeness: number        // 0.0–1.0
    staleness: "fresh" | "stale" | "very-stale"
    lastRefresh: ISO8601
    filesIndexed: number
    symbolsIndexed: number
    errors: number
  }
  backends: {
    scip: "healthy" | "degraded" | "unavailable"
    git: "healthy" | "degraded" | "unavailable"
    lsp: "healthy" | "degraded" | "unavailable"
  }
  cache: {
    hitRate: number             // 0.0–1.0
    entries: number
    evictions: number
  }
}
```

---

*This remediation plan ensures v5.2 delivers on its promises rather than surfacing incomplete data with low confidence.*

---

*Document version: v5.2-draft-5 (with remediation plan)*  
*Last updated: December 2024*

# CKB (Code Knowledge Backend) Design Document v5

> **For Claude Code**: We are building CKB—a language-agnostic codebase comprehension layer that orchestrates existing code intelligence backends (SCIP, Glean, LSP, Git) and provides semantically compressed, LLM-optimized views.
>
> **What CKB does**: Consumes existing semantic indexes (SCIP/Glean/LSP). Performs cheap file scans for architecture heuristics. Compresses raw facts into bounded, actionable responses.
>
> **What CKB does NOT do**: Build language semantic indexes. Store source code content (including LSP hover text). Replace your indexer.
>
> **V5 Key Changes**: `getSymbol` tool, RepoStateMode rules, source-of-truth table, error taxonomy, metadata supplement rules, import edge classification, tombstones, structured doctor fixes, backend budgets, deterministic encoding, union conflict rules.

---

## Table of Contents

1. [Vision](#1-vision)
2. [Goals & Non-Goals](#2-goals--non-goals)
3. [Architecture](#3-architecture)
4. [Identity System](#4-identity-system)
5. [Module System](#5-module-system)
6. [Backend System](#6-backend-system)
7. [Compression Contract](#7-compression-contract)
8. [Impact Analysis](#8-impact-analysis)
9. [Cache System](#9-cache-system)
10. [Response Budget & Backend Limits](#10-response-budget--backend-limits)
11. [Partial Results Contract](#11-partial-results-contract)
12. [Deterministic Output](#12-deterministic-output)
13. [Error Handling](#13-error-handling)
14. [Security & Privacy](#14-security--privacy)
15. [Performance](#15-performance)
16. [V1 MVP Specification](#16-v1-mvp-specification)
17. [Implementation Plan](#17-implementation-plan)
18. [Appendices](#appendices)

---

## 1. Vision

### The Problem

LLMs struggle with large codebases because:

1. **Raw code intelligence is too granular** — Glean returns 847 references; LLMs need "4 modules affected"
2. **Context windows are limited** — Can't dump entire repos into prompts
3. **Existing tools don't compose** — LSP, Glean, Git speak different protocols
4. **No semantic compression exists** — Nothing translates "code facts" into "codebase understanding"

### The Solution

```
┌─────────────────────────────────────────────────────────────┐
│  LLM Tools (Claude Code, CogniCode, IDEs)                   │
└─────────────────────────┬───────────────────────────────────┘
                          │ Structured facts + explanations
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  CKB: Comprehension Layer                                   │
│  - Consumes semantic indexes (does NOT build them)          │
│  - Backend ladder (SCIP → LSP fallback)                     │
│  - Semantic compression (measurable, bounded)               │
│  - Honest completeness + provenance                         │
└─────────────────────────┬───────────────────────────────────┘
                          │ Raw facts queries
                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Code Intelligence Backends                                 │
│  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐                    │
│  │ SCIP  │ │  LSP  │ │  Git  │ │ Glean │                    │
│  │(pref) │ │(fallb)│ │(always│ │ (v2)  │                    │
│  └───────┘ └───────┘ └───────┘ └───────┘                    │
└─────────────────────────────────────────────────────────────┘
```

### Core Principles

1. **Don't build indexes** — Consume SCIP, Glean, LSP; don't reinvent them
2. **Backend ladder** — Prefer high-quality backends; fall back gracefully
3. **Compression is the product** — Measurable ratios, bounded outputs
4. **Structured-first** — Every response has testable facts, derived explanations
5. **Honest about quality** — Completeness reasons, not just numbers
6. **Deterministic** — Same inputs = same outputs (for caching, testing, sanity)
7. **Graceful degradation** — Always return partial results with clear provenance

---

## 2. Goals & Non-Goals

### V1 Goals (MVP)

- [x] Backend ladder: SCIP (preferred) → LSP (fallback) → Git (always)
- [x] Stable + versioned canonical IDs with alias redirects + tombstones
- [x] Seven core tools: `getStatus`, `doctor`, `getSymbol`, `searchSymbols`, `findReferences`, `getArchitecture`, `analyzeImpact`
- [x] RepoStateMode rules (full vs head per tool)
- [x] Deterministic output (ordering + encoding)
- [x] Completeness with reasons
- [x] Cache tiers with working-tree change detection
- [x] Response budgets + backend limits
- [x] Smart drilldowns based on truncation reason
- [x] Stable error taxonomy
- [x] MCP server for Claude Code integration

### V2 Goals

- [ ] Glean adapter
- [ ] LLM-generated semantic summaries
- [ ] Test coverage integration
- [ ] Semantic duplication detection

### Non-Goals (Never)

- ❌ Building language semantic indexes (use SCIP/Glean)
- ❌ Storing source code content (including LSP hover text)
- ❌ Code modification or generation
- ❌ Replacing your existing indexer

---

## 3. Architecture

### 3.1 System Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         CKB System                              │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  API Layer (MCP / HTTP / CLI)                             │  │
│  │  All call unified QueryEngine                             │  │
│  └───────────────────────────┬───────────────────────────────┘  │
│                              │                                  │
│  ┌───────────────────────────▼───────────────────────────────┐  │
│  │  Query Engine                                             │  │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │  │
│  │  │ResponseBudget│ │  Compressor  │ │ View Builder │       │  │
│  │  └──────────────┘ └──────────────┘ └──────────────┘       │  │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │  │
│  │  │   Orderer    │ │Drilldown Gen │ │Error Mapper  │       │  │
│  │  └──────────────┘ └──────────────┘ └──────────────┘       │  │
│  └───────────────────────────┬───────────────────────────────┘  │
│                              │                                  │
│  ┌───────────────────────────▼───────────────────────────────┐  │
│  │  Backend Orchestrator                                     │  │
│  │  - Backend ladder (preference order)                      │  │
│  │  - Merge strategy (prefer-first + supplement)             │  │
│  │  - Concurrency limits + request coalescing                │  │
│  │  - Budget enforcement                                     │  │
│  └───────────────────────────┬───────────────────────────────┘  │
│                              │                                  │
│  ┌───────────────────────────▼───────────────────────────────┐  │
│  │  Backend Adapters                                         │  │
│  │  ┌────────┐ ┌────────────┐ ┌────────┐ ┌────────┐          │  │
│  │  │  SCIP  │ │    LSP     │ │  Git   │ │ Glean  │          │  │
│  │  │ (pref) │ │(supervisor)│ │(always)│ │ (v2)   │          │  │
│  │  └────────┘ └────────────┘ └────────┘ └────────┘          │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Storage                                                  │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │  │
│  │  │ ID Maps  │ │ Aliases  │ │  Cache   │ │Dep Index │      │  │
│  │  │+Tombstone│ │ +Chains  │ │ (tiered) │ │          │      │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 Source of Truth Table

Each data type has one authoritative backend. This prevents mixing facts incorrectly.

| Data Type | Authoritative | Fallback | Notes |
|-----------|---------------|----------|-------|
| Symbol stable IDs | SCIP / Glean | Computed fingerprint | Never use LSP as ID anchor |
| Symbol definitions | SCIP / Glean | LSP | |
| References | SCIP / Glean | LSP (with completeness warning) | |
| Call graph | SCIP / Glean | None | Don't fake it from LSP |
| Module dependency edges | Import scan | SCIP occurrences | Architecture heuristic |
| History / churn | Git | None | |
| Visibility | SCIP modifiers | Ref analysis → naming | Cascading fallback |
| Repo state | Git | None | |

### 3.3 RepoState

```typescript
interface RepoState {
  repoStateId: string;           // Hash of all components
  headCommit: string;
  stagedDiffHash: string;
  workingTreeDiffHash: string;
  untrackedListHash: string;
  dirty: boolean;                // Any uncommitted changes
  computedAt: string;
}

function computeRepoState(): RepoState {
  const head = exec('git rev-parse HEAD');
  const staged = hash(exec('git diff --cached'));
  const working = hash(exec('git diff HEAD'));
  const untracked = hash(exec('git ls-files --others --exclude-standard'));
  const dirty = staged !== EMPTY_HASH || working !== EMPTY_HASH || untracked !== EMPTY_HASH;
  
  return {
    repoStateId: hash(`${head}:${staged}:${working}:${untracked}`),
    headCommit: head,
    stagedDiffHash: staged,
    workingTreeDiffHash: working,
    untrackedListHash: untracked,
    dirty,
    computedAt: new Date().toISOString()
  };
}
```

### 3.4 RepoStateMode Rules

Different tools require different freshness guarantees.

| Tool | RepoStateMode | Reason | Cache Key Uses |
|------|---------------|--------|----------------|
| `getStatus` | `head` | Metadata only | HEAD |
| `doctor` | `head` | Diagnostic | HEAD |
| `getSymbol` | `head` (default) | Hot path, location flagged if dirty | HEAD |
| `searchSymbols` | `head` | Index-backed names | HEAD |
| `findReferences` | `full` | Returns locations | Full repoStateId |
| `analyzeImpact` | `full` | Refs + locations | Full repoStateId |
| `getArchitecture` | `full` | Module deps change with uncommitted files | Full repoStateId |

**`getSymbol` special handling**:
- Default: `repoStateMode=head` for speed
- Response includes `locationFreshness: 'fresh' | 'may-be-stale'`
- If `repoState.dirty=true`: `locationFreshness='may-be-stale'` + warning + drilldown
- Optional param: `repoStateMode: 'full'` for exact location

### 3.5 Workspace Root Semantics

```typescript
interface WorkspaceConfig {
  // Where .ckb/ lives
  repoRoot: string;
  
  // LSP workspace strategy
  workspaceStrategy: 'repo-root' | 'manifest-roots';  // Default: repo-root
  
  // Module roots (detected or configured)
  moduleRoots: ModuleRoot[];
}
```

**Default behavior**:
- One LSP process per language at repo root
- `manifest-roots` is optional/experimental (one LSP per detected module)
- `doctor` warns if language is known to need per-module roots (e.g., Java)

### 3.6 Path Canonicalization

All paths in CKB are:
- Repo-relative
- Forward slashes always (`/`)
- Case-preserving, but case-insensitive comparison on Windows
- Symlinks resolved to real path

```typescript
function canonicalizePath(absolutePath: string, repoRoot: string): string {
  const resolved = fs.realpathSync(absolutePath);  // Resolve symlinks
  const relative = path.relative(repoRoot, resolved);
  return relative.replace(/\\/g, '/');  // Forward slashes
}
```

**FileId format**: Repo-relative path (not hash—humans need to read it).

---

## 4. Identity System

### 4.1 Stable ID + Definition Version ID

| ID Type | Purpose | Changes When |
|---------|---------|--------------|
| `stableId` | Long-term reference, linking | Container/name/kind changes |
| `definitionVersionId` | Freshness detection | Signature changes |

```typescript
interface SymbolIdentity {
  stableId: string;   // ckb:<repo>:sym:<stableFingerprint>
  
  definitionVersionId?: string;
  definitionVersionSemantics: 
    | 'backend-definition-hash'
    | 'structural-signature-hash'
    | 'unknown';
  
  fingerprint: {
    qualifiedContainer: string;
    name: string;
    kind: SymbolKind;
    arity?: number;
    signatureNormalized?: string;
  };
  
  location: Location;
  locationFreshness: 'fresh' | 'may-be-stale';
  
  lastVerifiedAt: string;
  lastVerifiedStateId: string;
}
```

### 4.2 Backends as ID Anchors

| Backend | ID Stability | Role |
|---------|--------------|------|
| SCIP | High | Primary anchor |
| Glean | High | Primary anchor |
| LSP | Low | Resolver only, never anchor |

### 4.3 Symbol State + Tombstones

Symbols can be deleted. Instead of fuzzy-matching to wrong things, we track state.

```typescript
type SymbolState = 'active' | 'deleted' | 'unknown';

interface SymbolMapping {
  stableId: string;
  state: SymbolState;
  
  // If deleted
  deletedAt?: string;
  deletedInStateId?: string;
  
  // ... other fields
}
```

**Resolution behavior**:
- `state=active`: Normal response
- `state=deleted`: Return `{ symbol: null, deleted: true, deletedAt, deletedInStateId }`
- `state=unknown`: Return with warning

### 4.4 Alias/Redirect Mechanism

```sql
CREATE TABLE symbol_aliases (
  old_stable_id TEXT NOT NULL,
  new_stable_id TEXT NOT NULL,
  reason TEXT NOT NULL,           -- 'renamed', 'moved', 'merged', 'fuzzy-match'
  confidence REAL NOT NULL,
  created_at TEXT NOT NULL,
  created_state_id TEXT NOT NULL,
  PRIMARY KEY (old_stable_id, new_stable_id)
);
```

### 4.5 Alias Resolution

```typescript
const ALIAS_CHAIN_MAX_DEPTH = 3;

async function resolveSymbolId(
  requestedId: SymbolId,
  depth: number = 0,
  visited: Set<string> = new Set()
): Promise<ResolvedSymbol> {
  // Cycle detection
  if (visited.has(requestedId)) {
    return { symbol: null, error: 'ALIAS_CYCLE' };
  }
  visited.add(requestedId);
  
  // Max depth
  if (depth > ALIAS_CHAIN_MAX_DEPTH) {
    return { symbol: null, error: 'ALIAS_CHAIN_TOO_DEEP' };
  }
  
  // Direct lookup
  const direct = await db.symbols.get(requestedId);
  if (direct) {
    if (direct.state === 'deleted') {
      return { 
        symbol: null, 
        deleted: true, 
        deletedAt: direct.deletedAt 
      };
    }
    if (direct.state === 'active' && !direct.stale) {
      return { symbol: direct, redirected: false };
    }
  }
  
  // Check aliases
  const alias = await db.aliases.getByOldId(requestedId);
  if (alias) {
    const resolved = await resolveSymbolId(alias.newStableId, depth + 1, visited);
    if (resolved.symbol) {
      return {
        symbol: resolved.symbol,
        redirected: true,
        redirectedFrom: requestedId,
        redirectReason: alias.reason,
        redirectConfidence: alias.confidence
      };
    }
  }
  
  return { symbol: null, error: 'SYMBOL_NOT_FOUND' };
}
```

### 4.6 Alias Creation Strategy

```typescript
async function createAliasesOnRefresh(
  oldMappings: SymbolMapping[],
  newMappings: SymbolMapping[],
  repoStateId: string
): Promise<void> {
  const newByBackendId = indexBy(newMappings, m => m.backendStableId);
  
  for (const old of oldMappings) {
    // Skip if still exists
    if (newMappings.find(n => n.stableId === old.stableId)) continue;
    
    // Strategy 1: Backend ID match (high confidence)
    if (old.backendStableId) {
      const newByBackend = newByBackendId.get(old.backendStableId);
      if (newByBackend && newByBackend.stableId !== old.stableId) {
        await db.aliases.create({
          oldStableId: old.stableId,
          newStableId: newByBackend.stableId,
          reason: 'renamed',
          confidence: 0.95,
          createdStateId: repoStateId
        });
        continue;
      }
    }
    
    // Strategy 2: Fuzzy match (low confidence)
    const fuzzy = findFuzzyMatch(old, newMappings);
    if (fuzzy && fuzzy.confidence >= 0.6) {
      await db.aliases.create({
        oldStableId: old.stableId,
        newStableId: fuzzy.mapping.stableId,
        reason: 'fuzzy-match',
        confidence: fuzzy.confidence,
        createdStateId: repoStateId
      });
      continue;
    }
    
    // Mark as deleted (tombstone)
    await db.symbols.update(old.stableId, {
      state: 'deleted',
      deletedAt: new Date().toISOString(),
      deletedInStateId: repoStateId
    });
  }
}
```

---

## 5. Module System

### 5.1 Module Resolution Order

```
1. Explicit config     (.ckb/config.json modules.roots)
2. Manifest roots      (package.json, pubspec.yaml, go.mod, Cargo.toml)
3. Language conventions (src/, lib/, internal/, pkg/)
4. Directory fallback  (top-level directories)
```

### 5.2 Import Edge Classification

Import scanning classifies each dependency edge:

```typescript
type ImportEdgeKind = 
  | 'local-file'           // ./foo, ../bar
  | 'local-module'         // Same workspace, different package
  | 'workspace-package'    // Monorepo sibling
  | 'external-dependency'  // npm/pub/cargo package
  | 'stdlib'               // dart:core, node builtins
  | 'unknown';

interface ImportEdge {
  from: FileId;
  to: string;              // May be path or package name
  kind: ImportEdgeKind;
  confidence: number;
  rawImport: string;       // Original import string
}
```

**Classification logic**:
```typescript
function classifyImport(importStr: string, fromFile: FileId, context: ModuleContext): ImportEdgeKind {
  // Relative paths
  if (importStr.startsWith('./') || importStr.startsWith('../')) {
    return 'local-file';
  }
  
  // Stdlib patterns
  if (isStdlib(importStr, context.language)) {
    return 'stdlib';
  }
  
  // Check if it's a workspace package
  if (context.workspacePackages.has(importStr)) {
    return 'workspace-package';
  }
  
  // Check if it resolves to local module
  const resolved = resolveImport(importStr, fromFile, context);
  if (resolved && isWithinRepo(resolved, context.repoRoot)) {
    return 'local-module';
  }
  
  // External
  if (context.declaredDependencies.has(importStr)) {
    return 'external-dependency';
  }
  
  return 'unknown';
}
```

---

## 6. Backend System

### 6.1 Backend Ladder

```typescript
interface QueryPolicy {
  backendPreferenceOrder: ['scip', 'glean', 'lsp'];
  alwaysUse: ['git'];
  
  maxInFlightPerBackend: {
    scip: 10,
    lsp: 3,
    glean: 20,
    git: 5
  };
  
  coalesceWindowMs: 50;
  
  mergeMode: 'prefer-first' | 'union';
  supplementThreshold: 0.8;
  
  timeoutMs: {
    scip: 5000,
    lsp: 15000,
    glean: 10000,
    git: 5000
  };
}
```

### 6.2 Merge Strategy: Prefer-First

**Default mode.** Uses highest-preference backend; supplements metadata only.

#### Metadata Supplement Rules

**Allowed to enrich** (from equal-or-higher precedence backend):
- `visibility`
- `visibilityConfidence`
- `signatureNormalized`
- `signatureFull`
- `kind` (refinement only, e.g., `function` → `method`)
- `containerName`
- `moduleId`

**Forbidden in prefer-first mode**:
- Adding new references or call edges
- Changing locations
- Changing stableId mappings
- Overwriting higher-confidence data with lower-confidence

**Conflict resolution**:
- Precedence: SCIP > Glean > LSP
- If backends disagree on allowed fields, use higher-precedence
- Record in `provenance.metadataConflicts[]`

```typescript
async function supplementMetadata(
  primaryResult: BackendResult,
  symbolId: SymbolId,
  primaryBackend: string
): Promise<SupplementedResult> {
  const conflicts: string[] = [];
  const supplementSources: string[] = [];
  
  for (const item of primaryResult.data) {
    // Only supplement from equal-or-higher precedence
    const fallbacks = getFallbackBackends(primaryBackend);
    
    for (const fallbackId of fallbacks) {
      const fallback = this.backends.get(fallbackId);
      if (!fallback?.isAvailable()) continue;
      
      const enrichment = await fallback.getSymbolMetadata(item.symbolId);
      if (!enrichment) continue;
      
      // Supplement allowed fields
      for (const field of ALLOWED_SUPPLEMENT_FIELDS) {
        if (item[field] === undefined && enrichment[field] !== undefined) {
          item[field] = enrichment[field];
          supplementSources.push(fallbackId);
        } else if (item[field] !== enrichment[field] && enrichment[field] !== undefined) {
          conflicts.push(`${field}: ${primaryBackend}=${item[field]}, ${fallbackId}=${enrichment[field]}`);
        }
      }
    }
  }
  
  return { 
    data: primaryResult.data, 
    supplementSources: [...new Set(supplementSources)],
    conflicts
  };
}

const ALLOWED_SUPPLEMENT_FIELDS = [
  'visibility',
  'visibilityConfidence', 
  'signatureNormalized',
  'signatureFull',
  'kind',
  'containerName',
  'moduleId'
];
```

### 6.3 Merge Strategy: Union Mode

Explicit opt-in via `--merge=union`. Queries all backends, merges results.

**Conflict handling in union mode**:
```typescript
interface UnionConflict {
  field: string;
  values: { backend: string; value: any }[];
  resolved: any;          // What we picked
  resolvedBy: string;     // Which backend won
}

async function mergeUnionResults(results: BackendResult[]): Promise<MergedResult> {
  const merged = new Map<string, MergedItem>();
  const conflicts: UnionConflict[] = [];
  
  // Sort by backend precedence (highest first)
  results.sort((a, b) => BACKEND_PRECEDENCE[a.backendId] - BACKEND_PRECEDENCE[b.backendId]);
  
  for (const result of results) {
    for (const item of result.data) {
      const key = computeRefKey(item);
      const existing = merged.get(key);
      
      if (!existing) {
        merged.set(key, { ...item, sourceBackend: result.backendId });
      } else {
        // Check for conflicts
        for (const field of ['kind', 'visibility']) {
          if (existing[field] !== item[field]) {
            conflicts.push({
              field,
              values: [
                { backend: existing.sourceBackend, value: existing[field] },
                { backend: result.backendId, value: item[field] }
              ],
              resolved: existing[field],  // Higher precedence wins
              resolvedBy: existing.sourceBackend
            });
          }
        }
      }
    }
  }
  
  return {
    data: Array.from(merged.values()),
    conflicts,
    backends: results.map(r => r.backendId)
  };
}
```

### 6.4 LSP Supervisor

```typescript
interface LspSupervisor {
  // Hard cap on total LSP processes
  maxTotalProcesses: 4;
  
  // Per-process config
  processes: Map<string, LspProcess>;
  
  // Queue config
  queueSizePerLanguage: 10;
  maxQueueWaitMs: 200;
}

interface LspProcess {
  languageId: string;
  workspaceRoot: string;
  state: 'starting' | 'initializing' | 'ready' | 'unhealthy' | 'dead';
  
  // Health
  lastResponseTime?: string;
  consecutiveFailures: number;
  
  // Backoff
  restartCount: number;
  nextRestartAt?: string;
}

class LspSupervisorImpl {
  private readonly maxConsecutiveFailures = 3;
  private readonly baseBackoffMs = 1000;
  private readonly maxBackoffMs = 30000;
  
  async query<T>(languageId: string, method: string, params: any): Promise<T> {
    const process = this.processes.get(languageId);
    
    if (!process || process.state !== 'ready') {
      throw new CkbError('WORKSPACE_NOT_READY', `LSP ${languageId} not ready`);
    }
    
    // Check queue
    const queueSize = this.getQueueSize(languageId);
    if (queueSize >= this.queueSizePerLanguage) {
      // Try to wait briefly
      const waited = await this.waitForSlot(languageId, this.maxQueueWaitMs);
      if (!waited) {
        throw new CkbError('RATE_LIMITED', `LSP ${languageId} overloaded`, {
          drilldowns: [
            { label: 'Retry in 2s', query: `${method} --retry-after=2s` },
            { label: 'Check status', query: 'getStatus' },
            { label: 'Generate SCIP index', query: 'doctor --check=scip' }
          ]
        });
      }
    }
    
    try {
      const result = await this.executeWithTimeout(process, method, params);
      process.consecutiveFailures = 0;
      process.lastResponseTime = new Date().toISOString();
      return result;
    } catch (error) {
      process.consecutiveFailures++;
      if (process.consecutiveFailures >= this.maxConsecutiveFailures) {
        await this.handleCrash(languageId);
      }
      throw error;
    }
  }
  
  private async handleCrash(languageId: string): Promise<void> {
    const process = this.processes.get(languageId)!;
    process.state = 'dead';
    process.restartCount++;
    
    const backoffMs = Math.min(
      this.baseBackoffMs * Math.pow(2, process.restartCount - 1),
      this.maxBackoffMs
    );
    
    process.nextRestartAt = new Date(Date.now() + backoffMs).toISOString();
    setTimeout(() => this.restart(languageId), backoffMs);
  }
  
  // Eviction when at capacity
  private async ensureCapacity(): Promise<void> {
    if (this.processes.size < this.maxTotalProcesses) return;
    
    // LRU eviction
    const lru = [...this.processes.values()]
      .sort((a, b) => (a.lastResponseTime ?? '').localeCompare(b.lastResponseTime ?? ''))
      [0];
    
    await this.shutdown(lru.languageId);
  }
}
```

### 6.5 Completeness with Reasons

```typescript
type CompletenessReason =
  | 'full-backend'
  | 'best-effort-lsp'
  | 'workspace-not-ready'
  | 'timed-out'
  | 'truncated'
  | 'single-file-only'
  | 'no-backend-available'
  | 'index-stale'
  | 'unknown';

interface CompletenessInfo {
  score: number;
  reason: CompletenessReason;
  details?: string;
}
```

### 6.6 Index Freshness

```typescript
interface IndexFreshness {
  staleAgainstHead: boolean;       // indexedCommit !== HEAD
  staleAgainstRepoState: boolean;  // repoState.dirty
  commitsBehindHead: number;
  warning?: string;
}

function computeIndexFreshness(
  indexedCommit: string,
  repoState: RepoState
): IndexFreshness {
  const staleAgainstHead = indexedCommit !== repoState.headCommit;
  const commitsBehind = staleAgainstHead 
    ? parseInt(exec(`git rev-list --count ${indexedCommit}..${repoState.headCommit}`))
    : 0;
  
  let warning: string | undefined;
  if (staleAgainstHead) {
    warning = `SCIP index is ${commitsBehind} commits behind HEAD`;
  } else if (repoState.dirty) {
    warning = 'SCIP index matches HEAD but repo has uncommitted changes; results may miss local edits';
  }
  
  return {
    staleAgainstHead,
    staleAgainstRepoState: repoState.dirty,
    commitsBehindHead: commitsBehind,
    warning
  };
}
```

---

## 7. Compression Contract

### 7.1 Schema Versioning

```typescript
interface SchemaInfo {
  // Facts schemas are versioned and stable
  factsSchemaVersion: number;  // Breaking changes increment this
  
  // Views are derived, can change more freely
  viewsSchemaVersion: number;
}

// Current versions
const FACTS_SCHEMA_VERSION = 1;
const VIEWS_SCHEMA_VERSION = 1;
```

### 7.2 Response Structure

```typescript
interface CompressedResponse<TFacts> {
  // Schema info
  ckbVersion: string;
  schemaVersion: number;
  capabilities: string[];       // Active backends
  
  // Core data
  facts: TFacts;
  explanation: DerivedText;
  provenance: Provenance;
  drilldowns: Drilldown[];
  compression: CompressionMetrics;
}
```

### 7.3 Provenance Structure

```typescript
interface Provenance {
  repoStateId: string;
  repoStateDirty: boolean;
  repoStateMode: 'head' | 'full';
  
  backends: BackendContribution[];
  completeness: CompletenessInfo;
  
  // Freshness
  indexFreshness?: IndexFreshness;
  
  // Analysis limits
  analysisLimits?: AnalysisLimits;
  
  // Conflicts from merge
  metadataConflicts?: string[];
  
  // Timing (excluded from cache key comparison)
  cachedAt?: string;
  queryDurationMs: number;
  
  // Issues
  warnings: string[];
  timeouts: string[];
  truncations: string[];
}

interface AnalysisLimits {
  typeContext: 'full' | 'partial' | 'none';
  notes: string[];
}
```

---

## 8. Impact Analysis

### 8.1 Visibility Derivation

```typescript
function deriveVisibility(symbol: Symbol, refs: Reference[]): VisibilityInfo {
  // Cascading fallback per source-of-truth table
  
  // 1. SCIP/Glean modifiers (authoritative)
  if (symbol.source === 'scip' && symbol.modifiers) {
    return {
      visibility: parseScipVisibility(symbol.modifiers),
      confidence: 0.95,
      source: 'scip-modifiers'
    };
  }
  
  // 2. Reference analysis
  const externalRefs = refs.filter(r => 
    getModuleId(r.fromLocation) !== symbol.moduleId
  );
  if (externalRefs.length > 0) {
    return {
      visibility: 'public',
      confidence: 0.9,
      source: 'ref-analysis'
    };
  }
  
  // 3. Naming conventions (low confidence)
  if (symbol.name.startsWith('_') || symbol.name.startsWith('#')) {
    return {
      visibility: 'private',
      confidence: 0.6,
      source: 'naming-convention'
    };
  }
  
  return {
    visibility: 'unknown',
    confidence: 0.3,
    source: 'naming-convention'
  };
}
```

### 8.2 Impact Classification

Same as v4—deterministic rules based on ref kind and visibility.

### 8.3 Risk Score

Same as v4—uses only v1-available inputs.

---

## 9. Cache System

### 9.1 Cache Tiers

```typescript
interface CacheTiers {
  queryCache: {
    ttlSeconds: 300,
    keyIncludes: 'headCommit'
  };
  viewCache: {
    ttlSeconds: 3600,
    keyIncludes: 'repoStateId'
  };
  negativeCache: {
    ttlSeconds: 60,
    keyIncludes: 'repoStateId'
  };
}
```

### 9.2 Negative Cache Policy

```typescript
const NEGATIVE_CACHE_POLICY = {
  'symbol-not-found': { ttlSeconds: 60 },
  'backend-unavailable': { ttlSeconds: 15 },
  'workspace-not-ready': { ttlSeconds: 10, triggersAction: 'warmup' },
  'timeout': { ttlSeconds: 5 }
};
```

### 9.3 Cache Invalidation Triggers

| Cache Type | Invalidated By |
|------------|----------------|
| Query cache | HEAD change, backend restart, TTL expiry |
| View cache | RepoStateId mismatch, config hash mismatch, schema version change |
| Negative cache | RepoStateId change, backend status change |

---

## 10. Response Budget & Backend Limits

### 10.1 Response Budget

```typescript
interface ResponseBudget {
  maxModules: 10,
  maxSymbolsPerModule: 5,
  maxImpactItems: 20,
  maxDrilldowns: 5,
  estimatedMaxTokens: 4000
}
```

### 10.2 Backend Limits

Hard caps to prevent runaway queries:

```typescript
interface BackendLimits {
  // Per-query limits
  maxRefsPerQuery: 10000,
  maxSymbolsPerSearch: 1000,
  
  // Scan limits
  maxFilesScanned: 5000,
  maxFileSizeBytes: 1_000_000,
  
  // Union mode limits
  maxUnionModeTimeMs: 60000,
  
  // Memory limits
  maxScipIndexSizeMb: 500,  // Warn if larger
}
```

---

## 11. Partial Results Contract

### 11.1 Guaranteed Fields

Every response includes:

```typescript
interface PartialResultsContract {
  provenance: {
    repoStateId: string;
    completeness: CompletenessInfo;
    warnings: string[];
    timeouts: string[];
    truncations: string[];
  };
  drilldowns: Drilldown[];
}
```

### 11.2 Smart Drilldowns

Generated based on what was truncated or incomplete:

```typescript
function generateDrilldowns(context: DrilldownContext): Drilldown[] {
  const drilldowns: Drilldown[] = [];
  
  // Truncation-based
  if (context.truncationReason === 'max-modules') {
    drilldowns.push({
      label: `Explore top module: ${context.topModule.name}`,
      query: `getModuleOverview ${context.topModule.id}`
    });
  }
  
  if (context.truncationReason === 'max-items') {
    drilldowns.push({
      label: 'Scope to specific module',
      query: `findReferences ${context.symbolId} --scope=<moduleId>`
    });
  }
  
  // Completeness-based
  if (context.completeness.reason === 'best-effort-lsp') {
    drilldowns.push({
      label: 'Check workspace status',
      query: 'getStatus'
    });
  }
  
  if (context.completeness.reason === 'workspace-not-ready') {
    drilldowns.push({
      label: 'Retry after warmup',
      query: `findReferences ${context.symbolId} --wait-for-ready`
    });
  }
  
  if (context.completeness.score < 0.8) {
    drilldowns.push({
      label: 'Get maximum results (slower)',
      query: `findReferences ${context.symbolId} --merge=union`
    });
  }
  
  // Index freshness-based
  if (context.indexFreshness?.staleAgainstHead) {
    drilldowns.push({
      label: 'Regenerate SCIP index',
      query: 'doctor --check=scip'
    });
  }
  
  return drilldowns.slice(0, context.budget.maxDrilldowns);
}
```

---

## 12. Deterministic Output

### 12.1 Ordering Contract

All arrays are deterministically sorted.

| Array | Primary | Secondary | Tertiary |
|-------|---------|-----------|----------|
| modules | impactCount DESC | symbolCount DESC | moduleId ASC |
| symbols | confidence DESC | refCount DESC | stableId ASC |
| references | fileId ASC | startLine ASC | startColumn ASC |
| impactItems | kind priority | confidence DESC | stableId ASC |
| drilldowns | relevanceScore DESC | label ASC | — |
| warnings | severity DESC | text ASC | — |

### 12.2 JSON Encoding Rules

For byte-identical outputs:

1. **Stable key ordering**: Encode objects with sorted keys
2. **Float formatting**: Max 6 decimal places, no trailing zeros
3. **Timestamps**: Only in `provenance` block, excluded from snapshot tests
4. **Null handling**: Omit null/undefined fields rather than including them

```typescript
function deterministicEncode(obj: any): string {
  return JSON.stringify(obj, (key, value) => {
    if (value === null || value === undefined) return undefined;
    if (typeof value === 'number') {
      return Math.round(value * 1000000) / 1000000;
    }
    if (typeof value === 'object' && !Array.isArray(value)) {
      return Object.keys(value).sort().reduce((sorted, k) => {
        sorted[k] = value[k];
        return sorted;
      }, {} as any);
    }
    return value;
  });
}
```

### 12.3 Snapshot Test Exclusions

When comparing responses for tests, exclude:
- `provenance.cachedAt`
- `provenance.queryDurationMs`
- `provenance.computedAt`

---

## 13. Error Handling

### 13.1 Error Taxonomy

Stable error codes for all failure modes:

```typescript
type ErrorCode =
  | 'BACKEND_UNAVAILABLE'     // Backend not running/reachable
  | 'INDEX_MISSING'           // SCIP index not found
  | 'INDEX_STALE'             // SCIP index too old
  | 'WORKSPACE_NOT_READY'     // LSP still initializing
  | 'TIMEOUT'                 // Query timed out
  | 'RATE_LIMITED'            // Too many concurrent requests
  | 'SYMBOL_NOT_FOUND'        // Symbol doesn't exist
  | 'SYMBOL_DELETED'          // Symbol was deleted
  | 'SCOPE_INVALID'           // Invalid scope parameter
  | 'ALIAS_CYCLE'             // Circular alias chain
  | 'ALIAS_CHAIN_TOO_DEEP'    // Alias chain > max depth
  | 'BUDGET_EXCEEDED'         // Hit backend/response limits
  | 'INTERNAL_ERROR';         // Unexpected error

interface CkbError {
  code: ErrorCode;
  message: string;
  details?: any;
  suggestedFixes?: FixAction[];
  drilldowns?: Drilldown[];
}
```

### 13.2 Error to Action Mapping

Each error has suggested actions:

```typescript
const ERROR_ACTIONS: Record<ErrorCode, FixAction[]> = {
  'INDEX_MISSING': [
    { type: 'run-command', command: 'ckb doctor --check=scip', safe: true }
  ],
  'INDEX_STALE': [
    { type: 'run-command', command: '${detected_scip_command}', safe: true }
  ],
  'WORKSPACE_NOT_READY': [
    { type: 'run-command', command: 'ckb status --wait-for-ready', safe: true }
  ],
  'RATE_LIMITED': [
    { type: 'run-command', command: 'sleep 2 && ckb ${retry_command}', safe: true }
  ],
  // ...
};
```

---

## 14. Security & Privacy

### 14.1 Data Storage Rules

| Stored | NOT Stored |
|--------|------------|
| Symbol names + signatures | Source code content |
| File paths (repo-relative) | LSP hover text / docstrings |
| Reference locations | Absolute paths |
| Module structure | Secrets/credentials |
| Git commit hashes | Full commit messages |
| Import edges | Raw source lines |

**Explicit rule**: CKB may read file contents transiently for import scanning. It stores only extracted facts (import paths, counts, hashes). Raw source lines are never persisted. LSP hover text is used transiently and never stored.

### 14.2 Privacy Modes

```typescript
type PrivacyMode = 'normal' | 'anonymized';

interface PrivacyConfig {
  mode: PrivacyMode;
  hashSalt?: string;
}
```

---

## 15. Performance

### 15.1 Import Scanning

```typescript
interface ImportScanPolicy {
  enabled: true,
  maxFileSizeBytes: 1_000_000,
  scanTimeoutMs: 30_000,
  maxFilesPerModule: 10_000,
  skipBinary: true,
  ignoreDirs: ['node_modules', 'vendor', 'build', '.dart_tool']
}
```

### 15.2 Built-in Import Patterns

```typescript
const IMPORT_PATTERNS: Record<string, LanguagePattern> = {
  typescript: {
    extensions: ['.ts', '.tsx', '.js', '.jsx'],
    patterns: [
      /import\s+.*?from\s+['"]([^'"]+)['"]/g,
      /export\s+.*?from\s+['"]([^'"]+)['"]/g,
      /require\s*\(\s*['"]([^'"]+)['"]\s*\)/g,
      /import\s*\(\s*['"]([^'"]+)['"]\s*\)/g  // Dynamic import
    ]
  },
  dart: {
    extensions: ['.dart'],
    patterns: [
      /import\s+['"]([^'"]+)['"]/g,
      /export\s+['"]([^'"]+)['"]/g
    ]
  },
  // ... other languages
};
```

### 15.3 Performance Targets

| Metric | Target |
|--------|--------|
| Cached view | < 100ms |
| Warm query (SCIP) | < 500ms |
| Warm query (LSP) | < 2s |
| Cold query | < 30s |
| Architecture (1000 files) | < 30s |
| Import scan (1000 files) | < 10s |

---

## 16. V1 MVP Specification

### 16.1 Tools

| Tool | Description | RepoStateMode |
|------|-------------|---------------|
| `getStatus` | System health, backends, cache | head |
| `doctor` | Diagnose issues, suggest fixes | head |
| `getSymbol` | Symbol metadata + location | head (dirty-aware) |
| `searchSymbols` | Find symbols by name | head |
| `findReferences` | Compressed refs with completeness | full |
| `getArchitecture` | Module map + dependencies | full |
| `analyzeImpact` | Impact classification | full |

### 16.2 Symbol Search Semantics

```typescript
interface SearchOptions {
  query: string;
  scope?: ModuleId;
  kinds?: SymbolKind[];
  limit?: number;  // Default: 20
}

interface SearchRanking {
  // 1. Exact match bonus
  exactMatchBonus: 100,
  
  // 2. Visibility (public surfaces first)
  visibilityWeights: {
    public: 30,
    internal: 20,
    private: 10,
    unknown: 5
  },
  
  // 3. Backend confidence
  confidenceWeight: 20,
  
  // 4. Symbol kind priority
  kindWeights: {
    class: 25,
    interface: 25,
    function: 20,
    method: 15,
    property: 10,
    variable: 5
  }
}

// Tie-breaker: stableId ASC
```

**V1 match type**: Substring, case-insensitive. No fuzzy matching yet.

### 16.3 Doctor Response

```typescript
interface DoctorResponse {
  healthy: boolean;
  checks: DoctorCheck[];
}

interface DoctorCheck {
  name: string;
  status: 'pass' | 'warn' | 'fail';
  message: string;
  suggestedFixes?: FixAction[];
}

type FixAction =
  | { type: 'run-command'; command: string; safe: boolean; description: string }
  | { type: 'open-docs'; url: string }
  | { type: 'install-tool'; tool: string; methods: ('brew' | 'npm' | 'cargo' | 'manual')[] };
```

**`ckb doctor --fix` behavior**: Outputs script (bash/PowerShell), does NOT auto-execute.

### 16.4 getSymbol Response

```typescript
interface GetSymbolResponse extends CompressedResponse<{
  symbol: {
    stableId: string;
    name: string;
    kind: SymbolKind;
    signature?: string;
    signatureNormalized?: string;
    visibility: VisibilityInfo;
    moduleId: ModuleId;
    moduleName: string;
    containerName?: string;
    location: Location;
    locationFreshness: 'fresh' | 'may-be-stale';
    
    // Version info
    definitionVersionId?: string;
    definitionVersionSemantics: string;
    
    // Backend info
    backendMappings: { backend: string; nativeId: string }[];
  } | null;
  
  // If redirected
  redirected?: boolean;
  redirectedFrom?: string;
  redirectReason?: string;
  
  // If deleted
  deleted?: boolean;
  deletedAt?: string;
}> {}
```

### 16.5 getArchitecture Response

```typescript
interface GetArchitectureResponse extends CompressedResponse<{
  modules: ModuleSummary[];
  dependencyGraph: DependencyEdge[];
  entrypoints: Entrypoint[];
}> {}

interface DependencyEdge {
  from: ModuleId;
  to: ModuleId;
  kind: ImportEdgeKind;  // Classified
  strength: number;      // Reference count
}
```

**Default**: `includeExternalDeps=false`. Externals filtered but classified internally.

### 16.6 Logging & Observability

```typescript
interface LogConfig {
  format: 'json' | 'human';  // Default: human for CLI, json for daemon
  level: 'debug' | 'info' | 'warn' | 'error';  // Default: info
  output: 'stdout' | 'file';
  file?: string;
}
```

**Diagnostic bundle**: `ckb diag --out bundle.zip`
- Includes: config (sanitized), doctor output, backend status, recent errors
- Excludes: source code, symbol names (if anonymized mode)

---

## 17. Implementation Plan

### Phase 1: Foundation (Weeks 1-2)

#### Task 1.1: Project Setup
- [ ] Go module + cobra/viper
- [ ] Config + `.ckb/` management
- [ ] RepoState computation
- [ ] Logging (structured JSON + human)
- [ ] Error taxonomy

**DoD**: `ckb init`, `ckb version`, `ckb --help` work

#### Task 1.2: Identity System
- [ ] Stable fingerprint
- [ ] Definition version ID (optional)
- [ ] Mapping table + tombstones
- [ ] Alias table + chain resolution

**DoD**: IDs survive renames, deleted symbols return tombstone

#### Task 1.3: Module Detection + Import Classification
- [ ] Manifest scanner
- [ ] Import edge classification
- [ ] Path canonicalization

**DoD**: `ckb modules` lists with edge classification

#### Task 1.4: Storage Layer
- [ ] SQLite (pure Go)
- [ ] Cache tiers
- [ ] Negative cache
- [ ] Dependency index

**DoD**: Cache invalidation works correctly

---

### Phase 2: Backends (Weeks 3-4)

#### Task 2.1: Backend Interface + Ladder
- [ ] Adapter interface
- [ ] Query policy
- [ ] Merge strategy (prefer-first)
- [ ] Metadata supplement rules

**DoD**: SCIP-first selection, supplement works

#### Task 2.2: SCIP Adapter
- [ ] Protobuf parsing
- [ ] Index freshness
- [ ] Stable ID extraction

**DoD**: Full completeness on SCIP repos

#### Task 2.3: LSP Supervisor
- [ ] Process management
- [ ] Health tracking
- [ ] Queue + reject-fast
- [ ] Crash recovery

**DoD**: LSP restarts cleanly, rate limiting works

#### Task 2.4: Git Adapter
- [ ] RepoState (full working tree)
- [ ] History, churn

**DoD**: Detects all uncommitted changes

---

### Phase 3: Comprehension (Weeks 5-6)

#### Task 3.1: Deterministic Output
- [ ] Sort functions
- [ ] JSON encoding
- [ ] Snapshot test helpers

**DoD**: Same query twice = identical bytes

#### Task 3.2: Compression + Drilldowns
- [ ] Response budget
- [ ] Smart drilldown generation
- [ ] Deduplication

**DoD**: Drilldowns are contextual

#### Task 3.3: Impact Analyzer
- [ ] Visibility derivation
- [ ] Classification
- [ ] Risk score
- [ ] Analysis limits

**DoD**: `analyzeImpact` works with honest limits

#### Task 3.4: Architecture Generator
- [ ] Import scanning
- [ ] External filtering
- [ ] Backend limits

**DoD**: `getArchitecture` < 30s on 1000 files

---

### Phase 4: API Layer (Weeks 7-8)

#### Task 4.1: Core Tools
- [ ] `getStatus`
- [ ] `doctor` with fix actions
- [ ] `getSymbol` (dirty-aware)
- [ ] `searchSymbols`
- [ ] `findReferences`
- [ ] `getArchitecture`
- [ ] `analyzeImpact`

**DoD**: All tools work via CLI

#### Task 4.2: HTTP API
- [ ] REST endpoints
- [ ] OpenAPI spec

#### Task 4.3: MCP Server
- [ ] Tool definitions
- [ ] Resource definitions
- [ ] Capabilities handshake

**DoD**: Claude Code can query via MCP

---

### Phase 5: Polish (Weeks 9-10)

#### Task 5.1: Testing
- [ ] Unit tests (>80%)
- [ ] Determinism tests
- [ ] Integration tests
- [ ] Cross-platform (macOS + Windows)

#### Task 5.2: Documentation
- [ ] README
- [ ] Config reference
- [ ] MCP docs

#### Task 5.3: Distribution
- [ ] Multi-platform builds
- [ ] `ckb diag` bundle
- [ ] Homebrew formula

---

## Appendices

### Appendix A: MCP Schemas

```json
{
  "tools": [
    {
      "name": "getStatus",
      "description": "Get CKB system status",
      "inputSchema": { "type": "object", "properties": {} }
    },
    {
      "name": "doctor", 
      "description": "Diagnose issues and get fix suggestions",
      "inputSchema": { "type": "object", "properties": {} }
    },
    {
      "name": "getSymbol",
      "description": "Get symbol metadata and location",
      "inputSchema": {
        "type": "object",
        "properties": {
          "symbolId": { "type": "string" },
          "repoStateMode": { "type": "string", "enum": ["head", "full"], "default": "head" }
        },
        "required": ["symbolId"]
      }
    },
    {
      "name": "searchSymbols",
      "description": "Search for symbols by name",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" },
          "scope": { "type": "string" },
          "kinds": { "type": "array", "items": { "type": "string" } },
          "limit": { "type": "number", "default": 20 }
        },
        "required": ["query"]
      }
    },
    {
      "name": "findReferences",
      "description": "Find references to a symbol",
      "inputSchema": {
        "type": "object",
        "properties": {
          "symbolId": { "type": "string" },
          "scope": { "type": "string" },
          "merge": { "type": "string", "enum": ["prefer-first", "union"], "default": "prefer-first" },
          "limit": { "type": "number", "default": 100 }
        },
        "required": ["symbolId"]
      }
    },
    {
      "name": "getArchitecture",
      "description": "Get codebase architecture",
      "inputSchema": {
        "type": "object",
        "properties": {
          "depth": { "type": "number", "default": 2 },
          "includeExternalDeps": { "type": "boolean", "default": false },
          "refresh": { "type": "boolean", "default": false }
        }
      }
    },
    {
      "name": "analyzeImpact",
      "description": "Analyze impact of changing a symbol",
      "inputSchema": {
        "type": "object",
        "properties": {
          "symbolId": { "type": "string" },
          "depth": { "type": "number", "default": 2 }
        },
        "required": ["symbolId"]
      }
    }
  ],
  "resources": [
    { "uri": "ckb://status", "name": "System Status" },
    { "uri": "ckb://architecture", "name": "Architecture" },
    { "uriTemplate": "ckb://module/{moduleId}", "name": "Module" },
    { "uriTemplate": "ckb://symbol/{symbolId}", "name": "Symbol" }
  ]
}
```

### Appendix B: Configuration Reference

```json
{
  "version": 5,
  "repoRoot": ".",
  
  "backends": {
    "scip": {
      "enabled": true,
      "indexPath": ".scip/index.scip"
    },
    "lsp": {
      "enabled": true,
      "workspaceStrategy": "repo-root",
      "servers": {
        "typescript": { "command": "typescript-language-server", "args": ["--stdio"] },
        "dart": { "command": "dart", "args": ["language-server"] }
      }
    },
    "git": { "enabled": true }
  },
  
  "queryPolicy": {
    "backendPreferenceOrder": ["scip", "glean", "lsp"],
    "alwaysUse": ["git"],
    "maxInFlightPerBackend": { "scip": 10, "lsp": 3, "git": 5 },
    "coalesceWindowMs": 50,
    "mergeMode": "prefer-first",
    "supplementThreshold": 0.8,
    "timeoutMs": { "scip": 5000, "lsp": 15000, "git": 5000 }
  },
  
  "lspSupervisor": {
    "maxTotalProcesses": 4,
    "queueSizePerLanguage": 10,
    "maxQueueWaitMs": 200
  },
  
  "modules": {
    "detection": "auto",
    "roots": [],
    "ignore": ["node_modules", "build", ".dart_tool", "vendor"]
  },
  
  "importScan": {
    "enabled": true,
    "maxFileSizeBytes": 1000000,
    "scanTimeoutMs": 30000,
    "customPatterns": {}
  },
  
  "cache": {
    "queryTtlSeconds": 300,
    "viewTtlSeconds": 3600,
    "negativeTtlSeconds": 60
  },
  
  "budget": {
    "maxModules": 10,
    "maxSymbolsPerModule": 5,
    "maxImpactItems": 20,
    "maxDrilldowns": 5,
    "estimatedMaxTokens": 4000
  },
  
  "backendLimits": {
    "maxRefsPerQuery": 10000,
    "maxFilesScanned": 5000,
    "maxUnionModeTimeMs": 60000
  },
  
  "privacy": {
    "mode": "normal"
  },
  
  "logging": {
    "format": "human",
    "level": "info"
  }
}
```

### Appendix C: CLI Commands

```bash
# Setup
ckb init                          # Create .ckb/ config
ckb version                       # Show version

# Diagnostics
ckb status                        # System status
ckb doctor                        # Check for issues
ckb doctor --fix                  # Output fix script
ckb diag --out bundle.zip         # Export diagnostic bundle

# Queries
ckb symbol <symbolId>             # Get symbol info
ckb search <query>                # Search symbols
ckb refs <symbolId>               # Find references
ckb refs <symbolId> --merge=union # Union mode
ckb arch                          # Architecture view
ckb impact <symbolId>             # Impact analysis

# Service
ckb serve [--port 8080]           # HTTP API
ckb mcp [--stdio]                 # MCP server

# Cache
ckb cache warm                    # Pre-warm cache
ckb cache clear                   # Clear cache
```

---

*End of design document v5.*
# CKB v6.0 — Architectural Memory

*From query engine to living knowledge base*

---

**Theme:** Persistent architectural understanding that survives across sessions  
**Non-goal:** Code generation, enforcement, automated refactoring

v6.0 transforms CKB from a stateless query engine into a **living knowledge base** that accumulates and maintains architectural understanding over time.

> **Positioning:** v5.x answers "what is this code?" — v6.0 answers "what does this codebase *know about itself*?"

---

## The Problem v6.0 Solves

Every development team experiences the same knowledge decay:

| What gets lost | Why it matters |
|----------------|----------------|
| Module boundaries | New code lands in wrong places |
| Ownership | PRs get assigned to wrong reviewers |
| Responsibilities | Duplicate implementations emerge |
| Hot spots | Risky areas get insufficient review |
| Design decisions | Same debates repeat every 6 months |

**Current state:** This knowledge exists in heads, scattered docs, and tribal memory. When people leave, it leaves with them.

**v6.0 state:** CKB maintains a persistent architectural model that learns from code, git history, and explicit annotations — and exposes it through MCP tools.

---

## 0. Platform Contracts

### 0.1 Persistence Model

v6.0 introduces **persistent state** that survives across sessions.

```
~/.ckb/
├── config.toml              # global config
└── repos/
    └── <repo-hash>/
        ├── ckb.db            # unified SQLite database (all tables)
        ├── decisions/        # ADR markdown files (canonical source)
        │   ├── ADR-001-*.md
        │   └── ...
        └── index.scip        # existing SCIP index
```

#### Unified Database Schema

Single `ckb.db` with versioned tables:

```sql
-- Schema version tracking
CREATE TABLE schema_versions (
    table_name TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    migrated_at TEXT NOT NULL
);

-- Modules table
CREATE TABLE modules (
    id TEXT PRIMARY KEY,           -- stable identifier (see 0.5)
    name TEXT NOT NULL,
    paths TEXT NOT NULL,           -- JSON array of globs
    boundaries TEXT,               -- JSON: {public: [], internal: []}
    responsibility TEXT,
    owner_ref TEXT,
    tags TEXT,                     -- JSON array
    source TEXT NOT NULL,          -- "declared" | "inferred"
    confidence REAL NOT NULL,
    confidence_basis TEXT,         -- JSON array
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_modules_source ON modules(source);

-- Ownership table
CREATE TABLE ownership (
    id INTEGER PRIMARY KEY,
    pattern TEXT NOT NULL,         -- glob pattern
    owners TEXT NOT NULL,          -- JSON array of Owner objects
    scope TEXT NOT NULL,           -- "maintainer" | "reviewer" | "contributor"
    source TEXT NOT NULL,          -- "codeowners" | "git-blame" | "declared" | "inferred"
    confidence REAL NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_ownership_pattern ON ownership(pattern);

-- Ownership history (append-only)
CREATE TABLE ownership_history (
    id INTEGER PRIMARY KEY,
    pattern TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    event TEXT NOT NULL,           -- "added" | "removed" | "promoted" | "demoted"
    reason TEXT,
    recorded_at TEXT NOT NULL
);
CREATE INDEX idx_ownership_history_pattern ON ownership_history(pattern);

-- Hotspot snapshots (time-series, append-only)
CREATE TABLE hotspot_snapshots (
    id INTEGER PRIMARY KEY,
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL,     -- "file" | "module" | "symbol"
    snapshot_date TEXT NOT NULL,
    churn_commits_30d INTEGER,
    churn_commits_90d INTEGER,
    churn_authors_30d INTEGER,
    complexity_cyclomatic REAL,
    complexity_cognitive REAL,
    coupling_afferent INTEGER,
    coupling_efferent INTEGER,
    coupling_instability REAL,
    score REAL NOT NULL
);
CREATE INDEX idx_hotspot_target ON hotspot_snapshots(target_id, snapshot_date);

-- Responsibilities table
CREATE TABLE responsibilities (
    id INTEGER PRIMARY KEY,
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL,     -- "module" | "file" | "symbol"
    summary TEXT NOT NULL,
    capabilities TEXT,             -- JSON array
    source TEXT NOT NULL,          -- "declared" | "inferred" | "llm-generated"
    confidence REAL NOT NULL,
    updated_at TEXT NOT NULL,
    verified_at TEXT               -- human verification timestamp
);
CREATE INDEX idx_responsibilities_target ON responsibilities(target_id);

-- Decisions index (metadata only; content in markdown files)
CREATE TABLE decisions (
    id TEXT PRIMARY KEY,           -- "ADR-001" style
    title TEXT NOT NULL,
    status TEXT NOT NULL,          -- "proposed" | "accepted" | "deprecated" | "superseded"
    affected_modules TEXT,         -- JSON array of module IDs
    file_path TEXT NOT NULL,       -- relative path to .md file
    author TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_decisions_status ON decisions(status);
CREATE INDEX idx_decisions_modules ON decisions(affected_modules);

-- FTS5 for text search
CREATE VIRTUAL TABLE decisions_fts USING fts5(
    id, title, content,
    content='decisions',
    content_rowid='rowid'
);

CREATE VIRTUAL TABLE responsibilities_fts USING fts5(
    target_id, summary, capabilities,
    content='responsibilities',
    content_rowid='rowid'
);
```

#### Data Classification

| Entity | Classification | Rebuild Strategy |
|--------|---------------|------------------|
| Declared modules | **Canonical** | Parse from MODULES.toml |
| Inferred modules | Derived | Regenerate from SCIP + git |
| CODEOWNERS ownership | **Canonical** | Parse from CODEOWNERS |
| Git-blame ownership | Derived | Regenerate from git |
| Annotations | **Canonical** | Never rebuild; preserve |
| Hotspot snapshots | Derived (append-only) | Regenerate; keep history |
| ADR files | **Canonical** | Never rebuild; preserve |
| ADR index | Derived | Regenerate from files |
| Responsibilities (declared) | **Canonical** | Never rebuild |
| Responsibilities (inferred) | Derived | Regenerate |

> **Rebuild rule:** Only derived data can be regenerated. Canonical data is preserved across rebuilds and migrations.

#### Storage Principles

| Principle | Rule |
|-----------|------|
| Single database | One `ckb.db` per repo for simpler migrations |
| Append-only history | History tables never updated, only appended |
| Canonical source separation | Human-authored content in files, not DB |
| Portable | SQLite + markdown = works everywhere |
| Versioned | Per-table schema versions; auto-migration on upgrade |

### 0.2 Concurrency & Locking

```typescript
interface LockContract {
  // Per-repo write lock
  lockFile: "~/.ckb/repos/<hash>/ckb.lock"
  
  // Rules
  rules: {
    singleWriter: true           // only one write operation at a time
    multipleReaders: true        // reads don't block each other
    readsDuringRefresh: true     // reads see last-committed state
    refreshTimeout: "5m"         // auto-release if process dies
  }
}
```

**Implementation:**
- Use SQLite WAL mode for concurrent reads during writes
- File-based lock (`ckb.lock`) for write operations
- Lock includes PID + timestamp for stale lock detection

### 0.3 Learning Modes

v6.0 data can be populated through three modes:

| Mode | Source | Confidence | Persistence |
|------|--------|------------|-------------|
| **Inferred** | Static analysis + git history | 0.69 max | Regenerated on refresh |
| **Observed** | Runtime telemetry (optional) | 0.89 max | Accumulated over time |
| **Declared** | Explicit annotations (CODEOWNERS, docs) | 1.0 | Persisted until changed |

**Composition rule:** When sources conflict, `declared > observed > inferred`.

### 0.4 Confidence Computation (Extended)

Building on v5.2 confidence rules:

| Condition | Cap |
|-----------|-----|
| Declared by human (CODEOWNERS, annotations) | 1.0 |
| Observed from runtime + static analysis | 0.89 |
| Inferred from static analysis only | 0.79 |
| Inferred from git history only | 0.69 |
| Heuristics only (naming, location) | 0.59 |
| Speculative / insufficient data | 0.39 |

**Composable confidence formula:**

```typescript
confidence = min(
  capForSource(source),
  baseScore * evidenceMultiplier
)

// Evidence multipliers by domain:
const evidenceMultipliers = {
  ownership: {
    blameLinesCovered: 0.5,    // % of file covered by blame
    commitRecency: 0.3,        // recent commits weighted higher
    authorConsistency: 0.2     // single author vs many
  },
  responsibilities: {
    docCommentPresent: 0.4,
    symbolCoverage: 0.3,       // % of exports documented
    readmePresent: 0.3
  },
  modules: {
    explicitBoundary: 0.5,     // paths explicitly declared
    importCohesion: 0.3,       // internal imports vs external
    namingConsistency: 0.2
  }
}
```

### 0.5 Stable Identifier Rules

Stable IDs are critical for history continuity and cross-linking.

| Entity | ID Generation | Rename Handling |
|--------|--------------|-----------------|
| Declared modules | `id` field from MODULES.toml | Manual update required |
| Inferred modules | `mod_` + `sha256(normalized_root_path)[:12]` | Mapping table tracks renames |
| Files | Relative path from repo root | Git rename detection |
| Symbols | SCIP symbol ID | Follows SCIP conventions |
| Decisions | Sequential `ADR-NNN` | Never renumbered |
| Owners | `@username` or `@org/team` or email | Canonical form normalized |

**Inferred module ID stability:**

```sql
-- Rename tracking table (in ckb.db)
CREATE TABLE module_renames (
    old_id TEXT NOT NULL,
    new_id TEXT NOT NULL,
    renamed_at TEXT NOT NULL,
    reason TEXT               -- "directory_rename" | "manual" | "merge"
);
CREATE INDEX idx_module_renames_old ON module_renames(old_id);
```

When a directory is renamed, CKB:
1. Detects via git rename detection
2. Creates mapping in `module_renames`
3. Updates `modules.id` to new value
4. Preserves history links via mapping table

### 0.6 Ownership Algorithm

**Git-blame ownership computation:**

```typescript
interface BlameOwnershipConfig {
  // Time decay: recent commits matter more
  timeDecayHalfLife: 90,       // days; commits older than this count half
  
  // Filtering
  excludeBots: true,           // filter commits from known bots
  excludeMergeCommits: true,   // ignore merge commits
  botPatterns: [               // regex patterns for bot detection
    /\[bot\]$/,
    /^dependabot/,
    /^renovate/
  ],
  
  // Thresholds for scope assignment
  thresholds: {
    maintainer: 0.50,          // >= 50% weighted contribution
    reviewer: 0.20,            // >= 20% weighted contribution
    contributor: 0.05          // >= 5% weighted contribution
  }
}

function computeOwnership(file: string, config: BlameOwnershipConfig): Owner[] {
  const blame = gitBlame(file)
  const weights = new Map<string, number>()
  
  for (const line of blame.lines) {
    if (config.excludeBots && isBot(line.author, config.botPatterns)) continue
    if (config.excludeMergeCommits && line.isMergeCommit) continue
    
    const age = daysSince(line.commitDate)
    const decay = Math.pow(0.5, age / config.timeDecayHalfLife)
    
    weights.set(line.author, (weights.get(line.author) || 0) + decay)
  }
  
  // Normalize to 0-1
  const total = sum(weights.values())
  const normalized = mapValues(weights, w => w / total)
  
  // Assign scopes
  return sortByWeight(normalized).map(([author, weight]) => ({
    type: inferOwnerType(author),  // user | team | email
    id: normalizeOwnerId(author),
    weight,
    scope: weight >= config.thresholds.maintainer ? "maintainer" :
           weight >= config.thresholds.reviewer ? "reviewer" : "contributor"
  }))
}
```

**CODEOWNERS + blame interaction:**

| Scenario | Behavior |
|----------|----------|
| CODEOWNERS exists | Team from CODEOWNERS; individuals from blame within team |
| CODEOWNERS missing | Pure blame-based ownership |
| Blame insufficient (<100 lines) | Fall back to directory-level ownership |
| Conflict | CODEOWNERS wins for team; blame wins for individuals |

### 0.7 Structured Limitations

All tool responses include typed limitations:

```typescript
type Limitation =
  | { type: "stale_data"; scope: string; dataAge: Duration }
  | { type: "partial_language_support"; language: string; missing: string[] }
  | { type: "no_codeowners"; fallback: "git-blame" | "heuristic" }
  | { type: "low_confidence_inference"; targetId: string; confidence: number }
  | { type: "truncated_results"; requested: number; returned: number; reason: string }
  | { type: "missing_backend"; backend: string; impact: string }
  | { type: "complexity_unavailable"; reason: string }

interface LimitationsResponse {
  limitations: Limitation[]
  overallConfidence: number    // min of all component confidences
}
```

### 0.8 Staleness Model

Persistent data can become stale:

```typescript
interface StalenessInfo {
  dataAge: Duration              // time since last update
  codeChanges: number            // commits since last update
  staleness: "fresh" | "aging" | "stale" | "obsolete"
  refreshRecommended: boolean
}
```

| Staleness | Condition | Action |
|-----------|-----------|--------|
| fresh | < 7 days, < 50 commits | Use as-is |
| aging | 7-30 days or 50-200 commits | Use with warning |
| stale | 30-90 days or 200-500 commits | Suggest refresh |
| obsolete | > 90 days or > 500 commits | Require refresh |

### 0.9 Language Support (v6.0)

v6.0 provides tiered language support:

| Tier | Languages | Capabilities |
|------|-----------|--------------|
| **Full** | Go | Modules, ownership, complexity, coupling, responsibilities |
| **Good** | TypeScript, JavaScript, Python | Modules, ownership, churn; complexity via tree-sitter |
| **Basic** | Java, Rust, C, C++ | Modules, ownership, churn only |
| **Minimal** | Others | Ownership + churn from git; no structural analysis |

**Complexity/coupling sources by tier:**

| Metric | Full (Go) | Good (TS/JS/Py) | Basic | Minimal |
|--------|-----------|-----------------|-------|---------|
| Churn | git log | git log | git log | git log |
| Cyclomatic complexity | SCIP + go/ast | tree-sitter | ❌ | ❌ |
| Cognitive complexity | go/ast heuristics | tree-sitter heuristics | ❌ | ❌ |
| Afferent coupling | SCIP imports | SCIP/regex imports | ❌ | ❌ |
| Efferent coupling | SCIP references | SCIP/regex references | ❌ | ❌ |

**Polyglot repo handling:**
- Metrics computed per-language where available
- Missing metrics surfaced as `{ type: "complexity_unavailable", reason: "language_not_supported" }`
- Hotspot scores computed from available metrics only (churn always available)

### 0.10 LLM Privacy Contract

When `llm-generated` responsibilities are enabled:

| Concern | Guarantee |
|---------|-----------|
| Data location | Code snippets sent to configured LLM endpoint only |
| Storage | Only generated summaries stored; prompts not persisted |
| Opt-in | Requires explicit `ckb config set llm.enabled true` |
| Redaction | Literals and secrets stripped before sending (configurable) |
| Local option | Supports local models via Ollama/llama.cpp endpoint |

```toml
# config.toml
[llm]
enabled = false                    # must explicitly enable
endpoint = "http://localhost:11434" # default: local Ollama
model = "codellama:13b"
redact_literals = true             # strip string/number literals
redact_secrets = true              # strip env vars, API keys
max_context_lines = 100            # limit code sent per request
```

---

## 1. Architectural Memory Core

### 1.1 Module Registry

**Purpose:** Maintain canonical list of modules with boundaries and metadata

#### Schema

```typescript
interface Module {
  id: string                     // stable identifier
  name: string                   // human-readable name
  paths: string[]                // file patterns (glob)
  boundaries: {
    public: string[]             // exported symbols/paths
    internal: string[]           // internal-only symbols/paths
  }
  responsibility: string         // one-sentence description
  owner: OwnerRef               // link to ownership
  tags: string[]                 // e.g., ["core", "deprecated", "experimental"]
  
  // Metadata
  source: "declared" | "inferred"
  confidence: number
  lastUpdated: ISO8601
}
```

#### Population Sources

| Source | Priority | How |
|--------|----------|-----|
| `MODULES.toml` / `modules.yaml` | 1 | Explicit declaration |
| Go packages (`go.mod` + directories) | 2 | Package structure |
| Import clusters | 3 | Files that import each other |
| Directory structure | 4 | Top-level directories |

### 1.2 Ownership Registry

**Purpose:** Map code paths to responsible humans/teams

#### Schema

```typescript
interface OwnershipEntry {
  pattern: string               // glob pattern
  owners: Owner[]               // ordered by priority
  scope: "maintainer" | "reviewer" | "contributor"
  source: "codeowners" | "git-blame" | "declared" | "inferred"
  confidence: number
  lastUpdated: ISO8601
}

interface Owner {
  type: "user" | "team" | "email"
  id: string                    // @username, @org/team, email
  weight: number                // 0.0-1.0, contribution weight
}
```

#### Population Sources

| Source | Priority | Confidence |
|--------|----------|------------|
| `CODEOWNERS` file | 1 | 1.0 |
| Explicit annotations | 2 | 1.0 |
| Git blame (> 50% of lines) | 3 | 0.79 |
| Recent commits (> 70% in 90 days) | 4 | 0.69 |
| Heuristic (directory name → team) | 5 | 0.59 |

### 1.3 Responsibility Map

**Purpose:** Track what each module/file is responsible for

#### Schema

```typescript
interface Responsibility {
  targetId: string              // module or file ID
  targetType: "module" | "file" | "symbol"
  summary: string               // one-sentence description
  capabilities: string[]        // what it can do
  dependencies: string[]        // what it needs
  consumers: string[]           // who uses it
  
  source: "declared" | "inferred" | "llm-generated"
  confidence: number
  lastUpdated: ISO8601
  lastVerified?: ISO8601        // human verification timestamp
}
```

#### Generation Strategy

| Source | Method | Confidence |
|--------|--------|------------|
| Doc comments | Extract from `// Package X does Y` | 0.89 |
| README.md | Parse module-level docs | 0.89 |
| LLM summary | Generate from code (optional, requires consent) | 0.69 |
| Symbol analysis | Infer from exports + dependencies | 0.59 |

### 1.4 Hotspot Tracker

**Purpose:** Identify areas that deserve extra attention

#### Schema

```typescript
interface Hotspot {
  targetId: string
  targetType: "file" | "module" | "symbol"
  
  metrics: {
    churn: ChurnMetrics
    complexity: ComplexityMetrics
    coupling: CouplingMetrics
    defects: DefectMetrics       // if issue tracker integrated
  }
  
  score: number                  // 0.0-1.0 composite hotspot score
  trend: "increasing" | "stable" | "decreasing"
  ranking: Ranking
}

interface ChurnMetrics {
  commits30d: number
  commits90d: number
  commits365d: number
  authors30d: number             // distinct authors
  linesChanged30d: number
}

interface ComplexityMetrics {
  cyclomaticComplexity: number
  cognitiveComplexity: number
  linesOfCode: number
  dependencyCount: number
}

interface CouplingMetrics {
  afferentCoupling: number       // incoming dependencies
  efferentCoupling: number       // outgoing dependencies
  instability: number            // efferent / (afferent + efferent)
  coChangeFrequency: Map<string, number>  // files that change together
}
```

#### Hotspot Scoring

```typescript
function computeHotspotScore(h: Hotspot): number {
  const churnScore = normalize(h.metrics.churn.commits30d, 0, 50)
  const complexityScore = normalize(h.metrics.complexity.cyclomaticComplexity, 0, 50)
  const couplingScore = normalize(h.metrics.coupling.instability, 0, 1)
  
  return (
    churnScore * 0.4 +
    complexityScore * 0.35 +
    couplingScore * 0.25
  )
}
```

### 1.5 Decision Log

**Purpose:** Capture architectural decisions that explain "why"

#### Schema

```typescript
interface ArchitecturalDecision {
  id: string                     // ADR-001 style
  title: string
  status: "proposed" | "accepted" | "deprecated" | "superseded"
  context: string                // why was this decision needed?
  decision: string               // what was decided?
  consequences: string[]         // what are the implications?
  
  affectedModules: string[]      // which modules this applies to
  alternatives: string[]         // what was considered but rejected
  supersededBy?: string          // if deprecated, what replaces it
  
  author: string
  date: ISO8601
  lastReviewed?: ISO8601
}
```

#### Storage

Decisions stored as markdown files following ADR format:

```
~/.ckb/repos/<hash>/decisions/
├── ADR-001-use-scip-for-indexing.md
├── ADR-002-module-boundary-strategy.md
└── index.json                   # metadata index
```

---

## 2. MCP Tools

### 2.1 getArchitecture

**Purpose:** Return the current architectural model

**Budget:** Heavy (may require aggregation)

#### Input

```typescript
interface GetArchitectureOptions {
  depth?: number                 // module nesting depth (default: 2)
  includeMetrics?: boolean       // include hotspot/coupling metrics
  format?: "graph" | "tree" | "list"
}
```

#### Output

```typescript
interface ArchitectureResponse {
  modules: Module[]
  dependencies: Dependency[]
  clusters: Cluster[]            // inferred groupings
  metrics?: ArchitectureMetrics
  staleness: StalenessInfo
  limitations: string[]
}

interface Dependency {
  from: string                   // module ID
  to: string                     // module ID
  strength: number               // 0.0-1.0
  type: "import" | "call" | "data" | "inferred"
}

interface Cluster {
  id: string
  name: string
  modules: string[]
  cohesion: number               // internal coupling strength
  description?: string
}
```

#### Hard Caps & Downsampling

| Constraint | Soft Limit | Hard Limit | Downsampling Strategy |
|------------|------------|------------|----------------------|
| Modules | 50 | 100 | Cluster small modules; keep top-N by symbol count |
| Edges | 200 | 500 | Keep top-N by strength; drop edges < 0.1 strength |
| Depth | 4 | 4 | Flatten deeper levels into parent |

**Downsampling algorithm:**

```typescript
function downsampleArchitecture(arch: Architecture, limits: Limits): Architecture {
  // 1. Cluster small modules (< 5 files) into parent or "misc" cluster
  const clustered = clusterSmallModules(arch.modules, minFiles: 5)
  
  // 2. If still over limit, keep top-N by symbol count
  const topModules = clustered
    .sort((a, b) => b.symbolCount - a.symbolCount)
    .slice(0, limits.maxModules)
  
  // 3. Filter edges to only include kept modules
  const relevantEdges = arch.dependencies
    .filter(e => topModules.has(e.from) && topModules.has(e.to))
  
  // 4. If edges over limit, keep strongest
  const topEdges = relevantEdges
    .filter(e => e.strength >= 0.1)
    .sort((a, b) => b.strength - a.strength)
    .slice(0, limits.maxEdges)
  
  return {
    modules: topModules,
    dependencies: topEdges,
    limitations: [
      ...(clustered.length > topModules.length 
        ? [{ type: "truncated_results", requested: clustered.length, returned: topModules.length, reason: "module_limit" }] 
        : []),
      ...(relevantEdges.length > topEdges.length
        ? [{ type: "truncated_results", requested: relevantEdges.length, returned: topEdges.length, reason: "edge_limit" }]
        : [])
    ]
  }
}
```

---

### 2.2 getModuleResponsibilities

**Purpose:** Explain what each module is responsible for

**Budget:** Cheap (reads from cache)

#### Input

```typescript
interface GetModuleResponsibilitiesOptions {
  moduleId?: string              // specific module, or all if omitted
  includeFiles?: boolean         // include file-level responsibilities
  includeSymbols?: boolean       // include key symbol responsibilities
}
```

#### Output

```typescript
interface ResponsibilitiesResponse {
  modules: ModuleResponsibility[]
  staleness: StalenessInfo
  limitations: string[]
}

interface ModuleResponsibility {
  moduleId: string
  name: string
  summary: string
  capabilities: string[]
  keySymbols: SymbolResponsibility[]  // if includeSymbols
  files: FileResponsibility[]         // if includeFiles
  confidence: number
  confidenceBasis: ConfidenceBasis[]
}
```

---

### 2.3 getHotspots

**Purpose:** Identify risky/volatile areas

**Budget:** Heavy (requires metrics computation)

*Note: This extends v5.2's `getHotspots` with persistence and trending.*

#### Input

```typescript
interface GetHotspotsOptions {
  timeWindow?: TimeWindow        // default: 30 days
  scope?: string                 // module filter
  minScore?: number              // filter threshold (default: 0.3)
  includeHistory?: boolean       // include trend data
  limit?: number                 // default: 20
}
```

#### Output

```typescript
interface HotspotsResponse {
  hotspots: Hotspot[]
  summary: {
    totalHotspots: number
    averageScore: number
    trend: "improving" | "stable" | "degrading"
    topRiskModule: string
  }
  staleness: StalenessInfo
  limitations: string[]
}
```

#### New in v6.0: Historical Trends

```typescript
interface HotspotHistory {
  targetId: string
  snapshots: Array<{
    date: ISO8601
    score: number
    metrics: HotspotMetrics
  }>
  trend: {
    direction: "increasing" | "stable" | "decreasing"
    velocity: number             // rate of change
    projection30d: number        // predicted score in 30 days
  }
}
```

---

### 2.4 getOwnership

**Purpose:** Determine who owns a path/module/symbol

**Budget:** Cheap

#### Input

```typescript
interface GetOwnershipOptions {
  path?: string                  // file or directory path
  moduleId?: string              // module identifier
  symbolId?: string              // symbol identifier
  includeHistory?: boolean       // show ownership changes over time
}
```

#### Output

```typescript
interface OwnershipResponse {
  target: string
  targetType: "path" | "module" | "symbol"
  
  owners: Array<{
    owner: Owner
    scope: "maintainer" | "reviewer" | "contributor"
    confidence: number
    source: string
  }>
  
  history?: OwnershipHistory[]   // if includeHistory
  suggestedReviewers: Owner[]    // for PR assignment
  
  staleness: StalenessInfo
  limitations: string[]
}

interface OwnershipHistory {
  date: ISO8601
  owner: Owner
  event: "added" | "removed" | "promoted" | "demoted"
  reason: string                 // e.g., "git blame shift", "CODEOWNERS update"
}
```

---

### 2.5 recordDecision *(write operation)*

**Purpose:** Capture an architectural decision

**Budget:** Cheap

#### Input

```typescript
interface RecordDecisionOptions {
  title: string
  context: string
  decision: string
  consequences?: string[]
  affectedModules?: string[]
  alternatives?: string[]
  status?: "proposed" | "accepted"  // default: proposed
}
```

#### Output

```typescript
interface RecordDecisionResponse {
  id: string                     // assigned ADR ID
  path: string                   // file path
  status: "created" | "updated"
}
```

---

### 2.6 getDecisions

**Purpose:** Query architectural decisions

**Budget:** Cheap

#### Input

```typescript
interface GetDecisionsOptions {
  moduleId?: string              // filter by affected module
  status?: string[]              // filter by status
  search?: string                // text search
  limit?: number                 // default: 20
}
```

#### Output

```typescript
interface DecisionsResponse {
  decisions: ArchitecturalDecision[]
  totalCount: number
}
```

---

### 2.7 refreshArchitecture

**Purpose:** Rebuild architectural model from sources

**Budget:** Heavy (synchronous in v6.0)

> **v6.0 scope:** Refresh is synchronous and blocking. Background/async refresh deferred to v6.1 pending job runner design.

#### Input

```typescript
interface RefreshArchitectureOptions {
  scope?: "all" | "modules" | "ownership" | "hotspots" | "responsibilities"
  force?: boolean                // rebuild even if fresh
  dryRun?: boolean               // report what would change without writing
}
```

#### Output

```typescript
interface RefreshResponse {
  status: "completed" | "skipped"  // skipped if fresh and !force
  changes: {
    modulesUpdated: number
    ownershipUpdated: number
    hotspotsUpdated: number
    responsibilitiesUpdated: number
  }
  duration: Duration
  limitations: Limitation[]
}
```

#### Refresh Behavior by Scope

| Scope | Sources Read | Data Written |
|-------|--------------|--------------|
| `modules` | MODULES.toml, SCIP, directory structure | modules table |
| `ownership` | CODEOWNERS, git-blame | ownership + ownership_history |
| `hotspots` | git log, SCIP complexity | hotspot_snapshots (append) |
| `responsibilities` | doc comments, README, LLM (if enabled) | responsibilities |
| `all` | All of the above | All tables |
```

---

### 2.8 annotateModule *(write operation)*

**Purpose:** Add or update module metadata

**Budget:** Cheap

#### Input

```typescript
interface AnnotateModuleOptions {
  moduleId: string
  name?: string
  responsibility?: string
  owner?: string
  tags?: string[]
  boundaries?: {
    public?: string[]
    internal?: string[]
  }
}
```

#### Output

```typescript
interface AnnotateModuleResponse {
  moduleId: string
  status: "created" | "updated"
  changes: string[]              // which fields changed
}
```

---

## 3. Integration Points

### 3.1 CODEOWNERS Integration

```
# .github/CODEOWNERS
# CKB reads this file for ownership data

/src/auth/       @security-team
/src/billing/    @payments-team
*.test.ts        @platform-team
```

**CKB behavior:**
- Parse on startup and refresh
- Treat as `confidence: 1.0` source
- Fall back to git-blame when CODEOWNERS doesn't cover a path

### 3.2 Module Declaration Files

**Option A: TOML**

```toml
# MODULES.toml

[[modules]]
id = "auth"
name = "Authentication"
paths = ["src/auth/**"]
responsibility = "Handle user authentication and session management"
owner = "@security-team"
tags = ["core", "security-sensitive"]

[[modules]]
id = "billing"
name = "Billing & Payments"
paths = ["src/billing/**", "src/invoicing/**"]
responsibility = "Process payments and generate invoices"
owner = "@payments-team"
```

**Option B: YAML**

```yaml
# modules.yaml
modules:
  - id: auth
    name: Authentication
    paths:
      - src/auth/**
    responsibility: Handle user authentication and session management
    owner: "@security-team"
    tags: [core, security-sensitive]
```

### 3.3 ADR Directory

```
docs/decisions/
├── ADR-001-use-postgresql.md
├── ADR-002-event-sourcing.md
└── template.md
```

CKB scans this directory and indexes decisions for queryability.

### 3.4 IDE Integration (Future)

```typescript
// VS Code extension could call:
const ownership = await ckb.getOwnership({ path: currentFile })
const hotspot = await ckb.getHotspots({ scope: currentModule })

// Display in sidebar:
// 👤 Owner: @security-team
// 🔥 Hotspot score: 0.73 (high churn)
```

---

## 4. Data Lifecycle

### 4.1 Initial Population

```
ckb init --with-architecture
```

1. Scan for MODULES.toml / modules.yaml
2. Parse CODEOWNERS
3. Infer modules from package structure
4. Compute initial hotspots from git history
5. Generate responsibility summaries (optional, requires LLM)

### 4.2 Incremental Updates

On each `ckb refresh`:

1. Detect changed files since last refresh
2. Update affected module boundaries
3. Recompute ownership for changed paths (git-blame)
4. Update hotspot metrics incrementally
5. Mark stale data for modules with significant changes

### 4.3 Garbage Collection

```
ckb gc --architecture
```

- Remove orphaned module entries
- Compact SQLite databases
- Archive old hotspot snapshots (keep 365 days)
- Validate decision links

---

## 5. Privacy & Security

### 5.1 Data Sensitivity

| Data Type | Sensitivity | Storage |
|-----------|-------------|---------|
| Module structure | Low | Local SQLite |
| Ownership (names/emails) | Medium | Local SQLite, optionally redacted |
| Hotspot metrics | Low | Local SQLite |
| Decisions | Medium | Local markdown |
| LLM-generated summaries | Low | Local SQLite |

### 5.2 Opt-In Features

| Feature | Default | Opt-In |
|---------|---------|--------|
| Module inference | ✅ On | - |
| Ownership from git-blame | ✅ On | - |
| Hotspot tracking | ✅ On | - |
| LLM responsibility generation | ❌ Off | Requires explicit enable |
| Telemetry collection | ❌ Off | Requires explicit enable |

### 5.3 Data Portability

```
ckb export --architecture --format=json > architecture.json
ckb import --architecture architecture.json
```

---

## 6. What v6.0 Explicitly Does Not Add

| Excluded | Reason |
|----------|--------|
| Code generation | Different product surface |
| Automated refactoring | Requires different safety model |
| Enforcement/linting | Better handled by dedicated tools |
| CI/CD integration | Out of scope for v6.0 |
| Multi-repo sync | Complex; defer to v6.1+ |
| Real-time collaboration | Different architecture required |

---

## 7. Implementation Phases

### Phase 1 — Foundation (v6.0-alpha)

1. **Persistence layer** — SQLite schema, migration system
2. **Module registry** — Declaration parsing + inference
3. **getArchitecture** — Basic module graph
4. **refreshArchitecture** — Rebuild command

### Phase 2 — Ownership (v6.0-beta)

5. **Ownership registry** — CODEOWNERS + git-blame integration
6. **getOwnership** — Query interface
7. **Ownership history** — Track changes over time

### Phase 3 — Intelligence (v6.0-rc)

8. **Hotspot tracker** — Churn + complexity + coupling
9. **getHotspots** — With historical trends
10. **Responsibility map** — Doc extraction + inference

### Phase 4 — Decisions (v6.0-stable)

11. **Decision log** — ADR parsing + storage
12. **recordDecision / getDecisions** — Write + query
13. **annotateModule** — Manual enrichment

---

## 8. Success Metrics

| Metric | Target |
|--------|--------|
| Module detection accuracy | > 90% on test repos |
| Ownership accuracy vs CODEOWNERS | 100% (it's the source) |
| Ownership accuracy from git-blame | > 80% (top contributor = owner) |
| Hotspot score correlation with bug density | > 0.6 |
| Architecture refresh time | < 30s for 100k LOC |
| Query latency (cheap tools) | P95 < 300ms |
| Query latency (heavy tools) | P95 < 2000ms |

---

## 9. Migration from v5.2

v6.0 is **additive** — all v5.2 tools continue to work unchanged.

| v5.2 Tool | v6.0 Status |
|-----------|-------------|
| `findSymbols` | Unchanged |
| `explainFile` | Enhanced with responsibility data |
| `explainSymbol` | Enhanced with ownership data |
| `traceUsage` | Unchanged |
| `listEntrypoints` | Unchanged |
| `summarizeDiff` | Enhanced with ownership + hotspot context |
| `getHotspots` | Enhanced with persistence + trends |
| `getArchitectureMap` | Superseded by `getArchitecture` (alias maintained) |

---

## One-Sentence Positioning

> **CKB v6.0 transforms code navigation into architectural memory — a persistent, queryable model of module boundaries, ownership, responsibilities, and risk areas that accumulates knowledge over time.**

---

## Appendix A: Tool Budget Classification (v6.0)

| Tool | Budget | Max Latency | Notes |
|------|--------|-------------|-------|
| getArchitecture | Heavy | 2000ms | May aggregate from multiple sources |
| getModuleResponsibilities | Cheap | 300ms | Reads from cache |
| getHotspots | Heavy | 2000ms | Requires metrics computation |
| getOwnership | Cheap | 300ms | Reads from cache |
| recordDecision | Cheap | 300ms | Append-only write |
| getDecisions | Cheap | 300ms | SQLite query + FTS5 |
| refreshArchitecture | Heavy | 30000ms | Synchronous; blocks until complete |
| annotateModule | Cheap | 300ms | Single record update |

---

## Appendix B: Schema Versions

```typescript
const SCHEMA_VERSIONS = {
  modules: 1,
  ownership: 1,
  hotspots: 1,
  decisions: 1,
  module_renames: 1,
  ownership_history: 1,
  hotspot_snapshots: 1,
  responsibilities: 1,
}
```

Migration strategy: 
- **Derived tables:** Rebuild from sources on version mismatch
- **Canonical data:** Preserve and migrate schema (never rebuild)
- **History tables:** Append-only; schema changes add columns, never remove

---

## Appendix C: v6.0-stable Acceptance Criteria

Before declaring v6.0 stable, these invariants must hold:

### Must Be True

| # | Criterion | Verification |
|---|-----------|--------------|
| 1 | Declared modules + CODEOWNERS are always correct | Unit tests + manual verification |
| 2 | Declared modules load in < 100ms | Benchmark on 50-module repo |
| 3 | Inferred modules are clearly labeled as `source: "inferred"` | Schema constraint |
| 4 | Hotspots are reliable for churn (git-based) | Compare with `git log --stat` |
| 5 | Decisions are queryable by module ID | Integration test |
| 6 | Stable IDs survive directory renames | Rename detection test |
| 7 | Refresh preserves canonical data | Before/after comparison test |
| 8 | Concurrent reads don't block | Load test with parallel queries |

### Should Be True (Best Effort)

| # | Criterion | Notes |
|---|-----------|-------|
| 9 | Complexity metrics available for Go | Requires SCIP + go/ast |
| 10 | Ownership from git-blame matches intuition | Manual review on 3 repos |
| 11 | LLM summaries are coherent (if enabled) | Human review sample |

### Explicitly Deferred to v6.1+

| Feature | Reason |
|---------|--------|
| Async/background refresh | Needs job runner design |
| Multi-repo sync | Complex; needs cross-repo ID strategy |
| Runtime telemetry (observed mode) | Needs instrumentation design |
| Complexity for non-Go languages | Tree-sitter integration not ready |

---

## Appendix D: Feedback Incorporated (Draft 2)

| Feedback | Resolution |
|----------|------------|
| Decision storage conflicts (single file vs directory) | ADR directory is canonical; DB stores index only |
| Rebuild conflicts with audit trail | Explicit canonical vs derived classification |
| Async refresh needs mechanism | Deferred to v6.1; v6.0 is synchronous |
| SQLite schema not defined | Added full schema in 0.1 |
| Stable IDs need hard rule | Added 0.5 with generation rules + rename tracking |
| Ownership algorithm not specified | Added 0.6 with full algorithm |
| Complexity sources underspecified | Added 0.9 language support matrix |
| LLM privacy contract missing | Added 0.10 with config options |
| Single vs multiple DBs | Unified to single ckb.db |
| Confidence not composable | Added formula with evidence multipliers |
| Limitations should be structured | Added 0.7 with typed limitations |
| Concurrency contract missing | Added 0.2 with locking rules |
| Hard caps too tight for monorepos | Added soft/hard limits with downsampling |

---

*Document version: v6.0-draft-2*  
*Last updated: December 2024*

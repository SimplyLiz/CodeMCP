# CKB v6.0 Implementation Plan

*Architectural Memory: From query engine to living knowledge base*

---

## Overview

This plan implements CKB v6.0 based on the specification document. v6.0 transforms CKB from a stateless query engine into a **living knowledge base** that accumulates and maintains architectural understanding over time.

### Current State (v5.2)

**Existing infrastructure:**
- SQLite storage with migrations, WAL mode, schema versioning
- Module detection for 7+ languages (Go, TS/JS, Dart, Rust, Python, Java, Kotlin)
- 18 MCP tools with consistent response patterns
- Git backend with churn metrics and hotspot scoring
- Three-tier caching (query, view, negative)
- Call graph with caller/callee traversal

**What v6.0 adds:**
- Persistent architectural state that survives sessions
- Module boundaries and explicit declarations
- Ownership tracking (CODEOWNERS + git-blame)
- Responsibility mapping (doc extraction + inference)
- Hotspot trends with historical data
- Architectural decision records (ADRs)

---

## Phase 1: Foundation

*Persistence layer and module registry*

### 1.1 Schema Extension (v2)

**Goal:** Extend SQLite schema for v6.0 entities

**Files to modify:**
- `internal/storage/sqlite.go` - Add new tables
- `internal/storage/migrations.go` - v1 -> v2 migration

**Steps:**

- [ ] **1.1.1** Add `modules` table enhancements
  ```sql
  ALTER TABLE modules ADD COLUMN boundaries TEXT;      -- JSON: {public: [], internal: []}
  ALTER TABLE modules ADD COLUMN responsibility TEXT;
  ALTER TABLE modules ADD COLUMN owner_ref TEXT;
  ALTER TABLE modules ADD COLUMN tags TEXT;            -- JSON array
  ALTER TABLE modules ADD COLUMN source TEXT NOT NULL DEFAULT 'inferred';
  ALTER TABLE modules ADD COLUMN confidence REAL NOT NULL DEFAULT 0.5;
  ALTER TABLE modules ADD COLUMN confidence_basis TEXT;
  ```

- [ ] **1.1.2** Add `ownership` table
  ```sql
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
  ```

- [ ] **1.1.3** Add `ownership_history` table (append-only)
  ```sql
  CREATE TABLE ownership_history (
      id INTEGER PRIMARY KEY,
      pattern TEXT NOT NULL,
      owner_id TEXT NOT NULL,
      event TEXT NOT NULL,           -- "added" | "removed" | "promoted" | "demoted"
      reason TEXT,
      recorded_at TEXT NOT NULL
  );
  CREATE INDEX idx_ownership_history_pattern ON ownership_history(pattern);
  ```

- [ ] **1.1.4** Add `hotspot_snapshots` table (time-series, append-only)
  ```sql
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
  ```

- [ ] **1.1.5** Add `responsibilities` table
  ```sql
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
  ```

- [ ] **1.1.6** Add `decisions` table
  ```sql
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
  ```

- [ ] **1.1.7** Add `module_renames` tracking table
  ```sql
  CREATE TABLE module_renames (
      old_id TEXT NOT NULL,
      new_id TEXT NOT NULL,
      renamed_at TEXT NOT NULL,
      reason TEXT                    -- "directory_rename" | "manual" | "merge"
  );
  CREATE INDEX idx_module_renames_old ON module_renames(old_id);
  ```

- [ ] **1.1.8** Add FTS5 for text search
  ```sql
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

- [ ] **1.1.9** Implement schema version tracking
  ```sql
  CREATE TABLE schema_versions (
      table_name TEXT PRIMARY KEY,
      version INTEGER NOT NULL,
      migrated_at TEXT NOT NULL
  );
  ```

- [ ] **1.1.10** Write migration function v1 -> v2
  - Preserve existing data
  - Add new columns with defaults
  - Create new tables
  - Backfill `source: "inferred"` for existing modules

### 1.2 Module Declaration Parsing

**Goal:** Support explicit module declarations via MODULES.toml

**Files to create/modify:**
- `internal/modules/declaration.go` (new) - TOML parser
- `internal/modules/types.go` - Extend module types

**Steps:**

- [ ] **1.2.1** Define `DeclaredModule` type
  ```go
  type DeclaredModule struct {
      ID             string   `toml:"id"`
      Name           string   `toml:"name"`
      Paths          []string `toml:"paths"`           // glob patterns
      Boundaries     Boundaries `toml:"boundaries"`
      Responsibility string   `toml:"responsibility"`
      Owner          string   `toml:"owner"`
      Tags           []string `toml:"tags"`
  }

  type Boundaries struct {
      Public   []string `toml:"public"`   // exported paths/symbols
      Internal []string `toml:"internal"` // internal-only
  }
  ```

- [ ] **1.2.2** Implement MODULES.toml parser
  - Look for `MODULES.toml` or `modules.yaml` in repo root
  - Parse and validate declarations
  - Return `[]DeclaredModule`

- [ ] **1.2.3** Implement module source priority
  | Source | Priority | Confidence |
  |--------|----------|------------|
  | MODULES.toml | 1 | 1.0 |
  | go.mod packages | 2 | 0.89 |
  | Import clusters | 3 | 0.69 |
  | Directory structure | 4 | 0.59 |

- [ ] **1.2.4** Merge declared and inferred modules
  - Declared modules override inferred
  - Inferred modules fill gaps
  - Track source in `modules.source` field

### 1.3 Stable Module IDs

**Goal:** Generate stable IDs that survive renames

**Files to create/modify:**
- `internal/identity/module_id.go` (new)
- `internal/storage/sqlite.go` - Add rename tracking

**Steps:**

- [ ] **1.3.1** Implement ID generation rules
  | Entity | ID Generation |
  |--------|--------------|
  | Declared modules | `id` field from MODULES.toml |
  | Inferred modules | `mod_` + `sha256(normalized_root_path)[:12]` |

- [ ] **1.3.2** Implement rename detection
  - Hook into git rename detection
  - When directory renamed, create mapping in `module_renames`
  - Update `modules.id` to new value
  - Preserve history links via mapping table

- [ ] **1.3.3** Implement ID resolution with alias chain
  - When querying by old ID, follow rename chain
  - Max depth: 3 (same as symbol aliases)

### 1.4 Persistence Layer

**Goal:** Directory structure for persistent state

**Files to create/modify:**
- `internal/storage/paths.go` (new) - Path management
- `cmd/ckb/commands/init.go` - Create directories

**Steps:**

- [ ] **1.4.1** Define storage paths
  ```
  ~/.ckb/
  ├── config.toml              # global config
  └── repos/
      └── <repo-hash>/
          ├── ckb.db            # unified SQLite database
          ├── decisions/        # ADR markdown files
          │   ├── ADR-001-*.md
          │   └── ...
          └── index.scip        # existing SCIP index
  ```

- [ ] **1.4.2** Implement repo hash generation
  - `sha256(git_remote_url || repo_root_path)[:16]`
  - Stable across clones of same repo

- [ ] **1.4.3** Update `ckb init` to create v6.0 directories
  - Create `~/.ckb/repos/<hash>/` if not exists
  - Create `decisions/` subdirectory
  - Initialize empty ckb.db with v2 schema

- [ ] **1.4.4** Implement file-based locking
  - Lock file: `~/.ckb/repos/<hash>/ckb.lock`
  - Include PID + timestamp for stale lock detection
  - Auto-release after 5 minutes

### 1.5 Enhanced getArchitecture

**Goal:** Return persistent module graph with boundaries

**Files to modify:**
- `internal/mcp/tool_impls.go` - Enhance existing tool
- `internal/query/architecture.go` - Add boundary support

**Steps:**

- [ ] **1.5.1** Extend `GetArchitectureOptions`
  ```go
  type GetArchitectureOptions struct {
      Depth          int    `json:"depth"`           // module nesting depth (default: 2)
      IncludeMetrics bool   `json:"includeMetrics"`  // include hotspot/coupling metrics
      Format         string `json:"format"`          // "graph" | "tree" | "list"
  }
  ```

- [ ] **1.5.2** Extend response with v6.0 fields
  ```go
  type ArchitectureResponse struct {
      Modules      []Module      `json:"modules"`
      Dependencies []Dependency  `json:"dependencies"`
      Clusters     []Cluster     `json:"clusters"`     // inferred groupings
      Metrics      *ArchMetrics  `json:"metrics,omitempty"`
      Staleness    StalenessInfo `json:"staleness"`
      Limitations  []Limitation  `json:"limitations"`
  }
  ```

- [ ] **1.5.3** Implement downsampling for large repos
  | Constraint | Soft Limit | Hard Limit | Strategy |
  |------------|------------|------------|----------|
  | Modules | 50 | 100 | Cluster small modules |
  | Edges | 200 | 500 | Keep top-N by strength |
  | Depth | 4 | 4 | Flatten deeper levels |

- [ ] **1.5.4** Add staleness info to response
  ```go
  type StalenessInfo struct {
      DataAge           time.Duration `json:"dataAge"`
      CodeChanges       int           `json:"codeChanges"`       // commits since update
      Staleness         string        `json:"staleness"`         // "fresh" | "aging" | "stale" | "obsolete"
      RefreshRecommended bool         `json:"refreshRecommended"`
  }
  ```

### 1.6 refreshArchitecture Tool

**Goal:** Rebuild architectural model from sources

**Files to create/modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler
- `internal/query/refresh.go` (new)

**Steps:**

- [ ] **1.6.1** Define tool interface
  ```go
  type RefreshArchitectureOptions struct {
      Scope   string `json:"scope"`   // "all" | "modules" | "ownership" | "hotspots" | "responsibilities"
      Force   bool   `json:"force"`   // rebuild even if fresh
      DryRun  bool   `json:"dryRun"`  // report changes without writing
  }

  type RefreshResponse struct {
      Status   string        `json:"status"`   // "completed" | "skipped"
      Changes  RefreshChanges `json:"changes"`
      Duration time.Duration `json:"duration"`
      Limitations []Limitation `json:"limitations"`
  }
  ```

- [ ] **1.6.2** Implement refresh logic by scope
  | Scope | Sources Read | Data Written |
  |-------|--------------|--------------|
  | `modules` | MODULES.toml, SCIP, directory structure | modules table |
  | `ownership` | CODEOWNERS, git-blame | ownership + history |
  | `hotspots` | git log, SCIP complexity | hotspot_snapshots (append) |
  | `responsibilities` | doc comments, README | responsibilities |
  | `all` | All of above | All tables |

- [ ] **1.6.3** Implement staleness check
  - Skip refresh if data is fresh and `force: false`
  - Fresh: < 7 days, < 50 commits since last update

- [ ] **1.6.4** Add MCP tool definition
  - Budget: Heavy
  - Max latency: 30000ms

---

## Phase 2: Ownership

*CODEOWNERS + git-blame integration*

### 2.1 CODEOWNERS Parser

**Goal:** Parse and cache CODEOWNERS rules

**Files to create:**
- `internal/ownership/codeowners.go` (new)
- `internal/ownership/types.go` (new)

**Steps:**

- [ ] **2.1.1** Define ownership types
  ```go
  type Owner struct {
      Type   string  `json:"type"`   // "user" | "team" | "email"
      ID     string  `json:"id"`     // @username, @org/team, email
      Weight float64 `json:"weight"` // 0.0-1.0 contribution weight
  }

  type OwnershipRule struct {
      Pattern    string  `json:"pattern"`
      Owners     []Owner `json:"owners"`
      Source     string  `json:"source"`     // "codeowners" | "git-blame"
      Confidence float64 `json:"confidence"`
  }
  ```

- [ ] **2.1.2** Implement CODEOWNERS file discovery
  - Check: `.github/CODEOWNERS`, `CODEOWNERS`, `docs/CODEOWNERS`
  - Parse GitHub CODEOWNERS format
  - Handle glob patterns

- [ ] **2.1.3** Implement pattern matching
  - Match file paths against CODEOWNERS patterns
  - Return owners in priority order

- [ ] **2.1.4** Cache rules in `ownership` table
  - Parse on refresh
  - Store with `source: "codeowners"`, `confidence: 1.0`

### 2.2 Git Blame Integration

**Goal:** Extract ownership from git blame

**Files to create/modify:**
- `internal/backends/git/blame.go` (new)
- `internal/backends/git/adapter.go` - Add methods

**Steps:**

- [ ] **2.2.1** Implement git blame parsing
  ```go
  type LineOwnership struct {
      LineNumber int
      Author     string
      Email      string
      Timestamp  time.Time
      CommitHash string
  }

  func (g *GitAdapter) GetFileBlame(filePath string) ([]LineOwnership, error)
  ```

- [ ] **2.2.2** Implement ownership computation algorithm
  ```go
  type BlameConfig struct {
      TimeDecayHalfLife   int      // days (default: 90)
      ExcludeBots         bool     // filter bot commits
      ExcludeMergeCommits bool
      BotPatterns         []string // regex patterns
      Thresholds          struct {
          Maintainer  float64 // >= 0.50 weighted contribution
          Reviewer    float64 // >= 0.20
          Contributor float64 // >= 0.05
      }
  }

  func ComputeOwnership(blame []LineOwnership, config BlameConfig) []Owner
  ```

- [ ] **2.2.3** Implement time-decay weighting
  - Recent commits matter more
  - `decay = 0.5 ^ (age_days / half_life)`

- [ ] **2.2.4** Implement bot filtering
  - Default patterns: `[bot]$`, `^dependabot`, `^renovate`
  - Configurable via config

- [ ] **2.2.5** Implement scope assignment
  - >= 50% weighted contribution -> maintainer
  - >= 20% -> reviewer
  - >= 5% -> contributor

### 2.3 Ownership Resolution

**Goal:** Merge CODEOWNERS and blame into unified ownership

**Files to create:**
- `internal/ownership/resolver.go` (new)

**Steps:**

- [ ] **2.3.1** Implement ownership resolver
  ```go
  type OwnershipResolver interface {
      GetOwnership(path string) (*OwnershipResult, error)
      GetModuleOwnership(moduleId string) (*OwnershipResult, error)
      GetSymbolOwnership(symbolId string) (*OwnershipResult, error)
  }
  ```

- [ ] **2.3.2** Implement source priority
  | Scenario | Behavior |
  |----------|----------|
  | CODEOWNERS exists | Team from CODEOWNERS; individuals from blame within team |
  | CODEOWNERS missing | Pure blame-based ownership |
  | Blame insufficient (<100 lines) | Fall back to directory-level ownership |
  | Conflict | CODEOWNERS wins for team; blame wins for individuals |

- [ ] **2.3.3** Implement ownership aggregation for modules
  - Aggregate file ownership within module
  - Weight by file size/importance
  - Return top owners

### 2.4 getOwnership Tool

**Goal:** Query ownership for path/module/symbol

**Files to modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler

**Steps:**

- [ ] **2.4.1** Define tool interface
  ```go
  type GetOwnershipOptions struct {
      Path           string `json:"path"`           // file or directory
      ModuleId       string `json:"moduleId"`       // module identifier
      SymbolId       string `json:"symbolId"`       // symbol identifier
      IncludeHistory bool   `json:"includeHistory"` // show changes over time
  }

  type OwnershipResponse struct {
      Target             string          `json:"target"`
      TargetType         string          `json:"targetType"` // "path" | "module" | "symbol"
      Owners             []OwnerEntry    `json:"owners"`
      History            []OwnerHistory  `json:"history,omitempty"`
      SuggestedReviewers []Owner         `json:"suggestedReviewers"`
      Staleness          StalenessInfo   `json:"staleness"`
      Limitations        []Limitation    `json:"limitations"`
  }
  ```

- [ ] **2.4.2** Implement path ownership query
  - Match against CODEOWNERS patterns
  - Fall back to blame

- [ ] **2.4.3** Implement module ownership query
  - Aggregate from file ownership
  - Return weighted owners

- [ ] **2.4.4** Implement symbol ownership query
  - Get file containing symbol
  - Return file ownership

- [ ] **2.4.5** Implement ownership history
  - Query `ownership_history` table
  - Return chronological events

- [ ] **2.4.6** Add MCP tool definition
  - Budget: Cheap
  - Max latency: 300ms

### 2.5 Ownership History Tracking

**Goal:** Record ownership changes over time

**Files to modify:**
- `internal/ownership/history.go` (new)
- `internal/storage/sqlite.go` - Add history methods

**Steps:**

- [ ] **2.5.1** Implement history recording
  ```go
  type OwnershipEvent struct {
      Pattern    string
      OwnerId    string
      Event      string // "added" | "removed" | "promoted" | "demoted"
      Reason     string
      RecordedAt time.Time
  }

  func RecordOwnershipChange(event OwnershipEvent) error
  ```

- [ ] **2.5.2** Detect ownership changes on refresh
  - Compare new ownership with previous
  - Record additions, removals, scope changes

- [ ] **2.5.3** Track reasons for changes
  - "git_blame_shift" - majority contributor changed
  - "codeowners_update" - CODEOWNERS file changed
  - "manual_assignment" - explicit annotation

---

## Phase 3: Intelligence

*Hotspot trends and responsibility mapping*

### 3.1 Hotspot Persistence

**Goal:** Store hotspot snapshots with historical trends

**Files to modify:**
- `internal/query/hotspots.go` - Add persistence
- `internal/storage/sqlite.go` - Add snapshot methods

**Steps:**

- [ ] **3.1.1** Implement snapshot storage
  ```go
  type HotspotSnapshot struct {
      TargetId            string
      TargetType          string // "file" | "module" | "symbol"
      SnapshotDate        time.Time
      ChurnCommits30d     int
      ChurnCommits90d     int
      ChurnAuthors30d     int
      ComplexityCyclomatic float64
      ComplexityCognitive  float64
      CouplingAfferent    int
      CouplingEfferent    int
      CouplingInstability float64
      Score               float64
  }

  func SaveHotspotSnapshot(snapshot HotspotSnapshot) error
  ```

- [ ] **3.1.2** Implement trend calculation
  ```go
  type HotspotTrend struct {
      Direction    string  // "increasing" | "stable" | "decreasing"
      Velocity     float64 // rate of change
      Projection30d float64 // predicted score
  }

  func CalculateTrend(targetId string, days int) (*HotspotTrend, error)
  ```

- [ ] **3.1.3** Implement module-level aggregation
  - Aggregate file hotspots to module level
  - Weight by file importance (LOC, symbol count)

- [ ] **3.1.4** Add complexity metrics (Go only)
  - Cyclomatic complexity via go/ast
  - Cognitive complexity via heuristics

### 3.2 Enhanced getHotspots

**Goal:** Add persistence, trends, and module aggregation

**Files to modify:**
- `internal/mcp/tool_impls.go` - Enhance existing tool

**Steps:**

- [ ] **3.2.1** Extend response with trends
  ```go
  type HotspotInfo struct {
      TargetId   string       `json:"targetId"`
      TargetType string       `json:"targetType"`
      Metrics    HotspotMetrics `json:"metrics"`
      Score      float64      `json:"score"`
      Trend      HotspotTrend `json:"trend"`
      Ranking    Ranking      `json:"ranking"`
  }
  ```

- [ ] **3.2.2** Add `includeHistory` option
  - Return historical snapshots
  - Enable trend visualization

- [ ] **3.2.3** Add module-level hotspots
  - Aggregate when `targetType: "module"`
  - Return top modules by hotspot score

### 3.3 Responsibility Extraction

**Goal:** Extract responsibilities from code and docs

**Files to create:**
- `internal/responsibilities/extractor.go` (new)
- `internal/responsibilities/types.go` (new)

**Steps:**

- [ ] **3.3.1** Define responsibility types
  ```go
  type Responsibility struct {
      TargetId     string   `json:"targetId"`
      TargetType   string   `json:"targetType"` // "module" | "file" | "symbol"
      Summary      string   `json:"summary"`
      Capabilities []string `json:"capabilities"`
      Source       string   `json:"source"` // "declared" | "inferred" | "llm-generated"
      Confidence   float64  `json:"confidence"`
      UpdatedAt    time.Time
      VerifiedAt   *time.Time
  }
  ```

- [ ] **3.3.2** Implement doc comment extraction
  - Go: `// Package X does Y` comments
  - Extract from AST or SCIP documentation field

- [ ] **3.3.3** Implement README parsing
  - Find README.md in module directory
  - Extract first paragraph as summary

- [ ] **3.3.4** Implement symbol analysis fallback
  - Infer from exported symbols
  - Generate "Provides X, Y, Z" from export list

- [ ] **3.3.5** Implement confidence assignment
  | Source | Confidence |
  |--------|------------|
  | Doc comment present | 0.89 |
  | README present | 0.89 |
  | Symbol analysis | 0.59 |
  | Heuristic only | 0.39 |

### 3.4 getModuleResponsibilities Tool

**Goal:** Query responsibilities for modules

**Files to modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler

**Steps:**

- [ ] **3.4.1** Define tool interface
  ```go
  type GetModuleResponsibilitiesOptions struct {
      ModuleId       string `json:"moduleId"`       // specific module, or all
      IncludeFiles   bool   `json:"includeFiles"`   // file-level responsibilities
      IncludeSymbols bool   `json:"includeSymbols"` // key symbol responsibilities
  }

  type ResponsibilitiesResponse struct {
      Modules     []ModuleResponsibility `json:"modules"`
      Staleness   StalenessInfo          `json:"staleness"`
      Limitations []Limitation           `json:"limitations"`
  }
  ```

- [ ] **3.4.2** Implement query logic
  - Return from cache if fresh
  - Regenerate if stale

- [ ] **3.4.3** Add MCP tool definition
  - Budget: Cheap
  - Max latency: 300ms

---

## Phase 4: Decisions

*Architectural decision records*

### 4.1 ADR Parser

**Goal:** Parse ADR markdown files

**Files to create:**
- `internal/decisions/parser.go` (new)
- `internal/decisions/types.go` (new)

**Steps:**

- [ ] **4.1.1** Define ADR types
  ```go
  type ArchitecturalDecision struct {
      ID              string   `json:"id"`      // "ADR-001"
      Title           string   `json:"title"`
      Status          string   `json:"status"`  // "proposed" | "accepted" | "deprecated" | "superseded"
      Context         string   `json:"context"`
      Decision        string   `json:"decision"`
      Consequences    []string `json:"consequences"`
      AffectedModules []string `json:"affectedModules"`
      Alternatives    []string `json:"alternatives"`
      SupersededBy    string   `json:"supersededBy,omitempty"`
      Author          string   `json:"author"`
      Date            time.Time
      LastReviewed    *time.Time
  }
  ```

- [ ] **4.1.2** Implement ADR markdown parser
  - Support standard ADR format (Michael Nygard style)
  - Extract YAML frontmatter if present
  - Parse markdown sections

- [ ] **4.1.3** Implement ADR directory discovery
  - Check: `docs/decisions/`, `docs/adr/`, `adr/`, `decisions/`
  - Also check `~/.ckb/repos/<hash>/decisions/`

- [ ] **4.1.4** Index ADRs in database
  - Store metadata in `decisions` table
  - Keep content in markdown files (canonical)
  - Build FTS5 index for search

### 4.2 recordDecision Tool

**Goal:** Create new ADR via MCP

**Files to modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler

**Steps:**

- [ ] **4.2.1** Define tool interface
  ```go
  type RecordDecisionOptions struct {
      Title           string   `json:"title"`
      Context         string   `json:"context"`
      Decision        string   `json:"decision"`
      Consequences    []string `json:"consequences"`
      AffectedModules []string `json:"affectedModules"`
      Alternatives    []string `json:"alternatives"`
      Status          string   `json:"status"` // default: "proposed"
  }

  type RecordDecisionResponse struct {
      ID     string `json:"id"`
      Path   string `json:"path"`
      Status string `json:"status"` // "created" | "updated"
  }
  ```

- [ ] **4.2.2** Implement ADR ID generation
  - Find max existing ADR number
  - Increment: `ADR-NNN`

- [ ] **4.2.3** Generate ADR markdown file
  - Use standard template
  - Write to `~/.ckb/repos/<hash>/decisions/`

- [ ] **4.2.4** Update index in database

- [ ] **4.2.5** Add MCP tool definition
  - Budget: Cheap
  - Max latency: 300ms

### 4.3 getDecisions Tool

**Goal:** Query architectural decisions

**Files to modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler

**Steps:**

- [ ] **4.3.1** Define tool interface
  ```go
  type GetDecisionsOptions struct {
      ModuleId string   `json:"moduleId"` // filter by affected module
      Status   []string `json:"status"`   // filter by status
      Search   string   `json:"search"`   // text search
      Limit    int      `json:"limit"`    // default: 20
  }

  type DecisionsResponse struct {
      Decisions  []ArchitecturalDecision `json:"decisions"`
      TotalCount int                     `json:"totalCount"`
  }
  ```

- [ ] **4.3.2** Implement query with filters
  - Filter by module (JSON array contains)
  - Filter by status
  - Full-text search via FTS5

- [ ] **4.3.3** Add MCP tool definition
  - Budget: Cheap
  - Max latency: 300ms

### 4.4 annotateModule Tool

**Goal:** Add or update module metadata

**Files to modify:**
- `internal/mcp/tools.go` - Add tool definition
- `internal/mcp/tool_impls.go` - Add handler

**Steps:**

- [ ] **4.4.1** Define tool interface
  ```go
  type AnnotateModuleOptions struct {
      ModuleId       string   `json:"moduleId"`
      Name           string   `json:"name"`
      Responsibility string   `json:"responsibility"`
      Owner          string   `json:"owner"`
      Tags           []string `json:"tags"`
      Boundaries     *Boundaries `json:"boundaries"`
  }

  type AnnotateModuleResponse struct {
      ModuleId string   `json:"moduleId"`
      Status   string   `json:"status"` // "created" | "updated"
      Changes  []string `json:"changes"`
  }
  ```

- [ ] **4.4.2** Implement annotation logic
  - Update module record in database
  - Set `source: "declared"` for annotated fields
  - Set `confidence: 1.0`

- [ ] **4.4.3** Track changes
  - Return list of fields that changed

- [ ] **4.4.4** Add MCP tool definition
  - Budget: Cheap
  - Max latency: 300ms

---

## Phase 5: Polish & Testing

### 5.1 Integration Tests

- [x] **5.1.1** Test schema migration v1 -> v2 - Tested in storage package
- [x] **5.1.2** Test MODULES.toml parsing - `internal/modules/declaration_test.go`
- [x] **5.1.3** Test CODEOWNERS parsing - `internal/ownership/codeowners_test.go`
- [x] **5.1.4** Test git blame integration - `internal/ownership/blame_test.go`
- [x] **5.1.5** Test ownership resolution - `internal/ownership/*_test.go`
- [x] **5.1.6** Test hotspot persistence and trends - `internal/hotspots/persistence_test.go`
- [x] **5.1.7** Test ADR parsing and indexing - `internal/decisions/parser_test.go`, `writer_test.go`
- [x] **5.1.8** Test responsibility extraction - `internal/responsibilities/extractor_test.go`

### 5.2 Latency Verification

All in-memory processing benchmarks pass with >96% headroom. See `docs/benchmarks.md` for full results.

| Tool | Budget | Target | Test |
|------|--------|--------|------|
| getArchitecture | Heavy | 2000ms | [x] Verified |
| getModuleResponsibilities | Cheap | 300ms | [x] Verified |
| getHotspots | Heavy | 2000ms | [x] Verified - 5.7µs processing |
| getOwnership | Cheap | 300ms | [x] Verified - 9.2ms for 100 files |
| recordDecision | Cheap | 300ms | [x] Verified |
| getDecisions | Cheap | 300ms | [x] Verified |
| refreshArchitecture | Heavy | 30000ms | [x] Verified |
| annotateModule | Cheap | 300ms | [x] Verified |

### 5.3 Documentation

- [x] **5.3.1** Update benchmarks.md with v6.0 results
- [ ] **5.3.2** Document new MCP tools
- [ ] **5.3.3** Document MODULES.toml format
- [ ] **5.3.4** Document ADR format and workflow
- [ ] **5.3.5** Add migration guide from v5.2

---

## Gating Criteria

Before declaring v6.0 stable:

| # | Criterion | Verification |
|---|-----------|--------------|
| 1 | Declared modules + CODEOWNERS always correct | Unit tests + manual |
| 2 | Declared modules load in < 100ms | Benchmark |
| 3 | Inferred modules labeled as `source: "inferred"` | Schema constraint |
| 4 | Hotspots reliable for churn (git-based) | Compare with `git log` |
| 5 | Decisions queryable by module ID | Integration test |
| 6 | Stable IDs survive directory renames | Rename detection test |
| 7 | Refresh preserves canonical data | Before/after test |
| 8 | Concurrent reads don't block | Load test |

---

## Phase Dependencies

```
Phase 1 (Foundation)
    |
    +---> Phase 2 (Ownership)
    |         |
    |         v
    +---> Phase 3 (Intelligence)
              |
              v
         Phase 4 (Decisions)
              |
              v
         Phase 5 (Polish & Testing)
```

Note: Phases 2 and 3 can run in parallel after Phase 1 completes.

---

## Tool Budget Classification (v6.0)

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

## Explicitly Deferred to v6.1+

| Feature | Reason |
|---------|--------|
| Async/background refresh | Needs job runner design |
| Multi-repo sync | Complex; needs cross-repo ID strategy |
| Runtime telemetry (observed mode) | Needs instrumentation design |
| Complexity for non-Go languages | Tree-sitter integration not ready |
| LLM-generated responsibilities | Privacy contract needs user consent flow |

---

## v6.2 — Federation

*Cross-repository queries and unified visibility*

### Phase 1: Foundation

- [x] **1.1** Add federation path helpers to `internal/paths/paths.go`
  - `GetFederationDir(name)` — `~/.ckb/federation/<name>/`
  - `GetFederationConfigPath(name)` — `~/.ckb/federation/<name>/config.toml`
  - `GetFederationIndexPath(name)` — `~/.ckb/federation/<name>/index.db`
  - `EnsureFederationDir(name)` — Create if not exists
  - `ListFederations()` — List all federation names

- [x] **1.2** Add dependencies to `go.mod`
  - `github.com/google/uuid` — Repo UUID generation
  - `github.com/BurntSushi/toml` — TOML config parsing

### Phase 2: Federation Core

- [x] **2.1** Create `internal/federation/` package structure
  - `federation.go` — Federation manager
  - `config.go` — Parse config.toml
  - `index.go` — Index DB management
  - `repo_identity.go` — repoUid vs repoId
  - `sync.go` — Sync repos to index
  - `queries.go` — Federated query implementations
  - `staleness.go` — Staleness propagation
  - `schema_compat.go` — Schema version check (min v6)

- [x] **2.2** Implement federation config (TOML)
  ```toml
  name = "platform"
  created_at = "2024-12-19T00:00:00Z"

  [[repos]]
  repo_uid = "UUID"
  repo_id = "api"
  path = "/code/api-service"
  tags = ["backend"]
  ```

- [x] **2.3** Implement federation index schema (`index.db`)
  - `federation_repos` — Repo metadata
  - `federated_modules` — Module summaries
  - `federated_ownership` — Ownership summaries
  - `federated_hotspots` — Hotspot top-N per repo
  - `federated_decisions` — Decision metadata

- [x] **2.4** Implement repo identity
  - `repoUid` — Immutable UUID, generated on add
  - `repoId` — Mutable alias, user-defined
  - Rename tracking

- [x] **2.5** Implement federation sync mechanism
  - Read from each repo's `ckb.db`
  - Write summaries to federation `index.db`
  - Track staleness per repo

### Phase 3: Federated Queries

- [x] **3.1** Implement `federated.listRepos`
- [x] **3.2** Implement `federated.searchModules` (FTS across repos)
- [x] **3.3** Implement `federated.searchOwnership` (glob pattern match)
- [x] **3.4** Implement `federated.getHotspots` (merged, re-ranked)
- [x] **3.5** Implement `federated.searchDecisions` (FTS across repos)
- [x] **3.6** Implement staleness propagation (weakest link)

### Phase 4: CLI Commands

- [x] **4.1** Add `ckb federation create <name>` command
- [x] **4.2** Add `ckb federation delete <name>` command
- [x] **4.3** Add `ckb federation list` command
- [x] **4.4** Add `ckb federation status <name>` command
- [x] **4.5** Add `ckb federation add <name> --repo-id=<id> --path=<path>` command
- [x] **4.6** Add `ckb federation remove <name> <repo-id>` command
- [x] **4.7** Add `ckb federation rename <name> <old-id> <new-id>` command
- [x] **4.8** Add `ckb federation repos <name>` command
- [x] **4.9** Add `ckb federation sync <name>` command

### Phase 5: HTTP API

- [x] **5.1** Add `GET /federations` endpoint
- [x] **5.2** Add `GET /federations/:name/repos` endpoint
- [x] **5.3** Add `GET /federations/:name/modules` endpoint
- [x] **5.4** Add `GET /federations/:name/ownership` endpoint
- [x] **5.5** Add `GET /federations/:name/hotspots` endpoint
- [x] **5.6** Add `GET /federations/:name/decisions` endpoint
- [x] **5.7** Add `POST /federations/:name/sync` endpoint

### Phase 6: MCP Tools

- [x] **6.1** Add `listFederations` MCP tool
- [x] **6.2** Add `federationStatus` MCP tool
- [x] **6.3** Add `federationRepos` MCP tool
- [x] **6.4** Add `federationSearchModules` MCP tool
- [x] **6.5** Add `federationSearchOwnership` MCP tool
- [x] **6.6** Add `federationGetHotspots` MCP tool
- [x] **6.7** Add `federationSearchDecisions` MCP tool
- [x] **6.8** Add `federationSync` MCP tool

### Phase 7: Testing

- [ ] **7.1** Unit tests for federation config parsing
- [ ] **7.2** Unit tests for federation index operations
- [ ] **7.3** Integration tests for federated queries
- [ ] **7.4** CLI command tests

---

## v6.2.1 — Daemon Mode

*Always-on service for IDE/CI integration*

### Phase 1: Core Infrastructure

- [x] **1.1** Bump version to 6.2.1 in `internal/version/version.go`

- [x] **1.2** Add daemon paths to `internal/paths/paths.go`
  - `GetDaemonDir()` — `~/.ckb/daemon/`
  - `GetDaemonPIDPath()` — `daemon.pid`
  - `GetDaemonLogPath()` — `daemon.log`
  - `GetDaemonDBPath()` — `daemon.db`
  - `GetDaemonSocketPath()` — `daemon.sock`
  - `EnsureDaemonDir()` — Create if not exists
  - `GetDaemonInfo()` — Return all paths

- [x] **1.3** Add daemon config to `internal/config/config.go`
  - `DaemonConfig` struct with Port, Bind, LogLevel, LogFile
  - `DaemonAuthConfig` for Bearer token auth
  - `DaemonWatchConfig` for file watching settings
  - `DaemonScheduleConfig` for scheduler settings
  - Default values: Port 9120, Bind localhost

### Phase 2: Daemon Core Package

- [x] **2.1** Create `internal/daemon/daemon.go`
  - `Daemon` struct with lifecycle management
  - `New()`, `Start()`, `Stop()`, `Wait()` methods
  - Signal handling (SIGINT, SIGTERM)
  - `IsRunning()` and `StopRemote()` for CLI control
  - Integration with scheduler, watcher, webhooks

- [x] **2.2** Create `internal/daemon/pid.go`
  - PID file management
  - `Acquire()`, `Release()`, `IsRunning()` methods
  - Stale PID detection via signal 0

- [x] **2.3** Create `internal/daemon/server.go`
  - HTTP server setup with mux
  - Health endpoint (no auth): `GET /health`
  - API endpoints with auth: `/api/v1/*`
  - Response types: `APIResponse`, `APIError`, `APIMeta`

- [x] **2.4** Create `internal/daemon/auth.go`
  - Bearer token authentication middleware
  - Token sources: config, env var, file
  - `GenerateToken()` utility

### Phase 3: Supporting Packages

- [x] **3.1** Extend `internal/jobs/` with daemon job types
  - `JobTypeFederationSync`
  - `JobTypeWebhookDispatch`
  - `JobTypeScheduledTask`
  - Scope types for each job type

- [x] **3.2** Create `internal/scheduler/` package
  - `scheduler.go` — Scheduler runner with task handlers
  - `parser.go` — Parse cron expressions and intervals ("every 4h")
  - `types.go` — Schedule, ScheduleSummary, TaskType
  - SQLite-backed persistence in `scheduler.db`

- [x] **3.3** Create `internal/watcher/` package
  - `watcher.go` — File system watcher for git changes
  - `debouncer.go` — Debounce change events
  - Polling-based for cross-platform compatibility
  - Watch .git/HEAD and .git/index for changes

- [x] **3.4** Create `internal/webhooks/` package
  - `types.go` — Webhook, Delivery, DeadLetter types
  - `manager.go` — Webhook manager with delivery queue
  - Payload formats: JSON, Slack, PagerDuty, Discord
  - HMAC-SHA256 signing
  - Retry with exponential backoff
  - Dead letter queue

### Phase 4: CLI Commands

- [x] **4.1** Create `cmd/ckb/daemon.go`
  - `ckb daemon start [--port=9120] [--bind=localhost] [--foreground]`
  - `ckb daemon stop`
  - `ckb daemon restart`
  - `ckb daemon status`
  - `ckb daemon logs [--follow] [--lines=100]`
  - Background process spawning with setsid

### Phase 5: MCP Tools

- [x] **5.1** Add daemon MCP tools to `internal/mcp/tools.go`
  - `daemonStatus` — Daemon health and stats
  - `listSchedules` — List scheduled tasks
  - `runSchedule` — Run a scheduled task immediately
  - `listWebhooks` — List configured webhooks
  - `testWebhook` — Send test event to webhook
  - `webhookDeliveries` — Get delivery history

- [x] **5.2** Create `internal/mcp/tool_impls_daemon.go`
  - Tool handler implementations

### Phase 6: Testing

- [ ] **6.1** Unit tests for scheduler parser
- [ ] **6.2** Unit tests for webhook delivery
- [ ] **6.3** Integration tests for daemon lifecycle
- [ ] **6.4** CLI command tests

---

## v6.2.2 — Tree-sitter Complexity

*Language-agnostic complexity metrics via tree-sitter*

### Overview

Add cyclomatic and cognitive complexity metrics for all supported languages using tree-sitter parsers. Currently complexity is only computed for Go via go/ast.

### Phase 1: Tree-sitter Integration

- [x] **1.1** Add tree-sitter dependencies to `go.mod`
  - `github.com/smacker/go-tree-sitter`
  - Language grammars: TypeScript, Python, Rust, Java, Kotlin

- [x] **1.2** Create `internal/complexity/` package
  - `treesitter.go` — Tree-sitter parser wrapper
  - `analyzer.go` — Cyclomatic and cognitive complexity
  - `types.go` — ComplexityResult, FileComplexity types

- [x] **1.3** Implement language-specific complexity rules
  | Language | Decision nodes |
  |----------|---------------|
  | TypeScript/JS | if, else, for, while, switch, case, catch, &&, \|\|, ?: |
  | Python | if, elif, else, for, while, except, and, or, comprehensions |
  | Rust | if, else, match, loop, while, for, &&, \|\| |
  | Java/Kotlin | if, else, for, while, switch, case, catch, &&, \|\| |

### Phase 2: Integration

- [x] **2.1** Update `internal/hotspots/` to use tree-sitter complexity
  - Created `internal/hotspots/complexity.go` integration layer
  - Supports all languages via tree-sitter

- [ ] **2.2** Add complexity to `getHotspots` response for all languages

- [ ] **2.3** Add `getFileComplexity` MCP tool (optional)

### Phase 3: Testing

- [x] **3.1** Unit tests for each language parser
  - Go, JavaScript, Python, Rust, Java tested
  - Cognitive nesting penalty verified
- [ ] **3.2** Benchmark complexity computation
- [ ] **3.3** Validate against known complexity tools

---

## Scratched (Not Implementing)

| Feature | Reason |
|---------|--------|
| Cross-repo dependencies | v6.3 |
| Remote federation | v6.3+ |
| Team dashboard | v6.3 |

---

*Document version: 1.3*
*Based on: CKB v6.0-draft-2 + v6.2 specification + v6.2.1 daemon mode + v6.2.2 tree-sitter*
*Created: December 2024*

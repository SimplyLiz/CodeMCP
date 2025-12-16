# CKB Architecture

## Overview

CKB (Code Knowledge Backend) is designed as a layered system that abstracts multiple code intelligence backends behind a unified query interface.

```
┌─────────────────────────────────────────────────────────┐
│                    Interfaces                            │
│  ┌─────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   CLI   │  │  HTTP API   │  │     MCP Server      │  │
│  └────┬────┘  └──────┬──────┘  └──────────┬──────────┘  │
└───────┼──────────────┼────────────────────┼─────────────┘
        │              │                    │
        └──────────────┼────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────────┐
│                   Query Engine                           │
│  ┌────────────┐  ┌────────────┐  ┌────────────────────┐ │
│  │   Router   │  │  Merger    │  │    Compressor      │ │
│  └────────────┘  └────────────┘  └────────────────────┘ │
└─────────────────────────┬───────────────────────────────┘
                          │
┌─────────────────────────┼───────────────────────────────┐
│                   Backend Layer                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────┐  │
│  │  SCIP   │  │   LSP   │  │   Git   │  │  (Glean)  │  │
│  └─────────┘  └─────────┘  └─────────┘  └───────────┘  │
└─────────────────────────┬───────────────────────────────┘
                          │
┌─────────────────────────┼───────────────────────────────┐
│                   Storage Layer                          │
│  ┌────────────────┐  ┌────────────────────────────────┐ │
│  │    SQLite      │  │         Cache Tiers            │ │
│  │  (Symbols,     │  │  Query │ View │ Negative       │ │
│  │   Aliases)     │  │  Cache │ Cache│ Cache          │ │
│  └────────────────┘  └────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Interface Layer

#### CLI (`cmd/ckb/`)
- Cobra-based command structure
- Human-readable output
- Interactive commands

#### HTTP API (`internal/api/`)
- REST endpoints
- JSON responses
- OpenAPI specification
- Middleware (logging, CORS, recovery)

#### MCP Server (`internal/mcp/`)
- Model Context Protocol implementation
- Tool definitions for AI assistants
- Streaming support

### 2. Query Engine

#### Router
Routes queries to appropriate backends based on:
- Query type (definition, references, search)
- Backend availability
- Query policy configuration

#### Merger
Combines results from multiple backends:
- **prefer-first**: Use first successful response
- **union**: Merge all responses, deduplicate

#### Compressor (`internal/compression/`)
Optimizes responses for LLM consumption:
- Enforces response budgets
- Truncates with drilldown suggestions
- Deduplicates results

### 3. Backend Layer

#### SCIP Backend
- Reads pre-computed SCIP indexes
- Fastest and most accurate
- Requires index generation

#### LSP Backend
- Communicates with language servers
- Real-time analysis
- May require workspace initialization

#### Git Backend
- Fallback for basic operations
- File listing, blame, history
- Always available in git repos

### 4. Storage Layer

#### SQLite Database (`.ckb/ckb.db`)

**Tables:**
- `symbol_mappings` - Stable ID to backend ID mappings
- `symbol_aliases` - Redirect mappings for renamed symbols
- `modules` - Detected modules cache
- `dependency_edges` - Module dependency graph

#### Cache Tiers

| Tier | TTL | Key Contains | Use Case |
|------|-----|--------------|----------|
| Query Cache | 5 min | headCommit | Frequent queries |
| View Cache | 1 hour | repoStateId | Expensive computations |
| Negative Cache | 5-60s | repoStateId | Avoid repeated failures |

## Key Subsystems

### Identity System (`internal/identity/`)

Provides stable symbol identification across refactors.

```
┌─────────────────────────────────────────┐
│           Symbol Identity               │
│                                         │
│  Stable ID: ckb:repo:sym:<fingerprint>  │
│                                         │
│  Fingerprint = hash(                    │
│    container + name + kind + signature  │
│  )                                      │
└─────────────────────────────────────────┘
```

**Alias Resolution:**
```
Old ID ──alias──> New ID ──alias──> Current ID
         │                 │
         └── max depth: 3 ─┘
```

### Impact Analysis (`internal/impact/`)

Analyzes the blast radius of code changes.

```
┌─────────────────────────────────────────┐
│           Impact Analysis               │
│                                         │
│  1. Derive Visibility                   │
│     - SCIP modifiers (0.95 confidence)  │
│     - Reference patterns (0.7-0.9)      │
│     - Naming conventions (0.5-0.7)      │
│                                         │
│  2. Classify References                 │
│     - direct-caller                     │
│     - transitive-caller                 │
│     - type-dependency                   │
│     - test-dependency                   │
│                                         │
│  3. Compute Risk Score                  │
│     - Visibility (30%)                  │
│     - Direct callers (35%)              │
│     - Module spread (25%)               │
│     - Impact kind (10%)                 │
└─────────────────────────────────────────┘
```

### Deterministic Output (`internal/output/`)

Ensures identical queries produce identical bytes.

**Guarantees:**
- Stable key ordering (alphabetical)
- Float precision (6 decimals)
- Consistent sorting (multi-field, stable)
- Nil/empty field omission

### Repository State (`internal/repostate/`)

Tracks repository state for cache invalidation.

```
RepoStateID = hash(
  headCommit +
  stagedDiffHash +
  workingTreeDiffHash +
  untrackedListHash
)
```

## Data Flow

### Query Flow

```
1. Request arrives (CLI/HTTP/MCP)
           │
           ▼
2. Parse parameters, validate
           │
           ▼
3. Check cache (query/view/negative)
           │
      ┌────┴────┐
      │ cached? │
      └────┬────┘
           │
     yes ──┴── no
      │        │
      ▼        ▼
4. Return   5. Route to backends
   cached      │
              ┌┴┐
              │ │ (parallel or sequential)
              └┬┘
               │
               ▼
6. Merge results
               │
               ▼
7. Compress (apply budget)
               │
               ▼
8. Generate drilldowns
               │
               ▼
9. Cache result
               │
               ▼
10. Return response
```

### Symbol Resolution Flow

```
1. Receive symbol ID
         │
         ▼
2. Check if alias exists
         │
    ┌────┴────┐
    │ alias?  │
    └────┬────┘
         │
   yes ──┴── no
    │        │
    ▼        │
3. Follow   │
   chain    │
   (max 3)  │
    │        │
    └────┬───┘
         │
         ▼
4. Return resolved symbol
   (with redirect info if aliased)
```

## Configuration

### Query Policy

```json
{
  "queryPolicy": {
    "backendLadder": ["scip", "lsp", "git"],
    "mergeStrategy": "prefer-first"
  }
}
```

### Response Budget

```json
{
  "budget": {
    "maxModules": 10,
    "maxSymbolsPerModule": 5,
    "maxImpactItems": 20,
    "maxDrilldowns": 5,
    "estimatedMaxTokens": 4000
  }
}
```

### Backend Limits

```json
{
  "backendLimits": {
    "maxRefsPerQuery": 10000,
    "maxSymbolsPerSearch": 1000,
    "maxFilesScanned": 5000,
    "maxUnionModeTimeMs": 60000
  }
}
```

## Error Handling

### Error Taxonomy (`internal/errors/`)

All errors include:
- Error code (machine-readable)
- Message (human-readable)
- Details (context-specific)
- Suggested fixes
- Drilldown queries

### Negative Caching

Failed queries are cached to avoid repeated failures:

| Error Type | TTL | Triggers Warmup |
|------------|-----|-----------------|
| symbol-not-found | 60s | No |
| backend-unavailable | 15s | No |
| workspace-not-ready | 10s | Yes |
| timeout | 5s | No |

## Extension Points

### Adding a New Backend

1. Implement backend interface in `internal/backends/`
2. Register in backend factory
3. Add to configuration schema
4. Update backend ladder options

### Adding a New Tool

1. Add handler in `internal/api/handlers.go`
2. Register route in `internal/api/routes.go`
3. Add MCP tool definition in `internal/mcp/`
4. Update OpenAPI spec

### Adding a New Cache Tier

1. Add table in `internal/storage/schema.go`
2. Implement cache methods in `internal/storage/cache.go`
3. Define invalidation triggers

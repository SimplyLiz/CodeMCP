# Storage Layer

The storage layer provides persistent storage for CKB using SQLite (via modernc.org/sqlite - pure Go, no CGO).

## Overview

The storage layer is organized into several components:

- **Database Management** (`db.go`) - Connection handling, transactions, schema migrations
- **Schema** (`schema.go`) - Table definitions following the design document
- **Cache** (`cache.go`) - Three-tier caching system (query, view, negative)
- **Negative Cache** (`negative_cache.go`) - Error caching with type-specific policies
- **Repositories** (`repositories.go`) - CRUD operations for all entities

## Database Location

The database is stored at `.ckb/ckb.db` in the repository root.

## Tables

### Symbol Mappings (`symbol_mappings`)

Stores symbol mappings with tombstones (Section 4.3 of design doc):

```sql
CREATE TABLE symbol_mappings (
  stable_id TEXT PRIMARY KEY,
  state TEXT NOT NULL CHECK(state IN ('active', 'deleted', 'unknown')),
  backend_stable_id TEXT,
  fingerprint_json TEXT NOT NULL,
  location_json TEXT NOT NULL,
  definition_version_id TEXT,
  definition_version_semantics TEXT,
  last_verified_at TEXT NOT NULL,
  last_verified_state_id TEXT NOT NULL,
  deleted_at TEXT,
  deleted_in_state_id TEXT
)
```

### Symbol Aliases (`symbol_aliases`)

Stores alias/redirect relationships (Section 4.4):

```sql
CREATE TABLE symbol_aliases (
  old_stable_id TEXT NOT NULL,
  new_stable_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  confidence REAL NOT NULL CHECK(confidence >= 0.0 AND confidence <= 1.0),
  created_at TEXT NOT NULL,
  created_state_id TEXT NOT NULL,
  PRIMARY KEY (old_stable_id, new_stable_id)
)
```

### Modules (`modules`)

Stores detected modules:

```sql
CREATE TABLE modules (
  module_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL,
  manifest_type TEXT,
  detected_at TEXT NOT NULL,
  state_id TEXT NOT NULL
)
```

### Dependency Edges (`dependency_edges`)

Stores module dependencies:

```sql
CREATE TABLE dependency_edges (
  from_module TEXT NOT NULL,
  to_module TEXT NOT NULL,
  kind TEXT NOT NULL,
  strength INTEGER NOT NULL,
  PRIMARY KEY (from_module, to_module)
)
```

## Cache Tiers

The storage layer implements three cache tiers as specified in Section 9:

### 1. Query Cache

- **TTL**: 300 seconds (5 minutes)
- **Key includes**: Query parameters + `headCommit`
- **Use case**: Cache query results that depend on HEAD commit
- **Table**: `query_cache`

### 2. View Cache

- **TTL**: 3600 seconds (1 hour)
- **Key includes**: View parameters + `repoStateId`
- **Use case**: Cache expensive view computations
- **Table**: `view_cache`

### 3. Negative Cache

- **TTL**: Variable by error type (5-60 seconds)
- **Key includes**: Query parameters + `repoStateId`
- **Use case**: Cache errors to avoid repeated failed operations
- **Table**: `negative_cache`

## Negative Cache Policies

Different error types have different TTLs and behaviors (Section 9.2):

| Error Type | TTL | Triggers Warmup | Description |
|------------|-----|-----------------|-------------|
| `symbol-not-found` | 60s | No | Symbol doesn't exist in codebase |
| `backend-unavailable` | 15s | No | Backend service is down |
| `workspace-not-ready` | 10s | Yes | LSP workspace initializing |
| `timeout` | 5s | No | Query timed out |
| `index-not-found` | 60s | No | SCIP index not found |
| `parse-error` | 60s | No | Failed to parse file/query |

## Cache Invalidation

Caches are automatically invalidated when (Section 9.3):

1. Repository state changes (different `repoStateId`)
2. HEAD commit changes (for query cache)
3. Manual invalidation via API
4. Entry expires based on TTL

## Usage Examples

### Initialize Database

```go
import (
    "github.com/ckb/ckb/internal/storage"
    "github.com/ckb/ckb/internal/logging"
)

logger := logging.NewLogger(logging.Config{
    Format: logging.HumanFormat,
    Level:  logging.InfoLevel,
})

db, err := storage.Open("/path/to/repo", logger)
if err != nil {
    return err
}
defer db.Close()
```

### Symbol Mappings

```go
repo := storage.NewSymbolRepository(db)

// Create
mapping := &storage.SymbolMapping{
    StableID:            "sym-123",
    State:               "active",
    FingerprintJSON:     `{"name":"myFunc","kind":"function"}`,
    LocationJSON:        `{"path":"main.go","line":42}`,
    LastVerifiedAt:      time.Now(),
    LastVerifiedStateID: "state-xyz",
}
err := repo.Create(mapping)

// Retrieve
retrieved, err := repo.GetByStableID("sym-123")

// Mark as deleted (tombstone)
err = repo.MarkAsDeleted("sym-123", "state-new")
```

### Caching

```go
cache := storage.NewCache(db)

// Query cache
err := cache.SetQueryCache("key", `{"result":"data"}`, "commit-abc", "state-xyz", 300)
value, found, err := cache.GetQueryCache("key", "commit-abc")

// View cache
err := cache.SetViewCache("view-key", `{"view":"data"}`, "state-xyz", 3600)
value, found, err := cache.GetViewCache("view-key", "state-xyz")

// Negative cache with policies
manager := storage.NewNegativeCacheManager(cache)
err := manager.CacheError("key", storage.SymbolNotFound, "Not found", "state-xyz")
entry, err := manager.CheckError("key", "state-xyz")

// Invalidate by state
err = cache.InvalidateByStateID("state-xyz")

// Cleanup expired entries
err = cache.CleanupExpiredEntries()
```

### Modules and Dependencies

```go
moduleRepo := storage.NewModuleRepository(db)
depRepo := storage.NewDependencyRepository(db)

// Create module
module := &storage.Module{
    ModuleID:   "mod-core",
    Name:       "core",
    RootPath:   "src/core",
    DetectedAt: time.Now(),
    StateID:    "state-123",
}
err := moduleRepo.Create(module)

// Create dependency
edge := &storage.DependencyEdge{
    FromModule: "mod-core",
    ToModule:   "mod-utils",
    Kind:       "import",
    Strength:   10,
}
err := depRepo.Create(edge)

// Query dependencies
deps, err := depRepo.GetByFromModule("mod-core")
reverseDeps, err := depRepo.GetByToModule("mod-utils")
```

### Transactions

```go
err := db.WithTx(func(tx *sql.Tx) error {
    // All operations in this function are part of one transaction
    // If any operation fails, everything is rolled back
    _, err := tx.Exec("INSERT INTO ...")
    return err
})
```

## Testing

Run tests with:

```bash
go test ./internal/storage/
```

Tests use temporary databases that are automatically cleaned up.

## Performance

The database is configured with optimized pragmas:

- **WAL mode**: Better concurrency for read/write operations
- **64MB cache**: Fast in-memory lookup
- **256MB mmap**: Memory-mapped I/O for performance
- **5 second busy timeout**: Handles concurrent access

## Schema Migrations

The storage layer supports schema migrations. Current schema version: **1**

When schema changes are needed:
1. Increment `currentSchemaVersion` in `schema.go`
2. Add migration function in `runMigrations()`
3. Migrations run automatically on database open

## Design Document References

This implementation follows the CKB Design Document:

- **Section 4.3**: Symbol mappings with tombstones
- **Section 4.4**: Alias/redirect table
- **Section 9**: Cache tiers and policies
- **Section 9.2**: Negative cache policies
- **Section 9.3**: Cache invalidation triggers

# Phase 1.4: Storage Layer Implementation

**Status**: COMPLETED
**Date**: 2025-12-16
**Implementation Path**: `/Users/lisa/Work/Ideas/CodeMCP/internal/storage/`

## Overview

Implemented the complete storage layer for CKB using SQLite (modernc.org/sqlite - pure Go, no CGO). The storage layer provides persistent storage for symbol mappings, aliases, modules, dependencies, and three-tier caching.

## Files Implemented

1. **db.go** (4,111 bytes)
   - Database connection management
   - Transaction helpers (`WithTx`, `BeginTx`)
   - SQLite pragmas for performance (WAL mode, 64MB cache, 256MB mmap)
   - Schema initialization and migrations
   - Direct query methods (`Exec`, `Query`, `QueryRow`)

2. **schema.go** (9,586 bytes)
   - Schema version tracking (current: v1)
   - Table creation for all entities:
     - `symbol_mappings` - Symbol mappings with tombstones (Section 4.3)
     - `symbol_aliases` - Alias/redirect table (Section 4.4)
     - `modules` - Module cache
     - `dependency_edges` - Module dependencies
     - `query_cache`, `view_cache`, `negative_cache` - Cache tiers (Section 9)
   - Index creation for query optimization
   - Migration framework for future schema changes

3. **cache.go** (10,594 bytes)
   - Three-tier cache implementation:
     - **Query Cache**: TTL 300s, includes headCommit
     - **View Cache**: TTL 3600s, includes repoStateId
     - **Negative Cache**: TTL 60s (variable by type), includes repoStateId
   - Cache operations: Get, Set, Invalidate, InvalidateAll
   - State-based invalidation (`InvalidateByStateID`)
   - Automatic expiration handling
   - Cache statistics (`GetCacheStats`)
   - Periodic cleanup (`CleanupExpiredEntries`)

4. **negative_cache.go** (5,956 bytes)
   - Error type definitions:
     - `symbol-not-found` (60s TTL)
     - `backend-unavailable` (15s TTL)
     - `workspace-not-ready` (10s TTL, triggers warmup)
     - `timeout` (5s TTL)
     - `index-not-found` (60s TTL)
     - `parse-error` (60s TTL)
   - Policy enforcement per Section 9.2
   - `NegativeCacheManager` for high-level operations
   - Error statistics tracking

5. **repositories.go** (15,664 bytes)
   - **SymbolRepository**: CRUD for symbol_mappings
     - Create, GetByStableID, Update, Delete
     - MarkAsDeleted (tombstone support)
     - ListByState
   - **AliasRepository**: CRUD for symbol_aliases
     - Create, GetByOldStableID, Delete
   - **ModuleRepository**: CRUD for modules
     - Create, GetByID, ListAll, Delete
   - **DependencyRepository**: CRUD for dependency_edges
     - Create, GetByFromModule, GetByToModule, Delete
   - Proper timestamp handling (RFC3339 format)
   - NULL-safe SQL operations

6. **storage_test.go** (10,956 bytes)
   - Comprehensive test suite:
     - Database initialization
     - Symbol repository operations
     - Alias repository operations
     - Cache operations (all three tiers)
     - Negative cache manager
     - Module and dependency repositories
   - Temporary database setup/teardown
   - Tests verify: creation, retrieval, updates, deletions, expiration

7. **example_usage.go** (8,213 bytes)
   - Documentation through examples:
     - Basic setup
     - Symbol CRUD operations
     - Alias management
     - Cache usage patterns
     - Negative cache with policies
     - Module and dependency tracking
     - Transaction usage
   - Serves as practical API documentation

8. **README.md** (7,206 bytes)
   - Complete documentation of storage layer
   - Table schemas with SQL
   - Cache tier descriptions
   - Negative cache policy table
   - Usage examples for all operations
   - Performance configuration details
   - Testing instructions
   - Design document cross-references

## Database Schema

### Tables Created

1. **schema_version** - Tracks schema migrations
2. **symbol_mappings** - Symbol mappings with tombstones
   - Indexes: state, backend_stable_id, last_verified_state_id, deleted_in_state_id
3. **symbol_aliases** - Symbol redirects with confidence scores
   - Indexes: new_stable_id, created_state_id
   - Foreign keys to symbol_mappings
4. **modules** - Detected modules
   - Indexes: name, root_path, state_id
5. **dependency_edges** - Module dependency graph
   - Indexes: to_module, kind
   - Foreign keys to modules
6. **query_cache** - Query result cache
   - Indexes: expires_at, state_id
7. **view_cache** - View computation cache
   - Indexes: expires_at, state_id
8. **negative_cache** - Error cache
   - Indexes: expires_at, state_id, error_type

### Database Configuration

- **Location**: `.ckb/ckb.db`
- **Journal Mode**: WAL (Write-Ahead Logging)
- **Synchronous**: NORMAL (balanced safety/performance)
- **Foreign Keys**: Enabled
- **Cache Size**: 64MB
- **MMAP Size**: 256MB
- **Busy Timeout**: 5 seconds

## Cache Implementation

### Query Cache
- **TTL**: 300 seconds (5 minutes)
- **Key Format**: `{query_params}:{head_commit}`
- **Use Case**: Results that depend on HEAD commit
- **Invalidation**: On HEAD commit change or manual

### View Cache
- **TTL**: 3600 seconds (1 hour)
- **Key Format**: `{view_params}:{repo_state_id}`
- **Use Case**: Expensive view computations
- **Invalidation**: On repo state change or manual

### Negative Cache
- **TTL**: 5-60 seconds (type-dependent)
- **Key Format**: `{query_params}:{repo_state_id}`
- **Use Case**: Avoid repeated failures
- **Invalidation**: On repo state change, manual, or expiration

## Negative Cache Policies

| Error Type | TTL | Triggers Warmup | Description |
|------------|-----|-----------------|-------------|
| symbol-not-found | 60s | No | Symbol doesn't exist |
| backend-unavailable | 15s | No | Service down |
| workspace-not-ready | 10s | Yes | Initializing |
| timeout | 5s | No | Query timeout |
| index-not-found | 60s | No | SCIP index missing |
| parse-error | 60s | No | Syntax error |

## Testing

Created comprehensive test suite covering:
- Database initialization and schema creation
- All CRUD operations for each repository
- Cache get/set/invalidate for all tiers
- Negative cache with policy enforcement
- Transaction rollback behavior
- Expiration handling
- Foreign key constraints

All tests use temporary databases for isolation.

## Dependencies Added

```
modernc.org/sqlite v1.40.1
├── modernc.org/libc v1.66.10
├── modernc.org/mathutil v1.7.1
├── modernc.org/memory v1.11.0
├── github.com/ncruces/go-strftime v0.1.9
├── github.com/google/uuid v1.6.0
├── github.com/dustin/go-humanize v1.0.1
└── github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec
```

## Integration with Existing Components

The storage layer integrates with:
- **internal/config**: Uses config for cache TTL settings
- **internal/logging**: Structured logging throughout
- **internal/repostate**: Uses RepoState.RepoStateID for cache keys

## Definition of Done - Verified

- [x] Storage layer initializes DB at `.ckb/ckb.db`
- [x] All schema tables created with proper constraints and indexes
- [x] Cache get/set works for all three tiers
- [x] Negative cache policies enforce correct TTLs
- [x] Repositories provide full CRUD operations
- [x] Transaction helpers work correctly
- [x] Cache invalidation by state ID works
- [x] Comprehensive test suite passes
- [x] Documentation complete

## Design Document Compliance

This implementation follows the CKB Design Document:

- **Section 4.3**: Symbol mappings with tombstones - Implemented in `symbol_mappings` table
- **Section 4.4**: Alias/redirect table - Implemented in `symbol_aliases` table
- **Section 9**: Cache tiers - Implemented all three cache tiers
- **Section 9.2**: Negative cache policies - Implemented with type-specific TTLs
- **Section 9.3**: Cache invalidation triggers - Implemented state-based invalidation

## Performance Characteristics

- **Read Performance**: Optimized with indexes on frequently queried columns
- **Write Performance**: WAL mode enables concurrent reads during writes
- **Cache Hit Rate**: Three-tier system reduces backend queries
- **Memory Usage**: 64MB cache + 256MB mmap = ~320MB for database layer
- **Concurrency**: 5-second busy timeout handles concurrent access
- **Expiration**: Lazy expiration (checked on access) + periodic cleanup

## Future Enhancements

1. **Metrics**: Add cache hit/miss rate tracking
2. **Warmup**: Implement warmup action for workspace-not-ready errors
3. **Compression**: Consider compressing large JSON values in cache
4. **Partitioning**: Add date-based partitioning for historical data
5. **Analytics**: Query statistics for performance tuning
6. **Backup**: Automated backup/restore functionality
7. **Replication**: Read replicas for query scalability

## Usage Example

```go
// Initialize
logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
db, err := storage.Open(repoRoot, logger)
defer db.Close()

// Create symbol mapping
symbolRepo := storage.NewSymbolRepository(db)
mapping := &storage.SymbolMapping{
    StableID:            "sym-123",
    State:               "active",
    FingerprintJSON:     `{"name":"myFunc"}`,
    LocationJSON:        `{"path":"main.go","line":42}`,
    LastVerifiedAt:      time.Now(),
    LastVerifiedStateID: "state-xyz",
}
symbolRepo.Create(mapping)

// Use cache
cache := storage.NewCache(db)
cache.SetQueryCache("key", `{"result":"data"}`, headCommit, stateID, 300)
value, found, _ := cache.GetQueryCache("key", headCommit)

// Negative cache with policies
manager := storage.NewNegativeCacheManager(cache)
manager.CacheError("key", storage.SymbolNotFound, "Not found", stateID)
```

## Conclusion

Phase 1.4 is complete. The storage layer provides a solid foundation for persisting CKB data with efficient caching, proper error handling, and comprehensive test coverage. The implementation follows all design specifications and is ready for integration with other CKB components.

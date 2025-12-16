# Identity System Architecture

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    Symbol Discovery                             │
│  (from SCIP/Glean/LSP backends)                                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│              Fingerprint Computation                            │
│                                                                 │
│  Input:                                                         │
│  - Container: "mypackage.MyClass"                               │
│  - Name: "myMethod"                                             │
│  - Kind: "method"                                               │
│  - Arity: 2                                                     │
│  - Signature: "myMethod(string, int)"                           │
│                                                                 │
│  Process:                                                       │
│  1. Normalize components                                        │
│  2. Sort deterministically                                      │
│  3. Hash with SHA-256                                           │
│                                                                 │
│  Output: "a3f5e9..." (64 char hex)                              │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│               Stable ID Generation                              │
│                                                                 │
│  Format: ckb:<repo>:sym:<fingerprint>                           │
│  Example: ckb:my-repo:sym:a3f5e9...                             │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│           Backend ID Anchoring Check                            │
│                                                                 │
│  if backend == SCIP || backend == Glean:                        │
│      store backend_stable_id (for rename detection)             │
│  else if backend == LSP:                                        │
│      skip anchoring (LSP IDs are unstable)                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│            SymbolMapping Creation                               │
│                                                                 │
│  {                                                              │
│    stableId: "ckb:repo:sym:a3f5e9...",                          │
│    backendStableId: "scip:github.com/.../MyClass#method",       │
│    fingerprint: {...},                                          │
│    state: "active",                                             │
│    location: {path: "src/main.go", line: 42, ...},             │
│    locationFreshness: "fresh",                                  │
│    definitionVersionId: "d4e6f8...",                            │
│    lastVerifiedAt: "2025-12-16T16:00:00Z",                      │
│    lastVerifiedStateId: "repo-state-123"                        │
│  }                                                              │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│              Store in Database                                  │
│                                                                 │
│  INSERT INTO symbol_mappings (...)                              │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│               Symbol Available                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Symbol Resolution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│           Query: ResolveSymbolId("ckb:repo:sym:old123")         │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│               Direct Lookup in symbol_mappings                  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                    Found? ├─── YES ───┐
                         │            │
                        NO             ▼
                         │      ┌─────────────────────────────────┐
                         │      │ Check Symbol State              │
                         │      │                                 │
                         │      │ if state == "active":           │
                         │      │     return symbol               │
                         │      │ else if state == "deleted":     │
                         │      │     return tombstone            │
                         │      └─────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│              Lookup in symbol_aliases                           │
│                                                                 │
│  SELECT new_stable_id, reason, confidence                       │
│  FROM symbol_aliases                                            │
│  WHERE old_stable_id = "ckb:repo:sym:old123"                    │
└────────────────────────┬────────────────────────────────────────┘
                         │
                    Found? ├─── NO ──→ Return "NOT_FOUND"
                         │
                        YES
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│               Recursive Resolution                              │
│                                                                 │
│  depth++                                                        │
│  visited.add("ckb:repo:sym:old123")                             │
│                                                                 │
│  Check:                                                         │
│  - depth > 3? → Return "CHAIN_TOO_DEEP"                         │
│  - new_id in visited? → Return "ALIAS_CYCLE"                    │
│                                                                 │
│  Recurse:                                                       │
│  ResolveSymbolId(new_stable_id, depth, visited)                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│           Return ResolvedSymbol                                 │
│                                                                 │
│  {                                                              │
│    symbol: {...},                    // Final symbol            │
│    redirected: true,                 // Was redirected          │
│    redirectedFrom: "ckb:...:old123", // Original request        │
│    redirectReason: "renamed",        // Why redirected          │
│    redirectConfidence: 0.95          // Match confidence        │
│  }                                                              │
└─────────────────────────────────────────────────────────────────┘
```

## Alias Creation Flow (During Refresh)

```
┌─────────────────────────────────────────────────────────────────┐
│          Trigger: Codebase Refresh                              │
│  (git commit, file changes, backend reindex)                    │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│       Fetch Current Symbols from Database                       │
│       oldMappings = SELECT * FROM symbol_mappings               │
│                     WHERE state = 'active'                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│       Scan Backend for Current Symbols                          │
│       newMappings = backend.GetAllSymbols()                     │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│       Compare: oldMappings vs newMappings                       │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│       For Each Old Symbol                                       │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
                Still exists ──YES──→ Skip (no change)
                in new set?
                         │
                        NO
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│       Strategy 1: Backend ID Match                              │
│                                                                 │
│  if old.backendStableId exists in newMappings:                  │
│      → Symbol renamed/moved but backend tracked it              │
│      → Create high-confidence alias (0.95)                      │
│      → Reason: "renamed"                                        │
└────────────────────────┬────────────────────────────────────────┘
                         │
                    Match found?
                         │
                     ├── YES ──→ Create Alias → DONE
                     │
                     NO
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│       Strategy 2: Fuzzy Match                                   │
│                                                                 │
│  For each new symbol:                                           │
│      score = 0.0                                                │
│                                                                 │
│      if same kind:                                              │
│          score += 0.3                                           │
│                                                                 │
│      if same/similar name:                                      │
│          score += 0.4 (exact) or 0.2 (similar)                  │
│                                                                 │
│      if same/similar container:                                 │
│          score += 0.2 (exact) or 0.1 (similar)                  │
│                                                                 │
│      if same/similar location:                                  │
│          score += 0.1 (same file) or 0.05 (same dir)            │
│                                                                 │
│  bestMatch = symbol with highest score                          │
│                                                                 │
│  if bestMatch.score >= 0.6:                                     │
│      → Create fuzzy alias (0.6-0.8 confidence)                  │
│      → Reason: "fuzzy-match"                                    │
└────────────────────────┬────────────────────────────────────────┘
                         │
                    Match found?
                         │
                     ├── YES ──→ Create Alias → DONE
                     │
                     NO
                     │
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│       Create Tombstone                                          │
│                                                                 │
│  UPDATE symbol_mappings SET                                     │
│      state = 'deleted',                                         │
│      deleted_at = NOW(),                                        │
│      deleted_in_state_id = current_repo_state_id                │
│  WHERE stable_id = old.stableId                                 │
│                                                                 │
│  Symbol truly deleted - no match found                          │
└─────────────────────────────────────────────────────────────────┘
```

## Backend ID Anchoring Decision Tree

```
┌─────────────────────────────────────────────────────────────────┐
│              Received Backend Symbol                            │
│  { backendId: "...", ... }                                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
                  Check backend ID format
                         │
         ┌───────────────┴───────────────┐
         │                               │
         ▼                               ▼
    Starts with               Contains "glean:"?
     "scip:"?                           │
         │                              │
        YES                            YES
         │                              │
         ▼                              ▼
┌────────────────────┐        ┌────────────────────┐
│  SCIP Backend      │        │  Glean Backend     │
│  Role: ANCHOR      │        │  Role: ANCHOR      │
│  Confidence: HIGH  │        │  Confidence: HIGH  │
│                    │        │                    │
│  Store as:         │        │  Store as:         │
│  backend_stable_id │        │  backend_stable_id │
│                    │        │                    │
│  Use for:          │        │  Use for:          │
│  - Rename detect   │        │  - Rename detect   │
│  - Alias creation  │        │  - Alias creation  │
└────────────────────┘        └────────────────────┘
         │                              │
         └──────────────┬───────────────┘
                        │
                        ▼
         ┌───────────────────────────┐
         │  All other formats        │
         │  (LSP, unknown)           │
         │  Role: RESOLVER ONLY      │
         │  Confidence: LOW          │
         │                           │
         │  Do NOT store as:         │
         │  backend_stable_id        │
         │  (leave empty)            │
         │                           │
         │  Why?                     │
         │  - May change on restart  │
         │  - Not persistence-safe   │
         │  - Backend-specific       │
         └───────────────────────────┘
```

## State Transition Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                    Symbol Lifecycle                              │
└──────────────────────────────────────────────────────────────────┘

                    ┌─────────────┐
                    │   Discovered│
                    │  (from scan)│
                    └──────┬──────┘
                           │
                           ▼
                    ┌─────────────┐
            ┌───────│   ACTIVE    │◄──────┐
            │       │             │       │
            │       └──────┬──────┘       │
            │              │              │
            │              │ Symbol       │ Rediscovered
            │              │ disappears   │ after false
            │              │              │ deletion
            │              ▼              │
            │       ┌─────────────┐       │
            │       │   DELETED   │───────┘
            │       │ (tombstone) │
            │       └──────┬──────┘
            │              │
            │              │ After retention
            │              │ period (optional)
            │              ▼
            │       ┌─────────────┐
            │       │  Purged     │
            │       │ (removed)   │
            │       └─────────────┘
            │
            │ Backend error/
            │ uncertainty
            ▼
     ┌─────────────┐
     │   UNKNOWN   │
     │             │
     └──────┬──────┘
            │
            │ Verification
            │ succeeds
            │
            └──────────────┘

States:
- ACTIVE: Symbol exists and is tracked
- DELETED: Symbol was removed (tombstone preserved)
- UNKNOWN: Uncertain state (backend unreachable, etc.)

Constraints:
- DELETED requires: deleted_at, deleted_in_state_id
- ACTIVE/UNKNOWN: deleted_at, deleted_in_state_id must be NULL
```

## Performance Characteristics

### Fingerprint Computation
- **Time Complexity**: O(1) - constant number of fields
- **Space Complexity**: O(1) - fixed 64-byte hash output
- **Determinism**: Guaranteed for same inputs

### Alias Resolution
- **Best Case**: O(1) - direct hit
- **Worst Case**: O(3) - max depth of 3
- **Average Case**: O(1.5) - most symbols are direct or 1 redirect
- **Cycle Detection**: O(n) where n = chain length (max 3)

### Fuzzy Matching
- **Time Complexity**: O(n*m) where n = old symbols, m = new symbols
- **Space Complexity**: O(n + m) - store both sets
- **Optimizations**:
  - Early termination on high scores
  - Backend ID match tried first (O(1) lookup)
  - Fuzzy match only if backend match fails

### Database Queries

#### By Stable ID (Primary Key)
```sql
SELECT * FROM symbol_mappings WHERE stable_id = ?
```
- **Index**: Primary key
- **Time**: O(log n) - B-tree lookup

#### By Backend ID
```sql
SELECT * FROM symbol_mappings WHERE backend_stable_id = ?
```
- **Index**: idx_symbol_mappings_backend_stable_id
- **Time**: O(log n) - B-tree lookup

#### Alias Lookup
```sql
SELECT * FROM symbol_aliases WHERE old_stable_id = ?
```
- **Index**: Primary key (composite)
- **Time**: O(log n) - B-tree lookup

## Scalability Considerations

### Symbol Count
- **Target**: 100K-1M symbols per repository
- **Database Size**: ~100-500 MB (with indexes)
- **Query Performance**: Sub-millisecond for indexed lookups

### Alias Chain Depth
- **Max Depth**: 3 (enforced)
- **Typical Depth**: 1 (90% of aliases are single-hop)
- **Rare Cases**: 2-3 (multiple renames/refactors)

### Refresh Performance
- **Full Refresh**: O(n*m) for fuzzy matching
- **Incremental**: O(n) for backend ID matching only
- **Optimization**: Batch processing, parallel comparison

## Error Handling

### Resolution Errors
1. **SYMBOL_NOT_FOUND**: No direct match, no alias
2. **ALIAS_CYCLE**: Circular redirect detected
3. **ALIAS_CHAIN_TOO_DEEP**: Chain exceeds max depth

### Validation Errors
1. **Invalid stable ID format**
2. **Missing required fields**
3. **State transition violations**
4. **Confidence out of range [0.0, 1.0]**

All errors include:
- Error code (stable identifier)
- Human-readable message
- Context (which ID, what depth, etc.)

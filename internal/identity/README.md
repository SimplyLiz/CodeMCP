# Identity System - Phase 1.2

This package implements the stable identity system for CKB symbols as described in Section 4 of the Design Document.

## Overview

The identity system ensures that symbols can be tracked across refactorings, renames, and other code changes. It provides:

- **Stable IDs**: Persistent identifiers that survive most refactorings
- **Definition Version IDs**: Track signature changes
- **Alias/Redirect System**: Follow symbols across renames and moves
- **Tombstones**: Track deleted symbols to avoid false matches
- **Backend ID Anchoring**: Use stable backend IDs (SCIP/Glean) as anchors

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────┐
│ SymbolIdentity                                      │
│ - Stable ID (long-term reference)                  │
│ - Definition Version ID (signature tracking)       │
│ - Fingerprint (container, name, kind, signature)   │
│ - Location + Freshness                             │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│ SymbolMapping (Database Record)                     │
│ - Identity fields                                   │
│ - State (active/deleted/unknown)                    │
│ - Backend stable ID (for anchoring)                 │
│ - Verification timestamps                           │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│ SymbolAlias (Redirects)                             │
│ - Old → New stable ID mapping                       │
│ - Reason (renamed/moved/merged/fuzzy-match)         │
│ - Confidence score                                  │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│ IdentityResolver                                    │
│ - Follows alias chains (max depth = 3)              │
│ - Returns tombstones for deleted symbols            │
│ - Cycle detection                                   │
└─────────────────────────────────────────────────────┘
```

## Files

### id.go
- `SymbolIdentity`: Core identity structure
- `VersionSemantics`: How definition versions are computed
- `LocationFreshness`: Whether location may be stale
- `Location`: Source code position
- `SymbolKind`: Symbol types (function, class, etc.)

### fingerprint.go
- `SymbolFingerprint`: Components for stable ID generation
- `ComputeStableFingerprint()`: Hash fingerprint components
- `GenerateStableId()`: Create full stable ID (format: `ckb:<repo>:sym:<hash>`)
- `NormalizeSignature()`: Normalize signatures for comparison
- `ComputeDefinitionVersionId()`: Hash full signature

### state.go
- `SymbolState`: Symbol lifecycle states (active/deleted/unknown)
- `SymbolMapping`: Complete database record
- Validation logic for state transitions
- Helper methods: `IsActive()`, `IsDeleted()`, `IsUnknown()`

### alias.go
- `AliasReason`: Why an alias was created
- `SymbolAlias`: Redirect from old to new stable ID
- Validation and confidence helpers

### resolution.go
- `IdentityResolver`: Main resolution engine
- `ResolveSymbolId()`: Follow alias chains to current symbol
- Cycle detection and max depth enforcement
- Returns `ResolvedSymbol` with redirect metadata

### alias_creation.go
- `AliasCreator`: Creates aliases during symbol refresh
- `CreateAliasesOnRefresh()`: Compare old/new mappings
- `FindFuzzyMatch()`: Heuristic matching for disappeared symbols
- Two-strategy approach:
  1. Backend ID match (high confidence ~0.95)
  2. Fuzzy match (lower confidence 0.6-0.8)
- Creates tombstones for symbols with no match

### anchors.go
- Backend ID stability rules
- `GetBackendIdRole()`: Determine if backend can be anchor
- SCIP/Glean: Primary anchors (stable)
- LSP: Resolver only (unstable)

### repository.go
- `SymbolRepository`: Database CRUD operations
- `Get()`: Retrieve by stable ID
- `GetByBackendId()`: Retrieve by backend stable ID
- `Create()`: Insert new symbol
- `Update()`: Update existing symbol
- `MarkDeleted()`: Create tombstone
- `List()`: Query with filters

## Usage Examples

### Creating a Symbol Identity

```go
// Create fingerprint
fp := &identity.SymbolFingerprint{
    QualifiedContainer:  "mypackage.MyClass",
    Name:                "myMethod",
    Kind:                identity.KindMethod,
    Arity:               2,
    SignatureNormalized: "myMethod(string,int)",
}

// Generate stable ID
stableId := identity.GenerateStableId("my-repo", fp)
// Result: "ckb:my-repo:sym:<hash>"

// Create definition version ID
defVersionId := identity.ComputeDefinitionVersionId(fullSignature)

// Create mapping
mapping := &identity.SymbolMapping{
    StableId:                   stableId,
    BackendStableId:            backendId, // From SCIP/Glean
    Fingerprint:                fp,
    State:                      identity.StateActive,
    Location:                   &identity.Location{Path: "src/main.go", Line: 42, Column: 5},
    LocationFreshness:          identity.Fresh,
    DefinitionVersionId:        defVersionId,
    DefinitionVersionSemantics: identity.BackendDefinitionHash,
    LastVerifiedAt:             time.Now().UTC().Format(time.RFC3339),
    LastVerifiedStateId:        repoStateId,
}

// Save to database
repo := identity.NewSymbolRepository(db, logger)
err := repo.Create(mapping)
```

### Resolving a Symbol ID

```go
resolver := identity.NewIdentityResolver(db, logger)

resolved, err := resolver.ResolveSymbolId(requestedId)
if err != nil {
    // Handle error
}

if resolved.Deleted {
    // Symbol was deleted
    fmt.Printf("Symbol deleted at %s\n", resolved.DeletedAt)
} else if resolved.Symbol != nil {
    // Found symbol (possibly via redirect)
    fmt.Printf("Found symbol: %s\n", resolved.Symbol.StableId)

    if resolved.Redirected {
        fmt.Printf("Redirected from: %s (reason: %s, confidence: %.2f)\n",
            resolved.RedirectedFrom,
            resolved.RedirectReason,
            resolved.RedirectConfidence)
    }
} else {
    // Not found
    fmt.Printf("Symbol not found: %s\n", resolved.Error)
}
```

### Creating Aliases During Refresh

```go
creator := identity.NewAliasCreator(db, logger)

// Compare old and new symbol lists
err := creator.CreateAliasesOnRefresh(oldMappings, newMappings, newRepoStateId)
if err != nil {
    // Handle error
}

// This will:
// 1. Match symbols by backend ID (high confidence)
// 2. Try fuzzy matching for unmatched symbols
// 3. Create tombstones for symbols with no match
```

### Checking Backend ID Stability

```go
// SCIP backend ID
backendId := "scip:github.com/user/repo:src/main.go:MyClass#method"
if identity.CanBeIdAnchor(backendId) {
    // Use as stable anchor
    mapping.BackendStableId = backendId
}

// LSP backend ID
lspId := "file:///path/to/file.ts#L10:5"
if !identity.CanBeIdAnchor(lspId) {
    // Don't use as anchor - LSP IDs are unstable
    mapping.BackendStableId = "" // Leave empty or use SCIP ID
}
```

## Key Design Decisions

### 1. Stable ID Format
- Format: `ckb:<repo>:sym:<fingerprint-hash>`
- Hash is computed from: container, name, kind, arity, signature
- Deterministic and collision-resistant

### 2. Two-Level ID System
- **Stable ID**: Changes only when symbol identity changes (rename, move)
- **Definition Version ID**: Changes when signature changes
- This allows tracking both identity and evolution

### 3. Backend ID Anchoring
- SCIP and Glean IDs are stable across sessions
- LSP IDs are unstable and should not be used as anchors
- When available, backend IDs provide high-confidence rename detection

### 4. Alias Chain Resolution
- Maximum depth of 3 to prevent performance issues
- Cycle detection to prevent infinite loops
- Returns metadata about redirects for transparency

### 5. Fuzzy Matching Strategy
- Multi-factor scoring: name, kind, container, location
- Minimum confidence threshold of 0.6
- Lower confidence than backend ID matching (0.6-0.8 vs 0.95)

### 6. Tombstones
- Deleted symbols remain in database with state=deleted
- Prevents false matches when symbols are actually gone
- Includes deletion timestamp and repo state ID

## Database Schema

The identity system uses two tables:

### symbol_mappings
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
    deleted_in_state_id TEXT,
    CHECK(
        (state = 'deleted' AND deleted_at IS NOT NULL) OR
        (state != 'deleted' AND deleted_at IS NULL)
    )
);
```

### symbol_aliases
```sql
CREATE TABLE symbol_aliases (
    old_stable_id TEXT NOT NULL,
    new_stable_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    confidence REAL NOT NULL CHECK(confidence >= 0.0 AND confidence <= 1.0),
    created_at TEXT NOT NULL,
    created_state_id TEXT NOT NULL,
    PRIMARY KEY (old_stable_id, new_stable_id),
    FOREIGN KEY (old_stable_id) REFERENCES symbol_mappings(stable_id) ON DELETE CASCADE,
    FOREIGN KEY (new_stable_id) REFERENCES symbol_mappings(stable_id) ON DELETE CASCADE
);
```

## Testing

The package includes comprehensive tests in `identity_test.go`:

- Fingerprint computation and determinism
- Symbol CRUD operations
- Alias resolution with redirects
- Alias creation from old/new mappings
- Backend ID role detection
- Tombstone handling

Run tests with:
```bash
go test ./internal/identity/...
```

## Integration Points

This package is used by:

- **Backend adapters** (SCIP, Glean, LSP) to generate and resolve symbol IDs
- **Query engine** to resolve symbol references
- **Refresh process** to maintain ID stability across code changes
- **getSymbol tool** to return stable identity information

## Future Enhancements

- Machine learning for improved fuzzy matching
- Cross-repository symbol linking
- Identity confidence scoring
- Symbol genealogy tracking

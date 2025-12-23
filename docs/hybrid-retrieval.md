# Hybrid Retrieval for CKB (v7.4)

Graph-based retrieval enhancement using Personalized PageRank (PPR) and multi-signal fusion scoring.

## Overview

CKB v7.4 adds hybrid retrieval that combines:
- **Lexical search** (FTS5) - Fast text matching
- **Graph proximity** (PPR) - Symbol relationship awareness
- **Signal fusion** - Weighted combination of multiple ranking signals

This approach is based on 2024-2025 research showing graph-augmented retrieval outperforms embedding-only approaches for code navigation tasks.

## Results

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Recall@10 | 62.1% | 100% | +61% |
| MRR | 0.546 | 0.914 | +67% |
| Latency | 29.4ms | 4.5ms | -85% |

## Components

### 1. Eval Suite (`internal/eval/`)

Retrieval quality measurement framework.

**Files:**
- `suite.go` - Test harness with recall@K, MRR, latency metrics
- `fixtures/*.json` - Test cases (needle, ranking, expansion types)

**CLI:**
```bash
ckb eval                        # Run built-in fixtures
ckb eval --fixtures=./tests.json  # Custom fixtures
ckb eval --format=json          # JSON output
```

**Test Types:**
- **needle** - Find at least one expected symbol in top-K
- **ranking** - Verify expected symbol ranks in top positions
- **expansion** - Check graph connectivity to related symbols

### 2. PPR Algorithm (`internal/graph/`)

Personalized PageRank implementation for symbol graphs.

**Files:**
- `ppr.go` - Core PPR algorithm with power iteration
- `ppr_test.go` - Unit tests and benchmarks
- `builder.go` - Graph construction from SCIP index

**Algorithm:**
```
Input: seed nodes (FTS hits), SCIP graph, damping=0.85
Output: ranked nodes with scores + explanation paths

1. Build sparse adjacency from call/reference edges
2. Initialize scores on seed nodes
3. Power iterate: scores = damping * A @ scores + (1-damping) * teleport
4. Return top-K with backtracked paths
```

**Edge Weights:**
| Edge Type | Weight |
|-----------|--------|
| Call | 1.0 |
| Definition | 0.9 |
| Reference | 0.8 |
| Implements | 0.7 |
| Type-of | 0.6 |
| Same-module | 0.3 |

### 3. Fusion Scoring (`internal/query/ranking.go`)

Multi-signal ranking that combines:

| Signal | Weight | Source |
|--------|--------|--------|
| FTS score | 0.40 | Full-text search |
| PPR score | 0.30 | Graph proximity |
| Hotspot | 0.15 | Recent churn |
| Recency | 0.10 | File modification |
| Exact match | 0.05 | Name equality |

**Integration:**
PPR re-ranking is applied in `SearchSymbols` after initial FTS results, before limit truncation.

### 4. Export Organizer (`internal/export/organizer.go`)

Structured context packing for LLM consumption.

**Features:**
- Module map with symbol counts and top exports
- Cross-module bridge detection
- Clustered output by module
- Importance-ordered symbols

**Output Format:**
```markdown
## Module Map
| Module | Symbols | Files | Key Exports |
|--------|---------|-------|-------------|
| internal/query | 150 | 12 | Engine, SearchSymbols |

## Cross-Module Connections
- internal/query → internal/backends
- internal/mcp → internal/query

## Module Details
### internal/query/
**engine.go**
  $ Engine
  # SearchSymbols() [c=12] ★★
```

## Implementation Milestones

### Phase 1: Eval Suite ✓
- [x] Create `internal/eval/suite.go` with metrics calculation
- [x] Build fixtures from CKB codebase (29 test cases)
- [x] Add `ckb eval` CLI command
- [x] Establish baseline: 62.1% recall, 0.546 MRR

### Phase 2: PPR Implementation ✓
- [x] Implement sparse graph representation
- [x] Implement power iteration with damping
- [x] Add path backtracking for explainability
- [x] Add graph builder from SCIP index
- [x] Unit tests and benchmarks

### Phase 3: Fusion Scoring ✓
- [x] Create `internal/query/ranking.go`
- [x] Integrate PPR into `SearchSymbols`
- [x] Weighted signal combination
- [x] Validate: 93.1% recall, 0.891 MRR

### Phase 4: Export Organizer ✓
- [x] Create `internal/export/organizer.go`
- [x] Module map generation
- [x] Cross-module bridge detection
- [x] Integrate into `exportForLLM` MCP tool

## Files Changed

**New Files:**
```
cmd/ckb/eval.go                     # CLI command
internal/eval/suite.go              # Eval framework
internal/eval/fixtures/ckb_core.json
internal/eval/fixtures/ckb_advanced.json
internal/graph/ppr.go               # PPR algorithm
internal/graph/ppr_test.go          # Tests
internal/graph/builder.go           # Graph from SCIP
internal/query/ranking.go           # Fusion scoring
internal/export/organizer.go        # Context organizer
```

**Modified Files:**
```
internal/query/symbols.go           # PPR integration
internal/mcp/tool_impls_v65.go      # Organizer integration
```

## Configuration

No configuration required. PPR is automatically applied when:
- SCIP index is available
- Search returns > 3 results
- Graph has nodes

## Research Basis

Based on these 2024-2025 papers:

1. **HippoRAG 2** (ICML 2025) - PPR over knowledge graphs improves associative retrieval
2. **CodeRAG** (Sep 2025) - Multi-path retrieval + reranking beats single-path
3. **GraphCoder** (Jun 2024) - Code context graphs for repo-level retrieval
4. **GraphRAG surveys** - Explicit organizer step improves context packing

## What's NOT Included (By Design)

Per CKB's "structured over semantic" principle:

| Feature | Why Skipped |
|---------|-------------|
| Embeddings | Adds complexity, PPR sufficient for navigation |
| Learned reranker | Deterministic scoring works |
| CFG/PDG analysis | Call graph is enough |
| External vector DB | Violates single-binary principle |

## Future Work (v2 If Needed)

- Learned cross-encoder reranker (if precision gaps measured)
- Graph embeddings via node2vec (if semantic search demand)
- Full call graph expansion for expansion tests

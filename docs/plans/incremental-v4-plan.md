# CKB Incremental Indexing v4 — Implementation Plan

**Version:** 4.0
**Branch:** `feature/7.3-incremental-v4`
**Status:** ✅ Complete
**Prerequisites:** v7.3 (Federation remote mode complete)

## Implementation Summary

| Phase | Status | Commits |
|-------|--------|---------|
| Phase 1: Delta Artifacts | ✅ Complete | `b37617e`, `85da6b2` |
| Phase 2: FTS5 Search | ✅ Complete | `b37617e` |
| Phase 3: Analytics-Lite | ⏭️ Skipped | Covered by v6.4+ telemetry/hotspots |
| Phase 4: Operational Hardening | ✅ Complete | `6cf55b0` |
| Phase 5: Language Maturity | ✅ Complete | `2efee20` |

---

## Overview

v4 makes CKB **production-grade**: faster ingestion, modern search, operational reliability, usage analytics, and mature language support.

**Goals:**
- Fast ingestion via CI-generated delta artifacts (O(delta) not O(N))
- Modern FTS5 search (instant, not SQLite LIKE scans)
- Operational reliability (snapshots, compaction, monitoring)
- Analytics-lite (unused symbols, hotspots, search terms)
- Language maturity (TS/Python solid, Rust/Java promoted)

---

## Phase 1: Delta Artifacts from CI

### Problem
v3 ingestion computes diffs by comparing staging DB to current DB — O(N) over all symbols/refs/calls. Painful for repos with 500k+ symbols.

### Solution
CI emits delta manifest alongside the index, server applies delta directly.

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/ckb/diff.go` | Create | `ckb diff` CLI command |
| `internal/diff/types.go` | Create | Delta JSON schema types |
| `internal/diff/generator.go` | Create | Delta generation (compare two DBs) |
| `internal/diff/validator.go` | Create | Delta validation logic |
| `internal/diff/hasher.go` | Create | Canonical hash computation |
| `internal/api/handlers_delta.go` | Modify | Delta ingestion endpoint |
| `internal/api/index_processor.go` | Modify | Support delta ingestion path |

### Delta JSON Schema

```json
{
  "delta_schema_version": 1,
  "base_snapshot_id": "sha256:abc123...",
  "new_snapshot_id": "sha256:def456...",
  "commit": "def456789",
  "timestamp": 1703260800,
  "deltas": {
    "symbols": {
      "added": ["scip-go...NewFunc()."],
      "modified": ["scip-go...ChangedFunc()."],
      "deleted": ["scip-go...RemovedFunc()."]
    },
    "refs": {
      "added": [{"pk": "f_abc:42:12:scip-go...Foo().", "data": {...}}],
      "deleted": ["f_abc:50:5:scip-go...Old()."]
    },
    "callgraph": { "added": [...], "deleted": [...] },
    "files": { "added": [...], "modified": [...], "deleted": [...] }
  },
  "stats": { "total_added": 45, "total_modified": 12, "total_deleted": 8 }
}
```

### CLI Command

```bash
ckb diff \
  --base /path/to/old-snapshot.db \
  --new /path/to/new-snapshot.db \
  --output delta.json
```

### Hash Canonicalization

Length-prefixed fields to avoid delimiter ambiguity:
- Format: `${len}:${value}${len}:${value}...` where NULL → `0:`
- Algorithm: SHA-256, lowercase hex output

| Entity | Fields (in order) |
|--------|-------------------|
| symbol | id, name, kind, file_id, line, language, signature, documentation |
| ref | from_file_id, line, col, to_symbol_id, kind, language |
| callgraph | caller_file_id, call_line, call_col, callee_id, caller_id, language |

### Server Validation

1. Verify `delta_schema_version` is supported
2. Verify `base_snapshot_id` matches current active snapshot
3. Verify counts match stats
4. Spot-check hashes for modified entities
5. If validation fails → reject, require full snapshot

### Configuration

```toml
[ingestion]
delta_artifacts = true
delta_validation = "strict"  # strict | permissive
fallback_to_staging_diff = true
race_strategy = "strict"  # strict | practical (v4.1+)
```

### Tasks

- [x] Define delta JSON schema types
- [x] Implement canonical hash computation
- [x] Implement `ckb diff` CLI command
- [x] Implement delta validation
- [x] Add delta ingestion path to server
- [x] Add fallback to staging diff
- [ ] CI integration examples (GitHub Actions, GitLab CI)
- [x] Tests for delta generation and ingestion

---

## Phase 2: FTS5 Search

### Problem
v3 search uses SQLite `LIKE '%query%'` — scans entire tables. Unusable at 100k+ symbols.

### Solution
SQLite FTS5 virtual tables with automatic sync triggers.

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/storage/schema.go` | Modify | Add FTS5 tables and triggers |
| `internal/storage/fts.go` | Create | FTS maintenance (rebuild, vacuum) |
| `internal/query/search.go` | Modify | Use FTS5 for search |
| `internal/backends/scip/search.go` | Modify | FTS5 query building |

### Schema

```sql
CREATE VIRTUAL TABLE symbols_fts USING fts5(
    name,
    documentation,
    signature,
    content='symbols',
    content_rowid='rowid'
);

CREATE TRIGGER symbols_ai AFTER INSERT ON symbols BEGIN
    INSERT INTO symbols_fts(rowid, name, documentation, signature)
    VALUES (new.rowid, new.name, new.documentation, new.signature);
END;

CREATE TRIGGER symbols_au AFTER UPDATE ON symbols BEGIN
    INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
    VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
    INSERT INTO symbols_fts(rowid, name, documentation, signature)
    VALUES (new.rowid, new.name, new.documentation, new.signature);
END;

CREATE TRIGGER symbols_ad AFTER DELETE ON symbols BEGIN
    INSERT INTO symbols_fts(symbols_fts, rowid, name, documentation, signature)
    VALUES ('delete', old.rowid, old.name, old.documentation, old.signature);
END;
```

### Search Semantics

| Match Type | FTS5 Query | Ranking |
|------------|------------|---------|
| Exact | `"Authenticate"` | 1.0 |
| Prefix | `Auth*` | 0.8 |
| Substring | Fallback to LIKE if FTS misses | 0.5 |

### FTS Ingestion Strategy

**Full sync:**
1. DROP triggers
2. Bulk INSERT into symbols
3. DELETE FROM symbols_fts
4. INSERT INTO symbols_fts(symbols_fts) VALUES('rebuild')
5. Re-CREATE triggers

**Delta ingestion:**
- Triggers remain active
- If symbol changes > threshold → use full-sync strategy

### Configuration

```toml
[search]
backend = "fts5"  # fts5 | like
fts_trigger_threshold_symbols = 1000
fts_rebuild_timeout_s = 300
fts_rebuild_on_full_sync = true
```

### Performance Targets

| Repo Size | v3 (LIKE) | v4 (FTS5) |
|-----------|-----------|-----------|
| 10k symbols | 50ms | 5ms |
| 100k symbols | 500ms | 10ms |
| 500k symbols | 2.5s | 20ms |

### Tasks

- [x] Add FTS5 schema and triggers
- [x] Implement FTS maintenance (rebuild, vacuum, integrity-check)
- [x] Refactor search to use FTS5 first, LIKE fallback
- [x] Implement ranking and result merging
- [x] Add FTS corruption recovery
- [ ] Performance benchmarks
- [x] Tests for FTS search

---

## Phase 3: Analytics-Lite

> **Note:** Phase 3 was largely skipped as existing v6.4+ features (telemetry, hotspots, dead code detection) already provide this functionality.

### Problem
Users can't answer: "What symbols are never used?" "What's changing the most?" "What do people search for?"

### Solution
Lightweight usage metrics populated during ingestion.

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/storage/schema.go` | Modify | Add analytics tables |
| `internal/analytics/types.go` | Create | Analytics types |
| `internal/analytics/usage.go` | Create | Symbol usage tracking |
| `internal/analytics/unused.go` | Create | Unused symbol detection |
| `internal/analytics/hotspots.go` | Create | Churn hotspots |
| `internal/analytics/search_log.go` | Create | Search term tracking |
| `internal/api/handlers_analytics.go` | Create | Analytics endpoints |
| `cmd/ckb/analytics.go` | Create | CLI commands |

### Schema

```sql
CREATE TABLE symbol_usage (
    symbol_id TEXT PRIMARY KEY,
    ref_count INTEGER NOT NULL,
    caller_count INTEGER NOT NULL,
    last_modified_seq INTEGER,
    first_seen_seq INTEGER,
    churn_count INTEGER DEFAULT 0
);

CREATE INDEX idx_symbol_usage_refs ON symbol_usage(ref_count);
CREATE INDEX idx_symbol_usage_churn ON symbol_usage(churn_count);

CREATE TABLE search_log (
    id INTEGER PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    query_hash TEXT NOT NULL,
    query_normalized TEXT NOT NULL,
    result_count INTEGER,
    repo TEXT NOT NULL
);
```

### Endpoints

```
GET /repos/{repo}/analytics/unused
  ?kind=Function|Method|Type
  &min_age_days=30
  &limit=100

GET /repos/{repo}/analytics/hotspots
  ?metric=refs|callers|churn
  &limit=100

GET /repos/{repo}/analytics/search-terms
  ?days=30
  &limit=50
```

### Exported Symbol Heuristics

| Language | Exported when |
|----------|---------------|
| Go | Uppercase first letter |
| TypeScript | `export` or `export default` |
| Python | In `__all__`, or no leading `_` |
| Rust | `pub` visibility |
| Java/Kotlin | `public` modifier |

### Configuration

```toml
[analytics]
enabled = true
track_search_queries = true
unused_symbol_age_days = 30
churn_window_days = 90
```

### Tasks

- [ ] Add symbol_usage and search_log tables
- [ ] Populate usage stats during ingestion
- [ ] Implement unused symbols endpoint
- [ ] Implement churn hotspots endpoint
- [ ] Implement search term tracking (anonymized)
- [ ] Add CLI commands for analytics
- [ ] Add MCP tools for analytics
- [ ] Privacy review
- [ ] Tests for analytics

---

## Phase 4: Operational Hardening

### Problem
v3 lacks production polish: no compaction, no monitoring hooks, manual cleanup.

### Solution
Automatic compaction, health endpoints, Prometheus metrics, graceful degradation.

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/daemon/compaction.go` | Create | Snapshot compaction scheduler |
| `internal/api/handlers_health.go` | Modify | Enhanced health endpoints |
| `internal/api/metrics.go` | Create | Prometheus metrics |
| `internal/api/middleware_load.go` | Create | Load shedding middleware |

### Compaction

```toml
[server.compaction]
enabled = true
keep_snapshots = 5
keep_days = 30
compact_journal_after_days = 7
schedule = "0 3 * * *"  # Cron: 3 AM daily
```

**Tasks:**
1. Delete old snapshot DBs (keep last N)
2. Prune change journal
3. VACUUM FTS tables
4. Update storage metrics

### Health Endpoints

```
GET /health
  → 200 OK | 503 Service Unavailable

GET /health/detailed
  → {
      "status": "healthy",
      "repos": {...},
      "storage": {...},
      "journal": {...}
    }
```

### Prometheus Metrics

```
ckb_ingestion_duration_seconds{repo, type}
ckb_ingestion_entities_total{repo, entity, op}
ckb_search_duration_seconds{repo, type}
ckb_search_results_total{repo}
ckb_storage_bytes{repo, type}
ckb_snapshots_total{repo}
ckb_ratelimit_exceeded_total{repo, principal}
```

### Graceful Degradation

1. Prioritize `/symbols/{id}` and `/search` over bulk operations
2. Return `503 Retry-After` for expensive operations under load
3. Shed load from low-priority tokens first

### Configuration

```toml
[server.metrics]
enabled = true
endpoint = "/metrics"
format = "prometheus"

[server.health]
enabled = true
detailed_endpoint = "/health/detailed"
```

### Tasks

- [x] Implement compaction scheduler
- [x] Implement detailed health endpoint
- [x] Add Prometheus metrics
- [x] Implement load shedding middleware
- [ ] Create Grafana dashboard examples
- [x] Tests for compaction and metrics

---

## Phase 5: Language Maturity

### Problem
TypeScript and Python have rough edges. Rust and Java are "medium priority" but not production-ready.

### Solution
Tiered language support with clear promotion criteria.

### Support Tiers

| Tier | Languages | Guarantee |
|------|-----------|-----------|
| **Tier 1** | Go | Full support, all features |
| **Tier 2** | TypeScript, Python | Full support, known edge cases |
| **Tier 3** | Rust, Java/Kotlin | Basic support, callgraph may be incomplete |
| **Tier 4** | C#, others | Experimental |

### Promotion Criteria

To move from Tier N to Tier N-1:
1. Indexer stability: < 1% failure rate
2. Symbol coverage: > 95% of definitions
3. Ref accuracy: > 90% resolve correctly
4. Callgraph quality: `ok` for > 80% of repos
5. Test suite: Golden tests pass

### v4 Language Goals

| Language | Current | v4 Target | Work Required |
|----------|---------|-----------|---------------|
| Go | Tier 1 | Tier 1 | Maintenance |
| TypeScript | Tier 2 | Tier 1 | Edge case fixes, monorepo support |
| Python | Tier 2 | Tier 2 | Virtual env handling |
| Rust | Tier 3 | Tier 2 | rust-analyzer SCIP stability |
| Java/Kotlin | Tier 3 | Tier 2 | scip-java, Gradle support |
| C# | — | Tier 4 | Evaluate scip-dotnet |

### Per-Language Quality Dashboard

Expose in `/repos/{repo}/meta`:

```json
{
  "languages": {
    "go": {
      "tier": 1,
      "quality": "ok",
      "symbol_count": 5000,
      "ref_accuracy": 0.98,
      "callgraph_quality": "ok"
    }
  }
}
```

### Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/project/detect.go` | Modify | Better monorepo detection |
| `internal/project/typescript.go` | Create | TS-specific handling |
| `internal/project/python.go` | Create | Python venv detection |
| `internal/api/handlers_meta.go` | Modify | Language quality dashboard |

### Tasks

- [ ] TypeScript edge case fixes
- [x] TypeScript monorepo support
- [x] Python virtual env detection
- [ ] Rust tier promotion testing
- [ ] Java/Kotlin Gradle support
- [ ] C# experimental evaluation
- [x] Per-language quality dashboard
- [ ] Language golden tests

---

## Implementation Order

| Phase | Feature | Effort | Priority |
|-------|---------|--------|----------|
| 1 | Delta Artifacts | 2-3 weeks | P0 (core performance) |
| 2 | FTS5 Search | 2 weeks | P0 (core UX) |
| 3 | Analytics-Lite | 2 weeks | P1 (value-add) |
| 4 | Operational Hardening | 2 weeks | P1 (production-ready) |
| 5 | Language Maturity | 3-4 weeks | P2 (quality) |

**Total:** 11-15 weeks

---

## Rollout Strategy

| Release | Features |
|---------|----------|
| v7.3.1-alpha | Delta artifacts + FTS5 search |
| v7.3.1-beta | + Analytics-lite |
| v7.3.1 | + Operational hardening |
| v7.3.2 | + TypeScript Tier 1 promotion |
| v7.3.3 | + Rust/Java Tier 2 promotion |

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Delta format wrong | Version schema, validate strictly |
| FTS5 corruption | Rebuild on startup if corrupt, LIKE fallback |
| Analytics misleading | Clear caveats, "may miss dynamic usage" |
| Compaction deletes data | Conservative defaults, dry-run mode |
| Language promotion aggressive | Clear criteria, automated quality gates |

---

## Success Criteria

1. **Ingestion:** Delta artifacts reduce time by > 80%
2. **Search:** P95 latency < 50ms for 100k symbol repos
3. **Analytics:** Unused symbols with < 5% false positives
4. **Operations:** Zero manual intervention for routine maintenance
5. **Languages:** TypeScript promoted to Tier 1

---

## NOT in v4

| Feature | Reason | Future |
|---------|--------|--------|
| Cross-language linking | Separate correctness contracts | v5 |
| SaaS multi-tenancy | Infrastructure complexity | v5+ |
| Real-time sync (WebSocket) | Limited demand | v5+ |
| Fuzzy search | FTS5 doesn't support well | v4.x |

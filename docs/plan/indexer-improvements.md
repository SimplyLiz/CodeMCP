# Index Management Cleanup

**Status:** DONE
**Created:** 2025-12-26
**PR:** #57

---

## Summary

Housekeeping PR to close out stale items from index management backlog.

## Changes

| Item | Action | Rationale |
|------|--------|-----------|
| `lastRefresh` in MCP | Fixed | Data computed but not returned - doc/impl mismatch |
| File watcher in MCP | Closed (by design) | Polling is intentional for cross-platform simplicity |
| Preemptive warmup | Closed (not needed) | SQLite + incremental indexing already fast |

---

## What Was Fixed

### `lastRefresh` Not in MCP Response

`internal/mcp/tool_impls.go` now includes `lastRefresh` in getStatus:

```json
{
  "lastRefresh": {
    "at": "2024-12-25T10:00:00Z",
    "trigger": "head-changed",
    "triggerInfo": "branch or commit changed",
    "durationMs": 1200
  }
}
```

### Warmup TODO Removed

`internal/storage/negative_cache.go` had a stale TODO for "later phase" warmup.
Removed because:
- SQLite is already memory-mapped
- Incremental indexing is fast (1-2s typical)
- No measured slow path to optimize

---

## What Was Closed (Not Implemented)

### File Watcher in MCP Mode

Already documented as intentional in Index-Management.md:
> MCP watch mode uses **polling only** (no file watchers)

Rationale:
- Cross-platform consistency
- Simpler debugging
- 10s poll is acceptable for interactive use

### Preemptive Warmup

No problem to solve. Branch switch flow:
1. Detect HEAD change (10s poll)
2. Incremental reindex (1-2s)
3. First query hits SQLite (already fast)

---

## Future Improvements (v8.1+)

Higher-value work if index experience needs improvement:

1. **Progress reporting** — Show parsing/indexing progress for large repos
2. **Partial index** — `ckb index --path internal/auth/` for monorepos
3. **Index health in doctor** — Show files changed since last index

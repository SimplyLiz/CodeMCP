# Index Refresh Improvements

**Status:** Backlog
**Priority:** Low - daemon already handles most use cases well
**Original investigation:** Git hooks for instant reindex

---

## Summary

Investigation into git hooks for instant reindex revealed that **the daemon already solves this problem** via `.git/HEAD` file watching. The real gaps are visibility and MCP-mode parity.

---

## Current State

| Mode | Mechanism | Latency | Branch Switch? |
|------|-----------|---------|----------------|
| **Daemon** | Polling `.git/HEAD` + `.git/index` (2s interval, 5s debounce) | ~7s worst case | ✅ Works well |
| **MCP `--watch`** | Polling + `CheckFreshness()` | 10s default | ✅ Works well |

**Note:** Both modes use polling, not fsnotify. This was chosen for simplicity and cross-platform compatibility. See `internal/watcher/watcher.go:199`.

**Conclusion:** Both modes now provide reasonable latency for branch switching. The 10s MCP interval (reduced from 30s in v7.6) is acceptable for most workflows.

---

## Potential Improvements

### 1. Add `.git/HEAD` Watching to MCP Mode

**Gap:** `ckb mcp --watch` uses pure polling, unlike daemon's file watcher.

**Fix:** Add lightweight file watching alongside polling:
```go
watcher.Add(filepath.Join(repoRoot, ".git", "HEAD"))
watcher.Add(filepath.Join(repoRoot, ".git", "index"))
```

**Effort:** Medium (1-2 days)
**Impact:** High for MCP-only users

### 2. Add Refresh Source Visibility

**Gap:** Users can't see what triggered a refresh.

**Fix:** Add trigger reason to logs and `getStatus` response:
```
[INFO] Refresh triggered: .git/HEAD changed (branch: main → feature/auth)
```

```json
{
  "lastRefresh": {
    "at": "2024-12-25T10:00:00Z",
    "trigger": "HEAD changed (main → feature/auth)",
    "duration": "1.2s"
  }
}
```

**Effort:** Low (0.5 days)
**Impact:** Medium - helps users understand the system

### 3. Document Daemon Watcher

**Gap:** Users don't know daemon already handles branch switches.

**Fix:** Add to docs:
```markdown
## Branch Switching

When using the daemon, branch switches are detected automatically:
1. You run `git checkout feature-branch`
2. Git updates `.git/HEAD`
3. Daemon detects the change within 5 seconds
4. Reindex triggers automatically

**No git hooks needed.**
```

**Effort:** Low (0.5 days)
**Impact:** High - reduces confusion

### 4. Reduce Default MCP Poll Interval ✅ DONE (v7.6)

**Was:** 30s
**Now:** 10s

```go
const defaultWatchInterval = 10 * time.Second
```

**Effort:** Trivial
**Impact:** Low-Medium

### 5. Preemptive Index Warmup

**Gap:** First query after branch switch is slow (cold cache).

**Fix:** When HEAD changes, preload index into memory before user queries.

**Effort:** Medium
**Impact:** Medium

---

## What Was Considered But Rejected

### Git Hooks

**Original idea:** Install `post-merge`, `post-checkout` hooks that call daemon API.

**Why rejected:**
- Daemon already watches `.git/HEAD` with ~5s latency
- 5s → 0s is imperceptible for most workflows
- Adds complexity (cross-platform scripts, hook chaining with Husky, etc.)
- Maintenance cost outweighs marginal benefit

**Verdict:** Skip entirely. File watching is sufficient.

---

## Where Git Hooks / Webhooks WOULD Matter

These are **different use cases** from local branch switching:

### 1. CI/CD Integration
```yaml
# GitHub Actions example
- name: Notify CKB
  run: curl -X POST http://ckb-server/api/v1/refresh
```
Trigger reindex after deploy completes. Daemon isn't running in CI.

### 2. Federated Repositories
When upstream repo pushes, downstream CKB instances need notification. Webhooks from GitHub/GitLab would trigger federated index refresh.

### 3. Team-Shared Index Servers
Central CKB server serving multiple developers. Git server webhooks notify on any push.

### 4. Editor Integration (Future)
VS Code/Cursor extensions could call refresh API on file save for real-time indexing during editing (not just git events).

**These are separate features** - not relevant to the "instant reindex on branch switch" question for local development.

---

## Recommended Action

**Status: Largely addressed.** The poll interval reduction (Priority 4) is done. Both daemon and MCP modes now have reasonable latency.

**Remaining opportunities:**
1. Add refresh source visibility (Priority 2) - helps users understand what triggered reindex
2. Document the polling behavior more clearly (Priority 3)

**Not recommended:**
- Git hooks (marginal benefit for significant complexity)
- fsnotify migration (current polling works well, cross-platform compatible)

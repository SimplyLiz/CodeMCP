# Git Hook Trigger Support for CKB

**Priority:** Low-Medium (table stakes, not a differentiator)
**Effort:** ~2-3 days
**Status:** Planned

## Goal
Add git hook support so reindexing happens **instantly** after `git pull`, `git checkout`, etc., instead of waiting for the 2-second poll interval.

## Context: Why This Is Low Priority

Research shows users' real pain points are **stale/incorrect data** and **large codebase performance**, not the 2-second polling latency. The current daemon setup (5s debounced file watching) is already fast enough for most workflows.

**What would have higher impact:**
1. **Index projection from stale commits** (Sourcegraph approach) - project results from last-known-good index using git diff
2. **Editor-integrated refresh** - hook into VS Code/Cursor file save events
3. **Index warmup on `ckb mcp` start** - preload before user queries

Git hooks are worth doing as table stakes (users expect them), but shouldn't be over-engineered.

## Design

**Approach:** Install shell scripts in `.git/hooks/` that call the daemon's `/api/v1/refresh` endpoint.

**Hook types to support:**
- `post-merge` - fires after `git pull`
- `post-checkout` - fires after `git checkout` / `git switch`
- `post-rewrite` - fires after `git rebase` (optional)

**Coexistence:** Hooks provide instant triggering; polling remains as fallback for repos without hooks installed.

---

## Implementation Steps

### 1. Create CLI commands (`cmd/ckb/hooks.go`)

New subcommands:
```
ckb hooks install [--type=post-merge,post-checkout] [--force]
ckb hooks remove [--type=...]
ckb hooks status
```

**install:**
- Detect git root from cwd
- Check for existing hooks (warn if present, require --force to overwrite)
- Write hook scripts to `.git/hooks/{post-merge,post-checkout}`
- Make scripts executable (chmod +x)

**remove:**
- Remove only CKB-generated hooks (check for marker comment)
- Optionally restore backup if one exists

**status:**
- Show which hooks are installed
- Verify they're executable and contain valid CKB markers

### 2. Hook script template

```bash
#!/bin/bash
# CKB auto-generated hook - do not edit
# Triggers reindex on git operations

CKB_DAEMON_PORT="${CKB_DAEMON_PORT:-8371}"
REPO_PATH="$(git rev-parse --show-toplevel 2>/dev/null)"

# Fire-and-forget POST to daemon (timeout 2s, ignore errors)
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d "{\"repo\":\"$REPO_PATH\"}" \
  --connect-timeout 2 \
  --max-time 5 \
  "http://127.0.0.1:$CKB_DAEMON_PORT/api/v1/refresh" \
  >/dev/null 2>&1 &

exit 0
```

Key properties:
- Fire-and-forget (`&` at end) - git operations don't block
- Fail silently if daemon not running
- Uses env var for port override
- Marker comment for identification

### 3. Daemon side (minimal changes)

The existing `/api/v1/refresh` endpoint already handles this. Optional enhancements:

- Add `source` field to RefreshRequest: `{"repo":"...","source":"hook:post-merge"}`
- Log hook-triggered refreshes distinctly
- Emit webhook event with hook source info

**File:** `internal/daemon/server.go:247-320` (RefreshRequest/handleRefresh)

### 4. Config (optional)

Add to DaemonConfig in `internal/config/config.go`:
```go
type HooksConfig struct {
    AutoInstall bool `json:"autoInstall"` // Install hooks on `ckb init`
}
```

---

## Files to Create/Modify

| File | Action |
|------|--------|
| `cmd/ckb/hooks.go` | **CREATE** - CLI commands |
| `internal/hooks/templates.go` | **CREATE** - Hook script templates |
| `internal/hooks/install.go` | **CREATE** - Hook installation logic |
| `internal/daemon/server.go` | **MODIFY** - Add `source` to RefreshRequest (optional) |
| `cmd/ckb/init.go` | **MODIFY** - Offer hook install during init (optional) |

---

## Testing

1. **Manual test flow:**
   - `ckb hooks install` in a repo
   - Verify `.git/hooks/post-merge` exists and is executable
   - Run `git pull` (with remote changes)
   - Confirm daemon logs show refresh triggered

2. **Unit tests:**
   - `internal/hooks/install_test.go` - template generation, permission setting
   - Test marker detection for safe removal

---

## Edge Cases

- **No daemon running:** Hook fails silently, polling picks up changes later
- **Bare repos:** Skip (no working tree)
- **Worktrees:** Use `git rev-parse --git-dir` for correct hooks path

---

## Decision Point 1: Handling Existing Hooks

When `.git/hooks/post-merge` already exists, what should `ckb hooks install` do?

### Option A: Chain to existing hook (Recommended)
Rename existing hook to `post-merge.pre-ckb`, then call it from CKB's hook.

```bash
#!/bin/bash
# CKB hook - chains to original
# ... CKB logic ...

# Call original hook if it exists
if [ -x ".git/hooks/post-merge.pre-ckb" ]; then
    .git/hooks/post-merge.pre-ckb "$@"
fi
```

| Pros | Cons |
|------|------|
| Preserves user's existing automation | More complex implementation |
| Non-destructive, easy to reverse | Need to handle nested chaining if run twice |
| Professional/expected behavior | Must pass through hook arguments correctly |

### Option B: Warn and require `--force`
Refuse to install if hook exists. With `--force`, overwrite (no backup).

| Pros | Cons |
|------|------|
| Simple implementation | Destroys user's existing hooks |
| Clear user intent with --force | User must manually backup |
| No complexity around chaining | Frustrating UX for users with hooks |

### Option C: Warn, require `--force`, but backup
Like B, but saves original to `post-merge.backup` before overwriting.

| Pros | Cons |
|------|------|
| Simple, but recoverable | User still loses their automation |
| Easy to restore manually | No auto-chaining means hooks don't run together |

**Recommendation:** Option A (chain). It's the behavior users expect from tools that install hooks. The complexity is manageable - just exec the backup at the end.

---

## Decision Point 2: Auto-install hooks on `ckb init`

Should `ckb init` automatically install git hooks?

### Option A: Auto-install by default (opt-out)
`ckb init` installs hooks unless `--no-hooks` flag is passed.

| Pros | Cons |
|------|------|
| Instant reindex works out of the box | Modifies `.git/hooks` without explicit consent |
| Best UX for new users | May surprise users who audit their hooks |
| Matches "it just works" philosophy | Could conflict with team hook policies |

### Option B: Don't auto-install (opt-in) (Recommended)
`ckb init` does NOT install hooks. User runs `ckb hooks install` separately.

| Pros | Cons |
|------|------|
| Explicit user consent for hook modification | Extra step for full functionality |
| No surprises in `.git/` directory | Users may not discover the feature |
| Safe for repos with existing hook policies | Polling still works, just 2s delay |
| Clear separation of concerns | |

### Option C: Prompt during init
`ckb init` asks "Install git hooks for instant reindex? [y/N]"

| Pros | Cons |
|------|------|
| User makes informed choice | Adds friction to init flow |
| Educational - explains the feature | Doesn't work in non-interactive/CI contexts |

**Recommendation:** Option B (opt-in). Modifying `.git/hooks` is a side effect users should explicitly request. The polling fallback means CKB works fine without hooks - they're an optimization, not a requirement. Print a hint at the end of `ckb init`:

```
Hint: Run 'ckb hooks install' for instant reindex on git pull/checkout
```

---

## Minimal Viable Scope

To minimize investment while still shipping the feature:

| Include | Skip |
|---------|------|
| `post-merge` + `post-checkout` hooks | `post-rewrite` (rare use case) |
| Basic chaining (rename to `.pre-ckb`) | Nested chaining detection |
| `ckb hooks install` + `ckb hooks remove` | Extensive `ckb hooks status` diagnostics |
| Fire-and-forget curl | Retry logic, health checks |

**Test manually, skip exhaustive unit tests.** This is a low-risk feature.

---

## Future: Higher-Impact Features

These would differentiate CKB more than git hooks:

### 1. Index Projection from Stale Commits (Sourcegraph approach)

When index is stale for HEAD, project results from last-indexed commit using git diff:
- Query the stale index
- Adjust file paths and line numbers using diff hunks
- Return results with confidence score

**Why:** User never sees "stale" warnings. Results are useful immediately.

### 2. Editor-Integrated Refresh

Hook into VS Code/Cursor file save events via:
- Extension API (requires building a VS Code extension)
- Or: watch for file mtime changes more aggressively

**Why:** Real-time indexing during editing, not just git operations.

### 3. Index Warmup on MCP Start

When `ckb mcp` starts, preload the index into memory:
- Parse SCIP index
- Warm symbol caches
- Pre-compute common queries

**Why:** First query is fast, no cold-start penalty.

### 4. Stale Reference Tolerance

Instead of "may be stale" warnings, return results with confidence scores:
- `confidence: 0.95` - index matches HEAD
- `confidence: 0.7` - index is 3 commits behind, projected
- `confidence: 0.4` - index is very stale, use with caution

Let the AI decide if it needs fresher data.

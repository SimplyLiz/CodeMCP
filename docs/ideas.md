# CKB Feature Ideas

Ideas evaluated against what CKB actually is and already has. Organized by adoption impact.

---

## What CKB Already Has (Don't Reinvent)

| Feature | Already Exists As |
|---------|-------------------|
| Budget control | `budget.*` config + `tier` system (fast/standard/full) |
| Risk factor breakdown | `auditRisk` with 8 weighted factors |
| Privacy/redaction | `privacy.mode: "redacted"` + index server per-repo privacy |
| Scheduled tasks | Daemon scheduler with cron/interval expressions |
| Export for LLM | `exportForLLM` tool |
| Provenance | Every response has `provenance.backends` + `repoStateId` |
| Drilldowns | Truncated results include `drilldowns` with follow-up queries |
| Multi-repo switching | `ckb repo` commands + `switchRepo` MCP tool |
| Doc-symbol linking | `getDocsForSymbol`, `checkDocStaleness`, `getDocCoverage` |
| Confidence levels | 0.0-1.0 scores throughout the system |
| Query coalescing | `queryPolicy.coalesceWindowMs` already implemented |
| Webhooks | Slack, PagerDuty, Discord delivery with retry logic |
| Setup generator | `ckb setup --tool=cursor|claude-code|windsurf|vscode` |
| Diagnostics | `ckb doctor` checks config, backends, index freshness |

---

## Tier 0: Adoption Blockers (Fix First)

These prevent users from successfully adopting CKB. Ship before features.

### 1. Fix npx Sandbox Problem at Product Level
**Problem:** The #1 cause of CKB connection failures. When launched via `npx @tastehub/ckb mcp`, it runs from a temp dir and can't find `.ckb/`. Current workaround is `CKB_REPO` or `--repo` flag.

**Fix:** Ship a tiny Node wrapper in the npm package that:
- Detects when running via npx
- Automatically sets `--repo` to the real project cwd
- Preserves the "small single Go binary" runtime

**Effort:** Low - Node shim in npm package.

| Assessment | |
|------------|---|
| **Value** | 95% |
| **How it helps** | Eliminates the #1 support issue. First impressions become "it just works" instead of "connection failed." |
| **Adoption rate** | 100% of npx users (automatic, invisible fix) |

---

### 2. Stable Error Taxonomy + Actionable Failures
**What:** Machine- and human-friendly error contract on every response:
```json
{
  "error": {
    "code": "SCIP_INDEX_MISSING",
    "message": "No SCIP index found",
    "action": "Run 'ckb index' to generate the index",
    "docs": "https://..."
  }
}
```

**Error codes:**
- `REPO_NOT_FOUND` - no `.ckb/` directory
- `SCIP_INDEX_MISSING` - need to run `ckb index`
- `SCIP_INDEX_STALE` - index N commits behind
- `LSP_TIMEOUT` - language server didn't respond
- `LSP_NOT_CONFIGURED` - no server for this language
- `PERMISSION_DENIED` - auth required
- `REDACTION_ACTIVE` - output limited by privacy mode
- `BUDGET_EXCEEDED` - results truncated

**Effort:** Low-Medium - define taxonomy, add to error paths.

| Assessment | |
|------------|---|
| **Value** | 85% |
| **How it helps** | Users self-recover instead of asking for help. AI clients can auto-suggest fixes. Reduces support load significantly. |
| **Adoption rate** | 100% (automatic, all errors use this format) |

---

### 3. MCP Test Harness
**What:** `ckb mcp:test` command that:
- Runs scripted JSON-RPC calls against stdio
- Validates schemas match documentation
- Checks output contracts don't regress
- Reports latency vs budget targets

**Effort:** Medium - test framework + fixtures.

| Assessment | |
|------------|---|
| **Value** | 70% |
| **How it helps** | Prevents breaking changes from shipping. Catches regressions before users do. Essential for maintaining 74 tools. |
| **Adoption rate** | 5% of users (mostly CKB developers), but 100% of users benefit indirectly |

---

### 4. Enhanced Doctor Output
**What:** Extend `ckb doctor` to show:
- Repo root detection + why it was chosen
- Per-backend status with specific reason (missing binary, missing index, timeout)
- Index freshness as "N commits behind" + age
- Tool latency estimates vs budget
- Cache health summary
- Suggested fix commands for each issue

**Effort:** Low - enhance existing command.

| Assessment | |
|------------|---|
| **Value** | 75% |
| **How it helps** | Self-service debugging. Users paste doctor output in issues = faster resolution. Reduces "why isn't it working?" questions. |
| **Adoption rate** | 40% of users when troubleshooting, 100% benefit from better issue reports |

---

### 5. Quickstart Command
**What:** `ckb quickstart` that runs the golden path in one command:
1. `ckb init` (if needed)
2. Detect language
3. `ckb index` (or select fast tier if indexer unavailable)
4. Print the exact `ckb setup` command for detected editor

**Effort:** Low - orchestration of existing commands.

| Assessment | |
|------------|---|
| **Value** | 80% |
| **How it helps** | Time-to-first-success drops from 10+ minutes to 3 minutes. Reduces "where do I start?" drop-off. |
| **Adoption rate** | 70% of new users (power users will still use individual commands) |

---

### 6. Standardized Response Schema
**What:** Hard contract across ALL tools (cheap + heavy):
```json
{
  "data": { ... },
  "confidence": {
    "score": 0.85,
    "tier": "high",
    "reasons": ["SCIP index used", "index is fresh"]
  },
  "limitations": [
    "Dynamic dispatch may hide some callers",
    "LSP backend unavailable"
  ],
  "suggested_next_calls": [
    {"tool": "getCallGraph", "args": {"symbolId": "..."}, "reason": "See who calls this"},
    {"tool": "explainSymbol", "args": {"symbolId": "..."}, "reason": "Get full context"}
  ]
}
```

**Effort:** Medium - audit all tools, enforce schema.

| Assessment | |
|------------|---|
| **Value** | 90% |
| **How it helps** | AI clients behave predictably. Reduces hallucinated leaps. Users understand what CKB knows vs. guesses. Trust increases. |
| **Adoption rate** | 100% (automatic, all responses use this format) |

---

## Tier 1: High Value, Low Effort

### 7. Enhanced Provenance Display
**What:** Make existing provenance more useful:
- Show confidence tier labels ("high", "medium", "low") not just numbers
- Add `indexAge` to every response (e.g., "3 commits behind")
- Include truncation summary ("Showing 10 of 47 modules")

**Effort:** Low - surface existing data better.

| Assessment | |
|------------|---|
| **Value** | 50% |
| **How it helps** | Users understand result quality at a glance. Partially superseded by #6 (standardized schema) if that ships first. |
| **Adoption rate** | 100% (automatic), but only ~30% of users will actively notice/use it |

---

### 8. PR Comment GitHub Action
**What:** A GitHub Action that:
- Runs `summarizePr` on PR open/update
- Posts formatted comment with risk, reviewers, impacted modules
- Links to relevant ADRs

**Effort:** Low - wrapper around existing tools.

| Assessment | |
|------------|---|
| **Value** | 85% |
| **How it helps** | Viral growth mechanism. Every PR comment exposes CKB to reviewers who didn't install it. Concrete CI/CD value proposition. |
| **Adoption rate** | 20% of users (teams with CI culture), but high visibility multiplier |

---

### 9. Personal Config Overlays
**What:** `~/.ckb/overlays/<repo-name>.json` for user-scoped settings:
- Custom ignore paths without committing to repo
- Personal budget preferences
- Disable specific tool families per-repo

**Effort:** Low - config merge logic.

| Assessment | |
|------------|---|
| **Value** | 40% |
| **How it helps** | Removes friction when trying CKB in repos where you can't commit changes. Helps consultants/contractors. |
| **Adoption rate** | 15% of users (most users control their repo config) |

---

### 10. Cache Stats Command
**What:** `ckb cache stats` showing:
- Hit/miss rates per tier (query/view/negative)
- Cache size on disk
- "Clear negative cache only" option

**Effort:** Low - expose existing metrics.

| Assessment | |
|------------|---|
| **Value** | 30% |
| **How it helps** | Performance debugging for power users. Answers "why is it slow?" Minimal user-facing impact for most. |
| **Adoption rate** | 10% of users (developers debugging performance issues) |

---

## Tier 2: High Value, Medium Effort

### 11. MCP Resources for Persistent Artifacts
**What:** Expose stable MCP resources (not just tools):
- `/architecture/overview` - cached architecture view
- `/hotspots/top` - current top hotspots
- `/ownership/map` - ownership summary
- `/decisions/recent` - recent ADRs

Resources are backed by existing caches/jobs. Clients can subscribe/poll instead of repeatedly calling tools.

**Effort:** Medium - implement MCP resources protocol.

| Assessment | |
|------------|---|
| **Value** | 55% |
| **How it helps** | Reduces token usage in AI clients. Faster context loading. But depends on client support for MCP resources (limited today). |
| **Adoption rate** | 5-10% initially (depends on Claude/Cursor implementing resource support) |

---

### 12. Per-Client Quotas (Daemon/Index Server)
**What:** Multi-tenant rate limiting:
- Per-token tool budgets (X calls/minute)
- Per-client concurrency limits
- Request tracing with `requestId` across retries

**Effort:** Medium - quota tracking + enforcement.

| Assessment | |
|------------|---|
| **Value** | 35% |
| **How it helps** | Enterprise readiness. Prevents one user from DOSing shared server. Required for hosted/team deployments. |
| **Adoption rate** | 5% of users (teams running shared daemon/index server) |

---

### 13. Onboarding Report
**What:** `ckb onboard` command generating single report:
- Module map, top entry points, key concepts
- Owner summary, "start here" suggestions

**Effort:** Medium - orchestration + formatting.

| Assessment | |
|------------|---|
| **Value** | 60% |
| **How it helps** | New-to-codebase developers get oriented fast. Shareable artifact for team onboarding. Demo-friendly output. |
| **Adoption rate** | 25% of users (new team members, consultants, anyone joining a project) |

---

### 14. Test Recommender
**What:** `ckb suggest-tests <changed-files>` returning:
- Test files that map to changed code
- Tests based on co-change coupling
- Priority score based on impact

**Effort:** Medium - test discovery heuristics.

| Assessment | |
|------------|---|
| **Value** | 70% |
| **How it helps** | Direct CI cost savings. Faster feedback loops. Answers the common question "what tests should I run?" |
| **Adoption rate** | 30% of users (teams with test suites who want faster CI) |

---

### 15. Confidence Factor Breakdown
**What:** Add `confidence.factors` showing why confidence is what it is:
```json
{
  "confidence": 0.72,
  "factors": {
    "backendSource": "scip",
    "indexFreshness": 0.9,
    "dynamicDispatchRisk": -0.18
  }
}
```

**Effort:** Medium - instrument confidence calculations.

| Assessment | |
|------------|---|
| **Value** | 45% |
| **How it helps** | Transparency builds trust. Users understand when to verify results. Partially covered by #6 if that ships. |
| **Adoption rate** | 20% of users will read factors, 100% benefit from AI clients using them |

---

### 16. Incremental Cache Invalidation
**What:** Instead of invalidating all cache on git change:
- Map changed files → affected symbol caches only
- Keep unrelated module caches intact

**Effort:** Medium-High - dependency tracking.

| Assessment | |
|------------|---|
| **Value** | 65% |
| **How it helps** | Major perf win for active repos. Reduces "stale feeling" when working. Most impactful for large monorepos. |
| **Adoption rate** | 100% (automatic), but only ~30% of users in active repos will notice improvement |

---

## Tier 3: Uncertain ROI

### 17. Scoped Export for Sharing
**What:** `ckb export --scope pr:123` producing focused JSON for team handoff.

**Why hesitant:** `exportForLLM` + drilldowns may be sufficient.

| Assessment | |
|------------|---|
| **Value** | 25% |
| **How it helps** | Team knowledge transfer. Incident handoff. But overlaps with existing export. |
| **Adoption rate** | 5% of users (team leads, incident responders) |

---

### 18. Streaming for Heavy Operations
**What:** Partial results during `refreshArchitecture`.

**Why hesitant:** MCP transport support varies.

| Assessment | |
|------------|---|
| **Value** | 30% |
| **How it helps** | Better UX for long operations. But current job system works, and client support is inconsistent. |
| **Adoption rate** | Depends entirely on MCP client implementation |

---

### 19. Cross-Backend Consistency Checks
**What:** Debug command comparing SCIP vs LSP results.

**Why hesitant:** Developer tool, not user-facing.

| Assessment | |
|------------|---|
| **Value** | 15% |
| **How it helps** | Catches indexing bugs. Internal quality tool. Zero direct user value. |
| **Adoption rate** | <1% (CKB developers only) |

---

## Future Consideration: Hybrid Retrieval

**Principle tension:** "Structured over semantic" is a design principle, but modern expectations include semantic search.

**Possible approach:** Optional retrieval provider (embeddings) used *only* for candidate selection. SCIP/LSP/Git remains the source of truth. Embeddings help find what to look up, not what to return.

**Status:** Placeholder. Keep CKB as structured analysis. Only revisit if users consistently ask "why can't I search by concept?"

| Assessment | |
|------------|---|
| **Value** | 50% (uncertain) |
| **How it helps** | "Search by intent" instead of exact symbol names. Matches 2025 expectations. But changes product identity. |
| **Adoption rate** | Unknown - depends on user demand signals |

---

## Removed Ideas (Already Exist or Don't Fit)

| Idea | Status |
|------|--------|
| Budget Profiles | Already exists: `tier` config |
| Risk Factor Breakdown | Already exists: `auditRisk` |
| Scheduled Health Reports | Already exists: Daemon scheduler + webhooks |
| Setup Generator | Already exists: `ckb setup --tool=X` |
| Dynamic Toolsets | Wrong layer: Client's job |
| Output Templates | Unclear value: JSON + human format sufficient |
| Investigation Macros | Wrong layer: Client orchestrates |
| Context Packs | Mostly exists: `exportForLLM` + cache + drilldowns |
| Session Bookmarks | Use `ckb export --out snapshot.json` |
| Egress/Redaction | Mostly exists: `privacy.mode` + index server settings |
| Reference Set Comparisons | Too early: Needs federation adoption |
| Prompt Injection Defenses | Edge case: Read-only limits attack surface |
| Semantic Summaries | Out of scope: Would need LLM |
| LocalDocs Expansion | Already exists: Doc-symbol linking |
| Full Workflow System | Wrong product: Use GitHub Actions |
| Graph Store Rewrite | Premature: SQLite works |
| Interactive REPL | Wrong layer: AI clients are interactive |
| Code Reuse Engine | Out of scope: Needs embeddings |
| ASCII Visualizer | Low value: Clients prefer JSON |

---

## Complementary Products (Later)

### A. CKB Studio (Local UI)
Web UI over daemon API for non-AI-client access.

**When:** Only if users explicitly request.

| Assessment | |
|------------|---|
| **Value** | 35% |
| **How it helps** | Non-AI-client users can explore. Demo purposes. But violates "MCP-first" principle. |
| **Adoption rate** | 10% of users (those without AI client access) |

---

### B. Architecture Contracts CI
Enforceable module boundaries. Detect dependency violations.

**When:** When `MODULES.toml` `allowed_dependencies` is widely used.

| Assessment | |
|------------|---|
| **Value** | 60% |
| **How it helps** | Governance without code changes. Architecture enforcement. But requires ecosystem maturity first. |
| **Adoption rate** | 15% of users (teams with architectural discipline) |

---

## Summary: Value vs Effort Matrix

| Idea | Value | Adoption | Effort | Priority |
|------|-------|----------|--------|----------|
| 1. npx fix | 95% | 100% | Low | **Ship now** |
| 6. Standardized schema | 90% | 100% | Medium | **Ship now** |
| 2. Error taxonomy | 85% | 100% | Low-Med | **Ship now** |
| 8. PR GitHub Action | 85% | 20% | Low | **Ship now** |
| 5. Quickstart | 80% | 70% | Low | **Ship now** |
| 4. Enhanced doctor | 75% | 40% | Low | **Ship now** |
| 3. MCP test harness | 70% | 5% | Medium | This sprint |
| 14. Test recommender | 70% | 30% | Medium | Next sprint |
| 16. Incremental cache | 65% | 30% | High | Later |
| 13. Onboarding report | 60% | 25% | Medium | Next sprint |
| 11. MCP resources | 55% | 10% | Medium | Wait for client support |
| 7. Enhanced provenance | 50% | 30% | Low | Superseded by #6 |
| 15. Confidence factors | 45% | 20% | Medium | Included in #6 |
| 9. Personal overlays | 40% | 15% | Low | Nice to have |
| 12. Per-client quotas | 35% | 5% | Medium | Enterprise only |
| 10. Cache stats | 30% | 10% | Low | Nice to have |
| 17. Scoped export | 25% | 5% | Low | Skip |
| 19. Backend checks | 15% | <1% | Medium | Internal only |

---

## Priority Order

**Immediate (Adoption Blockers):**
1. npx sandbox fix (95% value, 100% adoption)
2. Stable error taxonomy (85% value, 100% adoption)
3. Standardized response schema (90% value, 100% adoption)
4. Quickstart command (80% value, 70% adoption)
5. Enhanced doctor output (75% value, 40% adoption)

**This Sprint:**
6. PR Comment GitHub Action (85% value, 20% adoption, high visibility)
7. MCP test harness (70% value, internal quality)

**Next Sprint:**
8. Test recommender (70% value, 30% adoption)
9. Onboarding report (60% value, 25% adoption)

**Later:**
10. Incremental cache invalidation (65% value, complex)
11. MCP resources (wait for client support)
12. Per-client quotas (enterprise only)

**Skip/Deprioritize:**
- Enhanced provenance (#7) - superseded by standardized schema (#6)
- Confidence factors (#15) - included in standardized schema (#6)
- Cache stats (#10) - low adoption
- Personal overlays (#9) - low adoption
- Scoped export (#17) - overlaps with existing
- Backend checks (#19) - internal only

---

## Design Principles

1. **Librarian, not author** - CKB reads and explains code, never modifies
2. **Small binary** - Single Go executable, npm/npx distributable
3. **MCP-first** - Primary interface is AI tools, not humans
4. **Structured over semantic** - Symbol tables + AST, not embeddings
5. **Don't duplicate** - Check wiki before adding features
6. **Let clients orchestrate** - CKB provides tools, AI clients sequence them
7. **Actionable errors** - Every failure tells user the exact next step

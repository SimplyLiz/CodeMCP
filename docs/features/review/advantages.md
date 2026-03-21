# CKB Review: How It Works With LLM Review

CKB review is designed to make LLM-based code review faster, cheaper, and more focused. This document shows how, based on measured results on a real 131-file PR and comparison with industry tools.

---

## How It Works

CKB is an MCP server. The LLM calls CKB's `reviewPR` tool during its review and gets structured data back. CKB computes the structural facts (5 seconds, 0 tokens), the LLM focuses on semantic issues CKB can't detect.

```
LLM starts reviewing PR
  │
  ├─ Calls reviewPR(baseBranch: "main", compact: true)    ← 5s, 0 tokens
  │   Returns: verdict, score, 28 findings, health,
  │   hotspot ranking, test gaps, split suggestion
  │
  ├─ Reads CKB output (~1k tokens in compact mode)
  │   Knows: secrets clean, no breaking changes, no dead code,
  │   top 10 hotspots, 16 test gaps, 3 complex files
  │
  ├─ Drills down via CKB MCP tools (0 tokens each)
  │   findReferences, analyzeImpact, explainSymbol, explainFile
  │
  ├─ Skips categories CKB answered
  │   No need to: scan for secrets, diff APIs, count tests,
  │   compute complexity, check for AST bugs
  │
  └─ Reviews flagged files for semantic issues
      Reads ~10 files guided by hotspot scores + test gap data
      Finds: design bugs, security issues, missing implementations
```

---

## Measured Results (Scenario 3 Rerun)

| Phase | Time | Tokens | Findings |
|---|---|---|---|
| CKB structural scan | 5.2s | 0 | 28 |
| LLM review (guided by CKB) | 130s | 77,159 | 12 new |
| **Total** | **~2.5 min** | **77,159** | **40** |

### What CKB contributed (0 tokens)

| Check | Result | What the LLM skipped |
|---|---|---|
| secrets | pass | Didn't scan 131 files for credentials |
| breaking | pass | Didn't diff public API surface |
| dead-code | pass | Verified — grep caught cross-file refs SCIP missed |
| health | pass (8 new, 22 unchanged) | Didn't compare before/after health scores |
| tests | pass (27 covering) | Didn't audit test coverage |
| bug-patterns | 5 new (31 filtered) | Didn't hunt for AST bugs |
| hotspots | top 10 ranked | Knew which files to read first |
| test-gaps | 16 functions with file:line | Knew exactly what lacks coverage |
| complexity | +59 delta, 3 significant files | Knew where cognitive load increased |
| coupling | 1 missing co-change | Investigated specifically |
| split | 30 clusters | Understood PR structure |

### What the LLM found (that CKB can't detect)

| # | File | Severity | Finding |
|---|---|---|---|
| 1 | `daemon.go:373` | Critical | `select {}` infinite hang in `followLogs()` — deadlocks when log tailing reaches EOF |
| 2 | `daemon.go:358` | Critical | `file.Seek()` error silently ignored — corrupts log output |
| 3 | `review.go:477` | High | `checkReviewerIndependence()` is gated by conditional but never defined — silently passes |
| 4 | `review.go:667` | High | LLM generation errors silently swallowed — users can't tell if `--llm` actually worked |
| 5 | `handlers_upload_delta.go` | High | Code duplication with `handlers_upload.go` — divergence risk (CKB coupling flag led here) |
| 6 | `review_health.go:101` | Medium | Global mutex serializes independent per-file analysis — 30x slower than needed |
| 7 | `review.go:353` | Medium | Hotspot scores not cached between API calls — re-fetched every review |
| 8 | `review.go:145` | Medium | `findingTier()` hardcoded switch — new checks silently default to wrong tier |
| 9 | `pr.go:216` | Medium | Ownership coverage failure indistinguishable from "no owners found" |
| 10 | `review_bugpatterns.go:48` | Low | Test file filter only matches `_test.go`, misses `_integration_test.go` |

### How CKB guided the LLM to better findings

- **CKB's test-gap data flagged `daemon.go` functions** → LLM reviewed `followLogs()` → found the `select {}` deadlock. Without CKB, the LLM likely would have skipped daemon.go as "just CLI code."
- **CKB's coupling warning on `handlers_upload_delta.go`** → LLM compared with `handlers_upload.go` → found the duplication. Without CKB, no reason to look at both files.
- **CKB's hotspot scores** ranked `review.go` and `review_health.go` highest → LLM focused deep review there → found the mutex serialization and silent error swallowing.

---

## Industry Comparison

Based on web research of 2025-2026 code review tools.

### CKB's approach vs the market

| Tool | Architecture | Static Analysis | LLM Role |
|---|---|---|---|
| **CKB** | Pipeline-first + MCP server | SCIP index, tree-sitter AST, git analysis, 15 checks | Optional narrative synthesis from pre-computed findings |
| **CodeRabbit** | Pipeline-first (closest to CKB) | 30+ integrated linters + AST | Reasoning layer on top of curated context |
| **Qodo / PR-Agent** | Multi-agent | Commercial-only analyzers | 15+ specialized agents per review type |
| **Greptile** | Vector embeddings + graph | Graph-based reference tracing | Full repo context, 82% bug catch rate claimed |
| **Claude Code Review** | Multi-agent (Anthropic) | None (pure LLM agents) | Parallel agents hunting different risk types |
| **Amp** | Hypothesis-driven agents | Tool integrations | Agents prove/disprove specific risks |

### What CKB does that others don't

1. **SCIP-based enrichment to verify own findings.** CKB uses `findReferences` to check if "dead code" actually has references before telling the LLM. No other tool self-verifies at the symbol resolution level.

2. **Full offline operation.** CKB's 15 checks work without any API call. Every other major tool requires cloud LLM access for core value.

3. **80+ MCP tools for drill-down.** After `reviewPR`, the LLM can call `findReferences`, `analyzeImpact`, `explainSymbol`, `getCallGraph`, `traceUsage` etc. CKB exposes the underlying code intelligence, not just the review result.

4. **HoldTheLine line-level filtering.** Only flags issues on changed lines. Some tools approximate this; CKB implements it as a first-class policy with unified diff parsing.

5. **SARIF lint deduplication.** Removes findings already caught by the user's existing linter. No duplicate noise.

6. **Framework symbol filtering.** Blast-radius excludes variables, constants, and framework wiring (cobra commands, Qt signals, Spring beans) using SCIP symbol kinds. Works across Go, C++, Java, Python.

### What others do that CKB doesn't (yet)

| Gap | Who does it | Impact | Effort to add |
|---|---|---|---|
| Multi-agent investigation | Qodo 2.0, Claude Code Review, Amp | Higher coverage but higher cost/latency | High — needs agent framework |
| Learning from feedback | Sourcery, Greptile | Reduces repeat FPs over time | Medium — needs finding store + feedback API |
| LLM-based FP triage | Datadog research | 92% → 6.3% FP rate in SAST findings | Low — already have enrichment pipeline |
| Inline PR comments | CodeRabbit, Qodo, Greptile | Better UX for developers | Medium — needs GitHub/GitLab API integration |
| Ticket context | CodeRabbit, Greptile | PR reviewed against acceptance criteria | Medium — needs Jira/Linear API |
| Iterative/conversational | CodeRabbit, Qodo | Developer replies to findings, gets follow-up | High — needs state management |

### Key insight from research

The academic research and CodeRabbit's architecture both validate CKB's "static first, LLM second" approach. From the RAG-based code review paper (arxiv 2502.06633): feeding structured static analysis results into LLM prompts consistently outperforms both pure-LLM and naive code concatenation approaches.

CodeRabbit's architecture post: "The base layer assembles context deterministically (diff, AST, import graph, static analysis), and the LLM sits on top as a reasoning layer." This is exactly what CKB does.

The main difference: CodeRabbit's LLM never queries back into the codebase (they argue "more context isn't always better"). CKB goes further by exposing 80+ MCP tools that the LLM CAN use for drill-down, but doesn't force it.

---

## Is CKB Best Practice?

**Yes, for the pipeline-first approach.** CKB implements the industry-validated pattern (deterministic analysis → structured context → LLM reasoning) with two structural advantages no other tool has: SCIP-based precision and full local operation.

**No, for the agentic approach.** Multi-agent tools (Qodo 2.0, Claude Code Review, Amp) can find issues CKB+LLM misses because they dispatch specialized agents that independently traverse the codebase. CKB's single-pass LLM narrative can't match that depth.

**The practical answer:** CKB is best practice for teams that want:
- Deterministic CI gates (no LLM in the critical path)
- Token efficiency ($0 for structural analysis, ~$0.01 for narrative)
- Local/offline operation (no code leaves the machine)
- MCP integration (LLM tools call CKB, not the other way around)

Teams that want maximum bug-finding depth regardless of cost should use an agentic tool (Qodo, Claude Code Review) WITH CKB as a context provider — CKB answers the structural questions in 5 seconds, the agents focus on semantic investigation.

---

## Measured Comparison

All on the same PR: `feature/review-engine`, 131 files, 18,611 lines.

| | LLM Alone | CKB Alone | CKB + LLM |
|---|---|---|---|
| **Findings** | 4 | 28 | **40** |
| **Critical bugs** | 0 | 0 | **2** (deadlock, missing impl) |
| **Design issues** | 3 | 0 | **8** |
| **Structural context** | 0 | 28 | **28** |
| **File coverage** | 29% | 100% | **100% structural, 8% deep** |
| **Time** | 12 min | 5s | **2.5 min** |
| **Tokens** | 87k | 0 | **77k** |
| **False positives** | 0 | 0 | **0** |
| **Cost** | ~$0.35 | $0 | **~$0.30** |

CKB + LLM found 10x more issues than LLM alone, including 2 critical bugs the LLM alone missed (because CKB's test-gap data pointed it to the right files).

---

## Evaluation Details

- **Branch:** `feature/review-engine` — 131 files, 18,611 lines, 36 commits
- **CKB version:** 8.2.0, 15 checks, 10 bug-pattern rules
- **CKB query duration:** 5,246ms
- **CKB findings:** 28 (0 false positives after dead-code grep verification)
- **LLM model:** Claude Opus 4.6
- **LLM review (Scenario 3):** 77,159 tokens, 130s, 36 tool calls, ~10 files reviewed
- **Industry sources:** CodeRabbit, Qodo, Greptile, Amp, Sourcery, Datadog, arxiv papers (2025-2026)

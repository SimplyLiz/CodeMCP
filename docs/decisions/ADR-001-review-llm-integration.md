# ADR-001: Review Engine LLM Integration Architecture

**Status:** accepted

**Date:** 2026-03-21

**Author:** lisa

## Context

CKB's review engine runs 15 deterministic checks (secrets, breaking changes, dead code, complexity, health, coupling, hotspots, risk, test gaps, blast radius, bug patterns, etc.) in ~5 seconds with zero API cost. The question is how to integrate LLM-based review to add semantic understanding (design bugs, security reasoning, edge cases) that deterministic checks can't detect.

Industry approaches diverge into two camps:
- **Pipeline-first** (CodeRabbit): static analysis runs, results curate what the LLM sees. LLM never fetches its own context.
- **Agentic** (Qodo 2.0, Claude Code Review, Amp): multiple LLM agents independently traverse the codebase, each hunting different risk types. Higher depth, higher cost.

Measured on a 131-file PR: LLM alone found 4 issues (12 min, 87k tokens, 29% file coverage). CKB + LLM found 40 issues (2.5 min, 77k tokens, 100% structural + 8% deep coverage), including 2 critical bugs the LLM alone missed because CKB's test-gap data pointed it to the right files.

## Decision

CKB follows the **pipeline-first** approach with three integration layers:

### 1. Self-enrichment (0 tokens)

Before any LLM call, CKB verifies its own findings using its own query engine:
- Dead-code findings: `findReferences` to check if the symbol actually has references (catches cross-package refs SCIP misses)
- Blast-radius findings: detect `cmd/` package symbols as framework wiring
- Each enriched finding gets a `triage` field: `"confirmed"`, `"likely-fp"`, or `"verify"`

This eliminated the FormatSARIF false positive that previously poisoned the LLM's reasoning.

### 2. Multi-provider LLM narrative (`--llm` flag)

The `generateLLMNarrative` function sends enriched findings (not raw source) to the LLM:
- Input: ~1.5k tokens (verdict, score, top 15 enriched findings with triage, health summary)
- Output: ~500 tokens (prioritized narrative)
- Providers: auto-detects `GEMINI_API_KEY` or `ANTHROPIC_API_KEY`
- The LLM is instructed to respect triage fields and explain when findings are likely false positives

### 3. MCP tool suite for drill-down

CKB exposes `reviewPR` (with compact mode) plus 80+ tools (`findReferences`, `analyzeImpact`, `explainSymbol`, `explainFile`, `getCallGraph`, `traceUsage`) via MCP. The LLM can:
1. Call `reviewPR(compact: true)` → get ~1k tokens of structured context
2. Drill down on specific findings using CKB tools → 0 tokens per call
3. Only read source files for issues that survive drill-down

### 4. Feedback learning

A `DismissalStore` at `.ckb/review-dismissals.json` lets users dismiss specific findings by rule+file. Dismissed findings are filtered from all future reviews. This closes the "same noise every run" gap relative to Sourcery/Greptile.

### 5. Inline PR posting

`--post <PR>` flag generates markdown and posts via `gh pr comment`. Keeps the review pipeline local while delivering results to the PR platform.

## Consequences

- CKB review is fully functional without any LLM (deterministic CI gates)
- LLM integration is additive: narrative synthesis, not decision-making
- Token efficiency: ~1.5k tokens per `--llm` call vs ~445k for a full LLM review from source
- Self-enrichment reduces FP rate before the LLM sees findings, preventing FP amplification
- The `/review` and `/ckb-review` Claude Code skills orchestrate a token-optimized workflow: CKB structural scan → targeted source reading of flagged files only → terse review output
- Framework symbol filtering (variables, constants, CLI wiring) works across Go, C++, Java, Python via SCIP symbol kinds

## Affected Modules

- `internal/query/review.go` — orchestrator, HoldTheLine, dismissal filtering
- `internal/query/review_llm.go` — multi-provider LLM client, enrichment, triage
- `internal/query/review_dismissals.go` — feedback store
- `internal/query/review_bugpatterns.go` — 10 AST rules with differential analysis
- `internal/query/review_blastradius.go` — framework symbol filter
- `internal/query/review_deadcode.go` — grep verification for cross-package refs
- `internal/mcp/tool_impls_review.go` — compact MCP response mode
- `cmd/ckb/review.go` — `--llm`, `--post` flags
- `.claude/commands/review.md` — `/review` skill

## Alternatives Considered

- **Agentic approach** (multiple LLM agents per review): Higher depth potential but 10-50x more expensive, non-deterministic, and can't provide CI gates. Not suitable for CKB's "deterministic first, LLM optional" philosophy.
- **LLM-as-filter** (run static analysis, ask LLM to triage each finding): Evaluated from Datadog research (92% → 6.3% FP rate). We adopted a hybrid: deterministic enrichment (SCIP reference checks) handles the 80% case, triage field lets the LLM handle the remaining 20%.
- **Vector embeddings** (Greptile approach): Pre-index repo into embeddings for semantic search. SCIP provides more precise symbol-level queries; embeddings would add value for natural-language queries ("find functions related to auth") but not for the structured review pipeline.
- **No LLM integration**: Viable for CI gates but misses the 2 critical bugs found only by semantic review in our evaluation. The LLM's judgment on test-gap priorities directly led to finding the `followLogs()` deadlock.

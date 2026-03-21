# CKB Review: Benchmark Data

All numbers from real measurements on a production PR (133 files, 19,200 lines, 37 commits).

---

## Token Savings

### LLM Review Without CKB (Scenario 1)

| Metric | Value |
|---|---|
| Model | Claude Opus 4.6 |
| Files in PR | 133 |
| Files LLM reviewed | 37 (28%) |
| Tokens consumed | 87,336 |
| Tool calls (file reads, searches) | 71 |
| Duration | 718 seconds (12 minutes) |
| Findings | 4 |
| False positives | 0 |
| Tokens per finding | 21,834 |

The LLM spent 87k tokens and still only covered 28% of files. It couldn't check secrets, breaking changes, dead code, test coverage, complexity, coupling, or churn history.

### LLM Review With CKB (Scenario 3 — Final)

| Metric | Value |
|---|---|
| Model | Claude Opus 4.6 |
| CKB runtime | 5,246ms |
| CKB tokens | 0 |
| CKB findings | 31 |
| LLM files reviewed | ~10 (8%) |
| LLM tokens consumed | 45,784 |
| LLM tool calls | 47 |
| LLM duration | ~17 minutes |
| New LLM findings | 2 (verified CKB bug-patterns) |
| Total findings | 33 |
| False positives | 0 |
| Tokens per finding | 1,388 |

### Comparison

| Metric | Without CKB | With CKB | Improvement |
|---|---|---|---|
| Tokens | 87,336 | 45,784 | **-48%** |
| File coverage (structural) | 28% | 100% | **+72pp** |
| Findings | 4 | 33 | **8.3x** |
| Tokens per finding | 21,834 | 1,388 | **15.7x more efficient** |
| Secrets checked | No | Yes (all 133 files) | +133 files |
| Breaking changes checked | No | Yes (SCIP-verified) | Impossible without CKB |
| Test gaps identified | No | 16 functions | Impossible without CKB |

---

## CKB Standalone Performance

### Runtime

| PR Size | Files | Lines | CKB Duration | Checks |
|---|---|---|---|---|
| Small (measured) | 2 | 10 | ~500ms | 15 |
| Medium (estimated) | 30 | 2,000 | ~2s | 15 |
| Large (measured) | 133 | 19,200 | 5.2s | 15 |

All 15 checks run in parallel. The bottleneck is tree-sitter complexity analysis (~1.8s) and coupling analysis (~1.9s).

### Findings Quality Progression

Over 5 tuning iterations on the same PR:

| Iteration | Total Findings | Noise | False Positives | Score |
|---|---|---|---|---|
| Raw (no tuning) | 258 | 230 | 1 | 20 |
| + Infallible-write allowlist | 89 | 62 | 1 | 54 |
| + Threshold tuning | 27 | 8 | 1 | 63 |
| + Framework symbol filter | 19 | 1 | 1 | 71 |
| + Dead-code grep verification | 18 | 0 | 0 | 74 |
| **Final (with test-gap details)** | **31** | **0** | **0** | **61** |

Score dropped from 74 → 61 in the final version because test-gap findings (10 new findings with file:line details) were added to the output. These are informational findings, not quality regressions.

### Check Execution Times (133-file PR)

| Check | Duration | What it does |
|---|---|---|
| complexity | 1,799ms | Tree-sitter cyclomatic/cognitive analysis, before/after comparison |
| coupling | 1,772ms | Git co-change analysis across history |
| tests | 904ms | SCIP + heuristic test coverage mapping |
| health | 875ms | 8-factor weighted health score per file |
| bug-patterns | 871ms | 10 AST rules with differential base comparison |
| dead-code | 812ms | SCIP reference count + grep cross-verification |
| blast-radius | 701ms | SCIP caller graph traversal |
| secrets | 395ms | Pattern + entropy scanning |
| test-gaps | 149ms | Tree-sitter function extraction + test cross-ref |
| breaking | 39ms | SCIP API surface comparison |
| format-consistency | 12ms | Output format divergence check |
| comment-drift | 3ms | Numeric reference scanning |
| risk | <1ms | Composite score (pre-computed inputs) |
| split | <1ms | Module clustering (pre-computed) |
| hotspots | <1ms | Score lookup (pre-computed by coupling check) |

Total wall clock: 5.2s (parallel execution).

---

## MCP Response Sizes

| Mode | Response Size | Tokens (~4 chars/tok) | Use Case |
|---|---|---|---|
| Full JSON | 120 KB | ~30,000 | Raw data export, CI pipelines |
| Compact JSON | 4 KB | ~1,000 | LLM consumers (MCP tool calls) |
| Human text | 2 KB | ~500 | Terminal output |
| Markdown | 3 KB | ~750 | PR comments |

Compact mode strips to: verdict, non-pass checks, top 10 findings, health summary, split suggestion. The LLM gets exactly what it needs for decision-making without wasting context window.

---

## False Positive History

| Finding | Initial State | Fix | Final State |
|---|---|---|---|
| `FormatSARIF` flagged as dead code | SCIP missed cross-file reference in cmd/ckb | Added grep verification for same-package refs | Eliminated |
| 169 `discarded-error` on strings.Builder | Builder.Write never errors | Receiver-type tracking in AST | Eliminated |
| 10 `discarded-error` on hash.Hash | Hash.Write never errors | Added hash constructors to allowlist | Eliminated |
| 8 blast-radius on cobra Command vars | Framework registrations, not real callers | Framework symbol filter (skip variables/constants) | Eliminated |

Current false positive rate: **0%** (0 of 31 findings).

---

## Cost Comparison

Based on Claude Sonnet 4 pricing ($3/MTok input, $15/MTok output).

| Scenario | Input Tokens | Output Tokens | Cost | Findings |
|---|---|---|---|---|
| LLM reviews alone (small PR, 10 files) | ~20,000 | ~2,000 | $0.09 | ~2 |
| LLM reviews alone (large PR, 100 files) | ~200,000 | ~5,000 | $0.68 | ~4 |
| LLM reviews alone (huge PR, 600 files) | ~500,000 | ~10,000 | $1.65 | ~4 |
| CKB + LLM (small PR) | ~15,000 | ~2,000 | $0.08 | ~15 |
| CKB + LLM (large PR) | ~50,000 | ~3,000 | $0.20 | ~30 |
| CKB + LLM (huge PR) | ~80,000 | ~5,000 | $0.32 | ~30 |
| CKB alone (any size) | 0 | 0 | **$0.00** | 20-30 |

CKB's value scales with PR size. On a 10-file PR, savings are minimal (~10%). On a 600-file PR, savings are **80%** ($1.65 → $0.32).

---

## Environment

- **Hardware:** Apple Silicon (M-series), macOS
- **CKB version:** 8.2.0
- **Go version:** 1.26.1
- **SCIP indexer:** scip-go
- **LLM:** Claude Opus 4.6 (1M context)
- **MCP transport:** stdio

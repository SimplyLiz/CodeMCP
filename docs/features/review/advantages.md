# CKB Review: Three Scenarios on a Real PR

All numbers measured on the same PR: `feature/review-engine`, 128 files, 16,740 lines added.

---

## Results at a Glance

| | Scenario 1: LLM Alone | Scenario 2: CKB Alone | Scenario 3: CKB + LLM |
|---|---|---|---|
| **Total findings** | 4 | 19 | **24** (19 CKB + 5 new LLM) |
| **Files analyzed** | 37 / 128 (29%) | 127 / 127 (100%) | **127 CKB + 9 LLM deep** |
| **Time** | 12 min | 5.2 sec | **5.2s + 14 min** |
| **Tokens** | 87,336 | 0 | **105,537** |
| **Tool calls** | 71 | 0 | **49** (-31%) |
| **Secrets checked** | No | All 127 files | **Yes** |
| **Breaking changes** | No | SCIP-verified | **Yes** |
| **Dead code** | No | 1 found (SCIP) | **Yes** |
| **Test gaps** | No | 22 found | **Yes** |
| **Hotspot ranking** | No | 50 scored, top 10 shown | **Yes** |
| **Design/logic bugs** | 4 found | 0 | **5 found** |
| **CI-ready output** | No | SARIF, exit codes | **Yes** |

---

## How the Integration Works

CKB is an MCP server. The LLM doesn't run CKB and then separately do its own review — it calls CKB's `reviewPR` tool during its review and gets structured data back in its context window. One flow, not two sequential steps.

```
LLM starts reviewing PR
  │
  ├─ Calls CKB tool: reviewPR(baseBranch: "main", compact: true)
  │   ← 5 seconds, 0 tokens, ~1k tokens in response (compact mode)
  │   └─ Returns: verdict, score, 19 findings, health report,
  │      hotspot ranking, split suggestion, test gaps
  │
  ├─ LLM reads CKB output (in context)
  │   └─ Knows: secrets clean, no breaking changes, top 10 hotspots,
  │      22 test gaps, 1 dead-code item, 3 complex files
  │
  ├─ LLM drills down on specific findings via CKB tools (0 tokens each)
  │   └─ findReferences, analyzeImpact, explainSymbol, explainFile
  │
  ├─ LLM skips categories CKB answered
  │   └─ No need to: scan for secrets, diff APIs, count tests,
  │      compute complexity, check for AST bugs
  │
  └─ LLM focuses on semantic review of flagged files
      └─ Reads 9 files (guided by hotspot scores)
      └─ Finds: missing timeout, scope issues, design problems
```

The LLM calls CKB once, drills down on findings, then reviews the flagged files. It's not "CKB report + LLM report" — it's "LLM review, informed by CKB data."

---

## Scenario 1: LLM Reviews Alone

The LLM reads source code, reasons about it, finds issues on its own. No CKB, no pre-computed data.

**Measured:** 87,336 tokens, 718 seconds (12 min), 71 tool calls, 37 files read.

### What it found (4 findings)

| # | File | Severity | Finding |
|---|---|---|---|
| 1 | `review.go:1361` | Bug | Config merge logic — `DeadCodeMinConfidence` initialized to 0.8 in defaults, but merge checks `== 0`, so config file overrides are silently ignored |
| 2 | `handlers_review.go:20` | Design | `context.Background()` instead of request context — reviews can't be cancelled |
| 3 | `review_baseline.go:239` | Edge case | Fingerprint truncated to 64 bits — collision risk in baseline comparison |
| 4 | `handlers_review.go:71` | Robustness | `io.EOF` silently ignored in JSON decoder — malformed requests treated as empty |

### What it could NOT check

- 91 of 128 files not reviewed (71% uncovered)
- No git history analysis — couldn't detect coupling, churn, hotspots
- No SCIP index — couldn't verify dead code, breaking changes, blast radius
- No test coverage data — couldn't identify untested functions
- No secret scanning — didn't search for credentials

---

## Scenario 2: CKB Reviews Alone

CKB runs 15 parallel checks using git history, SCIP index, and tree-sitter. No LLM.

**Measured:** 0 tokens, 5,246ms, 127 files analyzed, 19 findings.

### What it found

| Check | Status | Findings | What |
|---|---|---|---|
| hotspots | info | 10 (top 10 of 50) | Files ranked by historical churn score |
| complexity | pass | 3 | Files with +5 or more cyclomatic delta |
| risk | warn | 4 | Composite risk factors |
| dead-code | warn | 1 | Unused `FormatSARIF` constant |
| coupling | warn | 1 | Missing co-change file |
| blast-radius | info | 0 | Framework symbols filtered (see below) |
| bug-patterns | warn | 0 output | 5 new AST bugs, filtered by HoldTheLine |
| test-gaps | info | — | 22 untested functions (check summary only) |
| split | warn | — | 28 independent clusters identified |
| health | pass | — | 0 degraded, 7 new files |
| tests | pass | — | 27 tests cover changes |
| secrets | pass | — | No credentials detected |
| breaking | pass | — | No API removals |
| comment-drift | pass | — | No stale references |
| format-consistency | pass | — | Formatters consistent |

### Framework symbol filtering

CKB's blast-radius check filters out framework registration patterns that create false "callers." This works across languages because SCIP provides symbol kinds uniformly:

| Symbol kind | Why filtered | Example |
|---|---|---|
| `variable` | References are reads/writes, not call fan-out | Go cobra `Command` vars, C++ Qt signal vars |
| `constant` | References are value lookups, not dependency chains | Go const blocks, C++ `constexpr` |
| `property`, `field` | Struct field access, not function calls | Java Spring `@Bean` fields |

Additionally, known framework function patterns are filtered:
- `init()` — Go init, C++ static initializers
- `register`, `configure`, `setup`, `teardown` — framework wiring across languages
- `*Cmd` in `cmd/` packages — CLI command registrations

This eliminated all 8 cobra variable findings from `daemon.go` that were noise in earlier iterations.

### What it could NOT find

The 2 real bugs the LLM found (config merge logic, missing timeout) — and any other issue requiring semantic understanding.

---

## Scenario 3: LLM Reviews with CKB as a Tool (Intended Use)

The LLM calls CKB's `reviewPR` MCP tool at the start of its review. CKB returns structured data in ~5 seconds. The LLM then drills down on specific findings using CKB's tools, and reviews flagged files.

**Measured:** CKB tool call 5.2s (0 tokens) + LLM review 105,537 tokens (849s / 14 min), 49 tool calls.

### What CKB told the LLM (saved work)

| CKB result | LLM action |
|---|---|
| `secrets: pass` | Skipped credential scanning of 127 files |
| `breaking: pass` | Skipped API surface comparison |
| `tests: 27 covering` | Skipped test coverage audit |
| `health: 0 degraded` | Skipped health regression analysis |
| `bug-patterns: 5 new` | Skipped AST bug hunting |
| `dead-code: FormatSARIF` | Knew exactly where to look |
| `hotspots: top 10 ranked` | Knew which files to prioritize |
| `coupling: 1 missing` | Checked `handlers_upload_delta.go` specifically |
| `blast-radius: 0` | No fan-out concerns — framework noise already filtered |

### What the LLM found (5 new findings beyond CKB)

| # | File | Severity | Finding |
|---|---|---|---|
| 1 | `handlers_review.go:20` | High | `context.Background()` — no timeout |
| 2 | `format.go:15` | Medium | `FormatSARIF` not handled in generic `FormatResponse` switch (but IS handled in review switch — **false positive**) |
| 3 | `review.go:659` | Low | Provenance object only populates 3 of 8 fields |
| 4 | `review_commentdrift.go:29` | Low | Hard cap at 20 files |
| 5 | `engine_helper.go:110` | Medium | CLI `newContext()` also has no timeout |

---

## Honest Assessment: What Actually Matters

### Findings that should be fixed: 2

Both found only by the LLM. CKB missed them entirely.

| # | Finding | Source | Why it matters |
|---|---|---|---|
| 1 | Config merge ignores `DeadCodeMinConfidence` override — default 0.8 makes `== 0` check unreachable | LLM-alone | Users will report this when config doesn't work |
| 2 | API handler uses `context.Background()` — no timeout, reviews can hang indefinitely | LLM-alone + CKB+LLM | Will cause hung CI jobs on large repos |

### Findings that are good to know: 5

| # | Finding | Source |
|---|---|---|
| 3 | CLI `newContext()` also has no timeout | CKB+LLM |
| 4 | Baseline fingerprint truncated to 64 bits | LLM-alone |
| 5 | Comment-drift check silently caps at 20 files | CKB+LLM |
| 6 | Provenance object only populates 3 of 8 fields | CKB+LLM |
| 7 | JSON decoder silently ignores EOF on malformed requests | LLM-alone |

### Useful structural context from CKB: 19 findings

- Top 10 hotspot files ranked by churn score (review prioritization)
- 3 files with significant complexity increase (+6, +11, +13 cyclomatic)
- 1 coupling gap (co-change pattern)
- 1 dead-code item
- 4 risk factors (PR size/shape)
- 0 blast-radius (framework symbols correctly filtered)

### False positives: 2

| Source | Finding | What went wrong |
|---|---|---|
| CKB | `FormatSARIF` flagged as dead code | SCIP didn't capture the cross-file reference in `cmd/ckb/review.go:235` |
| CKB+LLM | LLM concluded `FormatSARIF` isn't handled in any switch | LLM trusted CKB's false positive and only checked one switch, not both |

**CKB false positives can seed LLM false positives.** The LLM saw "CKB says it's dead code" and stopped verifying. The self-enrichment in `--llm` mode partially mitigates this — CKB's `findReferences` call detects the reference and marks it as "likely false positive" in the narrative.

### The real comparison

| | LLM alone | CKB alone | CKB + LLM |
|---|---|---|---|
| **Real bugs found** | 1 (config merge) | 0 | 0* |
| **Design issues found** | 3 | 0 | 4 |
| **Useful structural context** | 0 | 19 | 19 |
| **File coverage** | 29% | 100% | 100% structural, 7% deep |
| **False positives** | 0 | 1 | 1 (inherited + amplified) |
| **Noise findings** | 0 | 0 | 0 |

*Scenario 3 missed the config merge bug that Scenario 1 found — LLM review is non-deterministic. CKB context steered Scenario 3 toward different files.

---

## Where CKB Actually Adds Value

CKB's value is NOT in finding bugs. It found zero real bugs across all runs. Its value is in three things:

### 1. Answering questions the LLM can't

The LLM cannot compute these without tool access:

| Question | CKB answer | LLM alone |
|---|---|---|
| Any secrets in 127 files? | No (scanned all, 395ms) | Can't check |
| Any breaking API changes? | No (SCIP comparison, 39ms) | Can't check |
| Which files have highest churn? | Top 10 ranked with scores | Can't compute |
| How many tests cover the changes? | 27 tests | Can't count |
| Which functions lack tests? | 22 identified | Can't cross-reference |
| What's the complexity delta? | +59 total, 3 files significant | Can't parse |
| Should this PR be split? | Yes, 28 clusters | Can't analyze module boundaries |
| Who should review? | 2 reviewers with coverage % | Can't query CODEOWNERS + blame |

### 2. Telling the LLM where NOT to look

CKB's clean checks save the LLM from wasting tokens on mechanical verification:

- `secrets: pass` → skip reading 127 files for credential patterns
- `breaking: pass` → skip diffing public API surface
- `health: 0 degraded` → skip checking for quality regression
- `bug-patterns: 5 new (31 filtered)` → skip hunting for defer-in-loop, nil-after-deref, etc.
- `blast-radius: 0` → no fan-out concerns (framework wiring already filtered)

In Scenario 3, the LLM reviewed 9 files instead of 37 (76% fewer) because CKB eliminated categories of work.

### 3. CI gating (no LLM needed)

CKB provides deterministic, fast, token-free CI gates:

```bash
ckb review --base=main --ci
# Exit 0 = pass, 1 = fail, 2 = warn
```

Secrets detected? Fail the build. Breaking API change? Fail the build. No LLM needed, no tokens, 5 seconds.

---

## Where CKB Does NOT Add Value

Being honest:

- **CKB found zero real bugs.** Both bugs that should be fixed came from the LLM.
- **CKB's 1 false positive poisoned the LLM.** The dead-code FP on `FormatSARIF` led to a second FP.
- **CKB cannot replace LLM review for code quality.** It can only supplement it with structural data.

---

## Noise Reduction Journey

Over the course of this evaluation, CKB's output was iteratively tuned from 258 findings (mostly noise) to 19 findings (all useful):

| Change | Findings | Noise removed | Key technique |
|---|---|---|---|
| Initial v8.2 raw | 258 | — | discarded-error FP flood |
| + Builder/Buffer/Hash allowlist | 89 | 169 | Receiver-type tracking in AST |
| + Per-rule score cap | 89 | 0 | maxPerRule = 10 points |
| + Hotspot top-10 cap | 49 | 40 | Only show highest-churn files |
| + Complexity min delta +5 | 37 | 12 | Skip trivial +1/+2 increases |
| + Blast-radius min 3 callers | 29 | 8 | Skip normal 1-2 caller coupling |
| + Framework symbol filter | **19** | **10** | Skip variables/constants/CLI wiring |

The framework filter is the most general — it works across languages by using SCIP's uniform symbol kinds. Variables and constants aren't call targets regardless of whether you're writing Go, C++, Java, or Python.

---

## Token Efficiency

| | Scenario 1 | Scenario 3 | Difference |
|---|---|---|---|
| LLM tokens used | 87,336 | 105,537 | +21% |
| Files reviewed by LLM | 37 | 9 | **-76%** |
| Tool calls | 71 | 49 | **-31%** |
| Total findings (real + structural) | 4 | 24 | **+500%** |
| Tokens per finding | 21,834 | 4,397 | **5x more efficient** |

Scenario 3 used more total tokens but produced 6x more findings because the LLM didn't waste tokens on questions CKB already answered.

With compact mode (`reviewPR(compact: true)`), the CKB response is ~1k tokens instead of ~30k — a 30x reduction in context window usage.

---

## Evaluation Details

- **Branch:** `feature/review-engine` — 128 files changed, 16,740 insertions, 503 deletions
- **CKB version:** 8.2.0, 15 checks, 10 bug-pattern rules
- **CKB query duration:** 5,246ms (self-reported provenance)
- **CKB findings:** 19 (after all tuning: hotspot top-10, complexity min +5, framework symbol filter)
- **CKB score:** 71/100
- **LLM model:** Claude Opus 4.6 (1M context)
- **Scenario 1:** 87,336 tokens, 718s, 71 tool calls, 37 files reviewed
- **Scenario 3:** 105,537 tokens, 849s, 49 tool calls, 9 files reviewed (guided by CKB)
- **All scenarios run on same machine, same branch, same commit**

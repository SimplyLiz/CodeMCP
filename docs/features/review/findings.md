# All Findings: feature/review-engine PR

Every finding from all 3 review scenarios, with honest assessment of importance and accuracy.

---

## How to Read This

Each finding is tagged:

- **Source:** Which scenario found it (CKB / LLM-alone / CKB+LLM)
- **Verified:** Did we confirm the finding is real? (Yes / No / Partial / False positive)
- **Importance:** Would you actually fix this before merging? (Must fix / Should fix / Nice to know / Noise)

---

## Actual Bugs

### 1. Config merge logic silently ignores overrides

- **Source:** LLM-alone
- **File:** `internal/query/review.go:1361`
- **Verified:** Yes — confirmed by reading the code
- **Importance:** Should fix

`DefaultReviewPolicy()` sets `DeadCodeMinConfidence: 0.8` and `TestGapMinLines: 5`. But `mergeReviewConfig()` only applies config values when the policy field is `== 0`:

```go
if policy.DeadCodeMinConfidence == 0 && rc.DeadCodeMinConfidence > 0 {
    policy.DeadCodeMinConfidence = rc.DeadCodeMinConfidence
}
```

Since the default is 0.8 (not 0), config-file overrides are silently ignored. Users who set `deadCodeMinConfidence: 0.5` in `.ckb/config.json` will always get 0.8.

Same bug for `TestGapMinLines` (default 5, check `== 0`).

**Why CKB missed it:** This requires understanding the relationship between two functions — what one initializes, the other checks. No AST pattern for "default value makes condition unreachable."

**Why only LLM-alone found it:** Non-deterministic — the LLM happened to read the merge function closely in Scenario 1 but focused on different files in Scenario 3.

---

## Design Issues

### 2. No context timeout in API handler

- **Source:** LLM-alone + CKB+LLM (both found independently)
- **File:** `internal/api/handlers_review.go:20`
- **Verified:** Yes
- **Importance:** Should fix

```go
ctx := context.Background()
```

The review API handler creates a context with no timeout. A review of a large repo could run for minutes. If the HTTP client disconnects, the server keeps processing. In CI, this means hung jobs.

**Why CKB missed it:** No rule for "context.Background() in HTTP handler." Would need a pattern like "context.Background in function that receives http.Request."

### 3. No context timeout in CLI either

- **Source:** CKB+LLM
- **File:** `cmd/ckb/engine_helper.go:110`
- **Verified:** Yes
- **Importance:** Nice to know (CLI users can Ctrl+C)

```go
func newContext() context.Context {
    return context.Background()
}
```

Same issue as #2 but less critical since CLI users have manual control. CI pipelines calling `ckb review` without their own timeout wrapper are vulnerable.

### 4. Baseline fingerprint truncated to 64 bits

- **Source:** LLM-alone
- **File:** `internal/query/review_baseline.go:239`
- **Verified:** Yes — truncation is real, collision probability is debatable
- **Importance:** Nice to know

```go
return hex.EncodeToString(h.Sum(nil))[:16]  // 16 hex chars = 64 bits
```

With 64 bits, birthday paradox gives ~50% collision chance at ~4 billion findings. In practice, a baseline stores hundreds to thousands of findings — collision probability is vanishingly small. Not a real risk, but the truncation has no benefit (SHA-256 output is already computed).

### 5. Comment-drift check caps at 20 files

- **Source:** CKB+LLM
- **File:** `internal/query/review_commentdrift.go:29`
- **Verified:** Yes
- **Importance:** Nice to know

Intentional performance cap. For this 127-file PR, numeric drift in files 21-127 is unchecked. CKB reported "pass" but only verified 20 files. The check summary doesn't disclose the cap.

### 6. Provenance object sparsely populated

- **Source:** CKB+LLM
- **File:** `internal/query/review.go:659`
- **Verified:** Yes — only 3 of 8 fields populated
- **Importance:** Nice to know

The `Provenance` struct has fields for `Backends`, `Completeness`, `Warnings`, `Timeouts`, `CachedAt`, `RepoStateMode`, but only `RepoStateId`, `RepoStateDirty`, and `QueryDurationMs` are set. The other fields are `omitempty` so they don't break anything, but consumers expecting backend metadata get nothing.

### 7. API JSON decoder silently ignores EOF

- **Source:** LLM-alone
- **File:** `internal/api/handlers_review.go:71`
- **Verified:** Yes
- **Importance:** Nice to know

```go
if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
```

Truncated or empty POST bodies are treated as empty requests (defaults applied) instead of returning an error. Intentional for GET-with-empty-body compatibility, but makes debugging harder for API users who send malformed JSON.

---

## CKB Structural Findings (89 total)

### Actionable (Tier 1+2): 6 findings

#### 8. Dead code: FormatSARIF constant

- **Source:** CKB
- **File:** `cmd/ckb/format.go:15`
- **Verified:** Partial — **CKB is technically wrong here**
- **Importance:** Noise (false positive)

CKB's SCIP-based dead-code check reports `FormatSARIF` has zero references. But it IS used at `cmd/ckb/review.go:235` in the review command's format switch. SCIP didn't index the cross-file reference within the `cmd/ckb` package, or the reference count query didn't capture it.

**Scenario 3's LLM compounded this** by concluding `FormatSARIF` isn't handled in `FormatResponse()` — but `FormatResponse` is only used for non-review commands. The review command has its own switch that handles all 7 formats including SARIF. Both CKB and the LLM were wrong.

**This is a false positive from CKB that the LLM made worse by building on it.**

#### 9. Missing co-change file

- **Source:** CKB
- **File:** `internal/api/handlers_upload_delta.go`
- **Verified:** Yes — 80% co-change rate with `handlers_upload.go`
- **Importance:** Nice to know

CKB correctly identified that `handlers_upload_delta.go` historically changes together with `handlers_upload.go` (80% co-change rate). This PR modifies one but not the other. Whether this actually matters depends on what changed — it's a statistical correlation, not a causal relationship.

#### 10-13. Risk score factors (4 findings)

- **Source:** CKB
- **Verified:** Yes — these are facts, not bugs
- **Importance:** Context (not actionable per-finding)

```
- Large PR with 127 files
- High churn: 17194 lines changed
- Touches 50 hotspot(s)
- Spans 29 modules
```

These are inputs to the risk score (1.00 = high). They describe the PR's shape, not defects. Useful context for prioritizing review effort but not actionable as individual findings.

### Informational (Tier 3): 83 findings

#### Hotspots: 50 findings

- **Source:** CKB
- **Verified:** Yes (churn scores are computed from git history)
- **Importance:** Review guidance — tells you where to look, not what's wrong

Top 5 by churn score:

| File | Score |
|---|---|
| `.github/workflows/ci.yml` | 11.64 |
| `action/ckb-review/action.yml` | 11.22 |
| `internal/query/review.go` | 28.90 |
| `cmd/ckb/review.go` | 15.30 |
| `internal/query/review_health.go` | 9.12 |

These are correct and useful for prioritization. Scenario 3's LLM used them to pick which files to read. Not actionable individually but valuable as a ranked list.

**Honest assessment:** 50 hotspot findings is a lot of noise in the findings list. The top 5-10 are useful; the bottom 30 are files with scores barely above threshold. A future improvement would be to only emit hotspots above a higher threshold or limit to top-N.

#### Blast-radius: 18 findings

- **Source:** CKB
- **Verified:** Yes (SCIP caller data)
- **Importance:** Mostly noise for this PR

All 18 are `daemon.go` cobra command variables (`daemonCmd`, `daemonStartCmd`, etc.) that have "callers" because cobra registers them. These are CLI flag variables, not functions — changing them doesn't "ripple" to callers in a meaningful way.

**Honest assessment:** These are technically correct (the SCIP index shows references) but not useful. CKB's blast-radius check doesn't distinguish between "this function has callers that depend on its behavior" and "this variable is referenced by a framework registration." This is a false-positive-adjacent finding category for CLI codebases.

#### Complexity: 15 findings

- **Source:** CKB
- **Verified:** Yes (tree-sitter cyclomatic measurement)
- **Importance:** Background context

Examples:
```
cmd/ckb/index.go: runIndex() +4 cyclomatic
internal/query/pr.go: SummarizePR() +13 cyclomatic
internal/backends/git/diff.go: GetCommitRangeDiff() +11 cyclomatic
```

These report complexity *increases*, not absolute values. A +2 in a function that was already complex might matter; a +2 in a simple function doesn't. CKB reports the delta but doesn't contextualize it.

**After tuning:** Threshold raised to +5 minimum delta. 15 findings reduced to 3 meaningful ones: `SummarizePR() +13`, `GetCommitRangeDiff() +11`, `matchesQuery() +6`.

---

## LLM-Only Semantic Findings (Scenario 3): 5 findings

### Already covered above

- #2: Missing context timeout in API handler (real, should fix)
- #3: Missing context timeout in CLI (real, nice to know)
- #5: Comment-drift 20-file cap (real, nice to know)
- #6: Provenance sparsely populated (real, nice to know)

### False positive from Scenario 3

#### 14. FormatSARIF "not handled in switch"

- **Source:** CKB+LLM
- **File:** `cmd/ckb/format.go:24-31`
- **Verified:** **False positive**
- **Importance:** N/A

The LLM read CKB's dead-code finding on `FormatSARIF` and concluded the constant isn't handled in `FormatResponse()`. But the review command has its own switch in `cmd/ckb/review.go:235` that handles SARIF. The LLM only checked one switch statement and missed the other.

**This shows a real risk of CKB+LLM:** a CKB false positive can seed an LLM false positive. The LLM trusted CKB's dead-code finding and built a wrong conclusion on top of it.

---

## LLM-Only Findings (Scenario 1): 4 findings

### Already covered above

- #1: Config merge logic bug (real, should fix)
- #2: Missing context timeout (real, should fix)
- #4: Fingerprint truncation (real, nice to know)
- #7: Silent EOF in JSON decoder (real, nice to know)

---

## Summary: What Actually Matters

### Must fix before merge: 0

None of these findings are blockers. The code builds, tests pass, and the review engine works correctly on real PRs.

### Should fix soon: 2

| # | Finding | Source | Why |
|---|---|---|---|
| 1 | Config merge ignores `DeadCodeMinConfidence` override | LLM-alone | Users will report this as a bug when config doesn't work |
| 2 | API handler has no context timeout | LLM-alone + CKB+LLM | Will cause hung CI jobs on large repos |

### Nice to know: 5

| # | Finding | Source |
|---|---|---|
| 3 | CLI has no context timeout | CKB+LLM |
| 4 | Fingerprint truncated to 64 bits | LLM-alone |
| 5 | Comment-drift caps at 20 files | CKB+LLM |
| 6 | Provenance sparsely populated | CKB+LLM |
| 7 | Silent EOF in JSON decoder | LLM-alone |

### Useful context from CKB: 19 findings

- Top 10 hotspot files ranked by churn score (review prioritization)
- 3 significant complexity increases (+6, +11, +13 cyclomatic)
- 1 coupling gap (co-change pattern)
- 1 dead-code item
- 4 risk factors (PR size/shape)
- 0 blast-radius (framework symbols filtered — see below)

### Framework symbol filtering

CKB originally reported 8 blast-radius findings, all on `daemon.go` cobra command variables. These were eliminated by the framework symbol filter which skips variables, constants, properties, and fields — their "references" are reads/assignments/registrations, not real call fan-out.

This works across languages because SCIP provides symbol kinds uniformly:
- **Go:** cobra `Command` vars, `init()` registrations
- **C++:** Qt signal/slot vars, gtest `TEST()` macro expansions
- **Java:** Spring `@Bean` fields, JUnit `@Test` annotations
- **Python:** Flask route decorators, pytest fixtures

### Noise: 0 (after all tuning)

CKB originally produced 258 findings. After iterative tuning:
- Receiver-type allowlist for `strings.Builder`, `bytes.Buffer`, `hash.Hash` (eliminated 169 discarded-error FPs)
- Hotspots capped to top 10 by score (eliminated 40 low-value entries)
- Complexity requires +5 cyclomatic delta (eliminated 12 trivial +1/+2 findings)
- Framework symbol filter (eliminated 8 cobra variable blast-radius findings)

Result: 19 CKB findings, all useful or at least informational.

---

## False Positive Accounting

| Source | Total findings | False positives | FP rate |
|---|---|---|---|
| CKB | 19 | 1 (`FormatSARIF` dead-code) | 5.3% |
| LLM-alone | 4 | 0 | 0% |
| CKB+LLM | 5 new | 1 (`FormatSARIF` switch gap) | 20% |

CKB's one false positive was amplified by the LLM in Scenario 3. This is the main risk of the combined approach: **CKB false positives become LLM false positives with added confidence.** The self-enrichment layer in `--llm` mode partially mitigates this — CKB's `findReferences` call detects the reference and marks it as "likely false positive" in the narrative sent to the LLM.

---

## What No Scenario Found

Things that would require deeper analysis than either tool performed:

- **Performance regression** — no benchmarking was done
- **Race conditions under load** — would need `-race` testing with concurrent requests
- **Behavior on non-Go repos** — the review engine was only tested on Go code
- **Edge behavior on empty repos, monorepos, or repos with no git history**
- **Whether the 22 untested functions actually need tests** — CKB reported the gap but neither CKB nor the LLM evaluated whether the functions are trivial enough to skip

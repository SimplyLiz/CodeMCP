# Spec: Performance Scanning

**Status:** DRAFT
**Author:** Claude + Lisa
**Created:** 2026-04-13
**Target Version:** v9.x

---

## Summary

Extend `ckb perf` and `reviewPR` with static detection of common runtime performance
anti-patterns: N+1 query loops, resource leaks (goroutine, file, connection), unbounded
memory growth, large value copies, and lock scope violations. All checks are tree-sitter
based — no runtime profiling required. They live alongside the existing `structural.go`
loop-call scanner and share the same `internal/perf` package and CGO build tag.

The intent is not to replace pprof. It's to catch the structural patterns that cause
performance bugs before they reach production — the same way `checkSecrets` catches
credentials before they're committed.

---

## Background

### What already exists

`internal/perf/structural.go` runs tree-sitter over hot files (≥3 commits in 90 days)
and reports call expressions found directly inside loop bodies. It annotates each
finding with churn count, entrypoint proximity, and severity. The `ckb perf structural`
CLI command surfaces these results; `ckb perf coupling` runs a separate co-change
analysis. Both are wired but not yet integrated into `reviewPR`.

### What's missing

The existing scanner has one pattern: **calls inside loops**. That catches O(n) cost
concentration but misses the other categories that routinely cause production incidents:

| Gap | Typical bug |
|-----|-------------|
| N+1 queries | DB/HTTP call inside ORM iteration |
| Resource leaks | goroutine started, never joined; file opened, defer missing |
| Unbounded growth | `append` / map insert inside loop, no size bound or eviction |
| Large copies | 100-field struct passed by value to hot function |
| Lock scope | mutex locked, then a slow call made before unlock |
| Missing error drain | channel send in goroutine, receiver never reads → goroutine park |

---

## Checkers

Each checker is a self-contained function in `internal/perf/` with a signature
matching the existing pattern:

```go
func findXxx(root *sitter.Node, source []byte, lang Language, file string,
             churnCount int, nearEP bool, functions []ComplexityResult) []PerfFinding
```

`PerfFinding` is the unified result type (see Data Model below). Each checker is
independently toggled via `PerfScanOptions.Checks []string` so users can run a
subset.

---

### Checker 1 — N+1 Queries (`n1-queries`)

**Pattern:** a call whose name matches a known I/O vocabulary (`db`, `query`, `exec`,
`find`, `fetch`, `get`, `http`, `request`, `send`, `rpc`) appears directly inside a
loop body. This is a superset of what `structural.go` already detects; the difference
is that N+1 detection applies a vocabulary filter so it reports separately from generic
loop calls.

**Implementation:**

1. Reuse `findLoopCallSites` from `structural.go`.
2. After collecting call sites, filter by receiver/function name against the I/O
   vocabulary list (case-insensitive prefix/suffix match).
3. Classify: if the call receiver is a struct field named `db`, `conn`, `client`,
   `store`, `repo` — severity escalates to `high` regardless of churn.

**Vocabulary list** (initial, extend via config):
```
db query exec find fetch get scan insert update delete
http request send post put patch do
rpc call invoke
redis get set hget hset
cache lookup
```

**False-positive guard:** calls inside a function whose name contains `batch`, `bulk`,
`pipeline`, or `transaction` are downgraded to `info`.

**Languages:** Go, TypeScript/JavaScript, Python, Java, Kotlin, Rust.

---

### Checker 2 — Resource Leaks (`resource-leaks`)

Detects resources that are opened but not paired with a close/cleanup at the same
scope level.

**Sub-patterns:**

#### 2a. Goroutine leak (Go only)

`go func(...)` or `go <ident>(...)` inside a loop body with no `wg.Add`/`wg.Wait`
or `errgroup` usage in the enclosing function. Heuristic: if the function body
contains neither `WaitGroup` nor `errgroup` nor a `done` channel receive, every
loop iteration leaks a goroutine.

**Tree-sitter query:** `go_statement` nodes inside loop bodies, filter out functions
that also contain `sync.WaitGroup` or `golang.org/x/sync/errgroup` references.

#### 2b. File/connection not closed (Go, Python, Java)

`os.Open`, `os.Create`, `sql.Open`, `net.Dial`, `http.Get` (and language equivalents)
without a `defer <var>.Close()` or explicit `Close()` call on the same variable in the
same scope. Match by: find the assignment target, check for a `defer` statement whose
call receiver is the same identifier anywhere in the enclosing function body.

**Implementation:**
1. Find all `call_expression` nodes whose function is in the open-vocabulary list.
2. Extract the assignment target identifier.
3. Walk the enclosing function body for `defer_statement` / `call_expression` on that
   identifier with suffix `Close`, `Done`, `Cancel`, `Release`, `Free`.
4. If none found → finding.

**Open vocabulary:** `os.Open os.Create os.OpenFile sql.Open db.Begin net.Dial
net.DialTCP http.Get http.Post bufio.NewReader bufio.NewWriter`

#### 2c. Context leak

`context.WithCancel`, `context.WithTimeout`, `context.WithDeadline` without a
corresponding `defer cancel()`. Same pairing approach as 2b.

---

### Checker 3 — Unbounded Growth (`unbounded-growth`)

**Pattern:** `append(slice, ...)` or `map[k] = v` inside a loop body where the slice
or map is declared outside the loop and has no capacity hint or eviction.

**Implementation:**

1. Find all loop bodies.
2. Inside each loop, find `append` calls or map index assignments (`index_expression`
   on the left of `=`).
3. Walk outward to find where the target variable was declared.
4. If declared outside the loop and the declaration has no capacity argument (for
   slices: `make([]T, 0, N)`) → finding.
5. Severity: `high` if the loop is in an entrypoint, `medium` otherwise.

**Languages:** Go (primary). TypeScript/Python: `arr.push(x)` inside a loop where
`arr` is declared outside.

**False-positive guard:** if the containing function name ends in `Batch`, `Buffer`,
`Accumulate`, `Collect`, or `Build` → downgrade to `info` (intentional accumulation).

---

### Checker 4 — Large Value Copies (`large-copies`)

**Pattern:** a function parameter or return value has a struct type whose field count
exceeds a configurable threshold (default: 8 fields), passed by value rather than
pointer.

**Implementation:**

This checker operates at the file level, not inside loops, because copy cost is
proportional to call frequency, not loop nesting.

1. Build a struct-size map: for each `type_declaration` / `struct_type` node in the
   file, count fields.
2. For each `function_declaration` / `method_declaration`, inspect parameter types and
   return types.
3. If a parameter or return type refers to a struct with ≥ threshold fields and is not
   a pointer (`*T`) → finding at the parameter site.
4. Only report for hot files (same churn filter as structural.go) to avoid noise on
   rarely-called code.

**Threshold:** `LargeCopyThreshold int` in `PerfScanOptions`, default 8.

**Languages:** Go (field counting is straightforward with tree-sitter). Rust: `struct`
fields. Java/Kotlin: deferred (generics complicate field counting).

---

### Checker 5 — Lock Scope Violations (`lock-scope`)

**Pattern:** a mutex or RWMutex is locked, and before the corresponding `Unlock` (or
`defer Unlock`) there is a call that is likely slow: I/O call, channel send/receive,
`time.Sleep`, or another lock acquisition.

**Implementation (Go-focused):**

1. Find all `Lock()` / `RLock()` call sites on any receiver.
2. Find the matching `Unlock()` / `RUnlock()` / `defer Unlock` in the same function.
3. Collect all call sites between Lock and Unlock in textual order (a simplification —
   no CFG analysis).
4. If any of those calls are in the slow-call vocabulary → finding.

**Slow-call vocabulary:** I/O vocabulary from Checker 1, plus `time.Sleep`,
`time.After`, channel `<-` operations, `http.*`, `os.*`, `net.*`.

**Severity:** always `high` — a slow path under a held lock is a latency cliff.

**Languages:** Go. Java (`synchronized` blocks with slow calls inside): deferred.

---

### Checker 6 — Missing Error Drain (`error-drain`, Go only)

**Pattern:** a goroutine sends on a channel that has no corresponding receive in the
calling scope, which will park the goroutine if the buffer fills (or if the channel is
unbuffered).

**Implementation:**

1. Find all goroutine launches (`go_statement` nodes).
2. Inside the goroutine body, find channel send expressions (`send_statement`).
3. Find the channel variable's declaration to determine buffer capacity: `make(chan T)`
   (0) vs `make(chan T, N)`.
4. Check whether the enclosing function contains a receive expression `<-<ident>` on
   the same channel variable.
5. If unbuffered send with no receive → `high`. If buffered with no receive → `medium`.

---

## Data Model

```go
// PerfFinding is the unified result type for all performance checkers.
// It replaces LoopCallSite for new checkers; LoopCallSite remains for
// backward compatibility with the existing structural scanner output.
type PerfFinding struct {
    Checker     string // "n1-queries", "resource-leaks", "unbounded-growth", etc.
    SubType     string // "goroutine-leak", "file-not-closed", etc. (checker-specific)
    File        string
    Line        int
    EndLine     int    // 0 if single-line
    FunctionName string
    Snippet     string // up to 120 chars of the offending expression
    Severity    string // "high" | "medium" | "low" | "info"
    ChurnCount  int
    NearEntrypoint bool
    Explanation string
    // For resource-leak checkers: the variable that was leaked.
    LeakedVar   string
}

// PerfScanOptions controls which checkers run and their parameters.
type PerfScanOptions struct {
    // Checks is the list of checker IDs to run. Empty = all.
    Checks []string

    // Existing structural options (unchanged):
    Scope         string
    WindowDays    int
    MinChurnCount int
    Limit         int
    EntrypointFiles []string

    // Checker-specific thresholds:
    LargeCopyThreshold int    // Checker 4, default 8
    IOVocabulary       []string // extend the default I/O word list
}
```

The existing `StructuralPerfResult` is extended:

```go
type StructuralPerfResult struct {
    LoopCallSites []LoopCallSite  // existing — unchanged
    Findings      []PerfFinding   // new — all other checkers
    Summary       StructuralPerfSummary
}
```

---

## CLI

### New flags on `ckb perf structural`

```
--checks=n1-queries,resource-leaks,unbounded-growth,large-copies,lock-scope,error-drain
    Comma-separated list of checkers to run. Default: all.

--large-copy-threshold=8
    Minimum struct field count to flag as a large copy.

--io-vocab=redis,cassandra
    Comma-separated additional terms to append to the I/O vocabulary.
```

### Output format

Findings are interleaved with existing `LoopCallSite` output, grouped by severity:

```
[HIGH]  internal/api/handler.go:142  lock-scope
  Handler.ServeHTTP holds mu.Lock() across an HTTP call to db.Query()
  → release lock before I/O or use a copy of the data under lock

[HIGH]  internal/store/cache.go:89   n1-queries
  populateCache: db.Find() called inside for/range loop (38 commits)
  → batch with FindByIDs or move query outside loop

[MEDIUM] internal/worker/pool.go:56  resource-leaks / goroutine-leak
  startWorker: goroutine launched inside loop with no WaitGroup (12 commits)
  → add wg.Add(1)/wg.Done() or use errgroup

[MEDIUM] internal/index/builder.go:201  unbounded-growth
  Build: append(results, ...) inside for loop, slice declared without capacity
  → pre-allocate with make([]T, 0, estimatedSize)
```

---

## Integration with `reviewPR`

Add a `"perf"` check to the `reviewPR` check suite, running at the same tier as
`"complexity"` (Tier 2 — important, non-blocking by default).

```go
// In review.go, alongside existing checks:
if checkEnabled("perf") {
    wg.Add(1)
    go func() {
        defer wg.Done()
        c, ff := e.checkPerformance(ctx, changedFiles)
        if c.Name != "" { addCheck(c); addFindings(ff) }
    }()
}
```

`checkPerformance` runs only checkers `n1-queries`, `resource-leaks`, and
`unbounded-growth` (the high signal-to-noise ones) on changed files only — not the
full hot-file window. `MinChurnCount` is overridden to 0 so all changed files are
scanned regardless of history.

Finding → ReviewFinding mapping:

| PerfFinding.Severity | ReviewFinding.Severity |
|---|---|
| high | error |
| medium | warning |
| low / info | info |

`large-copies` and `lock-scope` are excluded from `reviewPR` by default (too noisy
for PR context) but available via `--checks=perf:lock-scope`.

---

## MCP tool

Add `analyzePerf` as an MCP tool in the `refactor` preset:

```
analyzePerf(
  path: string,           // file or directory to scan
  checks?: string[],      // subset of checker IDs; default: all
  minChurn?: number,      // default 3
  windowDays?: number,    // default 90
  limit?: number,         // default 50
)
→ { findings: PerfFinding[], summary: { filesScanned, findingsByChecker, findingsBySeverity } }
```

---

## False-positive strategy

Static analysis without a type system is noisy. The goal is high precision (few false
positives) at the cost of recall (some real issues missed). Specific mitigations:

| Checker | Mitigation |
|---|---|
| N+1 | Vocabulary filter + batch-name suppression |
| Goroutine leak | Only flag loops; single goroutine launches are common and safe |
| File not closed | Require the open call to be in an assignment; bare `os.Open(...)` return ignored |
| Unbounded growth | Suppress when function name implies accumulation |
| Large copies | Hot-file filter; generated files excluded |
| Lock scope | Only I/O + sleep vocabulary; pure CPU calls ignored |

All checkers emit `info`-severity findings when confidence is low (e.g. the
batch-name suppression above still emits an `info` finding, just doesn't surface it
in `reviewPR`).

---

## Build

All new code lives under `//go:build cgo` alongside `structural.go`. No new
dependencies — everything uses the existing `go-tree-sitter` and `complexity`
packages.

A non-CGO stub returns empty results with a `"perf scanning requires CGO"` message,
matching the existing pattern in `structural_nocgo.go`.

---

## File layout

```
internal/perf/
  structural.go          existing — loop call sites (CGO)
  structural_nocgo.go    existing — stub
  checker_n1.go          new — N+1 query checker
  checker_leaks.go       new — resource leak checkers (goroutine, file, context)
  checker_growth.go      new — unbounded growth checker
  checker_copies.go      new — large value copy checker
  checker_locks.go       new — lock scope checker
  checker_drain.go       new — error drain checker
  findings.go            new — PerfFinding type + PerfScanOptions
  vocabulary.go          new — shared I/O vocabulary list
```

---

## Out of scope

- **pprof / runtime integration** — no profiling, no flamegraph parsing. That's a
  separate tool category.
- **Benchmark regression tracking** — tracking `go test -bench` output over time is
  valuable but belongs in a CI plugin, not static analysis.
- **SQL query analysis** — detecting inefficient query plans requires schema awareness.
  Detecting "query inside loop" (Checker 1) is sufficient at the static layer.
- **Race condition detection** — this requires a happens-before analysis that tree-sitter
  cannot provide. Use `go test -race`.
- **Cross-function analysis** — all checkers are intra-function. Inter-procedural
  dataflow (e.g. "this function always calls db.Query, so passing it into a loop is
  an N+1") requires a full call graph pass and is deferred.

---

## Open questions

1. **Lock-scope false positive rate** — the receiver-matching heuristic (find `Lock()`
   on any receiver) will fire on user-defined types that happen to have a `Lock` method.
   Consider restricting to receivers whose type name or field contains `Mutex`, `RWMutex`,
   `sync.Locker`. Needs a small eval suite before enabling in `reviewPR`.

2. **Python resource leaks** — Python's `with` statement is the idiomatic close-pairing.
   The checker should treat `with open(...) as f:` as "closed" and flag bare `open()`
   without a `with`. Worth speccing separately once Go is validated.

3. **TypeScript/Node N+1** — `await fetch(...)` or `await db.query(...)` inside a
   `for...of` or `forEach` is the same pattern. The vocabulary filter applies but
   `await` expressions need a separate node type (`await_expression` wrapping the call).
   Low lift to add.

4. **Severity tuning** — initial thresholds (entrypoint + churn ≥ 10 = high) are
   inherited from `structural.go`. Run against a sample of real repos before finalising.

5. **Config file** — should `io-vocab` and `large-copy-threshold` live in
   `.ckb/config.json` under a `perf` key? Avoids long CLI flags in CI scripts.

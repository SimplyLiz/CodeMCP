# Performance Log

Benchmark results over time. Run with `benchstat` for before/after comparisons.

```bash
go test -bench=. ./internal/compliance/... -benchmem -count=6 > before.txt
# make changes
go test -bench=. ./internal/compliance/... -benchmem -count=6 > after.txt
benchstat before.txt after.txt
```

---

## 2026-04-10 — `internal/perf` package: hidden-coupling scanner + structural perf analysis (Apple M4 Pro, arm64, -count=3)

Branch: `bench/compliance-scanner-baselines`

New package implementing two scan modes:
- **Hidden coupling** (`Scan`): git log → co-change pair counts → correlation filter → import-edge check
- **Structural perf** (`AnalyzeStructural`, CGO only): git churn → tree-sitter loop/call-site detection → severity ranking

### Optimization 1: lift seen-map out of `recordCommit`

`buildCoChangePairs` calls `recordCommit` once per commit. Originally each call allocated a fresh `make(map[string]bool)` for deduplication. Changed to allocate once before the loop and pass it in; `recordCommit` clears it with `for k := range seen { delete(seen, k) }`.

Effect on `CoChangePipelineSimulated` (the dominant CPU path):

| Scenario | allocs/op before | allocs/op after | Δ |
|---|---|---|---|
| 500 commits × 10 files | 1526 | 29 | **−98%** |
| 1k commits × 20 files | 3072 | 75 | **−97.6%** |
| 1k commits × 20 files (B/op) | 2,522,031 | 1,586,994 | **−37%** |
| 1k commits × 20 files (ns/op) | ~5.4 ms | ~4.8 ms | −11% |

The bulk of the pre-optimization allocs were 1× `make(map[string]bool)` per commit — invisible in profiling but compounding across thousands of commits in real repos.

### Optimization 2: `buildExplanation` — `fmt.Sprintf` → `strings.Builder`

`buildExplanation` is called once per loop call site in the structural scan. Replaced chained `fmt.Sprintf` with a pre-grown `strings.Builder` (`b.Grow(320)`) + `strconv.Quote` / `strconv.Itoa`.

| Variant | ns/op before | ns/op after | allocs/op before | allocs/op after |
|---|---|---|---|---|
| non-entrypoint | 352 | 208 | 6 | **3** |
| entrypoint | 350 | 188 | 7 | **3** |

~40–46% faster, allocs halved. The remaining 3 allocs are the `strings.Builder` internal buffer, `strconv.Quote`'s escape output, and the final `b.String()` copy — irreducible without a pre-allocated byte pool.

### Hot path baselines

#### `internal/perf` — co-change scanner

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `recordCommit/2files` | 79 | 0 | 0 |
| `recordCommit/10files` | 2,713 | 5,816 | 13 |
| `recordCommit/50files` | 73,730 | 197,000 | 34 |
| `recordCommit_Reuse/10files` | 1,027 | 0 | 0 |
| `CoChangePipeline/100c_5f` | 38,257 | 55,888 | 12 |
| `CoChangePipeline/500c_10f` | 628,257 | 406,802 | 29 |
| `CoChangePipeline/1kc_20f` | 4,793,762 | 1,586,994 | 75 |
| `importCouldReferTo/10imports` | 277 | 0 | 0 |
| `importCouldReferTo_Miss` | 513 | 0 | 0 |
| `shouldIgnore` | 1.8–7.1 | 0 | 0 |
| `correlationLevel` | 0.26 | 0 | 0 |
| `correlationFilter/~20k pairs` | ~372,000 | 0 | 0 |

#### `internal/perf` — structural (CGO)

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `computeSeverity` | 0.26 | 0 | 0 |
| `buildExplanation/non_ep` | 208 | 432 | 3 |
| `buildExplanation/entrypoint` | 188 | 416 | 3 |
| `findEnclosingFunction/10fns` | 3.0 | 0 | 0 |
| `findEnclosingFunction/50fns` | 14 | 0 | 0 |
| `CallSitePipeline/100sites` | 33,000 | 34,266 | 620 |
| `CallSitePipeline/500sites` | 160,000 | 171,334 | 3,100 |

### Known bottlenecks

- `recordCommit` is O(files²) per commit — formatting sweeps and mass renames produce commits with 100+ files, which hit ~75 µs/commit and ~197 KB/call. No fix yet; caller could skip commits above a file-count threshold.
- `correlationFilter` iterates all ~N²/2 pairs in memory — fine up to ~200 files but will need chunking or a threshold-based early prune for monorepos with thousands of hot files.
- `buildExplanation`'s 3 remaining allocs are irreducible without a `sync.Pool` on the `strings.Builder` buffer. Not worth it at current call volumes.

---

## 2026-04-09 — Compliance scanner baseline (Apple M4 Pro, arm64, -count=3)

Branch: `bench/compliance-scanner-baselines`

### Hot path functions

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `NormalizeIdentifier` | 137 | 138 | 4 |
| `NormalizeIdentifier_Long` | 673 | 1352 | 9 |
| `ExtractIdentifiers` | 555 | 219 | 6 |
| `ExtractContainer` | 517 | 24 | 0 |
| `IsNonPIIIdentifier` | 205 | 0 | 0 |
| `matchPII (mixed hit/miss)` | 739 | 0 | 0 |
| `matchPII (miss, full scan)` | 1267 | 0 | 0 |
| `NewPIIScanner` | 2421 | 13048 | 6 |
| `NewPIIScannerWithExtras` | 5853 | 20640 | 32 |

### Scanner pipeline (per-file, single file)

Scales linearly: ~14 allocs/line, ~3.7 µs/line.

| Lines | ns/op | MB/s | B/op | allocs/op |
|---|---|---|---|---|
| 500 | 1,854,122 | 8.23 | 209,857 | 6,989 |
| 5k | 18,692,730 | 8.19 | 2,098,509 | 69,845 |
| 50k | 185,942,685 | 8.23 | 20,857,361 | 698,378 |

### Audit file set (full repo scan simulation)

| Files (×300 lines) | Total lines | ns/op | MB/s | B/op | allocs/op |
|---|---|---|---|---|---|
| 100 | ~30k | 110,690,443 | 8.29 | 12,602,896 | 419,038 |
| 1k | ~300k | 1,114,492,597 | 8.24 | 126,979,875 | 4,190,378 |
| 5k | ~1.5M | 6,325,314,514 | 7.33 | 629,844,261 | 20,951,883 |

**Notable:** 5k-file run shows 24% variance across 3 runs (5.85s–7.25s) and MB/s drops from ~8.3 to ~6.3–7.8. GC pressure from 630 MB heap allocation. Root cause: `extractIdentifiers` allocates a map + slice per line — ~4.2M allocs for a 1.5M-line scan.

### Pattern scale (`matchPII`, miss path)

| Patterns | ns/op |
|---|---|
| ~80 (default) | 1,174 |
| 100 | 1,386 |
| 200 | 2,355 |
| 500 | 5,238 |

Linear degradation with custom pattern count. 80→500 patterns = ~4.5× slower on the miss path. Relevant for users with large custom PII configs on big repos.

### Known bottlenecks

- `extractIdentifiers`: allocates `map[string]bool` + `[]string` per source line. At 5k files × 300 lines, this is ~4.2M allocs per audit run. Pooling the map would be the highest-leverage fix.
- GC pressure at repo scale: 630 MB allocated for a 1.5M-line scan. MB/s degrades ~10% at this scale due to GC pauses.
- Custom PII patterns scale linearly on the miss path — no trie or bloom filter in front of the suffix scan.

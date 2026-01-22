# Feature Plan: Change Impact Analysis

**Status:** DRAFT
**Author:** Claude + Lisa
**Created:** 2025-12-26
**Target Version:** v8.0

---

## Executive Summary

Change Impact Analysis answers the questions developers ask before every commit:

1. **"What downstream code might break?"** — Blast radius analysis
2. **"Which tests should I run?"** — Targeted test selection
3. **"Who needs to review this?"** — Affected owners identification

This is something LLMs fundamentally cannot do without tooling — they can't reason about the full dependency graph, test mappings, or cross-file relationships at scale.

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Feature Design](#feature-design)
3. [Implementation Plan](#implementation-plan)
4. [Reusable Components](#reusable-components)
5. [New Development](#new-development)
6. [Testing Strategy](#testing-strategy)
7. [Open Questions](#open-questions)
8. [Appendix A: Research Decisions](#appendix-a-research-decisions)
9. [Appendix B: Industry Analysis](#appendix-b-industry-analysis)

---

## Problem Statement

### The Problem LLMs Can't Solve Alone

```
User: "I'm changing the User.Authenticate method. What might break?"

Without CKB:
- LLM reads a few files
- Guesses at callers based on grep-like search
- Misses indirect dependencies
- No test mapping
- No confidence scores

With CKB Change Impact Analysis:
- Precise caller graph (SCIP)
- Transitive impact propagation
- Test coverage mapping
- Risk scoring per affected symbol
- Owner identification for review routing
```

### Differentiation

| Competitor | What They Do | What They Miss |
|------------|--------------|----------------|
| GitHub's "Used by" | Direct references only | Transitive impact, tests |
| IDE "Find Usages" | Callers in current file/project | Cross-repo, risk scoring |
| Sourcegraph | References + ownership | Test mapping, impact scoring |
| **CKB** | Full graph + tests + risk + owners | — |

---

## Feature Design

### New MCP Tools

#### 1. `analyzeChange` — Primary Entry Point

Analyzes impact of code changes from various input sources.

```typescript
interface AnalyzeChangeInput {
  // What changed? (provide ONE of these)
  diff?: string;              // Raw diff content
  files?: string[];           // List of changed file paths
  symbols?: string[];         // Specific symbol IDs
  commit?: string;            // Git commit SHA
  commitRange?: string;       // "base..head" format

  // Analysis options
  depth?: number;             // Transitive depth (default: 2, max: 4)
  includeTests?: boolean;     // Map to affected tests (default: true)
  includeOwners?: boolean;    // Include CODEOWNERS (default: true)
  riskThreshold?: 'low' | 'medium' | 'high';  // Filter by risk
}

interface AnalyzeChangeOutput {
  summary: {
    filesChanged: number;
    symbolsChanged: number;
    directlyAffected: number;
    transitivelyAffected: number;
    testsToRun: number;
    estimatedRisk: 'low' | 'medium' | 'high' | 'critical';
  };

  changedSymbols: ChangedSymbol[];
  affectedSymbols: AffectedSymbol[];
  affectedTests: AffectedTest[];
  affectedOwners: AffectedOwner[];

  recommendations: Recommendation[];
}
```

#### 2. `getAffectedTests` — Test Selection

Focused tool for CI integration.

```typescript
interface GetAffectedTestsInput {
  files?: string[];
  symbols?: string[];
  commit?: string;
  commitRange?: string;

  strategy?: 'precise' | 'safe' | 'full';
  // precise: Only tests that directly cover changed code
  // safe: Include tests for transitive dependencies (default)
  // full: All tests in affected modules
}

interface GetAffectedTestsOutput {
  strategy: string;
  confidence: number;  // 0.0-1.0, based on coverage data quality

  tests: {
    name: string;
    path: string;
    reason: string;
    priority: number;
  }[];

  // Ready-to-run commands
  commands: {
    go?: string;      // "go test -run 'TestA|TestB' ./..."
  };

  warnings: {
    uncoveredSymbols: string[];
    staleData: boolean;
  };
}
```

#### 3. `getChangeOwners` — Review Routing

```typescript
interface GetChangeOwnersInput {
  files?: string[];
  symbols?: string[];
  commit?: string;
  includeSecondary?: boolean;  // Include transitive owners
}

interface GetChangeOwnersOutput {
  primary: Owner[];      // Direct owners of changed files
  secondary: Owner[];    // Owners of affected (not changed) files

  reviewStrategy: {
    minimum: Owner[];    // Must approve
    recommended: Owner[]; // Should approve
  };
}
```

### CLI Commands

#### `ckb impact`

```bash
# Analyze uncommitted changes
$ ckb impact

# Analyze specific commit
$ ckb impact --commit abc123

# Analyze commit range
$ ckb impact --range main..feature-branch

# Output formats
$ ckb impact --format=summary    # Default: human-readable
$ ckb impact --format=json       # Machine-readable

# Filter by risk
$ ckb impact --min-risk=medium   # Only show medium+ risk
```

#### `ckb affected-tests`

```bash
# Get tests for uncommitted changes
$ ckb affected-tests

# Output as runnable command
$ ckb affected-tests --output=command
# → go test -run 'TestAuth|TestLogin|TestSession' ./...

# Strategy options
$ ckb affected-tests --strategy=safe     # Default
$ ckb affected-tests --strategy=precise  # Only direct coverage
$ ckb affected-tests --strategy=full     # All tests in affected modules
```

#### `ckb reviewers`

```bash
# Get suggested reviewers for uncommitted changes
$ ckb reviewers

# Include affected (not just changed) owners
$ ckb reviewers --include-affected
```

---

## Implementation Plan

### Phase 1: Core Infrastructure (Week 1)

**Goal:** Git diff → changed symbols mapping

| Task | Files | LOC Est. |
|------|-------|----------|
| Add go-diff dependency | go.mod | 5 |
| Create diff parser | `internal/diff/gitdiff.go` | ~200 |
| Map diff lines → SCIP symbols | `internal/diff/symbolmap.go` | ~150 |
| Unit tests | `internal/diff/*_test.go` | ~200 |

**Dependencies:**
- `github.com/sourcegraph/go-diff` for unified diff parsing

**Deliverable:** `ParseGitDiff(diff string) → []ChangedSymbol`

### Phase 2: Multi-Symbol Impact Aggregation (Week 1-2)

**Goal:** Extend existing `AnalyzeImpact` for batch mode

| Task | Files | LOC Est. |
|------|-------|----------|
| Add `AnalyzeChangeSet()` method | `internal/query/impact.go` | ~150 |
| Dedupe affected symbols across results | `internal/query/impact.go` | ~50 |
| Aggregate blast radii | `internal/impact/analyzer.go` | ~80 |
| Wire to MCP tool | `internal/mcp/tool_impls.go` | ~100 |

**Reuses:**
- `impact.ImpactAnalyzer.Analyze()` (100%)
- `impact.ComputeRiskScore()` (100%)
- `scip.BuildCallGraph()` (100%)

**Deliverable:** `analyzeChange` MCP tool working with `symbols[]` input

### Phase 3: Diff Input Sources (Week 2)

**Goal:** Accept various diff input formats

| Task | Files | LOC Est. |
|------|-------|----------|
| Parse raw diff string | `internal/diff/gitdiff.go` | ~50 |
| Get diff from commit SHA | `internal/backends/git/diff.go` | ~80 |
| Get diff from commit range | `internal/backends/git/diff.go` | ~60 |
| CLI command `ckb impact` | `cmd/ckb/impact.go` | ~200 |

**Deliverable:** `ckb impact` CLI and `analyzeChange` with all input types

### Phase 4: Test Mapping - Fallback Strategies (Week 3)

**Goal:** Map symbols to tests without external coverage data

| Task | Files | LOC Est. |
|------|-------|----------|
| Naming convention mapper | `internal/testmap/naming.go` | ~80 |
| Import-based test discovery | `internal/testmap/imports.go` | ~120 |
| Package-level test grouping | `internal/testmap/packages.go` | ~60 |
| Test mapper interface | `internal/testmap/mapper.go` | ~50 |
| Unit tests | `internal/testmap/*_test.go` | ~200 |

**Deliverable:** Test selection without coverage files (80% accuracy target)

### Phase 5: Coverage File Parsing (Week 3-4)

**Goal:** Parse local coverage files for precise test mapping

| Task | Files | LOC Est. |
|------|-------|----------|
| Go coverage.out parser | `internal/testmap/coverage_go.go` | ~150 |
| LCov parser | `internal/testmap/coverage_lcov.go` | ~120 |
| Coverage → symbol mapping | `internal/testmap/coverage.go` | ~100 |
| `getAffectedTests` MCP tool | `internal/mcp/tool_impls.go` | ~100 |
| CLI `ckb affected-tests` | `cmd/ckb/affected_tests.go` | ~150 |

**Deliverable:** `ckb affected-tests` with coverage-based precision

### Phase 6: Owner Aggregation (Week 4)

**Goal:** Aggregate owners for changed + affected files

| Task | Files | LOC Est. |
|------|-------|----------|
| Extend ownership for batch | `internal/ownership/batch.go` | ~100 |
| Secondary owner detection | `internal/ownership/transitive.go` | ~80 |
| `getChangeOwners` MCP tool | `internal/mcp/tool_impls.go` | ~80 |
| CLI `ckb reviewers` | `cmd/ckb/reviewers.go` | ~120 |

**Reuses:**
- `ownership.GetOwnersForPath()` (100%)
- `ownership.CodeownersToOwners()` (100%)

**Deliverable:** `ckb reviewers` CLI and MCP tool

### Phase 7: CI Integration & Polish (Week 5)

**Goal:** CI-friendly output formats and GitHub Actions examples

| Task | Files | LOC Est. |
|------|-------|----------|
| JSON output format | Already exists | 0 |
| GitHub annotations format | `internal/output/annotations.go` | ~80 |
| Example workflows | `docs/examples/github-actions/` | ~100 |
| Documentation | Wiki updates | ~300 |

**Deliverable:** Production-ready CI integration

---

## Reusable Components

### From `internal/impact/`

| Component | Location | Reuse |
|-----------|----------|-------|
| `ImpactAnalyzer.Analyze()` | `analyzer.go:68` | 100% - core analysis loop |
| `ComputeRiskScore()` | `risk.go:37` | 100% - weighted factor scoring |
| `DeriveVisibility()` | `visibility.go` | 100% - symbol visibility detection |
| `BlastRadius` struct | `types.go:61` | 100% - impact metrics |
| `ClassifyBlastRadius()` | `types.go:78` | 100% - risk classification |
| `TransitiveCallerProvider` | `analyzer.go:11` | 100% - interface for call graph |

### From `internal/backends/scip/`

| Component | Location | Reuse |
|-----------|----------|-------|
| `FindCallers()` | `callgraph.go:138` | 100% - direct callers |
| `BuildCallGraph()` | `callgraph.go:264` | 100% - BFS with depth limit |
| `FindReferences()` | `adapter.go` | 100% - symbol references |

### From `internal/ownership/`

| Component | Location | Reuse |
|-----------|----------|-------|
| `ParseCodeownersFile()` | `codeowners.go:36` | 100% |
| `GetOwnersForPath()` | `codeowners.go:160` | 100% |
| `CodeownersToOwners()` | `codeowners.go:329` | 100% |

### From `internal/query/`

| Component | Location | Reuse |
|-----------|----------|-------|
| `Engine.AnalyzeImpact()` | `impact.go:171` | Extend for batch mode |
| `SummarizePR()` | `pr.go:82` | Pattern for diff handling |
| `getSuggestedReviewers()` | `pr.go:273` | Pattern for owner aggregation |

### From `internal/diff/`

| Component | Location | Reuse |
|-----------|----------|-------|
| `Delta` types | `types.go` | Reference for symbol change types |
| `SymbolRecord` | `types.go:54` | Reference structure |

---

## New Development

### New Package: `internal/testmap/`

Purpose: Map code changes to affected tests

```go
// internal/testmap/mapper.go
package testmap

// TestMapper maps symbols/files to tests
type TestMapper interface {
    // GetTestsForSymbol returns tests that cover a symbol
    GetTestsForSymbol(symbolID string) ([]Test, error)

    // GetTestsForFile returns tests that cover a file
    GetTestsForFile(filePath string) ([]Test, error)

    // GetTestsForPackage returns all tests in a package
    GetTestsForPackage(pkgPath string) ([]Test, error)
}

// Test represents a test case
type Test struct {
    Name     string   // Test function name
    Path     string   // Test file path
    Package  string   // Package path
    Reason   string   // Why this test was selected
    Priority int      // Run order priority
}

// CompositeMapper chains multiple mappers
type CompositeMapper struct {
    mappers []TestMapper
}
```

### New Package: `internal/diff/gitdiff.go`

Purpose: Parse git unified diffs and map to symbols

```go
// internal/diff/gitdiff.go
package diff

import (
    "github.com/sourcegraph/go-diff/diff"
)

// ChangedSymbol represents a symbol affected by a diff
type ChangedSymbol struct {
    SymbolID   string
    Name       string
    File       string
    ChangeType ChangeType // Added, Modified, Deleted
    Lines      []int      // Changed line numbers
}

type ChangeType string

const (
    ChangeAdded    ChangeType = "added"
    ChangeModified ChangeType = "modified"
    ChangeDeleted  ChangeType = "deleted"
)

// ParseGitDiff parses a unified diff and extracts changed files/lines
func ParseGitDiff(diffContent string) (*ParsedDiff, error)

// MapDiffToSymbols maps changed lines to SCIP symbols
func (p *ParsedDiff) MapToSymbols(scipIndex *scip.SCIPIndex) ([]ChangedSymbol, error)
```

### Extension: `internal/query/impact.go`

Add batch analysis method:

```go
// AnalyzeChangeSetOptions contains options for batch impact analysis
type AnalyzeChangeSetOptions struct {
    Symbols      []string // Symbol IDs to analyze
    Depth        int      // Transitive depth
    IncludeTests bool
    IncludeOwners bool
}

// AnalyzeChangeSetResponse aggregates impact across multiple symbols
type AnalyzeChangeSetResponse struct {
    Summary           ChangeSummary
    ChangedSymbols    []ChangedSymbolInfo
    AffectedSymbols   []ImpactItem        // Deduplicated
    AffectedModules   []ModuleImpact      // Aggregated
    AffectedTests     []AffectedTest
    AffectedOwners    []Owner
    BlastRadius       *BlastRadiusSummary // Combined
    RiskAssessment    *RiskScore          // Aggregated
    Recommendations   []Recommendation
}

// AnalyzeChangeSet analyzes impact of multiple changed symbols
func (e *Engine) AnalyzeChangeSet(ctx context.Context, opts AnalyzeChangeSetOptions) (*AnalyzeChangeSetResponse, error)
```

---

## Testing Strategy

### Unit Tests

| Package | Test Focus |
|---------|------------|
| `internal/diff/` | Diff parsing, line extraction, symbol mapping |
| `internal/testmap/` | Each mapper strategy, composite chaining |
| `internal/query/` | `AnalyzeChangeSet` aggregation logic |

### Integration Tests

| Test | Description |
|------|-------------|
| `TestImpactFromDiff` | End-to-end: git diff → impact analysis |
| `TestAffectedTestsNamingConvention` | foo.go → foo_test.go mapping |
| `TestAffectedTestsWithCoverage` | coverage.out → test selection |
| `TestReviewersAggregation` | CODEOWNERS → reviewer list |

### NFR Tests

| Metric | Target |
|--------|--------|
| Token usage (summary output) | <2000 tokens |
| Latency (100 changed symbols) | <5s |
| Accuracy (test selection vs full suite) | >90% |

---

## Design Decisions

These questions have been resolved:

### 1. Cross-Repository Impact

**Decision:** No cross-repo for v8.0. Single-repo only.

**Rationale:**
- Federation support in CKB is for querying across repos, not unified impact graphs
- Cross-repo impact requires understanding dependency relationships (go.mod replace directives, internal module versions)
- Most users asking "what breaks?" mean within their current working directory

**Future:** Consider `--follow-dependents` flag for v8.1 that checks if changed package is imported by other indexed repos.

### 2. Stale Index Behavior

**Decision:** Warn but proceed with best-effort analysis.

```
$ ckb impact --commit HEAD~1

⚠ Warning: SCIP index is 3 days older than HEAD
  Some symbols may be missing or have moved.
  Run 'ckb index' to refresh.

Impact Analysis:
  ...results...
```

**Rationale:**
- Refusing breaks CI workflows where index might be slightly stale
- Auto-reindex is unpredictable and breaks scripting expectations
- Best-effort with clear warning lets users decide

**Implementation:** Add `--strict` flag for users who want hard failure on staleness.

### 3. Relationship with Existing `analyzeImpact`

**Decision:** Keep both tools separate.

| Tool | Input | Use Case |
|------|-------|----------|
| `analyzeImpact` | Single symbol ID | "What calls this function?" |
| `analyzeChange` | Diff/commit/files | "What might break from these changes?" |

**Rationale:**
- Different mental models: symbol exploration vs. change validation
- `analyzeImpact` is useful mid-development ("I'm about to change this, show me callers")
- `analyzeChange` is useful pre-commit/PR ("Here's what I changed, what's affected?")
- Internally, `analyzeChange` calls `analyzeImpact` for each changed symbol — composition, not replacement

### 4. Minimum Go Version

**Decision:** Verify during Phase 1, likely compatible.

go-diff requires Go 1.19+. CKB likely already requires 1.21+ for other dependencies.

```bash
# Phase 1 action item
go mod edit -require github.com/sourcegraph/go-diff@latest
go mod tidy
go build ./...
```

### 5. Status vs Doctor Integration

**Decision:** Split checks between `ckb status` (repo-specific) and `ckb doctor` (installation-wide).

| Command | Scope | Purpose |
|---------|-------|---------|
| `ckb status` | Current repo | "What's the state of this indexed repo?" |
| `ckb doctor` | CKB installation | "Is my CKB setup healthy?" |

**`ckb status` additions** (repo-specific):
```
$ ckb status

Repository: /Users/lisa/code/ckb
Index:      ✓ Fresh (indexed 5 minutes ago, 8,432 symbols)
Language:   Go

Change Impact Analysis:
  Coverage:   ⚠ Not found (test mapping will use heuristics)
              Generate: go test -coverprofile=coverage.out ./...
  CODEOWNERS: ⚠ Not found (reviewer suggestions unavailable)
              Create: .github/CODEOWNERS
```

**`ckb doctor` additions** (installation-wide):
```
$ ckb doctor

CKB Installation
  Version:    v8.0.0

Dependencies:
  ✓ Git 2.43.0
  ✓ go-diff (bundled)

Optional Tools:
  ✓ gh CLI (for 'ckb reviewers --format=gh')
  ⚠ flutter not found (coverage generation for Flutter projects)

Health:
  ✓ All checks passed
```

### 6. Coverage Configuration

**Decision:** Add per-repo config for coverage file locations.

```yaml
# .ckb/config.yaml
coverage:
  paths:                    # Explicit paths (checked in order)
    - coverage/lcov.info
    - coverage.out
  auto_detect: true         # Also check common locations
  max_age: 168h             # Warn if older than 7 days

ownership:
  paths:
    - .github/CODEOWNERS
    - CODEOWNERS
```

This allows customization for non-standard project layouts while providing sensible defaults.

---

## Appendix A: Research Decisions

### Decision 1: Static TIA vs. ML-Based Predictive Test Selection

**Context:** Industry offers two approaches:
- **Static TIA**: Code analysis → dependency graph → test mapping (deterministic)
- **Predictive Test Selection**: ML on historical test results (adaptive)

**Decision:** Use Static TIA

**Rationale:**
1. CKB already has SCIP indexes with dependency data
2. We don't have access to users' historical test results
3. Deterministic results align with "code intelligence" positioning
4. ML approach requires 3-4 weeks of training data (not practical for CLI tool)

**Sources:**
- [Launchable - ML Alternative to Traditional TIA](https://www.launchableinc.com/blog/machine-learning-alternative-to-test-impact-analysis/)
- [Martin Fowler - The Rise of Test Impact Analysis](https://martinfowler.com/articles/rise-test-impact-analysis.html)

---

### Decision 2: Local Coverage Files First, Not Codecov API

**Context:** The original spec prioritized Codecov API integration.

**Decision:** Start with local coverage file parsing (`coverage.out`, `lcov.info`)

**Rationale:**
1. Codecov's "Impact Analysis" is actually **production telemetry** (OpenTelemetry spans), not test coverage mapping
2. Local coverage files are universally available
3. No API keys or external dependencies required
4. Matches industry standard (diff-cover pattern)

**Sources:**
- [Codecov Impact Analysis Python Quickstart](https://docs.codecov.com/docs/impact-analysis-quickstart-python) - Uses OpenTelemetry, not coverage
- [diff-cover](https://github.com/Bachmann1234/diff_cover) - Industry standard pattern

---

### Decision 3: Use sourcegraph/go-diff for Diff Parsing

**Context:** Need to parse unified git diffs.

**Decision:** Use `github.com/sourcegraph/go-diff`

**Alternatives Considered:**
| Library | Pros | Cons |
|---------|------|------|
| `sourcegraph/go-diff` | Mature, widely used, parse-only | Parse-only (what we need) |
| `bluekeyes/go-gitdiff` | Full patch application | More than we need |
| `waigani/diffparser` | Simple API | Less maintained |

**Rationale:**
1. Parse-only is exactly what we need (we don't apply patches)
2. Maintained by Sourcegraph (same org as SCIP)
3. Well-tested in production

**Source:** [sourcegraph/go-diff](https://github.com/sourcegraph/go-diff)

---

### Decision 4: Transitive Depth Default of 2, Not 3

**Context:** Spec proposed depth 3, existing code uses 2.

**Decision:** Keep default depth at 2

**Rationale:**
1. Existing `internal/impact/analyzer.go:27` uses 2
2. Depth 3+ causes exponential growth in results
3. Performance concern for large codebases
4. User can override via `--depth` flag if needed

---

### Decision 5: Three-Strategy Test Selection Model

**Context:** How precise should test selection be?

**Decision:** Offer three strategies: `precise`, `safe` (default), `full`

| Strategy | Description | Use Case |
|----------|-------------|----------|
| `precise` | Only tests that directly cover changed lines | Fast feedback, may miss edge cases |
| `safe` | Include tests for transitive callers (1 level) | Default, good balance |
| `full` | All tests in affected packages | Pre-merge, paranoid mode |

**Rationale:**
1. Different CI stages need different trade-offs
2. Matches industry practice (Launchable's "confidence curve")
3. User controls risk tolerance

---

### Decision 6: Drop Live Watch Mode from Phase 1

**Context:** Spec proposed `watchChange` for live impact updates.

**Decision:** Defer to future version

**Rationale:**
1. Requires file system watching + incremental SCIP updates
2. Significant complexity for marginal value
3. Focus on core value proposition first
4. Can add later based on user demand

---

### Decision 7: Test Mapping Fallback Chain

**Context:** What to do when coverage files are unavailable?

**Decision:** Use fallback chain with decreasing precision:

```
1. Coverage file (coverage.out, lcov.info)     → Precise line-level
2. Naming convention (foo.go → foo_test.go)    → File-level
3. Import analysis (tests importing package)    → Package-level
4. Package tests (all *_test.go in directory)  → Directory-level
```

**Rationale:**
1. Always provides some answer (never "unknown")
2. Confidence score reflects data quality
3. Matches [gocovdiff](https://github.com/vearutop/gocovdiff) approach

---

### Decision 8: Existing `internal/diff/` Package Is Unrelated

**Context:** Spec assumed `internal/diff/` parses git diffs.

**Finding:** It doesn't. It generates delta artifacts for incremental SCIP indexing.

**Decision:** Create new git diff parsing code, don't modify existing package

**Rationale:**
1. Different concern (incremental indexing vs. change analysis)
2. Existing code is well-tested for its purpose
3. New code can be in same package but separate files

---

## Appendix B: Industry Analysis

### How Industry Leaders Do This

| Tool | Approach | Data Source | Precision |
|------|----------|-------------|-----------|
| **diff-cover** | Static, file-level | Coverage XML + git diff | Line-level |
| **gocovdiff** | Static, file-level | Go coverage.out + git diff | Line-level |
| **Launchable** | ML-based | Historical test results | Probabilistic |
| **Codecov Impact** | Runtime | OpenTelemetry spans | Call-frequency |
| **Google TAP** | Static + ML | Coverage + history | High |

### CKB's Approach

CKB combines:
1. **SCIP indexes** for precise symbol-level dependency graph
2. **Local coverage files** for test mapping
3. **Git integration** for change detection
4. **Static analysis** for deterministic results

This positions CKB as more precise than grep-based tools, more accessible than ML-based tools (no training period), and more comprehensive than IDE-based tools (cross-file, risk scoring).

### Key Differentiators

1. **Symbol-level granularity** — Not just file-level like diff-cover
2. **Transitive impact** — Propagates through call graph
3. **Risk scoring** — Weighted factors, not just counts
4. **Integrated ownership** — CODEOWNERS + git blame
5. **LLM-optimized output** — Token budgets, drilldowns

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Test selection accuracy | >90% vs full suite | Compare failures caught |
| False negatives (missed breaks) | <1% | Production incidents |
| Token usage (summary) | <2000 tokens | NFR tests |
| Latency (medium codebase) | <5s | NFR tests |
| User adoption | Used in >50% of PRs | Telemetry (opt-in) |

---

## References

- [Martin Fowler - The Rise of Test Impact Analysis](https://martinfowler.com/articles/rise-test-impact-analysis.html)
- [Launchable - What is Predictive Test Selection?](https://www.launchableinc.com/blog/what-is-predictive-test-selection/)
- [Launchable - How Launchable selects tests](https://help.launchableinc.com/features/predictive-test-selection/how-launchable-selects-tests/)
- [diff-cover - GitHub](https://github.com/Bachmann1234/diff_cover)
- [gocovdiff - GitHub](https://github.com/vearutop/gocovdiff)
- [sourcegraph/go-diff - GitHub](https://github.com/sourcegraph/go-diff)
- [Codecov Impact Analysis Docs](https://docs.codecov.com/docs/impact-analysis-quickstart-python)
- [SCIP Code Intelligence Protocol](https://sourcegraph.com/blog/announcing-scip)

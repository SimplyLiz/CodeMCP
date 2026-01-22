# Change Impact Analysis — Implementation Checklist

**Feature Plan:** [change-impact-analysis.md](./change-impact-analysis.md)
**Target Version:** v8.0
**Last Updated:** 2025-12-26

---

## Questions Resolved

- [x] **Cross-repo impact**: No for v8.0. Single-repo only. Consider `--follow-dependents` for v8.1.
- [x] **Stale index behavior**: Warn but proceed. Add `--strict` flag for CI that wants hard failure.
- [x] **Tool consolidation**: Keep both. `analyzeImpact` (single symbol) and `analyzeChange` (diff/commit) serve different use cases.
- [x] **Minimum Go version**: Verify in Phase 1 during dependency addition. Likely compatible (go-diff requires 1.19+).

---

## Phase 0: Interface Definitions

**Goal:** Define core interfaces to enable parallel development

### Core Interfaces (`internal/impact/interfaces.go`)
- [ ] Define `DiffParser` interface
- [ ] Define `SymbolMapper` interface
- [ ] Define `TestMapper` interface
- [ ] Define `ImpactAggregator` interface
- [ ] Document interface contracts in godoc

**Acceptance:** Interfaces compiled, documented, reviewed by team

---

## Phase 1: Core Infrastructure

### Dependencies
- [ ] Add `github.com/sourcegraph/go-diff` to go.mod

### Git Diff Parser (`internal/diff/gitdiff.go`)
- [ ] Create `ParsedDiff` struct
- [ ] Create `ChangedFile` struct with hunks
- [ ] Create `ChangedSymbol` struct with confidence:
  ```go
  type ChangedSymbol struct {
      SymbolID   string
      Name       string
      File       string
      ChangeType ChangeType  // Added, Modified, Deleted
      Lines      []int       // Changed line numbers
      Confidence float64     // How certain is the mapping? (0.0-1.0)
      HunkIndex  int         // For debugging/tracing
  }
  ```
- [ ] Implement `ParseGitDiff(diffContent string) (*ParsedDiff, error)`
- [ ] Implement `ParseGitDiffFromFile(path string) (*ParsedDiff, error)`
- [ ] Handle file renames (detect via similarity threshold)
- [ ] Skip whitespace-only and comment-only changes (lower confidence)
- [ ] Unit tests for diff parsing

### Symbol Mapping (`internal/diff/symbolmap.go`)
- [ ] Implement `(p *ParsedDiff) MapToSymbols(scipIndex) ([]ChangedSymbol, error)`
- [ ] Handle added lines → new symbols
- [ ] Handle modified lines → changed symbols
- [ ] Handle deleted lines → removed symbols
- [ ] Set confidence based on mapping precision:
  - 1.0: Exact symbol definition on changed line
  - 0.8: Symbol body contains changed line
  - 0.5: Changed line in function but no symbol found
  - 0.3: Whitespace/comment only change
- [ ] Unit tests for symbol mapping

**Deliverable:** `ParseGitDiff()` → `[]ChangedSymbol` working

**Acceptance Criteria:**
- Parses unified diff format correctly
- Handles file renames and binary files gracefully
- >90% unit test coverage
- Confidence scores reflect mapping precision

---

## Phase 2: Multi-Symbol Impact Aggregation

### Query Engine Extension (`internal/query/impact.go`)
- [ ] Create `AnalyzeChangeSetOptions` struct
- [ ] Create `AnalyzeChangeSetResponse` struct
- [ ] Create `ChangeSummary` struct
- [ ] Implement `(e *Engine) AnalyzeChangeSet(ctx, opts) (*AnalyzeChangeSetResponse, error)`
- [ ] Deduplicate affected symbols across all inputs:
  - Dedup key: fully-qualified symbol ID (SCIP stable ID)
  - When same symbol appears multiple times: keep highest confidence
  - When same symbol changed in multiple commits: merge change types
- [ ] Aggregate blast radii (union of modules, max of callers per module)
- [ ] Aggregate risk scores (weighted average, not just max)
- [ ] Generate combined recommendations
- [ ] Unit tests for aggregation logic

### MCP Tool (`internal/mcp/tool_impls.go`)
- [ ] Register `analyzeChange` tool
- [ ] Implement tool handler with `symbols[]` input
- [ ] Wire to `Engine.AnalyzeChangeSet()`
- [ ] Integration test for MCP tool

**Deliverable:** `analyzeChange` MCP tool with `symbols[]` input

**Acceptance Criteria:**
- Handles 100+ symbols in <5s
- Deduplication produces deterministic results
- Risk aggregation matches expected weighted formula

---

## Phase 3: Diff Input Sources

### Git Backend Extension (`internal/backends/git/diff.go`)
- [ ] Implement `GetDiffFromCommit(sha string) (string, error)`
- [ ] Implement `GetDiffFromRange(base, head string) (string, error)`
- [ ] Implement `GetUncommittedDiff() (string, error)`
- [ ] Implement `GetStagedDiff() (string, error)` — for pre-commit hooks
- [ ] Unit tests for git operations

### MCP Tool Extension
- [ ] Add `diff` input option to `analyzeChange`
- [ ] Add `commit` input option
- [ ] Add `commitRange` input option
- [ ] Add `files[]` input option (treat as file paths, get symbols)
- [ ] Add `staged` boolean option (only staged changes)

### Stale Index Detection (`internal/query/staleness.go`)
- [ ] Compare SCIP index timestamp to git HEAD timestamp
- [ ] Return staleness info (days old, commits behind)
- [ ] Unit tests

### CLI Command (`cmd/ckb/impact.go`)
- [ ] Create `impact` command
- [ ] Add `--commit` flag
- [ ] Add `--range` flag
- [ ] Add `--files` flag
- [ ] Add `--staged` flag — for pre-commit hooks
- [ ] Add `--strict` flag — fail on stale index (for CI)
- [ ] Add `--format` flag (summary, json)
- [ ] Add `--min-risk` flag
- [ ] Add `--depth` flag
- [ ] Show staleness warning when index is old
- [ ] Human-readable summary output
- [ ] JSON output format
- [ ] Integration test for CLI

**Deliverable:** `ckb impact` CLI with all input types

**Acceptance Criteria:**
- All input types (diff, commit, range, files, staged) produce consistent output
- `--staged` works correctly in pre-commit hook context
- `--strict` returns exit code 1 on stale index
- JSON output is valid and matches schema

---

## Phase 4: Test Mapping — Fallback Strategies

### Test Mapper Interface (`internal/testmap/mapper.go`)
- [ ] Define `TestMapper` interface
- [ ] Define `Test` struct with priority field
- [ ] Define `MapperResult` with confidence
- [ ] Implement `CompositeMapper` for chaining

### Priority Order (highest to lowest):
1. **CoverageMapper** (Phase 5) — confidence 1.0, line-level precision
2. **ImportMapper** — confidence 0.8, package-level
3. **NamingMapper** — confidence 0.6, file-level
4. **PackageMapper** — confidence 0.4, directory-level fallback

### Import-Based Mapper (`internal/testmap/imports.go`)
- [ ] Implement `ImportMapper` struct
- [ ] Query SCIP for test files importing changed package
- [ ] Filter to only `_test.go` files
- [ ] Unit tests

### Naming Convention Mapper (`internal/testmap/naming.go`)
- [ ] Implement `NamingMapper` struct
- [ ] `foo.go` → `foo_test.go` mapping
- [ ] `internal/pkg/bar.go` → `internal/pkg/bar_test.go`
- [ ] Handle table-driven test patterns
- [ ] Unit tests

### Package Mapper (`internal/testmap/packages.go`)
- [ ] Implement `PackageMapper` struct
- [ ] Find all `*_test.go` in same directory
- [ ] Unit tests

### Composite Mapper (`internal/testmap/composite.go`)
- [ ] Chain mappers in priority order
- [ ] Return confidence based on which mapper matched
- [ ] Deduplicate tests across mappers
- [ ] Unit tests

**Deliverable:** Test selection without coverage files

**Acceptance Criteria:**
- Falls back gracefully when higher-priority mappers return empty
- Confidence scores correctly reflect data source
- >80% accuracy on real Go projects (vs manual inspection)

---

## Phase 5: Coverage File Parsing

### Go Coverage Parser (`internal/testmap/coverage_go.go`)
- [ ] Parse `coverage.out` format
- [ ] Extract file:line → covered boolean
- [ ] Map covered lines to test files (from coverage mode)
- [ ] Unit tests with sample coverage files

### LCov Parser (`internal/testmap/coverage_lcov.go`) — *Stretch Goal*
- [ ] Parse `lcov.info` format
- [ ] Extract file:line → hit count
- [ ] Unit tests

### Coverage Staleness Detection (`internal/testmap/staleness.go`)
- [ ] Compare coverage file mtime to source file mtimes
- [ ] Track which source files are newer than coverage
- [ ] Emit warning if coverage is stale
- [ ] Add `--max-coverage-age` flag (default: 7d)
- [ ] Unit tests

### Coverage Mapper (`internal/testmap/coverage.go`)
- [ ] Implement `CoverageMapper` struct
- [ ] Auto-detect format from file extension (.out, .info)
- [ ] Integrate with composite mapper (highest priority)
- [ ] Return staleness warnings in result
- [ ] Unit tests

### MCP Tool (`internal/mcp/tool_impls.go`)
- [ ] Register `getAffectedTests` tool
- [ ] Implement tool handler
- [ ] Return tests with reasons and priorities
- [ ] Include staleness warnings in response
- [ ] Generate runnable commands

### CLI Command (`cmd/ckb/affectedtests.go`)
- [ ] Create `affected-tests` command
- [ ] Add `--strategy` flag (precise, safe, full)
- [ ] Add `--output` flag (list, command, json)
- [ ] Add `--coverage` flag (path to coverage file)
- [ ] Add `--max-coverage-age` flag
- [ ] Human-readable output with reasons
- [ ] Show staleness warnings
- [ ] Integration test

**Deliverable:** `ckb affected-tests` with coverage-based precision

**Acceptance Criteria:**
- Parses Go coverage.out correctly
- Detects and warns on stale coverage
- Command output is copy-pastable (`go test -run '...'`)

---

## Phase 6: Owner Aggregation

**Note:** CODEOWNERS parsing already exists in `internal/ownership/codeowners.go`. This phase extends it for batch operations.

### Batch Ownership (`internal/ownership/batch.go`)
- [ ] Implement `GetOwnersForPaths(paths []string) ([]Owner, error)`
- [ ] Deduplicate owners across files
- [ ] Track which files each owner covers
- [ ] Handle glob patterns (`*.go`, `internal/**`)
- [ ] Handle team references (`@org/team`)

### Transitive Ownership (`internal/ownership/transitive.go`) — *Stretch Goal*
- [ ] Implement `GetSecondaryOwners(affectedFiles []string) ([]Owner, error)`
- [ ] Filter to owners of affected-but-not-changed files
- [ ] Lower confidence for secondary owners

### MCP Tool (`internal/mcp/tool_impls.go`)
- [ ] Register `getChangeOwners` tool
- [ ] Implement tool handler
- [ ] Return primary and secondary owners
- [ ] Generate review strategy

### CLI Command (`cmd/ckb/reviewers.go`)
- [ ] Create `reviewers` command
- [ ] Add `--include-affected` flag
- [ ] Add `--format` flag (human, json, gh)
- [ ] Output for `gh pr edit --add-reviewer` integration
- [ ] Human-readable output
- [ ] Integration test

**Deliverable:** `ckb reviewers` CLI and MCP tool

**Acceptance Criteria:**
- Correctly parses all CODEOWNERS patterns
- `--format=gh` produces valid `gh` CLI arguments

---

## Phase 7: CI Integration & Polish

### Output Formats (`internal/output/`)
- [ ] GitHub Actions annotations format (`annotations.go`)
- [ ] GitLab CI format (`gitlab.go`) — *Stretch Goal*
- [ ] Markdown format for PR comments (`markdown.go`)
- [ ] JUnit XML for test results aggregation — *Stretch Goal*

### Status Integration (`cmd/ckb/status.go`)

Repo-specific checks (shown when running `ckb status` in a repo):

- [ ] Auto-detect coverage file by language:
  - Go: `coverage.out`, `coverage.txt`
  - Flutter/Dart: `coverage/lcov.info`
  - Generic: `lcov.info`, `.coverage`
- [ ] Show coverage file path and age (if found)
- [ ] Warn if coverage older than `coverage.max_age` config
- [ ] Check for CODEOWNERS in standard locations
- [ ] Show CODEOWNERS stats (team count, pattern count) if found
- [ ] Suggest generation command based on detected language
- [ ] Add "Change Impact Analysis" section to status output

Example output:
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

### Doctor Integration (`cmd/ckb/doctor.go`)

Installation/environment checks (independent of current repo):

- [ ] Check git is available and version
- [ ] Check go-diff is bundled correctly (always passes, internal)
- [ ] Check optional tools that enhance functionality:
  - `gh` CLI (for `--format=gh` output)
  - Language toolchains for coverage hints
- [ ] Verify index cache directory is writable
- [ ] Check for stale/orphaned indexes (repos that no longer exist)

Example output:
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

### Configuration (`internal/config/config.go`)

Per-repo config for coverage paths:

- [ ] Add `coverage.paths` option (list of paths to check)
- [ ] Add `coverage.auto_detect` option (default: true)
- [ ] Add `coverage.max_age` option (default: 168h / 7 days)
- [ ] Add `ownership.paths` option (default: standard locations)
- [ ] `ckb status` reads from config, falls back to auto-detect

```yaml
# .ckb/config.yaml
coverage:
  paths:
    - coverage/lcov.info
    - coverage.out
  auto_detect: true
  max_age: 168h

ownership:
  paths:
    - .github/CODEOWNERS
    - CODEOWNERS
```

### Documentation
- [ ] Update CLI reference in wiki
- [ ] Add `analyzeChange` to MCP tools documentation
- [ ] Add `getAffectedTests` to MCP tools documentation
- [ ] Add `getChangeOwners` to MCP tools documentation
- [ ] Create GitHub Actions workflow example
- [ ] Add configuration options to Configuration.md
- [ ] Add troubleshooting section (stale index, missing coverage)

### NFR Tests (with targets)
| Metric | Target | Test |
|--------|--------|------|
| Token usage (`analyzeChange` summary) | <2000 tokens | `TestAnalyzeChangeTokenBudget` |
| Token usage (`getAffectedTests`) | <1000 tokens | `TestAffectedTestsTokenBudget` |
| Latency (100 changed symbols) | <5s | `BenchmarkAnalyzeChange100` |
| Latency (1000 changed symbols) | <30s | `BenchmarkAnalyzeChange1000` |
| Memory (1000 symbols) | <500MB | `BenchmarkAnalyzeChangeMemory` |

### Final Polish
- [ ] Error messages for edge cases
- [ ] Help text for all CLI commands
- [ ] Update CLAUDE.md with new commands
- [ ] Add `--quiet` flag for CI scripts
- [ ] Exit codes: 0=ok, 1=error, 2=high-risk (for CI gates)

**Deliverable:** Production-ready feature

**Acceptance Criteria:**
- All NFR targets met
- GitHub Actions example runs successfully
- Documentation reviewed and merged

---

## Testing Checklist

### Unit Tests
- [ ] `internal/diff/gitdiff_test.go`
- [ ] `internal/diff/symbolmap_test.go`
- [ ] `internal/testmap/mapper_test.go`
- [ ] `internal/testmap/naming_test.go`
- [ ] `internal/testmap/imports_test.go`
- [ ] `internal/testmap/packages_test.go`
- [ ] `internal/testmap/composite_test.go`
- [ ] `internal/testmap/coverage_go_test.go`
- [ ] `internal/testmap/coverage_lcov_test.go` (if implemented)
- [ ] `internal/testmap/staleness_test.go`
- [ ] `internal/ownership/batch_test.go`
- [ ] `internal/query/impact_changeset_test.go`

### Integration Tests
- [ ] End-to-end: git diff → impact analysis
- [ ] End-to-end: commit range → affected tests
- [ ] End-to-end: staged changes → impact
- [ ] MCP tool integration tests (`analyzeChange`, `getAffectedTests`, `getChangeOwners`)
- [ ] CLI integration tests (`ckb impact`, `ckb affected-tests`, `ckb reviewers`)

### NFR/Benchmark Tests
- [ ] `internal/query/impact_bench_test.go`
- [ ] `internal/mcp/impact_token_test.go`

### Manual Testing
- [ ] Test on CKB's own codebase
- [ ] Test on a large Go project (kubernetes, etc.)
- [ ] Test with missing coverage files (fallback)
- [ ] Test with stale coverage files (warning)
- [ ] Test pre-commit hook scenario (`--staged`)
- [ ] Test CI scenario (JSON output, exit codes)

---

## Files Summary

### New Files
```
# Core
internal/impact/interfaces.go
internal/query/staleness.go
internal/query/staleness_test.go

# Diff Parsing
internal/diff/gitdiff.go
internal/diff/gitdiff_test.go
internal/diff/symbolmap.go
internal/diff/symbolmap_test.go

# Test Mapping
internal/testmap/mapper.go
internal/testmap/mapper_test.go
internal/testmap/naming.go
internal/testmap/naming_test.go
internal/testmap/imports.go
internal/testmap/imports_test.go
internal/testmap/packages.go
internal/testmap/packages_test.go
internal/testmap/composite.go
internal/testmap/composite_test.go
internal/testmap/coverage.go
internal/testmap/coverage_go.go
internal/testmap/coverage_go_test.go
internal/testmap/staleness.go
internal/testmap/staleness_test.go

# Ownership
internal/ownership/batch.go
internal/ownership/batch_test.go

# Output
internal/output/annotations.go
internal/output/markdown.go

# CLI
cmd/ckb/impact.go
cmd/ckb/affectedtests.go
cmd/ckb/reviewers.go

# Docs
docs/examples/github-actions/impact-analysis.yml
```

### Modified Files
```
go.mod                              # Add go-diff dependency
internal/query/impact.go            # Add AnalyzeChangeSet method
internal/query/impact_test.go       # Add AnalyzeChangeSet tests
internal/mcp/tools.go               # Register new tools
internal/mcp/tool_impls.go          # Implement new tools
internal/backends/git/adapter.go    # Add diff methods (GetStagedDiff, etc.)
internal/config/config.go           # Add coverage/ownership config options
cmd/ckb/status.go                   # Add Change Impact Analysis section
cmd/ckb/doctor.go                   # Add optional tool checks
CLAUDE.md                           # Document new commands
```

### Estimated Total LOC
| Category | Lines |
|----------|-------|
| New code | ~2800 |
| Tests | ~1800 |
| Documentation | ~600 |
| **Total** | **~5200** |

---

## Stretch Goals (Can Be Cut for v8.0)

These items are marked with *Stretch Goal* in the checklist above:

1. **LCov parser** (`internal/testmap/coverage_lcov.go`) — Go coverage.out may be sufficient
2. **Secondary/transitive owners** (`internal/ownership/transitive.go`) — Primary owners are more important
3. **GitLab CI format** (`internal/output/gitlab.go`) — GitHub Actions is priority
4. **JUnit XML output** — Only needed if users request it

### Minimum Viable Feature (v8.0-beta)

Phases 1-3 alone would be valuable:
- `ckb impact` with commit/range input
- `analyzeChange` MCP tool
- Risk scoring and blast radius

Test mapping (Phases 4-5) can be v8.1 if time is tight.

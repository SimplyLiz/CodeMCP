# CKB Review: Quickstart

## Install (30 seconds)

```bash
npm install -g @tastehub/ckb
```

## Setup for your AI tool (30 seconds)

```bash
# Claude Code
ckb setup --tool=claude-code

# Cursor
ckb setup --tool=cursor

# Windsurf
ckb setup --tool=windsurf

# VS Code (Copilot)
ckb setup --tool=vscode

# Interactive (prompts for tool + options)
ckb setup
```

## Index your repo (one time)

```bash
cd your-project
ckb init
ckb index
```

This creates a SCIP index for full code intelligence. Without it, CKB falls back to git-only checks (still useful, just fewer features).

## Review a PR

### From your AI assistant

Ask Claude Code, Cursor, or Windsurf:

> Review this PR against main

Your assistant will call CKB's `reviewPR` tool automatically and use the results to focus its review.

If you installed the `/ckb-review` skill (Claude Code prompts during setup):

> /ckb-review

### From the CLI

```bash
# Human-readable output
ckb review --base=main

# JSON (for piping to other tools)
ckb review --base=main --format=json

# Review staged changes
ckb review --staged

# Only specific checks
ckb review --checks=secrets,breaking,bug-patterns

# CI mode (exit codes: 0=pass, 1=fail, 2=warn)
ckb review --base=main --ci

# Post as PR comment
ckb review --base=main --post=123
```

### In CI

```yaml
# GitHub Actions
- name: CKB Review
  run: npx @tastehub/ckb review --base=${{ github.event.pull_request.base.ref }} --ci --format=sarif > review.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: review.sarif
```

## What You Get

```
CKB Review: WARN  ·  133 files  ·  19200 lines
========================================================

  Changes 133 files across 32 modules (go). 8 new bug
  pattern(s); 133 files across 32 clusters — split
  recommended.

Checks:
  ⚠ bug-patterns     8 new bug pattern(s) (31 pre-existing filtered)
  ⚠ coupling         1 co-changed file missing
  ⚠ risk             Risk score: 1.00 (high)
  ⚠ split            32 independent clusters
  ○ test-gaps        16 untested functions (top 10 shown)
  ○ hotspots         50 hotspot files (top 10 shown)
  ✓ secrets · breaking · dead-code · health · tests · complexity
    · format-consistency · comment-drift

Top Findings:
  ⚠ review.go:267       err shadowed (outer error lost)
  ⚠ setup.go:215        err shadowed (non-fatal)
  ⚠ diff.go             +11 cyclomatic in GetCommitRangeDiff()
  ⚠ pr.go               +13 cyclomatic in SummarizePR()
```

**15 checks, 5 seconds, 0 tokens, 0 API calls.**

## The 20 Checks

| Check | What it detects | Requires SCIP? |
|---|---|---|
| secrets | Leaked credentials (API keys, tokens) | No |
| breaking | Removed/renamed public API symbols | Yes |
| tests | Test coverage of changed code | Partial |
| complexity | Cyclomatic/cognitive complexity increases | No (tree-sitter) |
| health | 8-factor weighted code health score | Partial |
| coupling | Files that historically change together | No (git) |
| hotspots | High-churn files ranked by volatility | No (git) |
| risk | Composite risk score (size, churn, modules) | No |
| dead-code | Symbols with zero references | Yes |
| test-gaps | Functions above complexity threshold without tests | Partial |
| blast-radius | Symbols with many callers | Yes |
| bug-patterns | 10 AST rules (defer-in-loop, nil-after-deref, etc.) | No (tree-sitter) |
| split | PR decomposition into independent clusters | No |
| comment-drift | Stale numeric references in comments | No |
| format-consistency | Human vs markdown output divergence | No |
| critical | Safety-critical path changes | No (config) |
| traceability | Commit-to-ticket linkage | No (config) |
| independence | Author != reviewer verification | No (git) |
| generated | Generated file detection and exclusion | No |
| classify | Change categorization (new, modified, refactored) | No |

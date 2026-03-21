# How CKB Makes AI Code Review Better

## The Token Problem

When an AI assistant reviews a PR, it reads files to answer questions. Most questions have deterministic answers that don't need an LLM:

| Question | LLM approach | Cost |
|---|---|---|
| Any secrets in the diff? | Read every file, scan for patterns | ~160k tokens |
| Any breaking API changes? | Read all public interfaces, diff | ~200k tokens |
| Which files have the most churn? | Can't compute — no git history | Impossible |
| Which functions lack tests? | Read all test files, cross-reference | ~80k tokens |
| What's the complexity delta? | Parse every function, compare | ~100k tokens |
| Is there dead code? | Cross-reference all symbols | ~200k tokens |

On a 100-file PR, an LLM spends ~500k tokens just establishing baseline facts before it even starts reviewing logic.

## What CKB Does

CKB computes all of those answers in 5 seconds for 0 tokens using local tools:

- **SCIP index** — pre-built symbol graph for reference counting, API comparison, dead code detection
- **Tree-sitter** — fast AST parsing for complexity metrics and bug pattern detection
- **Git history** — churn analysis, co-change patterns, hotspot scoring
- **Pattern matching** — secrets detection, generated file detection

The AI assistant calls `reviewPR(compact: true)` via MCP and gets ~1k tokens of structured results instead of spending ~500k tokens computing them from source.

## What the AI Assistant Gets

```json
{
  "verdict": "warn",
  "score": 61,
  "activeChecks": [
    {"name": "bug-patterns", "status": "warn", "summary": "8 new bug pattern(s)"},
    {"name": "coupling", "status": "warn", "summary": "1 co-changed file missing"},
    {"name": "test-gaps", "status": "info", "summary": "16 untested functions"}
  ],
  "passedChecks": ["secrets", "breaking", "dead-code", "health", "tests", "complexity", ...],
  "findings": [
    {"check": "bug-patterns", "file": "review.go", "line": 267, "message": "err shadowed"},
    {"check": "hotspots", "file": "review.go", "message": "Hotspot (score: 20.21)"},
    ...
  ],
  "drillDown": "Use findReferences, analyzeImpact, explainSymbol to investigate"
}
```

From this, the AI assistant knows:
- **Skip** secrets, breaking, dead-code, health, tests, complexity, format-consistency, comment-drift (all pass)
- **Read** review.go (hotspot 20.21, has err shadow), setup.go (complexity +16, has err shadow)
- **Check** coupling gap with handlers_upload_delta.go
- **Investigate** the 16 untested functions for potential risks

## What the AI Assistant Skips

On our 133-file test PR, the assistant:

| Category | Without CKB | With CKB |
|---|---|---|
| Secret scanning | Read 133 files (~160k tokens) | Skipped — CKB says clean |
| API diffing | Read all exports (~200k tokens) | Skipped — CKB says no breaks |
| Dead code search | Cross-reference symbols (~200k tokens) | Skipped — CKB says none |
| Test audit | Read test files (~80k tokens) | Skipped — CKB says 27 covering |
| Health check | Compare before/after (~50k tokens) | Skipped — CKB says 0 degraded |
| Files to read | 37 files | **10 files** |
| **Total tokens** | **87,336** | **45,784 (-48%)** |

## What the AI Assistant Finds Better

CKB doesn't just save tokens — it helps the assistant find real bugs by pointing to the right files:

**Without CKB:** The assistant picked 37 files to review based on file names and diff size. It found 4 issues. It missed the deadlock in `followLogs()` because it didn't know that function was untested and high-complexity.

**With CKB:** The assistant saw CKB's test-gap finding: "`followLogs` — complexity 6, untested." It read the function and found the `select{}` deadlock. It also verified CKB's 2 err-shadow findings as real bugs.

CKB tells the assistant what the reviewer needs to know. The assistant tells the reviewer what the code actually does wrong.

## The Drill-Down Advantage

After the initial `reviewPR` call, the AI assistant can use CKB's 80+ MCP tools to investigate findings without reading source:

```
Assistant sees: "dead-code: FormatSARIF — no references"
Assistant calls: findReferences(symbolId: "...FormatSARIF...")
CKB returns: "3 references found in review.go, format_review_test.go"
Assistant concludes: false positive, skip it
```

Each drill-down call is 0 tokens (CKB answers from its in-memory SCIP index). The assistant reads source only when CKB's tools can't answer the question.

## The Scale Effect

CKB's value grows with PR size:

| PR Size | Without CKB | With CKB | Token Savings |
|---|---|---|---|
| 10 files | ~20k tokens | ~18k tokens | 10% |
| 50 files | ~100k tokens | ~40k tokens | 60% |
| 100 files | ~200k tokens | ~50k tokens | 75% |
| 600 files | ~500k tokens | ~80k tokens | **84%** |

On small PRs, CKB is a nice-to-have. On large PRs, it's the difference between "review the whole thing" and "here are the 10 files that matter."

## Works With Any MCP-Compatible Tool

CKB runs as an MCP server. Any AI tool that supports MCP gets the same benefits:

- **Claude Code** — `/ckb-review` skill included
- **Cursor** — calls `reviewPR` via MCP
- **Windsurf** — calls `reviewPR` via MCP
- **VS Code (Copilot)** — MCP support available
- **OpenCode** — MCP support available
- **Custom agents** — any MCP client

Setup for each tool:
```bash
ckb setup --tool=claude-code
ckb setup --tool=cursor
ckb setup --tool=windsurf
```

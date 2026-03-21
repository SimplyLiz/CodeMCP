Run a comprehensive code review using CKB's deterministic analysis + your semantic review.

## Input
$ARGUMENTS - Optional: base branch (default: main), or "staged" for staged changes, or a PR number

## MCP vs CLI

CKB runs as an MCP server in this environment. MCP mode is strongly preferred for interactive review because the SCIP index stays loaded between calls — drill-down tools like `findReferences`, `analyzeImpact`, and `explainSymbol` execute instantly against the in-memory index. CLI mode reloads the index on every invocation.

## The Three Phases

### Phase 1: CKB structural scan (5 seconds, 0 tokens)

Call the `reviewPR` MCP tool with compact mode:
```
reviewPR(baseBranch: "main", compact: true)
```

This returns ~1k tokens instead of ~30k — just the verdict, non-pass checks, top 10 findings, and action items. Use `compact: false` only if you need the full raw data.

If a PR number was given, get the base branch first:
```bash
BASE=$(gh pr view $ARGUMENTS --json baseRefName -q .baseRefName)
```
Then pass it: `reviewPR(baseBranch: BASE, compact: true)`

> **If CKB is not running as an MCP server** (last resort), use the CLI instead:
> ```bash
> ./ckb review --base=main --format=json
> ```
> Note: CLI mode reloads the SCIP index on every call, so drill-down steps will be slower.

From CKB's output, immediately note:
- **Passed checks** → skip these categories. Don't waste tokens re-checking secrets, breaking changes, test coverage, etc.
- **Warned checks** → your review targets
- **Top hotspot files** → read these first
- **Test gaps** → functions to evaluate

### Phase 2: Drill down on CKB findings (0 tokens via MCP)

Before reading source code, use CKB's MCP tools to investigate specific findings. These calls are instant because the SCIP index is already loaded from Phase 1.

| CKB finding | Drill-down tool | What to check |
|---|---|---|
| Dead code | `findReferences(symbolId: "...")` or `searchSymbols` → `findReferences` | Does it actually have references? CKB's SCIP index can miss cross-package refs |
| Blast radius | `analyzeImpact(symbolId: "...")` | Are the "callers" real logic or just framework registrations? |
| Coupling gap | `explainSymbol(name: "...")` on the missing file | What does the co-change partner do? Does it actually need updates? |
| Bug patterns | Already verified by differential analysis | Just check the specific line CKB flagged |
| Complexity | `explainFile(path: "...")` | What functions are driving the increase? |
| Test gaps | `getAffectedTests(baseBranch: "main")` | Which tests exist? Which functions are actually untested? |
| Hotspots | `getHotspots(limit: 10)` | Full churn history for the flagged files |

### Phase 3: Semantic review of high-risk files

Now read the actual source — but only for:
1. Files CKB ranked as top hotspots
2. Files with warned findings that survived drill-down
3. New files (CKB can't assess design quality of new code)

For each file, look for things CKB CANNOT detect:
- Logic bugs (wrong conditions, off-by-one, race conditions)
- Security issues (injection, auth bypass, data exposure)
- Design problems (wrong abstraction, unclear naming, leaky interfaces)
- Edge cases (nil inputs, empty collections, concurrent access)
- Error handling quality (not just missing — wrong strategy)

### Phase 4: Write the review

Format:

```markdown
## Summary
One paragraph: what the PR does, overall assessment.

## Must Fix
Findings that should block merge. File:line references.

## Should Fix
Issues worth addressing but not blocking.

## CKB Analysis
- Verdict: [pass/warn/fail], Score: [0-100]
- [N] checks passed, [N] warned
- Key findings: [top 3]
- False positives identified: [any CKB findings you disproved]
- Test gaps: [N] untested functions — [your assessment of which matter]

## Recommendation
Approve / Request changes / Needs discussion
```

## Tips

- If CKB says "secrets: pass" — trust it, don't re-scan 100+ files
- If CKB says "breaking: pass" — trust it, SCIP-verified API comparison
- If CKB says "dead-code: FormatSARIF" — DON'T trust blindly, verify with `findReferences` or grep
- CKB's hotspot scores are based on git churn history — higher score = more volatile file = review more carefully
- CKB's complexity delta shows WHERE cognitive load increased — read those functions

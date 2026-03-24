Run a CKB-augmented code review optimized for minimal token usage.

## Input
$ARGUMENTS - Optional: base branch (default: main), or "staged" for staged changes, or a PR number

## Philosophy

CKB already answered the structural questions (secrets? breaking? dead code? test gaps?).
The LLM's job is ONLY what CKB can't do: semantic reasoning about correctness, design,
and intent. Every source line you read costs tokens — read only what CKB says is risky.

## Phase 1: Structural scan (~1k tokens into context)

```bash
ckb review --base=main --format=json --compact 2>/dev/null
```

If a PR number was given:
```bash
BASE=$(gh pr view $ARGUMENTS --json baseRefName -q .baseRefName)
ckb review --base=$BASE --format=json --compact 2>/dev/null
```

From the output, build three lists:
- **SKIP**: passed checks — don't touch these files or topics
- **INVESTIGATE**: warned/failed checks — these are your review scope
- **READ**: hotspot files + files with warn/fail findings — the only files you'll read

**Early exit**: If verdict=pass and score≥80, write a one-line approval and stop. No source reading needed.

## Phase 2: Targeted source reading (the only token-expensive step)

Do NOT read the full diff. Do NOT read every changed file.

Read ONLY:
1. Files that appear in INVESTIGATE findings (just the changed hunks via `git diff main...HEAD -- <file>`)
2. New files (CKB has no history for these) — but only if <500 lines each
3. Skip generated files, test files for existing tests, and config/CI files

For each file you read, look for exactly:
- Logic errors (wrong condition, off-by-one, nil deref)
- Security issues (injection, auth bypass, secrets)
- Design problems (wrong abstraction, leaky interface)
- Missing edge cases the tests don't cover

Do NOT look for: style, naming, formatting, documentation, test coverage —
CKB already checked these structurally.

## Phase 3: Write the review (be terse)

```markdown
## [APPROVE|REQUEST CHANGES|DISCUSS] — CKB score: [N]/100

[One sentence: what the PR does]

### Issues
1. **[must-fix|should-fix]** `file:line` — [issue in one sentence]
2. ...

### CKB passed (no review needed)
[comma-separated list of passed checks]

### CKB flagged (verified above)
[for each warn/fail finding: confirmed/false-positive + one-line reason]
```

If no issues found: just the header line + CKB passed list. Nothing else.

## Anti-patterns (token waste)

- Reading files CKB marked as pass → waste
- Reading generated files → waste
- Summarizing what the PR does in detail → waste (git log exists)
- Explaining why passed checks passed → waste
- Running MCP drill-down tools when CLI already gave enough signal → waste
- Reading test files to "verify test quality" → waste unless CKB flagged test-gaps
- Reading hotspot-only files with no findings → high churn ≠ needs review right now

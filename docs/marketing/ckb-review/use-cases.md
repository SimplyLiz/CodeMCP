# CKB Review: Use Cases

## 1. AI-Assisted PR Review (Primary)

**Who:** Developers using Claude Code, Cursor, Windsurf, or any MCP-compatible AI tool.

**The workflow:**
- Developer asks AI assistant: "review this PR"
- Assistant calls CKB `reviewPR` tool → gets structural analysis in 5 seconds
- Assistant skips categories CKB confirmed clean
- Assistant reads only flagged files → finds real issues

**Token savings:** 50-80% on PRs with 50+ files. The AI assistant reads 10 files instead of 600.

**Quality improvement:** CKB tells the assistant which files are hotspots, which functions lack tests, and which symbols have high fan-out. The assistant finds bugs it would miss without this context — in our evaluation, CKB's test-gap data led the assistant to a deadlock bug in a function it would have skipped.

**Setup:**
```bash
npx @tastehub/ckb setup --tool=claude-code
# Then ask Claude: /ckb-review
```

---

## 2. CI Quality Gates (Zero Cost)

**Who:** Teams running CI/CD on GitHub Actions, GitLab CI, or any pipeline.

**The workflow:**
- CKB runs on every push/PR — 5 seconds, no API keys, no tokens
- Blocks on secrets, breaking API changes
- Posts SARIF results to GitHub Security tab
- Posts markdown review summary as PR comment

**What it catches automatically:**
- Leaked credentials (API keys, tokens, passwords)
- Breaking API changes (removed/renamed public symbols)
- Dead code left behind after refactoring
- Missing test coverage for complex functions
- Code health regressions

**Setup:**
```yaml
# .github/workflows/review.yml
- uses: tastehub/ckb-review@v1
  with:
    base-branch: main
    fail-on: error
```

Or standalone:
```bash
npx @tastehub/ckb review --base=main --ci --format=sarif > results.sarif
```

**Cost:** $0. No LLM. No cloud. Runs in your CI runner.

---

## 3. Large PR Triage

**Who:** Tech leads, senior developers reviewing PRs with 100+ files.

**The problem:** A 200-file PR lands. Where do you start? Reading all 200 files takes hours. Skimming the diff misses the important changes buried in boilerplate.

**What CKB gives you in 5 seconds:**
- **Split suggestion:** "This is 12 independent clusters — split into 12 smaller PRs"
- **Hotspot ranking:** "These 10 files have the most historical churn — review these first"
- **Risk score:** "0.85/1.00 — high risk due to 8 modules touched + 30 hotspots"
- **Test gaps:** "16 functions with complexity 5+ have no tests"
- **Health report:** "2 files degraded from B to C grade"

This is the "table of contents" for a large PR. Human reviewers and AI assistants both benefit.

---

## 4. Onboarding Code Review

**Who:** New team members reviewing code they don't fully understand yet.

**The problem:** A new developer is asked to review a PR in a codebase they joined 2 weeks ago. They don't know which files are critical, which modules have high coupling, or where the test gaps are.

**What CKB gives them:**
- **Coupling analysis:** "This file usually changes with that file — check both"
- **Hotspot scores:** "This file changes 3x more than average — it's fragile"
- **Blast radius:** "This function has 7 callers — changes here ripple"
- **Complexity map:** "Complexity increased +13 in SummarizePR() — that's the function to scrutinize"

CKB gives new reviewers the institutional knowledge they don't have yet.

---

## 5. Refactoring Validation

**Who:** Teams doing large refactors (rename, extract, move, restructure).

**The problem:** A 300-file refactor lands. Did it break any public APIs? Leave dead code behind? Drop test coverage? Increase complexity?

**CKB answers all of these deterministically:**
- **Breaking changes:** SCIP-based API comparison — catches removed/renamed exports
- **Dead code:** SCIP reference count + grep — finds symbols with 0 references
- **Test gaps:** Cross-references changed functions with test files
- **Health delta:** Before/after health score per file — flags regressions
- **Complexity delta:** Per-function cyclomatic change — flags functions that got harder to maintain

This is verification, not review. CKB confirms the refactor didn't make things worse.

---

## 6. Multi-Tool AI Review

**Who:** Teams using multiple AI tools (Claude Code + Cursor, or Claude Code + custom agents).

**The problem:** Each AI tool reviews the PR independently, each reading the same files, each computing the same structural analysis. Double the tokens, double the cost.

**CKB as shared context:** CKB runs once, produces JSON. Every AI tool consumes the same structured analysis. No duplication.

```bash
# Run once
ckb review --base=main --format=json > review.json

# Feed to any AI tool
cat review.json | claude "Review this CKB analysis and focus on the high-risk findings"
```

Or via MCP: every tool calls `reviewPR` and gets the same cached result.

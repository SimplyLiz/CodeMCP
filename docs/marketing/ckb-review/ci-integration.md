# CKB Review: CI Integration

## Zero-Cost Quality Gates

CKB review runs in CI without any LLM, API keys, or cloud services. 5 seconds, deterministic, reproducible.

```bash
npx @tastehub/ckb review --base=main --ci
# Exit 0 = pass, 1 = fail, 2 = warn
```

## GitHub Actions

### Basic (exit code gating)

```yaml
name: CKB Review
on: [pull_request]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for churn analysis

      - name: CKB Review
        run: npx @tastehub/ckb review --base=${{ github.event.pull_request.base.ref }} --ci
```

### With SARIF upload (GitHub Security tab)

```yaml
      - name: CKB Review
        run: npx @tastehub/ckb review --base=${{ github.event.pull_request.base.ref }} --ci --format=sarif > review.sarif
        continue-on-error: true

      - name: Upload SARIF
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: review.sarif
```

### With PR comment

```yaml
      - name: CKB Review
        run: npx @tastehub/ckb review --base=${{ github.event.pull_request.base.ref }} --post=${{ github.event.pull_request.number }}
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Full (SCIP index for maximum analysis)

```yaml
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: CKB Init + Index
        run: |
          npx @tastehub/ckb init
          npx @tastehub/ckb index

      - name: CKB Review
        run: npx @tastehub/ckb review --base=${{ github.event.pull_request.base.ref }} --ci --format=sarif > review.sarif

      - name: Upload SARIF
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: review.sarif
```

## GitLab CI

```yaml
ckb-review:
  image: node:22
  stage: test
  script:
    - npx @tastehub/ckb review --base=$CI_MERGE_REQUEST_TARGET_BRANCH_NAME --ci --format=codeclimate > codeclimate.json
  artifacts:
    reports:
      codequality: codeclimate.json
  rules:
    - if: $CI_MERGE_REQUEST_IID
```

## Output Formats

| Format | Flag | Use Case |
|---|---|---|
| human | `--format=human` | Terminal output (default) |
| json | `--format=json` | Programmatic consumption, piping to other tools |
| markdown | `--format=markdown` | PR comments |
| sarif | `--format=sarif` | GitHub Security tab, VS Code |
| codeclimate | `--format=codeclimate` | GitLab Code Quality |
| github-actions | `--format=github-actions` | GitHub Actions annotations (inline in diff) |
| compliance | `--format=compliance` | Audit evidence reports |

## What CI Gets (No SCIP Index)

Without `ckb index`, CKB falls back to git-only analysis. Still useful:

| Check | Without SCIP | With SCIP |
|---|---|---|
| secrets | Full | Full |
| breaking | Skip | Full |
| tests | Heuristic | SCIP-enhanced |
| complexity | Full (tree-sitter) | Full |
| health | Full (tree-sitter) | Full |
| coupling | Full (git) | Full |
| hotspots | Full (git) | Full |
| risk | Full | Full |
| dead-code | Skip | Full |
| test-gaps | Partial | Full |
| blast-radius | Skip | Full |
| bug-patterns | Full (tree-sitter) | Full |
| split | Full | Full |

8 of 15 checks work without any indexing. Add `ckb index` for the full 15.

## Configuration

### Policy file (.ckb/review.json)

```json
{
  "blockBreakingChanges": true,
  "blockSecrets": true,
  "failOnLevel": "error",
  "maxRiskScore": 0.8,
  "maxComplexityDelta": 20,
  "criticalPaths": ["drivers/**", "protocol/**"],
  "traceabilityPatterns": ["JIRA-\\d+"],
  "requireTraceability": true
}
```

### Environment variables

```bash
CKB_REVIEW_FAIL_ON=warning  # Override fail level
CKB_REVIEW_MAX_RISK=0.9     # Override risk threshold
```

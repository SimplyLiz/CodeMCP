# CKB PR Analysis Action

Automatically analyze pull requests with [CKB](https://github.com/tastehub/ckb) code intelligence and post formatted comments with risk assessment, suggested reviewers, and architectural insights.

## Features

- **Risk Assessment** - Identifies high-risk changes based on code churn, complexity, and ownership
- **Module Impact** - Shows which modules are affected by the changes
- **Suggested Reviewers** - Recommends reviewers based on CODEOWNERS and git history
- **Related ADRs** - Links to relevant architectural decisions

## Usage

Add to your workflow file (e.g., `.github/workflows/ckb.yml`):

```yaml
name: CKB PR Analysis
on:
  pull_request:
    types: [opened, synchronize]

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history needed for analysis

      - uses: tastehub/ckb-action@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
```

## Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `github-token` | GitHub token for posting comments | Required |
| `base-branch` | Base branch to compare against | `main` |
| `risk-threshold` | Minimum risk score to post comment (0-1) | `0` |
| `include-reviewers` | Include suggested reviewers | `true` |
| `include-adrs` | Include related ADRs | `true` |
| `fail-on-high-risk` | Fail if risk > 0.8 | `false` |
| `ckb-version` | CKB version to use | `latest` |

## Outputs

| Output | Description |
|--------|-------------|
| `risk-score` | Calculated risk score (0-1) |
| `risk-level` | Risk level (low, medium, high) |
| `comment-url` | URL of the posted comment |

## Example Comment

The action posts a comment like this:

> ## CKB Analysis
>
> ### Risk Assessment
> | Level | Score | Factors |
> |-------|-------|---------|
> | ⚠️ **Medium** | 0.58 | High churn, Multiple owners |
>
> ### Affected Modules
> | Module | Risk | Notes |
> |--------|------|-------|
> | `internal/auth/` | High | Hotspot |
> | `cmd/server/` | Low | |
>
> ### Suggested Reviewers
> | Reviewer | Reason |
> |----------|--------|
> | @alice | Owns internal/auth/ |
> | @bob | Recent commits |

## Advanced Usage

### Only comment on high-risk PRs

```yaml
- uses: tastehub/ckb-action@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    risk-threshold: '0.5'
```

### Fail CI on high-risk changes

```yaml
- uses: tastehub/ckb-action@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    fail-on-high-risk: 'true'
```

### Use specific CKB version

```yaml
- uses: tastehub/ckb-action@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    ckb-version: '7.3.0'
```

## Requirements

- Repository must have git history (use `fetch-depth: 0` in checkout)
- CKB works best with a SCIP index, but will use git-based analysis as fallback

## License

MIT

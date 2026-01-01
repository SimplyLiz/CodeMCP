# CKB GitLab CI Examples

This directory contains GitLab CI configuration for integrating CKB into your pipelines.

## Quick Start

Copy `.gitlab-ci.yml` to your repository root, or include it in your existing configuration:

```yaml
include:
  - local: 'ci/ckb.gitlab-ci.yml'
```

## Jobs Overview

| Job | Stage | Trigger | Description |
|-----|-------|---------|-------------|
| `index` | setup | All | Index repository for analysis |
| `impact-analysis` | analyze | MR | Analyze change impact and risk |
| `complexity-check` | analyze | MR | Check complexity thresholds |
| `suggest-reviewers` | analyze | MR | Get suggested reviewers |
| `hotspot-check` | analyze | MR | Warn about hotspot modifications |
| `post-mr-notes` | report | MR | Post combined report as MR note |
| `architecture-refresh` | analyze | Schedule | Full architecture refresh |

## Configuration

### Variables

Set these in your GitLab CI/CD settings or in `.gitlab-ci.yml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `CKB_VERSION` | `latest` | CKB version to install |
| `RISK_THRESHOLD` | `critical` | Risk level to fail on |
| `MAX_CYCLOMATIC` | `15` | Maximum cyclomatic complexity |
| `MAX_COGNITIVE` | `20` | Maximum cognitive complexity |
| `GITLAB_TOKEN` | - | Token for posting MR notes |

### Caching

The configuration uses per-branch caching:

```yaml
cache:
  key: ckb-${CI_COMMIT_REF_SLUG}
  paths:
    - .ckb/
```

For monorepos or large projects, consider path-based keys:

```yaml
cache:
  key: ckb-${CI_COMMIT_REF_SLUG}-${CI_PROJECT_PATH_SLUG}
```

## MR Notes

To automatically post analysis as MR notes:

1. Create a project access token with `api` scope
2. Add it as `GITLAB_TOKEN` in CI/CD variables
3. The `post-mr-notes` job will post combined reports

## Scheduled Refresh

Add a pipeline schedule for daily architecture refresh:

1. Go to CI/CD → Schedules
2. Create schedule with cron: `0 3 * * *` (3 AM daily)
3. The `architecture-refresh` job will run automatically

## Customization

### Run Only Specific Jobs

Use `rules` to control job execution:

```yaml
impact-analysis:
  rules:
    - if: $CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "main"
```

### Custom Thresholds

Override variables per-job:

```yaml
complexity-check:
  variables:
    MAX_CYCLOMATIC: "20"
    MAX_COGNITIVE: "25"
```

### Parallel Analysis

For large repositories, run analysis in parallel:

```yaml
impact-analysis:
  parallel:
    matrix:
      - MODULE: [api, web, core]
  script:
    - ckb impact --scope=${MODULE} ...
```

## Artifacts

All jobs produce artifacts for debugging and reporting:

- `impact.json` / `impact.md` - Impact analysis
- `complexity.md` - Complexity report
- `hotspots.json` / `hotspots.md` - Hotspot data
- `reviewers.json` - Suggested reviewers
- `architecture.json` - Full architecture data

Artifacts expire after 1 week (30 days for scheduled jobs).

## Troubleshooting

### Slow indexing

Enable caching and use `--if-stale`:

```yaml
script:
  - ckb index --if-stale=24h
```

### Missing dependencies

The alpine image needs git and jq:

```yaml
before_script:
  - apk add --no-cache git curl jq
```

### Token permissions

For MR notes, the token needs:
- `api` scope for posting notes
- `read_repository` scope for accessing code

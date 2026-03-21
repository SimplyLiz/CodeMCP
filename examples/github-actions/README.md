# CKB GitHub Actions Examples

This directory contains example GitHub Actions workflows for integrating CKB into your CI/CD pipeline.

## Workflows

### pr-review.yml (Recommended)

Runs the unified `ckb review` engine on pull requests — 14 quality checks in one command:
- Breaking API changes, secret detection, test coverage
- Complexity delta, code health scoring, coupling gaps
- Hotspot overlap, risk scoring, critical-path checks
- Traceability, reviewer independence, PR split suggestion
- Posts markdown PR comment, emits GHA annotations, uploads SARIF
- CI mode with configurable fail level (error/warning/none)

**Usage:**
1. Copy to `.github/workflows/pr-review.yml`
2. The workflow runs automatically on PR open/update
3. Customize checks, fail level, and critical paths in the workflow env

### pr-analysis.yml (Legacy)

Uses the HTTP API to analyze PRs. Superseded by `pr-review.yml` which uses the CLI directly.

- Summary of changed files and lines
- Risk assessment (low/medium/high)
- Hotspots touched
- Suggested reviewers based on code ownership
- Ownership drift warnings

**Usage:**
1. Copy to `.github/workflows/pr-analysis.yml`
2. Update the CKB installation step for your setup
3. The workflow runs automatically on PR open/update

### scheduled-refresh.yml

Runs daily to refresh CKB's architectural model:
- Updates module detection
- Recomputes hotspots
- Caches analysis for faster queries
- Generates architecture reports

**Usage:**
1. Copy to `.github/workflows/scheduled-refresh.yml`
2. Update the CKB installation step
3. Optionally enable the "Commit CKB Database" step to cache results

## Configuration

### Installing CKB

Replace the installation step with your preferred method:

```yaml
# npm (Recommended)
- uses: actions/setup-node@v4
  with:
    node-version: '20'
- run: npm install -g @tastehub/ckb

# Or use npx (no install needed)
- run: npx @tastehub/ckb init

# Pre-built binary (alternative)
- run: |
    curl -sSL https://github.com/SimplyLiz/CodeMCP/releases/latest/download/ckb_linux_amd64.tar.gz | tar xz
    sudo mv ckb /usr/local/bin/
```

### Customizing Analysis

CKB exposes a REST API. Start the server and use `curl`:

```bash
# Start server in background
ckb serve --port 8080 &
sleep 2

# PR analysis with ownership
curl -X POST http://localhost:8080/pr/summary \
  -H "Content-Type: application/json" \
  -d '{"baseBranch": "main", "includeOwnership": true}'

# Ownership drift for specific module
curl "http://localhost:8080/ownership/drift?scope=internal/api&threshold=0.4&limit=10"

# Get hotspots
curl "http://localhost:8080/hotspots?limit=20"

# Async architecture refresh
curl -X POST http://localhost:8080/architecture/refresh \
  -H "Content-Type: application/json" \
  -d '{"scope": "all", "async": true}'
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CKB_REPO_ROOT` | Repository root path | Current directory |
| `CKB_LOG_LEVEL` | Log verbosity | `info` |
| `CKB_CONFIG_PATH` | Custom config location | `.ckb/config.json` |

## Tips

1. **Fetch full history**: Use `fetch-depth: 0` to enable git-blame analysis
2. **Cache CKB database**: Commit `.ckb/` to cache analysis results
3. **Parallel jobs**: CKB is safe to run in parallel for different modules
4. **Rate limiting**: Add delays between API calls if hitting limits

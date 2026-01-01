# CKB Examples

Production-ready CI/CD workflows and configuration examples.

## Quick Start

```bash
# GitHub Actions - copy a single workflow
cp examples/github-actions/pr-analysis.yml .github/workflows/

# GitHub Actions - copy all workflows
cp examples/github-actions/*.yml .github/workflows/

# GitLab CI
cp examples/gitlab-ci/.gitlab-ci.yml .gitlab-ci.yml

# Git hooks
cp examples/hooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Directory Structure

```
examples/
├── github-actions/          # GitHub Actions workflows
│   ├── starter-template.yml # Modular starter - copy & customize
│   ├── full-showcase.yml    # Complete feature demo (CKB's own workflow)
│   ├── pr-analysis.yml      # PR risk assessment and reviewers
│   ├── impact-analysis.yml  # Change impact with risk gates
│   ├── impact-comment.yml   # Simple PR comment widget
│   ├── incremental-indexing.yml  # Fast CI with caching
│   ├── complexity-gate.yml  # Block high-complexity code
│   ├── affected-tests.yml   # Run only affected tests
│   ├── dead-code-detection.yml  # Weekly dead code scan
│   ├── hotspot-monitor.yml  # Track code hotspots
│   ├── codeowners-sync.yml  # Auto-update CODEOWNERS
│   ├── contract-check.yml   # API contract safety
│   ├── doc-quality.yml      # Documentation coverage
│   ├── risk-audit.yml       # 8-factor risk analysis
│   ├── slack-notifications.yml  # Team alerts
│   ├── reusable-analysis.yml    # Org-wide standardization
│   ├── scheduled-refresh.yml    # Daily architecture refresh
│   ├── eval-suite.yml           # Search quality regression testing
│   ├── coupling-analysis.yml    # Co-change pattern detection
│   ├── language-quality.yml     # Per-language quality metrics
│   └── telemetry-dead-code.yml  # Production-aware dead code
├── gitlab-ci/               # GitLab CI configuration
│   └── .gitlab-ci.yml       # Complete GitLab CI template
└── hooks/                   # Git hooks for local development
    ├── pre-commit           # Block complex/risky commits
    ├── pre-push             # Validate before pushing
    ├── .pre-commit-config.yaml  # pre-commit framework
    ├── husky-pre-commit     # Husky integration
    └── ckb-check.js         # lint-staged integration
```

## Workflow Categories

### Getting Started
| Workflow | Description |
|----------|-------------|
| `starter-template.yml` | **Start here** - modular template with all features, delete what you don't need |
| `impact-comment.yml` | Minimal setup - just posts impact analysis as PR comment |
| `pr-analysis.yml` | Standard PR analysis with reviewers and risk |

### Full Featured
| Workflow | Description |
|----------|-------------|
| `full-showcase.yml` | **Complete demo** - all features, beautiful reports |
| `impact-analysis.yml` | Impact analysis with risk gates and reviewer assignment |
| `reusable-analysis.yml` | Organization-wide standardized workflow |

### Quality Gates
| Workflow | Description |
|----------|-------------|
| `complexity-gate.yml` | Fail PRs exceeding complexity thresholds |
| `contract-check.yml` | Detect breaking API contract changes |
| `doc-quality.yml` | Enforce documentation coverage |
| `eval-suite.yml` | Search quality regression testing |

### Optimization
| Workflow | Description |
|----------|-------------|
| `incremental-indexing.yml` | Fast CI with cached incremental indexes |
| `affected-tests.yml` | Run only tests affected by changes |

### Maintenance
| Workflow | Description |
|----------|-------------|
| `dead-code-detection.yml` | Weekly scan for unused code |
| `telemetry-dead-code.yml` | Production-aware dead code (with telemetry) |
| `hotspot-monitor.yml` | Track high-churn files over time |
| `codeowners-sync.yml` | Keep CODEOWNERS accurate |
| `risk-audit.yml` | Comprehensive codebase risk report |
| `coupling-analysis.yml` | Detect co-change patterns |
| `language-quality.yml` | Per-language indexer quality metrics |

### Notifications
| Workflow | Description |
|----------|-------------|
| `slack-notifications.yml` | Alerts for high-risk changes |

### Scheduled
| Workflow | Description |
|----------|-------------|
| `scheduled-refresh.yml` | Daily architecture model refresh |

## Documentation

Full documentation for each workflow with example outputs:

**[Workflow Examples](https://github.com/SimplyLiz/CodeMCPWiki/wiki/Workflow-Examples)** in the wiki

## Customization

Most workflows use environment variables for configuration:

```yaml
env:
  MAX_CYCLOMATIC: 15        # Complexity threshold
  MAX_COGNITIVE: 20         # Cognitive complexity threshold
  RISK_THRESHOLD: critical  # Risk level to fail on
```

See individual workflow files for all available options.

## Requirements

- **Git history**: Use `fetch-depth: 0` for accurate analysis
- **CKB installation**: Workflows use `npm install -g @tastehub/ckb`
- **Caching**: Most workflows cache `.ckb/` for performance

## Contributing

To add a new workflow:

1. Create the workflow file in the appropriate directory
2. Add documentation to `Workflow-Examples.md` in the wiki
3. Update this README's directory structure

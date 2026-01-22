# CKB CI/CD Intelligence Features

**Theme:** Enable AI coding assistants to optimally plan and execute CI/CD strategies using CKB's code intelligence capabilities.

**Origin:** Market research (January 2026) revealed strong demand for AI-assisted CI/CD planning. CircleCI, Argo CD, and Jenkins already have MCP servers. Test Impact Analysis (TIA) is becoming standard. CKB has the foundational primitives but lacks the orchestration layer to synthesize them into CI/CD recommendations.

---

## Architectural Decision: Part of CKB vs Separate Tool

### Option A: Part of CKB

**Pros:**
- Features are tightly coupled to existing primitives (`getAffectedTests`, `analyzeImpact`, `prepareChange`, etc.)
- Single MCP server — AI assistants don't need to coordinate multiple tools
- Shared caching, backends, symbol identity system
- One install, one config
- Lower adoption friction — users already have CKB

**Cons:**
- CKB already has 58 tools — risk of bloat
- Muddies the "code intelligence" positioning
- CI platform integrations (Phase 4) feel like a different domain

### Option B: Separate Tool (`ckb-ci` or `cikit`)

**Pros:**
- Cleaner separation of concerns
- Independent release cycle
- Could be marketed to CI/DevOps personas specifically
- CKB stays focused on code intelligence

**Cons:**
- Must call CKB via MCP or embed it — adds latency and complexity
- Two tools to install, configure, keep in sync
- AI assistants need to manage two MCP servers
- Duplicates infrastructure (caching, error handling, etc.)

### Decision

**Phases 1-3 belong in CKB** — they're essentially compound operations over existing tools. `planCI` is just `getAffectedTests` + `analyzeImpact` + `auditRisk` + synthesis logic. Same pattern as `explore` and `prepareChange`.

**Phase 4 (platform integrations) is debatable** — calling GitHub/CircleCI APIs is a different concern. Options:
- A thin CLI wrapper that calls CKB for analysis, then talks to platforms
- A separate MCP server that federates to CKB
- Documentation on how to integrate CKB with existing CI platform MCP servers (CircleCI already has one)

**Recommendation:** Keep Phases 1-3 in CKB. Revisit Phase 4 scope when implementation begins — it might make sense as a separate `ckb-ci` tool or simply as integration guides for existing platform MCP servers.

---

## Problem Statement

### Current State

CKB provides powerful primitives for CI/CD-relevant analysis:

| Existing Tool | CI/CD Relevance |
|---------------|-----------------|
| `getAffectedTests` | Test selection (TIA) |
| `analyzeImpact` / `prepareChange` | Blast radius assessment |
| `auditRisk` | Risk scoring for deployment decisions |
| `getHotspots` | Identify volatile code |
| `summarizeDiff` / `summarizePr` | Change understanding |
| `findDeadCode` | CI cleanup candidates |
| `compareAPI` | Breaking change detection |

### The Gap

AI assistants must manually orchestrate 4-6 tool calls to answer basic CI/CD questions:

1. "What tests should I run for this PR?" → `getAffectedTests` + `analyzeImpact` + `getHotspots`
2. "Is this change safe to deploy?" → `auditRisk` + `prepareChange` + `getOwnership`
3. "What's the optimal CI pipeline for this change?" → No direct answer

**Key insight:** CI/CD platforms (CircleCI, GitHub Actions, GitLab CI) are integrating AI. CKB can differentiate by providing **pre-indexed semantic understanding** that other tools lack—impact analysis without runtime coverage, symbol-level change detection, and static test mapping.

### Market Context

- **CircleCI MCP Server** — Natural language CI queries, but no code intelligence
- **Elastic Self-Healing PRs** — Claude fixed 24 broken PRs in month one, saved ~20 dev days
- **Test Impact Analysis** — Datadog, Azure DevOps, Launchable all offer TIA; CKB can provide static alternative
- **GitLab AI** — 1.5M developers using AI-assisted CI/CD (30% faster releases)

---

## Feature Overview

| Phase | Version | Scope | Priority |
|-------|---------|-------|----------|
| 1. CI Strategy Planner | v8.3 | Core compound tool for CI recommendations | High |
| 2. Build Error Intelligence | v8.3 | Enhanced error parsing with CI context | High |
| 3. Pipeline Generation | v8.4 | Generate and optimize pipeline configs | Medium |
| 4. CI Platform Integrations | v8.5 | Direct integrations (scope TBD) | Medium |
| 5. CI Feedback Loop | v9.x | Learn from CI outcomes | Future |

---

## Phase 1: CI Strategy Planner

**Version:** v8.3
**Priority:** High
**Effort:** Medium
**Scope:** Part of CKB (compound operation)

### Tool: `planCI`

**Purpose:** Given a change (commit, PR, or file list), return a comprehensive CI strategy recommendation.

```json
// Input
{
  "target": "HEAD~1..HEAD",        // commit range, PR number, or file list
  "targetType": "auto",            // "auto" | "commit" | "pr" | "files"
  "pipelineContext": {             // optional: existing pipeline info
    "platform": "github-actions",  // "github-actions" | "circleci" | "gitlab" | "jenkins" | "generic"
    "defaultTestCommand": "go test ./...",
    "availableRunners": ["ubuntu-latest", "macos-latest"]
  }
}

// Output
{
  "summary": {
    "changeScope": "moderate",      // "trivial" | "moderate" | "significant" | "major"
    "riskLevel": "medium",          // "low" | "medium" | "high" | "critical"
    "confidence": 0.87,
    "recommendation": "Run targeted tests with integration suite"
  },

  "testStrategy": {
    "recommended": "targeted",       // "skip" | "smoke" | "targeted" | "full" | "full+extended"
    "rationale": "Changes affect 3 modules with good test coverage",
    "affectedTests": {
      "unit": ["pkg/auth/handler_test.go", "pkg/auth/validation_test.go"],
      "integration": ["tests/integration/auth_flow_test.go"],
      "e2e": []
    },
    "testCount": {
      "affected": 12,
      "total": 456,
      "coverage": "2.6%"
    },
    "fallbackToFull": {
      "recommended": false,
      "reason": "High confidence in test mapping"
    }
  },

  "blastRadius": {
    "filesChanged": 5,
    "symbolsChanged": 8,
    "directConsumers": 23,
    "transitiveImpact": 67,
    "modulesAffected": ["auth", "api", "middleware"],
    "crossModuleBoundary": true
  },

  "riskFactors": [
    {
      "factor": "hotspot",
      "severity": "medium",
      "detail": "auth/handler.go changed 12 times in 30 days",
      "mitigation": "Ensure thorough review"
    },
    {
      "factor": "low_coverage",
      "severity": "low",
      "detail": "middleware/ratelimit.go has 45% coverage",
      "mitigation": "Consider adding tests before merge"
    },
    {
      "factor": "cross_team",
      "severity": "medium",
      "detail": "Changes span @backend-team and @security-team ownership",
      "mitigation": "Request reviews from both teams"
    }
  ],

  "deploymentGuidance": {
    "strategy": "canary",            // "direct" | "canary" | "blue-green" | "feature-flag"
    "rationale": "Cross-module changes with medium risk suggest gradual rollout",
    "suggestedCanaryPercentage": 10,
    "monitoringFocus": ["auth latency", "error rates in /api/v1/login"]
  },

  "reviewers": {
    "required": [
      {"user": "alice", "reason": "Owns auth module", "confidence": 0.95}
    ],
    "suggested": [
      {"user": "bob", "reason": "Recent contributor to affected files", "confidence": 0.72}
    ],
    "securityReviewRequired": true
  },

  "pipelineHints": {
    "parallelizable": ["lint", "unit-tests", "security-scan"],
    "sequential": [["unit-tests", "integration-tests"], ["integration-tests", "e2e-tests"]],
    "canSkip": ["full-rebuild"],
    "mustRun": ["security-scan"],
    "estimatedDuration": {
      "targeted": "3-5 min",
      "full": "15-20 min"
    }
  },

  "cacheInfo": {
    "indexFreshness": "current",
    "backendSources": ["scip", "git"],
    "analysisTimestamp": "2026-01-17T10:30:00Z"
  }
}
```

### Implementation

```
internal/query/ci_strategy.go
├── PlanCI(target, options) → CIStrategy
│   ├── ResolveTarget() → []FileChange, []SymbolChange
│   ├── parallel:
│   │   ├── GetAffectedTests(symbolChanges)
│   │   ├── AnalyzeImpact(symbolChanges)
│   │   ├── AuditRisk(symbolChanges)
│   │   ├── GetHotspots(affectedFiles)
│   │   └── GetOwnership(affectedFiles)
│   ├── SynthesizeTestStrategy(affectedTests, riskScore, coverage)
│   ├── CalculateBlastRadius(impact)
│   ├── AssessRiskFactors(hotspots, coverage, ownership)
│   ├── DetermineDeploymentStrategy(riskLevel, changeScope)
│   └── GeneratePipelineHints(testStrategy, platform)
```

### Files to Create

| File | Purpose |
|------|---------|
| `internal/ci/strategy_types.go` | Type definitions for CI strategy |
| `internal/ci/test_strategy.go` | Test strategy calculation logic |
| `internal/ci/deployment_advisor.go` | Deployment strategy recommendations |
| `internal/query/ci_strategy.go` | Core CI planning logic |
| `internal/query/ci_strategy_test.go` | Unit tests |
| `internal/mcp/tool_impls_ci.go` | MCP handler for `planCI` |

### Tasks

- [ ] **1.1** Define `CIStrategy` and related types in `internal/ci/strategy_types.go`
- [ ] **1.2** Implement target resolution (commit range → file/symbol changes)
- [ ] **1.3** Implement parallel data gathering (tests, impact, risk, hotspots, ownership)
- [ ] **1.4** Implement test strategy synthesis algorithm
- [ ] **1.5** Implement blast radius calculation (aggregating from `analyzeImpact`)
- [ ] **1.6** Implement risk factor assessment with severity and mitigation
- [ ] **1.7** Implement deployment strategy advisor
- [ ] **1.8** Implement pipeline hints generation
- [ ] **1.9** Add MCP handler for `planCI` tool
- [ ] **1.10** Add comprehensive tests with golden fixtures
- [ ] **1.11** Add documentation and examples

### Success Metrics

| Metric | Target |
|--------|--------|
| Tool call reduction | 60%+ fewer calls for CI planning workflows |
| Test selection accuracy | 95%+ of affected tests identified |
| Response time | <3s p95 |
| Risk prediction accuracy | 80%+ correlation with actual deployment issues |

---

## Phase 2: Build Error Intelligence

**Version:** v8.3
**Priority:** High
**Effort:** Medium
**Scope:** Part of CKB
**Dependency:** Builds on v8.2 `parseBuildErrors`

### Enhancement: `parseBuildErrors` CI Mode

Extend the v8.2 `parseBuildErrors` tool with CI-specific analysis.

```json
// Extended Input
{
  "output": "<build log>",
  "format": "auto",
  "ciMode": true,               // NEW: Enable CI-specific analysis
  "recentCommits": 5            // NEW: How many commits to check for cause
}

// Extended Output (in addition to v8.2 fields)
{
  "errors": [...],  // existing v8.2 error parsing

  "ciAnalysis": {
    "likelyCause": {
      "confidence": 0.85,
      "commit": "abc123",
      "author": "alice",
      "message": "Refactor auth validation",
      "filesChanged": ["auth/handler.go", "auth/validation.go"],
      "explanation": "Renamed ValidateToken to ValidateUserToken but caller not updated"
    },
    "breakingChange": {
      "detected": true,
      "type": "renamed_symbol",
      "oldSymbol": "ValidateToken",
      "newSymbol": "ValidateUserToken",
      "affectedLocations": 3
    },
    "autoFixable": {
      "count": 2,
      "fixes": [
        {
          "file": "auth/handler.go",
          "line": 42,
          "current": "ValidateToken(user)",
          "suggested": "ValidateUserToken(user)",
          "confidence": 0.95
        }
      ]
    },
    "pipelineRecommendation": {
      "action": "fix_and_retry",    // "fix_and_retry" | "manual_intervention" | "rollback"
      "rationale": "Simple rename fix with high confidence"
    }
  }
}
```

### Tool: `diagnoseCI`

**Purpose:** Comprehensive CI failure diagnosis combining build errors, test failures, and recent changes.

```json
// Input
{
  "buildLog": "<full CI log>",
  "testResults": "<test output>",    // optional
  "pipelineRun": {                   // optional: pipeline context
    "platform": "github-actions",
    "runId": "12345",
    "branch": "feature/auth-refactor"
  }
}

// Output
{
  "diagnosis": {
    "primaryIssue": "build_failure",  // "build_failure" | "test_failure" | "timeout" | "resource"
    "rootCause": {
      "confidence": 0.87,
      "explanation": "Breaking change in commit abc123 renamed ValidateToken",
      "evidence": [
        "Error message mentions undefined ValidateToken",
        "Commit abc123 2h ago renamed ValidateToken to ValidateUserToken",
        "3 call sites not updated"
      ]
    }
  },

  "buildErrors": { /* parseBuildErrors output */ },
  "testFailures": { /* parseTestResults output if tests failed */ },

  "suggestedFixes": [
    {
      "priority": 1,
      "type": "code_change",
      "description": "Update 3 call sites from ValidateToken to ValidateUserToken",
      "autoFixConfidence": 0.92,
      "changes": [
        {"file": "auth/handler.go", "line": 42, "change": "..."}
      ]
    }
  ],

  "alternativeActions": [
    {
      "action": "revert",
      "commit": "abc123",
      "risk": "low",
      "rationale": "Clean revert possible, no dependent commits"
    }
  ],

  "preventionAdvice": [
    "Run `ckb compareAPI` before pushing breaking changes",
    "Add pre-commit hook for symbol rename detection"
  ]
}
```

### Tasks

- [ ] **2.1** Extend `parseBuildErrors` with `ciMode` parameter
- [ ] **2.2** Implement commit history correlation for error causes
- [ ] **2.3** Implement breaking change detection from symbol aliases
- [ ] **2.4** Implement auto-fix suggestion generation
- [ ] **2.5** Create `diagnoseCI` compound tool
- [ ] **2.6** Integrate test result parsing (from v8.1 if available)
- [ ] **2.7** Add prevention advice generation
- [ ] **2.8** Add MCP handlers
- [ ] **2.9** Add tests with real-world build log fixtures

---

## Phase 3: Pipeline Generation

**Version:** v8.4
**Priority:** Medium
**Effort:** Medium-High
**Scope:** Part of CKB

### Tool: `generatePipeline`

**Purpose:** Generate CI/CD pipeline configuration based on code analysis.

```json
// Input
{
  "platform": "github-actions",     // "github-actions" | "circleci" | "gitlab" | "jenkins"
  "scope": "full",                  // "full" | "pr" | "targeted"
  "targetBranch": "main",
  "options": {
    "includeSecurityScan": true,
    "includeCoverageReport": true,
    "parallelization": "auto",
    "caching": true
  }
}

// Output
{
  "pipeline": {
    "raw": "# Generated by CKB\nname: CI\non: [push, pull_request]...",
    "format": "yaml",
    "platform": "github-actions"
  },

  "explanation": {
    "stages": [
      {
        "name": "lint",
        "rationale": "Go project uses golangci-lint (detected in Makefile)",
        "duration": "~1 min"
      },
      {
        "name": "test-unit",
        "rationale": "456 unit tests in 12 packages",
        "duration": "~3 min",
        "parallelization": "4 shards by package"
      }
    ],
    "optimizations": [
      "Dependency caching enabled (go mod cache)",
      "Parallelized unit tests across 4 runners",
      "Integration tests run only on main branch"
    ]
  },

  "codebaseAnalysis": {
    "language": "go",
    "buildTool": "go build",
    "testFramework": "go test",
    "linter": "golangci-lint",
    "packageManager": "go mod",
    "detectedPatterns": ["monorepo", "microservices"]
  },

  "customization": {
    "placeholders": [
      {"key": "{{DOCKER_REGISTRY}}", "description": "Docker registry URL"},
      {"key": "{{DEPLOY_ENV}}", "description": "Target deployment environment"}
    ],
    "optionalStages": [
      {"name": "e2e-tests", "enabled": false, "reason": "No e2e tests detected"},
      {"name": "deploy", "enabled": false, "reason": "Requires deployment config"}
    ]
  }
}
```

### Tool: `optimizePipeline`

**Purpose:** Analyze existing pipeline and suggest optimizations.

```json
// Input
{
  "pipelineFile": ".github/workflows/ci.yml",
  "recentRuns": 10    // optional: analyze recent run performance
}

// Output
{
  "currentPipeline": {
    "stages": 5,
    "avgDuration": "12 min",
    "failureRate": "8%"
  },

  "recommendations": [
    {
      "type": "parallelization",
      "impact": "high",
      "description": "Parallelize lint and test stages",
      "estimatedSavings": "4 min",
      "change": {
        "before": "stages: [lint, test, build]",
        "after": "stages: [[lint, test], build]"
      }
    },
    {
      "type": "caching",
      "impact": "medium",
      "description": "Add dependency caching for go mod",
      "estimatedSavings": "2 min",
      "change": { /* yaml snippet */ }
    },
    {
      "type": "test_selection",
      "impact": "high",
      "description": "Use CKB test impact analysis for PR builds",
      "estimatedSavings": "6 min on PRs",
      "integration": "ckb planCI --target=$PR_SHA | jq '.testStrategy.affectedTests'"
    }
  ],

  "antiPatterns": [
    {
      "issue": "Full test suite on every commit",
      "severity": "medium",
      "recommendation": "Use targeted testing for PRs, full suite for main"
    }
  ]
}
```

### Pipeline Templates

```
internal/ci/templates/
├── github-actions/
│   ├── go.yaml.tmpl
│   ├── typescript.yaml.tmpl
│   ├── python.yaml.tmpl
│   └── rust.yaml.tmpl
├── circleci/
│   └── ...
├── gitlab/
│   └── ...
└── jenkins/
    └── ...
```

### Tasks

- [ ] **3.1** Design pipeline template system with Go text/template
- [ ] **3.2** Implement codebase analysis for pipeline generation (language, tools, patterns)
- [ ] **3.3** Create base templates for GitHub Actions (Go, TypeScript, Python, Rust)
- [ ] **3.4** Create base templates for CircleCI
- [ ] **3.5** Create base templates for GitLab CI
- [ ] **3.6** Implement `generatePipeline` tool with template selection
- [ ] **3.7** Implement `optimizePipeline` tool with pattern detection
- [ ] **3.8** Add CKB integration suggestions in generated pipelines
- [ ] **3.9** Add MCP handlers
- [ ] **3.10** Add tests with snapshot comparisons

---

## Phase 4: CI Platform Integrations

**Version:** v8.5
**Priority:** Medium
**Effort:** High
**Scope:** TBD — may be separate tool or integration guides

### Decision Point

Before implementing Phase 4, evaluate:

1. **Use existing MCP servers** — CircleCI, Argo CD, Jenkins already have MCP servers. Document how to use them alongside CKB.

2. **Thin wrapper (`ckb-ci`)** — Separate CLI that:
   - Calls CKB for analysis
   - Calls platform APIs for actions
   - Single binary, separate release

3. **Full integration in CKB** — Add platform adapters directly. Risk: scope creep.

### If Proceeding: Integration Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CKB CI Module                           │
├─────────────────────────────────────────────────────────────┤
│  planCI │ diagnoseCI │ generatePipeline │ optimizePipeline  │
├─────────────────────────────────────────────────────────────┤
│                    Platform Adapters                         │
├──────────────┬──────────────┬──────────────┬────────────────┤
│ GitHub       │ CircleCI     │ GitLab       │ Jenkins        │
│ Actions      │              │ CI           │                │
│ Adapter      │ Adapter      │ Adapter      │ Adapter        │
├──────────────┴──────────────┴──────────────┴────────────────┤
│                    Platform APIs                             │
│  gh CLI        circleci CLI   gitlab CLI    jenkins CLI     │
│  REST API      REST API       REST API      REST API        │
└─────────────────────────────────────────────────────────────┘
```

### Tool: `ciStatus`

**Purpose:** Unified CI status across platforms.

```json
// Input
{
  "ref": "HEAD",                    // commit, branch, or PR
  "platform": "auto"                // "auto" | "github" | "circleci" | etc.
}

// Output
{
  "platform": "github-actions",
  "ref": "abc123",
  "runs": [
    {
      "id": "12345",
      "workflow": "CI",
      "status": "failed",
      "conclusion": "failure",
      "duration": "8m 23s",
      "failedJobs": [
        {
          "name": "test",
          "step": "Run tests",
          "error": "FAIL: TestAuthHandler (auth/handler_test.go:42)"
        }
      ],
      "url": "https://github.com/org/repo/actions/runs/12345"
    }
  ],
  "analysis": {
    "failurePattern": "test_failure",
    "ckbDiagnosis": { /* diagnoseCI output */ },
    "suggestedAction": "Fix test or update test expectation"
  }
}
```

### Tool: `triggerCI`

**Purpose:** Trigger CI runs with CKB-optimized configuration.

```json
// Input
{
  "platform": "github-actions",
  "workflow": "ci.yml",
  "ref": "feature/auth",
  "ckbOptimize": true,              // Use CKB test selection
  "inputs": {
    "test_scope": "targeted"        // Passed to workflow
  }
}

// Output
{
  "triggered": true,
  "runId": "12345",
  "url": "https://github.com/org/repo/actions/runs/12345",
  "optimization": {
    "applied": true,
    "testsSelected": 12,
    "testsSkipped": 444,
    "estimatedSavings": "10 min"
  }
}
```

### GitHub Actions Integration Example

```yaml
# .github/workflows/ci-optimized.yml
# CKB-optimized CI workflow

name: CI (CKB Optimized)
on:
  pull_request:
    branches: [main]

jobs:
  plan:
    runs-on: ubuntu-latest
    outputs:
      test_scope: ${{ steps.ckb.outputs.test_scope }}
      affected_tests: ${{ steps.ckb.outputs.affected_tests }}
    steps:
      - uses: actions/checkout@v4
      - name: CKB CI Analysis
        id: ckb
        run: |
          ckb planCI --target=HEAD~1..HEAD --format=github-output >> $GITHUB_OUTPUT

  test:
    needs: plan
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run Tests
        run: |
          if [ "${{ needs.plan.outputs.test_scope }}" == "targeted" ]; then
            echo "${{ needs.plan.outputs.affected_tests }}" | xargs go test
          else
            go test ./...
          fi
```

### Tasks (if proceeding with full integration)

- [ ] **4.1** Design platform adapter interface
- [ ] **4.2** Implement GitHub Actions adapter (gh CLI + REST API)
- [ ] **4.3** Implement CircleCI adapter
- [ ] **4.4** Implement GitLab CI adapter
- [ ] **4.5** Implement Jenkins adapter
- [ ] **4.6** Create `ciStatus` unified status tool
- [ ] **4.7** Create `triggerCI` tool with optimization
- [ ] **4.8** Create reusable GitHub Action for CKB integration
- [ ] **4.9** Create CircleCI orb for CKB integration
- [ ] **4.10** Add MCP handlers
- [ ] **4.11** Add integration tests with mocked platform APIs
- [ ] **4.12** Add documentation for each platform integration

---

## Phase 5: CI Feedback Loop (Future)

**Version:** v9.x
**Priority:** Low (future)
**Effort:** High

### Concept: Learning from CI Results

Feed CI results back into CKB to improve future recommendations.

```json
// Tool: recordCIOutcome
{
  "runId": "12345",
  "platform": "github-actions",
  "outcome": {
    "status": "failed",
    "failureType": "test_failure",
    "failedTests": ["TestAuthHandler"],
    "ckbPrediction": {
      "predictedTests": ["TestAuthHandler", "TestAuthFlow"],
      "actuallyFailed": ["TestAuthHandler"],
      "missedFailures": []
    }
  }
}

// Improves future predictions:
// - Test mapping accuracy
// - Risk scoring calibration
// - Hotspot identification
```

### Concept: CI Dashboard

Web UI showing CI intelligence insights.

```
┌─────────────────────────────────────────────────────────────┐
│  CKB CI Dashboard                                    v8.5   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Test Selection Accuracy        Risk Prediction Accuracy    │
│  ████████████░░░ 87%            ██████████░░░░░ 72%         │
│                                                             │
│  Recent Optimizations           CI Time Saved               │
│  ┌─────────────────────┐       ┌─────────────────────┐     │
│  │ PR #123: 12/456     │       │ This week: 4.2h     │     │
│  │ PR #124: 8/456      │       │ This month: 18.5h   │     │
│  │ PR #125: full suite │       │ Total: 142h         │     │
│  └─────────────────────┘       └─────────────────────┘     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation Roadmap

```
v8.3 (Q1 2026)
├── Phase 1: CI Strategy Planner
│   ├── planCI tool (in CKB)
│   ├── Test strategy synthesis
│   └── Deployment guidance
└── Phase 2: Build Error Intelligence
    ├── parseBuildErrors CI mode (in CKB)
    └── diagnoseCI tool (in CKB)

v8.4 (Q2 2026)
└── Phase 3: Pipeline Generation
    ├── generatePipeline tool (in CKB)
    ├── optimizePipeline tool (in CKB)
    └── Templates for major platforms

v8.5 (Q3 2026)
└── Phase 4: CI Platform Integrations
    ├── Decision: in CKB vs separate tool vs docs only
    ├── Platform adapters (if building)
    └── Reusable actions/orbs

v9.x (Future)
└── Phase 5: CI Feedback Loop
    ├── Outcome recording
    ├── Model improvement
    └── CI Dashboard
```

---

## Success Metrics

### Phase 1-2 (v8.3)

| Metric | Target |
|--------|--------|
| Tool calls for CI planning | 60%+ reduction |
| Test selection accuracy | 95%+ of affected tests identified |
| Build error diagnosis accuracy | 80%+ correct root cause |
| Time to CI strategy | <3s p95 |

### Phase 3-4 (v8.4-v8.5)

| Metric | Target |
|--------|--------|
| Pipeline generation adoption | 25% of users |
| CI time savings | 40%+ reduction for PR builds |
| Platform integration usage | 15% of users |
| User satisfaction | 4.5/5 rating |

### Overall

| Metric | Target |
|--------|--------|
| CI-related MCP tool usage | 30% of all tool calls |
| Enterprise adoption | 3+ enterprise customers using CI features |
| Community contributions | 10+ pipeline templates contributed |

---

## Dependencies

### Internal

- `getAffectedTests` — existing (v7.6)
- `analyzeImpact` / `prepareChange` — existing (v8.0)
- `auditRisk` — existing (v6.5)
- `getHotspots` — existing (v6.0)
- `getOwnership` — existing (v6.0)
- `summarizeDiff` / `summarizePr` — existing (v6.0)
- `parseBuildErrors` — v8.2 (in progress)
- `diagnoseFailure` — v8.2 (in progress)

### External

- `gh` CLI — GitHub Actions integration (Phase 4)
- `circleci` CLI — CircleCI integration (Phase 4)
- Platform REST APIs — for programmatic access (Phase 4)

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Test selection misses failures | Medium | High | Always provide "fall back to full" recommendation; track accuracy |
| Pipeline generation creates broken configs | Medium | Medium | Validate generated YAML; provide "dry run" option |
| Platform API changes | Low | Medium | Abstract behind adapter interface; version lock |
| Adoption requires workflow changes | Medium | Medium | Provide migration guides; make adoption incremental |
| Phase 4 scope creep | High | Medium | Make explicit decision before starting; consider docs-only approach |

---

## Related Documents

- `docs/plans/roadmap-v8.md` — v8.0 compound operations
- `docs/plans/8.2_the_rest.md` — v8.2 AI workflow optimization
- `docs/ideas.md` — Feature ideas with value/effort matrix
- `docs/featureplans/change-impact-analysis.md` — Impact analysis foundation

---

## Appendix: Research Sources

- [CircleCI MCP Server](https://circleci.com/blog/circleci-mcp-server/) — Natural language CI
- [Elastic Self-Healing PRs](https://www.elastic.co/search-labs/blog/ci-pipelines-claude-ai-agent) — Claude CI integration
- [Datadog Test Impact Analysis](https://www.datadoghq.com/blog/streamline-ci-testing-with-datadog-intelligent-test-runner/) — TIA implementation
- [Azure DevOps TIA](https://learn.microsoft.com/en-us/azure/devops/pipelines/test/test-impact-analysis) — Microsoft's approach
- [InfoWorld - 10 MCP Servers for DevOps](https://www.infoworld.com/article/4096223/10-mcp-servers-for-devops.html) — DevOps MCP landscape
- [Spacelift - AI DevOps Tools 2026](https://spacelift.io/blog/ai-devops-tools) — Market overview

# CKB — Code Knowledge Backend

**Give your AI assistant superpowers for understanding code.**

CKB is a code intelligence layer that gives AI assistants (like Claude Code) deep understanding of your codebase. Instead of grepping through files, your AI can now *navigate* code like a senior engineer would—with knowledge of who owns what, what's risky to change, and how everything connects.

## What It Does

### Symbol Navigation
Find any function, class, or variable across your entire codebase in milliseconds. Filter by type, scope to specific modules, and get full metadata.

### Call Flow & Tracing
Trace how code is reached from API endpoints, CLI commands, or jobs. See the full call chain, not just direct callers.

### Impact Analysis
Before refactoring, know exactly what breaks. Get a risk score, see all affected code paths, and identify hotspots.

### Architecture Maps
Understand how modules connect. See dependency graphs, module responsibilities, and key domain concepts.

### Ownership Intelligence
Know who owns what code—from CODEOWNERS rules and git blame with time-weighted analysis. Get reviewer suggestions for any path.

### Hotspot Detection
Identify volatile areas before they become problems. Track churn trends and get 30-day risk projections.

### Architectural Decisions
Record and query Architectural Decision Records (ADRs). Link decisions to affected modules with full-text search.

### Dead Code Detection
Get keep/investigate/remove verdicts on symbols based on usage analysis.

### Background Jobs (v6.1)
Run long operations asynchronously. Queue architecture refreshes, track progress, and cancel jobs.

### CI/CD Integration (v6.1)
Analyze PRs for risk assessment and reviewer suggestions. Detect ownership drift between CODEOWNERS and actual contributors.

### Federation (v6.2)
Query across multiple repositories. Group related repos into federations and search modules, ownership, hotspots, and decisions organization-wide.

### Daemon Mode (v6.2.1)
Always-on service with HTTP API, scheduled tasks (cron/intervals), file watching for git changes, and webhooks to Slack/PagerDuty/Discord.

### Tree-sitter Complexity (v6.2.2)
Language-agnostic complexity metrics for Go, JavaScript, TypeScript, Python, Rust, Java, and Kotlin. Computes cyclomatic and cognitive complexity to feed into hotspot risk scores.

### Contract-Aware Impact Analysis (v6.3)
Cross-repo intelligence through explicit API boundaries. Detect protobuf and OpenAPI contracts, track consumer dependencies with evidence tiers, and answer "What breaks if I change this shared API?"

## Three Ways to Use It

| Interface | Best For |
|-----------|----------|
| **Claude Code (MCP)** | AI-assisted development — Claude can query your codebase directly |
| **CLI** | Quick lookups from terminal |
| **HTTP API** | IDE plugins, CI integration, custom tooling |

## Quick Start

```bash
# 1. Build
git clone https://github.com/SimplyLiz/CodeMCP.git
cd CodeMCP
go build -o ckb ./cmd/ckb

# 2. Initialize in your project
cd /path/to/your/project
/path/to/ckb init

# 3. Generate index (Go example)
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest
scip-go --repository-root=.

# 4. Connect to Claude Code
claude mcp add --transport stdio ckb -- /path/to/ckb mcp
```

Now Claude can answer questions like:
- *"What calls the HandleRequest function?"*
- *"How is ProcessPayment reached from the API?"*
- *"What's the blast radius if I change UserService?"*
- *"Who owns the internal/api module?"*
- *"Is this legacy code still used?"*
- *"Summarize PR #123 by risk level"*

## MCP Tools (50 Available)

CKB exposes code intelligence through the Model Context Protocol:

### v5.1 — Core Navigation
| Tool | Purpose |
|------|---------|
| `searchSymbols` | Find symbols by name with filtering |
| `getSymbol` | Get symbol details |
| `findReferences` | Find all usages |
| `explainSymbol` | AI-friendly symbol explanation |
| `justifySymbol` | Keep/investigate/remove verdict |
| `getCallGraph` | Caller/callee relationships |
| `getModuleOverview` | Module statistics |
| `analyzeImpact` | Change risk analysis |
| `getStatus` | System health |
| `doctor` | Diagnostics |

### v5.2 — Discovery & Flow
| Tool | Purpose |
|------|---------|
| `traceUsage` | How is this symbol reached? |
| `listEntrypoints` | System entrypoints (API, CLI, jobs) |
| `explainFile` | File-level orientation |
| `explainPath` | Why does this path exist? |
| `summarizeDiff` | What changed, what might break? |
| `getArchitecture` | Module dependency overview |
| `getHotspots` | Volatile areas with trends |
| `listKeyConcepts` | Domain concepts in codebase |
| `recentlyRelevant` | What matters now? |

### v6.0 — Architectural Memory
| Tool | Purpose |
|------|---------|
| `getOwnership` | Who owns this code? |
| `getModuleResponsibilities` | What does this module do? |
| `recordDecision` | Create an ADR |
| `getDecisions` | Query architectural decisions |
| `annotateModule` | Add module metadata |
| `refreshArchitecture` | Rebuild architectural model |

### v6.1 — Production Ready
| Tool | Purpose |
|------|---------|
| `getJobStatus` | Query background job status |
| `listJobs` | List jobs with filters |
| `cancelJob` | Cancel queued/running job |
| `summarizePr` | PR risk analysis & reviewers |
| `getOwnershipDrift` | CODEOWNERS vs actual ownership |

### v6.2 — Federation
| Tool | Purpose |
|------|---------|
| `listFederations` | List all federations |
| `federationStatus` | Get federation status |
| `federationRepos` | List repos in federation |
| `federationSearchModules` | Cross-repo module search |
| `federationSearchOwnership` | Cross-repo ownership search |
| `federationGetHotspots` | Merged hotspots across repos |
| `federationSearchDecisions` | Cross-repo decision search |
| `federationSync` | Sync federation index |

### v6.2.1 — Daemon Mode
| Tool | Purpose |
|------|---------|
| `daemonStatus` | Daemon health and stats |
| `listSchedules` | List scheduled tasks |
| `runSchedule` | Run a schedule immediately |
| `listWebhooks` | List configured webhooks |
| `testWebhook` | Send test webhook |
| `webhookDeliveries` | Get delivery history |

### v6.3 — Contract-Aware Impact Analysis
| Tool | Purpose |
|------|---------|
| `listContracts` | List contracts in federation |
| `analyzeContractImpact` | Analyze impact of contract changes |
| `getContractDependencies` | Get contract deps for a repo |
| `suppressContractEdge` | Suppress false positive edge |
| `verifyContractEdge` | Verify an edge |
| `getContractStats` | Contract statistics |

## Documentation

📚 **[Full Documentation Wiki](https://github.com/SimplyLiz/CodeMCP/wiki)**

| Page | Description |
|------|-------------|
| **[Quick Start](https://github.com/SimplyLiz/CodeMCP/wiki/Quick-Start)** | Step-by-step installation for Windows, macOS, Linux |
| **[Prompt Cookbook](https://github.com/SimplyLiz/CodeMCP/wiki/Prompt-Cookbook)** | Real prompts for real problems — start here! |
| **[Practical Limits](https://github.com/SimplyLiz/CodeMCP/wiki/Practical-Limits)** | Accuracy notes, blind spots, validation tips |
| [User Guide](https://github.com/SimplyLiz/CodeMCP/wiki/User-Guide) | CLI commands and best practices |
| [Daemon Mode](https://github.com/SimplyLiz/CodeMCP/wiki/Daemon-Mode) | Always-on service, scheduler, webhooks (v6.2.1) |
| [Federation](https://github.com/SimplyLiz/CodeMCP/wiki/Federation) | Cross-repository queries & contracts (v6.3) |
| [CI/CD Integration](https://github.com/SimplyLiz/CodeMCP/wiki/CI-CD-Integration) | GitHub Actions, PR analysis (v6.1) |
| [API Reference](https://github.com/SimplyLiz/CodeMCP/wiki/API-Reference) | HTTP API documentation |
| [MCP Integration](https://github.com/SimplyLiz/CodeMCP/wiki/MCP-Integration) | Claude Code / AI assistant setup (50 tools) |
| [Architecture](https://github.com/SimplyLiz/CodeMCP/wiki/Architecture) | System design and components |
| [Configuration](https://github.com/SimplyLiz/CodeMCP/wiki/Configuration) | All options including MODULES.toml |
| [Performance](https://github.com/SimplyLiz/CodeMCP/wiki/Performance) | Latency targets and benchmarks |
| [Contributing](https://github.com/SimplyLiz/CodeMCP/wiki/Contributing) | Development guidelines |

## Why CKB?

| Without CKB | With CKB |
|-------------|----------|
| AI greps for patterns | AI navigates semantically |
| "I found 47 matches for Handler" | "HandleRequest is called by 3 routes via CheckoutService" |
| Guessing at impact | Knowing the blast radius with risk scores |
| Reading entire files for context | Getting exactly what's relevant |
| "Who owns this?" → search CODEOWNERS | Instant ownership with reviewer suggestions |
| "Is this safe to change?" → hope | Hotspot trends + impact analysis |

## CLI Usage

```bash
# System status
ckb status

# Search for symbols
ckb search NewServer

# Find references
ckb refs NewServer

# Get architecture overview
ckb arch

# Analyze change impact
ckb impact <symbol-id>

# Query ownership
ckb ownership internal/api/handler.go

# List architectural decisions
ckb decisions

# Refresh architectural model
ckb refresh

# Run diagnostics
ckb doctor

# Federation commands (v6.2)
ckb federation create platform --description "Our microservices"
ckb federation add platform --repo-id=api --path=/code/api
ckb federation status platform
ckb federation sync platform

# Daemon commands (v6.2.1)
ckb daemon start [--port=9120]
ckb daemon status
ckb daemon logs --follow
ckb daemon stop

# Scheduler commands
ckb daemon schedule list
ckb daemon schedule run <schedule-id>

# Webhook commands
ckb webhooks list
ckb webhooks test <webhook-id>
ckb webhooks deliveries <webhook-id>

# Contract commands (v6.3)
ckb contracts list platform
ckb contracts impact platform --repo=api --path=proto/api/v1/user.proto
ckb contracts deps platform --repo=api
ckb contracts stats platform
```

## HTTP API

```bash
# Start the HTTP server
ckb serve --port 8080

# Example API calls
curl http://localhost:8080/health
curl http://localhost:8080/status
curl http://localhost:8080/search?q=NewServer
curl http://localhost:8080/architecture
curl "http://localhost:8080/ownership?path=internal/api"
curl http://localhost:8080/hotspots
curl http://localhost:8080/decisions
```

## MCP Server (Claude Code Integration)

```bash
# Add to current project
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or add globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify configuration
claude mcp list
```

Or manually add to `.mcp.json`:
```json
{
  "mcpServers": {
    "ckb": {
      "command": "/path/to/ckb",
      "args": ["mcp"]
    }
  }
}
```

## Configuration

CKB configuration is stored in `.ckb/config.json`:

```json
{
  "version": 6,
  "backends": {
    "scip": { "enabled": true, "indexPath": "index.scip" },
    "lsp": { "enabled": true },
    "git": { "enabled": true }
  },
  "ownership": {
    "enabled": true,
    "codeownersPath": ".github/CODEOWNERS",
    "gitBlameEnabled": true,
    "timeDecayHalfLife": 90
  },
  "decisions": {
    "enabled": true,
    "directories": ["docs/decisions", "docs/adr"]
  },
  "modules": {
    "detectStrategy": "auto",
    "declarationFile": "MODULES.toml"
  },
  "daemon": {
    "port": 9120,
    "bind": "localhost",
    "auth": { "enabled": true, "token": "${CKB_DAEMON_TOKEN}" },
    "watch": { "enabled": true, "debounceMs": 5000 }
  }
}
```

See [Configuration Guide](https://github.com/SimplyLiz/CodeMCP/wiki/Configuration) for all options.

## Under the Hood

CKB orchestrates multiple code intelligence backends:

- **SCIP** — Precise, pre-indexed symbol data (fastest)
- **LSP** — Real-time language server queries
- **Git** — Blame, history, churn analysis, ownership

Results are merged intelligently and compressed for LLM context limits.

### Architectural Memory

CKB maintains persistent knowledge:
- **Module Registry** — Boundaries, responsibilities, tags (from MODULES.toml or inference)
- **Ownership Registry** — CODEOWNERS + git-blame with time decay
- **Hotspot Tracker** — Historical snapshots with trend analysis
- **Decision Log** — ADRs with full-text search

## Project Structure

```
.
├── cmd/ckb/              # CLI commands
├── internal/
│   ├── api/              # HTTP API server
│   ├── backends/
│   │   ├── git/          # Git backend (blame, history)
│   │   ├── lsp/          # LSP backend adapter
│   │   └── scip/         # SCIP backend adapter
│   ├── complexity/       # Tree-sitter complexity metrics (v6.2.2)
│   ├── config/           # Configuration management
│   ├── daemon/           # Daemon process lifecycle (v6.2.1)
│   ├── decisions/        # ADR parsing and storage
│   ├── federation/       # Cross-repo federation (v6.2)
│   ├── hotspots/         # Hotspot tracking and trends
│   ├── identity/         # Symbol identity and aliasing
│   ├── jobs/             # Background job queue (v6.1)
│   ├── mcp/              # MCP server for Claude Code
│   ├── modules/          # Module detection
│   ├── ownership/        # Ownership tracking
│   ├── query/            # Query engine
│   ├── responsibilities/ # Module responsibility extraction
│   ├── scheduler/        # Cron/interval task scheduler (v6.2.1)
│   ├── storage/          # SQLite storage layer
│   ├── watcher/          # File system watcher (v6.2.1)
│   └── webhooks/         # Webhook delivery (v6.2.1)
└── .ckb/                 # CKB data directory
    ├── config.json       # Configuration
    └── ckb.db            # SQLite database
```

## Requirements

- Go 1.21+
- Git (for repository operations)
- Optional: gopls (for LSP support)
- Optional: scip-go (for SCIP indexing)

## Development

```bash
# Run tests
go test ./...

# Build
go build -o ckb ./cmd/ckb

# Regenerate SCIP index
scip-go --repository-root=.
```

## License

Free for personal use. Commercial/enterprise use requires a license. See [LICENSE](LICENSE) for details.

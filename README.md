# CKB — Code Knowledge Backend

[![npm version](https://img.shields.io/npm/v/@tastehub/ckb.svg)](https://www.npmjs.com/package/@tastehub/ckb)
[![Documentation](https://img.shields.io/badge/docs-wiki-blue.svg)](https://github.com/SimplyLiz/CodeMCP/wiki)

**The missing link between your codebase and AI assistants.**

CKB gives AI assistants deep understanding of your code. Instead of grepping through files, your AI can now *navigate* code like a senior engineer—with knowledge of who owns what, what's risky to change, and how everything connects.

> CKB analyzes and explains your code but never modifies it. Think of it as a librarian who knows everything about the books but never rewrites them.

## The Problem

### AI Assistants Are Blind to Code Structure

When you ask an AI "what calls this function?", it typically:
1. Searches for text patterns (error-prone)
2. Reads random files hoping to find context (inefficient)
3. Gives up and asks you to provide more context (frustrating)

### Existing Tools Don't Talk to Each Other

Your codebase has valuable intelligence scattered across SCIP indexes, language servers, git history, and CODEOWNERS files. Each speaks a different language. None are optimized for AI consumption.

### Context Windows Are Limited

Even with 100K+ token context, you can't dump your entire codebase into an LLM. You need relevant information only, properly compressed, with smart truncation.

## What CKB Gives You

```
You: "What's the impact of changing UserService.authenticate()?"

CKB provides:
├── Symbol details (signature, visibility, location)
├── 12 direct callers across 4 modules
├── Risk score: HIGH (public API, many dependents)
├── Affected modules: auth, api, admin, tests
├── Code owners: @security-team, @api-team
└── Suggested drilldowns for deeper analysis
```

```
You: "Show me the architecture of this codebase"

CKB provides:
├── Module dependency graph
├── Key symbols per module
├── Module responsibilities and ownership
├── Import/export relationships
└── Compressed to fit LLM context
```

```
You: "Is it safe to rename this function?"

CKB provides:
├── All references (not just text matches)
├── Cross-module dependencies
├── Test coverage of affected code
├── Hotspot risk assessment
└── Breaking change warnings
```

## Quick Start

### Option 1: npm (Recommended)

```bash
# Install globally
npm install -g @tastehub/ckb

# Or run directly with npx (no install needed)
npx @tastehub/ckb init
```

### Option 2: Build from Source

```bash
git clone https://github.com/SimplyLiz/CodeMCP.git
cd CodeMCP
go build -o ckb ./cmd/ckb
```

### Setup

```bash
# 1. Initialize in your project
cd /path/to/your/project
ckb init   # or: npx @tastehub/ckb init

# 2. Generate SCIP index (optional but recommended)
ckb index  # auto-detects language and runs appropriate indexer

# 3. Connect to Claude Code
ckb setup  # creates .mcp.json automatically

# Or manually:
claude mcp add --transport stdio ckb -- npx @tastehub/ckb mcp
```

Now Claude can answer questions like:
- *"What calls the HandleRequest function?"*
- *"How is ProcessPayment reached from the API?"*
- *"What's the blast radius if I change UserService?"*
- *"Who owns the internal/api module?"*
- *"Is this legacy code still used?"*

## Why CKB?

| Without CKB | With CKB |
|-------------|----------|
| AI greps for patterns | AI navigates semantically |
| "I found 47 matches for Handler" | "HandleRequest is called by 3 routes via CheckoutService" |
| Guessing at impact | Knowing the blast radius with risk scores |
| Reading entire files for context | Getting exactly what's relevant |
| "Who owns this?" → search CODEOWNERS | Instant ownership with reviewer suggestions |
| "Is this safe to change?" → hope | Hotspot trends + impact analysis |

## Three Ways to Use It

| Interface | Best For |
|-----------|----------|
| **[MCP](https://github.com/SimplyLiz/CodeMCP/wiki/MCP-Integration)** | AI-assisted development — Claude, Cursor, Windsurf, VS Code, OpenCode |
| **[CLI](https://github.com/SimplyLiz/CodeMCP/wiki/User-Guide)** | Quick lookups from terminal, scripting |
| **[HTTP API](https://github.com/SimplyLiz/CodeMCP/wiki/API-Reference)** | IDE plugins, CI integration, custom tooling |

## Features

### Core Intelligence
- **Symbol Navigation** — Find any function, class, or variable in milliseconds
- **Call Flow & Tracing** — Trace how code is reached from API endpoints, CLI commands, or jobs
- **Impact Analysis** — Know exactly what breaks before refactoring, with risk scores
- **Architecture Maps** — Module dependency graphs, responsibilities, domain concepts
- **Dead Code Detection** — Keep/investigate/remove verdicts based on usage analysis

### Ownership & Risk
- **Ownership Intelligence** — CODEOWNERS + git blame with time-weighted analysis
- **Hotspot Detection** — Track churn trends, get 30-day risk projections
- **Architectural Decisions** — Record and query ADRs with full-text search

### Advanced Capabilities
- **Federation** — Query across multiple repos organization-wide
- **Daemon Mode** — Always-on service with HTTP API, scheduled tasks, webhooks
- **Contract Analysis** — Protobuf and OpenAPI discovery with cross-repo impact
- **Runtime Observability** — OpenTelemetry integration for dead code detection
- **Developer Intelligence** — Symbol origins, co-change coupling, risk audit

### Fast Indexing
- **Zero-Index Operation** — Tree-sitter fallback works without SCIP index
- **Incremental Updates** — O(changed files) instead of full reindex (Go)
- **Smart Caching** — Skip-if-fresh, watch mode, transitive invalidation

### Remote Index Server
- **Index Serving** — Serve symbol indexes over HTTP for federation
- **Index Upload** — Push SCIP indexes with compression (gzip/zstd)
- **Delta Updates** — Upload only changed files
- **API Key Auth** — Scoped keys with rate limiting

📋 **[Full Changelog](https://github.com/SimplyLiz/CodeMCP/blob/main/CHANGELOG.md)** — Detailed version history from v5.1 to current

## MCP Tools (64 Available)

CKB exposes code intelligence through the Model Context Protocol:

<details>
<summary><strong>v5.1 — Core Navigation</strong></summary>

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

</details>

<details>
<summary><strong>v5.2 — Discovery & Flow</strong></summary>

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

</details>

<details>
<summary><strong>v6.0 — Architectural Memory</strong></summary>

| Tool | Purpose |
|------|---------|
| `getOwnership` | Who owns this code? |
| `getModuleResponsibilities` | What does this module do? |
| `recordDecision` | Create an ADR |
| `getDecisions` | Query architectural decisions |
| `annotateModule` | Add module metadata |
| `refreshArchitecture` | Rebuild architectural model |

</details>

<details>
<summary><strong>v6.1 — Production Ready</strong></summary>

| Tool | Purpose |
|------|---------|
| `getJobStatus` | Query background job status |
| `listJobs` | List jobs with filters |
| `cancelJob` | Cancel queued/running job |
| `summarizePr` | PR risk analysis & reviewers |
| `getOwnershipDrift` | CODEOWNERS vs actual ownership |

</details>

<details>
<summary><strong>v6.2+ — Federation, Daemon, Contracts, Telemetry, Intelligence</strong></summary>

| Tool | Purpose |
|------|---------|
| `listFederations` | List all federations |
| `federationStatus` | Get federation status |
| `federationSearchModules` | Cross-repo module search |
| `federationSearchOwnership` | Cross-repo ownership search |
| `federationGetHotspots` | Merged hotspots across repos |
| `daemonStatus` | Daemon health and stats |
| `listSchedules` | List scheduled tasks |
| `listWebhooks` | List configured webhooks |
| `getFileComplexity` | Cyclomatic/cognitive complexity |
| `listContracts` | List contracts in federation |
| `analyzeContractImpact` | Contract change impact |
| `getTelemetryStatus` | Coverage metrics and sync status |
| `getObservedUsage` | Observed usage for a symbol |
| `findDeadCodeCandidates` | Zero runtime call detection |
| `explainOrigin` | Why does this code exist? |
| `analyzeCoupling` | Co-change analysis |
| `exportForLLM` | LLM-friendly export |
| `auditRisk` | Multi-signal risk audit |

</details>

<details>
<summary><strong>v7.3 — Doc-Symbol Linking</strong></summary>

| Tool | Purpose |
|------|---------|
| `indexDocs` | Scan and index documentation |
| `getDocsForSymbol` | Find docs referencing a symbol |
| `getSymbolsInDoc` | List symbols in a document |
| `getDocsForModule` | Find docs linked to a module |
| `checkDocStaleness` | Check for stale references |
| `getDocCoverage` | Documentation coverage stats |

</details>

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

# Run diagnostics
ckb doctor

# Check tier-specific requirements
ckb doctor --tier enhanced

# Start MCP server for AI assistants
ckb mcp
```

<details>
<summary><strong>More CLI commands</strong></summary>

```bash
# Federation (v6.2)
ckb federation create platform --description "Our microservices"
ckb federation add platform --repo-id=api --path=/code/api
ckb federation status platform
ckb federation sync platform

# Daemon (v6.2.1)
ckb daemon start [--port=9120]
ckb daemon status
ckb daemon logs --follow
ckb daemon stop

# Contracts (v6.3)
ckb contracts list platform
ckb contracts impact platform --repo=api --path=proto/api/v1/user.proto
ckb contracts deps platform --repo=api

# Telemetry (v6.4)
ckb telemetry status
ckb telemetry usage --symbol="internal/api/handler.go:HandleRequest"
ckb dead-code --min-confidence=0.7

# Developer Intelligence (v6.5)
ckb explain internal/api/handler.go:42
ckb coupling internal/query/engine.go --min-correlation=0.5
ckb export --min-complexity=10 --max-symbols=200
ckb audit --min-score=60 --quick-wins
```

</details>

## HTTP API

```bash
# Start the HTTP server
ckb serve --port 8080

# Example calls
curl http://localhost:8080/health
curl http://localhost:8080/status
curl "http://localhost:8080/search?q=NewServer"
curl http://localhost:8080/architecture
curl "http://localhost:8080/ownership?path=internal/api"
curl http://localhost:8080/hotspots

# Index Server Mode (v7.3) - serve indexes to remote clients
ckb serve --port 8080 --index-server --index-config config.toml

# Index server endpoints
curl http://localhost:8080/index/repos
curl http://localhost:8080/index/repos/company%2Fcore-lib/meta
curl "http://localhost:8080/index/repos/company%2Fcore-lib/symbols?limit=100"
curl "http://localhost:8080/index/repos/company%2Fcore-lib/search/symbols?q=Handler"

# Upload endpoints (with compression + auth)
curl -X POST http://localhost:8080/index/repos \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ckb_xxx" \
  -d '{"id":"my-org/my-repo","name":"My Repo"}'

gzip -c index.scip | curl -X POST http://localhost:8080/index/repos/my-org%2Fmy-repo/upload \
  -H "Content-Encoding: gzip" \
  -H "Authorization: Bearer ckb_xxx" \
  --data-binary @-

# Token management (index server admin)
ckb token create --name "ci-upload" --scope upload    # Create API key
ckb token list                                         # List all tokens
ckb token revoke ckb_xxx                              # Revoke a token
ckb token rotate ckb_xxx                              # Rotate (new secret, same ID)
```

## MCP Integration

CKB works with any MCP-compatible AI coding tool.

<details>
<summary><strong>Claude Code</strong></summary>

```bash
# Auto-configure for current project
npx @tastehub/ckb setup

# Or add globally for all projects
npx @tastehub/ckb setup --global
```

Or manually add to `.mcp.json`:
```json
{
  "mcpServers": {
    "ckb": {
      "command": "npx",
      "args": ["@tastehub/ckb", "mcp"]
    }
  }
}
```

</details>

<details>
<summary><strong>Cursor</strong></summary>

Add to `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):
```json
{
  "mcpServers": {
    "ckb": {
      "command": "npx",
      "args": ["@tastehub/ckb", "mcp"]
    }
  }
}
```

</details>

<details>
<summary><strong>Windsurf</strong></summary>

Add to `~/.codeium/windsurf/mcp_config.json`:
```json
{
  "mcpServers": {
    "ckb": {
      "command": "npx",
      "args": ["@tastehub/ckb", "mcp"]
    }
  }
}
```

</details>

<details>
<summary><strong>VS Code</strong></summary>

Add to your VS Code `settings.json`:
```json
{
  "mcp": {
    "servers": {
      "ckb": {
        "type": "stdio",
        "command": "npx",
        "args": ["@tastehub/ckb", "mcp"]
      }
    }
  }
}
```

</details>

<details>
<summary><strong>OpenCode</strong></summary>

Add to `opencode.json` in project root:
```json
{
  "mcp": {
    "ckb": {
      "type": "local",
      "command": ["npx", "@tastehub/ckb", "mcp"],
      "enabled": true
    }
  }
}
```

</details>

<details>
<summary><strong>Claude Desktop</strong></summary>

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):
```json
{
  "mcpServers": {
    "ckb": {
      "command": "npx",
      "args": ["@tastehub/ckb", "mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

</details>

<details>
<summary><strong>Windows</strong></summary>

Use `cmd /c` wrapper in any config above:
```json
{
  "mcpServers": {
    "ckb": {
      "command": "cmd",
      "args": ["/c", "npx", "@tastehub/ckb", "mcp"]
    }
  }
}
```

</details>

## Under the Hood

CKB orchestrates multiple code intelligence backends:

- **SCIP** — Precise, pre-indexed symbol data (fastest)
- **LSP** — Real-time language server queries
- **Git** — Blame, history, churn analysis, ownership

Results are merged intelligently and compressed for LLM context limits.

Persistent knowledge survives across sessions:
- **Module Registry** — Boundaries, responsibilities, tags
- **Ownership Registry** — CODEOWNERS + git-blame with time decay
- **Hotspot Tracker** — Historical snapshots with trend analysis
- **Decision Log** — ADRs with full-text search

## Who Should Use CKB?

- **Developers using AI assistants** — Give your AI tools superpowers
- **Teams with large codebases** — Navigate complexity efficiently
- **Anyone doing refactoring** — Understand impact before changing
- **Code reviewers** — See the full picture of changes
- **Tech leads** — Track architectural health over time

## Documentation

See the **[Full Documentation Wiki](https://github.com/SimplyLiz/CodeMCP/wiki)** for:

- [Quick Start](https://github.com/SimplyLiz/CodeMCP/wiki/Quick-Start) — Step-by-step installation
- [Prompt Cookbook](https://github.com/SimplyLiz/CodeMCP/wiki/Prompt-Cookbook) — Real prompts for real problems
- [Language Support](https://github.com/SimplyLiz/CodeMCP/wiki/Language-Support) — SCIP indexers and support tiers
- [Practical Limits](https://github.com/SimplyLiz/CodeMCP/wiki/Practical-Limits) — Accuracy notes, blind spots
- [User Guide](https://github.com/SimplyLiz/CodeMCP/wiki/User-Guide) — CLI commands and best practices
- [Incremental Indexing](https://github.com/SimplyLiz/CodeMCP/wiki/Incremental-Indexing) — Fast index updates for Go projects
- [Doc-Symbol Linking](https://github.com/SimplyLiz/CodeMCP/wiki/Doc-Symbol-Linking) — Symbol detection in docs, staleness checking
- [MCP Integration](https://github.com/SimplyLiz/CodeMCP/wiki/MCP-Integration) — Claude Code setup, 64 tools
- [API Reference](https://github.com/SimplyLiz/CodeMCP/wiki/API-Reference) — HTTP API documentation
- [Configuration](https://github.com/SimplyLiz/CodeMCP/wiki/Configuration) — All options including MODULES.toml
- [Telemetry](https://github.com/SimplyLiz/CodeMCP/wiki/Telemetry) — Runtime observability, dead code detection
- [Federation](https://github.com/SimplyLiz/CodeMCP/wiki/Federation) — Cross-repository queries
- [CI/CD Integration](https://github.com/SimplyLiz/CodeMCP/wiki/CI-CD-Integration) — GitHub Actions, PR analysis

## Requirements

**Using npm (recommended):**
- Node.js 16+
- Git

**Building from source:**
- Go 1.21+
- Git

**Optional (for enhanced analysis):**
- SCIP indexer for your language (scip-go, scip-typescript, etc.) — run `ckb index` to auto-install

## License

Free for personal use. Commercial/enterprise use requires a license. See [LICENSE](LICENSE) for details.

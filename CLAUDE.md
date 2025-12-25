# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build
go build -o ckb ./cmd/ckb

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/query/...

# Run a single test
go test ./internal/query/... -run TestSearchSymbols

# Run tests with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Lint (if golangci-lint installed)
golangci-lint run
```

## Running the Application

```bash
# Initialize CKB in a project directory
./ckb init

# Generate SCIP index (auto-detects language)
./ckb index

# Check system status
./ckb status

# Run diagnostics
./ckb doctor

# Start HTTP API server
./ckb serve --port 8080

# Start MCP server (for AI tool integration)
./ckb mcp

# Auto-configure AI tool integration (interactive)
./ckb setup

# Configure for specific AI tools
./ckb setup --tool=claude-code
./ckb setup --tool=cursor
./ckb setup --tool=vscode
```

## npm Distribution (v7.0)

CKB is also available via npm:

```bash
# Install globally
npm install -g @tastehub/ckb

# Or run directly with npx
npx @tastehub/ckb init
npx @tastehub/ckb setup
```

## MCP Integration

CKB provides 58 code intelligence tools via MCP. Supports Claude Code, Cursor, Windsurf, VS Code, OpenCode, and Claude Desktop.

```bash
# Auto-configure (interactive)
ckb setup

# Configure for specific tool
ckb setup --tool=claude-code
ckb setup --tool=cursor --global

# Or manually add to .mcp.json
claude mcp add ckb -- npx @tastehub/ckb mcp
```

### Key MCP Tools

**Navigation:** `searchSymbols`, `getSymbol`, `findReferences`, `getCallGraph`, `traceUsage`, `listEntrypoints`

**Understanding:** `explainSymbol`, `explainFile`, `explainPath`, `getArchitecture`, `listKeyConcepts`

**Impact & Risk:** `analyzeImpact`, `justifySymbol`, `getHotspots`, `summarizeDiff`, `summarizePr`

**Ownership:** `getOwnership`, `getModuleResponsibilities`, `getOwnershipDrift`

**Decisions:** `recordDecision`, `getDecisions`, `annotateModule`

**Federation (v6.2+):** `listFederations`, `federationSearchModules`, `federationGetHotspots`

**Contracts (v6.3):** `listContracts`, `analyzeContractImpact`, `getContractDependencies`

**Telemetry (v6.4):** `getTelemetryStatus`, `getObservedUsage`, `findDeadCodeCandidates`

**Intelligence (v6.5):** `explainOrigin`, `analyzeCoupling`, `exportForLLM`, `auditRisk`

## Architecture Overview

CKB is a code intelligence orchestration layer with three interfaces (CLI, HTTP API, MCP) that all flow through a central query engine.

### Layer Structure

```
Interfaces (cmd/ckb/, internal/api/, internal/mcp/)
    ↓
Query Engine (internal/query/)
    ↓
Backend Orchestrator (internal/backends/orchestrator.go)
    ↓
Backends: SCIP, LSP, Git (internal/backends/{scip,lsp,git}/)
    ↓
Storage Layer (internal/storage/) - SQLite for caching and symbol mappings
```

### Key Packages

- **internal/query/**: Central query engine. `Engine` is the main entry point for all operations.
- **internal/backends/**: Backend abstraction. `Orchestrator` routes to SCIP/LSP/Git based on `QueryPolicy`.
- **internal/identity/**: Stable symbol IDs that survive refactoring. Uses fingerprinting and alias chains.
- **internal/compression/**: Response budget enforcement for LLM-optimized output.
- **internal/impact/**: Change impact analysis with visibility detection and risk scoring.
- **internal/mcp/**: Model Context Protocol server (58 tools).
- **internal/ownership/**: CODEOWNERS parsing + git-blame analysis with time decay.
- **internal/responsibilities/**: Module responsibility extraction from READMEs and code.
- **internal/hotspots/**: Churn tracking with trend analysis and projections.
- **internal/decisions/**: Architectural Decision Records (ADRs) with full-text search.
- **internal/federation/**: Cross-repository queries and unified visibility.
- **internal/daemon/**: Background service with scheduler, file watcher, webhooks.
- **internal/complexity/**: Tree-sitter based cyclomatic/cognitive complexity.
- **internal/telemetry/**: OpenTelemetry integration for runtime observability.
- **internal/audit/**: Multi-factor risk scoring (8 weighted factors).
- **internal/coupling/**: Co-change analysis from git history.

### Data Flow

1. Request arrives via CLI/HTTP/MCP
2. Query engine checks cache (query cache → view cache → negative cache)
3. Orchestrator routes to backends via "backend ladder" (SCIP first, then LSP, then Git)
4. Results merged using configured strategy (prefer-first or union)
5. Compression applied to fit response budgets
6. Drilldown suggestions generated for truncated results

### Configuration

Runtime config stored in `.ckb/config.json`. Key settings:
- `queryPolicy.backendLadder`: Backend priority order
- `queryPolicy.mergeStrategy`: "prefer-first" or "union"
- `budget.*`: Response size limits for LLM optimization
- `cache.*`: TTL settings for each cache tier

### Symbol Identity System

Symbols get stable IDs: `ckb:<repo>:sym:<fingerprint>`

Fingerprint = hash(container + name + kind + signature)

When symbols are renamed, alias chains redirect old IDs to new ones (max depth: 3).

### Language Support

CKB works at different quality levels depending on available tooling:

- **Enhanced tier** (SCIP index): Go, TypeScript, Python, Rust, Java, Kotlin, C++, Dart, Ruby, C#
- **Basic tier** (LSP only): Any language with a language server
- **Minimal tier** (heuristics): Git-based features work for all languages

Run `ckb index` to auto-detect and run the appropriate SCIP indexer.

### Keeping Your Index Fresh (v7.5+)

CKB offers multiple ways to keep your index up-to-date:

```bash
# Manual (default)
ckb index                    # Run when needed

# Watch mode (CLI)
ckb index --watch            # Watch and auto-reindex
ckb index --watch --watch-interval 1m

# MCP watch mode
ckb mcp --watch              # Auto-reindex during IDE session
ckb mcp --watch --watch-interval 15s

# Background daemon
ckb daemon start             # Watches all registered repos

# Trigger from CI (v7.5+)
curl -X POST http://localhost:9120/api/v1/refresh
curl -X POST http://localhost:9120/api/v1/refresh -d '{"full":true}'

# Check freshness
ckb status                   # Shows commits behind, index age
```

The `getStatus` MCP tool now includes index freshness info (`index.fresh`, `index.commitsBehind`, `index.indexAge`).

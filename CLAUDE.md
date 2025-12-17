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
```

## Running the Application

```bash
# Initialize CKB in a project directory
./ckb init

# Check system status
./ckb status

# Run diagnostics
./ckb doctor

# Start HTTP API server
./ckb serve --port 8080

# Start MCP server (for Claude Code integration)
./ckb mcp
```

## MCP Integration with Claude Code

CKB provides code intelligence tools via MCP. To enable in Claude Code:

```bash
# Add CKB MCP server to your project (creates .mcp.json)
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or add globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify it's configured
claude mcp list
```

Once configured, Claude Code will have access to these tools:
- `searchSymbols` - Find symbols by name
- `getSymbol` - Get symbol details
- `findReferences` - Find all usages
- `getArchitecture` - Codebase structure
- `analyzeImpact` - Change impact analysis
- `explainSymbol` - AI-friendly symbol overview
- `justifySymbol` - Keep/investigate/remove verdict
- `getCallGraph` - Caller/callee relationships
- `getModuleOverview` - Module statistics

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

- **internal/query/**: Central query engine that coordinates all operations. `Engine` is the main entry point.
- **internal/backends/**: Backend abstraction layer. `Orchestrator` routes queries to SCIP/LSP/Git backends based on `QueryPolicy`.
- **internal/identity/**: Stable symbol ID generation that survives refactoring. Uses fingerprinting and alias chains.
- **internal/compression/**: Response budget enforcement for LLM-optimized output. Handles truncation and drilldown suggestions.
- **internal/impact/**: Change impact analysis with visibility detection and risk scoring.
- **internal/mcp/**: Model Context Protocol server implementation for AI assistant integration.

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

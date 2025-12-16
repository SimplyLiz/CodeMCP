# CKB User Guide

## Introduction

CKB (Code Knowledge Backend) is a tool that helps AI assistants understand your codebase. It provides a unified interface to query code intelligence from multiple backends (SCIP, LSP, Git) and returns optimized responses designed for LLM context windows.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Git repository
- (Optional) SCIP index for your project
- (Optional) Language server for your project's language

### Installation

```bash
# Build from source
git clone https://github.com/SimplyLiz/CodeMCP.git
cd CodeMCP
go build -o ckb ./cmd/ckb

# Add to PATH or move to /usr/local/bin
mv ckb /usr/local/bin/
```

### Initialize Your Project

Navigate to your project root and initialize CKB:

```bash
cd /path/to/your/project
ckb init
```

This creates a `.ckb/` directory with:
- `config.json` - Configuration file
- `ckb.db` - SQLite database for caching and symbol mappings

### Verify Setup

```bash
# Check system status
ckb status

# Run diagnostics
ckb doctor
```

## Core Concepts

### Stable Symbol IDs

CKB assigns stable IDs to symbols that survive refactoring:

```
ckb:myrepo:sym:a1b2c3d4e5f6...
```

When a symbol is renamed, CKB creates an alias from the old ID to the new one, so references using the old ID continue to work.

### Repository State

CKB tracks repository state using:
- HEAD commit hash
- Staged changes hash
- Working tree changes hash
- Untracked files hash

This composite state ID is used for cache invalidation.

### Backend Ladder

CKB queries backends in priority order:
1. **SCIP** - Pre-computed index (fastest, most accurate)
2. **LSP** - Language server (real-time, may be slower)
3. **Git** - Fallback for basic operations

## CLI Commands

### `ckb init`

Initialize CKB in the current directory.

```bash
ckb init
```

Creates `.ckb/config.json` with default settings.

### `ckb status`

Show system status including:
- Repository state
- Backend availability
- Cache statistics
- Index freshness

```bash
ckb status
```

### `ckb doctor`

Run diagnostic checks:

```bash
ckb doctor
```

Checks:
- Configuration validity
- Backend availability
- SCIP index presence and freshness
- Database integrity

### `ckb search`

Search for symbols:

```bash
# Basic search
ckb search "myFunction"

# Filter by kind
ckb search "User" --kinds=class,interface

# Limit results
ckb search "handle" --limit=10

# Search within module
ckb search "process" --scope=internal/api
```

### `ckb refs`

Find references to a symbol:

```bash
# Find all references
ckb refs "symbol-id"

# Limit scope
ckb refs "symbol-id" --scope=internal/

# Include test files
ckb refs "symbol-id" --include-tests
```

### `ckb impact`

Analyze impact of changing a symbol:

```bash
# Basic impact analysis
ckb impact "symbol-id"

# Set analysis depth
ckb impact "symbol-id" --depth=3
```

### `ckb serve`

Start the HTTP API server:

```bash
# Default (localhost:8080)
ckb serve

# Custom port
ckb serve --port 8081

# Bind to all interfaces
ckb serve --host 0.0.0.0
```

### `ckb symbol`

Get detailed information about a specific symbol:

```bash
# Get symbol by stable ID
ckb symbol "ckb:myrepo:sym:abc123"

# Output as human-readable format
ckb symbol "ckb:myrepo:sym:abc123" --format=human

# Use full repo state (includes dirty working tree)
ckb symbol "ckb:myrepo:sym:abc123" --repo-state-mode=full
```

### `ckb arch`

Get a high-level architecture view of the codebase:

```bash
# Basic architecture overview
ckb arch

# Increase dependency depth
ckb arch --depth=3

# Include external dependencies
ckb arch --include-external-deps

# Force refresh (bypass cache)
ckb arch --refresh
```

### `ckb mcp`

Start the MCP (Model Context Protocol) server for AI assistant integration:

```bash
# Start MCP server (stdio mode)
ckb mcp

# Start with verbose logging
ckb mcp --verbose
```

See [MCP Integration](mcp-integration.md) for setup with Claude Desktop.

### `ckb diag`

Create a diagnostic bundle for troubleshooting:

```bash
# Create diagnostic bundle
ckb diag --out ckb-diagnostic.zip

# Anonymize symbol names and paths
ckb diag --out ckb-diagnostic.zip --anonymize
```

The bundle includes sanitized configuration, doctor output, backend status, and system information. It excludes source code and sensitive credentials.

## Working with Responses

### Response Structure

All CKB responses include:

```json
{
  "data": { ... },
  "provenance": {
    "backends": ["scip", "lsp"],
    "repoStateId": "abc123...",
    "cachedAt": "2025-12-16T12:00:00Z"
  },
  "warnings": [],
  "drilldowns": []
}
```

### Drilldowns

When results are truncated, CKB suggests follow-up queries:

```json
{
  "drilldowns": [
    {
      "label": "Explore top module: internal/api",
      "query": "getModuleOverview internal/api",
      "relevanceScore": 0.9
    }
  ]
}
```

### Warnings

CKB reports limitations in analysis:

```json
{
  "warnings": [
    {
      "severity": "warning",
      "text": "SCIP index is 3 commits behind HEAD"
    }
  ]
}
```

## Best Practices

### Keep SCIP Index Fresh

Regenerate your SCIP index after significant changes:

```bash
# For Go projects
scip-go

# For TypeScript projects
scip-typescript index
```

### Use Scoped Queries

For large codebases, scope queries to specific modules:

```bash
ckb search "handler" --scope=internal/api
```

### Monitor Cache

Check cache statistics to ensure efficient operation:

```bash
ckb status
```

### Run Doctor Regularly

```bash
ckb doctor
```

Fix any issues before they impact queries.

## Troubleshooting

### "Backend unavailable" errors

1. Check if the backend is installed
2. Run `ckb doctor` to diagnose
3. Check backend-specific configuration in `.ckb/config.json`

### Stale results

1. Check repository state with `ckb status`
2. Regenerate SCIP index if stale
3. Clear cache: `ckb cache clear`

### Slow queries

1. Ensure SCIP index exists (fastest backend)
2. Reduce query scope
3. Lower result limits
4. Check if LSP server is responsive

## Next Steps

- [API Reference](api-reference.md) - Detailed API documentation
- [Configuration Guide](configuration.md) - All configuration options
- [Architecture Overview](architecture.md) - How CKB works internally

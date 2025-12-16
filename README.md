# CKB (Code Knowledge Backend)

A language-agnostic codebase comprehension layer that orchestrates multiple code intelligence backends (SCIP, LSP, Git) and provides semantically compressed, LLM-optimized views.

> **New to CKB?** Check out the **[Quick Start Guide](QUICKSTART.md)** for step-by-step installation instructions for Windows, macOS, and Linux.

## Overview

CKB acts as a unified interface for code intelligence, aggregating data from multiple sources:

- **SCIP** - Source Code Intelligence Protocol indexes for precise, pre-computed symbol information
- **LSP** - Language Server Protocol for on-demand queries
- **Git** - Repository state, blame, and history information

## Features

- **Multi-Backend Orchestration**: Query multiple backends and merge results intelligently
- **Symbol Search**: Find symbols by name across the codebase
- **Find References**: Locate all usages of a symbol
- **Impact Analysis**: Understand the blast radius of changes
- **Architecture Views**: Get high-level codebase structure
- **Claude Code Integration**: Native MCP server for Claude Code
- **Cross-Repo Support**: Query multiple SCIP indexes at once with repo-qualified symbol IDs

## Installation

For detailed installation instructions with copy-paste commands, see the **[Quick Start Guide](QUICKSTART.md)**.

**Quick install (macOS/Linux):**

```bash
# Clone and build
git clone https://github.com/anthropics/ckb.git
cd ckb
go build -o ckb ./cmd/ckb

# Optional: Install globally
sudo cp ckb /usr/local/bin/
```

## Quick Start

### Initialize CKB

```bash
# Initialize CKB in your project
./ckb init

# Generate SCIP index (requires scip-go for Go projects)
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest
scip-go --repository-root=.
```

### CLI Commands

```bash
# Check system status
./ckb status

# Search for symbols
./ckb search NewServer

# Find references to a symbol
./ckb refs NewServer

# Get symbol details
./ckb symbol "scip-go gomod pkg version \`pkg/path\`/Symbol()."

# Get architecture overview
./ckb arch

# Analyze change impact
./ckb impact <symbol-id>

# Run diagnostics
./ckb doctor
```

### HTTP API

```bash
# Start the HTTP server
./ckb serve --port 8080

# Example API calls
curl http://localhost:8080/health
curl http://localhost:8080/search?q=NewServer
curl http://localhost:8080/status
curl http://localhost:8080/architecture
```

### MCP Server (Claude Code Integration)

```bash
# Start MCP server (reads from stdin, writes to stdout)
./ckb mcp
```

Configure in Claude Code's MCP settings:
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
  "version": 5,
  "repoRoot": ".",
  "backends": {
    "scip": {
      "enabled": true,
      "indexPath": ".scip/index.scip",
      "indexes": [
        {
          "name": "api",
          "repoRoot": "../api-service",
          "indexPath": ".scip/index.scip"
        },
        {
          "name": "frontend",
          "repoRoot": "../web-app",
          "indexPath": ".scip/index.scip"
        }
      ]
    },
    "lsp": {
      "enabled": true,
      "servers": {
        "go": {
          "command": "/path/to/gopls",
          "args": []
        }
      }
    },
    "git": {
      "enabled": true
    }
  }
}
```

**Repo-qualified symbol IDs**: When multiple SCIP indexes are configured, returned symbol IDs are prefixed with the repository name (e.g., `frontend::scip-go gomod pkg main main.Main()`). File paths are similarly prefixed (`frontend:src/main.ts`) so results stay unambiguous across repositories.

## Project Structure

```
.
├── cmd/ckb/           # CLI commands
├── internal/
│   ├── api/           # HTTP API server
│   ├── backends/
│   │   ├── git/       # Git backend (blame, history)
│   │   ├── lsp/       # LSP backend adapter
│   │   └── scip/      # SCIP backend adapter
│   ├── config/        # Configuration management
│   ├── identity/      # Symbol identity and aliasing
│   ├── mcp/           # MCP server for Claude Code
│   ├── query/         # Query engine
│   └── storage/       # SQLite storage layer
└── .ckb/              # CKB data directory
    ├── config.json    # Configuration
    └── ckb.db         # SQLite database
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/status` | GET | System status |
| `/search` | GET | Search symbols (`?q=query`) |
| `/symbol/:id` | GET | Get symbol by ID |
| `/refs/:id` | GET | Find references |
| `/architecture` | GET | Architecture overview |
| `/impact/:id` | GET | Impact analysis |
| `/doctor` | GET | Run diagnostics |
| `/openapi.json` | GET | OpenAPI specification |

## MCP Tools

| Tool | Description |
|------|-------------|
| `searchSymbols` | Search for symbols by name |
| `getSymbol` | Get detailed symbol information |
| `findReferences` | Find all references to a symbol |
| `getArchitecture` | Get codebase architecture overview |
| `analyzeImpact` | Analyze impact of changing a symbol |
| `getStatus` | Get CKB system status |

## Development

```bash
# Run tests
go test ./...

# Build
go build -o ckb ./cmd/ckb

# Regenerate SCIP index
scip-go --repository-root=.
```

## Requirements

- Go 1.21+
- Git (for repository operations)
- Optional: gopls (for LSP support)
- Optional: scip-go (for SCIP indexing)

## License

Free for personal use. Commercial/enterprise use requires a license. See [LICENSE](LICENSE) for details.

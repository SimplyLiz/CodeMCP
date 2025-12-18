# CKB — Code Knowledge Backend

**Give your AI assistant superpowers for understanding code.**

CKB is a code intelligence layer that gives AI assistants (like Claude Code) deep understanding of your codebase. Instead of grepping through files, your AI can now *navigate* code like a senior engineer would.

## What It Does

### Instant Symbol Search
Find any function, class, or variable across your entire codebase in milliseconds. Filter by type, scope to specific modules.

### Find All References
"Where is this function called?" — answered instantly with full context, not just file matches.

### Impact Analysis
Before refactoring, know exactly what breaks. Get a risk score and see all affected code paths.

### Architecture Maps
Understand how modules connect. See dependency graphs without digging through imports.

### Call Graphs
Trace execution flow. See what calls what, and who depends on whom.

### Dead Code Detection
Get keep/investigate/remove verdicts on symbols based on usage analysis.

## Three Ways to Use It

| Interface | Best For |
|-----------|----------|
| **Claude Code (MCP)** | AI-assisted development — Claude can query your codebase directly |
| **CLI** | Quick lookups from terminal |
| **HTTP API** | IDE plugins, CI integration, custom tooling |

## Quick Start

> **New to CKB?** Check out the **[Quick Start Guide](QUICKSTART.md)** for detailed installation instructions for Windows, macOS, and Linux.

```bash
# 1. Build
git clone https://github.com/anthropics/ckb.git
cd ckb
go build -o ckb ./cmd/ckb

# 2. Initialize in your project
./ckb init

# 3. Generate index (Go example)
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest
scip-go --repository-root=.

# 4. Connect to Claude Code
claude mcp add --transport stdio ckb -- /path/to/ckb mcp
```

Now Claude can answer questions like:
- *"What calls the HandleRequest function?"*
- *"What's the blast radius if I change UserService?"*
- *"Is this legacy code still used?"*

## Features at a Glance

| Feature | CLI | MCP Tool | What It Does |
|---------|-----|----------|--------------|
| Search symbols | `ckb search` | `searchSymbols` | Find by name |
| Get details | `ckb symbol` | `getSymbol` | Full metadata |
| Find references | `ckb refs` | `findReferences` | All usages |
| Architecture | `ckb arch` | `getArchitecture` | Module structure |
| Impact analysis | `ckb impact` | `analyzeImpact` | Change risk |
| Call graph | — | `getCallGraph` | Caller/callee flow |
| Dead code check | — | `justifySymbol` | Keep/remove verdict |
| Module overview | — | `getModuleOverview` | Stats & activity |
| Health check | `ckb status` | `getStatus` | System status |
| Diagnostics | `ckb doctor` | `doctor` | Fix issues |

## Why CKB?

| Without CKB | With CKB |
|-------------|----------|
| AI greps for patterns | AI navigates semantically |
| "I found 47 matches for Handler" | "HandleRequest is called by 3 routes in api/server.go" |
| Guessing at impact | Knowing the blast radius |
| Reading entire files for context | Getting exactly what's relevant |

## Under the Hood

CKB orchestrates multiple code intelligence backends:

- **SCIP** — Precise, pre-indexed symbol data
- **LSP** — Real-time language server queries
- **Git** — Blame, history, churn analysis

Results are merged intelligently and compressed for LLM context limits.

## CLI Usage

```bash
# Check system status
./ckb status

# Search for symbols
./ckb search NewServer

# Find references to a symbol
./ckb refs NewServer

# Get symbol details
./ckb symbol "scip-go gomod pkg version `pkg/path`/Symbol()."

# Get architecture overview
./ckb arch

# Analyze change impact
./ckb impact <symbol-id>

# Run diagnostics
./ckb doctor
```

## HTTP API

```bash
# Start the HTTP server
./ckb serve --port 8080

# Example API calls
curl http://localhost:8080/health
curl http://localhost:8080/search?q=NewServer
curl http://localhost:8080/status
curl http://localhost:8080/architecture
```

### API Endpoints

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

## MCP Server (Claude Code Integration)

```bash
# Start MCP server (reads from stdin, writes to stdout)
./ckb mcp
```

Configure in Claude Code:
```bash
# Add to current project (creates .mcp.json)
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or add globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify configuration
claude mcp list
```

Or manually add to your MCP config:
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

For detailed MCP tool documentation, see **[MCP Tools Reference](docs/mcp.md)**.

## Configuration

CKB configuration is stored in `.ckb/config.json`:

```json
{
  "version": 5,
  "repoRoot": ".",
  "backends": {
    "scip": {
      "enabled": true,
      "indexPath": ".scip/index.scip"
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

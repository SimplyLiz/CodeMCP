# MCP Integration Guide

## Overview

CKB implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to provide code intelligence tools to AI assistants like Claude.

## What is MCP?

MCP is a standard protocol that allows AI assistants to interact with external tools and data sources. CKB exposes its code intelligence capabilities as MCP tools.

## Starting the MCP Server

```bash
# Start MCP server (stdio mode)
ckb mcp

# Start with verbose logging
ckb mcp --verbose
```

## Available Tools

### getSymbol

Get detailed information about a symbol.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |

**Example:**
```json
{
  "tool": "getSymbol",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123"
  }
}
```

**Response:**
```json
{
  "symbol": {
    "stableId": "ckb:repo:sym:abc123",
    "name": "ProcessData",
    "kind": "function",
    "signature": "func ProcessData(input Input) (Output, error)",
    "location": {
      "fileId": "internal/service/processor.go",
      "startLine": 42
    },
    "moduleId": "internal/service",
    "visibility": "public"
  }
}
```

---

### searchSymbols

Search for symbols matching a query.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `scope` | string | No | Module to search within |
| `kinds` | string[] | No | Symbol kinds to include |
| `limit` | number | No | Max results (default: 50) |

**Example:**
```json
{
  "tool": "searchSymbols",
  "arguments": {
    "query": "Process",
    "kinds": ["function", "method"],
    "limit": 10
  }
}
```

---

### findReferences

Find all references to a symbol.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |
| `scope` | string | No | Module to search within |
| `merge` | string | No | Backend merge strategy: "prefer-first" (default) or "union" |
| `limit` | number | No | Max references (default: 100) |

**Example:**
```json
{
  "tool": "findReferences",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123",
    "merge": "prefer-first",
    "limit": 100
  }
}
```

---

### getArchitecture

Get codebase architecture overview.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `depth` | number | No | Module depth (default: 2) |
| `includeExternal` | boolean | No | Include external deps |

**Example:**
```json
{
  "tool": "getArchitecture",
  "arguments": {
    "depth": 2
  }
}
```

---

### analyzeImpact

Analyze the impact of changing a symbol.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |
| `depth` | number | No | Analysis depth (default: 2) |
| `includeTests` | boolean | No | Include test impacts |

**Example:**
```json
{
  "tool": "analyzeImpact",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123",
    "depth": 3
  }
}
```

---

### getStatus

Get system status.

**Parameters:** None

**Example:**
```json
{
  "tool": "getStatus",
  "arguments": {}
}
```

---

### doctor

Run diagnostic checks.

**Parameters:** None

**Example:**
```json
{
  "tool": "doctor",
  "arguments": {}
}
```

---

## AI Navigation Tools

These tools are designed specifically for AI assistants to navigate and understand codebases.

### explainSymbol

Get an AI-friendly explanation of a symbol including usage statistics, git history, and a summary.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |

**Example:**
```json
{
  "tool": "explainSymbol",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123"
  }
}
```

**Response includes:**
- Symbol metadata (name, kind, signature, location)
- Usage statistics (caller count, reference count)
- Git history (creation date, last modified, author)
- AI-generated summary

---

### justifySymbol

Get a keep/investigate/remove verdict for a symbol based on usage analysis.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID |

**Example:**
```json
{
  "tool": "justifySymbol",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123"
  }
}
```

**Response includes:**
- Verdict: `keep`, `investigate`, or `remove`
- Confidence score (0-1)
- Reasoning explanation
- List of callers

---

### getCallGraph

Get a lightweight call graph showing callers and/or callees of a symbol.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Root symbol ID |
| `direction` | string | No | `callers`, `callees`, or `both` (default: `both`) |
| `depth` | number | No | Max traversal depth 1-4 (default: 1) |

**Example:**
```json
{
  "tool": "getCallGraph",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123",
    "direction": "callers",
    "depth": 2
  }
}
```

**Response includes:**
- Root node with symbol info
- Nodes array with caller/callee symbols
- Edges array showing relationships

---

### getModuleOverview

Get a high-level overview of a module including file count, symbol count, and recent activity.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | No | Path to module directory |
| `name` | string | No | Friendly name for the module |

**Example:**
```json
{
  "tool": "getModuleOverview",
  "arguments": {
    "path": "internal/query",
    "name": "Query Engine"
  }
}
```

**Response includes:**
- Module metadata (path, name)
- File and line counts
- Recent git activity (commits, active files)
- Entry points (exported symbols)

---

## Integration with Claude Code

### Quick Setup (Recommended)

Use the Claude Code CLI to add CKB:

```bash
# Add to current project (creates .mcp.json)
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or add globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify it's configured
claude mcp list
```

## Integration with Claude Desktop

For Claude Desktop (not Claude Code), add CKB to your MCP settings:

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "ckb": {
      "command": "/path/to/ckb",
      "args": ["mcp"],
      "cwd": "/path/to/your/repo"
    }
  }
}
```

### Multiple Repositories

Configure multiple CKB instances for different repos:

```json
{
  "mcpServers": {
    "ckb-frontend": {
      "command": "/path/to/ckb",
      "args": ["mcp"],
      "cwd": "/path/to/frontend-repo"
    },
    "ckb-backend": {
      "command": "/path/to/ckb",
      "args": ["mcp"],
      "cwd": "/path/to/backend-repo"
    }
  }
}
```

## Protocol Details

### Transport

CKB MCP server uses stdio transport:
- Input: JSON-RPC messages on stdin
- Output: JSON-RPC messages on stdout
- Logs: stderr (when --verbose)

### Message Format

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "searchSymbols",
    "arguments": {
      "query": "ProcessData"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"results\": [...]}"
      }
    ]
  }
}
```

### Error Handling

Errors are returned with CKB error codes:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32000,
    "message": "Symbol not found",
    "data": {
      "ckbCode": "SYMBOL_NOT_FOUND",
      "suggestedFixes": [...]
    }
  }
}
```

## Best Practices

### For AI Assistants

1. **Start with getStatus** - Check system health before queries
2. **Use searchSymbols first** - Find symbol IDs before detailed queries
3. **Follow drilldowns** - Use suggested queries for deeper exploration
4. **Respect truncation** - Large results are truncated with suggestions
5. **Handle errors gracefully** - Check suggestedFixes for recovery

### Example Workflow

```
1. getStatus()
   → Check backends are available

2. searchSymbols("UserService")
   → Get symbol ID: ckb:repo:sym:abc123

3. getSymbol("ckb:repo:sym:abc123")
   → Get full symbol details

4. findReferences("ckb:repo:sym:abc123")
   → Find all usages

5. analyzeImpact("ckb:repo:sym:abc123")
   → Understand change risk
```

## Troubleshooting

### Server won't start

```bash
# Check CKB is installed
which ckb

# Test manually
echo '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | ckb mcp
```

### No tools available

```bash
# Verify CKB is initialized in the repo
ls -la .ckb/

# Initialize if needed
ckb init
```

### Slow responses

1. Check if SCIP index exists
2. Run `ckb doctor` to diagnose
3. Consider reducing query scope

### Connection errors

Check Claude Desktop logs:
```bash
# macOS
tail -f ~/Library/Logs/Claude/mcp*.log
```

## Development

### Testing MCP locally

```bash
# Start server
ckb mcp --verbose 2>mcp.log &

# Send test request
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ckb mcp

# Check logs
tail -f mcp.log
```

### Debugging

Enable verbose logging:
```bash
ckb mcp --verbose
```

Logs include:
- Incoming requests
- Tool invocations
- Response times
- Errors

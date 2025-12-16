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
| `includeTests` | boolean | No | Include test files |
| `limit` | number | No | Max references |

**Example:**
```json
{
  "tool": "findReferences",
  "arguments": {
    "symbolId": "ckb:repo:sym:abc123",
    "includeTests": false,
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

### runDoctor

Run diagnostic checks.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `fix` | boolean | No | Return fix script |

**Example:**
```json
{
  "tool": "runDoctor",
  "arguments": {
    "fix": false
  }
}
```

## Integration with Claude Desktop

### Configuration

Add CKB to your Claude Desktop MCP settings:

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

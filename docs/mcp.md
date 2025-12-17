# MCP Tools Reference

CKB exposes code intelligence capabilities via the Model Context Protocol (MCP), enabling AI assistants like Claude Code to understand and navigate codebases.

## Quick Setup

```bash
# Add CKB to Claude Code (project-level)
claude mcp add --transport stdio ckb --scope project -- /path/to/ckb mcp

# Or globally for all projects
claude mcp add --transport stdio ckb --scope user -- /path/to/ckb mcp

# Verify
claude mcp list
```

## Tools Overview

| Tool | Purpose | Use Case |
|------|---------|----------|
| [searchSymbols](#searchsymbols) | Find symbols by name | "Find all handlers in the codebase" |
| [getSymbol](#getsymbol) | Get symbol details | "What does UserService do?" |
| [findReferences](#findreferences) | Find all usages | "Where is this function called?" |
| [getArchitecture](#getarchitecture) | Module overview | "How is this codebase structured?" |
| [analyzeImpact](#analyzeimpact) | Change risk analysis | "What breaks if I change this?" |
| [explainSymbol](#explainsymbol) | AI-friendly summary | "Explain this symbol to me" |
| [justifySymbol](#justifysymbol) | Keep/remove verdict | "Is this code still needed?" |
| [getCallGraph](#getcallgraph) | Caller/callee graph | "What calls this function?" |
| [getModuleOverview](#getmoduleoverview) | Module statistics | "How active is this package?" |
| [getStatus](#getstatus) | System health | "Is CKB working?" |
| [doctor](#doctor) | Diagnostics | "Why isn't CKB working?" |

---

## Core Navigation Tools

### searchSymbols

Search for symbols by name with optional filtering.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `query` | string | Yes | - | Search query (substring match, case-insensitive) |
| `scope` | string | No | - | Module ID to limit search scope |
| `kinds` | string[] | No | - | Symbol kinds: `function`, `method`, `class`, `interface`, `variable`, `constant` |
| `limit` | number | No | 20 | Maximum results |

**Use Cases:**

1. **Find all HTTP handlers:**
   ```json
   { "query": "Handler", "kinds": ["function", "method"], "limit": 50 }
   ```

2. **Find classes in a specific module:**
   ```json
   { "query": "", "scope": "internal/api", "kinds": ["class"] }
   ```

3. **Quick symbol lookup:**
   ```json
   { "query": "NewServer" }
   ```

**Response:**
```json
{
  "symbols": [
    {
      "stableId": "scip-go gomod ckb ... `pkg/path`/Symbol().",
      "name": "NewServer",
      "kind": "function",
      "score": 100,
      "location": { "fileId": "internal/api/server.go", "startLine": 42 },
      "visibility": { "visibility": "public", "confidence": 0.9 }
    }
  ],
  "totalCount": 15,
  "truncated": true,
  "provenance": { "queryDurationMs": 45 }
}
```

---

### getSymbol

Get detailed metadata for a specific symbol by its stable ID.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID from search results |
| `repoStateMode` | string | No | `head` | `head` (committed only) or `full` (include uncommitted) |

**Use Cases:**

1. **Get full symbol details after search:**
   ```json
   { "symbolId": "scip-go gomod ckb ... `internal/query`/Engine#SearchSymbols()." }
   ```

2. **Include uncommitted changes:**
   ```json
   { "symbolId": "...", "repoStateMode": "full" }
   ```

**Response:**
```json
{
  "symbol": {
    "stableId": "scip-go gomod ckb ... `internal/query`/Engine#SearchSymbols().",
    "name": "SearchSymbols",
    "kind": "method",
    "signature": "func (e *Engine) SearchSymbols(ctx context.Context, opts SearchSymbolsOptions) (*SearchSymbolsResponse, error)",
    "containerName": "Engine",
    "documentation": "SearchSymbols finds symbols matching the query",
    "location": { "fileId": "internal/query/engine.go", "startLine": 156, "endLine": 210 },
    "moduleId": "internal/query",
    "visibility": { "visibility": "public", "confidence": 0.9 }
  }
}
```

---

### findReferences

Find all references to a symbol across the codebase.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID |
| `scope` | string | No | - | Module ID to limit search |
| `includeTests` | boolean | No | false | Include test file references |
| `merge` | string | No | `prefer-first` | Backend merge: `prefer-first` or `union` |
| `limit` | number | No | 100 | Maximum references |

**Use Cases:**

1. **Find all callers of a function:**
   ```json
   { "symbolId": "scip-go gomod ckb ... `internal/api`/NewServer()." }
   ```

2. **Find usages including tests:**
   ```json
   { "symbolId": "...", "includeTests": true }
   ```

3. **Find usages in specific module:**
   ```json
   { "symbolId": "...", "scope": "internal/mcp" }
   ```

**Response:**
```json
{
  "references": [
    {
      "kind": "call",
      "location": { "fileId": "cmd/ckb/serve.go", "startLine": 45 },
      "context": "server := api.NewServer(engine, logger)",
      "isTest": false
    }
  ],
  "totalCount": 12,
  "truncated": false
}
```

---

### getArchitecture

Get codebase architecture with module dependencies.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `depth` | number | No | 2 | Dependency traversal depth |
| `includeExternalDeps` | boolean | No | false | Include external dependencies |
| `refresh` | boolean | No | false | Force cache refresh |

**Use Cases:**

1. **Get codebase overview:**
   ```json
   {}
   ```

2. **Deep dependency analysis:**
   ```json
   { "depth": 4, "includeExternalDeps": true }
   ```

**Response:**
```json
{
  "modules": [
    {
      "moduleId": "internal/query",
      "name": "query",
      "path": "internal/query",
      "symbolCount": 245,
      "fileCount": 12,
      "language": "go"
    }
  ],
  "dependencyGraph": [
    { "from": "internal/api", "to": "internal/query", "kind": "import", "strength": 15 }
  ]
}
```

---

### analyzeImpact

Analyze the blast radius of changing a symbol.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Stable symbol ID to analyze |
| `depth` | number | No | 2 | Transitive impact depth |

**Use Cases:**

1. **Before refactoring a function:**
   ```json
   { "symbolId": "scip-go gomod ckb ... `internal/query`/Engine#SearchSymbols()." }
   ```

2. **Deep impact analysis:**
   ```json
   { "symbolId": "...", "depth": 4 }
   ```

**Response:**
```json
{
  "directImpact": [
    {
      "stableId": "...",
      "name": "handleSearch",
      "kind": "function",
      "distance": 1,
      "moduleId": "internal/api",
      "confidence": 0.95,
      "location": { "fileId": "internal/api/handlers.go", "startLine": 89 }
    }
  ],
  "transitiveImpact": [
    { "name": "ServeHTTP", "distance": 2, "moduleId": "internal/api" }
  ],
  "riskScore": {
    "score": 0.65,
    "level": "medium",
    "explanation": "Changes affect 3 modules with 12 direct dependents",
    "factors": [
      { "name": "publicAPI", "value": 0.8 },
      { "name": "crossModule", "value": 0.6 }
    ]
  }
}
```

---

## AI Navigation Tools

These tools are designed specifically for AI assistants to quickly understand code.

### explainSymbol

Get an AI-friendly explanation of a symbol including usage patterns, history, and summary.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID to explain |

**Use Cases:**

1. **Understand what a function does:**
   ```json
   { "symbolId": "scip-go gomod ckb ... `internal/query`/Engine#SearchSymbols()." }
   ```

**Response:**
```json
{
  "symbolId": "...",
  "name": "SearchSymbols",
  "kind": "method",
  "summary": "Core search function that finds symbols by name across the codebase",
  "signature": "func (e *Engine) SearchSymbols(...) (*SearchSymbolsResponse, error)",
  "location": { "fileId": "internal/query/engine.go", "startLine": 156 },
  "documentation": "SearchSymbols finds symbols matching the query...",
  "usageStats": {
    "referenceCount": 8,
    "callerCount": 5,
    "testCoverage": true
  },
  "history": {
    "lastModified": "2025-12-15",
    "recentCommits": 3,
    "authors": ["alice", "bob"]
  },
  "relatedSymbols": [
    { "name": "SearchSymbolsOptions", "kind": "struct", "relationship": "parameter" }
  ]
}
```

---

### justifySymbol

Get a keep/investigate/remove verdict for a symbol based on usage analysis. Useful for identifying dead code.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `symbolId` | string | Yes | Stable symbol ID to analyze |

**Use Cases:**

1. **Check if code is still needed:**
   ```json
   { "symbolId": "scip-go gomod ckb ... `internal/legacy`/OldHandler()." }
   ```

**Response:**
```json
{
  "symbolId": "...",
  "name": "OldHandler",
  "verdict": "investigate",
  "confidence": 0.75,
  "reasoning": [
    "Only 2 references found",
    "No test coverage",
    "Not modified in 6 months"
  ],
  "usageEvidence": {
    "referenceCount": 2,
    "callerModules": ["internal/api"],
    "isExported": true,
    "hasTests": false,
    "lastModified": "2025-06-10"
  },
  "recommendation": "Review usage in internal/api - may be dead code"
}
```

**Verdicts:**
- `keep` - Symbol is actively used, well-tested
- `investigate` - Low usage, may need review
- `remove` - Appears to be dead code

---

### getCallGraph

Get a lightweight call graph showing callers and callees of a symbol.

**Parameters:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `symbolId` | string | Yes | - | Root symbol for the graph |
| `direction` | string | No | `both` | `callers`, `callees`, or `both` |
| `depth` | number | No | 1 | Traversal depth (1-4) |

**Use Cases:**

1. **Find what calls a function:**
   ```json
   { "symbolId": "...", "direction": "callers" }
   ```

2. **Find what a function calls:**
   ```json
   { "symbolId": "...", "direction": "callees" }
   ```

3. **Full call graph with depth 2:**
   ```json
   { "symbolId": "...", "direction": "both", "depth": 2 }
   ```

**Response:**
```json
{
  "root": {
    "symbolId": "...",
    "name": "SearchSymbols",
    "kind": "method"
  },
  "callers": [
    {
      "symbolId": "...",
      "name": "handleSearch",
      "kind": "function",
      "location": { "fileId": "internal/api/handlers.go", "startLine": 89 },
      "depth": 1
    }
  ],
  "callees": [
    {
      "symbolId": "...",
      "name": "searchSCIP",
      "kind": "method",
      "depth": 1
    }
  ],
  "truncated": false
}
```

---

### getModuleOverview

Get a high-level overview of a module including size, complexity, and recent activity.

**Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | No | Path to module directory |
| `name` | string | No | Friendly name for the module |

**Use Cases:**

1. **Understand a package:**
   ```json
   { "path": "internal/query" }
   ```

2. **Get codebase root overview:**
   ```json
   {}
   ```

**Response:**
```json
{
  "moduleId": "internal/query",
  "name": "query",
  "path": "internal/query",
  "stats": {
    "fileCount": 12,
    "symbolCount": 245,
    "linesOfCode": 3500,
    "publicSymbols": 45,
    "privateSymbols": 200
  },
  "activity": {
    "lastModified": "2025-12-16",
    "commitsLast30Days": 15,
    "activeContributors": 3
  },
  "topSymbols": [
    { "name": "Engine", "kind": "struct", "referenceCount": 89 },
    { "name": "SearchSymbols", "kind": "method", "referenceCount": 45 }
  ],
  "dependencies": ["internal/backends", "internal/storage"],
  "dependents": ["internal/api", "internal/mcp", "cmd/ckb"]
}
```

---

## System Tools

### getStatus

Get CKB system status including backend health and cache statistics.

**Parameters:** None

**Use Cases:**

1. **Check system health before queries:**
   ```json
   {}
   ```

**Response:**
```json
{
  "status": "healthy",
  "healthy": true,
  "backends": [
    {
      "id": "scip",
      "available": true,
      "healthy": true,
      "capabilities": ["symbol-search", "find-references", "goto-definition"],
      "details": { "symbolCount": 2915, "documentCount": 153 }
    },
    {
      "id": "git",
      "available": true,
      "healthy": true,
      "capabilities": ["blame", "history", "churn"]
    }
  ],
  "cache": {
    "sizeBytes": 1048576,
    "queriesCached": 45,
    "viewsCached": 12,
    "hitRate": 0.85
  },
  "repoState": {
    "dirty": true,
    "headCommit": "abc123",
    "repoStateId": "def456"
  }
}
```

---

### doctor

Run diagnostic checks and get suggested fixes.

**Parameters:** None

**Use Cases:**

1. **Diagnose issues:**
   ```json
   {}
   ```

**Response:**
```json
{
  "healthy": false,
  "checks": [
    {
      "name": "SCIP Index",
      "status": "warning",
      "message": "SCIP index is stale (uncommitted changes)",
      "fixes": ["scip-go --repository-root=."]
    },
    {
      "name": "LSP Server",
      "status": "error",
      "message": "gopls not found in PATH",
      "fixes": ["go install golang.org/x/tools/gopls@latest"]
    }
  ]
}
```

---

## Recommended Workflows

### Understanding a New Codebase

```
1. getStatus()           → Verify CKB is healthy
2. getArchitecture()     → See module structure
3. getModuleOverview()   → Understand key modules
4. searchSymbols()       → Find entry points
```

### Before Making Changes

```
1. searchSymbols("TargetFunction")  → Find the symbol
2. getSymbol(symbolId)              → Get full details
3. findReferences(symbolId)         → Find all usages
4. analyzeImpact(symbolId)          → Assess risk
5. getCallGraph(symbolId)           → Understand call flow
```

### Code Review / Dead Code Detection

```
1. searchSymbols(query, kinds=["function"])  → Find candidates
2. justifySymbol(symbolId)                    → Get verdict
3. explainSymbol(symbolId)                    → Understand context
```

### Debugging

```
1. searchSymbols("ErrorHandler")    → Find error handling
2. getCallGraph(symbolId, "callers") → Trace call path
3. findReferences(symbolId)          → Find all error sites
```

---

## Error Handling

All tools return structured errors with suggested fixes:

```json
{
  "error": {
    "code": "SYMBOL_NOT_FOUND",
    "message": "Symbol not found: invalid-id",
    "suggestedFixes": [
      "Run searchSymbols() to find valid symbol IDs",
      "Check if the symbol was renamed or removed"
    ]
  }
}
```

**Common Error Codes:**
- `SYMBOL_NOT_FOUND` - Invalid symbol ID
- `BACKEND_UNAVAILABLE` - Required backend not running
- `INDEX_STALE` - SCIP index needs regeneration
- `QUERY_TIMEOUT` - Query took too long

---

## Performance Tips

1. **Use `limit` parameter** - Don't fetch more results than needed
2. **Use `scope` parameter** - Limit searches to relevant modules
3. **Start with `getStatus()`** - Avoid queries if backends are unhealthy
4. **Cache symbol IDs** - Reuse stable IDs across related queries
5. **Prefer `prefer-first` merge** - Faster than `union` for most cases

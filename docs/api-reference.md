# CKB API Reference

## Overview

CKB provides both a CLI and HTTP API. This document covers both interfaces.

## HTTP API

### Base URL

```
http://localhost:8080
```

### Authentication

Currently no authentication required (local development mode).

### Response Format

All responses are JSON with consistent structure:

```json
{
  "data": { ... },
  "provenance": {
    "backends": ["scip"],
    "repoStateId": "...",
    "headCommit": "...",
    "cachedAt": "..."
  },
  "warnings": [],
  "drilldowns": []
}
```

### Error Format

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": { ... },
  "suggestedFixes": [
    {
      "type": "run-command",
      "command": "ckb doctor",
      "description": "Run diagnostics",
      "safe": true
    }
  ],
  "drilldowns": []
}
```

---

## Endpoints

### Health & Status

#### GET /health

Simple liveness check.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-12-16T12:00:00Z",
  "version": "0.1.0"
}
```

#### GET /ready

Readiness check with backend status.

**Response:**
```json
{
  "status": "ready",
  "timestamp": "2025-12-16T12:00:00Z",
  "backends": {
    "scip": true,
    "lsp": true,
    "git": true
  }
}
```

#### GET /status

Full system status.

**Response:**
```json
{
  "repository": {
    "root": "/path/to/repo",
    "headCommit": "abc123...",
    "dirty": false,
    "repoStateId": "def456..."
  },
  "backends": {
    "scip": { "available": true, "indexAge": "2h" },
    "lsp": { "available": true, "serverPid": 1234 },
    "git": { "available": true }
  },
  "cache": {
    "queryCache": { "entries": 150, "hitRate": 0.85 },
    "viewCache": { "entries": 20, "hitRate": 0.92 },
    "negativeCache": { "entries": 5 }
  }
}
```

---

### Diagnostics

#### GET /doctor

Run diagnostic checks.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-12-16T12:00:00Z",
  "checks": [
    {
      "name": "config",
      "status": "pass",
      "message": "Configuration is valid"
    },
    {
      "name": "scip",
      "status": "warn",
      "message": "SCIP index is 5 commits behind HEAD"
    }
  ],
  "summary": {
    "total": 4,
    "passed": 3,
    "warnings": 1,
    "failed": 0
  }
}
```

#### POST /doctor/fix

Get fix script for issues.

**Response:**
```json
{
  "script": "#!/bin/bash\nscip-go\n",
  "issues": ["scip-index-stale"]
}
```

---

### Symbol Operations

#### GET /symbol/:id

Get symbol by stable ID.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `id` | path | Stable symbol ID |

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
      "startLine": 42,
      "startColumn": 6,
      "endLine": 42,
      "endColumn": 17
    },
    "moduleId": "internal/service",
    "visibility": "public",
    "modifiers": ["exported"]
  },
  "provenance": { ... }
}
```

**Errors:**
- `404 SYMBOL_NOT_FOUND` - Symbol does not exist
- `410 SYMBOL_DELETED` - Symbol was deleted (tombstone)

---

#### GET /search

Search for symbols.

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| `q` | string | required | Search query |
| `scope` | string | - | Module ID to search within |
| `kinds` | string | - | Comma-separated kinds (function, class, method, etc.) |
| `limit` | int | 50 | Maximum results |
| `merge` | string | prefer-first | Merge strategy: prefer-first, union |
| `includeExternal` | bool | false | Include external dependencies |

**Example:**
```bash
curl "http://localhost:8080/search?q=Process&kinds=function,method&limit=10"
```

**Response:**
```json
{
  "query": "Process",
  "results": [
    {
      "stableId": "ckb:repo:sym:abc123",
      "name": "ProcessData",
      "kind": "function",
      "moduleId": "internal/service",
      "confidence": 0.95
    }
  ],
  "total": 1,
  "hasMore": false,
  "truncation": null
}
```

---

#### GET /refs/:id

Find references to a symbol.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `id` | path | Stable symbol ID |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| `scope` | string | - | Module ID to search within |
| `limit` | int | 100 | Maximum references |
| `includeTests` | bool | true | Include test files |

**Response:**
```json
{
  "symbolId": "ckb:repo:sym:abc123",
  "references": [
    {
      "location": {
        "fileId": "cmd/main.go",
        "startLine": 25,
        "startColumn": 10
      },
      "kind": "call",
      "fromSymbol": "ckb:repo:sym:def456",
      "fromModule": "cmd"
    }
  ],
  "total": 15,
  "truncation": {
    "reason": "max-refs",
    "originalCount": 150,
    "returnedCount": 100
  },
  "drilldowns": [
    {
      "label": "Get all references",
      "query": "findReferences abc123 --limit=1000"
    }
  ]
}
```

---

### Analysis

#### GET /architecture

Get codebase architecture overview.

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| `depth` | int | 2 | Module depth |
| `includeExternal` | bool | false | Include external deps |

**Response:**
```json
{
  "modules": [
    {
      "moduleId": "internal/api",
      "name": "api",
      "rootPath": "internal/api",
      "symbolCount": 45,
      "impactCount": 120
    }
  ],
  "dependencies": [
    {
      "from": "internal/api",
      "to": "internal/service",
      "kind": "import",
      "weight": 15
    }
  ],
  "metrics": {
    "totalModules": 12,
    "totalSymbols": 350,
    "avgModuleSize": 29
  }
}
```

---

#### GET /impact/:id

Analyze impact of changing a symbol.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `id` | path | Stable symbol ID |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| `depth` | int | 2 | Analysis depth |
| `includeTests` | bool | true | Include test impacts |

**Response:**
```json
{
  "symbolId": "ckb:repo:sym:abc123",
  "symbol": {
    "name": "ProcessData",
    "kind": "function",
    "moduleId": "internal/service"
  },
  "visibility": {
    "visibility": "public",
    "confidence": 0.95,
    "source": "scip-modifiers"
  },
  "riskScore": {
    "score": 0.72,
    "level": "high",
    "factors": {
      "visibility": 0.9,
      "directCallers": 0.65,
      "moduleSpread": 0.8,
      "impactKind": 0.5
    },
    "explanation": "High risk: public symbol with 15 callers across 4 modules"
  },
  "directImpact": [
    {
      "stableId": "ckb:repo:sym:def456",
      "name": "HandleRequest",
      "kind": "direct-caller",
      "confidence": 0.9,
      "moduleId": "internal/api"
    }
  ],
  "modulesAffected": [
    {
      "moduleId": "internal/api",
      "directImpacts": 3,
      "transitiveImpacts": 5
    }
  ]
}
```

---

### Cache Operations

#### POST /cache/warm

Warm cache with common queries.

**Response:**
```json
{
  "status": "success",
  "message": "Cache warming initiated",
  "timestamp": "2025-12-16T12:00:00Z"
}
```

#### POST /cache/clear

Clear all caches.

**Response:**
```json
{
  "status": "success",
  "message": "Cache cleared",
  "entriesRemoved": 175,
  "timestamp": "2025-12-16T12:00:00Z"
}
```

---

## Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `BACKEND_UNAVAILABLE` | 503 | No backend can handle request |
| `INDEX_MISSING` | 503 | Required index not found |
| `INDEX_STALE` | 412 | Index is outdated |
| `WORKSPACE_NOT_READY` | 503 | LSP workspace initializing |
| `TIMEOUT` | 504 | Query timed out |
| `RATE_LIMITED` | 429 | Too many requests |
| `SYMBOL_NOT_FOUND` | 404 | Symbol doesn't exist |
| `SYMBOL_DELETED` | 410 | Symbol was deleted |
| `SCOPE_INVALID` | 400 | Invalid scope parameter |
| `BUDGET_EXCEEDED` | 413 | Response too large |
| `INTERNAL_ERROR` | 500 | Unexpected error |

---

## OpenAPI Specification

Full OpenAPI 3.0 spec available at:

```
GET /openapi.json
```

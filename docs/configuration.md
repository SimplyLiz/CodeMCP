# CKB Configuration Guide

## Overview

CKB configuration is stored in `.ckb/config.json` in your repository root. This file is created when you run `ckb init`.

## Configuration File

### Location

```
your-repo/
└── .ckb/
    ├── config.json    # Configuration
    └── ckb.db         # SQLite database
```

### Full Schema

```json
{
  "version": 5,
  "backends": {
    "scip": {
      "enabled": true,
      "indexPath": "index.scip",
      "autoReindex": false
    },
    "lsp": {
      "enabled": true,
      "servers": {}
    },
    "git": {
      "enabled": true
    }
  },
  "queryPolicy": {
    "backendLadder": ["scip", "lsp", "git"],
    "mergeStrategy": "prefer-first"
  },
  "lspSupervisor": {
    "startupTimeoutMs": 30000,
    "shutdownTimeoutMs": 5000,
    "maxRestarts": 3,
    "healthCheckIntervalMs": 60000
  },
  "modules": {
    "detectStrategy": "auto",
    "customRoots": []
  },
  "importScan": {
    "enabled": true,
    "maxDepth": 10
  },
  "cache": {
    "queryCacheTtlSeconds": 300,
    "viewCacheTtlSeconds": 3600,
    "negativeCacheTtlSeconds": 60,
    "maxCacheEntries": 10000
  },
  "budget": {
    "maxModules": 10,
    "maxSymbolsPerModule": 5,
    "maxImpactItems": 20,
    "maxDrilldowns": 5,
    "estimatedMaxTokens": 4000
  },
  "backendLimits": {
    "maxRefsPerQuery": 10000,
    "maxSymbolsPerSearch": 1000,
    "maxFilesScanned": 5000,
    "maxFileSizeBytes": 1048576,
    "maxUnionModeTimeMs": 60000,
    "maxScipIndexSizeMb": 500
  },
  "privacy": {
    "redactPaths": false,
    "excludePatterns": []
  },
  "logging": {
    "level": "info",
    "format": "human"
  }
}
```

## Configuration Sections

### version

Schema version number. Current version is `5`.

```json
{
  "version": 5
}
```

---

### backends

Configure which backends are enabled and their settings.

#### backends.scip

SCIP (Source Code Intelligence Protocol) backend.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | true | Enable SCIP backend |
| `indexPath` | string | "index.scip" | Path to SCIP index file |
| `autoReindex` | bool | false | Automatically regenerate stale index |

```json
{
  "backends": {
    "scip": {
      "enabled": true,
      "indexPath": "index.scip",
      "autoReindex": false
    }
  }
}
```

#### backends.lsp

Language Server Protocol backend.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | true | Enable LSP backend |
| `servers` | object | {} | Language-specific server configs |

```json
{
  "backends": {
    "lsp": {
      "enabled": true,
      "servers": {
        "go": {
          "command": "gopls",
          "args": ["serve"]
        },
        "typescript": {
          "command": "typescript-language-server",
          "args": ["--stdio"]
        }
      }
    }
  }
}
```

#### backends.git

Git backend for fallback operations.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | true | Enable Git backend |

---

### queryPolicy

Controls how queries are routed and merged.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `backendLadder` | string[] | ["scip", "lsp", "git"] | Backend priority order |
| `mergeStrategy` | string | "prefer-first" | How to merge results |

**Merge Strategies:**
- `prefer-first`: Use first successful backend response
- `union`: Merge all backend responses, deduplicate

```json
{
  "queryPolicy": {
    "backendLadder": ["scip", "lsp", "git"],
    "mergeStrategy": "prefer-first"
  }
}
```

---

### lspSupervisor

Controls LSP server lifecycle management.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `startupTimeoutMs` | int | 30000 | Max time to wait for server start |
| `shutdownTimeoutMs` | int | 5000 | Max time to wait for graceful shutdown |
| `maxRestarts` | int | 3 | Max restart attempts before giving up |
| `healthCheckIntervalMs` | int | 60000 | Health check interval |

---

### modules

Module detection settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `detectStrategy` | string | "auto" | Detection strategy |
| `customRoots` | string[] | [] | Additional module roots |

**Detection Strategies:**
- `auto`: Detect based on language markers (go.mod, package.json, etc.)
- `manual`: Only use customRoots
- `directory`: Treat each top-level directory as a module

```json
{
  "modules": {
    "detectStrategy": "auto",
    "customRoots": ["internal/legacy"]
  }
}
```

---

### importScan

Import/dependency scanning settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | true | Enable import scanning |
| `maxDepth` | int | 10 | Maximum dependency depth |

---

### cache

Cache tier configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `queryCacheTtlSeconds` | int | 300 | Query cache TTL (5 min) |
| `viewCacheTtlSeconds` | int | 3600 | View cache TTL (1 hour) |
| `negativeCacheTtlSeconds` | int | 60 | Negative cache TTL |
| `maxCacheEntries` | int | 10000 | Max entries per cache tier |

```json
{
  "cache": {
    "queryCacheTtlSeconds": 300,
    "viewCacheTtlSeconds": 3600,
    "negativeCacheTtlSeconds": 60,
    "maxCacheEntries": 10000
  }
}
```

---

### budget

Response budget limits for LLM optimization.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `maxModules` | int | 10 | Max modules in response |
| `maxSymbolsPerModule` | int | 5 | Max symbols per module |
| `maxImpactItems` | int | 20 | Max impact items |
| `maxDrilldowns` | int | 5 | Max drilldown suggestions |
| `estimatedMaxTokens` | int | 4000 | Target token budget |

```json
{
  "budget": {
    "maxModules": 10,
    "maxSymbolsPerModule": 5,
    "maxImpactItems": 20,
    "maxDrilldowns": 5,
    "estimatedMaxTokens": 4000
  }
}
```

---

### backendLimits

Hard limits to protect against resource exhaustion.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `maxRefsPerQuery` | int | 10000 | Max references per query |
| `maxSymbolsPerSearch` | int | 1000 | Max symbols per search |
| `maxFilesScanned` | int | 5000 | Max files to scan |
| `maxFileSizeBytes` | int | 1048576 | Max file size (1 MB) |
| `maxUnionModeTimeMs` | int | 60000 | Max time for union merge |
| `maxScipIndexSizeMb` | int | 500 | SCIP index size warning |

---

### privacy

Privacy and redaction settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `redactPaths` | bool | false | Redact file paths in responses |
| `excludePatterns` | string[] | [] | Glob patterns to exclude |

```json
{
  "privacy": {
    "redactPaths": false,
    "excludePatterns": [
      "**/secrets/**",
      "**/*.env"
    ]
  }
}
```

---

### logging

Logging configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | "info" | Log level (debug, info, warn, error) |
| `format` | string | "human" | Output format (human, json) |

```json
{
  "logging": {
    "level": "info",
    "format": "human"
  }
}
```

## Environment Variables

Configuration can be overridden with environment variables:

| Variable | Description |
|----------|-------------|
| `CKB_LOG_LEVEL` | Override log level |
| `CKB_LOG_FORMAT` | Override log format |
| `CKB_CONFIG_PATH` | Custom config path |

## Examples

### Minimal Configuration

```json
{
  "version": 5,
  "backends": {
    "scip": { "enabled": true },
    "lsp": { "enabled": false },
    "git": { "enabled": true }
  }
}
```

### High-Performance Configuration

```json
{
  "version": 5,
  "backends": {
    "scip": {
      "enabled": true,
      "autoReindex": true
    },
    "lsp": { "enabled": false },
    "git": { "enabled": true }
  },
  "queryPolicy": {
    "backendLadder": ["scip", "git"],
    "mergeStrategy": "prefer-first"
  },
  "cache": {
    "queryCacheTtlSeconds": 600,
    "viewCacheTtlSeconds": 7200,
    "maxCacheEntries": 50000
  }
}
```

### Large Codebase Configuration

```json
{
  "version": 5,
  "budget": {
    "maxModules": 20,
    "maxSymbolsPerModule": 10,
    "maxImpactItems": 50,
    "estimatedMaxTokens": 8000
  },
  "backendLimits": {
    "maxRefsPerQuery": 50000,
    "maxSymbolsPerSearch": 5000,
    "maxFilesScanned": 20000
  }
}
```

## Validation

Validate your configuration:

```bash
ckb doctor
```

This checks:
- JSON syntax
- Schema version compatibility
- Required fields
- Value ranges
- Backend availability

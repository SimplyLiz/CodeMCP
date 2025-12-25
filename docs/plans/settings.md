# Plan: Config Improvements (Lean Version)

**Status: IMPLEMENTED** ✅

## Summary

Add environment variable overrides and a simple `ckb config show` command. Skip the full get/set/explain CLI - JSON editing is fine for the 10% of users who need it.

**Actual effort:** ~350 LOC (config.go additions + cmd/ckb/config.go + tests)

---

## Part 1: Environment Variable Overrides

### Location
`internal/config/config.go` - modify `LoadConfig()`

### Pattern
`CKB_<SECTION>_<KEY>` (uppercase, underscores for camelCase)

### Examples
```bash
CKB_CACHE_QUERY_TTL_SECONDS=600 ckb mcp
CKB_BUDGET_MAX_MODULES=50 ckb arch
CKB_BACKENDS_SCIP_ENABLED=false ckb status
CKB_LOGGING_LEVEL=debug ckb serve
```

### Implementation
1. After loading JSON config, iterate over known env vars
2. Parse and apply overrides with type coercion
3. Log when overrides are applied (debug level)

### Supported Env Vars (most useful subset)
| Env Var | Config Path | Type |
|---------|-------------|------|
| `CKB_TIER` | tier | string |
| `CKB_LOGGING_LEVEL` | logging.level | string |
| `CKB_LOGGING_FORMAT` | logging.format | string |
| `CKB_CACHE_QUERY_TTL_SECONDS` | cache.queryTtlSeconds | int |
| `CKB_BUDGET_MAX_MODULES` | budget.maxModules | int |
| `CKB_BUDGET_MAX_SYMBOLS_PER_MODULE` | budget.maxSymbolsPerModule | int |
| `CKB_BUDGET_ESTIMATED_MAX_TOKENS` | budget.estimatedMaxTokens | int |
| `CKB_BACKENDS_SCIP_ENABLED` | backends.scip.enabled | bool |
| `CKB_BACKENDS_LSP_ENABLED` | backends.lsp.enabled | bool |
| `CKB_TELEMETRY_ENABLED` | telemetry.enabled | bool |
| `CKB_DAEMON_PORT` | daemon.port | int |

~100 LOC

---

## Part 2: `ckb config show`

### Location
`cmd/ckb/config.go` (new, minimal)

### Usage
```bash
ckb config show           # Pretty-print current config
ckb config show --json    # Raw JSON output
ckb config show --diff    # Only show non-default values
```

### Output (human format)
```
CKB Configuration (.ckb/config.json)
────────────────────────────────────

version: 5
tier: auto

backends:
  scip:
    enabled: true
    indexPath: .scip/index.scip
  lsp:
    enabled: true
  git:
    enabled: true

cache:
  queryTtlSeconds: 300
  viewTtlSeconds: 3600
  negativeTtlSeconds: 60

budget:
  maxModules: 10
  maxSymbolsPerModule: 5
  ...

[Env overrides: CKB_LOGGING_LEVEL=debug]
```

~80 LOC

---

## Part 3: Wiki Documentation

### Update Configuration.md

1. **Add missing settings** (~25 settings not currently documented):
   - `daemon.*` section (15 settings)
   - `webhooks[].*` section (6 per webhook)
   - `telemetry.attributes.*` (4 settings)

2. **Add Environment Variables section** documenting the supported overrides

3. **Add `ckb config show` to CLI reference**

~150 lines of wiki updates

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add `ApplyEnvOverrides()`, call from `LoadConfig()` |
| `cmd/ckb/config.go` | New file, `config show` command |
| `../CodeMCPWiki/Configuration.md` | Add daemon, webhooks, env vars sections |

---

## Implementation Order

1. Env var overrides in `config.go`
2. `ckb config show` command
3. Wiki documentation updates
4. Tests

---

## Future Considerations (not in scope)

- `ckb config get/set` - defer unless user demand emerges
- `ckb config preset` - defer
- Full schema registry - not needed for this lean approach

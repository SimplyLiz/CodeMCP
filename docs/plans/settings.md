# Plan: `ckb config` CLI Command + Wiki Documentation

## Summary

Add a `ckb config` CLI subcommand for managing configuration without editing JSON manually, and update the wiki documentation to cover all ~85 settings.

---

## Part 1: CLI Implementation

### New Files

| File | Purpose |
|------|---------|
| `cmd/ckb/config.go` | Config command with subcommands |
| `internal/config/accessor.go` | Dot-notation key access + type-safe parsing |
| `internal/config/schema.go` | Config key registry with metadata |

### Subcommands (v1)

```
ckb config list [section]     # Show all keys with current vs default
ckb config get <key>          # Get single value (e.g., cache.queryTtlSeconds)
ckb config set <key> <value>  # Set a value with validation
ckb config reset [section]    # Reset to defaults
ckb config explain <key>      # Show description, type, valid values
```

### Future Consideration (v2)

```
ckb config preset <name>      # Apply preset (monorepo, ci, air-gapped)
```

Presets would apply multiple related settings at once:
- `monorepo`: Higher limits, union merge mode
- `ci`: JSON logging, no caching, no daemon
- `air-gapped`: Telemetry disabled, local-only

### Implementation Phases

**Phase 1: Schema Registry** (`internal/config/schema.go`)
- Define `ConfigKey` struct with: Path, Type, Default, Description, EnvVar
- Build registry for all ~85 keys
- ~400 LOC

**Phase 2: Accessor** (`internal/config/accessor.go`)
- `GetValue(cfg, path)` - reflection-based dot notation access
- `SetValue(cfg, path, value)` - type parsing + validation
- `ResetSection(cfg, section)` - restore defaults
- ~300 LOC

**Phase 3: CLI Commands** (`cmd/ckb/config.go`)
- Follow pattern from `cmd/ckb/federation.go` for subcommand structure
- Support `--json` and human-readable output
- Add tab completion for key names
- ~400 LOC

**Phase 4: Environment Variable Overrides** (`internal/config/config.go`)
- Pattern: `CKB_<SECTION>_<KEY>` (uppercase, underscores)
- Examples:
  - `cache.queryTtlSeconds` → `CKB_CACHE_QUERY_TTL_SECONDS`
  - `backends.scip.enabled` → `CKB_BACKENDS_SCIP_ENABLED`
- Applied in `LoadConfig()` after reading JSON
- Essential for Docker/CI use cases
- ~100 LOC

**Phase 5: Tests**
- Unit tests for accessor and schema
- Integration tests for CLI commands

### Output Examples

```bash
$ ckb config get cache.queryTtlSeconds
300

$ ckb config list cache
cache.queryTtlSeconds     300    (default)
cache.viewTtlSeconds      3600   (default)
cache.negativeTtlSeconds  120    (modified, default: 60)

$ ckb config explain budget.maxModules
budget.maxModules
  Type:     integer
  Default:  10
  Current:  10
  EnvVar:   CKB_BUDGET_MAX_MODULES

  Maximum number of modules included in architecture responses.
  Higher values provide more context but increase response size.
```

---

## Part 2: Wiki Documentation Updates

### Current State

`Configuration.md` (1,603 lines) covers most settings but is missing:

| Missing Section | Settings Count |
|-----------------|----------------|
| `daemon.*` | 15 settings |
| `webhooks[].*` | 6 settings per webhook |
| `telemetry.attributes.*` | 4 settings |
| `telemetry.aggregation.storeCallers/maxCallersPerSymbol` | 2 settings |

**Total missing: ~25 settings**

### Recommended Approach

**Expand Configuration.md in place** (don't split into multiple files):
- Add `### daemon` section after `### logging`
- Add `### webhooks` section after `### daemon`
- Expand `### telemetry` to include attributes subsection
- Keep existing structure and style

### Documentation Tasks

1. **Add daemon section** (~150 lines)
   - daemon.port, daemon.bind, daemon.logLevel, daemon.logFile
   - daemon.auth.enabled, daemon.auth.token, daemon.auth.tokenFile
   - daemon.watch.enabled, daemon.watch.debounceMs, daemon.watch.ignorePatterns, daemon.watch.repos
   - daemon.schedule.refresh, daemon.schedule.federationSync, daemon.schedule.hotspotSnapshot

2. **Add webhooks section** (~80 lines)
   - id, url, secret, events, format, headers
   - Example webhook configuration

3. **Expand telemetry section** (~50 lines)
   - Add attributes subsection (functionKeys, namespaceKeys, fileKeys, lineKeys)
   - Add storeCallers and maxCallersPerSymbol to aggregation

4. **Update Full Schema example** at top of file to include daemon/webhooks

5. **Add CLI section** (~100 lines)
   - Document `ckb config` commands once implemented
   - Add examples

---

## Files to Modify

### Code Changes
- `cmd/ckb/config.go` (new)
- `internal/config/accessor.go` (new)
- `internal/config/schema.go` (new)
- `internal/config/config.go` (add env override support)
- `cmd/ckb/format.go` (add config formatters)

### Documentation Changes
- `../CodeMCPWiki/Configuration.md` (expand with missing settings)

---

## Estimated Effort

| Component | LOC | Complexity |
|-----------|-----|------------|
| Schema registry | ~400 | High (define all 85 keys) |
| Accessor | ~300 | Medium (reflection) |
| CLI commands | ~400 | Medium |
| Env var overrides | ~100 | Low |
| Tests | ~300 | Medium |
| Wiki updates | ~400 | Low |
| **Total** | **~1,900** | |

---

## Implementation Order

1. Schema registry (enables everything else)
2. Accessor functions
3. CLI `config get` (simplest, validates accessor)
4. CLI `config list`
5. CLI `config set`
6. CLI `config reset`
7. CLI `config explain`
8. Wiki documentation updates
9. Tests

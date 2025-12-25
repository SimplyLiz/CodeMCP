# Changelog

All notable changes to CKB will be documented in this file.

## [7.5.0]

### Added

#### Token Efficiency Visibility
Users can now see CKB's token savings compared to bloated MCP servers:

**Startup Banner:**
```
CKB MCP Server v7.5.0
  Active tools: 14 / 76 (18%)
  Estimated context: ~1k tokens
  Preset: core
```

**getStatus Response:**
```json
"preset": {
  "active": "core",
  "exposed": 14,
  "total": 76,
  "estimatedTokens": 1529,
  "fullPresetTokens": 9040,
  "tokenSavingsPercent": 83
}
```

This addresses community feedback about MCP tools consuming 50-80k tokens before conversations even start. CKB's preset system delivers 83% token reduction while maintaining full functionality.

**Future:** Per-tool token breakdown (`--tokens` flag showing individual tool costs) planned for a later release.

**Files Changed:**
- `cmd/ckb/mcp.go` — Multi-line startup banner with token info
- `internal/mcp/server.go` — Added `EstimateActiveTokens()`, `EstimateFullTokens()` methods
- `internal/mcp/presets.go` — Added `FormatTokens()` helper
- `internal/mcp/tool_impls.go` — Token fields in getStatus response

#### Auto Index Updates
Automatic index maintenance across all CKB interfaces—keep your index fresh without manual intervention:

**Watch Mode (CLI):**
```bash
# Watch for changes and auto-reindex (standalone)
ckb index --watch
ckb index --watch --watch-interval 15s

# Watch with MCP server (existing, now configurable)
ckb mcp --watch
ckb mcp --watch --watch-interval 1m
```

**Daemon File Watcher:**
The daemon's file watcher now triggers actual incremental refreshes instead of just logging. When git changes are detected, the daemon queues and executes an incremental update.

**Webhook API:**
```bash
# Trigger incremental refresh via HTTP
curl -X POST http://localhost:9120/api/v1/refresh

# Force full reindex
curl -X POST http://localhost:9120/api/v1/refresh -d '{"full": true}'

# Specify repository
curl -X POST http://localhost:9120/api/v1/refresh -d '{"repo": "/path/to/repo"}'
```

**Response:**
```json
{
  "status": "queued",
  "repo": "/path/to/repo",
  "type": "incremental"
}
```

**Index Staleness Visibility:**
- `ckb status` now shows commits behind HEAD and index age
- MCP `getStatus` response includes `index.commitsBehind`, `index.indexAge`, `index.reason`
- Fresh indexes show ✓, stale indexes show ⚠ with specific reason

**Files Added:**
- `internal/daemon/refresh.go` — RefreshManager for incremental/full reindex
- `cmd/ckb/status_test.go` — Status type tests
- `internal/daemon/refresh_test.go` — RefreshManager unit tests

**Files Changed:**
- `cmd/ckb/index.go` — Added `--watch` and `--watch-interval` flags
- `cmd/ckb/mcp.go` — Added `--watch-interval` flag (min 5s, max 5m)
- `cmd/ckb/status.go` — Added staleness display with commits behind
- `internal/daemon/daemon.go` — Connected file watcher to RefreshManager
- `internal/daemon/server.go` — Added `/api/v1/refresh` endpoint
- `internal/index/metadata.go` — Added `Staleness` type and `GetStaleness()` method
- `internal/mcp/tool_impls.go` — Added index staleness to `getStatus` response
- `CLAUDE.md` — Added "Keeping Your Index Fresh" section

See [[Index-Management]] in the wiki for detailed documentation.

## [7.4.0]

### Added

#### Tool Presets & Pagination
Token-optimized tool discovery reducing context overhead by up to 83%:

**Presets:**
| Preset | Tools | Tokens | Use Case |
|--------|------:|-------:|----------|
| `core` (default) | 14 | ~1,531 | Essential navigation and analysis |
| `review` | 19 | ~2,294 | Code review: PR summary, ownership |
| `refactor` | 19 | ~2,216 | Refactoring: coupling, dead code |
| `docs` | 20 | ~2,093 | Documentation: coverage, staleness |
| `ops` | 25 | ~2,366 | Operations: jobs, webhooks, metrics |
| `federation` | 28 | ~3,122 | Multi-repo: cross-repo search |
| `full` | 76 | ~9,043 | All tools (legacy behavior) |

**Features:**
- **MCP-compliant pagination** — `tools/list` cursor-based pagination per spec
- **Core-first ordering** — Page 1 always contains functional toolset for non-paginating clients
- **Cursor invalidation** — Cursor rejected when preset or toolset changes
- **`expandToolset` meta-tool** — Dynamic preset expansion with rate limiting (once per session)
- **`tools.listChanged` capability** — Enables dynamic tool list updates

**CLI:**
```bash
ckb mcp                      # Default: core preset (14 tools)
ckb mcp --preset=review      # Code review workflow
ckb mcp --preset=full        # All 76 tools (legacy)
```

**Setup Wizard:**
- `ckb setup` now prompts for preset selection
- `--preset` flag for non-interactive configuration

**Files Added:**
- `internal/mcp/presets.go` — Preset definitions and core-first ordering
- `internal/mcp/cursor.go` — MCP-compliant cursor pagination

**Files Changed:**
- `internal/mcp/server.go` — Preset management and toolset hash
- `internal/mcp/handler.go` — Paginated `handleListTools`
- `internal/mcp/tools.go` — `expandToolset` tool definition
- `internal/mcp/tool_impls.go` — `expandToolset` handler with rate limiting
- `internal/mcp/capabilities.go` — `tools.listChanged: true`
- `cmd/ckb/mcp.go` — `--preset` flag
- `cmd/ckb/setup.go` — Preset selection in wizard

#### Wide-Result Metrics Tracking
Infrastructure for monitoring tool output sizes and truncation rates:

- **`getWideResultMetrics` tool** — Expose wide-result statistics
- **SQLite persistence** — Historical tracking for optimization work
- **Per-tool aggregation** — Invocations, bytes, tokens, truncations
- **Response byte tracking** — Actual JSON payload size for each tool response
- **`ckb metrics` CLI** — View aggregated metrics with `--days`, `--tool`, `--format` flags
- **`ckb metrics export`** — Export versioned metrics to JSON for cross-version comparison

Tracked tools: searchSymbols, findReferences, analyzeImpact, getCallGraph, getHotspots, summarizePr

**Telemetry Findings:**
| Tool | Truncation Rate | Needs Frontier? |
|------|-----------------|-----------------|
| searchSymbols | 45% | Yes |
| getHotspots | 50% | Yes |
| findReferences | 18% | No |
| getCallGraph | 0% | No |
| analyzeImpact | 0% | No |

**Files Added:**
- `internal/mcp/wide_result_metrics.go` — In-memory aggregation with DB persistence
- `internal/storage/metrics_store.go` — SQLite metrics storage
- `cmd/ckb/metrics.go` — CLI metrics command

### Performance

#### SCIP Backend Optimizations
Major performance improvements to the SCIP backend through pre-computed indexes:

| Operation | Before | After | Improvement |
|-----------|--------|-------|-------------|
| FindReferences | 340μs | 2.5μs | **136x faster** |
| SearchSymbols | 930μs | 136μs | **7x faster** |
| FindSymbolLocation | 70μs | 28ns | **2,500x faster** |
| GetCachedSymbol | 210ns | 7.5ns | **28x faster** |

**Changes:**
- **RefIndex**: Inverted reference index built during SCIP load for O(1) reference lookups instead of O(n×m) scans
- **ConvertedSymbols Cache**: Pre-converted symbols avoid repeated parsing of SCIP identifiers, visibility inference, and location lookups
- **ContainerIndex**: Maps occurrence positions to containing symbols for O(1) containment lookup instead of O(n²) nested loops
- **Fast Location Lookup**: `findSymbolLocationFast` uses RefIndex for O(k) definition lookup where k = number of occurrences
- **RateLimiter Cleanup**: Added graceful shutdown with `Stop()` method to prevent goroutine leaks

**Files Changed:**
- `internal/backends/scip/loader.go` — Added `OccurrenceRef`, `RefIndex`, `ConvertedSymbols`, `ContainerIndex` to `SCIPIndex`
- `internal/backends/scip/references.go` — `FindReferences` uses inverted index, added `findContainingSymbolFast`
- `internal/backends/scip/symbols.go` — Added `GetCachedSymbol`, `findSymbolLocationFast`, cached `SearchSymbols`
- `internal/backends/limiter.go` — Added `done` channel and `Stop()` method

**Tests Added:**
- `internal/backends/scip/performance_test.go` — 11 unit tests + 10 benchmarks
- `internal/backends/limiter_test.go` — 5 unit tests + 1 benchmark

#### Git Backend Optimizations
Major performance improvement to `getHotspots` by consolidating git commands:

| Operation | Before | After | Improvement |
|-----------|--------|-------|-------------|
| getHotspots | 26.7s | 498ms | **53x faster** |

**Problem:** For each changed file, ran 4 separate git commands (rev-list, shortlog, log × 2).
With 100+ files = 400+ process spawns.

**Solution:** Single `git log --format=%H|%an|%aI --numstat` command parses all data in one pass.

**Files Changed:**
- `internal/backends/git/churn.go` — Rewrote `GetHotspots` to use single git command

### Added

#### Standardized Response Envelope
All 76 MCP tool responses now include structured metadata in a consistent envelope format:

**Envelope Schema:**
```json
{
  "schemaVersion": "1.0",
  "data": { },
  "meta": {
    "confidence": { "score": 0.85, "tier": "medium", "reasons": [] },
    "provenance": { "backends": ["scip", "git"], "repoStateId": "..." },
    "freshness": { "indexAge": { "commitsBehind": 3, "staleReason": "behind-head" } },
    "truncation": { "isTruncated": true, "shown": 10, "total": 47, "reason": "max-symbols" }
  },
  "warnings": [],
  "suggestedNextCalls": [{ "tool": "findReferences", "params": {...}, "reason": "..." }]
}
```

**Key Features:**
- **Confidence Tiers** — Results scored as high (≥0.95), medium (0.70-0.94), low (0.30-0.69), or speculative (<0.30)
- **Provenance Tracking** — See which backends (SCIP, LSP, git) contributed to results
- **Freshness Info** — Know how stale your index is (commits behind, uncommitted changes)
- **Truncation Metadata** — See how many results were trimmed and why
- **Suggested Next Calls** — AI-friendly drilldown suggestions as structured tool calls
- **Cross-repo Marking** — Federation queries automatically marked as speculative tier

**Files Added:**
- `internal/envelope/envelope.go` — Core types (Response, Meta, Confidence, etc.)
- `internal/envelope/builder.go` — Fluent builder API
- `internal/envelope/confidence.go` — Score to tier mapping
- `internal/envelope/envelope_test.go` — Comprehensive tests
- `internal/mcp/tool_helpers.go` — Convenience wrappers for tool implementations
- `internal/mcp/tool_helpers_test.go` — Tool helper tests

**Files Changed:**
- `internal/mcp/tools.go` — Updated ToolHandler signature
- `internal/mcp/handler.go` — Updated handleCallTool for envelope format
- All `internal/mcp/tool_impls*.go` files — Refactored to return envelope responses

#### npm Update Notifications
Automatic update checking for npm installations:

- **Auto-detection** — Detects when running from `npm install -g @tastehub/ckb`
- **Non-blocking check** — Runs asynchronously, never delays command execution
- **24-hour cache** — Checks npm registry at most once per day
- **Silent failures** — Network timeouts (3s), errors, and offline mode fail silently
- **Protocol-safe** — Skips `mcp` and `serve` commands to avoid breaking protocols

**Disable with:**
```bash
export CKB_NO_UPDATE_CHECK=1
```

**Example output:**
```
╭─────────────────────────────────────────────────────╮
│  Update available: 7.3.0 → 7.4.0                    │
│  Run: npm update -g @tastehub/ckb                   │
╰─────────────────────────────────────────────────────╯
```

#### Hybrid Retrieval with PPR
Graph-based retrieval enhancement using Personalized PageRank:

**Results:**
| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Recall@10 | 62.1% | 100% | +61% |
| MRR | 0.546 | 0.914 | +67% |

**Components:**
- **Eval Suite** — `ckb eval` command measures recall@K, MRR, latency
- **PPR Algorithm** — Personalized PageRank over SCIP symbol graph
- **Seed Expansion** — Expands struct fields to include methods for better cross-module discovery
- **Combined Scoring** — FTS position + PPR score fusion (0.6/0.4 weights)

See Wiki for full documentation.

### Files Added
- `internal/update/check.go` — Core update check logic with npm registry API
- `internal/update/cache.go` — 24-hour cache in `~/.ckb/update-check.json`
- `internal/update/check_test.go` — Tests for version comparison and caching
- `cmd/ckb/eval.go` — Eval CLI command
- `internal/eval/suite.go` — Eval framework with metrics
- `internal/eval/fixtures/*.json` — Test fixtures
- `internal/graph/ppr.go` — PPR algorithm with seed expansion
- `internal/graph/builder.go` — Graph construction from SCIP
- `internal/query/ranking.go` — PPR-based reranking

## [7.3.0]

### Added

#### npm Package Improvements
Better npmjs.com presence and npx reliability:

- **README on npmjs.com** - Package now displays full README on npm registry
- **LICENSE included** - MIT license file bundled with npm package
- **Issue tracker link** - "Report a bug" link on npm page
- **npx sandbox fix** - Node shim auto-detects repo root, fixing #1 support issue

**How the npx fix works:**
The Node.js shim walks up from `process.cwd()` looking for `.ckb/` or `.git/` and sets `CKB_REPO` automatically. This means `npx @tastehub/ckb` now works from subdirectories and MCP clients that don't guarantee working directory.

#### Incremental Indexing v4 (Production-Grade)
Fast, reliable incremental indexing for large codebases:

**Delta Artifacts:**
- **`ckb diff` command** - Generate delta manifests between snapshots
- **CI-generated diffs** - O(delta) ingestion instead of O(N) comparison
- **Delta validation** - Schema version, base snapshot, hash verification
- **`POST /delta/ingest`** - Ingest delta artifacts via API
- **`POST /delta/validate`** - Validate without applying

**FTS5 Search:**
- **SQLite FTS5** - Instant full-text search (replaces LIKE scans)
- **Automatic triggers** - Real-time sync with symbol changes
- **FTS maintenance** - Rebuild, vacuum, integrity-check
- **LIKE fallback** - Graceful degradation for edge cases

**Operational Hardening:**
- **Compaction scheduler** - Automatic snapshot cleanup, journal pruning, FTS vacuum
- **`GET /health/detailed`** - Per-repo metrics, storage info, memory usage
- **`GET /metrics`** - Prometheus metrics (counters, histograms, gauges)
- **Load shedding** - Priority endpoints, circuit breakers, adaptive shedding

#### Language Quality Assessment
Per-language quality metrics and environment detection:

**Quality Tiers:**
- **Tier 1 (Full)** - Go: full support, all features, stable
- **Tier 2 (Standard)** - TypeScript, JavaScript, Python: full support, known edge cases
- **Tier 3 (Basic)** - Rust, Java, Kotlin, C++, Ruby, Dart: basic support, callgraph may be incomplete
- **Tier 4 (Experimental)** - C#, PHP: experimental

**New Endpoints:**
- **`GET /meta/languages`** - Language quality dashboard with tier info, metrics, recommendations
- **`GET /meta/python-env`** - Python venv detection with activation recommendations
- **`GET /meta/typescript-monorepo`** - TypeScript monorepo detection (pnpm, lerna, nx, yarn)

**Environment Detection:**
- Python virtual environment detection (`.venv`, `venv`, `env`, `VIRTUAL_ENV`)
- Python package managers (pyproject.toml, requirements.txt, Pipfile)
- TypeScript monorepo workspaces with per-package tsconfig status

#### Remote Federation Client (v3 Federation Phase 5)
Connect to remote CKB index servers and query them alongside local repositories—transforming federation from local-only aggregation to a distributed code intelligence network.

**Core Features:**
- **Remote Server Management** — Add, remove, enable, disable remote CKB index servers
- **Hybrid Queries** — Search symbols across local federation repos AND remote servers in parallel
- **Source Attribution** — Results show whether they came from "local" or a named remote server
- **Graceful Degradation** — Queries succeed even when some remotes are unavailable

**Caching:**
- Repository list cached for 1 hour
- Metadata cached for 1 hour
- Symbol searches cached for 15 minutes
- Refs and call graph always fresh (not cached)
- Configurable per-server cache TTL

**HTTP Client:**
- Bearer token authentication with environment variable expansion (`${VAR}`)
- Exponential backoff retry logic (max 3 retries)
- Configurable timeouts per server
- Response body limiting (10MB max)

**CLI Commands:**
```bash
# Add a remote CKB index server
ckb federation add-remote <federation> <name> --url=<url> [--token=<token>] [--cache-ttl=1h] [--timeout=30s]

# Remove a remote server
ckb federation remove-remote <federation> <name>

# List remote servers
ckb federation list-remote <federation> [--json]

# Sync metadata from remote server(s)
ckb federation sync-remote <federation> [name] [--json]

# Check remote server status
ckb federation status-remote <federation> <name> [--json]

# Enable/disable remote server
ckb federation enable-remote <federation> <name>
ckb federation disable-remote <federation> <name>
```

**MCP Tools (7 new):**
- `federationAddRemote` — Add a remote server to a federation
- `federationRemoveRemote` — Remove a remote server
- `federationListRemote` — List remote servers in a federation
- `federationSyncRemote` — Sync metadata from remote servers
- `federationStatusRemote` — Get status of a remote server
- `federationSearchSymbolsHybrid` — Search symbols across local + remote
- `federationListAllRepos` — List repos from local and remote sources

**Configuration:**
```toml
[[remote_servers]]
name = "prod"
url = "https://ckb.company.com"
token = "${CKB_PROD_TOKEN}"   # Environment variable expansion
cache_ttl = "1h"
timeout = "30s"
enabled = true
```

#### Authentication & API Keys (v3 Federation Phase 4)
Scoped API key authentication for the index server, enabling secure multi-tenant access with fine-grained permissions.

**Scoped API Keys:**
- **read** — GET requests, symbol lookup, search
- **write** — POST requests, upload indexes, create repos
- **admin** — Full access including token management and deletions

**Per-Repository Restrictions:**
- Limit keys to specific repos using glob patterns (e.g., `myorg/*`)
- Prevents cross-tenant data access in shared deployments

**Rate Limiting:**
- Token bucket algorithm with configurable limits per key
- Returns `429 Too Many Requests` with `Retry-After` header
- Customizable default limits and burst sizes

**Token Management CLI:**
```bash
# Create a new token
ckb token create --name "CI Upload" --scopes write
ckb token create --name "Read-only" --scopes read --repos "myorg/*"
ckb token create --name "Admin" --scopes admin --expires 30d

# List all tokens
ckb token list
ckb token list --show-revoked

# Revoke a token
ckb token revoke ckb_key_abc123

# Rotate a token (new secret, same ID)
ckb token rotate ckb_key_abc123
```

**Token Format:**
- Token: `ckb_sk_` prefix + 64 hex chars (shown once at creation)
- Key ID: `ckb_key_` prefix + 16 hex chars (used for management)
- Secure bcrypt hashing for storage

**Configuration:**
```toml
[index_server.auth]
enabled = true
require_auth = true                    # false = unauthenticated gets read-only
legacy_token = "${CKB_LEGACY_TOKEN}"   # Backward compatibility

[[index_server.auth.static_keys]]
id = "ci-upload"
name = "CI Upload Key"
token = "${CI_CKB_TOKEN}"
scopes = ["write"]
repo_patterns = ["myorg/*"]
rate_limit = 100

[index_server.auth.rate_limiting]
enabled = true
default_limit = 60                     # Requests per minute
burst_size = 10
```

**HTTP Headers:**
- `Authorization: Bearer <token>` — Authentication
- `X-RateLimit-Key: <key_id>` — Rate limit tracking (response)
- `Retry-After: <seconds>` — When rate limited (response)

**Error Responses:**
- `401 Unauthorized` — Missing/invalid/expired/revoked token
- `403 Forbidden` — Insufficient scope or repo not allowed
- `429 Too Many Requests` — Rate limited

**Backward Compatibility:**
- Legacy single-token mode still works via `legacy_token` config
- When `require_auth = false`, unauthenticated requests get read-only access

#### Enhanced Uploads (v3 Federation Phase 3)
Compression support, progress reporting, and incremental (delta) updates for the index upload system. Reduces upload sizes by 70-90% for typical updates.

**Compression Support:**
- **gzip** — `Content-Encoding: gzip` for 60-80% compression
- **zstd** — `Content-Encoding: zstd` for 70-90% compression (faster than gzip)
- Automatic decompression on the server
- Response includes `compression_ratio` showing savings

**Progress Reporting:**
- Logs progress at 10MB intervals for large uploads
- Includes bytes received, MB count, and percentage when Content-Length is known

**Delta Uploads (Incremental):**
- `POST /index/repos/{repo}/upload/delta` — Upload only changed files
- Requires `X-CKB-Base-Commit` header matching current index
- Returns 409 Conflict with `current_commit` if base doesn't match
- Suggests full upload when >50% files changed (configurable)
- Reuses existing incremental infrastructure for efficient processing

**Configuration:**
```toml
[index_server]
enable_compression = true           # Default true
supported_encodings = ["gzip", "zstd"]
enable_delta_upload = true          # Default true
delta_threshold_percent = 50        # Suggest full upload if >N% changed
```

**Delta Upload Example:**
```bash
curl -X POST http://localhost:8080/index/repos/company/core-lib/upload/delta \
  -H "Content-Type: application/octet-stream" \
  -H "Content-Encoding: gzip" \
  -H "X-CKB-Base-Commit: abc123" \
  -H "X-CKB-Target-Commit: def456" \
  -H 'X-CKB-Changed-Files: [{"path":"src/main.go","change_type":"modified"}]' \
  --data-binary @partial-index.scip.gz
```

#### Index Upload (v3 Federation Phase 2)
Push SCIP indexes to the index server via HTTP, eliminating the need for local filesystem paths. This transforms CKB from a "bring your database" model to a centralized index hosting service.

**REST API Endpoints:**
- `POST /index/repos` — Create a new repo ready for upload
- `POST /index/repos/{repo}/upload` — Upload SCIP index file (supports gzip, zstd compression)
- `POST /index/repos/{repo}/upload/delta` — Delta upload (incremental changes only)
- `DELETE /index/repos/{repo}` — Delete an uploaded repo

**Upload Features:**
- Stream large files (100MB+) without memory issues
- Auto-create repos on first upload (configurable)
- Metadata headers: `X-CKB-Commit`, `X-CKB-Language`, `X-CKB-Indexer-Name`
- Full SCIP processing: symbols, refs, call graph extraction
- Compression support: gzip and zstd
- Progress logging for large uploads

**Configuration:**
```toml
[index_server]
enabled = true
data_dir = "~/.ckb-server"      # Server data directory
max_upload_size = 524288000     # 500MB default
allow_create_repo = true        # Allow repo creation via API
enable_compression = true       # Accept compressed uploads
enable_delta_upload = true      # Enable incremental updates
```

**Data Directory Structure:**
```
~/.ckb-server/
├── repos/
│   └── company-core-lib/
│       ├── ckb.db        # SQLite database
│       └── meta.json     # Repo metadata
└── uploads/              # Temp directory for uploads
```

#### Remote Index Serving (v3 Federation Phase 1)
Serve symbol indexes over HTTP for remote federation clients. This enables cross-repository code intelligence without requiring clients to have direct database access.

**Core Features:**
- **Index Server Mode** — New `--index-server` flag for `ckb serve` enables remote index endpoints
- **Multi-Repo Support** — Serve multiple repositories from a single CKB instance
- **TOML Configuration** — Configure repos, privacy settings, and pagination limits via config file
- **Read-Only Connections** — Index server opens databases in read-only mode for safety

**REST API Endpoints:**
- `GET /index/repos` — List all indexed repositories
- `GET /index/repos/{repo}/meta` — Repository metadata and capabilities
- `GET /index/repos/{repo}/files` — List files with cursor pagination
- `GET /index/repos/{repo}/symbols` — List symbols with filtering and pagination
- `GET /index/repos/{repo}/symbols/{id}` — Get single symbol by ID
- `POST /index/repos/{repo}/symbols:batchGet` — Batch get multiple symbols
- `GET /index/repos/{repo}/refs` — List references (call edges) with pagination
- `GET /index/repos/{repo}/callgraph` — List call graph edges with filtering
- `GET /index/repos/{repo}/search/symbols` — Search symbols by name
- `GET /index/repos/{repo}/search/files` — Search files by path

**Security & Privacy:**
- **HMAC-Signed Cursors** — Pagination cursors are signed to prevent tampering
- **Privacy Redaction** — Per-repo controls for exposing paths, docs, and signatures
- **Path Prefix Stripping** — Remove sensitive path prefixes from responses

**CLI:**
- `ckb serve --index-server` — Enable index-serving endpoints
- `ckb serve --index-config <path>` — Load configuration from TOML file

**Configuration Example:**
```toml
[index_server]
enabled = true
max_page_size = 10000

[[repos]]
id = "company/core-lib"
name = "Core Library"
path = "/repos/core-lib"

[default_privacy]
expose_paths = true
expose_docs = true
expose_signatures = true
```

#### Doc-Symbol Linking
Bridge documentation and code with automatic symbol detection:

**Core Features:**
- **Backtick detection** - Automatically detect `Symbol.Name` references in markdown
- **Directive support** - `<!-- ckb:symbol -->` for explicit references, `<!-- ckb:module -->` for module linking
- **Suffix resolution** - Resolve `UserService.Auth` to full SCIP symbol ID with confidence scoring
- **Staleness detection** - Find broken references when symbols are deleted or renamed

**v1.1 Enhancements:**
- **CI enforcement** - `--fail-under` flag for `ckb docs coverage` to enforce minimum coverage in CI
- **Rename detection** - Detect when documented symbols are renamed via alias chain, suggest new names
- **known_symbols directive** - `<!-- ckb:known_symbols Engine, Start -->` allows single-segment detection
- **Fence symbol scanning** - Extract identifiers from fenced code blocks using tree-sitter (8 languages)

**CLI Commands:**
- `ckb docs index` - Scan and index documentation for symbol references
- `ckb docs symbol <name>` - Find docs referencing a symbol
- `ckb docs file <path>` - Show symbols in a document
- `ckb docs stale [path]` - Check for stale references (or `--all` for all docs)
- `ckb docs coverage` - Documentation coverage statistics
- `ckb docs module <id>` - Find docs linked to a module

**MCP Tools:**
- `indexDocs` - Scan and index documentation
- `getDocsForSymbol` - Find docs referencing a symbol
- `getSymbolsInDoc` - List symbols in a document
- `getDocsForModule` - Find docs linked to a module
- `checkDocStaleness` - Check for stale references
- `getDocCoverage` - Coverage statistics

#### Multi-Repo Management
Quick context switching between multiple repositories in MCP sessions:

**Core Features:**
- **Global registry** - Named repo shortcuts stored at `~/.ckb/repos.json`
- **Smart --repo flag** - Auto-detects if argument is a path or registry name
- **Multi-engine support** - Up to 5 engines in memory with LRU eviction
- **Per-repo config** - Each engine loads its own `.ckb/config.json`
- **Repo state tracking** - `valid`, `uninitialized`, `missing` states

**CLI Commands:**
- `ckb repo add [name] [path]` - Register a repository (path defaults to cwd)
- `ckb repo list` - List repos grouped by state
- `ckb repo remove <name>` - Unregister a repo
- `ckb repo rename <old> <new>` - Rename a repo alias
- `ckb repo default [name]` - Get or set default repo
- `ckb repo info [name]` - Show detailed repo info
- `ckb repo which` - Print current repo (for scripts)
- `ckb repo check` - Validate all registered repos

**MCP Tools:**
- `listRepos` - List registered repos with state and active status
- `switchRepo` - Switch active repo context
- `getActiveRepo` - Get current repo info

**Command Flags:**
- `ckb mcp --repo <name>` - Start MCP with specific repo active
- `ckb serve --repo <name>` - Start HTTP server for specific repo

#### Incremental Indexing (Go only)
Index updates in seconds instead of full reindex—O(changed files) instead of O(entire repo).

**Core Features:**
- **Git-based change detection** — Uses `git diff -z` with NUL separators for accurate tracking
- **Rename support** — Properly tracks `git mv` with old path cleanup
- **Delta extraction** — Only processes SCIP documents for changed files
- **Delete+insert pattern** — Clean updates without complex diffing logic
- **Index state tracking** — Tracks "partial" vs "full" state with staleness warnings

#### Incremental Callgraph (v1.1)
Extends incremental indexing with call graph maintenance—outgoing calls from changed files are always accurate.

- **Call edge extraction** — Extracts caller→callee edges during incremental updates
- **Tiered callable detection** — Uses `SymbolInformation.Kind` first, falls back to `().` heuristic
- **Caller resolution** — Resolves enclosing function for each call site via line range matching
- **Location-anchored storage** — Call edges stored with `(caller_file, line, col, callee_id)` for precision
- **Caller-owned edges** — Edges deleted and rebuilt with their owning file (no stale outgoing calls)

#### Transitive Invalidation (v2)
Tracks file-level dependencies and automatically queues dependent files for rescanning when their dependencies change.

- **File dependency tracking** — `file_deps` table tracks which files reference symbols from other files
- **Rescan queue** — `rescan_queue` table with BFS depth tracking and attempt counting
- **Four invalidation modes:**
  - `none` — Disabled (no dependency tracking)
  - `lazy` — Enqueue dependents, drain on next full reindex (default)
  - `eager` — Enqueue and drain immediately with configurable budgets
  - `deferred` — Enqueue and drain periodically in background
- **Budget-limited draining** — `MaxRescanFiles` (default: 200) and `MaxRescanMs` (default: 1500ms) limits
- **Cascade depth control** — `Depth` setting limits BFS traversal (default: 1 = direct dependents only)

**Accuracy Guarantees:**
| Query Type | After Incremental | After Queue Drained |
|------------|-------------------|---------------------|
| Go to definition | Always accurate | Always accurate |
| Find refs FROM changed files | Always accurate | Always accurate |
| Find refs TO changed symbols | May be stale | Accurate |
| Call graph (callees/outgoing) | Always accurate | Always accurate |
| Call graph (callers/incoming) | May be stale | Accurate |

**Automatic Fallback:**
- Falls back to full reindex when >50% files changed
- Falls back on schema version mismatch
- Falls back when no tracked commit exists

**CLI Changes:**
- `ckb index` — Incremental by default for Go projects
- `ckb index --force` — Force full reindex when accuracy is critical

**Configuration (`.ckb/config.json`):**
```json
{
  "incremental": {
    "threshold": 50,
    "indexTests": false,
    "excludes": ["vendor", "testdata"]
  },
  "transitive": {
    "enabled": true,
    "mode": "lazy",
    "depth": 1,
    "maxRescanFiles": 200,
    "maxRescanMs": 1500
  }
}
```

### Files Added

**Incremental Indexing v4:**
- `internal/diff/` - Delta artifact generation
  - `types.go` - Delta JSON schema types
  - `generator.go` - Delta generation (compare two DBs)
  - `validator.go` - Delta validation logic
  - `hasher.go` - Canonical hash computation
- `internal/storage/fts.go` - FTS5 maintenance (rebuild, vacuum, integrity-check)
- `internal/daemon/compaction.go` - Compaction scheduler
- `internal/api/metrics.go` - Prometheus metrics exporter
- `internal/api/middleware_load.go` - Load shedding middleware
- `internal/api/handlers_delta.go` - Delta ingestion endpoints
- `cmd/ckb/diff.go` - `ckb diff` CLI command

**Language Quality:**
- `internal/project/quality.go` - Language quality assessment module
- `internal/api/handlers_quality.go` - Language quality API endpoints

**Remote Federation Client:**
- `internal/federation/` - Remote federation client
  - `remote_types.go` — Response types matching index server API
  - `remote_config.go` — Remote server configuration and env var expansion
  - `remote_client.go` — HTTP client with retry logic and all API methods
  - `remote_cache.go` — Caching wrapper with TTL management
  - `hybrid.go` — Local + remote query merging engine
  - `remote_test.go` — Tests for remote client and configuration
- `cmd/ckb/federation_remote.go` - CLI commands for remote federation
- `internal/mcp/tool_impls_v74.go` - MCP tool implementations for remote federation
- `internal/api/` - Remote index serving and upload
  - `index_config.go` — Configuration types and TOML loading (Phase 3: compression, delta config)
  - `index_types.go` — API response types
  - `index_cursor.go` — HMAC-signed cursor pagination
  - `index_repos.go` — Repository handle management (Phase 1 + 2 + 3)
  - `index_redaction.go` — Privacy redaction logic
  - `index_queries.go` — Database queries for symbols, files, refs, callgraph
  - `index_storage.go` — Server data directory management (Phase 2)
  - `index_processor.go` — SCIP processing pipeline (Phase 2 + 3 delta processing)
  - `handlers_index.go` — HTTP handlers for all index endpoints
  - `handlers_upload.go` — HTTP handlers with compression/progress (Phase 2 + 3)
  - `handlers_upload_delta.go` — Delta upload handler (Phase 3)
  - `handlers_index_test.go` — Tests for cursors, redaction, handlers
  - `handlers_upload_test.go` — Tests for upload, compression, delta (Phase 2 + 3)

**Doc-Symbol Linking:**
- `internal/docs/` - New package for doc-symbol linking
  - `types.go` - Core types (Document, DocReference, StalenessReport, etc.)
  - `scanner.go` - Markdown scanning with backtick/directive/fence detection
  - `resolver.go` - Symbol resolution with suffix matching
  - `staleness.go` - Staleness checking with rename detection
  - `indexer.go` - Document indexing orchestration
  - `store.go` - SQLite persistence for documents and references
  - `coverage.go` - Coverage analysis
  - `fence_parser.go` - Tree-sitter identifier extraction from fences
- `cmd/ckb/docs.go` - CLI commands
- `internal/query/docs.go` - Query engine integration
- `internal/mcp/handlers_docs.go` - MCP tool handlers
- `internal/incremental/` — New package for incremental indexing
  - `types.go` — Core types (FileState, ChangeSet, FileDelta, DeltaStats, CallEdge, TransitiveConfig)
  - `store.go` — SQLite persistence for indexed_files, file_symbols, index_meta
  - `detector.go` — Git-based and hash-based change detection
  - `extractor.go` — SCIP delta extraction for changed files only
  - `updater.go` — Database updates with delete+insert pattern
  - `deps.go` — Transitive invalidation with file dependency tracking and rescan queue
  - `indexer.go` — Orchestration and state management
  - `indexer_test.go`, `deps_test.go`, `types_test.go` — Tests

### Changed
- `internal/federation/config.go` — Added RemoteServers field to Config struct
- `internal/federation/index.go` — Schema v3 with remote_servers, remote_repos, remote_cache tables
- `internal/mcp/tools.go` — Registered 7 new MCP tools for remote federation
- `internal/api/server.go` — Added IndexRepoManager, NewServer now returns error
- `internal/api/routes.go` — Added /index/* route registration
- `cmd/ckb/serve.go` — Added --index-server and --index-config flags
- `internal/storage/schema.go` — Schema v8 with callgraph, file_deps, and rescan_queue tables
- `cmd/ckb/index.go` — Incremental indexing flow with `--force` flag

## [7.2.0]

### Added

#### `ckb setup` - Multi-Tool MCP Configuration
- Interactive setup wizard for configuring CKB with AI coding tools
- Support for 6 AI tools:
  - **Claude Code** - `.mcp.json` (project) or `claude mcp add` (global)
  - **Cursor** - `.cursor/mcp.json` (project/global)
  - **Windsurf** - `~/.codeium/mcp_config.json` (global only)
  - **VS Code** - `.vscode/mcp.json` (project) or `code --add-mcp` (global)
  - **OpenCode** - `opencode.json` (project/global)
  - **Claude Desktop** - Platform-specific paths (global only)
- `--tool` flag to skip interactive menu
- `--npx` flag for portable npx-based setup
- Windows path support for Windsurf and Claude Desktop

#### `ckb index` - Extended Language Support
- Added 5 new languages:
  - **C/C++** via scip-clang with `--compdb` flag for compile_commands.json
  - **Dart** via scip-dart
  - **Ruby** via scip-ruby with sorbet/config validation
  - **C#** via scip-dotnet with *.csproj detection
  - **PHP** via scip-php with vendor/bin check
- Bounded-depth glob scanning for nested project detection
- Language-specific validation and prerequisite checks

#### Smart Indexing
- **Skip-if-fresh**: `ckb index` automatically skips reindexing when index matches current repo state
- **Freshness tracking**: Detects commits behind HEAD and uncommitted changes to tracked files
- **Index metadata**: Persists index info to `.ckb/index-meta.json` (commit hash, file count, duration)
- **Lock file**: Prevents concurrent indexing with flock-based `.ckb/index.lock`

#### `ckb status` - Index Freshness Display
- New "Index Status" section showing freshness with commit hash
- Shows stale reasons: "3 commit(s) behind HEAD", "uncommitted changes detected"
- Displays file count for fresh indexes

#### `ckb mcp --watch` - Auto-Reindex Mode
- New `--watch` flag for poll-based auto-reindexing
- Polls every 30 seconds, reindexes when stale
- Uses lock file to prevent conflicts with manual `ckb index`
- Logs reindex activity to stderr

#### Explicit Analysis Tiers
- User-controllable analysis tiers: **fast**, **standard**, **full**
- CLI flag: `ckb search "foo" --tier=fast`
- Environment variable: `CKB_TIER=standard`
- Config file: Add `"tier": "standard"` to `.ckb/config.json`
- Tier display in `ckb status` shows mode (explicit vs auto-detected)
- Precedence: CLI flag > env var > config > auto-detect

#### `ckb doctor --tier` - Tier-Aware Diagnostics
- New `--tier` flag for tier-specific tool requirement checks
- Shows per-language tool status (installed, version, path)
- Displays missing tools with OS-specific install commands
- Validates prerequisites (go.mod, package.json, Cargo.toml, etc.)
- Accepts both naming conventions: `basic`/`fast`, `enhanced`/`standard`, `full`
- Capability matrix showing which features are available per language
- JSON output with `--format json` for scripting

### Changed

- Tier names rebranded: Basic → **Fast**, Enhanced → **Standard**, Full → **Full**

- Multi-language detection now errors instead of silently defaulting to a language

### Fixed

- Fixed Kotlin indexer URL in documentation
- Fixed PHP indexer URL in documentation

## [7.1.0] - 2024-12-XX

Zero-Friction Operation - CKB v7.1 enables code intelligence without requiring a SCIP index upfront.

### Added

#### Tree-sitter Symbol Fallback
- Symbol extraction for 8 languages (Go, TypeScript, JavaScript, TSX, Python, Rust, Java, Kotlin)
- `searchSymbols` works without SCIP index
- Results include `Source: "treesitter"` and `Confidence: 0.7` for transparency

#### `ckb index` Command
- Auto-detects project language from manifests (go.mod, package.json, Cargo.toml, etc.)
- Checks if SCIP indexer is installed, shows install instructions if not
- `--force` flag for re-indexing, `--dry-run` to preview
- Language-specific troubleshooting tips on failure

#### Universal MCP Documentation
- Setup instructions for Claude Code, Cursor, Windsurf, VS Code, OpenCode, Claude Desktop
- Windows `cmd /c` wrapper instructions

### Files Added
- `internal/symbols/treesitter.go` - Tree-sitter symbol extraction
- `internal/symbols/treesitter_test.go` - Tests for all 8 languages
- `internal/project/detect.go` - Language and indexer detection

## [7.0.0] - 2024-12-XX

### Added
- Initial npm package release via `@tastehub/ckb`
- 58 MCP tools for code intelligence

## [6.5.0] - 2024-12-XX

### Added

#### Developer Intelligence
- **Symbol Origins** — `explainOrigin`: Why does this code exist? Git history, linked issues/PRs
- **Co-change Coupling** — `analyzeCoupling`: Find files that historically change together
- **LLM Export** — `exportForLLM`: Token-efficient codebase summaries with importance ranking
- **Risk Audit** — `auditRisk`: 8-factor scoring (complexity, coverage, bus factor, security, staleness, errors, coupling, churn)

## [6.4.0] - 2024-12-XX

### Added

#### Runtime Observability
- **OpenTelemetry Integration** — `getTelemetryStatus`: See real call counts, not just static analysis
- **Dead Code Confidence** — `findDeadCodeCandidates`: Find symbols with zero runtime calls
- **Observed Callers** — `getObservedUsage`: Enrich impact analysis with production data

## [6.3.0] - 2024-12-XX

### Added

#### Contract-Aware Analysis
- **API Boundary Detection** — `listContracts`: Protobuf and OpenAPI contract discovery
- **Consumer Tracking** — Three evidence tiers for cross-repo dependencies
- **Cross-Repo Impact** — `analyzeContractImpact`: "What breaks if I change this shared API?"
- **Contract Dependencies** — `getContractDependencies`: See consumers and dependencies

## [6.2.0] - 2024-12-XX

### Added

#### Federation & Cross-Repository
- **Federation** — Query across multiple repos organization-wide
- **Federation Tools** — `listFederations`, `federationStatus`, `federationSearchModules`, `federationSearchOwnership`, `federationGetHotspots`
- **Daemon Mode** — Always-on service with HTTP API, scheduled tasks, file watching, webhooks
- **Daemon Tools** — `daemonStatus`, `listSchedules`, `listWebhooks`
- **Tree-sitter Complexity** — `getFileComplexity`: Language-agnostic cyclomatic/cognitive complexity for 7 languages

## [6.1.0] - 2024-12-XX

### Added

#### Production Ready
- **Background Jobs** — Queue long operations, track progress, cancel jobs
- **Job Tools** — `getJobStatus`, `listJobs`, `cancelJob`
- **CI/CD Integration** — `summarizePr`: PR risk analysis, ownership drift detection
- **Ownership Drift** — `getOwnershipDrift`: CODEOWNERS vs actual ownership

## [6.0.0] - 2024-12-XX

### Added

#### Architectural Memory
- **Ownership Intelligence** — `getOwnership`: CODEOWNERS + git blame with time-weighted analysis
- **Module Responsibilities** — `getModuleResponsibilities`: What does this module do?
- **Architectural Decisions** — `recordDecision`, `getDecisions`: ADRs with full-text search
- **Module Annotations** — `annotateModule`: Add module metadata
- **Architecture Refresh** — `refreshArchitecture`: Rebuild architectural model

## [5.2.0] - 2024-12-XX

### Added

#### Discovery & Flow
- **Usage Tracing** — `traceUsage`: How is this symbol reached?
- **Entrypoints** — `listEntrypoints`: System entrypoints (API, CLI, jobs)
- **File Orientation** — `explainFile`: File-level orientation
- **Path Explanation** — `explainPath`: Why does this path exist?
- **Diff Summary** — `summarizeDiff`: What changed, what might break?
- **Architecture Overview** — `getArchitecture`: Module dependency overview
- **Hotspots** — `getHotspots`: Volatile areas with trends
- **Key Concepts** — `listKeyConcepts`: Domain concepts in codebase
- **Recently Relevant** — `recentlyRelevant`: What matters now?

## [5.1.0] - 2024-12-XX

### Added

#### Core Navigation
- **Symbol Search** — `searchSymbols`: Find symbols by name with filtering
- **Symbol Details** — `getSymbol`: Get symbol details
- **References** — `findReferences`: Find all usages
- **Symbol Explanation** — `explainSymbol`: AI-friendly symbol explanation
- **Symbol Justification** — `justifySymbol`: Keep/investigate/remove verdict
- **Call Graph** — `getCallGraph`: Caller/callee relationships
- **Module Overview** — `getModuleOverview`: Module statistics
- **Impact Analysis** — `analyzeImpact`: Change risk analysis
- **System Status** — `getStatus`: System health
- **Diagnostics** — `doctor`: System diagnostics

# Changelog

All notable changes to CKB will be documented in this file.

## [7.3.0] - 2024-12-22

### Added

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
- `internal/api/server.go` — Added IndexRepoManager, NewServer now returns error
- `internal/api/routes.go` — Added /index/* route registration
- `cmd/ckb/serve.go` — Added --index-server and --index-config flags
- `internal/storage/schema.go` — Schema v8 with callgraph, file_deps, and rescan_queue tables
- `cmd/ckb/index.go` — Incremental indexing flow with `--force` flag

## [7.2.0] - 2024-12-21

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

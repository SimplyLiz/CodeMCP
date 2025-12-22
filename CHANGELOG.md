# Changelog

All notable changes to CKB will be documented in this file.

## [7.3.0] - 2024-12-22

### Added

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

### Files Added
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

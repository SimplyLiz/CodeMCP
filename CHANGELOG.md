# Changelog

All notable changes to CKB will be documented in this file.

## [7.2.0] - Unreleased

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

### Changed

- Multi-language detection now errors instead of silently defaulting to a language

### Fixed

- Fixed Kotlin indexer URL in documentation
- Fixed PHP indexer URL in documentation

## [7.1.0] - 2024-12-XX

### Added
- npm distribution via `@tastehub/ckb`
- `ckb setup` command for Claude Code integration

## [7.0.0] - 2024-12-XX

### Added
- Initial npm package release
- 58 MCP tools for code intelligence

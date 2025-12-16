# CKB Tasks

## High Priority

### Fix SCIP Index Warnings
- [ ] Regenerate SCIP index with repository info to fix "stale" warning
- [ ] Clean up symbol names (currently showing commit hash in display name)
- [ ] Populate module IDs in search results

### SQLite Cache Population
- [ ] Wire SCIP adapter to populate SQLite cache on index load
- [ ] Implement cache invalidation when SCIP index changes
- [ ] Fix doctor "Database tables not initialized" warning

### LSP Backend
- [ ] Add gopls configuration for Go language support
- [ ] Implement LSP server auto-discovery
- [ ] Add LSP server health monitoring
- [ ] Document LSP configuration in config.json

## Medium Priority

### Testing
- [ ] Add tests for `internal/api/` (HTTP handlers)
- [ ] Add tests for `internal/mcp/` (MCP server tools)
- [ ] Add tests for `cmd/ckb/` (CLI commands)
- [ ] Add integration tests (end-to-end CLI testing)

### Documentation
- [ ] Write README.md with:
  - Project overview
  - Installation instructions
  - Quick start guide
  - CLI command reference
  - Configuration guide
- [ ] Document MCP integration for Claude Code
- [ ] Add inline code documentation where missing

### Error Handling
- [ ] Improve error messages for common failures
- [ ] Add retry logic for transient backend failures
- [ ] Better handling when no backends are available

## Low Priority

### Performance
- [ ] Add query result caching
- [ ] Implement lazy loading for large SCIP indexes
- [ ] Profile and optimize hot paths
- [ ] Add benchmarks

### Features
- [ ] Add `ckb watch` command for continuous indexing
- [ ] Implement `--wait-for-ready` flag on status command
- [ ] Add support for multiple SCIP indexes (monorepo)
- [ ] Implement code search (grep-like with semantic awareness)
- [ ] Add `ckb explain` command for symbol documentation

### CI/CD
- [ ] Add GitHub Actions workflow
- [ ] Automated testing on push
- [ ] Release automation
- [ ] Binary builds for multiple platforms

### Polish
- [ ] Add color output support for terminal
- [ ] Progress indicators for long operations
- [ ] Tab completion for CLI
- [ ] Config file validation on load

## Completed

- [x] Core Query Engine
- [x] Backend Ladder (SCIP -> LSP -> Git fallback)
- [x] CLI commands wired to Query Engine
- [x] HTTP API handlers wired to Query Engine
- [x] MCP server tools wired to Query Engine
- [x] SCIP index generation
- [x] Human-readable output formatting
- [x] Query Engine tests
- [x] Init command with --force flag
- [x] Module path cleanup (removed github.com prefix)

## Architecture Notes

### Current Backend Status
| Backend | Status | Notes |
|---------|--------|-------|
| SCIP | Working | 2,897 symbols indexed |
| LSP | Not configured | Needs gopls setup |
| Git | Working | Blame, history, churn |

### Key Files
- `internal/query/engine.go` - Main Query Engine
- `internal/backends/scip/` - SCIP adapter
- `internal/backends/lsp/` - LSP supervisor
- `internal/backends/git/` - Git adapter
- `cmd/ckb/` - CLI implementation
- `internal/api/` - HTTP API
- `internal/mcp/` - MCP server

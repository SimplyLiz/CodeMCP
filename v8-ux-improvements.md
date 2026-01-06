# v8.0 UX Improvements

Working document for v8.0 UX-focused improvements.

## Current Pain Points

### 1. Active Repository Visibility (High Priority)

**Problem:** It's difficult to know which repository is currently active in CKB, especially:
- When using MCP with Claude Code or other AI tools
- When switching between multiple projects
- No visual indicator in shell or status output

**Current state:**
- `ckb repo which` prints the default repo (but requires separate command)
- MCP has `listRepos` tool but it's not discoverable
- `ckb status` doesn't show the active repo prominently
- No shell integration (PS1/prompt helper)

**Proposed solutions:**
- [ ] Add prominent active repo display to `ckb status` output (first line)
- [ ] Add `CKB_ACTIVE_REPO` environment variable that MCP server sets
- [ ] Shell integration: `ckb prompt` for PS1/prompt scripts
- [ ] MCP: Include active repo in every tool response header (or status field)

---

### 2. Repository Switching (High Priority)

**Problem:** Switching repos is awkward:
- Must use `ckb repo use <name>` but this only sets the default for future sessions
- In MCP sessions, need to call `switchRepo` tool but not obvious it exists
- No fuzzy matching for repo names
- No recent repos list

**Current state:**
- CLI: `ckb repo use <name>`, `ckb repo default <name>`
- MCP: `switchRepo` tool, `listRepos` tool
- No quick-switch mechanism

**Proposed solutions:**
- [ ] Add `ckb use <name>` as top-level shortcut (not just `repo use`)
- [ ] Fuzzy matching for repo names
- [ ] `ckb recent` to show recently used repos
- [ ] MCP: Suggest available repos in getStatus response when no repo active

---

### 3. Unified Status View (High Priority)

**Problem:** `ckb status` doesn't give a complete picture of what's running:
- Missing: daemon status, MCP sessions, active jobs, watch mode
- Need to run `ckb daemon status` separately
- No way to see all running CKB processes

**Current `ckb status` shows:**
- CKB version
- Tier info
- Repo state (branch, commit)
- Backends (SCIP, LSP, Git) - availability and health
- Cache stats
- Index freshness
- Change impact availability

**Missing from status:**
- Active repository name (should be first line!)
- Daemon running? (PID, port, uptime)
- Any MCP sessions active?
- Watch mode active?
- Active jobs/background tasks
- Available/active tools

**Proposed new `ckb status` output:**
```
CKB v8.0.0

Active: my-project (/Users/lisa/Work/my-project)
Branch: feature/new-api (3 commits ahead)

Services:
  daemon    running (PID 12345, :9120, uptime 2h)
  mcp       2 sessions active
  watcher   watching (last check 30s ago)

Index:
  status    fresh (2 minutes ago)
  symbols   45,321 in 892 files
  tier      enhanced (SCIP)

Backends:
  scip      healthy
  lsp       1 server (gopls)
  git       ready

Cache:
  queries   1,234 cached (85% hit rate)
  size      2.3 MB
```

---

### 4. Process Discovery

**Problem:** No easy way to find all running CKB processes:
- Daemon might be running but forgotten
- MCP sessions in different terminals
- Watch processes

**Proposed solutions:**
- [ ] `ckb ps` - list all CKB processes (daemon, MCP servers, watchers)
- [ ] Daemon tracks active MCP sessions via registration
- [ ] PID file management for all long-running processes

---

### 5. Tool Discovery (Medium Priority)

**Problem:** Hard to know what MCP tools are available and what they do:
- 58 tools is a lot to remember
- No categorization in MCP response
- No "what can I do?" command

**Current state:**
- MCP `tools/list` returns all tools with descriptions
- No grouping or categorization
- No usage examples in tool descriptions

**Proposed solutions:**
- [ ] `ckb tools` - list available tools grouped by category
- [ ] `ckb tools <name>` - detailed help for a specific tool
- [ ] MCP: Add categories to tool metadata
- [ ] MCP: `getHelp` tool that explains available tools

---

## Implementation Order

### Phase 1: Quick Wins (v8.0.1) - COMPLETED
1. [x] Add active repo to `ckb status` first line
2. [x] Add `ckb use <name>` top-level shortcut
3. [x] Add daemon status to `ckb status` output

**Current `ckb status` output:**
```
CKB v8.0.0
──────────────────────────────────────────────────────────
Active: ckb (/Users/lisa/Work/Ideas/CodeMCP) (from current directory)
Daemon: running (PID 12345, port 9120, uptime 2h)

◉ Analysis Tier: Standard (SCIP index) [auto-detected]
  Available Tools: 22 of 24
...
```

### Phase 2: Enhanced Status (v8.0.2)
4. Unified status with all services
5. `ckb ps` command
6. MCP session tracking

### Phase 3: Discoverability (v8.0.3)
7. `ckb tools` command
8. Tool categories
9. Shell integration (`ckb prompt`)

---

## Technical Notes

### Active Repo Detection Priority
1. `CKB_REPO` environment variable (explicit override)
2. Current working directory matches a registered repo
3. Default repo from `~/.ckb/repos.json`
4. No active repo (error or prompt to select)

### MCP Session Tracking
- MCP server registers with daemon on startup (if daemon running)
- Heartbeat every 30s to maintain registration
- Session info: repo, PID, start time, tool call count

### Files Involved
- `cmd/ckb/status.go` - main status command
- `internal/query/status.go` - status query
- `internal/daemon/server.go` - daemon status
- `internal/mcp/server.go` - MCP session tracking
- `cmd/ckb/repo.go` - repo commands

---

## Open Questions

1. Should `ckb status` always check daemon even if not in daemon mode?
2. How to handle multiple MCP sessions with different active repos?
3. Should we add a TUI mode for status (`ckb status --live`)?

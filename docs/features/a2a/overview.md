# CKB A2A Protocol Support

CKB implements the [A2A (Agent-to-Agent) protocol v0.3](https://github.com/a2aproject/A2A), exposing all MCP tools as A2A skills over HTTP.

## Architecture

```
A2A Client (any A2A agent)
    │
    ├── GET  /.well-known/agent-card.json  → Agent discovery
    ├── POST /message:send                  → Skill invocation (HTTP+JSON)
    ├── POST /                              → Skill invocation (JSON-RPC 2.0)
    ├── POST /message:stream                → SSE streaming
    ├── GET  /tasks/{id}                    → Task status
    ├── GET  /health                        → Repo/index health + suggestions
    │
    ▼
internal/a2a/Server
    │
    ├── SkillRegistry  ← maps MCP tools → A2A skills (zero duplication)
    ├── TaskStore       ← SQLite (.ckb/a2a_tasks.db) task persistence
    ├── PushManager     ← webhook delivery with retries
    │
    └── MCPServer.CallTool()  ← executes the actual tool handler
            │
            ▼
        query.Engine → backends (SCIP, LSP, Git)
```

## Key Design Decisions

1. **Zero handler duplication** — A2A wraps MCP tools via `MCPServer.CallTool()`, not by reimplementing them.
2. **Dual protocol binding** — JSON-RPC and HTTP+JSON on the same port, sharing `do*` handler methods.
3. **Separate task store** — `.ckb/a2a_tasks.db` (SQLite), independent from the jobs DB.
4. **Atomic state transitions** — Conditional `UPDATE ... WHERE state IN (valid_sources)` prevents TOCTOU races.
5. **Index health surfaced** — `/health` and task metadata include index freshness so agents know when to reindex.

## Health & Index Hints

The `/health` endpoint returns:
- `index.initialized` — whether `ckb init` has been run
- `index.fresh` — whether the SCIP index is up to date
- `index.commitsBehind` — how stale the index is
- `suggestions[]` — actionable hints ("run ckb index", "use reindex skill")

Task responses include `metadata.indexWarnings` when results may be stale.

## Files

| File | Purpose |
|------|---------|
| `internal/a2a/types.go` | A2A v0.3 protocol types |
| `internal/a2a/server.go` | Server lifecycle, SSE fan-out |
| `internal/a2a/routes.go` | Route registration, health endpoint |
| `internal/a2a/handlers.go` | Shared do* handlers for both bindings |
| `internal/a2a/jsonrpc.go` | JSON-RPC 2.0 dispatch |
| `internal/a2a/streaming.go` | SSE streaming for send/subscribe |
| `internal/a2a/task_store.go` | SQLite task persistence |
| `internal/a2a/task_state.go` | State machine |
| `internal/a2a/skill_registry.go` | MCP tools → A2A skills bridge |
| `internal/a2a/agent_card.go` | Agent card generation |
| `internal/a2a/push.go` | Webhook delivery |
| `internal/a2a/middleware.go` | Auth, CORS, version, logging |
| `internal/a2a/errors.go` | A2A error codes |
| `internal/a2a/converter.go` | envelope.Response ↔ A2A types |
| `cmd/ckb/a2a.go` | CLI command |

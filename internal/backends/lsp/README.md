# LSP Backend Package

This package implements Phase 2.3: LSP Supervisor for the CKB (Codebase Knowledge Backend) system.

## Overview

The LSP backend provides Language Server Protocol integration for CKB, enabling real-time code intelligence through LSP servers. The supervisor manages multiple LSP processes, handles failures, implements rate limiting, and provides LRU-based eviction.

## Architecture

### Components

1. **Process Management** (`process.go`)
   - `LspProcess`: Represents a running LSP server process
   - State management: Starting → Initializing → Ready → Unhealthy/Dead
   - Tracks health metrics (last response time, failure count, restart count)
   - JSON-RPC message handling via stdin/stdout pipes

2. **JSON-RPC Protocol** (`jsonrpc.go`)
   - LSP message encoding/decoding (Content-Length headers + JSON payload)
   - Request/response correlation via message IDs
   - Notification handling (no response expected)
   - Server-initiated message handling (diagnostics, logging, etc.)

3. **Supervisor** (`supervisor.go`)
   - Central manager for all LSP processes
   - Process lifecycle: start, stop, restart
   - Configuration from `config.Backends.Lsp.Servers`
   - Health check loop (every 30s)
   - Statistics and monitoring

4. **Query Handling** (`query.go`)
   - High-level query API (definition, references, symbols, hover)
   - Request routing to appropriate language server
   - Auto-start servers on first request
   - Context-based cancellation support

5. **Queue & Rate Limiting** (`queue.go`)
   - Per-language request queues (default: 10 slots)
   - Reject-fast when queue >80% full
   - Max wait time: 200ms (configurable)
   - Request draining on server restart

6. **Health & Recovery** (`health.go`)
   - Crash detection and automatic restart
   - Exponential backoff: 1s → 2s → 4s → 8s → ... → 30s (max)
   - Failure threshold: 3 consecutive failures
   - Response timeout: 60s
   - Force restart capability

7. **LRU Eviction** (`eviction.go`)
   - Process limit enforcement (default: 4 concurrent)
   - Least Recently Used (LRU) eviction when at capacity
   - Idle process cleanup
   - Dynamic capacity adjustment

8. **Backend Adapter** (`adapter.go`)
   - Implements `backends.Backend` interface
   - Symbol search, references, definitions
   - LSP capability negotiation
   - Location and symbol result parsing

## Configuration

Default configuration in `internal/config/config.go`:

```go
LspSupervisor: LspSupervisorConfig{
    MaxTotalProcesses:    4,    // Max concurrent LSP processes
    QueueSizePerLanguage: 10,   // Queue size per language
    MaxQueueWaitMs:       200,  // Max time to wait for queue slot
}

Backends.Lsp: LspConfig{
    Enabled:           true,
    WorkspaceStrategy: "repo-root",
    Servers: map[string]LspServerCfg{
        "typescript": {
            Command: "typescript-language-server",
            Args:    []string{"--stdio"},
        },
        "dart": {
            Command: "dart",
            Args:    []string{"language-server"},
        },
        "go": {
            Command: "gopls",
            Args:    []string{},
        },
        "python": {
            Command: "pylsp",
            Args:    []string{},
        },
    },
}
```

## Usage

### Basic Usage

```go
// Create supervisor
cfg := config.DefaultConfig()
logger := logging.NewLogger(logging.Config{
    Format: logging.HumanFormat,
    Level:  logging.InfoLevel,
})
supervisor := NewLspSupervisor(cfg, logger)
defer supervisor.Shutdown()

// Start a language server
err := supervisor.StartServer("typescript")

// Query for definitions
ctx := context.Background()
result, err := supervisor.QueryDefinition(
    ctx,
    "typescript",
    "file:///path/to/file.ts",
    10,  // line (0-indexed in LSP)
    5,   // character (0-indexed)
)

// Query for references
refs, err := supervisor.QueryReferences(
    ctx,
    "typescript",
    "file:///path/to/file.ts",
    10, 5,
    true, // includeDeclaration
)

// Search symbols
symbols, err := supervisor.QueryWorkspaceSymbols(ctx, "typescript", "MyClass")
```

### Using the Backend Adapter

```go
// Create adapter for TypeScript
adapter := NewLspAdapter(supervisor, "typescript", logger)

// Check availability
if !adapter.IsAvailable() {
    log.Fatal("LSP backend not available")
}

// Get capabilities
caps := adapter.Capabilities()
// ["goto-definition", "find-references", "symbol-search", "hover"]

// Search symbols
searchResult, err := adapter.SearchSymbols(ctx, "MyClass", backends.SearchOptions{
    MaxResults:   10,
    IncludeTests: false,
})

// Find references
refResult, err := adapter.FindReferences(ctx, symbolID, backends.RefOptions{
    MaxResults:         100,
    IncludeDeclaration: true,
})
```

### Health Monitoring

```go
// Check health of all processes
status := supervisor.GetHealthStatus()
for langId, health := range status {
    fmt.Printf("%s: %v\n", langId, health)
}

// Force restart unhealthy process
err := supervisor.ForceRestart("typescript")

// Recover all unhealthy processes
errors := supervisor.RecoverAll()
for langId, err := range errors {
    log.Printf("Failed to recover %s: %v", langId, err)
}

// Get statistics
stats := supervisor.GetStats()
fmt.Printf("Running: %d/%d processes\n",
    stats["totalProcesses"],
    stats["maxProcesses"])
```

### Queue Management

```go
// Check queue sizes
queueStats := supervisor.GetQueueStats()

// Check if should reject fast
if supervisor.RejectFast("typescript") {
    log.Println("Queue pressure high, consider retrying later")
}

// Wait for queue to drain
success := supervisor.WaitForQueue("typescript", 2, 5*time.Second)
```

### Eviction Control

```go
// Evict idle processes (>1 hour idle)
evicted := supervisor.EvictIdle(1 * time.Hour)
fmt.Printf("Evicted %d idle processes\n", evicted)

// Get eviction candidates (LRU order)
candidates := supervisor.GetEvictionCandidates()

// Manually evict a process
err := supervisor.EvictProcess("python")

// Adjust capacity dynamically
err := supervisor.SetMaxProcesses(8)
```

## Process States

```
StateStarting ─────> StateInitializing ─────> StateReady
                                                   │
                                                   v
StateUnhealthy <────────────────────────────> StateDead
       │
       └──────> (restart with backoff) ──────> StateStarting
```

## Error Handling

The LSP backend uses CKB's structured error system:

- `BackendUnavailable`: LSP server not configured or not running
- `WorkspaceNotReady`: LSP still initializing
- `RateLimited`: Queue full, retry after delay
- `Timeout`: Request timed out or cancelled
- `SymbolNotFound`: Symbol doesn't exist

Each error includes suggested fixes via `errors.GetSuggestedFixes()`.

## Performance Characteristics

- **Startup time**: 1-3s per language server (includes initialization)
- **Query latency**: 100-500ms (depends on LSP server and codebase size)
- **Memory**: ~50-200MB per LSP process
- **CPU**: Low (idle), 10-30% per query
- **Concurrency**: Up to 4 processes, 10 queued requests per language

## Limitations

- LSP results are "best effort" (completeness: 0.7)
- Some LSP servers don't support all features
- Cross-file analysis depends on LSP server capabilities
- Process crashes reset all pending requests
- Platform-specific process management (signal 0 check on Unix)

## Testing

Run tests:

```bash
go test ./internal/backends/lsp/...
```

Run with verbose output:

```bash
go test -v ./internal/backends/lsp/...
```

Run specific test:

```bash
go test ./internal/backends/lsp/... -run TestLspSupervisorCreation
```

Benchmark:

```bash
go test -bench=. ./internal/backends/lsp/...
```

## Future Enhancements

- [ ] Windows process management support
- [ ] LSP workspace/didChangeConfiguration support
- [ ] Incremental text synchronization
- [ ] Multi-root workspace support
- [ ] LSP progress reporting integration
- [ ] Better handling of LSP server-specific quirks
- [ ] Metrics and telemetry
- [ ] Connection pooling for frequently used servers

# v8.0 Logging Improvements

Design document for improving CKB's logging system in v8.0.

## Current State

### Problems

1. **Custom logging package** - We have `internal/logging` instead of using Go's standard `log/slog` (available since Go 1.21)
2. **No global verbosity control** - No `--verbose` or `--debug` flags; must use `CKB_LOG_LEVEL=debug`
3. **Inconsistent log destinations** - Some commands log to stdout (mixing with output), others to stderr
4. **No log rotation** - Daemon logs grow unbounded
5. **Limited contextual info** - No request IDs, correlation IDs, or trace context
6. **No CLI-friendly quiet mode** - Can't suppress all logs for scripting

### Current Architecture

| Scope | Destination | Persisted | Format |
|-------|-------------|-----------|--------|
| CLI commands | stdout | No | human |
| Daemon | `~/.ckb/daemon/daemon.log` | Yes | json |
| MCP | stderr | No | json |
| HTTP serve | stdout | No | human |

## Proposed Improvements

### 1. Migrate to `log/slog`

Replace custom `internal/logging` with Go's standard library `log/slog`:

```go
// Before (custom)
logger := logging.NewLogger(logging.Config{
    Level:  logging.InfoLevel,
    Format: logging.HumanFormat,
    Output: os.Stdout,
})
logger.Info("message", map[string]interface{}{"key": "value"})

// After (slog)
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
logger.Info("message", "key", "value")
```

**Benefits:**
- Standard library, no custom code to maintain
- Built-in JSON and text handlers
- Better performance (zero-allocation for common cases)
- LogValuer interface for custom types
- Context integration (`slog.InfoContext`)

### 2. Global Verbosity Flags

Add global flags to root command:

```bash
ckb status              # Default: errors only (quiet)
ckb status -v           # Verbose: info level
ckb status -vv          # Very verbose: debug level
ckb status --quiet      # Quiet: suppress all logs
ckb status --debug      # Alias for -vv
```

Implementation:
```go
var (
    verbosity int
    quiet     bool
)

func init() {
    rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "Increase verbosity (-v=info, -vv=debug)")
    rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all log output")
}

func getLogLevel() slog.Level {
    if quiet {
        return slog.Level(100) // Above all levels
    }
    switch verbosity {
    case 0:
        return slog.LevelWarn
    case 1:
        return slog.LevelInfo
    default:
        return slog.LevelDebug
    }
}
```

### 3. Consistent Log Destinations

**Rule:** Logs go to stderr, output goes to stdout.

| Command Type | stdout | stderr |
|--------------|--------|--------|
| Query commands | JSON/human output | Logs |
| Status/info | Formatted output | Logs |
| MCP | JSON-RPC protocol | Logs |
| Daemon | (backgrounded) | Logs → file |

```go
// All commands should use stderr for logs
logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
```

### 4. Log Rotation for Daemon

Add log rotation to prevent unbounded growth:

```go
// Option A: Use lumberjack
import "gopkg.in/natefinch/lumberjack.v2"

logWriter := &lumberjack.Logger{
    Filename:   logPath,
    MaxSize:    10,    // MB
    MaxBackups: 3,
    MaxAge:     30,    // days
    Compress:   true,
}

// Option B: Simple rotation on startup
func rotateLogIfNeeded(path string, maxSize int64) error {
    info, err := os.Stat(path)
    if err != nil || info.Size() < maxSize {
        return nil
    }
    rotated := path + ".1"
    os.Remove(rotated)
    return os.Rename(path, rotated)
}
```

Configuration in `.ckb/config.json`:
```json
{
  "daemon": {
    "logFile": "~/.ckb/daemon/daemon.log",
    "logMaxSize": "10MB",
    "logMaxBackups": 3
  }
}
```

### 5. Contextual Logging

Add request/operation IDs for tracing:

```go
// Generate operation ID for CLI commands
opID := fmt.Sprintf("op_%s", uuid.New().String()[:8])
ctx := context.WithValue(ctx, "operation_id", opID)

// Use slog with context
logger.InfoContext(ctx, "Starting operation",
    "operation_id", opID,
    "command", "status",
    "repo", repoRoot,
)
```

For MCP, use JSON-RPC request ID:
```go
logger.Info("Handling tool call",
    "request_id", req.ID,
    "tool", req.Method,
    "repo", repoRoot,
)
```

### 6. Structured Error Logging

Consistent error logging with stack traces in debug mode:

```go
type LoggableError struct {
    Err   error
    Stack string
}

func (e LoggableError) LogValue() slog.Value {
    attrs := []slog.Attr{
        slog.String("message", e.Err.Error()),
    }
    if e.Stack != "" {
        attrs = append(attrs, slog.String("stack", e.Stack))
    }
    return slog.GroupValue(attrs...)
}
```

### 7. Environment Variable Consolidation

Simplify to fewer, well-documented env vars:

| Variable | Values | Description |
|----------|--------|-------------|
| `CKB_LOG_LEVEL` | debug, info, warn, error | Log level |
| `CKB_LOG_FORMAT` | text, json | Output format |
| `CKB_LOG_FILE` | path | Write logs to file |
| `CKB_DEBUG` | 1 | Shorthand for LOG_LEVEL=debug |

### 8. OpenTelemetry Integration (Future)

Prepare for observability integration:

```go
// Bridge slog to OpenTelemetry
import "go.opentelemetry.io/contrib/bridges/otelslog"

handler := otelslog.NewHandler("ckb", otelslog.WithLoggerProvider(provider))
logger := slog.New(handler)

// Logs automatically include trace context
logger.InfoContext(ctx, "Processing request") // Includes trace_id, span_id
```

## Implementation Plan

### Phase 1: Foundation
- [ ] Add `--verbose`, `--quiet`, `--debug` global flags
- [ ] Route all logs to stderr
- [x] Suppress logs in `status` command (done)
- [x] Add `ckb log` command (done)

### Phase 2: Migrate to slog
- [ ] Create `internal/slogutil` with helper functions
- [ ] Migrate `internal/logging` to wrap slog
- [ ] Update all command files to use new logger

### Phase 3: Enhanced Features
- [ ] Add log rotation for daemon
- [ ] Add operation IDs for CLI commands
- [ ] Add request ID logging for MCP
- [ ] Add `--log-file` flag for persisting CLI logs
- [ ] OpenTelemetry bridge for slog
- [ ] Correlation with existing telemetry
- [ ] Structured error logging with stack traces

## API Changes

### New Flags

```
Global Flags:
  -v, --verbose count   Increase verbosity (-v=info, -vv=debug)
  -q, --quiet           Suppress all log output
      --debug           Enable debug logging (same as -vv)
      --log-file path   Write logs to file
```

### New Commands

```bash
ckb log                 # View daemon logs (already implemented)
ckb log -f              # Follow daemon logs
ckb log --clear         # Clear daemon logs
```

### Config Changes

```json
{
  "logging": {
    "level": "warn",
    "format": "text",
    "file": "",
    "daemon": {
      "maxSize": "10MB",
      "maxBackups": 3,
      "maxAge": 30
    }
  }
}
```

## References

- [Structured Logging with slog - Go Blog](https://go.dev/blog/slog)
- [Logging in Go with Slog - Better Stack](https://betterstack.com/community/guides/logging/logging-in-go/)
- [gh CLI debug flag discussion](https://github.com/cli/cli/issues/5954)
- [Go Logging Best Practices - Uptrace](https://uptrace.dev/blog/golang-logging)

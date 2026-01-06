# Plan: v8.0 Logging Phase 2 & 3

Continuation of the logging improvements. Phase 1 (slog migration) is complete in PR#81.

## Design Principles

1. **Separation of concerns** - Different log files for different subsystems
2. **Per-repo logs** - Project-specific logs in `.ckb/logs/`
3. **Global logs** - System-wide logs in `~/.ckb/logs/`
4. **Configurable levels** - Global defaults + per-project overrides
5. **Active repo context** - `ckb log` shows logs for the currently active repo

## Log File Structure

```
~/.ckb/                          # Global CKB directory
├── logs/
│   ├── daemon.log               # Background daemon (system-wide)
│   └── system.log               # Global operations, errors
│
/path/to/repo/.ckb/              # Per-repo directory
├── logs/
│   ├── mcp.log                  # MCP server sessions for this repo
│   ├── api.log                  # HTTP API requests for this repo
│   ├── index.log                # Indexing operations
│   └── query.log                # Query engine operations (optional, debug)
```

## Log Levels

| Level | CLI Flag | Description |
|-------|----------|-------------|
| minimal | (default) | Errors and warnings only |
| standard | `-v` | + Info messages |
| verbose | `-vv` | + Detailed progress |
| debug | `-vvv` or `--debug` | + Debug traces |

Configuration precedence:
1. CLI flags (`-v`, `-q`, `--debug`)
2. Environment (`CKB_LOG_LEVEL`)
3. Per-project config (`.ckb/config.json`)
4. Global config (`~/.ckb/config.json`)

## Work Packages

### WP1: Log Directory Infrastructure
**Files:** `internal/paths/logs.go` (new)

```go
// Global log paths
func GetGlobalLogsDir() (string, error)      // ~/.ckb/logs/
func GetDaemonLogPath() (string, error)      // ~/.ckb/logs/daemon.log
func GetSystemLogPath() (string, error)      // ~/.ckb/logs/system.log

// Per-repo log paths (requires repo root)
func GetRepoLogsDir(repoRoot string) (string, error)   // .ckb/logs/
func GetMCPLogPath(repoRoot string) (string, error)    // .ckb/logs/mcp.log
func GetAPILogPath(repoRoot string) (string, error)    // .ckb/logs/api.log
func GetIndexLogPath(repoRoot string) (string, error)  // .ckb/logs/index.log

// Ensure directories exist
func EnsureGlobalLogsDir() (string, error)
func EnsureRepoLogsDir(repoRoot string) (string, error)
```

**Estimate:** ~50 lines

---

### WP2: Log Level Configuration
**Files:** `internal/config/config.go`, `internal/slogutil/level.go`

Add to config structure:
```go
type LoggingConfig struct {
    Level      string            `json:"level"`      // minimal, standard, verbose, debug
    MCP        string            `json:"mcp"`        // per-subsystem override
    API        string            `json:"api"`
    Index      string            `json:"index"`
    MaxSize    string            `json:"maxSize"`    // e.g., "10MB"
    MaxBackups int               `json:"maxBackups"` // number of rotated files
    Remote     *RemoteLogConfig  `json:"remote,omitempty"` // optional remote aggregator
}

type RemoteLogConfig struct {
    Type     string            `json:"type"`     // "loki", "otlp" (future)
    Endpoint string            `json:"endpoint"` // e.g., "http://localhost:3100"
    Labels   map[string]string `json:"labels"`   // static labels for all logs
    BatchSize int              `json:"batchSize"` // logs to batch before sending (default: 100)
    FlushInterval string       `json:"flushInterval"` // e.g., "5s" (default: 5s)
}

type Config struct {
    // ...existing fields...
    Logging LoggingConfig `json:"logging"`
}
```

Add level helpers:
```go
// slogutil/level.go
func ParseLevel(s string) slog.Level {
    switch strings.ToLower(s) {
    case "minimal":
        return slog.LevelWarn
    case "standard":
        return slog.LevelInfo
    case "verbose":
        return slog.Level(-2) // Between Debug and Info
    case "debug":
        return slog.LevelDebug
    default:
        return slog.LevelWarn
    }
}
```

**Estimate:** ~60 lines

---

### WP3: Subsystem Loggers
**Files:** `internal/slogutil/factory.go` (new)

Logger factory that creates appropriately configured loggers:

```go
type LoggerFactory struct {
    repoRoot   string
    config     *config.Config
    globalLevel slog.Level  // from CLI flags
}

func NewLoggerFactory(repoRoot string, cfg *config.Config, cliLevel slog.Level) *LoggerFactory

// Create loggers for each subsystem
func (f *LoggerFactory) MCPLogger() (*slog.Logger, io.Closer, error)
func (f *LoggerFactory) APILogger() (*slog.Logger, io.Closer, error)
func (f *LoggerFactory) IndexLogger() (*slog.Logger, io.Closer, error)
func (f *LoggerFactory) DaemonLogger() (*slog.Logger, io.Closer, error)  // global
func (f *LoggerFactory) SystemLogger() (*slog.Logger, io.Closer, error)  // global

// Helper to get effective level for subsystem
func (f *LoggerFactory) effectiveLevel(subsystem string) slog.Level
```

**Estimate:** ~100 lines

---

### WP4: MCP Logging Integration
**Files:** `cmd/ckb/mcp.go`

Update MCP command to use per-repo logging:

```go
func runMCP(cmd *cobra.Command, args []string) error {
    repoRoot := findRepoRoot()
    cfg := loadConfig(repoRoot)

    factory := slogutil.NewLoggerFactory(repoRoot, cfg, getGlobalLevel())
    logger, closer, err := factory.MCPLogger()
    if err != nil {
        return err
    }
    defer closer.Close()

    // Pass logger to MCP server
    server := mcp.NewMCPServer(cfg, engine, logger)
    // ...
}
```

**Estimate:** ~30 lines changed

---

### WP5: API Logging Integration
**Files:** `cmd/ckb/serve.go`, `internal/api/server.go`

Update HTTP API to use per-repo logging:

```go
func runServe(cmd *cobra.Command, args []string) error {
    factory := slogutil.NewLoggerFactory(repoRoot, cfg, getGlobalLevel())
    logger, closer, err := factory.APILogger()
    if err != nil {
        return err
    }
    defer closer.Close()

    server := api.NewServer(cfg, engine, logger)
    // ...
}
```

**Estimate:** ~30 lines changed

---

### WP6: Enhanced `ckb log` Command
**Files:** `cmd/ckb/log.go`

```go
var (
    logFollow  bool
    logLines   int
    logType    string  // mcp, api, index, daemon, system
    logClear   bool
    logPath    bool
    logGlobal  bool    // show global logs instead of repo logs
)

func init() {
    logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
    logCmd.Flags().IntVarP(&logLines, "lines", "n", 50, "Number of lines to show")
    logCmd.Flags().StringVarP(&logType, "type", "t", "mcp", "Log type: mcp, api, index, daemon, system")
    logCmd.Flags().BoolVar(&logClear, "clear", false, "Clear the log file")
    logCmd.Flags().BoolVar(&logPath, "path", false, "Print log file path")
    logCmd.Flags().BoolVar(&logGlobal, "global", false, "Show global logs (daemon, system)")
}

func runLog(cmd *cobra.Command, args []string) error {
    var logFile string

    if logGlobal || logType == "daemon" || logType == "system" {
        // Global logs
        switch logType {
        case "daemon":
            logFile, _ = paths.GetDaemonLogPath()
        case "system":
            logFile, _ = paths.GetSystemLogPath()
        default:
            logFile, _ = paths.GetDaemonLogPath()
        }
    } else {
        // Per-repo logs (uses active repo)
        repoRoot := getActiveRepoRoot()
        switch logType {
        case "mcp":
            logFile, _ = paths.GetMCPLogPath(repoRoot)
        case "api":
            logFile, _ = paths.GetAPILogPath(repoRoot)
        case "index":
            logFile, _ = paths.GetIndexLogPath(repoRoot)
        }
    }

    // ... rest of implementation
}
```

Usage:
```bash
ckb log                    # MCP logs for active repo
ckb log -t api             # API logs for active repo
ckb log -t index           # Index logs for active repo
ckb log -t daemon --global # Global daemon logs
ckb log -t system --global # Global system logs
ckb log --clear            # Clear MCP logs
ckb log --path             # Print log file path
```

**Estimate:** ~80 lines

---

### WP7: Log Rotation
**Files:** `internal/slogutil/rotation.go` (new)

```go
// RotateIfNeeded rotates log file if it exceeds maxSize.
// Keeps up to maxBackups rotated files: log.1, log.2, etc.
func RotateIfNeeded(path string, maxSize int64, maxBackups int) error

// Called automatically when opening log files
func OpenLogFile(path string, maxSize int64, maxBackups int) (*os.File, error)
```

**Estimate:** ~60 lines

---

### WP8: Loki Integration (Remote Log Aggregation)
**Files:** `internal/slogutil/loki.go` (new), `internal/slogutil/factory.go`

Grafana Loki integration for centralized log aggregation.

**Why Loki:**
- Free, open source (Grafana Labs)
- Simple HTTP push API
- Label-based querying (like Prometheus for logs)
- Pairs with Grafana dashboards
- Low resource footprint

```go
// internal/slogutil/loki.go

// LokiHandler implements slog.Handler and sends logs to Loki
type LokiHandler struct {
    endpoint    string
    labels      map[string]string
    batchSize   int
    flushInterval time.Duration

    buffer    []lokiEntry
    mu        sync.Mutex
    done      chan struct{}
    client    *http.Client
}

type lokiEntry struct {
    timestamp time.Time
    line      string
}

// NewLokiHandler creates a handler that pushes logs to Loki
func NewLokiHandler(cfg *RemoteLogConfig, baseLabels map[string]string) (*LokiHandler, error)

// Start begins the background flush goroutine
func (h *LokiHandler) Start()

// Stop flushes remaining logs and stops the handler
func (h *LokiHandler) Stop() error

// Enabled implements slog.Handler
func (h *LokiHandler) Enabled(ctx context.Context, level slog.Level) bool

// Handle implements slog.Handler - buffers log and flushes when batch is full
func (h *LokiHandler) Handle(ctx context.Context, r slog.Record) error

// flush sends buffered logs to Loki
func (h *LokiHandler) flush() error
```

**Loki Push Format:**
```json
POST /loki/api/v1/push
{
  "streams": [{
    "stream": {
      "app": "ckb",
      "repo": "myproject",
      "subsystem": "mcp",
      "host": "macbook.local"
    },
    "values": [
      ["1704067200000000000", "level=info msg=\"Tool called\" tool=searchSymbols duration=45ms"],
      ["1704067201000000000", "level=debug msg=\"Cache hit\" key=symbols:Engine"]
    ]
  }]
}
```

**Dynamic Labels (per log entry):**
- `level` - log level (info, warn, error, debug)
- `subsystem` - mcp, api, index, daemon
- `repo` - repository name (from active repo)

**Static Labels (from config):**
- `app` - always "ckb"
- `env` - user-defined (dev, prod, etc.)
- `host` - machine hostname

**Factory Integration:**
```go
// In factory.go, update logger creation to optionally add Loki handler
func (f *LoggerFactory) createLogger(subsystem string, logPath string) (*slog.Logger, io.Closer, error) {
    handlers := []slog.Handler{
        // File handler (always)
        NewCKBHandler(fileWriter, &slog.HandlerOptions{Level: level}),
    }

    // Add Loki handler if configured
    if f.config.Logging.Remote != nil && f.config.Logging.Remote.Type == "loki" {
        lokiHandler, err := NewLokiHandler(f.config.Logging.Remote, map[string]string{
            "app": "ckb",
            "repo": f.repoName,
            "subsystem": subsystem,
        })
        if err == nil {
            lokiHandler.Start()
            handlers = append(handlers, lokiHandler)
        }
    }

    return slog.New(NewTeeHandler(handlers...)), closer, nil
}
```

**Estimate:** ~150 lines

---

## Implementation Order

```
WP1 (paths) ──┬──> WP3 (factory) ──┬──> WP4 (MCP)
              │                    ├──> WP5 (API)
WP2 (config) ─┘                    └──> WP6 (ckb log)
                                           │
WP7 (rotation) ────────────────────────────┤
                                           │
WP8 (loki) ────────────────────────────────┘
```

**Phase 2a:** WP1 + WP2 (foundation)
**Phase 2b:** WP3 + WP4 + WP5 (integration)
**Phase 2c:** WP6 + WP7 (CLI + polish)
**Phase 2d:** WP8 (remote aggregation - optional)

## Config Example

Global config (`~/.ckb/config.json`):
```json
{
  "logging": {
    "level": "minimal",
    "maxSize": "10MB",
    "maxBackups": 3
  }
}
```

Per-repo config (`.ckb/config.json`):
```json
{
  "logging": {
    "level": "standard",
    "mcp": "verbose",
    "index": "debug"
  }
}
```

With Loki remote logging:
```json
{
  "logging": {
    "level": "standard",
    "maxSize": "10MB",
    "maxBackups": 3,
    "remote": {
      "type": "loki",
      "endpoint": "http://localhost:3100",
      "labels": {
        "env": "dev",
        "team": "platform"
      },
      "batchSize": 100,
      "flushInterval": "5s"
    }
  }
}
```

## Acceptance Criteria

- [ ] MCP creates logs at `.ckb/logs/mcp.log` in active repo
- [ ] API creates logs at `.ckb/logs/api.log` in active repo
- [ ] Daemon creates logs at `~/.ckb/logs/daemon.log` (global)
- [ ] `ckb log` shows MCP logs for active repo by default
- [ ] `ckb log -t api` shows API logs
- [ ] `ckb log -t daemon --global` shows daemon logs
- [ ] Log levels configurable in config.json (global + per-repo)
- [ ] `-v`/`-vv`/`-vvv` flags override config
- [ ] Log rotation works at configured size
- [ ] Loki integration sends logs when `remote.type: "loki"` is configured
- [ ] Build passes: `go build -o /dev/null ./...`
- [ ] Tests pass for new packages

## Out of Scope (Future)

- OpenTelemetry bridge
- Request/operation ID tracing
- Log streaming over WebSocket
- Log aggregation across repos
- Log search/filtering in CLI

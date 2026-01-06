# Plan: v8.0 Logging Phase 2 & 3

Continuation of the logging improvements. Phase 1 (slog migration) is complete in PR#81.

## Design Principles

1. **Separation of concerns** - Different log files for different subsystems
2. **Per-repo logs** - Project-specific logs in `.ckb/logs/`
3. **Global logs** - System-wide logs in `~/.ckb/logs/`
4. **Configurable levels** - Global defaults + per-project overrides
5. **Active repo context** - `ckb log` shows logs for the currently active repo
6. **Reuse existing code** - Extend `paths.go`, `config.go`, `slogutil.go` — no new files where existing patterns exist

## Existing Infrastructure (DO NOT DUPLICATE)

| Component | Location | Reuse |
|-----------|----------|-------|
| Path helpers | `internal/paths/paths.go` | Extend with log path functions |
| LoggingConfig | `internal/config/config.go:161-165` | Extend struct fields |
| Level parsing | `internal/slogutil/slogutil.go` | Extend `LevelFromString()` |
| File logger | `internal/slogutil/slogutil.go:18` | `NewFileLogger()` |
| Tee handler | `internal/slogutil/slogutil.go:70` | `TeeHandler` |
| Daemon log | `internal/paths/paths.go:457-465` | `GetDaemonLogPath()` — keep as-is |

## Log File Structure

```
~/.ckb/                          # Global CKB directory
├── daemon/
│   └── daemon.log               # Background daemon (EXISTING - keep here)
├── logs/
│   └── system.log               # Global operations, errors (NEW)
│
/path/to/repo/.ckb/              # Per-repo directory
├── logs/
│   ├── mcp.log                  # MCP server sessions for this repo
│   ├── api.log                  # HTTP API requests for this repo
│   └── index.log                # Indexing operations
```

**Note:** Daemon log stays at `~/.ckb/daemon/daemon.log` to avoid breaking existing users.

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
**Files:** `internal/paths/paths.go` (extend existing)

Add to existing `paths.go` (following established patterns):

```go
const (
    // LogsSubdir is the subdirectory for log files
    LogsSubdir = "logs"
)

// Global log paths (add to existing daemon section)
func GetGlobalLogsDir() (string, error)      // ~/.ckb/logs/
func GetSystemLogPath() (string, error)      // ~/.ckb/logs/system.log
func EnsureGlobalLogsDir() (string, error)

// Per-repo log paths (add new section)
func GetRepoLogsDir(repoRoot string) (string, error)   // .ckb/logs/
func GetMCPLogPath(repoRoot string) (string, error)    // .ckb/logs/mcp.log
func GetAPILogPath(repoRoot string) (string, error)    // .ckb/logs/api.log
func GetIndexLogPath(repoRoot string) (string, error)  // .ckb/logs/index.log
func EnsureRepoLogsDir(repoRoot string) (string, error)

// NOTE: GetDaemonLogPath() already exists at line 457 - DO NOT DUPLICATE
```

**Estimate:** ~40 lines added to existing file

---

### WP2: Log Level Configuration
**Files:** `internal/config/config.go`, `internal/slogutil/slogutil.go` (extend existing)

Extend existing `LoggingConfig` struct (line 161-165):
```go
// LoggingConfig contains logging configuration
// EXISTING: Format, Level
// NEW: Subsystem overrides, rotation, remote
type LoggingConfig struct {
    Format     string            `json:"format" mapstructure:"format"`     // EXISTING
    Level      string            `json:"level" mapstructure:"level"`       // EXISTING (extend values)
    MCP        string            `json:"mcp" mapstructure:"mcp"`           // NEW: per-subsystem override
    API        string            `json:"api" mapstructure:"api"`           // NEW
    Index      string            `json:"index" mapstructure:"index"`       // NEW
    MaxSize    string            `json:"maxSize" mapstructure:"maxSize"`   // NEW: e.g., "10MB"
    MaxBackups int               `json:"maxBackups" mapstructure:"maxBackups"` // NEW
    Remote     *RemoteLogConfig  `json:"remote,omitempty" mapstructure:"remote"` // NEW
}

// RemoteLogConfig for log aggregators (NEW struct)
type RemoteLogConfig struct {
    Type          string            `json:"type" mapstructure:"type"`
    Endpoint      string            `json:"endpoint" mapstructure:"endpoint"`
    Labels        map[string]string `json:"labels" mapstructure:"labels"`
    BatchSize     int               `json:"batchSize" mapstructure:"batchSize"`
    FlushInterval string            `json:"flushInterval" mapstructure:"flushInterval"`
}
```

Add env var mappings (line 510+):
```go
"CKB_LOGGING_MCP":         {path: "logging.mcp", varType: "string"},
"CKB_LOGGING_API":         {path: "logging.api", varType: "string"},
"CKB_LOGGING_INDEX":       {path: "logging.index", varType: "string"},
"CKB_LOGGING_MAX_SIZE":    {path: "logging.maxSize", varType: "string"},
"CKB_LOGGING_MAX_BACKUPS": {path: "logging.maxBackups", varType: "int"},
```

Extend `LevelFromString()` in `slogutil.go` (line 35-48):
```go
// LevelFromString converts a string to a slog.Level.
// Supports: debug, info, warn, error (case-insensitive)
// NEW: Also supports minimal, standard, verbose
func LevelFromString(s string) slog.Level {
    switch strings.ToLower(s) {
    case "debug":
        return slog.LevelDebug
    case "info", "standard":           // NEW alias
        return slog.LevelInfo
    case "warn", "warning", "minimal": // NEW alias
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    case "verbose":                    // NEW level
        return slog.Level(-2)          // Between Debug and Info
    default:
        return slog.LevelInfo
    }
}
```

**Estimate:** ~50 lines modified across 2 files

---

### WP3: Subsystem Loggers
**Files:** `internal/slogutil/factory.go` (new file, composes existing functions)

Logger factory that **composes existing functions** (`NewFileLogger`, `TeeHandler`, `LevelFromString`):

```go
type LoggerFactory struct {
    repoRoot    string
    config      *config.Config
    cliLevel    slog.Level  // from CLI flags (takes precedence)
    closers     []io.Closer // track open files for cleanup
}

func NewLoggerFactory(repoRoot string, cfg *config.Config, cliLevel slog.Level) *LoggerFactory

// Create loggers for each subsystem - REUSE existing functions
func (f *LoggerFactory) MCPLogger() (*slog.Logger, error) {
    logPath, _ := paths.GetMCPLogPath(f.repoRoot)
    _ = paths.EnsureRepoLogsDir(f.repoRoot)
    level := f.effectiveLevel("mcp")

    // REUSE: NewFileLogger from slogutil.go:18
    logger, file, err := NewFileLogger(logPath, level)
    if err != nil {
        return NewDiscardLogger(), nil  // REUSE: graceful fallback
    }
    f.closers = append(f.closers, file)
    return logger, nil
}

func (f *LoggerFactory) APILogger() (*slog.Logger, error)
func (f *LoggerFactory) IndexLogger() (*slog.Logger, error)
func (f *LoggerFactory) DaemonLogger() (*slog.Logger, error)  // uses paths.GetDaemonLogPath()
func (f *LoggerFactory) SystemLogger() (*slog.Logger, error)  // uses paths.GetSystemLogPath()

// Helper to get effective level for subsystem
// Precedence: CLI flag > subsystem config > global config > default
func (f *LoggerFactory) effectiveLevel(subsystem string) slog.Level {
    // CLI flag takes precedence
    if f.cliLevel != 0 {
        return f.cliLevel
    }
    // Check subsystem-specific config
    var subsystemLevel string
    switch subsystem {
    case "mcp":
        subsystemLevel = f.config.Logging.MCP
    case "api":
        subsystemLevel = f.config.Logging.API
    case "index":
        subsystemLevel = f.config.Logging.Index
    }
    if subsystemLevel != "" {
        return LevelFromString(subsystemLevel)  // REUSE
    }
    // Fall back to global config
    return LevelFromString(f.config.Logging.Level)  // REUSE
}

func (f *LoggerFactory) Close() error  // closes all tracked files
```

**Estimate:** ~80 lines (smaller due to reuse)

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
**Files:** `cmd/ckb/log.go` (extend existing)

Existing code (lines 14-17, 31-34) already has `logFollow` and `logLines` flags.
Add new flags and extend `runLog()`:

```go
var (
    logFollow  bool   // EXISTING
    logLines   int    // EXISTING
    logType    string // NEW: mcp, api, index, daemon, system
    logClear   bool   // NEW
    logPath    bool   // NEW
)

func init() {
    // EXISTING flags preserved
    logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
    logCmd.Flags().IntVarP(&logLines, "lines", "n", 50, "Number of lines to show")
    // NEW flags
    logCmd.Flags().StringVarP(&logType, "type", "t", "", "Log type: mcp, api, index, daemon, system")
    logCmd.Flags().BoolVar(&logClear, "clear", false, "Clear the log file")
    logCmd.Flags().BoolVar(&logPath, "path", false, "Print log file path instead of contents")
    rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
    var logFile string

    // Determine log file path
    switch logType {
    case "daemon", "":
        // Default: daemon log (backward compatible with existing behavior)
        logFile, _ = paths.GetDaemonLogPath()  // REUSE existing function
    case "system":
        logFile, _ = paths.GetSystemLogPath()  // NEW function from WP1
    case "mcp", "api", "index":
        // Per-repo logs - use active repo from global config
        repoRoot := getActiveRepoRoot()  // From v8.0 global config
        if repoRoot == "" {
            return fmt.Errorf("no active repo set, use 'ckb use <path>' first")
        }
        switch logType {
        case "mcp":
            logFile, _ = paths.GetMCPLogPath(repoRoot)
        case "api":
            logFile, _ = paths.GetAPILogPath(repoRoot)
        case "index":
            logFile, _ = paths.GetIndexLogPath(repoRoot)
        }
    default:
        return fmt.Errorf("unknown log type: %s", logType)
    }

    if logPath {
        fmt.Println(logFile)
        return nil
    }

    if logClear {
        return os.Truncate(logFile, 0)
    }

    // REUSE existing showLogLines() and followLogFile() functions
    if logFollow {
        return followLogFile(logFile)
    }
    return showLogLines(logFile, logLines)
}
```

Usage:
```bash
ckb log                    # Daemon logs (backward compatible)
ckb log -t mcp             # MCP logs for active repo
ckb log -t api             # API logs for active repo
ckb log -t index           # Index logs for active repo
ckb log -t daemon          # Global daemon logs (explicit)
ckb log -t system          # Global system logs
ckb log --clear -t mcp     # Clear MCP logs
ckb log --path -t api      # Print log file path
```

**Estimate:** ~50 lines modified (reuses existing showLogLines/followLogFile)

---

### WP7: Log Rotation
**Files:** `internal/slogutil/rotation.go` (new file)

```go
// RotatingFile wraps os.File with size-based rotation
type RotatingFile struct {
    path       string
    maxSize    int64
    maxBackups int
    file       *os.File
    size       int64
    mu         sync.Mutex
}

// OpenRotatingFile opens a file with rotation support
// Integrates with existing NewFileLogger by returning *os.File interface
func OpenRotatingFile(path string, maxSize int64, maxBackups int) (*RotatingFile, error)

// Write implements io.Writer - rotates if needed before writing
func (r *RotatingFile) Write(p []byte) (n int, err error)

// Close implements io.Closer
func (r *RotatingFile) Close() error

// rotate performs the rotation: log -> log.1 -> log.2 -> ...
func (r *RotatingFile) rotate() error
```

Update `NewFileLogger` in `slogutil.go` to optionally use rotation:

```go
// NewFileLoggerWithRotation creates a rotating file logger
// Uses config.Logging.MaxSize and MaxBackups
func NewFileLoggerWithRotation(path string, level slog.Level, maxSize string, maxBackups int) (*slog.Logger, io.Closer, error) {
    size := parseSize(maxSize)  // "10MB" -> 10485760
    rf, err := OpenRotatingFile(path, size, maxBackups)
    if err != nil {
        return nil, nil, err
    }
    return NewLogger(rf, level), rf, nil
}

func parseSize(s string) int64  // "10MB" -> 10485760, "1GB" -> 1073741824
```

**Estimate:** ~80 lines

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
WP1 (extend paths.go) ──┬──> WP3 (factory) ──┬──> WP4 (MCP)
                        │                    ├──> WP5 (API)
WP2 (extend config.go) ─┘                    └──> WP6 (extend log.go)
                                                      │
WP7 (rotation.go) ────────────────────────────────────┤
                                                      │
WP8 (loki.go) ────────────────────────────────────────┘
```

**Phase 2a:** WP1 + WP2 (extend existing files)
**Phase 2b:** WP3 + WP4 + WP5 (new factory + integration)
**Phase 2c:** WP6 + WP7 (extend CLI + rotation)
**Phase 2d:** WP8 (remote aggregation - optional)

## Code Reuse Summary

| New Code | Reuses |
|----------|--------|
| WP1: path functions | Pattern from `GetDaemonDir()`, `EnsureDaemonDir()` |
| WP2: config fields | Existing `LoggingConfig` struct, env var pattern |
| WP3: LoggerFactory | `NewFileLogger()`, `TeeHandler`, `LevelFromString()` |
| WP4-5: integration | Factory from WP3 |
| WP6: ckb log | `showLogLines()`, `followLogFile()`, `GetDaemonLogPath()` |
| WP7: rotation | Integrates with `NewLogger()` via `io.Writer` |
| WP8: loki | Uses `TeeHandler` for multi-destination |

**Files modified:** 4 existing (`paths.go`, `config.go`, `slogutil.go`, `log.go`)
**New files:** 3 (`factory.go`, `rotation.go`, `loki.go`)

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

### Paths (WP1)
- [ ] `paths.GetMCPLogPath(repoRoot)` returns `.ckb/logs/mcp.log`
- [ ] `paths.GetAPILogPath(repoRoot)` returns `.ckb/logs/api.log`
- [ ] `paths.GetSystemLogPath()` returns `~/.ckb/logs/system.log`
- [ ] `paths.GetDaemonLogPath()` unchanged (`~/.ckb/daemon/daemon.log`)

### Config (WP2)
- [ ] `LoggingConfig` has new fields: MCP, API, Index, MaxSize, MaxBackups, Remote
- [ ] `LevelFromString()` supports "minimal", "standard", "verbose"
- [ ] Env vars work: `CKB_LOGGING_MCP`, `CKB_LOGGING_API`, etc.

### Logging (WP3-5)
- [ ] MCP command writes to `.ckb/logs/mcp.log` in active repo
- [ ] API server writes to `.ckb/logs/api.log` in active repo
- [ ] CLI flags `-v`/`-vv`/`-vvv` override config levels

### CLI (WP6)
- [ ] `ckb log` shows daemon logs (backward compatible)
- [ ] `ckb log -t mcp` shows MCP logs for active repo
- [ ] `ckb log -t api` shows API logs for active repo
- [ ] `ckb log --path -t mcp` prints log file path
- [ ] `ckb log --clear -t mcp` truncates log file

### Rotation (WP7)
- [ ] Log rotation triggers at configured `maxSize`
- [ ] Keeps up to `maxBackups` rotated files

### Loki (WP8 - optional)
- [ ] Logs sent to Loki when `remote.type: "loki"` configured
- [ ] Batching works with configured `batchSize` and `flushInterval`

### Quality
- [ ] Build passes: `go build -o /dev/null ./...`
- [ ] Tests pass for modified and new packages
- [ ] No duplicate code - all reuse documented in Code Reuse Summary

## Out of Scope (Future)

- OpenTelemetry bridge
- Request/operation ID tracing
- Log streaming over WebSocket
- Log aggregation across repos
- Log search/filtering in CLI

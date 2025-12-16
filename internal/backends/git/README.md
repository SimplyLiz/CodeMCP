# Git Backend Adapter

Phase 2.4 implementation of the Git adapter for CKB (Codebase Knowledge Base).

## Overview

The Git backend adapter provides access to Git repository metadata including:
- Repository state (HEAD commit, diff hashes, dirty status)
- File history and commit information
- Churn metrics and hotspot analysis
- Diff statistics for staged/working tree changes
- Blame and author information

Per the design document, the Git backend is **always enabled** ("git is always").

## Architecture

The package consists of the following files:

### Core Files

- **`interface.go`** - Backend interface definitions
  - `Backend`: Base interface for all backends
  - `GitBackend`: Extended interface with Git-specific capabilities

- **`adapter.go`** - Main adapter implementation
  - `GitAdapter`: Implements the GitBackend interface
  - Always enabled (per design)
  - Query timeout from config (default 5000ms)
  - Integrates with logging and error packages

### Feature Modules

- **`history.go`** - History query capabilities
  - `GetFileHistory()`: Commit history for a file
  - `GetRecentCommits()`: Recent repository commits
  - `GetFileCommitCount()`: Number of commits for a file
  - `GetFileLastModified()`: Last modification timestamp
  - `GetFileAuthors()`: Unique authors for a file

- **`churn.go`** - Churn analysis
  - `GetFileChurn()`: Churn metrics for a file
  - `GetHotspots()`: Top files by churn score
  - `GetTotalChurnMetrics()`: Repository-wide churn stats
  - Hotspot scoring algorithm: `sqrt(changeCount) * log(authorCount+1) * log(avgChanges+1)`

- **`diff.go`** - Diff analysis
  - `GetStagedDiff()`: Statistics for staged changes
  - `GetWorkingTreeDiff()`: Statistics for working tree changes
  - `GetUntrackedFiles()`: List of untracked files
  - `GetDiffSummary()`: Combined summary of all changes
  - `GetCommitDiff()`: Diff for a specific commit

- **`repostate_integration.go`** - Repository state integration
  - `GetRepoState()`: Full RepoState using internal/repostate package
  - `GetRepoStateID()`: Composite ID for cache keys
  - `GetHeadCommit()`: Current HEAD commit
  - `IsDirty()`: Check for uncommitted changes
  - `GetCurrentBranch()`: Current branch name
  - `GetRemoteURL()`: Remote repository URL
  - `GetRepositoryInfo()`: Comprehensive repo information
  - `GetFileStatus()`: Git status for a specific file
  - `IsFileTracked()`: Check if file is tracked

### Testing

- **`adapter_test.go`** - Comprehensive test suite
  - Tests all major capabilities
  - Uses current repository for integration testing

## Data Types

### CommitInfo
```go
type CommitInfo struct {
    Hash      string // Full commit hash
    Author    string // Author name
    Timestamp string // ISO 8601 timestamp
    Message   string // First line of commit message
}
```

### FileHistory
```go
type FileHistory struct {
    FilePath     string
    CommitCount  int
    LastModified string      // ISO 8601 timestamp
    Commits      []CommitInfo // Most recent first
}
```

### ChurnMetrics
```go
type ChurnMetrics struct {
    FilePath       string
    ChangeCount    int     // Number of commits
    AuthorCount    int     // Unique authors
    LastModified   string  // ISO 8601 timestamp
    AverageChanges float64 // Lines changed per commit
    HotspotScore   float64 // Composite score
}
```

### DiffStats
```go
type DiffStats struct {
    FilePath  string
    Additions int
    Deletions int
    IsNew     bool
    IsDeleted bool
    IsRenamed bool
    OldPath   string // If renamed
}
```

## Configuration

The Git adapter reads configuration from `internal/config`:

```go
type GitConfig struct {
    Enabled bool // Ignored - Git is always enabled
}

// Query timeout from QueryPolicy.TimeoutMs["git"]
// Default: 5000ms
```

## Usage Example

```go
import (
    "github.com/ckb/ckb/internal/backends/git"
    "github.com/ckb/ckb/internal/config"
    "github.com/ckb/ckb/internal/logging"
)

// Create adapter
cfg := config.LoadConfig(repoRoot)
logger := logging.NewLogger(logging.Config{
    Format: logging.HumanFormat,
    Level:  logging.InfoLevel,
})

adapter, err := git.NewGitAdapter(cfg, logger)
if err != nil {
    log.Fatal(err)
}

// Get file history
history, err := adapter.GetFileHistory("internal/config/config.go", 10)
if err != nil {
    log.Fatal(err)
}

// Get churn hotspots
hotspots, err := adapter.GetHotspots(20, "1 month ago")
if err != nil {
    log.Fatal(err)
}

// Get repository state for cache keys
repoStateID, err := adapter.GetRepoStateID()
if err != nil {
    log.Fatal(err)
}
```

## Capabilities

The Git adapter reports the following capabilities:

- `repo-state` - Repository state tracking
- `file-history` - File commit history
- `churn-metrics` - Code churn analysis
- `blame-info` - Line attribution (future)
- `diff-stats` - Diff statistics
- `hotspots` - Hotspot identification

## Git Commands Used

The adapter uses the following git commands:

- `git rev-parse HEAD` - Get HEAD commit
- `git log --format=...` - History queries
- `git shortlog -sn` - Author counts
- `git diff --stat` - Change metrics
- `git diff --numstat` - Detailed diff stats
- `git diff --cached` - Staged changes
- `git ls-files --others --exclude-standard` - Untracked files
- `git status --porcelain` - File status
- `git remote get-url origin` - Remote URL
- `git rev-parse --abbrev-ref HEAD` - Current branch

All commands respect the configured query timeout (default 5000ms).

## Error Handling

The adapter uses `internal/errors.CkbError` for structured error reporting:

- `BackendUnavailable` - Git not available in directory
- `Timeout` - Command exceeded timeout
- `SymbolNotFound` - No history found for file
- `InternalError` - Unexpected errors

Errors include suggested fixes where applicable.

## Integration with Existing Packages

- **`internal/repostate`** - Uses existing RepoState computation
- **`internal/config`** - Reads configuration
- **`internal/logging`** - Structured logging
- **`internal/errors`** - Error handling

## Testing

Run the test suite:

```bash
cd /Users/lisa/Work/Ideas/CodeMCP
go test -v ./internal/backends/git/...
```

Tests verify:
- Adapter initialization
- Capability reporting
- History queries
- Churn analysis
- Diff statistics
- Repository state integration

## DoD Checklist

- [x] Git adapter can query history
- [x] Git adapter can analyze churn
- [x] Git adapter integrates with RepoState
- [x] Always enabled (per design doc)
- [x] Query timeout from config
- [x] Uses existing internal packages
- [x] Comprehensive error handling
- [x] Structured logging
- [x] Test coverage

## Future Enhancements

Potential additions not in scope for Phase 2.4:

- Git blame integration (`git blame`)
- Stash information
- Tag and release queries
- Submodule support
- Worktree support
- Performance optimization with git cat-file --batch
- Caching layer for expensive queries

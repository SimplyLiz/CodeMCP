package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ckb/internal/incremental"
	"ckb/internal/index"
	"ckb/internal/project"
	"ckb/internal/repostate"
	"ckb/internal/storage"
	"ckb/internal/webhooks"
)

// RefreshManager handles incremental and full reindex operations
type RefreshManager struct {
	logger         *slog.Logger
	stdLogger      interface{ Printf(string, ...interface{}) }
	webhookManager *webhooks.Manager

	// Track pending refreshes to prevent duplicates
	pending   map[string]bool
	pendingMu sync.RWMutex
}

// NewRefreshManager creates a new refresh manager
func NewRefreshManager(logger *slog.Logger, stdLogger interface{ Printf(string, ...interface{}) }, webhookMgr *webhooks.Manager) *RefreshManager {
	return &RefreshManager{
		logger:         logger,
		stdLogger:      stdLogger,
		webhookManager: webhookMgr,
		pending:        make(map[string]bool),
	}
}

// HasPendingRefresh checks if a refresh is already pending for the given repo
func (rm *RefreshManager) HasPendingRefresh(repoPath string) bool {
	rm.pendingMu.RLock()
	defer rm.pendingMu.RUnlock()
	return rm.pending[repoPath]
}

// markPending marks a refresh as pending
func (rm *RefreshManager) markPending(repoPath string) {
	rm.pendingMu.Lock()
	defer rm.pendingMu.Unlock()
	rm.pending[repoPath] = true
}

// clearPending clears the pending state
func (rm *RefreshManager) clearPending(repoPath string) {
	rm.pendingMu.Lock()
	defer rm.pendingMu.Unlock()
	delete(rm.pending, repoPath)
}

// RefreshResult contains the result of a refresh operation
type RefreshResult struct {
	RepoPath     string        `json:"repoPath"`
	Type         string        `json:"type"` // "incremental" or "full"
	Success      bool          `json:"success"`
	Duration     time.Duration `json:"duration"`
	FilesChanged int           `json:"filesChanged,omitempty"`
	Error        string        `json:"error,omitempty"`
	Trigger      string        `json:"trigger,omitempty"`
	TriggerInfo  string        `json:"triggerInfo,omitempty"`
}

// RunIncrementalRefresh performs an incremental refresh on a repository
func (rm *RefreshManager) RunIncrementalRefresh(ctx context.Context, repoPath string) *RefreshResult {
	return rm.RunIncrementalRefreshWithTrigger(ctx, repoPath, index.TriggerStale, "")
}

// RunIncrementalRefreshWithTrigger performs an incremental refresh with trigger info
func (rm *RefreshManager) RunIncrementalRefreshWithTrigger(ctx context.Context, repoPath string, trigger index.RefreshTrigger, triggerInfo string) *RefreshResult {
	rm.markPending(repoPath)
	defer rm.clearPending(repoPath)

	start := time.Now()
	result := &RefreshResult{
		RepoPath:    repoPath,
		Type:        "incremental",
		Trigger:     string(trigger),
		TriggerInfo: triggerInfo,
	}

	rm.stdLogger.Printf("Starting incremental refresh for %s (trigger: %s)", repoPath, trigger)

	// Open database
	db, err := storage.Open(repoPath, rm.logger)
	if err != nil {
		rm.stdLogger.Printf("Failed to open database for %s: %v", repoPath, err)
		result.Error = fmt.Sprintf("database error: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer func() { _ = db.Close() }()

	// Create incremental indexer with default config
	indexer := incremental.NewIncrementalIndexer(repoPath, db, nil, rm.logger)

	// Check if full reindex is needed
	needsFull, reason := indexer.NeedsFullReindex()
	if needsFull {
		rm.stdLogger.Printf("Incremental not possible for %s: %s, falling back to full", repoPath, reason)
		return rm.RunFullReindexWithTrigger(ctx, repoPath, trigger, triggerInfo)
	}

	// Run incremental update
	stats, err := indexer.IndexIncremental(ctx, "")
	if err != nil {
		rm.stdLogger.Printf("Incremental refresh failed for %s: %v, falling back to full", repoPath, err)
		return rm.RunFullReindexWithTrigger(ctx, repoPath, trigger, triggerInfo)
	}

	result.Success = true
	result.Duration = time.Since(start)
	result.FilesChanged = stats.FilesAdded + stats.FilesChanged + stats.FilesDeleted

	rm.stdLogger.Printf("Incremental refresh completed for %s: %d files changed in %v",
		repoPath, result.FilesChanged, result.Duration.Round(time.Millisecond))

	// Emit webhook event
	rm.emitWebhookEvent("index.updated", repoPath, map[string]interface{}{
		"type":         "incremental",
		"filesChanged": result.FilesChanged,
		"duration":     result.Duration.String(),
	})

	return result
}

// emitWebhookEvent sends a webhook event with the given data
func (rm *RefreshManager) emitWebhookEvent(eventType, source string, data map[string]interface{}) {
	if rm.webhookManager == nil {
		return
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		rm.stdLogger.Printf("Warning: failed to marshal webhook data: %v", err)
		return
	}

	_ = rm.webhookManager.Emit(&webhooks.Event{
		Type:      webhooks.EventType(eventType),
		Source:    source,
		Timestamp: time.Now(),
		Data:      dataBytes,
	})
}

// RunFullReindex performs a full reindex on a repository
func (rm *RefreshManager) RunFullReindex(ctx context.Context, repoPath string) *RefreshResult {
	return rm.RunFullReindexWithTrigger(ctx, repoPath, index.TriggerStale, "")
}

// RunFullReindexWithTrigger performs a full reindex with trigger info
func (rm *RefreshManager) RunFullReindexWithTrigger(ctx context.Context, repoPath string, trigger index.RefreshTrigger, triggerInfo string) *RefreshResult {
	start := time.Now()
	result := &RefreshResult{
		RepoPath:    repoPath,
		Type:        "full",
		Trigger:     string(trigger),
		TriggerInfo: triggerInfo,
	}

	rm.stdLogger.Printf("Starting full reindex for %s (trigger: %s)", repoPath, trigger)

	// Detect language
	lang, _, _ := project.DetectAllLanguages(repoPath)
	if lang == project.LangUnknown {
		result.Error = "could not detect project language"
		result.Duration = time.Since(start)
		rm.stdLogger.Printf("Full reindex failed for %s: %s", repoPath, result.Error)
		return result
	}

	// Get indexer info
	indexer := project.GetIndexerInfo(lang)
	if indexer == nil {
		result.Error = fmt.Sprintf("no indexer available for %s", project.LanguageDisplayName(lang))
		result.Duration = time.Since(start)
		rm.stdLogger.Printf("Full reindex failed for %s: %s", repoPath, result.Error)
		return result
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		result.Error = "cancelled"
		result.Duration = time.Since(start)
		return result
	default:
	}

	// Acquire lock
	ckbDir := filepath.Join(repoPath, ".ckb")
	lock, err := index.AcquireLock(ckbDir)
	if err != nil {
		result.Error = fmt.Sprintf("could not acquire lock: %v", err)
		result.Duration = time.Since(start)
		rm.stdLogger.Printf("Full reindex skipped for %s: %s", repoPath, result.Error)
		return result
	}
	defer lock.Release()

	// Run indexer
	command := indexer.Command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		result.Error = "empty indexer command"
		result.Duration = time.Since(start)
		return result
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		result.Error = fmt.Sprintf("indexer failed: %v (%s)", err, strings.TrimSpace(stderr.String()))
		result.Duration = time.Since(start)
		rm.stdLogger.Printf("Full reindex failed for %s: %s", repoPath, result.Error)
		return result
	}

	duration := time.Since(start)

	// Update metadata with refresh trigger info
	meta := &index.IndexMeta{
		CreatedAt:   time.Now(),
		Duration:    duration.Round(time.Millisecond * 100).String(),
		Indexer:     indexer.CheckCommand,
		IndexerArgs: parts,
		LastRefresh: &index.LastRefresh{
			At:          time.Now(),
			Trigger:     trigger,
			TriggerInfo: triggerInfo,
			DurationMs:  duration.Milliseconds(),
		},
	}

	if rs, rsErr := repostate.ComputeRepoState(repoPath); rsErr == nil {
		meta.CommitHash = rs.HeadCommit
		meta.RepoStateID = rs.RepoStateID
	}

	if saveErr := meta.Save(ckbDir); saveErr != nil {
		rm.stdLogger.Printf("Warning: could not save index metadata for %s: %v", repoPath, saveErr)
	}

	result.Success = true
	result.Duration = duration

	rm.stdLogger.Printf("Full reindex completed for %s in %v (trigger: %s)", repoPath, duration.Round(time.Millisecond), trigger)

	// Emit webhook event
	rm.emitWebhookEvent("index.updated", repoPath, map[string]interface{}{
		"type":     "full",
		"duration": result.Duration.String(),
	})

	return result
}

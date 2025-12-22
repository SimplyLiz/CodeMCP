package incremental

import (
	"context"
	"fmt"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

// CurrentSchemaVersion should match storage.currentSchemaVersion
const CurrentSchemaVersion = 6

// IncrementalIndexer orchestrates incremental index updates
type IncrementalIndexer struct {
	repoRoot  string
	db        *storage.DB
	detector  *ChangeDetector
	extractor *SCIPExtractor
	updater   *IndexUpdater
	store     *Store
	config    *Config
	logger    *logging.Logger
}

// NewIncrementalIndexer creates a new incremental indexer
func NewIncrementalIndexer(repoRoot string, db *storage.DB, config *Config, logger *logging.Logger) *IncrementalIndexer {
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure IndexPath has a sensible default
	indexPath := config.IndexPath
	if indexPath == "" {
		indexPath = ".scip/index.scip"
	}

	store := NewStore(db, logger)
	return &IncrementalIndexer{
		repoRoot:  repoRoot,
		db:        db,
		detector:  NewChangeDetector(repoRoot, store, config, logger),
		extractor: NewSCIPExtractor(repoRoot, indexPath, logger),
		updater:   NewIndexUpdater(db, store, logger),
		store:     store,
		config:    config,
		logger:    logger,
	}
}

// IndexIncremental performs an incremental index update
// Returns stats on success, error if incremental not possible
func (i *IncrementalIndexer) IndexIncremental(ctx context.Context, since string) (*DeltaStats, error) {
	start := time.Now()

	// 1. Detect changes
	changes, err := i.detector.DetectChanges(since)
	if err != nil {
		return nil, fmt.Errorf("change detection failed: %w", err)
	}

	if len(changes) == 0 {
		stats := &DeltaStats{
			Duration:   time.Since(start),
			IndexState: "unchanged",
		}
		return stats, nil
	}

	i.logger.Info("Detected file changes", map[string]interface{}{
		"changeCount": len(changes),
	})

	// 2. Check if we should do full reindex instead
	totalFiles := i.store.GetTotalFileCount()
	if totalFiles > 0 && i.config.IncrementalThreshold > 0 {
		changePercent := (len(changes) * 100) / totalFiles
		if changePercent > i.config.IncrementalThreshold {
			return nil, fmt.Errorf("too many changes (%d%% of files), use --force for full reindex", changePercent)
		}
	}

	// 3. Run scip-go to regenerate index
	i.logger.Info("Running scip-go indexer", nil)
	if runErr := i.extractor.RunSCIPGo(); runErr != nil {
		return nil, fmt.Errorf("scip-go indexer failed: %w", runErr)
	}

	// 4. Extract deltas from SCIP
	delta, err := i.extractor.ExtractDeltas(changes)
	if err != nil {
		return nil, fmt.Errorf("failed to extract deltas: %w", err)
	}

	// 5. Apply changes to database
	if err := i.updater.ApplyDelta(delta); err != nil {
		return nil, fmt.Errorf("failed to apply delta: %w", err)
	}

	// 6. Update index state
	commit := i.detector.GetCurrentCommit()
	if err := i.updater.UpdateIndexState(len(delta.FileDeltas), commit); err != nil {
		return nil, fmt.Errorf("failed to update index state: %w", err)
	}

	delta.Stats.Duration = time.Since(start)
	delta.Stats.IndexState = "partial"

	i.logger.Info("Incremental index complete", map[string]interface{}{
		"filesChanged":   delta.Stats.FilesChanged,
		"filesAdded":     delta.Stats.FilesAdded,
		"filesDeleted":   delta.Stats.FilesDeleted,
		"symbolsAdded":   delta.Stats.SymbolsAdded,
		"symbolsRemoved": delta.Stats.SymbolsRemoved,
		"duration":       delta.Stats.Duration.String(),
	})

	return &delta.Stats, nil
}

// NeedsFullReindex checks if we need a full reindex
func (i *IncrementalIndexer) NeedsFullReindex() (bool, string) {
	// No previous index
	if !i.store.HasIndex() {
		return true, "no previous index"
	}

	// Schema version mismatch
	schemaVersion := i.store.GetSchemaVersion()
	if schemaVersion != 0 && schemaVersion != CurrentSchemaVersion {
		return true, fmt.Sprintf("schema version mismatch (have %d, need %d)",
			schemaVersion, CurrentSchemaVersion)
	}

	// No tracked commit (can't compute diff)
	if i.store.GetLastIndexedCommit() == "" && i.detector.isGitRepo() {
		return true, "no tracked commit"
	}

	return false, ""
}

// GetIndexState returns current index state for UI display
func (i *IncrementalIndexer) GetIndexState() IndexState {
	state := i.store.GetIndexState()

	// Check if dirty (uncommitted changes)
	if i.detector.isGitRepo() {
		state.IsDirty = i.detector.HasDirtyWorkingTree()
	}

	// Compute effective state
	// partial_dirty: index is partial AND there are uncommitted changes
	// This helps UX: "run ckb index to incorporate working tree changes"
	baseState := state.State
	if state.IsDirty && baseState == "partial" {
		state.State = "partial_dirty"
	} else if state.IsDirty && baseState == "full" {
		state.State = "full_dirty"
	}

	return state
}

// PopulateAfterFullIndex populates tracking tables after a full reindex
// This enables subsequent incremental updates
func (i *IncrementalIndexer) PopulateAfterFullIndex() error {
	// Populate file tracking from SCIP index
	if err := i.updater.PopulateFromFullIndex(i.extractor); err != nil {
		return fmt.Errorf("failed to populate from full index: %w", err)
	}

	// Mark as full index
	commit := i.detector.GetCurrentCommit()
	if err := i.updater.SetFullIndexComplete(commit); err != nil {
		return fmt.Errorf("failed to set full index complete: %w", err)
	}

	return nil
}

// GetStore returns the underlying store for external access
func (i *IncrementalIndexer) GetStore() *Store {
	return i.store
}

// GetDetector returns the change detector for external access
func (i *IncrementalIndexer) GetDetector() *ChangeDetector {
	return i.detector
}

// FormatStats formats the delta stats for display
func FormatStats(stats *DeltaStats, state IndexState) string {
	if stats.IndexState == "unchanged" {
		return "Index is up to date. Nothing to do."
	}

	commitInfo := state.Commit
	if len(commitInfo) > 7 {
		commitInfo = commitInfo[:7]
	}
	if state.IsDirty {
		commitInfo += " (+dirty)"
	}

	result := fmt.Sprintf(`
Incremental Index Complete
--------------------------
Files:   %d modified, %d added, %d deleted
Symbols: %d added, %d removed
Refs:    %d updated
Time:    %v
Commit:  %s

Accuracy:
  OK  Go to definition     - accurate
  OK  Find refs (forward)  - accurate
  !!  Find refs (reverse)  - may be stale
  !!  Call graph           - may be stale

Run 'ckb index --force' for full accuracy (%d files since last full)
`,
		stats.FilesChanged, stats.FilesAdded, stats.FilesDeleted,
		stats.SymbolsAdded, stats.SymbolsRemoved,
		stats.RefsAdded,
		stats.Duration.Round(time.Millisecond),
		commitInfo,
		state.FilesSinceFull,
	)

	return result
}

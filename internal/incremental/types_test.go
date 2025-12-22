package incremental

import (
	"testing"
)

func TestDefaultConfig_TransitiveSettings(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Test basic config values
	if config.IndexPath != ".scip/index.scip" {
		t.Errorf("expected IndexPath '.scip/index.scip', got %q", config.IndexPath)
	}
	if config.IncrementalThreshold != 50 {
		t.Errorf("expected IncrementalThreshold 50, got %d", config.IncrementalThreshold)
	}
	if config.IndexTests != false {
		t.Error("expected IndexTests false")
	}

	// Test transitive config defaults
	tc := config.Transitive
	if !tc.Enabled {
		t.Error("expected Transitive.Enabled true")
	}
	if tc.Mode != InvalidationLazy {
		t.Errorf("expected Transitive.Mode 'lazy', got %q", tc.Mode)
	}
	if tc.Depth != 1 {
		t.Errorf("expected Transitive.Depth 1, got %d", tc.Depth)
	}
	if tc.MaxRescanFiles != 200 {
		t.Errorf("expected Transitive.MaxRescanFiles 200, got %d", tc.MaxRescanFiles)
	}
	if tc.MaxRescanMs != 1500 {
		t.Errorf("expected Transitive.MaxRescanMs 1500, got %d", tc.MaxRescanMs)
	}
}

func TestChangeType_Constants(t *testing.T) {
	// Verify change type constants have expected values
	tests := []struct {
		ct   ChangeType
		want string
	}{
		{ChangeAdded, "added"},
		{ChangeModified, "modified"},
		{ChangeDeleted, "deleted"},
		{ChangeRenamed, "renamed"},
	}

	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("ChangeType %v: expected %q, got %q", tt.ct, tt.want, string(tt.ct))
		}
	}
}

func TestInvalidationMode_Constants(t *testing.T) {
	// Verify invalidation mode constants have expected values
	tests := []struct {
		mode InvalidationMode
		want string
	}{
		{InvalidationNone, "none"},
		{InvalidationLazy, "lazy"},
		{InvalidationEager, "eager"},
		{InvalidationDeferred, "deferred"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("InvalidationMode %v: expected %q, got %q", tt.mode, tt.want, string(tt.mode))
		}
	}
}

func TestRescanReason_Constants(t *testing.T) {
	// Verify rescan reason constants have expected values
	tests := []struct {
		reason RescanReason
		want   string
	}{
		{RescanDepChange, "dep_change"},
		{RescanBudgetExceeded, "budget_exceeded"},
		{RescanManual, "manual"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.want {
			t.Errorf("RescanReason %v: expected %q, got %q", tt.reason, tt.want, string(tt.reason))
		}
	}
}

func TestMetaKey_Constants(t *testing.T) {
	// Verify meta key constants are defined
	keys := []string{
		MetaKeyIndexState,
		MetaKeyLastFull,
		MetaKeyLastIncremental,
		MetaKeyIndexCommit,
		MetaKeyFilesSinceFull,
		MetaKeySchemaVersion,
		MetaKeyCallgraphQuality,
		MetaKeyLastDepsUpdate,
		MetaKeyInvalidationMode,
	}

	for _, key := range keys {
		if key == "" {
			t.Error("Meta key constant is empty")
		}
	}

	// Verify they have distinct values
	seen := make(map[string]bool)
	for _, key := range keys {
		if seen[key] {
			t.Errorf("Duplicate meta key: %q", key)
		}
		seen[key] = true
	}
}

func TestCurrentSchemaVersion(t *testing.T) {
	// Schema version should be 9 for v7.3 incremental indexing
	if CurrentSchemaVersion != 9 {
		t.Errorf("expected CurrentSchemaVersion 9, got %d", CurrentSchemaVersion)
	}
}

func TestChangedFile_Struct(t *testing.T) {
	cf := ChangedFile{
		Path:       "new/path.go",
		OldPath:    "old/path.go",
		ChangeType: ChangeRenamed,
		Hash:       "abc123",
	}

	if cf.Path != "new/path.go" {
		t.Error("Path not set correctly")
	}
	if cf.OldPath != "old/path.go" {
		t.Error("OldPath not set correctly")
	}
	if cf.ChangeType != ChangeRenamed {
		t.Error("ChangeType not set correctly")
	}
	if cf.Hash != "abc123" {
		t.Error("Hash not set correctly")
	}
}

func TestFileDelta_Struct(t *testing.T) {
	fd := FileDelta{
		Path:             "file.go",
		OldPath:          "old_file.go",
		ChangeType:       ChangeRenamed,
		Symbols:          []Symbol{{ID: "sym1"}},
		Refs:             []Reference{{FromFile: "file.go"}},
		CallEdges:        []CallEdge{{CallerFile: "file.go"}},
		Hash:             "hash123",
		SCIPDocumentHash: "scip456",
		SymbolCount:      5,
	}

	if fd.Path != "file.go" {
		t.Error("Path not set correctly")
	}
	if len(fd.Symbols) != 1 {
		t.Error("Symbols not set correctly")
	}
	if len(fd.Refs) != 1 {
		t.Error("Refs not set correctly")
	}
	if len(fd.CallEdges) != 1 {
		t.Error("CallEdges not set correctly")
	}
}

func TestCallEdge_Struct(t *testing.T) {
	edge := CallEdge{
		CallerID:   "caller_sym",
		CallerFile: "caller.go",
		CalleeID:   "callee_sym",
		Line:       10,
		Column:     5,
		EndColumn:  15,
	}

	if edge.CallerID != "caller_sym" {
		t.Error("CallerID not set correctly")
	}
	if edge.CallerFile != "caller.go" {
		t.Error("CallerFile not set correctly")
	}
	if edge.CalleeID != "callee_sym" {
		t.Error("CalleeID not set correctly")
	}
	if edge.Line != 10 {
		t.Error("Line not set correctly")
	}
	if edge.Column != 5 {
		t.Error("Column not set correctly")
	}
	if edge.EndColumn != 15 {
		t.Error("EndColumn not set correctly")
	}
}

func TestFileDependency_Struct(t *testing.T) {
	dep := FileDependency{
		DependentFile: "consumer.go",
		DefiningFile:  "provider.go",
	}

	if dep.DependentFile != "consumer.go" {
		t.Error("DependentFile not set correctly")
	}
	if dep.DefiningFile != "provider.go" {
		t.Error("DefiningFile not set correctly")
	}
}

func TestRescanQueueEntry_Struct(t *testing.T) {
	entry := RescanQueueEntry{
		FilePath: "file.go",
		Reason:   RescanDepChange,
		Depth:    2,
		Attempts: 1,
	}

	if entry.FilePath != "file.go" {
		t.Error("FilePath not set correctly")
	}
	if entry.Reason != RescanDepChange {
		t.Error("Reason not set correctly")
	}
	if entry.Depth != 2 {
		t.Error("Depth not set correctly")
	}
	if entry.Attempts != 1 {
		t.Error("Attempts not set correctly")
	}
}

func TestIndexState_Struct(t *testing.T) {
	state := IndexState{
		State:           "partial",
		LastFull:        1000,
		LastIncremental: 2000,
		FilesSinceFull:  10,
		Commit:          "abc123",
		IsDirty:         true,
		PendingRescans:  5,
	}

	if state.State != "partial" {
		t.Error("State not set correctly")
	}
	if state.LastFull != 1000 {
		t.Error("LastFull not set correctly")
	}
	if state.LastIncremental != 2000 {
		t.Error("LastIncremental not set correctly")
	}
	if state.FilesSinceFull != 10 {
		t.Error("FilesSinceFull not set correctly")
	}
	if state.Commit != "abc123" {
		t.Error("Commit not set correctly")
	}
	if !state.IsDirty {
		t.Error("IsDirty not set correctly")
	}
	if state.PendingRescans != 5 {
		t.Error("PendingRescans not set correctly")
	}
}

func TestTransitiveConfig_Struct(t *testing.T) {
	tc := TransitiveConfig{
		Enabled:        true,
		Mode:           InvalidationEager,
		Depth:          3,
		MaxRescanFiles: 100,
		MaxRescanMs:    500,
	}

	if !tc.Enabled {
		t.Error("Enabled not set correctly")
	}
	if tc.Mode != InvalidationEager {
		t.Error("Mode not set correctly")
	}
	if tc.Depth != 3 {
		t.Error("Depth not set correctly")
	}
	if tc.MaxRescanFiles != 100 {
		t.Error("MaxRescanFiles not set correctly")
	}
	if tc.MaxRescanMs != 500 {
		t.Error("MaxRescanMs not set correctly")
	}
}

func TestDeltaStats_Struct(t *testing.T) {
	stats := DeltaStats{
		FilesChanged:   5,
		FilesAdded:     2,
		FilesDeleted:   1,
		SymbolsAdded:   20,
		SymbolsRemoved: 5,
		RefsAdded:      100,
		RefsRemoved:    10,
		CallsAdded:     50,
		IndexState:     "partial",
	}

	if stats.FilesChanged != 5 {
		t.Error("FilesChanged not set correctly")
	}
	if stats.CallsAdded != 50 {
		t.Error("CallsAdded not set correctly")
	}
	if stats.IndexState != "partial" {
		t.Error("IndexState not set correctly")
	}
}

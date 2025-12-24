package backends

import (
	"context"
	"testing"

	"ckb/internal/logging"
)

// ladderMockBackend implements Backend for ladder testing
type ladderMockBackend struct {
	id           BackendID
	available    bool
	capabilities []string
	priority     int
}

func (m *ladderMockBackend) ID() BackendID       { return m.id }
func (m *ladderMockBackend) IsAvailable() bool   { return m.available }
func (m *ladderMockBackend) Capabilities() []string { return m.capabilities }
func (m *ladderMockBackend) Priority() int       { return m.priority }

func newLadderMockBackend(id BackendID, available bool, caps []string) *ladderMockBackend {
	return &ladderMockBackend{
		id:           id,
		available:    available,
		capabilities: caps,
		priority:     1,
	}
}

func testLogger() *logging.Logger {
	return logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
}

func TestNewBackendLadder(t *testing.T) {
	policy := DefaultQueryPolicy()
	logger := testLogger()

	ladder := NewBackendLadder(policy, logger)
	if ladder == nil {
		t.Fatal("NewBackendLadder returned nil")
	}
	if ladder.policy != policy {
		t.Error("ladder.policy not set correctly")
	}
}

func TestSelectBackends_PreferFirst(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendLSP},
		AlwaysUse:              []BackendID{},
		MergeMode:              MergeModePreferFirst,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP: newLadderMockBackend(BackendSCIP, true, []string{"symbol-info", "symbol-search"}),
		BackendLSP:  newLadderMockBackend(BackendLSP, true, []string{"symbol-info", "symbol-search"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	selected := ladder.SelectBackends(ctx, backends, req)

	// Should only select first available (SCIP) in prefer-first mode
	if len(selected) != 1 {
		t.Errorf("SelectBackends() selected %d backends, want 1", len(selected))
	}
	if len(selected) > 0 && selected[0] != BackendSCIP {
		t.Errorf("SelectBackends() selected %v, want %v", selected[0], BackendSCIP)
	}
}

func TestSelectBackends_Union(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendLSP},
		AlwaysUse:              []BackendID{},
		MergeMode:              MergeModeUnion,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP: newLadderMockBackend(BackendSCIP, true, []string{"symbol-info"}),
		BackendLSP:  newLadderMockBackend(BackendLSP, true, []string{"symbol-info"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	selected := ladder.SelectBackends(ctx, backends, req)

	// Should select all available in union mode
	if len(selected) != 2 {
		t.Errorf("SelectBackends() selected %d backends, want 2", len(selected))
	}
}

func TestSelectBackends_AlwaysUse(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP},
		AlwaysUse:              []BackendID{BackendGit},
		MergeMode:              MergeModePreferFirst,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP: newLadderMockBackend(BackendSCIP, true, []string{"symbol-info"}),
		BackendGit:  newLadderMockBackend(BackendGit, true, []string{"blame"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	selected := ladder.SelectBackends(ctx, backends, req)

	// Should include always-use backend (Git) plus primary (SCIP)
	if len(selected) < 1 {
		t.Errorf("SelectBackends() selected %d backends, want at least 1", len(selected))
	}

	// Git should be in selected
	hasGit := false
	for _, id := range selected {
		if id == BackendGit {
			hasGit = true
			break
		}
	}
	if !hasGit {
		t.Error("AlwaysUse backend (git) not selected")
	}
}

func TestSelectBackends_SkipsUnavailable(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendLSP},
		MergeMode:              MergeModePreferFirst,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP: newLadderMockBackend(BackendSCIP, false, []string{"symbol-info"}), // unavailable
		BackendLSP:  newLadderMockBackend(BackendLSP, true, []string{"symbol-info"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	selected := ladder.SelectBackends(ctx, backends, req)

	// Should skip SCIP (unavailable) and select LSP
	if len(selected) != 1 {
		t.Errorf("SelectBackends() selected %d backends, want 1", len(selected))
	}
	if len(selected) > 0 && selected[0] != BackendLSP {
		t.Errorf("SelectBackends() selected %v, want %v", selected[0], BackendLSP)
	}
}

func TestSelectBackends_NoBackendsAvailable(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendLSP},
		MergeMode:              MergeModePreferFirst,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP: newLadderMockBackend(BackendSCIP, false, []string{}),
		BackendLSP:  newLadderMockBackend(BackendLSP, false, []string{}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	selected := ladder.SelectBackends(ctx, backends, req)

	if len(selected) != 0 {
		t.Errorf("SelectBackends() selected %d backends when none available, want 0", len(selected))
	}
}

func TestFallbackToNext(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendGlean, BackendLSP},
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP:  newLadderMockBackend(BackendSCIP, true, []string{"symbol-info"}),
		BackendGlean: newLadderMockBackend(BackendGlean, true, []string{"symbol-info"}),
		BackendLSP:   newLadderMockBackend(BackendLSP, true, []string{"symbol-info"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	// First failure: SCIP
	failed := []BackendID{BackendSCIP}
	next := ladder.FallbackToNext(ctx, backends, failed, req)
	if next != BackendGlean {
		t.Errorf("FallbackToNext() after SCIP failed = %v, want %v", next, BackendGlean)
	}

	// Second failure: SCIP + Glean
	failed = []BackendID{BackendSCIP, BackendGlean}
	next = ladder.FallbackToNext(ctx, backends, failed, req)
	if next != BackendLSP {
		t.Errorf("FallbackToNext() after SCIP+Glean failed = %v, want %v", next, BackendLSP)
	}

	// All failed
	failed = []BackendID{BackendSCIP, BackendGlean, BackendLSP}
	next = ladder.FallbackToNext(ctx, backends, failed, req)
	if next != "" {
		t.Errorf("FallbackToNext() when all failed = %v, want empty", next)
	}
}

func TestSelectSupplementBackends(t *testing.T) {
	policy := &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendGlean, BackendLSP},
		SupplementThreshold:    0.8,
	}
	ladder := NewBackendLadder(policy, testLogger())

	backends := map[BackendID]Backend{
		BackendSCIP:  newLadderMockBackend(BackendSCIP, true, []string{"symbol-info"}),
		BackendGlean: newLadderMockBackend(BackendGlean, true, []string{"symbol-info"}),
		BackendLSP:   newLadderMockBackend(BackendLSP, true, []string{"symbol-info"}),
	}

	ctx := context.Background()
	req := QueryRequest{Type: QueryTypeSymbol}

	t.Run("no supplement if complete", func(t *testing.T) {
		primaryCompleteness := NewCompletenessInfo(0.95, FullBackend, "complete")
		supplements := ladder.SelectSupplementBackends(ctx, backends, BackendSCIP, primaryCompleteness, req)
		if len(supplements) != 0 {
			t.Errorf("SelectSupplementBackends() with complete result = %d, want 0", len(supplements))
		}
	})

	t.Run("supplements when below threshold", func(t *testing.T) {
		primaryCompleteness := NewCompletenessInfo(0.6, BestEffortLSP, "incomplete")
		supplements := ladder.SelectSupplementBackends(ctx, backends, BackendLSP, primaryCompleteness, req)
		// Should suggest SCIP and Glean (higher priority than LSP)
		if len(supplements) < 1 {
			t.Error("SelectSupplementBackends() should suggest supplements for low completeness")
		}
	})
}

func TestContains(t *testing.T) {
	slice := []BackendID{BackendSCIP, BackendLSP, BackendGit}

	tests := []struct {
		item BackendID
		want bool
	}{
		{BackendSCIP, true},
		{BackendLSP, true},
		{BackendGit, true},
		{BackendGlean, false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.item), func(t *testing.T) {
			if got := contains(slice, tt.item); got != tt.want {
				t.Errorf("contains(%v) = %v, want %v", tt.item, got, tt.want)
			}
		})
	}
}

func TestContainsCapability(t *testing.T) {
	caps := []string{"symbol-info", "find-references", "workspace-symbols"}

	tests := []struct {
		cap  string
		want bool
	}{
		{"symbol-info", true},
		{"find-references", true},
		{"workspace-symbols", true},
		{"goto-definition", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.cap, func(t *testing.T) {
			if got := containsCapability(caps, tt.cap); got != tt.want {
				t.Errorf("containsCapability(%q) = %v, want %v", tt.cap, got, tt.want)
			}
		})
	}
}

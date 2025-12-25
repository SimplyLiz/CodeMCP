package backends

import (
	"testing"
)

func TestNewCompletenessInfo(t *testing.T) {
	info := NewCompletenessInfo(0.85, BestEffortLSP, "LSP workspace ready")

	if info.Score != 0.85 {
		t.Errorf("Score = %f, want 0.85", info.Score)
	}
	if info.Reason != BestEffortLSP {
		t.Errorf("Reason = %v, want %v", info.Reason, BestEffortLSP)
	}
	if info.Details != "LSP workspace ready" {
		t.Errorf("Details = %q, want %q", info.Details, "LSP workspace ready")
	}
}

func TestCompletenessInfo_IsComplete(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  bool
	}{
		{"complete at 1.0", 1.0, true},
		{"complete at 0.95", 0.95, true},
		{"complete at 0.99", 0.99, true},
		{"incomplete at 0.94", 0.94, false},
		{"incomplete at 0.5", 0.5, false},
		{"incomplete at 0.0", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := CompletenessInfo{Score: tt.score}
			if got := info.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletenessInfo_IsBestEffort(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  bool
	}{
		{"not best effort at 1.0", 1.0, false},
		{"not best effort at 0.95", 0.95, false},
		{"best effort at 0.94", 0.94, true},
		{"best effort at 0.5", 0.5, true},
		{"best effort at 0.75", 0.75, true},
		{"not best effort at 0.49", 0.49, false},
		{"not best effort at 0.0", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := CompletenessInfo{Score: tt.score}
			if got := info.IsBestEffort(); got != tt.want {
				t.Errorf("IsBestEffort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletenessInfo_IsIncomplete(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  bool
	}{
		{"not incomplete at 1.0", 1.0, false},
		{"not incomplete at 0.5", 0.5, false},
		{"incomplete at 0.49", 0.49, true},
		{"incomplete at 0.25", 0.25, true},
		{"incomplete at 0.0", 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := CompletenessInfo{Score: tt.score}
			if got := info.IsIncomplete(); got != tt.want {
				t.Errorf("IsIncomplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeCompleteness(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := MergeCompleteness([]CompletenessInfo{})
		if result.Score != 0.0 {
			t.Errorf("Score = %f, want 0.0", result.Score)
		}
		if result.Reason != NoBackendAvailable {
			t.Errorf("Reason = %v, want %v", result.Reason, NoBackendAvailable)
		}
	})

	t.Run("single input", func(t *testing.T) {
		input := NewCompletenessInfo(0.75, BestEffortLSP, "single")
		result := MergeCompleteness([]CompletenessInfo{input})
		if result.Score != 0.75 {
			t.Errorf("Score = %f, want 0.75", result.Score)
		}
		if result.Reason != BestEffortLSP {
			t.Errorf("Reason = %v, want %v", result.Reason, BestEffortLSP)
		}
	})

	t.Run("uses complete result if available", func(t *testing.T) {
		infos := []CompletenessInfo{
			NewCompletenessInfo(0.5, BestEffortLSP, "lsp"),
			NewCompletenessInfo(1.0, FullBackend, "scip"),
			NewCompletenessInfo(0.3, TimedOut, "git"),
		}
		result := MergeCompleteness(infos)
		if result.Score != 1.0 {
			t.Errorf("Score = %f, want 1.0", result.Score)
		}
		if result.Reason != FullBackend {
			t.Errorf("Reason = %v, want %v", result.Reason, FullBackend)
		}
	})

	t.Run("averages when no complete result", func(t *testing.T) {
		infos := []CompletenessInfo{
			NewCompletenessInfo(0.6, BestEffortLSP, "lsp"),
			NewCompletenessInfo(0.8, BestEffortLSP, "other"),
		}
		result := MergeCompleteness(infos)
		// Average of 0.6 and 0.8 is 0.7
		if result.Score != 0.7 {
			t.Errorf("Score = %f, want 0.7", result.Score)
		}
	})

	t.Run("uses highest scoring reason when averaging", func(t *testing.T) {
		infos := []CompletenessInfo{
			NewCompletenessInfo(0.4, TimedOut, "slow"),
			NewCompletenessInfo(0.8, BestEffortLSP, "fast"),
		}
		result := MergeCompleteness(infos)
		if result.Reason != BestEffortLSP {
			t.Errorf("Reason = %v, want %v", result.Reason, BestEffortLSP)
		}
	})
}

func TestCompletenessReasonConstants(t *testing.T) {
	// Verify reason constants are correct strings
	tests := []struct {
		reason CompletenessReason
		want   string
	}{
		{FullBackend, "full-backend"},
		{BestEffortLSP, "best-effort-lsp"},
		{WorkspaceNotReady, "workspace-not-ready"},
		{TimedOut, "timed-out"},
		{Truncated, "truncated"},
		{SingleFileOnly, "single-file-only"},
		{NoBackendAvailable, "no-backend-available"},
		{IndexStale, "index-stale"},
		{Unknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			if string(tt.reason) != tt.want {
				t.Errorf("Reason = %q, want %q", string(tt.reason), tt.want)
			}
		})
	}
}

package mcp

import (
	"testing"
	"time"
)

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"1 hour", 1 * time.Hour, "1 hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"1 day", 24 * time.Hour, "1 day ago"},
		{"5 days", 5 * 24 * time.Hour, "5 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Now().Add(-tt.ago)
			result := formatTimeAgo(timestamp)
			if result != tt.want {
				t.Errorf("formatTimeAgo(%v ago) = %q, want %q", tt.ago, result, tt.want)
			}
		})
	}
}

func TestGetBackendRemediation(t *testing.T) {
	tests := []struct {
		backendID string
		wantHas   string
	}{
		{"scip", "ckb index"},
		{"lsp", "config"},
		{"git", "git init"},
		{"unknown", "ckb doctor"},
	}

	for _, tt := range tests {
		t.Run(tt.backendID, func(t *testing.T) {
			result := getBackendRemediation(tt.backendID)
			if result == "" {
				t.Errorf("getBackendRemediation(%q) returned empty", tt.backendID)
			}
			if tt.wantHas != "" && !stringContains(result, tt.wantHas) {
				t.Errorf("getBackendRemediation(%q) = %q, want to contain %q", tt.backendID, result, tt.wantHas)
			}
		})
	}
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestReindexOutputStructure(t *testing.T) {
	// Verify ReindexOutput can be constructed
	output := &ReindexOutput{
		Status:  "action_required",
		Message: "Index is stale. Run 'ckb index' to refresh.",
	}

	if output.Status != "action_required" {
		t.Errorf("expected Status 'action_required', got %q", output.Status)
	}

	// Test with optional fields
	outputWithDuration := &ReindexOutput{
		JobID:    "job-123",
		Status:   "completed",
		Duration: "2.5s",
		Message:  "Reindex completed",
	}

	if outputWithDuration.JobID != "job-123" {
		t.Errorf("expected JobID 'job-123', got %q", outputWithDuration.JobID)
	}
}

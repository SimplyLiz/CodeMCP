package mcp

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "just now - 30 seconds ago",
			duration: 30 * time.Second,
			want:     "just now",
		},
		{
			name:     "1 minute ago",
			duration: 1 * time.Minute,
			want:     "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			duration: 5 * time.Minute,
			want:     "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			duration: 1 * time.Hour,
			want:     "1 hour ago",
		},
		{
			name:     "3 hours ago",
			duration: 3 * time.Hour,
			want:     "3 hours ago",
		},
		{
			name:     "1 day ago",
			duration: 24 * time.Hour,
			want:     "1 day ago",
		},
		{
			name:     "7 days ago",
			duration: 7 * 24 * time.Hour,
			want:     "7 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now().Add(-tt.duration)
			got := formatTimeAgo(testTime)
			if got != tt.want {
				t.Errorf("formatTimeAgo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetBackendRemediation(t *testing.T) {
	tests := []struct {
		backendID string
		wantPart  string
	}{
		{
			backendID: "scip",
			wantPart:  "ckb index",
		},
		{
			backendID: "lsp",
			wantPart:  "config.json",
		},
		{
			backendID: "git",
			wantPart:  "git init",
		},
		{
			backendID: "unknown",
			wantPart:  "ckb doctor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.backendID, func(t *testing.T) {
			got := getBackendRemediation(tt.backendID)
			if got == "" {
				t.Error("getBackendRemediation() returned empty string")
			}
			// Check that the remediation contains expected text
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("getBackendRemediation(%q) = %q, want to contain %q", tt.backendID, got, tt.wantPart)
			}
		})
	}
}

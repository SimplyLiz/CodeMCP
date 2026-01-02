// Package mcp contains v8.0 MCP tool implementations.
// v8.0 focuses on reliability and error clarity as the foundation for compound operations.
package mcp

import (
	"fmt"
	"time"

	"ckb/internal/envelope"
)

// getBackendRemediation returns remediation steps for a backend.
func getBackendRemediation(backendID string) string {
	switch backendID {
	case "scip":
		return "Run 'ckb index' to generate SCIP index for code intelligence"
	case "lsp":
		return "Configure LSP server in .ckb/config.json or run 'ckb setup'"
	case "git":
		return "Initialize git with 'git init' or navigate to a git repository"
	default:
		return "Run 'ckb doctor' to diagnose configuration"
	}
}

// formatTimeAgo formats a time as a human-readable "X ago" string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// ReindexInput is the input for the reindex tool.
type ReindexInput struct {
	Scope string `json:"scope"` // "full" | "incremental" | "file:<path>"
	Async bool   `json:"async"` // return immediately, poll status
}

// ReindexOutput is the output from the reindex tool.
type ReindexOutput struct {
	JobID    string `json:"jobId,omitempty"`
	Status   string `json:"status"` // started | completed | failed | skipped | action_required
	Duration string `json:"duration,omitempty"`
	Message  string `json:"message,omitempty"`
}

// toolReindex implements the reindex MCP tool.
// It triggers a refresh of the SCIP index without restarting CKB.
func (s *MCPServer) toolReindex(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing reindex", map[string]interface{}{
		"params": params,
	})

	// Parse parameters
	scope := "full"
	if scopeStr, ok := params["scope"].(string); ok && scopeStr != "" {
		scope = scopeStr
	}

	// Check current index state using getIndexStaleness
	indexInfo := s.getIndexStaleness()

	// For MCP context, we can't directly run shell commands.
	// Instead, provide guidance on how to reindex.
	//
	// In the future, this could integrate with the daemon's RefreshManager
	// if the daemon is running. For now, return actionable guidance.

	isFresh, _ := indexInfo["fresh"].(bool)
	if isFresh {
		return envelope.Operational(&ReindexOutput{
			Status:  "skipped",
			Message: "Index is already fresh. No reindex needed.",
		}), nil
	}

	// Index is stale - provide remediation
	message := "Index is stale. "
	if commitsBehind, ok := indexInfo["commitsBehind"].(int); ok && commitsBehind > 0 {
		message += fmt.Sprintf("Index is %d commits behind HEAD. ", commitsBehind)
	}
	if reason, ok := indexInfo["reason"].(string); ok && reason != "" {
		message += fmt.Sprintf("Reason: %s. ", reason)
	}

	remediation := "Run 'ckb index' in your terminal to refresh the index."
	if scope == "incremental" {
		remediation = "Run 'ckb index --incremental' for faster incremental update."
	}

	return envelope.Operational(&ReindexOutput{
		Status:  "action_required",
		Message: message + remediation,
	}), nil
}

package backends

// CompletenessReason describes why a result has a particular completeness score
type CompletenessReason string

const (
	// FullBackend indicates results from a complete index (SCIP/Glean)
	FullBackend CompletenessReason = "full-backend"

	// BestEffortLSP indicates LSP did its best but may be incomplete
	BestEffortLSP CompletenessReason = "best-effort-lsp"

	// WorkspaceNotReady indicates LSP is still initializing
	WorkspaceNotReady CompletenessReason = "workspace-not-ready"

	// TimedOut indicates query exceeded time limit
	TimedOut CompletenessReason = "timed-out"

	// Truncated indicates results were cut off due to limits
	Truncated CompletenessReason = "truncated"

	// SingleFileOnly indicates only single-file analysis was possible
	SingleFileOnly CompletenessReason = "single-file-only"

	// NoBackendAvailable indicates no backend could handle the query
	NoBackendAvailable CompletenessReason = "no-backend-available"

	// IndexStale indicates index is out of date
	IndexStale CompletenessReason = "index-stale"

	// Unknown indicates unknown completeness status
	Unknown CompletenessReason = "unknown"
)

// CompletenessInfo tracks the quality and completeness of a result
type CompletenessInfo struct {
	// Score represents completeness from 0.0 (incomplete) to 1.0 (complete)
	Score float64

	// Reason explains why this completeness score was assigned
	Reason CompletenessReason

	// Details provides additional context about completeness
	Details string
}

// NewCompletenessInfo creates a new CompletenessInfo with score and reason
func NewCompletenessInfo(score float64, reason CompletenessReason, details string) CompletenessInfo {
	return CompletenessInfo{
		Score:   score,
		Reason:  reason,
		Details: details,
	}
}

// IsComplete returns true if the result is considered complete (score >= 0.95)
func (c CompletenessInfo) IsComplete() bool {
	return c.Score >= 0.95
}

// IsBestEffort returns true if this is a best-effort result (0.5 <= score < 0.95)
func (c CompletenessInfo) IsBestEffort() bool {
	return c.Score >= 0.5 && c.Score < 0.95
}

// IsIncomplete returns true if the result is incomplete (score < 0.5)
func (c CompletenessInfo) IsIncomplete() bool {
	return c.Score < 0.5
}

// MergeCompleteness combines completeness scores from multiple backends
// Uses the highest score if all reasons are compatible, otherwise uses weighted average
func MergeCompleteness(infos []CompletenessInfo) CompletenessInfo {
	if len(infos) == 0 {
		return NewCompletenessInfo(0.0, NoBackendAvailable, "No backends provided results")
	}

	if len(infos) == 1 {
		return infos[0]
	}

	// Find the highest score
	maxScore := 0.0
	var maxInfo CompletenessInfo
	for _, info := range infos {
		if info.Score > maxScore {
			maxScore = info.Score
			maxInfo = info
		}
	}

	// If we have a complete result, use it
	if maxScore >= 0.95 {
		return maxInfo
	}

	// Otherwise, compute weighted average
	totalScore := 0.0
	for _, info := range infos {
		totalScore += info.Score
	}
	avgScore := totalScore / float64(len(infos))

	// Use reason from highest-scoring backend
	return NewCompletenessInfo(avgScore, maxInfo.Reason, "Merged from multiple backends")
}

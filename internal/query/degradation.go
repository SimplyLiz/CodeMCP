package query

import "fmt"

// DegradationWarning describes reduced capability with actionable fix guidance.
type DegradationWarning struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	CapabilityPercent int    `json:"capabilityPercent"`
	FixCommand        string `json:"fixCommand,omitempty"`
}

// GenerateDegradationWarnings checks backend availability and staleness,
// returning warnings that explain reduced capability and how to fix it.
func GenerateDegradationWarnings(scipAvailable, gitAvailable, scipStale bool, commitsBehind int) []DegradationWarning {
	var warnings []DegradationWarning

	if !scipAvailable {
		warnings = append(warnings, DegradationWarning{
			Code:              "scip_missing",
			Message:           "Running at ~40% capability — run 'ckb index' for full results",
			CapabilityPercent: 40,
			FixCommand:        "ckb index",
		})
	} else if scipStale && commitsBehind > 5 {
		warnings = append(warnings, DegradationWarning{
			Code:              "scip_stale",
			Message:           fmt.Sprintf("Index is %d commits behind (~60%% capability) — run 'ckb index'", commitsBehind),
			CapabilityPercent: 60,
			FixCommand:        "ckb index",
		})
	}

	if !gitAvailable {
		warnings = append(warnings, DegradationWarning{
			Code:              "git_unavailable",
			Message:           "Git not available. History-based features disabled (~20% capability)",
			CapabilityPercent: 20,
		})
	}

	return warnings
}

// GetDegradationWarnings inspects current engine backend state and returns
// any applicable degradation warnings.
func (e *Engine) GetDegradationWarnings() []DegradationWarning {
	scipAvailable := e.scipAdapter != nil && e.scipAdapter.IsAvailable()
	gitAvailable := e.gitAdapter != nil && e.gitAdapter.IsAvailable()

	scipStale := false
	commitsBehind := 0

	if scipAvailable {
		indexInfo := e.scipAdapter.GetIndexInfo()
		if indexInfo != nil && indexInfo.Freshness != nil {
			scipStale = indexInfo.Freshness.IsStale()
			commitsBehind = indexInfo.Freshness.CommitsBehindHead
		}
	}

	return GenerateDegradationWarnings(scipAvailable, gitAvailable, scipStale, commitsBehind)
}

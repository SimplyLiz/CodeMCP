package main

import (
	"time"

	"ckb/internal/compression"
	"ckb/internal/output"
	"ckb/internal/repostate"
)

// Response is the common wrapper for all CKB command responses
// Per Section 7.2: CompressedResponse format
type Response struct {
	CkbVersion    string                          `json:"ckbVersion"`
	SchemaVersion int                             `json:"schemaVersion"`
	Capabilities  []string                        `json:"capabilities"`
	Facts         interface{}                     `json:"facts"`
	Explanation   string                          `json:"explanation,omitempty"`
	Provenance    *Provenance                     `json:"provenance"`
	Drilldowns    []output.Drilldown              `json:"drilldowns"`
	Compression   *compression.CompressionMetrics `json:"compression,omitempty"`
}

// Provenance tracks the source and quality of response data
// Per Section 7.3: Provenance information
type Provenance struct {
	RepoStateId     string                `json:"repoStateId"`
	RepoStateDirty  bool                  `json:"repoStateDirty"`
	RepoStateMode   string                `json:"repoStateMode"`
	Backends        []BackendContribution `json:"backends"`
	Completeness    *CompletenessInfo     `json:"completeness"`
	IndexFreshness  *IndexFreshness       `json:"indexFreshness,omitempty"`
	Warnings        []string              `json:"warnings"`
	Timeouts        []string              `json:"timeouts"`
	Truncations     []string              `json:"truncations"`
	QueryDurationMs int64                 `json:"queryDurationMs"`
}

// BackendContribution tracks which backend provided data
type BackendContribution struct {
	BackendId    string   `json:"backendId"`
	Capabilities []string `json:"capabilities"`
	DataSources  []string `json:"dataSources,omitempty"` // e.g., ["index", "runtime"]
	DurationMs   int64    `json:"durationMs,omitempty"`
}

// CompletenessInfo describes result completeness
type CompletenessInfo struct {
	Score            float64 `json:"score"`            // 0.0-1.0
	Source           string  `json:"source"`           // "scip", "lsp", "git", etc.
	IsWorkspaceReady bool    `json:"isWorkspaceReady"` // For LSP backends
	IsBestEffort     bool    `json:"isBestEffort"`     // True if not fully complete
}

// IndexFreshness describes SCIP index staleness
type IndexFreshness struct {
	StaleAgainstHead  bool   `json:"staleAgainstHead"`
	LastIndexedCommit string `json:"lastIndexedCommit,omitempty"`
	HeadCommit        string `json:"headCommit,omitempty"`
}

// NewResponse creates a basic response with provenance
func NewResponse(facts interface{}, repoState *repostate.RepoState, repoStateMode string, durationMs int64) *Response {
	return &Response{
		CkbVersion:    CKBVersion,
		SchemaVersion: 1,
		Capabilities:  []string{},
		Facts:         facts,
		Provenance: &Provenance{
			RepoStateId:     repoState.RepoStateID,
			RepoStateDirty:  repoState.Dirty,
			RepoStateMode:   repoStateMode,
			Backends:        []BackendContribution{},
			Completeness:    &CompletenessInfo{Score: 1.0, Source: "direct"},
			Warnings:        []string{},
			Timeouts:        []string{},
			Truncations:     []string{},
			QueryDurationMs: durationMs,
		},
		Drilldowns: []output.Drilldown{},
	}
}

// AddBackend adds a backend contribution to the provenance
func (r *Response) AddBackend(backendId string, capabilities []string, durationMs int64) {
	r.Provenance.Backends = append(r.Provenance.Backends, BackendContribution{
		BackendId:    backendId,
		Capabilities: capabilities,
		DurationMs:   durationMs,
	})
}

// AddWarning adds a warning to the provenance
func (r *Response) AddWarning(warning string) {
	r.Provenance.Warnings = append(r.Provenance.Warnings, warning)
}

// AddTimeout records a backend timeout
func (r *Response) AddTimeout(backend string) {
	r.Provenance.Timeouts = append(r.Provenance.Timeouts, backend)
}

// AddTruncation records a truncation event
func (r *Response) AddTruncation(description string) {
	r.Provenance.Truncations = append(r.Provenance.Truncations, description)
}

// SetCompleteness updates the completeness information
func (r *Response) SetCompleteness(score float64, source string, isBestEffort bool) {
	r.Provenance.Completeness = &CompletenessInfo{
		Score:        score,
		Source:       source,
		IsBestEffort: isBestEffort,
	}
}

// SetIndexFreshness updates the index freshness information
func (r *Response) SetIndexFreshness(stale bool, lastCommit, headCommit string) {
	r.Provenance.IndexFreshness = &IndexFreshness{
		StaleAgainstHead:  stale,
		LastIndexedCommit: lastCommit,
		HeadCommit:        headCommit,
	}
}

// measureDuration is a helper to measure execution time
func measureDuration(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

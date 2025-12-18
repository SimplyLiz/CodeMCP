// Package explain provides symbol explanation functionality.
// It answers "Why does this code exist?" by showing origin, history, co-changes, and warnings.
package explain

import "time"

// SymbolExplanation is the complete explanation for a symbol
type SymbolExplanation struct {
	// Identity
	Symbol   string `json:"symbol"`
	SymbolId string `json:"symbolId,omitempty"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Module   string `json:"module,omitempty"`

	// Origin - who wrote it and why
	Origin Origin `json:"origin"`

	// Evolution - how it changed over time
	Evolution Evolution `json:"evolution"`

	// Ownership - who owns it now
	Ownership OwnershipInfo `json:"ownership,omitempty"`

	// Co-change patterns
	CoChangePatterns []CoChange `json:"coChangePatterns,omitempty"`

	// External references from commit messages
	References References `json:"references,omitempty"`

	// Runtime usage (from telemetry if enabled)
	ObservedUsage *ObservedUsage `json:"observedUsage,omitempty"`

	// Warnings/insights
	Warnings []Warning `json:"warnings,omitempty"`
}

// Origin represents the origin of a symbol
type Origin struct {
	Author        string    `json:"author"`
	AuthorEmail   string    `json:"authorEmail,omitempty"`
	Date          time.Time `json:"date"`
	CommitSha     string    `json:"commitSha"`
	CommitMessage string    `json:"commitMessage"` // the "why"
}

// Evolution represents how a symbol evolved over time
type Evolution struct {
	TotalCommits  int           `json:"totalCommits"`
	Contributors  []Contributor `json:"contributors"`
	LastTouched   time.Time     `json:"lastTouched"`
	LastTouchedBy string        `json:"lastTouchedBy"`
	Timeline      []TimelineEntry `json:"timeline,omitempty"`
}

// Contributor represents a contributor to a symbol
type Contributor struct {
	Name        string    `json:"name"`
	Email       string    `json:"email,omitempty"`
	CommitCount int       `json:"commitCount"`
	FirstCommit time.Time `json:"firstCommit"`
	LastCommit  time.Time `json:"lastCommit"`
}

// TimelineEntry represents a single entry in the evolution timeline
type TimelineEntry struct {
	Date    time.Time `json:"date"`
	Author  string    `json:"author"`
	Message string    `json:"message"`
	Sha     string    `json:"sha"`
}

// OwnershipInfo represents ownership information
type OwnershipInfo struct {
	CurrentOwner   string `json:"currentOwner,omitempty"`
	PrimaryContact string `json:"primaryContact,omitempty"` // most active recent contributor
	Team           string `json:"team,omitempty"`
}

// CoChange represents a file that often changes with the target
type CoChange struct {
	Symbol        string  `json:"symbol,omitempty"`
	File          string  `json:"file"`
	Correlation   float64 `json:"correlation"` // 0-1
	CoChangeCount int     `json:"coChangeCount"`
	TotalChanges  int     `json:"totalChanges"`
}

// References represents external references found in commit messages
type References struct {
	Issues      []string `json:"issues,omitempty"`      // #27, #84
	PRs         []string `json:"prs,omitempty"`         // PR #89
	JiraTickets []string `json:"jiraTickets,omitempty"` // PROJ-123
}

// ObservedUsage represents runtime usage from telemetry
type ObservedUsage struct {
	CallsPerDay float64   `json:"callsPerDay"`
	ErrorRate   float64   `json:"errorRate"`
	LastCalled  time.Time `json:"lastCalled,omitempty"`
	Trend       string    `json:"trend"` // "increasing" | "stable" | "decreasing"
}

// Warning represents a warning about the symbol
type Warning struct {
	Type     string `json:"type"`     // "temporary_code" | "bus_factor" | "high_coupling" | "stale" | "complex"
	Message  string `json:"message"`
	Severity string `json:"severity"` // "info" | "warning" | "critical"
}

// ExplainOptions configures the symbol explanation
type ExplainOptions struct {
	RepoRoot      string // Repository root path
	Symbol        string // Symbol name or file:line
	IncludeUsage  bool   // Include telemetry data (default: true)
	IncludeCoChange bool // Include co-change analysis (default: true)
	HistoryLimit  int    // Number of timeline entries (default: 10)
}

// WarningSeverity constants
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// WarningType constants
const (
	WarningTemporaryCode = "temporary_code"
	WarningBusFactor     = "bus_factor"
	WarningHighCoupling  = "high_coupling"
	WarningStale         = "stale"
	WarningComplex       = "complex"
)

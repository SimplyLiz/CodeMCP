// Package compliance provides regulatory compliance auditing for codebases.
// It maps static analysis findings to specific regulation articles/clauses
// across GDPR, EU AI Act, ISO 27001, ISO 27701, and IEC 61508 frameworks.
package compliance

import (
	"context"
	"log/slog"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// FrameworkID identifies a regulatory framework.
type FrameworkID string

const (
	FrameworkGDPR     FrameworkID = "gdpr"
	FrameworkEUAIAct  FrameworkID = "eu-ai-act"
	FrameworkISO27001 FrameworkID = "iso27001"
	FrameworkISO27701 FrameworkID = "iso27701"
	FrameworkIEC61508 FrameworkID = "iec61508"
)

// AllFrameworkIDs returns all supported framework identifiers.
var AllFrameworkIDs = []FrameworkID{
	FrameworkGDPR,
	FrameworkEUAIAct,
	FrameworkISO27001,
	FrameworkISO27701,
	FrameworkIEC61508,
}

// Framework defines a regulatory framework that can be audited.
type Framework interface {
	ID() FrameworkID
	Name() string    // e.g., "GDPR (Regulation (EU) 2016/679)"
	Version() string // e.g., "2016/679"
	Checks() []Check
}

// Check is a single compliance check within a framework.
type Check interface {
	ID() string          // e.g., "pii-in-logs"
	Name() string        // Human-readable: "PII in Log Statements"
	Article() string     // e.g., "Art. 25(1) GDPR" or "A.8.12 ISO 27001:2022"
	Severity() string    // "error", "warning", "info"
	Run(ctx context.Context, scope *ScanScope) ([]Finding, error)
}

// Finding represents a single compliance issue mapped to a regulation clause.
type Finding struct {
	CheckID    string      `json:"checkId"`
	Framework  FrameworkID `json:"framework"`
	Article    string      `json:"article"`    // Specific regulation clause
	Severity   string      `json:"severity"`   // "error", "warning", "info"
	File       string      `json:"file"`
	StartLine  int         `json:"startLine,omitempty"`
	EndLine    int         `json:"endLine,omitempty"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion,omitempty"`
	Confidence float64     `json:"confidence"` // 0.0-1.0, mandatory
	CWE        string      `json:"cwe,omitempty"`
}

// ToReviewFinding converts a compliance finding to the standard ReviewFinding type.
func (f Finding) ToReviewFinding() query.ReviewFinding {
	ruleID := "ckb/compliance/" + string(f.Framework) + "/" + f.CheckID
	tier := 2 // default: important
	if f.Severity == "error" {
		tier = 1
	} else if f.Severity == "info" {
		tier = 3
	}

	detail := f.Article
	if f.CWE != "" {
		detail += " (" + f.CWE + ")"
	}

	return query.ReviewFinding{
		Check:      string(f.Framework) + "/" + f.CheckID,
		Severity:   f.Severity,
		File:       f.File,
		StartLine:  f.StartLine,
		EndLine:    f.EndLine,
		Message:    f.Message,
		Detail:     detail,
		Suggestion: f.Suggestion,
		Category:   "compliance",
		RuleID:     ruleID,
		Tier:       tier,
		Confidence: f.Confidence,
	}
}

// ScanScope provides shared context to all checks.
type ScanScope struct {
	RepoRoot           string
	Files              []string // Relative paths to source files
	Config             *ComplianceConfig
	Logger             *slog.Logger
	ComplexityAnalyzer *complexity.Analyzer
}

// AuditOptions configures a compliance audit run.
type AuditOptions struct {
	RepoRoot      string        `json:"repoRoot"`
	Frameworks    []FrameworkID `json:"frameworks"`
	Scope         string        `json:"scope"`         // Path prefix filter
	MinConfidence float64       `json:"minConfidence"` // Default: 0.5
	SILLevel      int           `json:"silLevel"`      // 1-4 for IEC 61508
	Checks        []string      `json:"checks"`        // Filter to specific check IDs
	FailOn        string        `json:"failOn"`        // "error", "warning", "none"
}

// ComplianceReport is the top-level audit result.
type ComplianceReport struct {
	Repo       string              `json:"repo"`
	AnalyzedAt time.Time           `json:"analyzedAt"`
	Frameworks []FrameworkID       `json:"frameworks"`
	Verdict    string              `json:"verdict"` // "pass", "warn", "fail"
	Score      int                 `json:"score"`   // 0-100
	Checks     []query.ReviewCheck `json:"checks"`
	Findings   []query.ReviewFinding `json:"findings"`
	Coverage   []FrameworkCoverage `json:"coverage"`
	Summary    ComplianceSummary   `json:"summary"`
}

// FrameworkCoverage tracks per-framework check results.
type FrameworkCoverage struct {
	Framework   FrameworkID `json:"framework"`
	Name        string      `json:"name"`
	TotalChecks int         `json:"totalChecks"`
	Passed      int         `json:"passed"`
	Warned      int         `json:"warned"`
	Failed      int         `json:"failed"`
	Skipped     int         `json:"skipped"`
	Score       int         `json:"score"` // 0-100
}

// ComplianceSummary is the aggregate overview.
type ComplianceSummary struct {
	TotalFindings   int            `json:"totalFindings"`
	BySeverity      map[string]int `json:"bySeverity"`
	FilesScanned    int            `json:"filesScanned"`
	FilesWithIssues int            `json:"filesWithIssues"`
}

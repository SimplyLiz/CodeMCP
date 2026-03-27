// Package euaiact implements EU AI Act compliance checks.
// Regulation (EU) 2024/1689 — Artificial Intelligence Act.
package euaiact

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkEUAIAct }
func (f *framework) Name() string               { return "EU AI Act (Regulation (EU) 2024/1689)" }
func (f *framework) Version() string            { return "2024/1689" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&missingModelLoggingCheck{},
		&noAuditTrailCheck{},
		&missingConfidenceScoreCheck{},
		&noHumanOverrideCheck{},
		&noKillSwitchCheck{},
		&missingBiasTestingCheck{},
		&noDataProvenanceCheck{},
		&missingVersionTrackingCheck{},
	}
}

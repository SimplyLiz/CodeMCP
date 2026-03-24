// Package iso26262 implements ISO 26262 automotive functional safety checks.
// ISO 26262 — Road vehicles – Functional safety, with ASIL A-D integrity levels.
package iso26262

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkISO26262 }
func (f *framework) Name() string               { return "ISO 26262 (Automotive Functional Safety)" }
func (f *framework) Version() string             { return "2018" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// ASIL-gated checks
		&complexityExceededCheck{},
		&recursionCheck{},
		&dynamicMemoryCheck{},

		// Defensive programming
		&missingNullCheckCheck{},
		&uncheckedReturnCheck{},
	}
}

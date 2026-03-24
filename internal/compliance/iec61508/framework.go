// Package iec61508 implements IEC 61508 / SIL safety integrity checks.
// IEC 61508 — Functional Safety of Electrical/Electronic/Programmable Electronic Safety-related Systems.
package iec61508

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkIEC61508 }
func (f *framework) Name() string               { return "IEC 61508 / SIL (Safety Integrity)" }
func (f *framework) Version() string             { return "2010" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Structural checks
		&gotoUsageCheck{},
		&recursionCheck{},
		&deepNestingCheck{},
		&largeFunctionCheck{},
		&globalStateCheck{},

		// Defensive programming
		&uncheckedErrorCheck{},

		// Complexity
		&complexityExceededCheck{},
	}
}

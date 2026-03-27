// Package do178c implements DO-178C avionics software safety checks.
// DO-178C — Software Considerations in Airborne Systems and Equipment Certification.
package do178c

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkDO178C }
func (f *framework) Name() string               { return "DO-178C (Software Considerations in Airborne Systems)" }
func (f *framework) Version() string            { return "2011" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Dead code
		&deadCodeCheck{},

		// Structural
		&complexityExceededCheck{},
		&gotoUsageCheck{},
		&recursionCheck{},

		// Traceability
		&missingRequirementTagCheck{},
	}
}

// Package misra implements MISRA C:2023 / C++:2023 coding standard checks.
// MISRA — Motor Industry Software Reliability Association guidelines for C and C++.
package misra

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkMISRA }
func (f *framework) Name() string               { return "MISRA C:2023 / C++:2023" }
func (f *framework) Version() string            { return "2023" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Control flow
		&gotoUsageCheck{},
		&unreachableCodeCheck{},
		&missingSwitchDefaultCheck{},

		// Memory
		&dynamicAllocationCheck{},
		&unsafeStringFunctionsCheck{},

		// Type safety
		&implicitConversionCheck{},
	}
}

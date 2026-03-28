// Package ccpa implements CCPA/CPRA (California Privacy Rights Act) compliance checks.
package ccpa

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkCCPA }
func (f *framework) Name() string               { return "CCPA/CPRA (California Privacy Rights Act)" }
func (f *framework) Version() string            { return "2023" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// §1798.120 — Right to opt-out of sale
		&missingDoNotSellCheck{},

		// §1798.100 — Third-party data sharing
		&thirdPartySharingCheck{},

		// §1798.121 — Sensitive personal information
		&sensitivePIExposureCheck{},

		// §1798.110 — Right to know / data access
		&missingDataAccessCheck{},

		// §1798.105 — Right to delete
		&missingDeletionCheck{},
	}
}

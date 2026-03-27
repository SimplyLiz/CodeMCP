// Package iec62443 implements IEC 62443 industrial automation security checks.
// IEC 62443 — Industrial communication networks – Network and system security.
package iec62443

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkIEC62443 }
func (f *framework) Name() string               { return "IEC 62443 (Industrial Automation Security)" }
func (f *framework) Version() string            { return "4-2:2019" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Authentication
		&defaultCredentialsCheck{},
		&missingAuthCheck{},

		// Integrity
		&unvalidatedInputCheck{},
		&missingMessageAuthCheck{},

		// Secure development
		&unsafeFunctionsCheck{},
		&missingErrorHandlingCheck{},
	}
}

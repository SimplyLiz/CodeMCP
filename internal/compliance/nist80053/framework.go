// Package nist80053 implements NIST SP 800-53 Rev 5 security control checks.
package nist80053

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkNIST80053 }
func (f *framework) Name() string               { return "NIST SP 800-53 Rev 5" }
func (f *framework) Version() string            { return "Rev 5" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&missingAccessEnforcementCheck{},
		&defaultCredentialsCheck{},
		&insufficientAuditContentCheck{},
		&missingAuditEventsCheck{},
		&nonFIPSCryptoCheck{},
		&missingInputValidationCheck{},
	}
}

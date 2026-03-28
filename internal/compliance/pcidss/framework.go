// Package pcidss implements PCI DSS 4.0 compliance checks.
// Payment Card Industry Data Security Standard.
package pcidss

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkPCIDSS }
func (f *framework) Name() string               { return "PCI DSS 4.0 (Payment Card Industry)" }
func (f *framework) Version() string            { return "4.0" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&panInSourceCheck{},
		&panInLogsCheck{},
		&sqlInjectionCheck{},
		&xssPreventionCheck{},
		&weakPasswordPolicyCheck{},
		&hardcodedCredentialsCheck{},
	}
}

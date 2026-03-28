// Package hipaa implements HIPAA Security Rule compliance checks.
// Health Insurance Portability and Accountability Act.
package hipaa

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkHIPAA }
func (f *framework) Name() string {
	return "HIPAA (Health Insurance Portability and Accountability Act)"
}
func (f *framework) Version() string { return "Security Rule" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&phiDetectionCheck{},
		&phiInLogsCheck{},
		&missingAuditTrailCheck{},
		&phiUnencryptedCheck{},
		&minimumNecessaryCheck{},
	}
}

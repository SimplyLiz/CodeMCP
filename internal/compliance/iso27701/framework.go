// Package iso27701 implements ISO 27701 privacy extension checks.
// ISO 27701 extends ISO 27001 with privacy-specific controls.
package iso27701

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkISO27701 }
func (f *framework) Name() string               { return "ISO 27701 (Privacy Extension)" }
func (f *framework) Version() string             { return "2019" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&noConsentMechanismCheck{},
		&noDeletionEndpointCheck{},
		&noAccessEndpointCheck{},
		&noDataPortabilityCheck{},
		&noPurposeLoggingCheck{},
	}
}

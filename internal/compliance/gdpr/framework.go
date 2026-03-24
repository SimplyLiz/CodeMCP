// Package gdpr implements GDPR/DSGVO compliance checks.
// Regulation (EU) 2016/679 — General Data Protection Regulation.
package gdpr

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkGDPR }
func (f *framework) Name() string               { return "GDPR (Regulation (EU) 2016/679)" }
func (f *framework) Version() string             { return "2016/679" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&piiDetectionCheck{},
		&piiInLogsCheck{},
		&piiInErrorsCheck{},
		&weakPIICryptoCheck{},
		&plaintextPIICheck{},
		&noRetentionPolicyCheck{},
		&noDeletionEndpointCheck{},
		&missingConsentCheck{},
		&excessiveCollectionCheck{},
		&unencryptedTransportCheck{},
		&missingAccessLoggingCheck{},
	}
}

// Package nis2 implements NIS2 Directive (EU 2022/2555) compliance checks.
package nis2

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkNIS2 }
func (f *framework) Name() string               { return "NIS2 Directive (EU 2022/2555)" }
func (f *framework) Version() string             { return "2022/2555" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Art. 21(2)(d) — Supply chain security
		&unverifiedDependenciesCheck{},
		&missingIntegrityCheckCheck{},

		// Art. 21(2)(e) — Vulnerability handling
		&missingSecurityScanningCheck{},

		// Art. 21(2)(j) — Cryptography
		&deprecatedCryptoCheck{},

		// Art. 21(2)(g) — Access control / secrets
		&hardcodedSecretsCheck{},
	}
}

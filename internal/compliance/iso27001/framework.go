// Package iso27001 implements ISO 27001:2022 Annex A technology control checks.
package iso27001

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkISO27001 }
func (f *framework) Name() string               { return "ISO 27001:2022 (Annex A)" }
func (f *framework) Version() string             { return "2022" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// A.8.4 / A.8.12 — Secret/data leakage
		&hardcodedSecretCheck{},
		&piiInLogsCheck{},

		// A.8.9 — Configuration management
		&hardcodedConfigCheck{},

		// A.8.24 — Cryptography
		&weakCryptoCheck{},
		&insecureRandomCheck{},

		// A.8.28 — Secure coding
		&sqlInjectionCheck{},
		&pathTraversalCheck{},
		&unsafeDeserializationCheck{},

		// A.8.20 — Network security
		&missingTLSCheck{},

		// A.8.27 — Secure architecture
		&corsWildcardCheck{},
	}
}

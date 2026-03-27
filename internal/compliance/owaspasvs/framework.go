// Package owaspasvs implements OWASP ASVS 4.0 compliance checks.
package owaspasvs

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkOWASPASVS }
func (f *framework) Name() string {
	return "OWASP ASVS 4.0 (Application Security Verification Standard)"
}
func (f *framework) Version() string { return "4.0.3" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// V2 — Authentication
		&weakPasswordHashCheck{},
		&hardcodedCredentialsCheck{},

		// V3 — Session Management
		&insecureCookieCheck{},

		// V5 — Validation, Sanitization and Encoding
		&sqlInjectionCheck{},
		&xssPreventionCheck{},
		&commandInjectionCheck{},
		&evalInjectionCheck{},
		&xxeCheck{},

		// V6 — Cryptography
		&weakAlgorithmCheck{},
		&insecureRandomCheck{},

		// V9 — Communications
		&missingTLSCheck{},
		&tlsBypassCheck{},

		// V14 — Configuration
		&asvsCORSWildcardCheck{},
	}
}

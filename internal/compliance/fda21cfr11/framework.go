// Package fda21cfr11 implements FDA 21 CFR Part 11 electronic records checks.
// FDA 21 CFR Part 11 — Electronic Records; Electronic Signatures.
package fda21cfr11

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkFDAPart11 }
func (f *framework) Name() string               { return "FDA 21 CFR Part 11 (Electronic Records)" }
func (f *framework) Version() string            { return "2003" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Audit trail
		&missingAuditTrailCheck{},
		&mutableAuditRecordsCheck{},

		// Authority
		&missingAuthorityCheckCheck{},
		&missingESignatureCheck{},

		// Validation
		&missingInputValidationCheck{},
	}
}

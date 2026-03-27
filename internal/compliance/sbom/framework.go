// Package sbom implements SBOM & Supply Chain Security (EO 14028, SLSA) compliance checks.
package sbom

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkSBOM }
func (f *framework) Name() string               { return "SBOM & Supply Chain Security (EO 14028, SLSA)" }
func (f *framework) Version() string            { return "2021" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// EO 14028 §4(e) — SBOM generation
		&missingSBOMGenerationCheck{},
		&missingLockFileCheck{},

		// SLSA Level 2 — Provenance
		&unpinnedDependenciesCheck{},
		&missingProvenanceCheck{},
		&unsignedCommitsCheck{},
	}
}

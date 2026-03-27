// Package eucra implements EU Cyber Resilience Act (Regulation 2024/2847) compliance checks.
package eucra

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkEUCRA }
func (f *framework) Name() string               { return "EU Cyber Resilience Act (Regulation 2024/2847)" }
func (f *framework) Version() string            { return "2024/2847" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&insecureDefaultsCheck{},
		&unnecessaryAttackSurfaceCheck{},
		&missingDepScanningCheck{},
		&knownVulnerablePatternsCheck{},
		&missingSBOMCheck{},
		&missingUpdateMechanismCheck{},
	}
}

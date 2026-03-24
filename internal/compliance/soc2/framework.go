// Package soc2 implements SOC 2 Trust Service Criteria compliance checks.
package soc2

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkSOC2 }
func (f *framework) Name() string               { return "SOC 2 (Trust Service Criteria)" }
func (f *framework) Version() string             { return "2017" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		&missingAuthMiddlewareCheck{},
		&insecureTLSConfigCheck{},
		&swallowedErrorsCheck{},
		&missingSecurityLoggingCheck{},
		&todoInProductionCheck{},
		&debugModeEnabledCheck{},
	}
}

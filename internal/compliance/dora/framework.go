// Package dora implements DORA (Digital Operational Resilience Act) compliance checks.
package dora

import "github.com/SimplyLiz/CodeMCP/internal/compliance"

func init() {
	compliance.Register(NewFramework())
}

type framework struct{}

func NewFramework() compliance.Framework { return &framework{} }

func (f *framework) ID() compliance.FrameworkID { return compliance.FrameworkDORA }
func (f *framework) Name() string               { return "DORA (Digital Operational Resilience Act)" }
func (f *framework) Version() string            { return "2022/2554" }

func (f *framework) Checks() []compliance.Check {
	return []compliance.Check{
		// Art. 9 — ICT risk management: resilience
		&missingCircuitBreakerCheck{},
		&missingTimeoutCheck{},
		&missingRetryLogicCheck{},

		// Art. 10 — Detection
		&missingHealthEndpointCheck{},
		&missingCorrelationIDCheck{},

		// Art. 15 — ICT change management
		&missingRollbackCheck{},
	}
}

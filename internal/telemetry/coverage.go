package telemetry

// ComputeCoverage computes telemetry coverage from events and matches
func ComputeCoverage(events []CallAggregate, matches []SymbolMatch, federationRepoCount int) TelemetryCoverage {
	if len(events) == 0 {
		return TelemetryCoverage{
			Overall: OverallCoverage{
				Score: 0,
				Level: CoverageInsufficient,
				Warnings: []string{"No telemetry events received"},
			},
		}
	}

	total := float64(len(events))

	// Attribute coverage
	withFilePath := 0
	withNamespace := 0
	withLineNumber := 0
	for _, e := range events {
		if e.FilePath != "" {
			withFilePath++
		}
		if e.Namespace != "" {
			withNamespace++
		}
		if e.LineNumber > 0 {
			withLineNumber++
		}
	}

	attrFilePath := float64(withFilePath) / total
	attrNamespace := float64(withNamespace) / total
	attrLineNumber := float64(withLineNumber) / total
	attrOverall := (attrFilePath * 0.5) + (attrNamespace * 0.3) + (attrLineNumber * 0.2)

	// Match coverage
	exact := 0
	strong := 0
	weak := 0
	unmatched := 0
	for _, m := range matches {
		switch m.Quality {
		case MatchExact:
			exact++
		case MatchStrong:
			strong++
		case MatchWeak:
			weak++
		default:
			unmatched++
		}
	}

	matchTotal := float64(len(matches))
	matchExact := float64(exact) / matchTotal
	matchStrong := float64(strong) / matchTotal
	matchWeak := float64(weak) / matchTotal
	matchUnmatched := float64(unmatched) / matchTotal
	effectiveRate := matchExact + matchStrong

	// Service coverage
	servicesReporting := countUniqueServices(events)
	serviceCoverageRate := 0.0
	if federationRepoCount > 0 {
		serviceCoverageRate = min(float64(servicesReporting)/float64(federationRepoCount), 1.0)
	} else {
		serviceCoverageRate = 1.0 // No federation = assume full coverage
	}

	// Sampling detection (heuristic)
	samplingDetected, estimatedRate := detectSampling(events)

	// Overall score
	overallScore := (attrOverall * 0.3) + (effectiveRate * 0.5) + (serviceCoverageRate * 0.2)

	// Determine level
	var level CoverageLevel
	switch {
	case overallScore >= 0.8:
		level = CoverageHigh
	case overallScore >= 0.6:
		level = CoverageMedium
	case overallScore >= 0.4:
		level = CoverageLow
	default:
		level = CoverageInsufficient
	}

	// Build warnings
	var warnings []string
	if effectiveRate < 0.5 {
		warnings = append(warnings, "Low match rate — many events unmatched")
	}
	if serviceCoverageRate < 0.8 && federationRepoCount > 0 {
		warnings = append(warnings, "Not all services reporting telemetry")
	}
	if samplingDetected {
		warnings = append(warnings, "Sampling detected — call counts are estimates")
	}
	if attrFilePath < 0.5 {
		warnings = append(warnings, "Most events missing file_path — match quality limited")
	}

	return TelemetryCoverage{
		AttributeCoverage: AttributeCoverage{
			WithFilePath:   attrFilePath,
			WithNamespace:  attrNamespace,
			WithLineNumber: attrLineNumber,
			Overall:        attrOverall,
		},
		MatchCoverage: MatchCoverage{
			Exact:         matchExact,
			Strong:        matchStrong,
			Weak:          matchWeak,
			Unmatched:     matchUnmatched,
			EffectiveRate: effectiveRate,
		},
		ServiceCoverage: ServiceCoverage{
			ServicesReporting:    servicesReporting,
			ServicesInFederation: federationRepoCount,
			CoverageRate:         serviceCoverageRate,
		},
		Sampling: SamplingInfo{
			Detected:      samplingDetected,
			EstimatedRate: estimatedRate,
		},
		Overall: OverallCoverage{
			Score:    overallScore,
			Level:    level,
			Warnings: warnings,
		},
	}
}

// countUniqueServices counts unique service names in events
func countUniqueServices(events []CallAggregate) int {
	services := make(map[string]struct{})
	for _, e := range events {
		services[e.ServiceName] = struct{}{}
	}
	return len(services)
}

// detectSampling attempts to detect if events are sampled
func detectSampling(events []CallAggregate) (detected bool, estimatedRate float64) {
	// Heuristic: if many call counts are suspiciously round numbers
	// (multiples of 10, 100, etc.), sampling may be in effect
	roundCount := 0
	for _, e := range events {
		if e.CallCount > 0 && e.CallCount%10 == 0 {
			roundCount++
		}
	}

	if len(events) > 100 && float64(roundCount)/float64(len(events)) > 0.8 {
		// Most counts are round numbers - likely scaled up from sampling
		return true, 0.1 // Estimate 10% sampling (could be refined)
	}

	return false, 0
}

// CanUseDeadCode checks if coverage is sufficient for dead code detection
func (c TelemetryCoverage) CanUseDeadCode() bool {
	return c.Overall.Level == CoverageHigh || c.Overall.Level == CoverageMedium
}

// CanUseUsageDisplay checks if coverage is sufficient for usage display
func (c TelemetryCoverage) CanUseUsageDisplay() bool {
	return c.Overall.Level != CoverageInsufficient
}

// CanUseImpactEnrichment checks if coverage is sufficient for impact enrichment
func (c TelemetryCoverage) CanUseImpactEnrichment() bool {
	return (c.Overall.Level == CoverageHigh || c.Overall.Level == CoverageMedium) &&
		c.MatchCoverage.EffectiveRate >= 0.5
}

// CanUseHotspotWeighting checks if coverage is sufficient for hotspot weighting
func (c TelemetryCoverage) CanUseHotspotWeighting() bool {
	return c.Overall.Level != CoverageInsufficient &&
		c.MatchCoverage.EffectiveRate >= 0.4
}

// CoverageRequirement describes a coverage requirement for a feature
type CoverageRequirement struct {
	Feature           string
	MinCoverageLevel  CoverageLevel
	MinEffectiveRate  float64
	Description       string
}

// DefaultCoverageRequirements returns the default coverage requirements for features
func DefaultCoverageRequirements() []CoverageRequirement {
	return []CoverageRequirement{
		{
			Feature:          "dead_code_candidates",
			MinCoverageLevel: CoverageMedium,
			MinEffectiveRate: 0.6,
			Description:      "Dead code detection requires medium+ coverage and 60%+ match rate",
		},
		{
			Feature:          "usage_display",
			MinCoverageLevel: CoverageLow,
			MinEffectiveRate: 0.0,
			Description:      "Usage display works with any coverage but may be incomplete",
		},
		{
			Feature:          "impact_enrichment",
			MinCoverageLevel: CoverageMedium,
			MinEffectiveRate: 0.5,
			Description:      "Impact enrichment requires medium+ coverage and 50%+ match rate",
		},
		{
			Feature:          "hotspot_weighting",
			MinCoverageLevel: CoverageLow,
			MinEffectiveRate: 0.4,
			Description:      "Hotspot weighting requires low+ coverage and 40%+ match rate",
		},
	}
}

// CheckRequirement checks if coverage meets a specific requirement
func (c TelemetryCoverage) CheckRequirement(req CoverageRequirement) (met bool, reason string) {
	// Check coverage level
	levelMet := false
	switch req.MinCoverageLevel {
	case CoverageHigh:
		levelMet = c.Overall.Level == CoverageHigh
	case CoverageMedium:
		levelMet = c.Overall.Level == CoverageHigh || c.Overall.Level == CoverageMedium
	case CoverageLow:
		levelMet = c.Overall.Level != CoverageInsufficient
	case CoverageInsufficient:
		levelMet = true
	}

	if !levelMet {
		return false, "Coverage level " + string(c.Overall.Level) + " is below required " + string(req.MinCoverageLevel)
	}

	// Check effective rate
	if c.MatchCoverage.EffectiveRate < req.MinEffectiveRate {
		return false, "Effective match rate insufficient for " + req.Feature
	}

	return true, ""
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

package telemetry

import (
	"path/filepath"
	"strings"

	"ckb/internal/config"
)

// DeadCodeOptions configures dead code candidate detection
type DeadCodeOptions struct {
	MinConfidence      float64
	MinObservationDays int
	ExcludePatterns    []string
	ExcludeFunctions   []string
	Limit              int
}

// DefaultDeadCodeOptions returns the default dead code detection options
func DefaultDeadCodeOptions(cfg config.TelemetryDeadCodeConfig) DeadCodeOptions {
	return DeadCodeOptions{
		MinConfidence:      0.7,
		MinObservationDays: cfg.MinObservationDays,
		ExcludePatterns:    cfg.ExcludePatterns,
		ExcludeFunctions:   cfg.ExcludeFunctions,
		Limit:              100,
	}
}

// DeadCodeDetector finds dead code candidates based on telemetry data
type DeadCodeDetector struct {
	storage  *Storage
	coverage TelemetryCoverage
	options  DeadCodeOptions
}

// NewDeadCodeDetector creates a new dead code detector
func NewDeadCodeDetector(storage *Storage, coverage TelemetryCoverage, options DeadCodeOptions) *DeadCodeDetector {
	return &DeadCodeDetector{
		storage:  storage,
		coverage: coverage,
		options:  options,
	}
}

// SymbolInfo provides symbol metadata for dead code analysis
type SymbolInfo struct {
	ID         string
	Name       string
	File       string
	Kind       string
	StaticRefs int // Number of compile-time references
}

// FindCandidates finds dead code candidates from the given symbols
func (d *DeadCodeDetector) FindCandidates(symbols []SymbolInfo) []DeadCodeCandidate {
	// Gate: coverage must be sufficient
	if !d.coverage.CanUseDeadCode() {
		return nil // Can't make claims with low coverage
	}

	// Get observation window
	observationDays, _ := d.storage.GetObservationWindowDays()
	if observationDays < d.options.MinObservationDays {
		return nil // Observation window too short
	}

	var candidates []DeadCodeCandidate

	for _, symbol := range symbols {
		// Skip if excluded
		if excluded, _ := d.isExcluded(symbol); excluded {
			continue
		}

		// Get observed usage
		usages, err := d.storage.GetObservedUsage(symbol.ID, "")
		if err != nil {
			continue
		}

		// Calculate total calls
		var totalCalls int64
		var matchQuality MatchQuality = MatchStrong // Default to strong if no usage record
		for _, u := range usages {
			totalCalls += u.CallCount
			matchQuality = u.MatchQuality
		}

		// Skip if match quality is weak or unmatched
		if matchQuality == MatchWeak || matchQuality == MatchUnmatched {
			continue
		}

		// Check for zero calls
		if totalCalls == 0 {
			confidence := d.computeConfidence(matchQuality, symbol.StaticRefs, observationDays)

			if confidence < d.options.MinConfidence {
				continue
			}

			candidates = append(candidates, DeadCodeCandidate{
				SymbolID:          symbol.ID,
				Name:              symbol.Name,
				File:              symbol.File,
				StaticRefs:        symbol.StaticRefs,
				ObservedCalls:     0,
				LastObserved:      nil,
				ObservationWindow: observationDays,
				Confidence:        confidence,
				ConfidenceBasis:   d.deriveConfidenceBasis(matchQuality, observationDays, symbol.StaticRefs),
				MatchQuality:      matchQuality,
				CoverageLevel:     d.coverage.Overall.Level,
				CoverageWarnings:  d.coverage.Overall.Warnings,
				Excluded:          false,
			})
		}
	}

	// Sort by confidence (highest first) and limit
	sortByConfidence(candidates)
	if len(candidates) > d.options.Limit {
		candidates = candidates[:d.options.Limit]
	}

	return candidates
}

// isExcluded checks if a symbol should be excluded from dead code analysis
func (d *DeadCodeDetector) isExcluded(symbol SymbolInfo) (bool, string) {
	// Check path patterns
	for _, pattern := range d.options.ExcludePatterns {
		matched, err := filepath.Match(pattern, symbol.File)
		if err == nil && matched {
			return true, "path_pattern:" + pattern
		}
		// Also try with ** patterns (glob style)
		if matchGlobPattern(pattern, symbol.File) {
			return true, "path_pattern:" + pattern
		}
	}

	// Check function name patterns
	for _, pattern := range d.options.ExcludeFunctions {
		if matchFunctionPattern(pattern, symbol.Name) {
			return true, "function_pattern:" + pattern
		}
	}

	return false, ""
}

// matchGlobPattern matches glob patterns with ** support
func matchGlobPattern(pattern, path string) bool {
	// Handle ** patterns
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]

			// Remove leading/trailing slashes from suffix
			suffix = strings.TrimPrefix(suffix, "/")

			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}
			if suffix != "" && !strings.HasSuffix(path, suffix) && !strings.Contains(path, suffix) {
				return false
			}
			return true
		}
	}

	// Fall back to filepath.Match
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// matchFunctionPattern matches function name patterns with * wildcards
func matchFunctionPattern(pattern, name string) bool {
	// Handle simple wildcard patterns like "*Migration*"
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		middle := pattern[1 : len(pattern)-1]
		return strings.Contains(name, middle)
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := pattern[1:]
		return strings.HasSuffix(name, suffix)
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(name, prefix)
	}
	return name == pattern
}

// computeConfidence calculates dead code confidence score
func (d *DeadCodeDetector) computeConfidence(matchQuality MatchQuality, staticRefs int, observationDays int) float64 {
	// Base confidence from match quality
	var confidence float64
	if matchQuality == MatchExact {
		confidence = 0.90
	} else {
		confidence = 0.80
	}

	// Adjust for coverage
	switch d.coverage.Overall.Level {
	case CoverageHigh:
		confidence *= 1.0
	case CoverageMedium:
		confidence *= 0.9
	}

	// Adjust for static refs (more refs = less confident it's dead)
	if staticRefs > 10 {
		confidence *= 0.8
	} else if staticRefs > 5 {
		confidence *= 0.9
	}

	// Adjust for observation window
	if observationDays >= 180 {
		confidence *= 1.0
	} else if observationDays >= 90 {
		confidence *= 0.95
	} else {
		confidence *= 0.85
	}

	// Adjust for sampling
	if d.coverage.Sampling.Detected {
		confidence *= 0.7
	}

	// Never claim > 90% confidence
	if confidence > 0.90 {
		confidence = 0.90
	}

	return confidence
}

// deriveConfidenceBasis explains what factors contributed to confidence
func (d *DeadCodeDetector) deriveConfidenceBasis(matchQuality MatchQuality, observationDays int, staticRefs int) []string {
	var basis []string

	if matchQuality == MatchExact {
		basis = append(basis, "exact_location_match")
	} else {
		basis = append(basis, "strong_file_match")
	}

	if d.coverage.Overall.Level == CoverageHigh {
		basis = append(basis, "high_coverage")
	} else {
		basis = append(basis, "medium_coverage")
	}

	if observationDays >= 180 {
		basis = append(basis, "long_observation_window")
	} else if observationDays >= 90 {
		basis = append(basis, "adequate_observation_window")
	}

	if staticRefs == 0 {
		basis = append(basis, "no_static_refs")
	} else if staticRefs <= 5 {
		basis = append(basis, "few_static_refs")
	} else {
		basis = append(basis, "has_static_refs_reduces_confidence")
	}

	if d.coverage.Sampling.Detected {
		basis = append(basis, "sampling_detected_reduces_confidence")
	}

	return basis
}

// sortByConfidence sorts candidates by confidence descending
func sortByConfidence(candidates []DeadCodeCandidate) {
	// Simple bubble sort for small lists
	n := len(candidates)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if candidates[j].Confidence < candidates[j+1].Confidence {
				candidates[j], candidates[j+1] = candidates[j+1], candidates[j]
			}
		}
	}
}

// BuildSummary creates a summary of dead code candidates
func BuildSummary(candidates []DeadCodeCandidate, totalSymbols int) DeadCodeSummary {
	summary := DeadCodeSummary{
		TotalSymbols:    totalSymbols,
		TotalCandidates: len(candidates),
	}

	if len(candidates) == 0 {
		return summary
	}

	var totalConfidence float64
	for _, c := range candidates {
		totalConfidence += c.Confidence
		if c.Confidence >= 0.8 {
			summary.ByConfidenceLevel.High++
		} else if c.Confidence >= 0.6 {
			summary.ByConfidenceLevel.Medium++
		} else {
			summary.ByConfidenceLevel.Low++
		}
	}
	summary.AvgConfidence = totalConfidence / float64(len(candidates))

	return summary
}

package mcp

import (
	"fmt"
	"time"

	"ckb/internal/telemetry"
)

// v6.4 Telemetry tool implementations

// toolGetTelemetryStatus returns telemetry system status
func (s *MCPServer) toolGetTelemetryStatus(params map[string]interface{}) (interface{}, error) {
	cfg := s.engine.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("configuration not available")
	}

	status := telemetry.TelemetryStatus{
		Enabled: cfg.Telemetry.Enabled,
	}

	if !cfg.Telemetry.Enabled {
		status.Recommendations = []string{
			"Telemetry is disabled. Enable with: ckb config set telemetry.enabled true",
			"Configure service map to map service names to repository IDs",
		}
		return status, nil
	}

	// Get telemetry storage
	storage := s.getTelemetryStorage()
	if storage == nil {
		return status, nil
	}

	// Get last sync
	lastSync, err := storage.GetLastSyncLog()
	if err == nil && lastSync != nil {
		status.LastSync = &lastSync.StartedAt
	}

	// Get events in last 24h
	status.EventsLast24h, _ = storage.GetEventsLast24h()

	// Get active sources
	status.SourcesActive, _ = storage.GetActiveSourcesLast24h()

	// Get match stats to build coverage
	exact, strong, weak, unmatched, _ := storage.GetMatchStats()
	total := exact + strong + weak + unmatched
	if total > 0 {
		status.Coverage = telemetry.TelemetryCoverage{
			MatchCoverage: telemetry.MatchCoverage{
				Exact:         float64(exact) / float64(total),
				Strong:        float64(strong) / float64(total),
				Weak:          float64(weak) / float64(total),
				Unmatched:     float64(unmatched) / float64(total),
				EffectiveRate: float64(exact+strong) / float64(total),
			},
		}

		// Determine coverage level
		effectiveRate := status.Coverage.MatchCoverage.EffectiveRate
		if effectiveRate >= 0.8 {
			status.Coverage.Overall.Level = telemetry.CoverageHigh
		} else if effectiveRate >= 0.6 {
			status.Coverage.Overall.Level = telemetry.CoverageMedium
		} else if effectiveRate >= 0.4 {
			status.Coverage.Overall.Level = telemetry.CoverageLow
		} else {
			status.Coverage.Overall.Level = telemetry.CoverageInsufficient
		}
		status.Coverage.Overall.Score = effectiveRate
	}

	// Get unmapped services
	status.UnmappedServices, _ = storage.GetUnmappedServices(10)
	status.ServiceMapUnmapped = len(status.UnmappedServices)
	status.ServiceMapMapped = len(cfg.Telemetry.ServiceMap)

	// Build recommendations
	if status.Coverage.Overall.Level == telemetry.CoverageInsufficient {
		status.Recommendations = append(status.Recommendations,
			"Coverage is insufficient for dead code detection. Ensure telemetry events include file_path attribute.")
	}
	if status.ServiceMapUnmapped > 0 {
		status.Recommendations = append(status.Recommendations,
			fmt.Sprintf("%d services are unmapped. Add them to telemetry.serviceMap in config.", status.ServiceMapUnmapped))
	}
	if status.EventsLast24h == 0 {
		status.Recommendations = append(status.Recommendations,
			"No telemetry events received in last 24h. Check OTEL collector configuration.")
	}

	return status, nil
}

// toolGetObservedUsage returns observed usage data for a symbol
func (s *MCPServer) toolGetObservedUsage(params map[string]interface{}) (interface{}, error) {
	symbolID, _ := params["symbolId"].(string)
	if symbolID == "" {
		return nil, fmt.Errorf("symbolId is required")
	}

	period, _ := params["period"].(string)
	if period == "" {
		period = "90d"
	}

	includeCallers, _ := params["includeCallers"].(bool)

	cfg := s.engine.GetConfig()
	if cfg == nil || !cfg.Telemetry.Enabled {
		return nil, fmt.Errorf("telemetry is not enabled")
	}

	storage := s.getTelemetryStorage()
	if storage == nil {
		return nil, fmt.Errorf("telemetry storage not available")
	}

	// Convert period to filter
	periodFilter := computePeriodFilter(period)

	// Get usage data
	usages, err := storage.GetObservedUsage(symbolID, periodFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get observed usage: %w", err)
	}

	response := telemetry.ObservedUsageResponse{
		SymbolID:   symbolID,
		SymbolName: extractSymbolName(symbolID),
	}

	if len(usages) > 0 {
		var totalCalls int64
		var periodCalls int64
		var firstObserved, lastObserved time.Time

		for i, u := range usages {
			totalCalls += u.CallCount
			periodCalls += u.CallCount
			if i == 0 {
				lastObserved = u.IngestedAt
			}
			firstObserved = u.IngestedAt
		}

		response.Usage = &telemetry.UsageData{
			TotalCalls:    totalCalls,
			PeriodCalls:   periodCalls,
			FirstObserved: firstObserved,
			LastObserved:  lastObserved,
			MatchQuality:  usages[0].MatchQuality,
			Trend:         computeTrend(usages),
		}
	}

	// Get callers if requested
	if includeCallers && cfg.Telemetry.Aggregation.StoreCallers {
		callers, err := storage.GetObservedCallers(symbolID, 10)
		if err == nil {
			for _, c := range callers {
				response.Callers = append(response.Callers, telemetry.CallerBreakdown{
					Service:   c.CallerService,
					CallCount: c.CallCount,
				})
			}
		}
	}

	// Get static refs for blended confidence
	refs, _ := s.engine.GetReferenceCount(symbolID)
	response.StaticRefs = refs

	// Compute blended confidence
	if response.Usage != nil {
		response.BlendedConfidence = computeBlendedConfidence(
			0.79, // static max
			response.Usage.MatchQuality.Confidence(),
		)
	} else {
		response.BlendedConfidence = 0.79 // static only
	}

	return response, nil
}

// toolFindDeadCodeCandidates finds potential dead code based on telemetry
func (s *MCPServer) toolFindDeadCodeCandidates(params map[string]interface{}) (interface{}, error) {
	minConfidence := 0.7
	if v, ok := params["minConfidence"].(float64); ok {
		minConfidence = v
	}

	limit := 100
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	cfg := s.engine.GetConfig()
	if cfg == nil || !cfg.Telemetry.Enabled {
		return nil, fmt.Errorf("telemetry is not enabled")
	}

	storage := s.getTelemetryStorage()
	if storage == nil {
		return nil, fmt.Errorf("telemetry storage not available")
	}

	// Get match stats for coverage
	exact, strong, weak, unmatched, _ := storage.GetMatchStats()
	total := exact + strong + weak + unmatched
	if total == 0 {
		return telemetry.DeadCodeResponse{
			Limitations: []telemetry.Limitation{
				{
					Type:        "no_data",
					Description: "No telemetry data available",
					Impact:      "Cannot detect dead code without runtime telemetry",
				},
			},
		}, nil
	}

	// Build coverage
	effectiveRate := float64(exact+strong) / float64(total)
	var coverageLevel telemetry.CoverageLevel
	if effectiveRate >= 0.8 {
		coverageLevel = telemetry.CoverageHigh
	} else if effectiveRate >= 0.6 {
		coverageLevel = telemetry.CoverageMedium
	} else if effectiveRate >= 0.4 {
		coverageLevel = telemetry.CoverageLow
	} else {
		coverageLevel = telemetry.CoverageInsufficient
	}

	coverage := telemetry.TelemetryCoverage{
		MatchCoverage: telemetry.MatchCoverage{
			Exact:         float64(exact) / float64(total),
			Strong:        float64(strong) / float64(total),
			Weak:          float64(weak) / float64(total),
			Unmatched:     float64(unmatched) / float64(total),
			EffectiveRate: effectiveRate,
		},
		Overall: telemetry.OverallCoverage{
			Score: effectiveRate,
			Level: coverageLevel,
		},
	}

	// Check coverage requirement
	if !coverage.CanUseDeadCode() {
		return telemetry.DeadCodeResponse{
			Coverage: coverage,
			Limitations: []telemetry.Limitation{
				{
					Type:        "insufficient_coverage",
					Description: fmt.Sprintf("Coverage level %s is below required medium", coverageLevel),
					Impact:      "Cannot reliably detect dead code",
				},
			},
		}, nil
	}

	// Get symbols with zero calls
	observationDays, _ := storage.GetObservationWindowDays()
	if observationDays < cfg.Telemetry.DeadCode.MinObservationDays {
		return telemetry.DeadCodeResponse{
			Coverage: coverage,
			Limitations: []telemetry.Limitation{
				{
					Type:        "short_observation_window",
					Description: fmt.Sprintf("Only %d days of data (need %d)", observationDays, cfg.Telemetry.DeadCode.MinObservationDays),
					Impact:      "Wait for more telemetry data to accumulate",
				},
			},
		}, nil
	}

	// Create detector with options
	options := telemetry.DefaultDeadCodeOptions(cfg.Telemetry.DeadCode)
	options.MinConfidence = minConfidence
	options.Limit = limit

	detector := telemetry.NewDeadCodeDetector(storage, coverage, options)

	// Get symbols from engine and check for dead code
	symbols, err := s.engine.GetAllSymbols()
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	// Convert to SymbolInfo slice
	var symbolInfos []telemetry.SymbolInfo
	for _, sym := range symbols {
		refs, _ := s.engine.GetReferenceCount(sym.ID)
		symbolInfos = append(symbolInfos, telemetry.SymbolInfo{
			ID:         sym.ID,
			Name:       sym.Name,
			File:       sym.File,
			Kind:       sym.Kind,
			StaticRefs: refs,
		})
	}

	candidates := detector.FindCandidates(symbolInfos)

	response := telemetry.DeadCodeResponse{
		Candidates: candidates,
		Summary:    telemetry.BuildSummary(candidates, len(symbols)),
		Coverage:   coverage,
	}

	if len(candidates) == 0 {
		response.Limitations = append(response.Limitations, telemetry.Limitation{
			Type:        "no_candidates",
			Description: "No dead code candidates found above confidence threshold",
		})
	}

	return response, nil
}

// getTelemetryStorage returns the telemetry storage instance
func (s *MCPServer) getTelemetryStorage() *telemetry.Storage {
	db := s.engine.GetDB()
	if db == nil {
		return nil
	}
	return telemetry.NewStorage(db.Conn())
}

// Helper functions

func computePeriodFilter(period string) string {
	now := time.Now()
	switch period {
	case "7d":
		return now.AddDate(0, 0, -7).Format("2006-01-02")
	case "30d":
		return now.AddDate(0, 0, -30).Format("2006-01-02")
	case "90d":
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	case "all":
		return ""
	default:
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	}
}

func extractSymbolName(symbolID string) string {
	// Extract name from symbol ID like "ckb:repo:sym:fingerprint:Name"
	// Simplified - in practice would parse the symbol format
	return symbolID
}

func computeTrend(usages []telemetry.ObservedUsage) telemetry.UsageTrend {
	if len(usages) < 2 {
		return telemetry.TrendStable
	}

	// Compare first half vs second half
	mid := len(usages) / 2
	var recentCalls, olderCalls int64
	for i, u := range usages {
		if i < mid {
			recentCalls += u.CallCount
		} else {
			olderCalls += u.CallCount
		}
	}

	if olderCalls == 0 {
		if recentCalls > 0 {
			return telemetry.TrendIncreasing
		}
		return telemetry.TrendStable
	}

	ratio := float64(recentCalls) / float64(olderCalls)
	if ratio > 1.2 {
		return telemetry.TrendIncreasing
	} else if ratio < 0.8 {
		return telemetry.TrendDecreasing
	}
	return telemetry.TrendStable
}

func computeBlendedConfidence(staticConfidence, observedConfidence float64) float64 {
	// Take higher, with small boost if both agree
	base := staticConfidence
	if observedConfidence > base {
		base = observedConfidence
	}

	agreementBoost := 0.0
	if staticConfidence > 0.5 && observedConfidence > 0.5 {
		agreementBoost = 0.03
	}

	result := base + agreementBoost
	if result > 1.0 {
		result = 1.0
	}
	return result
}

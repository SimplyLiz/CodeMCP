package telemetry

import (
	"testing"
	"time"
)

func TestMatchQuality_Confidence(t *testing.T) {
	tests := []struct {
		quality    MatchQuality
		confidence float64
	}{
		{MatchExact, 0.95},
		{MatchStrong, 0.85},
		{MatchWeak, 0.60},
		{MatchUnmatched, 0.0},
		{MatchQuality("unknown"), 0.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.quality), func(t *testing.T) {
			result := tt.quality.Confidence()
			if result != tt.confidence {
				t.Errorf("Confidence() = %f, want %f", result, tt.confidence)
			}
		})
	}
}

func TestMatchQualityConstants(t *testing.T) {
	if MatchExact != "exact" {
		t.Errorf("MatchExact = %q, want %q", MatchExact, "exact")
	}
	if MatchStrong != "strong" {
		t.Errorf("MatchStrong = %q, want %q", MatchStrong, "strong")
	}
	if MatchWeak != "weak" {
		t.Errorf("MatchWeak = %q, want %q", MatchWeak, "weak")
	}
	if MatchUnmatched != "unmatched" {
		t.Errorf("MatchUnmatched = %q, want %q", MatchUnmatched, "unmatched")
	}
}

func TestCoverageLevelConstants(t *testing.T) {
	if CoverageHigh != "high" {
		t.Errorf("CoverageHigh = %q, want %q", CoverageHigh, "high")
	}
	if CoverageMedium != "medium" {
		t.Errorf("CoverageMedium = %q, want %q", CoverageMedium, "medium")
	}
	if CoverageLow != "low" {
		t.Errorf("CoverageLow = %q, want %q", CoverageLow, "low")
	}
	if CoverageInsufficient != "insufficient" {
		t.Errorf("CoverageInsufficient = %q, want %q", CoverageInsufficient, "insufficient")
	}
}

func TestUsageTrendConstants(t *testing.T) {
	if TrendIncreasing != "increasing" {
		t.Errorf("TrendIncreasing = %q, want %q", TrendIncreasing, "increasing")
	}
	if TrendStable != "stable" {
		t.Errorf("TrendStable = %q, want %q", TrendStable, "stable")
	}
	if TrendDecreasing != "decreasing" {
		t.Errorf("TrendDecreasing = %q, want %q", TrendDecreasing, "decreasing")
	}
}

func TestCallAggregateStruct(t *testing.T) {
	now := time.Now()
	call := CallAggregate{
		ServiceName:    "user-service",
		ServiceVersion: "abc123",
		FunctionName:   "GetUser",
		Namespace:      "pkg.users",
		FilePath:       "internal/users/service.go",
		LineNumber:     42,
		CallCount:      1000,
		ErrorCount:     10,
		PeriodStart:    now.Add(-24 * time.Hour),
		PeriodEnd:      now,
		Callers: []Caller{
			{ServiceName: "api-gateway", CallCount: 500},
			{ServiceName: "admin-service", CallCount: 500},
		},
	}

	if call.ServiceName != "user-service" {
		t.Errorf("ServiceName = %q, want %q", call.ServiceName, "user-service")
	}
	if call.CallCount != 1000 {
		t.Errorf("CallCount = %d, want %d", call.CallCount, 1000)
	}
	if len(call.Callers) != 2 {
		t.Errorf("Callers length = %d, want %d", len(call.Callers), 2)
	}
}

func TestSymbolMatchStruct(t *testing.T) {
	match := SymbolMatch{
		SymbolID:   "ckb:repo:sym:abc123",
		Quality:    MatchExact,
		Confidence: 0.95,
		MatchBasis: []string{"file_path", "function_name", "line_number"},
	}

	if match.Quality != MatchExact {
		t.Errorf("Quality = %v, want %v", match.Quality, MatchExact)
	}
	if match.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want %f", match.Confidence, 0.95)
	}
	if len(match.MatchBasis) != 3 {
		t.Errorf("MatchBasis length = %d, want %d", len(match.MatchBasis), 3)
	}
}

func TestObservedUsageStruct(t *testing.T) {
	now := time.Now()
	usage := ObservedUsage{
		SymbolID:        "ckb:repo:sym:abc123",
		MatchQuality:    MatchStrong,
		MatchConfidence: 0.85,
		Period:          "2024-12",
		PeriodType:      "monthly",
		CallCount:       5000,
		ErrorCount:      50,
		ServiceVersion:  "v1.2.3",
		Source:          "otel-collector",
		IngestedAt:      now,
	}

	if usage.MatchQuality != MatchStrong {
		t.Errorf("MatchQuality = %v, want %v", usage.MatchQuality, MatchStrong)
	}
	if usage.CallCount != 5000 {
		t.Errorf("CallCount = %d, want %d", usage.CallCount, 5000)
	}
}

func TestSyncLogStruct(t *testing.T) {
	now := time.Now()
	completed := now.Add(5 * time.Second)

	log := SyncLog{
		ID:                  1,
		Source:              "otel-collector",
		StartedAt:           now,
		CompletedAt:         &completed,
		Status:              "success",
		EventsReceived:      1000,
		EventsMatchedExact:  800,
		EventsMatchedStrong: 150,
		EventsMatchedWeak:   30,
		EventsUnmatched:     20,
		ServiceVersions:     map[string]string{"user-service": "abc123"},
		CoverageScore:       0.85,
		CoverageLevel:       "high",
	}

	if log.EventsReceived != 1000 {
		t.Errorf("EventsReceived = %d, want %d", log.EventsReceived, 1000)
	}
	if log.CoverageScore != 0.85 {
		t.Errorf("CoverageScore = %f, want %f", log.CoverageScore, 0.85)
	}
}

func TestDeadCodeCandidateStruct(t *testing.T) {
	lastObserved := time.Now().Add(-30 * 24 * time.Hour)

	candidate := DeadCodeCandidate{
		SymbolID:          "ckb:repo:sym:abc123",
		Name:              "OldFunction",
		File:              "internal/legacy/old.go",
		StaticRefs:        2,
		ObservedCalls:     0,
		LastObserved:      &lastObserved,
		ObservationWindow: 90,
		Confidence:        0.92,
		ConfidenceBasis:   []string{"no_runtime_calls", "low_static_refs"},
		MatchQuality:      MatchExact,
		CoverageLevel:     CoverageHigh,
		Excluded:          false,
	}

	if candidate.ObservedCalls != 0 {
		t.Errorf("ObservedCalls = %d, want %d", candidate.ObservedCalls, 0)
	}
	if candidate.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want %f", candidate.Confidence, 0.92)
	}
}

func TestTelemetryCoverageStruct(t *testing.T) {
	coverage := TelemetryCoverage{
		AttributeCoverage: AttributeCoverage{
			WithFilePath:   0.90,
			WithNamespace:  0.85,
			WithLineNumber: 0.70,
			Overall:        0.82,
		},
		MatchCoverage: MatchCoverage{
			Exact:         0.70,
			Strong:        0.20,
			Weak:          0.05,
			Unmatched:     0.05,
			EffectiveRate: 0.90,
		},
		ServiceCoverage: ServiceCoverage{
			ServicesReporting:    8,
			ServicesInFederation: 10,
			CoverageRate:         0.80,
		},
		Sampling: SamplingInfo{
			Detected:      true,
			EstimatedRate: 0.10,
			Warning:       "Sampling detected, actual call counts may be higher",
		},
		Overall: OverallCoverage{
			Score:    0.85,
			Level:    CoverageHigh,
			Warnings: nil,
		},
	}

	if coverage.Overall.Score != 0.85 {
		t.Errorf("Overall.Score = %f, want %f", coverage.Overall.Score, 0.85)
	}
	if coverage.MatchCoverage.EffectiveRate != 0.90 {
		t.Errorf("MatchCoverage.EffectiveRate = %f, want %f", coverage.MatchCoverage.EffectiveRate, 0.90)
	}
}

func TestBlendedConfidenceStruct(t *testing.T) {
	blended := BlendedConfidence{
		Confidence:         0.88,
		Basis:              []string{"static", "observed"},
		StaticConfidence:   0.80,
		ObservedConfidence: 0.95,
		ObservedBoost:      0.08,
	}

	if blended.Confidence != 0.88 {
		t.Errorf("Confidence = %f, want %f", blended.Confidence, 0.88)
	}
	if blended.ObservedBoost != 0.08 {
		t.Errorf("ObservedBoost = %f, want %f", blended.ObservedBoost, 0.08)
	}
}

func TestIngestPayloadStruct(t *testing.T) {
	now := time.Now()
	payload := IngestPayload{
		Source:         "otel-collector",
		ServiceVersion: "v1.0.0",
		Timestamp:      now,
		Calls: []CallAggregate{
			{ServiceName: "svc", FunctionName: "Func", CallCount: 100},
		},
	}

	if payload.Source != "otel-collector" {
		t.Errorf("Source = %q, want %q", payload.Source, "otel-collector")
	}
	if len(payload.Calls) != 1 {
		t.Errorf("Calls length = %d, want %d", len(payload.Calls), 1)
	}
}

func TestIngestResponseStruct(t *testing.T) {
	resp := IngestResponse{
		Accepted:      100,
		Matched:       85,
		Unmatched:     15,
		Errors:        nil,
		CoverageScore: 0.85,
		CoverageLevel: "high",
	}

	if resp.Accepted != 100 {
		t.Errorf("Accepted = %d, want %d", resp.Accepted, 100)
	}
	if resp.Matched != 85 {
		t.Errorf("Matched = %d, want %d", resp.Matched, 85)
	}
}

func TestTelemetryStatusStruct(t *testing.T) {
	now := time.Now()
	status := TelemetryStatus{
		Enabled:            true,
		LastSync:           &now,
		EventsLast24h:      10000,
		SourcesActive:      []string{"otel-collector-1", "otel-collector-2"},
		ServiceMapMapped:   8,
		ServiceMapUnmapped: 2,
		UnmappedServices:   []string{"legacy-service", "unknown-service"},
		Recommendations:    []string{"Add service mapping for legacy-service"},
	}

	if !status.Enabled {
		t.Error("Enabled should be true")
	}
	if status.EventsLast24h != 10000 {
		t.Errorf("EventsLast24h = %d, want %d", status.EventsLast24h, 10000)
	}
	if len(status.SourcesActive) != 2 {
		t.Errorf("SourcesActive length = %d, want %d", len(status.SourcesActive), 2)
	}
}

func TestLimitationStruct(t *testing.T) {
	limitation := Limitation{
		Type:        "sampling",
		Description: "10% sampling rate detected",
		Impact:      "Actual call counts may be 10x higher",
	}

	if limitation.Type != "sampling" {
		t.Errorf("Type = %q, want %q", limitation.Type, "sampling")
	}
}

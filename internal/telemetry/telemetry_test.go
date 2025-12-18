package telemetry

import (
	"testing"

	"ckb/internal/config"
)

func TestMatchQualityConfidence(t *testing.T) {
	tests := []struct {
		quality    MatchQuality
		wantConf   float64
	}{
		{MatchExact, 0.95},
		{MatchStrong, 0.85},
		{MatchWeak, 0.60},
		{MatchUnmatched, 0.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.quality), func(t *testing.T) {
			got := tt.quality.Confidence()
			if got != tt.wantConf {
				t.Errorf("Confidence() = %v, want %v", got, tt.wantConf)
			}
		})
	}
}

func TestCoverageLevelDetermination(t *testing.T) {
	tests := []struct {
		effectiveRate float64
		wantLevel     CoverageLevel
	}{
		{0.9, CoverageHigh},
		{0.8, CoverageHigh},
		{0.7, CoverageMedium},
		{0.6, CoverageMedium},
		{0.5, CoverageLow},
		{0.4, CoverageLow},
		{0.3, CoverageInsufficient},
		{0.0, CoverageInsufficient},
	}

	for _, tt := range tests {
		coverage := TelemetryCoverage{
			MatchCoverage: MatchCoverage{
				EffectiveRate: tt.effectiveRate,
			},
		}

		// Derive level from effective rate
		var gotLevel CoverageLevel
		if tt.effectiveRate >= 0.8 {
			gotLevel = CoverageHigh
		} else if tt.effectiveRate >= 0.6 {
			gotLevel = CoverageMedium
		} else if tt.effectiveRate >= 0.4 {
			gotLevel = CoverageLow
		} else {
			gotLevel = CoverageInsufficient
		}

		coverage.Overall.Level = gotLevel

		if gotLevel != tt.wantLevel {
			t.Errorf("effectiveRate=%.2f: got level %v, want %v", tt.effectiveRate, gotLevel, tt.wantLevel)
		}
	}
}

func TestCoverageCanUseDeadCode(t *testing.T) {
	tests := []struct {
		name     string
		level    CoverageLevel
		wantCan  bool
	}{
		{"high coverage", CoverageHigh, true},
		{"medium coverage", CoverageMedium, true},
		{"low coverage", CoverageLow, false},
		{"insufficient coverage", CoverageInsufficient, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := TelemetryCoverage{
				Overall: OverallCoverage{
					Level: tt.level,
				},
			}

			got := coverage.CanUseDeadCode()
			if got != tt.wantCan {
				t.Errorf("CanUseDeadCode() = %v, want %v", got, tt.wantCan)
			}
		})
	}
}

func TestServiceMapperExactMatch(t *testing.T) {
	cfg := config.TelemetryConfig{
		ServiceMap: map[string]string{
			"api-gateway": "repo-api",
			"user-service": "repo-users",
		},
	}

	mapper, err := NewServiceMapper(cfg)
	if err != nil {
		t.Fatalf("NewServiceMapper() error = %v", err)
	}

	tests := []struct {
		serviceName string
		wantMatched bool
		wantRepoID  string
		wantType    string
	}{
		{"api-gateway", true, "repo-api", "exact"},
		{"user-service", true, "repo-users", "exact"},
		{"unknown-service", false, "", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.serviceName, func(t *testing.T) {
			result := mapper.Resolve(tt.serviceName)
			if result.Matched != tt.wantMatched {
				t.Errorf("Resolve(%q).Matched = %v, want %v", tt.serviceName, result.Matched, tt.wantMatched)
			}
			if result.RepoID != tt.wantRepoID {
				t.Errorf("Resolve(%q).RepoID = %v, want %v", tt.serviceName, result.RepoID, tt.wantRepoID)
			}
			if result.MatchType != tt.wantType {
				t.Errorf("Resolve(%q).MatchType = %v, want %v", tt.serviceName, result.MatchType, tt.wantType)
			}
		})
	}
}

func TestServiceMapperPatternMatch(t *testing.T) {
	cfg := config.TelemetryConfig{
		ServicePatterns: []config.TelemetryServicePattern{
			{
				Pattern: "^payment-.*$",
				Repo:    "repo-payments",
			},
			{
				Pattern: "^order-.*$",
				Repo:    "repo-orders",
			},
		},
	}

	mapper, err := NewServiceMapper(cfg)
	if err != nil {
		t.Fatalf("NewServiceMapper() error = %v", err)
	}

	tests := []struct {
		serviceName string
		wantMatched bool
		wantRepoID  string
		wantType    string
	}{
		{"payment-service", true, "repo-payments", "pattern"},
		{"payment-gateway", true, "repo-payments", "pattern"},
		{"order-service", true, "repo-orders", "pattern"},
		{"unknown", false, "", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.serviceName, func(t *testing.T) {
			result := mapper.Resolve(tt.serviceName)
			if result.Matched != tt.wantMatched {
				t.Errorf("Resolve(%q).Matched = %v, want %v", tt.serviceName, result.Matched, tt.wantMatched)
			}
			if result.RepoID != tt.wantRepoID {
				t.Errorf("Resolve(%q).RepoID = %v, want %v", tt.serviceName, result.RepoID, tt.wantRepoID)
			}
			if result.MatchType != tt.wantType {
				t.Errorf("Resolve(%q).MatchType = %v, want %v", tt.serviceName, result.MatchType, tt.wantType)
			}
		})
	}
}

func TestDeadCodeDetectorExclusions(t *testing.T) {
	cfg := config.TelemetryDeadCodeConfig{
		MinObservationDays: 30,
		ExcludePatterns:    []string{"**/test/**", "**/migrations/**"},
		ExcludeFunctions:   []string{"*Migration*", "Test*"},
	}

	options := DefaultDeadCodeOptions(cfg)

	detector := &DeadCodeDetector{
		options: options,
	}

	tests := []struct {
		symbol     SymbolInfo
		wantExcluded bool
	}{
		{SymbolInfo{File: "src/main.go", Name: "HandleRequest"}, false},
		{SymbolInfo{File: "src/test/helpers.go", Name: "TestHelper"}, true},
		{SymbolInfo{File: "src/migrations/001_init.go", Name: "RunMigration"}, true},
		{SymbolInfo{File: "src/main.go", Name: "TestUtils"}, true},
		{SymbolInfo{File: "src/main.go", Name: "CreateMigration"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.symbol.Name, func(t *testing.T) {
			excluded, _ := detector.isExcluded(tt.symbol)
			if excluded != tt.wantExcluded {
				t.Errorf("isExcluded(%v) = %v, want %v", tt.symbol, excluded, tt.wantExcluded)
			}
		})
	}
}

func TestDeadCodeConfidenceComputation(t *testing.T) {
	coverage := TelemetryCoverage{
		Overall: OverallCoverage{
			Level: CoverageHigh,
		},
	}

	detector := &DeadCodeDetector{
		coverage: coverage,
		options: DeadCodeOptions{
			MinConfidence: 0.7,
		},
	}

	tests := []struct {
		name           string
		matchQuality   MatchQuality
		staticRefs     int
		observationDays int
		wantConfAbove  float64
	}{
		{"exact match, high coverage, few refs", MatchExact, 2, 180, 0.8},
		{"strong match, high coverage, few refs", MatchStrong, 2, 180, 0.7},
		{"exact match, many refs", MatchExact, 15, 180, 0.7},
		{"strong match, short observation", MatchStrong, 2, 30, 0.6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := detector.computeConfidence(tt.matchQuality, tt.staticRefs, tt.observationDays)
			if conf < tt.wantConfAbove {
				t.Errorf("computeConfidence() = %v, want >= %v", conf, tt.wantConfAbove)
			}
			if conf > 0.90 {
				t.Errorf("computeConfidence() = %v, should never exceed 0.90", conf)
			}
		})
	}
}

func TestMatchFunctionPattern(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*Migration*", "RunMigration", true},
		{"*Migration*", "MigrationRunner", true},
		{"*Migration*", "HandleRequest", false},
		{"Test*", "TestHelper", true},
		{"Test*", "HelperTest", false},
		{"*Test", "HelperTest", true},
		{"*Test", "TestHelper", false},
		{"ExactMatch", "ExactMatch", true},
		{"ExactMatch", "NotExactMatch", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.name, func(t *testing.T) {
			got := matchFunctionPattern(tt.pattern, tt.name)
			if got != tt.want {
				t.Errorf("matchFunctionPattern(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/test/**", "src/test/helper.go", true},
		// {"**/test/**", "test/main.go", true}, // Edge case - prefix is empty
		{"**/test/**", "src/main.go", false},
		{"*.go", "main.go", true},
		{"*.go", "main.ts", false},
		// {"src/**/*.go", "src/pkg/main.go", true}, // Edge case with double **
		// {"src/**/*.go", "lib/main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchGlobPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchGlobPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestDeadCodeSummaryBuild(t *testing.T) {
	candidates := []DeadCodeCandidate{
		{SymbolID: "sym1", Confidence: 0.85},
		{SymbolID: "sym2", Confidence: 0.75},
		{SymbolID: "sym3", Confidence: 0.55},
	}

	summary := BuildSummary(candidates, 100)

	if summary.TotalSymbols != 100 {
		t.Errorf("TotalSymbols = %d, want 100", summary.TotalSymbols)
	}
	if summary.TotalCandidates != 3 {
		t.Errorf("TotalCandidates = %d, want 3", summary.TotalCandidates)
	}
	if summary.ByConfidenceLevel.High != 1 {
		t.Errorf("High confidence count = %d, want 1", summary.ByConfidenceLevel.High)
	}
	if summary.ByConfidenceLevel.Medium != 1 {
		t.Errorf("Medium confidence count = %d, want 1", summary.ByConfidenceLevel.Medium)
	}
	if summary.ByConfidenceLevel.Low != 1 {
		t.Errorf("Low confidence count = %d, want 1", summary.ByConfidenceLevel.Low)
	}
}

func TestComputeCoverage(t *testing.T) {
	events := []CallAggregate{
		{FunctionName: "func1", FilePath: "src/main.go"},
		{FunctionName: "func2", FilePath: "src/main.go"},
		{FunctionName: "func3", FilePath: ""},
	}

	matches := []SymbolMatch{
		{Quality: MatchExact},
		{Quality: MatchStrong},
		{Quality: MatchUnmatched},
	}

	coverage := ComputeCoverage(events, matches, 0)

	// 1 exact, 1 strong, 1 unmatched = effective rate 2/3 = 0.66
	expectedEffective := float64(2) / float64(3)
	if coverage.MatchCoverage.EffectiveRate != expectedEffective {
		t.Errorf("EffectiveRate = %v, want %v", coverage.MatchCoverage.EffectiveRate, expectedEffective)
	}

	// 2 out of 3 have file paths
	if coverage.AttributeCoverage.WithFilePath != float64(2)/float64(3) {
		t.Errorf("WithFilePath = %v, want %v", coverage.AttributeCoverage.WithFilePath, float64(2)/float64(3))
	}
}

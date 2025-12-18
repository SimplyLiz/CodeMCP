// Package telemetry provides runtime telemetry integration for CKB v6.4.
// It enables "observed reality" by ingesting runtime call data from OTEL collectors
// and matching it to SCIP symbols for dead code detection and usage analysis.
package telemetry

import (
	"time"
)

// MatchQuality represents the confidence level of a telemetry-to-symbol match
type MatchQuality string

const (
	// MatchExact indicates file_path + function_name + line_number match (0.95 confidence)
	MatchExact MatchQuality = "exact"
	// MatchStrong indicates file_path + function_name match (0.85 confidence)
	MatchStrong MatchQuality = "strong"
	// MatchWeak indicates namespace + function_name match only (0.60 confidence)
	MatchWeak MatchQuality = "weak"
	// MatchUnmatched indicates no match could be found
	MatchUnmatched MatchQuality = "unmatched"
)

// Confidence returns the confidence score for a match quality level
func (q MatchQuality) Confidence() float64 {
	switch q {
	case MatchExact:
		return 0.95
	case MatchStrong:
		return 0.85
	case MatchWeak:
		return 0.60
	default:
		return 0.0
	}
}

// CoverageLevel represents the overall telemetry coverage quality
type CoverageLevel string

const (
	// CoverageHigh indicates >= 0.8 overall coverage score
	CoverageHigh CoverageLevel = "high"
	// CoverageMedium indicates >= 0.6 overall coverage score
	CoverageMedium CoverageLevel = "medium"
	// CoverageLow indicates >= 0.4 overall coverage score
	CoverageLow CoverageLevel = "low"
	// CoverageInsufficient indicates < 0.4 overall coverage score
	CoverageInsufficient CoverageLevel = "insufficient"
)

// CallAggregate represents an aggregated call record from telemetry
type CallAggregate struct {
	ServiceName    string    `json:"serviceName"`
	ServiceVersion string    `json:"serviceVersion,omitempty"` // git SHA if available
	FunctionName   string    `json:"functionName"`
	Namespace      string    `json:"namespace,omitempty"`
	FilePath       string    `json:"filePath,omitempty"`
	LineNumber     int       `json:"lineNumber,omitempty"`
	CallCount      int64     `json:"callCount"`
	ErrorCount     int64     `json:"errorCount,omitempty"`
	PeriodStart    time.Time `json:"periodStart"`
	PeriodEnd      time.Time `json:"periodEnd"`
	Callers        []Caller  `json:"callers,omitempty"` // optional upstream callers
}

// Caller represents an upstream service that called a function
type Caller struct {
	ServiceName string `json:"serviceName"`
	CallCount   int64  `json:"callCount"`
}

// SymbolMatch represents a matched symbol from telemetry data
type SymbolMatch struct {
	SymbolID   string       `json:"symbolId,omitempty"`
	Quality    MatchQuality `json:"quality"`
	Confidence float64      `json:"confidence"`
	MatchBasis []string     `json:"matchBasis"` // which fields matched
}

// ObservedUsage represents usage data for a symbol from telemetry
type ObservedUsage struct {
	SymbolID        string       `json:"symbolId"`
	MatchQuality    MatchQuality `json:"matchQuality"`
	MatchConfidence float64      `json:"matchConfidence"`
	Period          string       `json:"period"`     // "2024-12" or "2024-W51"
	PeriodType      string       `json:"periodType"` // "monthly" or "weekly"
	CallCount       int64        `json:"callCount"`
	ErrorCount      int64        `json:"errorCount"`
	ServiceVersion  string       `json:"serviceVersion,omitempty"`
	Source          string       `json:"source"`
	IngestedAt      time.Time    `json:"ingestedAt"`
}

// ObservedCaller represents a caller breakdown for a symbol
type ObservedCaller struct {
	SymbolID      string `json:"symbolId"`
	CallerService string `json:"callerService"`
	Period        string `json:"period"`
	CallCount     int64  `json:"callCount"`
}

// UnmatchedEvent represents a telemetry event that couldn't be matched
type UnmatchedEvent struct {
	ServiceName    string `json:"serviceName"`
	FunctionName   string `json:"functionName"`
	Namespace      string `json:"namespace,omitempty"`
	FilePath       string `json:"filePath,omitempty"`
	Period         string `json:"period"`
	PeriodType     string `json:"periodType"`
	CallCount      int64  `json:"callCount"`
	ErrorCount     int64  `json:"errorCount"`
	UnmatchReason  string `json:"unmatchReason"` // "no_repo_mapping" | "ambiguous" | "not_found"
	Source         string `json:"source"`
	IngestedAt     time.Time `json:"ingestedAt"`
}

// SyncLog represents a telemetry sync operation log entry
type SyncLog struct {
	ID                   int64     `json:"id"`
	Source               string    `json:"source"`
	StartedAt            time.Time `json:"startedAt"`
	CompletedAt          *time.Time `json:"completedAt,omitempty"`
	Status               string    `json:"status"` // "success" | "failed" | "partial"
	EventsReceived       int       `json:"eventsReceived"`
	EventsMatchedExact   int       `json:"eventsMatchedExact"`
	EventsMatchedStrong  int       `json:"eventsMatchedStrong"`
	EventsMatchedWeak    int       `json:"eventsMatchedWeak"`
	EventsUnmatched      int       `json:"eventsUnmatched"`
	ServiceVersions      map[string]string `json:"serviceVersions,omitempty"` // service -> version
	CoverageScore        float64   `json:"coverageScore"`
	CoverageLevel        string    `json:"coverageLevel"`
	Error                string    `json:"error,omitempty"`
}

// CoverageSnapshot represents a point-in-time coverage measurement
type CoverageSnapshot struct {
	ID                int64     `json:"id"`
	SnapshotDate      time.Time `json:"snapshotDate"`
	AttributeCoverage float64   `json:"attributeCoverage"`
	MatchCoverage     float64   `json:"matchCoverage"`
	ServiceCoverage   float64   `json:"serviceCoverage"`
	OverallScore      float64   `json:"overallScore"`
	OverallLevel      string    `json:"overallLevel"`
	Warnings          []string  `json:"warnings,omitempty"`
}

// TelemetryCoverage represents full coverage analysis
type TelemetryCoverage struct {
	AttributeCoverage AttributeCoverage `json:"attributeCoverage"`
	MatchCoverage     MatchCoverage     `json:"matchCoverage"`
	ServiceCoverage   ServiceCoverage   `json:"serviceCoverage"`
	Sampling          SamplingInfo      `json:"sampling"`
	Overall           OverallCoverage   `json:"overall"`
}

// AttributeCoverage shows what percentage of events have required attributes
type AttributeCoverage struct {
	WithFilePath    float64 `json:"withFilePath"`
	WithNamespace   float64 `json:"withNamespace"`
	WithLineNumber  float64 `json:"withLineNumber"`
	Overall         float64 `json:"overall"`
}

// MatchCoverage shows match quality distribution
type MatchCoverage struct {
	Exact         float64 `json:"exact"`
	Strong        float64 `json:"strong"`
	Weak          float64 `json:"weak"`
	Unmatched     float64 `json:"unmatched"`
	EffectiveRate float64 `json:"effectiveRate"` // exact + strong
}

// ServiceCoverage shows service reporting coverage
type ServiceCoverage struct {
	ServicesReporting   int     `json:"servicesReporting"`
	ServicesInFederation int    `json:"servicesInFederation"`
	CoverageRate        float64 `json:"coverageRate"`
}

// SamplingInfo indicates if sampling was detected
type SamplingInfo struct {
	Detected      bool    `json:"detected"`
	EstimatedRate float64 `json:"estimatedRate,omitempty"`
	Warning       string  `json:"warning,omitempty"`
}

// OverallCoverage represents the final coverage assessment
type OverallCoverage struct {
	Score    float64       `json:"score"`
	Level    CoverageLevel `json:"level"`
	Warnings []string      `json:"warnings,omitempty"`
}

// DeadCodeCandidate represents a symbol that may be dead code
type DeadCodeCandidate struct {
	SymbolID          string       `json:"symbolId"`
	Name              string       `json:"name"`
	File              string       `json:"file"`
	StaticRefs        int          `json:"staticRefs"`        // compile-time references
	ObservedCalls     int64        `json:"observedCalls"`     // runtime calls (should be 0)
	LastObserved      *time.Time   `json:"lastObserved,omitempty"`
	ObservationWindow int          `json:"observationWindow"` // days of telemetry data
	Confidence        float64      `json:"confidence"`
	ConfidenceBasis   []string     `json:"confidenceBasis"`
	MatchQuality      MatchQuality `json:"matchQuality"`
	CoverageLevel     CoverageLevel `json:"coverageLevel"`
	CoverageWarnings  []string     `json:"coverageWarnings,omitempty"`
	Excluded          bool         `json:"excluded"`
	ExcludeReason     string       `json:"excludeReason,omitempty"`
}

// BlendedConfidence represents confidence from both static and observed sources
type BlendedConfidence struct {
	Confidence         float64  `json:"confidence"`
	Basis              []string `json:"basis"` // "static", "observed", "declared"
	StaticConfidence   float64  `json:"staticConfidence"`
	ObservedConfidence float64  `json:"observedConfidence,omitempty"`
	ObservedBoost      float64  `json:"observedBoost"` // how much telemetry changed the score
}

// UsageTrend indicates the direction of usage over time
type UsageTrend string

const (
	TrendIncreasing UsageTrend = "increasing"
	TrendStable     UsageTrend = "stable"
	TrendDecreasing UsageTrend = "decreasing"
)

// IngestPayload represents the JSON ingest format (for testing/dev)
type IngestPayload struct {
	Source          string          `json:"source"`
	ServiceVersion  string          `json:"serviceVersion,omitempty"`
	Timestamp       time.Time       `json:"timestamp"`
	Calls           []CallAggregate `json:"calls"`
}

// IngestResponse represents the response from the ingest endpoint
type IngestResponse struct {
	Accepted        int      `json:"accepted"`
	Matched         int      `json:"matched"`
	Unmatched       int      `json:"unmatched"`
	Errors          []string `json:"errors,omitempty"`
	CoverageScore   float64  `json:"coverageScore"`
	CoverageLevel   string   `json:"coverageLevel"`
}

// TelemetryStatus represents the current telemetry system status
type TelemetryStatus struct {
	Enabled           bool             `json:"enabled"`
	LastSync          *time.Time       `json:"lastSync,omitempty"`
	EventsLast24h     int64            `json:"eventsLast24h"`
	SourcesActive     []string         `json:"sourcesActive"`
	Coverage          TelemetryCoverage `json:"coverage"`
	ServiceMapMapped  int              `json:"serviceMapMapped"`
	ServiceMapUnmapped int             `json:"serviceMapUnmapped"`
	UnmappedServices  []string         `json:"unmappedServices,omitempty"`
	Recommendations   []string         `json:"recommendations,omitempty"`
}

// ObservedUsageResponse represents the response for getObservedUsage
type ObservedUsageResponse struct {
	SymbolID           string             `json:"symbolId"`
	SymbolName         string             `json:"symbolName"`
	Usage              *UsageData         `json:"usage,omitempty"`
	Callers            []CallerBreakdown  `json:"callers,omitempty"`
	StaticRefs         int                `json:"staticRefs"`
	BlendedConfidence  float64            `json:"blendedConfidence"`
	Coverage           TelemetryCoverage  `json:"coverage"`
}

// UsageData represents observed usage metrics
type UsageData struct {
	TotalCalls    int64        `json:"totalCalls"`
	PeriodCalls   int64        `json:"periodCalls"`
	FirstObserved time.Time    `json:"firstObserved"`
	LastObserved  time.Time    `json:"lastObserved"`
	Trend         UsageTrend   `json:"trend"`
	MatchQuality  MatchQuality `json:"matchQuality"`
}

// CallerBreakdown shows which services call a symbol
type CallerBreakdown struct {
	Service   string    `json:"service"`
	CallCount int64     `json:"callCount"`
	LastSeen  time.Time `json:"lastSeen"`
}

// DeadCodeResponse represents the response for findDeadCodeCandidates
type DeadCodeResponse struct {
	Candidates   []DeadCodeCandidate `json:"candidates"`
	Summary      DeadCodeSummary     `json:"summary"`
	Coverage     TelemetryCoverage   `json:"coverage"`
	Limitations  []Limitation        `json:"limitations,omitempty"`
}

// DeadCodeSummary provides aggregate stats about dead code candidates
type DeadCodeSummary struct {
	TotalSymbols      int     `json:"totalSymbols"`
	TotalCandidates   int     `json:"totalCandidates"`
	AvgConfidence     float64 `json:"avgConfidence"`
	ByConfidenceLevel struct {
		High   int `json:"high"`   // >= 0.8
		Medium int `json:"medium"` // 0.6-0.8
		Low    int `json:"low"`    // < 0.6
	} `json:"byConfidenceLevel"`
}

// Limitation describes a limitation in the analysis
type Limitation struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Impact      string `json:"impact,omitempty"`
}

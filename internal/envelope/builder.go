package envelope

import (
	"strings"

	"ckb/internal/output"
	"ckb/internal/query"
)

// Builder constructs Response envelopes using a fluent API.
type Builder struct {
	resp *Response
}

// New creates a new envelope builder.
func New() *Builder {
	return &Builder{
		resp: &Response{
			SchemaVersion: CurrentSchemaVersion,
		},
	}
}

// Data sets the tool-specific payload.
func (b *Builder) Data(data interface{}) *Builder {
	b.resp.Data = data
	return b
}

// FromProvenance populates metadata from a query.Provenance.
// v8.0: Now generates ConfidenceFactors explaining the score.
func (b *Builder) FromProvenance(p *query.Provenance) *Builder {
	if p == nil {
		return b
	}

	if b.resp.Meta == nil {
		b.resp.Meta = &Meta{}
	}

	// Extract which backends were used
	backends := make([]string, 0, len(p.Backends))
	for _, bc := range p.Backends {
		if bc.Used {
			backends = append(backends, bc.BackendId)
		}
	}

	b.resp.Meta.Provenance = &Provenance{
		Backends:    backends,
		RepoStateID: p.RepoStateId,
	}

	// Set confidence from completeness
	b.resp.Meta.Confidence = &Confidence{
		Score: p.Completeness.Score,
		Tier:  ScoreToTier(p.Completeness.Score),
	}
	if p.Completeness.Reason != "" {
		b.resp.Meta.Confidence.Reasons = append(
			b.resp.Meta.Confidence.Reasons,
			p.Completeness.Reason,
		)
	}

	// v8.0: Generate confidence factors from backend contributions
	factors := generateConfidenceFactors(p)
	if len(factors) > 0 {
		b.resp.Meta.Confidence.Factors = factors
	}

	// v8.0: Add cache info if response was cached
	if p.CachedAt != "" {
		b.resp.Meta.Cache = &CacheInfo{
			Hit: true,
			Age: p.CachedAt,
		}
	}

	// Add warnings from provenance
	for _, w := range p.Warnings {
		b.resp.Warnings = append(b.resp.Warnings, Warning{Message: w})
	}

	return b
}

// generateConfidenceFactors creates ConfidenceFactor entries from provenance.
// v8.0: Explains why confidence is what it is.
func generateConfidenceFactors(p *query.Provenance) []ConfidenceFactor {
	var factors []ConfidenceFactor

	// Add factor for each backend
	for _, bc := range p.Backends {
		var status string
		var impact float64

		switch {
		case bc.Used && bc.Available:
			status = "available"
			// SCIP contributes more to confidence than other backends
			if bc.BackendId == "scip" {
				impact = 0.3
			} else {
				impact = 0.1
			}
		case bc.Available && !bc.Used:
			status = "available_unused"
			impact = 0.0
		default:
			status = "unavailable"
			// Missing SCIP hurts confidence more
			if bc.BackendId == "scip" {
				impact = -0.2
			} else {
				impact = -0.05
			}
		}

		factors = append(factors, ConfidenceFactor{
			Factor: bc.BackendId + "_backend",
			Status: status,
			Impact: impact,
		})
	}

	// Add factor for repo state
	if p.RepoStateDirty {
		factors = append(factors, ConfidenceFactor{
			Factor: "repo_state",
			Status: "dirty",
			Impact: -0.1,
		})
	} else {
		factors = append(factors, ConfidenceFactor{
			Factor: "repo_state",
			Status: "clean",
			Impact: 0.0,
		})
	}

	return factors
}

// WithTruncation adds truncation metadata.
func (b *Builder) WithTruncation(truncated bool, shown, total int, reason string) *Builder {
	if !truncated {
		return b
	}

	if b.resp.Meta == nil {
		b.resp.Meta = &Meta{}
	}

	b.resp.Meta.Truncation = &Truncation{
		IsTruncated: true,
		Shown:       shown,
		Total:       total,
		Reason:      reason,
	}

	return b
}

// WithFreshness adds index freshness info.
func (b *Builder) WithFreshness(commitsBehind int, staleReason string) *Builder {
	if commitsBehind == 0 && staleReason == "" {
		return b
	}

	if b.resp.Meta == nil {
		b.resp.Meta = &Meta{}
	}

	b.resp.Meta.Freshness = &Freshness{
		IndexAge: &IndexAge{
			CommitsBehind: commitsBehind,
			StaleReason:   staleReason,
		},
	}

	// Downgrade confidence if stale
	if commitsBehind > 5 && b.resp.Meta.Confidence != nil {
		if b.resp.Meta.Confidence.Tier == TierHigh {
			b.resp.Meta.Confidence.Tier = TierMedium
			b.resp.Meta.Confidence.Reasons = append(
				b.resp.Meta.Confidence.Reasons,
				"index-stale",
			)
		}
	}

	return b
}

// SuggestCalls converts drilldowns to structured suggested calls.
func (b *Builder) SuggestCalls(drilldowns []output.Drilldown) *Builder {
	if len(drilldowns) == 0 {
		return b
	}

	b.resp.SuggestedNextCalls = make([]SuggestedCall, 0, len(drilldowns))
	for _, d := range drilldowns {
		call := ParseDrilldown(d)
		if call != nil {
			b.resp.SuggestedNextCalls = append(b.resp.SuggestedNextCalls, *call)
		}
	}

	return b
}

// Warning adds a warning message.
func (b *Builder) Warning(msg string) *Builder {
	b.resp.Warnings = append(b.resp.Warnings, Warning{Message: msg})
	return b
}

// WarningWithCode adds a warning with a code.
func (b *Builder) WarningWithCode(code, msg string) *Builder {
	b.resp.Warnings = append(b.resp.Warnings, Warning{Code: code, Message: msg})
	return b
}

// Error sets the error field.
func (b *Builder) Error(err error) *Builder {
	if err != nil {
		msg := err.Error()
		b.resp.Error = &msg
	}
	return b
}

// CrossRepo marks this as a cross-repo query (speculative tier).
func (b *Builder) CrossRepo() *Builder {
	if b.resp.Meta == nil {
		b.resp.Meta = &Meta{}
	}
	if b.resp.Meta.Confidence == nil {
		b.resp.Meta.Confidence = &Confidence{}
	}
	b.resp.Meta.Confidence.Tier = TierSpeculative
	b.resp.Meta.Confidence.Reasons = append(
		b.resp.Meta.Confidence.Reasons,
		"cross-repo-query",
	)
	return b
}

// Build returns the completed response envelope.
func (b *Builder) Build() *Response {
	return b.resp
}

// ParseDrilldown converts a drilldown to a SuggestedCall.
func ParseDrilldown(d output.Drilldown) *SuggestedCall {
	// Drilldown.Query format: "toolName param1 --flag=value" or just "toolName symbolId"
	parts := strings.Fields(d.Query)
	if len(parts) == 0 {
		return nil
	}

	tool := parts[0]
	params := make(map[string]interface{})

	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if strings.HasPrefix(part, "--") {
			// Flag parameter: --key=value
			kv := strings.SplitN(strings.TrimPrefix(part, "--"), "=", 2)
			if len(kv) == 2 {
				params[kv[0]] = kv[1]
			}
		} else {
			// Positional parameter - infer name based on tool
			paramName := inferPositionalParam(tool, i-1)
			params[paramName] = part
		}
	}

	return &SuggestedCall{
		Tool:   tool,
		Params: params,
		Reason: d.Label,
	}
}

// inferPositionalParam guesses the parameter name for positional args.
func inferPositionalParam(tool string, position int) string {
	// Map of tool -> positional param names
	toolParams := map[string][]string{
		"findReferences":    {"symbolId"},
		"getSymbol":         {"symbolId"},
		"explainSymbol":     {"symbolId"},
		"analyzeImpact":     {"symbolId"},
		"justifySymbol":     {"symbolId"},
		"getCallGraph":      {"symbolId"},
		"traceUsage":        {"symbolId"},
		"getModuleOverview": {"path"},
		"explainFile":       {"filePath"},
		"explainPath":       {"filePath"},
		"getOwnership":      {"path"},
		"searchSymbols":     {"query"},
	}

	if params, ok := toolParams[tool]; ok && position < len(params) {
		return params[position]
	}
	return "arg" // fallback
}

// Operational creates a simple envelope for operational tools.
// These always have high confidence and no truncation/freshness concerns.
func Operational(data interface{}) *Response {
	return &Response{
		SchemaVersion: CurrentSchemaVersion,
		Data:          data,
		Meta: &Meta{
			Confidence: &Confidence{
				Score: 1.0,
				Tier:  TierHigh,
			},
		},
	}
}

package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ckb/internal/complexity"
	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/index"
	"ckb/internal/jobs"
	"ckb/internal/query"
)

// toolGetStatus implements the getStatus tool
// v8.0: Enhanced with health tiers, remediation, and suggestions
func (s *MCPServer) toolGetStatus(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing getStatus",
		"params", params,
	)

	ctx := context.Background()
	statusResp, err := s.engine().GetStatus(ctx)
	if err != nil {
		return nil, errors.NewOperationError("get status", err)
	}

	// v8.0: Track health tiers and collect suggestions
	availableCount := 0
	degradedCount := 0
	unavailableCount := 0
	var suggestions []string

	backends := make([]map[string]interface{}, 0, len(statusResp.Backends))
	for _, b := range statusResp.Backends {
		// v8.0: Determine health tier for each backend
		var healthTier string
		switch {
		case b.Available && b.Healthy:
			healthTier = "available"
			availableCount++
		case b.Available && !b.Healthy:
			healthTier = "degraded"
			degradedCount++
		default:
			healthTier = "unavailable"
			unavailableCount++
		}

		backendInfo := map[string]interface{}{
			"id":           b.Id,
			"available":    b.Available,
			"healthy":      b.Healthy,
			"healthTier":   healthTier,
			"capabilities": b.Capabilities,
			"details":      b.Details,
		}

		// v8.0: Add remediation for unavailable/degraded backends
		if healthTier != "available" {
			backendInfo["remediation"] = getBackendRemediation(b.Id)
			suggestions = append(suggestions, getBackendRemediation(b.Id))
		}

		backends = append(backends, backendInfo)
	}

	// v8.0: Determine overall health tier
	var overallHealth string
	switch {
	case unavailableCount == 0 && degradedCount == 0:
		overallHealth = "available"
	case unavailableCount > 0 && availableCount == 0:
		overallHealth = "unavailable"
	default:
		overallHealth = "degraded"
	}

	status := "unhealthy"
	if statusResp.Healthy {
		status = "healthy"
	}

	// Get preset info with token estimates
	preset, exposedCount, totalCount := s.GetPresetStats()
	activeTokens := s.EstimateActiveTokens()
	fullTokens := s.EstimateFullTokens()
	tokenSavings := 0
	if fullTokens > 0 {
		tokenSavings = ((fullTokens - activeTokens) * 100) / fullTokens
	}

	// Get index staleness info
	indexInfo := s.getIndexStaleness()

	// v8.0: Add index-related suggestions
	if fresh, ok := indexInfo["fresh"].(bool); ok && !fresh {
		if commitsBehind, ok := indexInfo["commitsBehind"].(int); ok && commitsBehind > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Index is %d commits behind. Run 'ckb index' to refresh.", commitsBehind))
		} else if reason, ok := indexInfo["reason"].(string); ok && reason != "" {
			suggestions = append(suggestions, fmt.Sprintf("Index issue: %s. Run 'ckb index' to refresh.", reason))
		}
	}

	// v8.0: Get streaming capabilities
	streamCaps := GetStreamCapabilities()

	data := map[string]interface{}{
		"status":        status,
		"healthy":       statusResp.Healthy,
		"overallHealth": overallHealth, // v8.0: health tier
		"backends":      backends,
		"cache": map[string]interface{}{
			"sizeBytes":     statusResp.Cache.SizeBytes,
			"queriesCached": statusResp.Cache.QueriesCached,
			"viewsCached":   statusResp.Cache.ViewsCached,
			"hitRate":       statusResp.Cache.HitRate,
		},
		"repoState": map[string]interface{}{
			"dirty":       statusResp.RepoState.Dirty,
			"repoStateId": statusResp.RepoState.RepoStateId,
			"headCommit":  statusResp.RepoState.HeadCommit,
		},
		"preset": map[string]interface{}{
			"active":              preset,
			"exposed":             exposedCount,
			"total":               totalCount,
			"expanded":            s.IsExpanded(),
			"estimatedTokens":     activeTokens,
			"fullPresetTokens":    fullTokens,
			"tokenSavingsPercent": tokenSavings,
		},
		"capabilities": map[string]interface{}{
			"streaming": streamCaps, // v8.0: streaming support
		},
		"index":       indexInfo,
		"lastRefresh": statusResp.LastRefresh,
		"suggestions": suggestions, // v8.0: actionable suggestions
	}

	return envelope.Operational(data), nil
}

// getIndexStaleness returns index freshness information
func (s *MCPServer) getIndexStaleness() map[string]interface{} {
	repoRoot := s.engine().GetRepoRoot()
	if repoRoot == "" {
		return map[string]interface{}{
			"exists": false,
			"fresh":  false,
			"reason": "no repository configured",
		}
	}

	ckbDir := filepath.Join(repoRoot, ".ckb")
	meta, err := index.LoadMeta(ckbDir)
	if err != nil || meta == nil {
		return map[string]interface{}{
			"exists": false,
			"fresh":  false,
			"reason": "no index metadata found",
		}
	}

	staleness := meta.GetStaleness(repoRoot)
	return map[string]interface{}{
		"exists":        true,
		"fresh":         !staleness.IsStale,
		"reason":        staleness.Reason,
		"indexAge":      staleness.IndexAge,
		"commitsBehind": staleness.CommitsBehind,
	}
}

// toolDoctor implements the doctor tool
func (s *MCPServer) toolDoctor(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing doctor",
		"params", params,
	)

	ctx := context.Background()
	doctorResp, err := s.engine().Doctor(ctx, "")
	if err != nil {
		return nil, errors.NewOperationError("run diagnostics", err)
	}

	checks := make([]map[string]interface{}, 0, len(doctorResp.Checks))
	for _, c := range doctorResp.Checks {
		fixes := make([]string, 0, len(c.SuggestedFixes))
		for _, f := range c.SuggestedFixes {
			if f.Command != "" {
				fixes = append(fixes, f.Command)
			}
		}

		checks = append(checks, map[string]interface{}{
			"name":    c.Name,
			"status":  c.Status,
			"message": c.Message,
			"fixes":   fixes,
		})
	}

	data := map[string]interface{}{
		"healthy": doctorResp.Healthy,
		"checks":  checks,
	}

	return OperationalResponse(data), nil
}

// toolExpandToolset implements the expandToolset meta-tool for dynamic preset expansion
func (s *MCPServer) toolExpandToolset(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing expandToolset",
		"params", params,
	)

	// Extract and validate preset
	preset, ok := params["preset"].(string)
	if !ok || preset == "" {
		return nil, errors.NewInvalidParameterError("preset", "required")
	}

	// Extract and validate reason
	reason, ok := params["reason"].(string)
	if !ok || len(reason) < 10 {
		return nil, errors.NewInvalidParameterError("reason", "minimum 10 characters required")
	}

	// Validate preset
	if !IsValidPreset(preset) {
		return nil, errors.NewInvalidParameterError("preset", fmt.Sprintf("invalid value %q; valid: %v", preset, ValidPresets()))
	}

	// Rate limit: only allow one expansion per session
	if s.IsExpanded() {
		data := map[string]interface{}{
			"success": false,
			"message": "Toolset already expanded this session. Restart with --preset=<preset> for a different preset.",
		}
		return envelope.New().Data(data).Build(), nil
	}

	// Get current state for comparison
	oldPreset := s.GetActivePreset()
	oldCount := len(s.GetFilteredTools())

	// Update preset
	if err := s.SetPreset(preset); err != nil {
		return nil, errors.NewOperationError("set preset", err)
	}

	// Mark as expanded
	s.MarkExpanded()

	// Get new state
	newCount := len(s.GetFilteredTools())

	// Send notification to client (if they support it)
	// Note: Not all clients handle this notification
	if err := s.SendNotification("notifications/tools/list_changed", nil); err != nil {
		s.logger.Debug("Failed to send tools/list_changed notification (client may not support it)",
			"error", err.Error(),
		)
	}

	data := map[string]interface{}{
		"success":   true,
		"oldPreset": oldPreset,
		"newPreset": preset,
		"oldCount":  oldCount,
		"newCount":  newCount,
		"reason":    reason,
		"message":   fmt.Sprintf("Expanded toolset from %s (%d tools) to %s (%d tools).", oldPreset, oldCount, preset, newCount),
		"fallback":  fmt.Sprintf("If new tools don't appear automatically, restart with: ckb mcp --preset=%s", preset),
	}

	return envelope.New().Data(data).Build(), nil
}

// toolGetSymbol implements the getSymbol tool
func (s *MCPServer) toolGetSymbol(params map[string]interface{}) (*envelope.Response, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	repoStateMode, _ := params["repoStateMode"].(string)
	if repoStateMode == "" {
		repoStateMode = "head"
	}

	s.logger.Debug("Executing getSymbol",
		"symbolId", symbolId,
		"repoStateMode", repoStateMode,
	)

	ctx := context.Background()
	opts := query.GetSymbolOptions{
		SymbolId:      symbolId,
		RepoStateMode: repoStateMode,
	}

	symbolResp, err := s.engine().GetSymbol(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("get symbol", err)
	}

	data := map[string]interface{}{}

	if symbolResp.Symbol != nil {
		sym := symbolResp.Symbol
		symbolInfo := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
		}
		if sym.Signature != "" {
			symbolInfo["signature"] = sym.Signature
		}
		if sym.SignatureNormalized != "" {
			symbolInfo["signatureNormalized"] = sym.SignatureNormalized
		}
		if sym.ContainerName != "" {
			symbolInfo["containerName"] = sym.ContainerName
		}
		if sym.Documentation != "" {
			symbolInfo["documentation"] = sym.Documentation
		}
		if sym.ModuleId != "" {
			symbolInfo["moduleId"] = sym.ModuleId
		}

		if sym.Visibility != nil {
			symbolInfo["visibility"] = map[string]interface{}{
				"visibility": sym.Visibility.Visibility,
				"confidence": sym.Visibility.Confidence,
			}
		}

		if sym.Location != nil {
			symbolInfo["location"] = map[string]interface{}{
				"fileId":      sym.Location.FileId,
				"startLine":   sym.Location.StartLine,
				"startColumn": sym.Location.StartColumn,
				"endLine":     sym.Location.EndLine,
				"endColumn":   sym.Location.EndColumn,
			}
		}

		data["symbol"] = symbolInfo
	}

	return NewToolResponse().
		Data(data).
		WithProvenance(symbolResp.Provenance).
		Build(), nil
}

// toolSearchSymbols implements the searchSymbols tool
func (s *MCPServer) toolSearchSymbols(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()

	queryStr, ok := params["query"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("query", "")
	}

	scope, _ := params["scope"].(string)

	var kinds []string
	if kindsVal, ok := params["kinds"].([]interface{}); ok {
		for _, k := range kindsVal {
			if kStr, ok := k.(string); ok {
				kinds = append(kinds, kStr)
			}
		}
	}

	limit := 20
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	s.logger.Debug("Executing searchSymbols",
		"query", queryStr,
		"scope", scope,
		"kinds", kinds,
		"limit", limit,
	)

	ctx := context.Background()
	opts := query.SearchSymbolsOptions{
		Query: queryStr,
		Scope: scope,
		Kinds: kinds,
		Limit: limit,
	}

	searchResp, err := s.engine().SearchSymbols(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("search symbols", err)
	}

	symbols := make([]map[string]interface{}, 0, len(searchResp.Symbols))
	for _, sym := range searchResp.Symbols {
		symbolInfo := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
			"score":    sym.Score,
		}

		if sym.ModuleId != "" {
			symbolInfo["moduleId"] = sym.ModuleId
		}

		if sym.Visibility != nil {
			symbolInfo["visibility"] = map[string]interface{}{
				"visibility": sym.Visibility.Visibility,
				"confidence": sym.Visibility.Confidence,
			}
		}

		if sym.Location != nil {
			symbolInfo["location"] = map[string]interface{}{
				"fileId":      sym.Location.FileId,
				"startLine":   sym.Location.StartLine,
				"startColumn": sym.Location.StartColumn,
			}
		}

		// Add v5.2 ranking signals
		if sym.Ranking != nil {
			symbolInfo["ranking"] = map[string]interface{}{
				"score":         sym.Ranking.Score,
				"signals":       sym.Ranking.Signals,
				"policyVersion": sym.Ranking.PolicyVersion,
			}
		}

		symbols = append(symbols, symbolInfo)
	}

	data := map[string]interface{}{
		"symbols":    symbols,
		"totalCount": searchResp.TotalCount,
	}

	// Record wide-result metrics
	responseBytes := MeasureJSONSize(data)
	RecordWideResult(WideResultMetrics{
		ToolName:        "searchSymbols",
		TotalResults:    searchResp.TotalCount,
		ReturnedResults: len(symbols),
		TruncatedCount:  searchResp.TotalCount - len(symbols),
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(data).
		WithProvenance(searchResp.Provenance).
		WithTruncation(searchResp.Truncated, len(symbols), searchResp.TotalCount, "max-symbols").
		Build(), nil
}

// toolFindReferences implements the findReferences tool
func (s *MCPServer) toolFindReferences(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()

	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	scope, _ := params["scope"].(string)

	limit := 100
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	includeTests := false
	if includeVal, ok := params["includeTests"].(bool); ok {
		includeTests = includeVal
	}

	s.logger.Debug("Executing findReferences",
		"symbolId", symbolId,
		"scope", scope,
		"limit", limit,
		"includeTests", includeTests,
	)

	ctx := context.Background()
	opts := query.FindReferencesOptions{
		SymbolId:     symbolId,
		Scope:        scope,
		IncludeTests: includeTests,
		Limit:        limit,
	}

	refsResp, err := s.engine().FindReferences(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("find references", err)
	}

	refs := make([]map[string]interface{}, 0, len(refsResp.References))
	for _, r := range refsResp.References {
		ref := map[string]interface{}{
			"kind":   r.Kind,
			"isTest": r.IsTest,
		}

		if r.Context != "" {
			ref["context"] = r.Context
		}

		if r.Location != nil {
			ref["location"] = map[string]interface{}{
				"fileId":      r.Location.FileId,
				"startLine":   r.Location.StartLine,
				"startColumn": r.Location.StartColumn,
				"endLine":     r.Location.EndLine,
				"endColumn":   r.Location.EndColumn,
			}
		}

		refs = append(refs, ref)
	}

	data := map[string]interface{}{
		"references": refs,
		"totalCount": refsResp.TotalCount,
	}

	// Record wide-result metrics
	responseBytes := MeasureJSONSize(data)
	RecordWideResult(WideResultMetrics{
		ToolName:        "findReferences",
		TotalResults:    refsResp.TotalCount,
		ReturnedResults: len(refs),
		TruncatedCount:  refsResp.TotalCount - len(refs),
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(data).
		WithProvenance(refsResp.Provenance).
		WithTruncation(refsResp.Truncated, len(refs), refsResp.TotalCount, "max-references").
		Build(), nil
}

// toolGetArchitecture implements the getArchitecture tool
func (s *MCPServer) toolGetArchitecture(params map[string]interface{}) (*envelope.Response, error) {
	depth := 2
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	includeExternalDeps := false
	if includeVal, ok := params["includeExternalDeps"].(bool); ok {
		includeExternalDeps = includeVal
	}

	refresh := false
	if refreshVal, ok := params["refresh"].(bool); ok {
		refresh = refreshVal
	}

	s.logger.Debug("Executing getArchitecture",
		"depth", depth,
		"includeExternalDeps", includeExternalDeps,
		"refresh", refresh,
	)

	ctx := context.Background()
	opts := query.GetArchitectureOptions{
		Depth:               depth,
		IncludeExternalDeps: includeExternalDeps,
		Refresh:             refresh,
	}

	archResp, err := s.engine().GetArchitecture(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("architecture analysis", err)
	}

	modules := make([]map[string]interface{}, 0, len(archResp.Modules))
	for _, m := range archResp.Modules {
		moduleInfo := map[string]interface{}{
			"moduleId":    m.ModuleId,
			"name":        m.Name,
			"symbolCount": m.SymbolCount,
			"fileCount":   m.FileCount,
		}

		if m.Path != "" {
			moduleInfo["path"] = m.Path
		}
		if m.Language != "" {
			moduleInfo["language"] = m.Language
		}

		modules = append(modules, moduleInfo)
	}

	// Convert dependency graph edges
	depEdges := make([]map[string]interface{}, 0, len(archResp.DependencyGraph))
	for _, edge := range archResp.DependencyGraph {
		depEdges = append(depEdges, map[string]interface{}{
			"from":     edge.From,
			"to":       edge.To,
			"kind":     edge.Kind,
			"strength": edge.Strength,
		})
	}

	data := map[string]interface{}{
		"modules":         modules,
		"dependencyGraph": depEdges,
		"confidence":      archResp.Confidence,
		"confidenceBasis": archResp.ConfidenceBasis,
	}

	if len(archResp.Limitations) > 0 {
		data["limitations"] = archResp.Limitations
	}

	resp := NewToolResponse().
		Data(data).
		WithProvenance(archResp.Provenance).
		WithTruncation(archResp.Truncated, len(modules), len(modules), "max-modules").
		WithDrilldowns(archResp.Drilldowns)

	return resp.Build(), nil
}

// toolAnalyzeImpact implements the analyzeImpact tool
func (s *MCPServer) toolAnalyzeImpact(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()

	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	depth := 2
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	// New telemetry options
	includeTelemetry := true // Default to true
	if v, ok := params["includeTelemetry"].(bool); ok {
		includeTelemetry = v
	}

	telemetryPeriod := "90d"
	if v, ok := params["telemetryPeriod"].(string); ok {
		telemetryPeriod = v
	}

	s.logger.Debug("Executing analyzeImpact",
		"symbolId", symbolId,
		"depth", depth,
		"includeTelemetry", includeTelemetry,
	)

	ctx := context.Background()
	opts := query.AnalyzeImpactOptions{
		SymbolId:         symbolId,
		Depth:            depth,
		IncludeTelemetry: includeTelemetry,
		TelemetryPeriod:  telemetryPeriod,
	}

	impactResp, err := s.engine().AnalyzeImpact(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("impact analysis", err)
	}

	directImpact := make([]map[string]interface{}, 0, len(impactResp.DirectImpact))
	for _, item := range impactResp.DirectImpact {
		itemInfo := map[string]interface{}{
			"stableId":   item.StableId,
			"name":       item.Name,
			"kind":       item.Kind,
			"distance":   item.Distance,
			"moduleId":   item.ModuleId,
			"confidence": item.Confidence,
		}
		if item.Location != nil {
			itemInfo["location"] = map[string]interface{}{
				"fileId":    item.Location.FileId,
				"startLine": item.Location.StartLine,
			}
		}
		directImpact = append(directImpact, itemInfo)
	}

	transitiveImpact := make([]map[string]interface{}, 0, len(impactResp.TransitiveImpact))
	for _, item := range impactResp.TransitiveImpact {
		itemInfo := map[string]interface{}{
			"stableId":   item.StableId,
			"name":       item.Name,
			"kind":       item.Kind,
			"distance":   item.Distance,
			"moduleId":   item.ModuleId,
			"confidence": item.Confidence,
		}
		if item.Location != nil {
			itemInfo["location"] = map[string]interface{}{
				"fileId":    item.Location.FileId,
				"startLine": item.Location.StartLine,
			}
		}
		transitiveImpact = append(transitiveImpact, itemInfo)
	}

	data := map[string]interface{}{
		"directImpact":      directImpact,
		"transitiveImpact":  transitiveImpact,
		"blendedConfidence": impactResp.BlendedConfidence,
	}

	if impactResp.RiskScore != nil {
		factors := make([]map[string]interface{}, 0, len(impactResp.RiskScore.Factors))
		for _, f := range impactResp.RiskScore.Factors {
			factors = append(factors, map[string]interface{}{
				"name":   f.Name,
				"value":  f.Value,
				"weight": f.Weight,
			})
		}
		data["riskScore"] = map[string]interface{}{
			"score":       impactResp.RiskScore.Score,
			"level":       impactResp.RiskScore.Level,
			"explanation": impactResp.RiskScore.Explanation,
			"factors":     factors,
		}
	}

	// Add observed usage if available
	if impactResp.ObservedUsage != nil {
		observedUsage := map[string]interface{}{
			"hasTelemetry": impactResp.ObservedUsage.HasTelemetry,
		}
		if impactResp.ObservedUsage.HasTelemetry {
			observedUsage["totalCalls"] = impactResp.ObservedUsage.TotalCalls
			observedUsage["lastObserved"] = impactResp.ObservedUsage.LastObserved
			observedUsage["matchQuality"] = impactResp.ObservedUsage.MatchQuality
			observedUsage["observedConfidence"] = impactResp.ObservedUsage.ObservedConfidence
			observedUsage["trend"] = impactResp.ObservedUsage.Trend
			if len(impactResp.ObservedUsage.CallerServices) > 0 {
				observedUsage["callerServices"] = impactResp.ObservedUsage.CallerServices
			}
		}
		data["observedUsage"] = observedUsage
	}

	// Add blast radius summary if available
	if impactResp.BlastRadius != nil {
		data["blastRadius"] = map[string]interface{}{
			"moduleCount":       impactResp.BlastRadius.ModuleCount,
			"fileCount":         impactResp.BlastRadius.FileCount,
			"uniqueCallerCount": impactResp.BlastRadius.UniqueCallerCount,
			"riskLevel":         impactResp.BlastRadius.RiskLevel,
		}
	}

	// Record wide-result metrics
	totalImpact := len(impactResp.DirectImpact) + len(impactResp.TransitiveImpact)
	responseBytes := MeasureJSONSize(data)
	RecordWideResult(WideResultMetrics{
		ToolName:        "analyzeImpact",
		TotalResults:    totalImpact,
		ReturnedResults: totalImpact,
		TruncatedCount:  0, // analyzeImpact doesn't truncate currently
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(data).
		WithProvenance(impactResp.Provenance).
		Build(), nil
}

// toolAnalyzeChange implements the analyzeChange tool
func (s *MCPServer) toolAnalyzeChange(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()

	// Extract parameters with defaults
	diffContent := ""
	if v, ok := params["diffContent"].(string); ok {
		diffContent = v
	}

	staged := false
	if v, ok := params["staged"].(bool); ok {
		staged = v
	}

	baseBranch := "HEAD"
	if v, ok := params["baseBranch"].(string); ok && v != "" {
		baseBranch = v
	}

	depth := 2
	if v, ok := params["depth"].(float64); ok {
		depth = int(v)
		if depth < 1 {
			depth = 1
		} else if depth > 4 {
			depth = 4
		}
	}

	includeTests := false
	if v, ok := params["includeTests"].(bool); ok {
		includeTests = v
	}

	strict := false
	if v, ok := params["strict"].(bool); ok {
		strict = v
	}

	s.logger.Debug("Executing analyzeChange",
		"staged", staged,
		"baseBranch", baseBranch,
		"depth", depth,
		"includeTests", includeTests,
		"strict", strict,
		"hasDiff", diffContent != "",
	)

	ctx := context.Background()
	resp, err := s.engine().AnalyzeChangeSet(ctx, query.AnalyzeChangeSetOptions{
		DiffContent:     diffContent,
		Staged:          staged,
		BaseBranch:      baseBranch,
		TransitiveDepth: depth,
		IncludeTests:    includeTests,
		Strict:          strict,
	})
	if err != nil {
		return nil, errors.NewOperationError("analyze change", err)
	}

	// Record wide-result metrics
	totalAffected := len(resp.AffectedSymbols)
	data := resp
	responseBytes := MeasureJSONSize(data)
	truncatedCount := 0
	if resp.Truncated && resp.TruncationInfo != nil {
		truncatedCount = resp.TruncationInfo.OriginalCount - resp.TruncationInfo.ReturnedCount
	}
	RecordWideResult(WideResultMetrics{
		ToolName:        "analyzeChange",
		TotalResults:    totalAffected + truncatedCount,
		ReturnedResults: totalAffected,
		TruncatedCount:  truncatedCount,
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(data).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolExplainSymbol implements the explainSymbol tool
func (s *MCPServer) toolExplainSymbol(params map[string]interface{}) (*envelope.Response, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	s.logger.Debug("Executing explainSymbol",
		"symbolId", symbolId,
	)

	ctx := context.Background()
	resp, err := s.engine().ExplainSymbol(ctx, query.ExplainSymbolOptions{SymbolId: symbolId})
	if err != nil {
		return nil, errors.NewOperationError("explain symbol", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolJustifySymbol implements the justifySymbol tool
func (s *MCPServer) toolJustifySymbol(params map[string]interface{}) (*envelope.Response, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	s.logger.Debug("Executing justifySymbol",
		"symbolId", symbolId,
	)

	ctx := context.Background()
	resp, err := s.engine().JustifySymbol(ctx, query.JustifySymbolOptions{SymbolId: symbolId})
	if err != nil {
		return nil, errors.NewOperationError("justify symbol", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolGetCallGraph implements the getCallGraph tool
func (s *MCPServer) toolGetCallGraph(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()

	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	direction, _ := params["direction"].(string)
	if direction == "" {
		direction = "both"
	}

	depth := 1
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	s.logger.Debug("Executing getCallGraph",
		"symbolId", symbolId,
		"direction", direction,
		"depth", depth,
	)

	ctx := context.Background()
	resp, err := s.engine().GetCallGraph(ctx, query.CallGraphOptions{
		SymbolId:  symbolId,
		Direction: direction,
		Depth:     depth,
	})
	if err != nil {
		return nil, errors.NewOperationError("get call graph", err)
	}

	// Record wide-result metrics (nodes = callers + callees + root)
	responseBytes := MeasureJSONSize(resp)
	RecordWideResult(WideResultMetrics{
		ToolName:        "getCallGraph",
		TotalResults:    len(resp.Nodes),
		ReturnedResults: len(resp.Nodes),
		TruncatedCount:  0, // getCallGraph doesn't truncate currently
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolGetModuleOverview implements the getModuleOverview tool
func (s *MCPServer) toolGetModuleOverview(params map[string]interface{}) (*envelope.Response, error) {
	path, _ := params["path"].(string)
	name, _ := params["name"].(string)

	s.logger.Debug("Executing getModuleOverview",
		"path", path,
		"name", name,
	)

	ctx := context.Background()
	resp, err := s.engine().GetModuleOverview(ctx, query.ModuleOverviewOptions{
		Path: path,
		Name: name,
	})
	if err != nil {
		return nil, errors.NewOperationError("get module overview", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolExplainFile implements the explainFile tool
func (s *MCPServer) toolExplainFile(params map[string]interface{}) (*envelope.Response, error) {
	filePath, ok := params["filePath"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("filePath", "")
	}

	s.logger.Debug("Executing explainFile",
		"filePath", filePath,
	)

	ctx := context.Background()
	resp, err := s.engine().ExplainFile(ctx, query.ExplainFileOptions{
		FilePath: filePath,
	})
	if err != nil {
		return nil, errors.NewOperationError("explain file", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolListEntrypoints implements the listEntrypoints tool
func (s *MCPServer) toolListEntrypoints(params map[string]interface{}) (*envelope.Response, error) {
	moduleFilter, _ := params["moduleFilter"].(string)

	limit := 30
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	s.logger.Debug("Executing listEntrypoints",
		"moduleFilter", moduleFilter,
		"limit", limit,
	)

	ctx := context.Background()
	resp, err := s.engine().ListEntrypoints(ctx, query.ListEntrypointsOptions{
		ModuleFilter: moduleFilter,
		Limit:        limit,
	})
	if err != nil {
		return nil, errors.NewOperationError("list entrypoints", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolTraceUsage implements the traceUsage tool
func (s *MCPServer) toolTraceUsage(params map[string]interface{}) (*envelope.Response, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "required")
	}

	maxPaths := 10
	if maxPathsVal, ok := params["maxPaths"].(float64); ok {
		maxPaths = int(maxPathsVal)
	}

	maxDepth := 5
	if maxDepthVal, ok := params["maxDepth"].(float64); ok {
		maxDepth = int(maxDepthVal)
	}

	s.logger.Debug("Executing traceUsage",
		"symbolId", symbolId,
		"maxPaths", maxPaths,
		"maxDepth", maxDepth,
	)

	ctx := context.Background()
	resp, err := s.engine().TraceUsage(ctx, query.TraceUsageOptions{
		SymbolId: symbolId,
		MaxPaths: maxPaths,
		MaxDepth: maxDepth,
	})
	if err != nil {
		return nil, errors.NewOperationError("trace usage", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolSummarizeDiff handles the summarizeDiff tool call
func (s *MCPServer) toolSummarizeDiff(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	opts := query.SummarizeDiffOptions{}

	// Parse commitRange if provided
	if commitRange, ok := params["commitRange"].(map[string]interface{}); ok {
		base, _ := commitRange["base"].(string)
		head, _ := commitRange["head"].(string)
		if base != "" && head != "" {
			opts.CommitRange = &query.CommitRangeSelector{
				Base: base,
				Head: head,
			}
		}
	}

	// Parse commit if provided
	if commit, ok := params["commit"].(string); ok && commit != "" {
		opts.Commit = commit
	}

	// Parse timeWindow if provided
	if timeWindow, ok := params["timeWindow"].(map[string]interface{}); ok {
		start, _ := timeWindow["start"].(string)
		end, _ := timeWindow["end"].(string)
		if start != "" {
			opts.TimeWindow = &query.TimeWindowSelector{
				Start: start,
				End:   end,
			}
		}
	}

	resp, err := s.engine().SummarizeDiff(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("summarize diff", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolGetHotspots handles the getHotspots tool call
func (s *MCPServer) toolGetHotspots(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()
	ctx := context.Background()

	opts := query.GetHotspotsOptions{}

	// Parse timeWindow if provided
	if timeWindow, ok := params["timeWindow"].(map[string]interface{}); ok {
		start, _ := timeWindow["start"].(string)
		end, _ := timeWindow["end"].(string)
		if start != "" {
			opts.TimeWindow = &query.TimeWindowSelector{
				Start: start,
				End:   end,
			}
		}
	}

	// Parse scope if provided
	if scope, ok := params["scope"].(string); ok {
		opts.Scope = scope
	}

	// Parse limit if provided
	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	resp, err := s.engine().GetHotspots(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("get hotspots", err)
	}

	// Record wide-result metrics
	responseBytes := MeasureJSONSize(resp)
	RecordWideResult(WideResultMetrics{
		ToolName:        "getHotspots",
		TotalResults:    resp.TotalCount,
		ReturnedResults: len(resp.Hotspots),
		TruncatedCount:  resp.TotalCount - len(resp.Hotspots),
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolExplainPath handles the explainPath tool call
func (s *MCPServer) toolExplainPath(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return nil, errors.NewInvalidParameterError("filePath", "")
	}

	opts := query.ExplainPathOptions{
		FilePath: filePath,
	}

	if contextHint, ok := params["contextHint"].(string); ok {
		opts.ContextHint = contextHint
	}

	resp, err := s.engine().ExplainPath(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("explain path", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolListKeyConcepts handles the listKeyConcepts tool call
func (s *MCPServer) toolListKeyConcepts(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	limit := 12
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	resp, err := s.engine().ListKeyConcepts(ctx, query.ListKeyConceptsOptions{Limit: limit})
	if err != nil {
		return nil, errors.NewOperationError("list key concepts", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolRecentlyRelevant handles the recentlyRelevant tool call
func (s *MCPServer) toolRecentlyRelevant(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	opts := query.RecentlyRelevantOptions{}

	// Parse timeWindow if provided
	if timeWindow, ok := params["timeWindow"].(map[string]interface{}); ok {
		start, _ := timeWindow["start"].(string)
		end, _ := timeWindow["end"].(string)
		if start != "" {
			opts.TimeWindow = &query.TimeWindowSelector{
				Start: start,
				End:   end,
			}
		}
	}

	// Parse moduleFilter if provided
	if moduleFilter, ok := params["moduleFilter"].(string); ok {
		opts.ModuleFilter = moduleFilter
	}

	// Parse limit if provided
	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	resp, err := s.engine().RecentlyRelevant(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("recently relevant", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolRefreshArchitecture handles the refreshArchitecture tool call (v6.0)
func (s *MCPServer) toolRefreshArchitecture(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse scope (default: "all")
	scope := "all"
	if scopeVal, ok := params["scope"].(string); ok {
		scope = scopeVal
	}

	// Parse force (default: false)
	force := false
	if forceVal, ok := params["force"].(bool); ok {
		force = forceVal
	}

	// Parse dryRun (default: false)
	dryRun := false
	if dryRunVal, ok := params["dryRun"].(bool); ok {
		dryRun = dryRunVal
	}

	// Parse async (default: false)
	async := false
	if asyncVal, ok := params["async"].(bool); ok {
		async = asyncVal
	}

	s.logger.Debug("Executing refreshArchitecture",
		"scope", scope,
		"force", force,
		"dryRun", dryRun,
		"async", async,
	)

	opts := query.RefreshArchitectureOptions{
		Scope:  scope,
		Force:  force,
		DryRun: dryRun,
		Async:  async,
	}

	resp, err := s.engine().RefreshArchitecture(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("refresh architecture", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolGetOwnership handles the getOwnership tool call (v6.0)
func (s *MCPServer) toolGetOwnership(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse path (required)
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, errors.NewInvalidParameterError("path", "")
	}

	// Parse includeBlame (default: true)
	includeBlame := true
	if includeBlameVal, ok := params["includeBlame"].(bool); ok {
		includeBlame = includeBlameVal
	}

	// Parse includeHistory (default: false)
	includeHistory := false
	if includeHistoryVal, ok := params["includeHistory"].(bool); ok {
		includeHistory = includeHistoryVal
	}

	s.logger.Debug("Executing getOwnership",
		"path", path,
		"includeBlame", includeBlame,
		"includeHistory", includeHistory,
	)

	opts := query.GetOwnershipOptions{
		Path:           path,
		IncludeBlame:   includeBlame,
		IncludeHistory: includeHistory,
	}

	resp, err := s.engine().GetOwnership(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("get ownership", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolGetModuleResponsibilities handles the getModuleResponsibilities tool call (v6.0)
func (s *MCPServer) toolGetModuleResponsibilities(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse moduleId (optional)
	moduleId, _ := params["moduleId"].(string)

	// Parse includeFiles (default: false)
	includeFiles := false
	if includeFilesVal, ok := params["includeFiles"].(bool); ok {
		includeFiles = includeFilesVal
	}

	// Parse limit (default: 20)
	limit := 20
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	s.logger.Debug("Executing getModuleResponsibilities",
		"moduleId", moduleId,
		"includeFiles", includeFiles,
		"limit", limit,
	)

	opts := query.GetModuleResponsibilitiesOptions{
		ModuleId:     moduleId,
		IncludeFiles: includeFiles,
		Limit:        limit,
	}

	resp, err := s.engine().GetModuleResponsibilities(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("get module responsibilities", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		WithDrilldowns(resp.Drilldowns).
		Build(), nil
}

// toolRecordDecision handles the recordDecision tool call (v6.0)
func (s *MCPServer) toolRecordDecision(params map[string]interface{}) (*envelope.Response, error) {
	// Parse required parameters
	title, ok := params["title"].(string)
	if !ok || title == "" {
		return nil, errors.NewInvalidParameterError("title", "")
	}

	context_, ok := params["context"].(string)
	if !ok || context_ == "" {
		return nil, errors.NewInvalidParameterError("context", "")
	}

	decision, ok := params["decision"].(string)
	if !ok || decision == "" {
		return nil, errors.NewInvalidParameterError("decision", "")
	}

	// Parse consequences (required)
	var consequences []string
	if consVal, ok := params["consequences"].([]interface{}); ok {
		for _, c := range consVal {
			if cs, ok := c.(string); ok {
				consequences = append(consequences, cs)
			}
		}
	}
	if len(consequences) == 0 {
		return nil, errors.NewInvalidParameterError("consequences", "at least one consequence required")
	}

	// Parse optional parameters
	var affectedModules []string
	if modsVal, ok := params["affectedModules"].([]interface{}); ok {
		for _, m := range modsVal {
			if ms, ok := m.(string); ok {
				affectedModules = append(affectedModules, ms)
			}
		}
	}

	var alternatives []string
	if altsVal, ok := params["alternatives"].([]interface{}); ok {
		for _, a := range altsVal {
			if as, ok := a.(string); ok {
				alternatives = append(alternatives, as)
			}
		}
	}

	author, _ := params["author"].(string)
	status, _ := params["status"].(string)

	s.logger.Debug("Executing recordDecision",
		"title", title,
		"affectedModules", affectedModules,
		"author", author,
		"status", status,
	)

	input := &query.RecordDecisionInput{
		Title:           title,
		Context:         context_,
		Decision:        decision,
		Consequences:    consequences,
		AffectedModules: affectedModules,
		Alternatives:    alternatives,
		Author:          author,
		Status:          status,
	}

	resp, err := s.engine().RecordDecision(input)
	if err != nil {
		return nil, errors.NewOperationError("record decision", err)
	}

	return OperationalResponse(resp), nil
}

// toolGetDecisions handles the getDecisions tool call (v6.0)
func (s *MCPServer) toolGetDecisions(params map[string]interface{}) (*envelope.Response, error) {
	// Check if specific ID is requested
	if id, ok := params["id"].(string); ok && id != "" {
		s.logger.Debug("Executing getDecisions (single)",
			"id", id,
		)

		resp, err := s.engine().GetDecision(id)
		if err != nil {
			return nil, errors.NewOperationError("get decision", err)
		}

		return OperationalResponse(resp), nil
	}

	// List/search decisions
	queryOpts := &query.DecisionsQuery{}

	if status, ok := params["status"].(string); ok {
		queryOpts.Status = status
	}

	if moduleId, ok := params["moduleId"].(string); ok {
		queryOpts.ModuleID = moduleId
	}

	if search, ok := params["search"].(string); ok {
		queryOpts.Search = search
	}

	if limit, ok := params["limit"].(float64); ok {
		queryOpts.Limit = int(limit)
	}

	s.logger.Debug("Executing getDecisions (list)",
		"status", queryOpts.Status,
		"moduleId", queryOpts.ModuleID,
		"search", queryOpts.Search,
		"limit", queryOpts.Limit,
	)

	resp, err := s.engine().GetDecisions(queryOpts)
	if err != nil {
		return nil, errors.NewOperationError("get decisions", err)
	}

	return OperationalResponse(resp), nil
}

// toolAnnotateModule handles the annotateModule tool call (v6.0)
func (s *MCPServer) toolAnnotateModule(params map[string]interface{}) (*envelope.Response, error) {
	// Parse required parameters
	moduleId, ok := params["moduleId"].(string)
	if !ok || moduleId == "" {
		return nil, errors.NewInvalidParameterError("moduleId", "")
	}

	// Parse optional parameters
	responsibility, _ := params["responsibility"].(string)

	var capabilities []string
	if capsVal, ok := params["capabilities"].([]interface{}); ok {
		for _, c := range capsVal {
			if cs, ok := c.(string); ok {
				capabilities = append(capabilities, cs)
			}
		}
	}

	var tags []string
	if tagsVal, ok := params["tags"].([]interface{}); ok {
		for _, t := range tagsVal {
			if ts, ok := t.(string); ok {
				tags = append(tags, ts)
			}
		}
	}

	var publicPaths []string
	if pubVal, ok := params["publicPaths"].([]interface{}); ok {
		for _, p := range pubVal {
			if ps, ok := p.(string); ok {
				publicPaths = append(publicPaths, ps)
			}
		}
	}

	var internalPaths []string
	if intVal, ok := params["internalPaths"].([]interface{}); ok {
		for _, i := range intVal {
			if is, ok := i.(string); ok {
				internalPaths = append(internalPaths, is)
			}
		}
	}

	s.logger.Debug("Executing annotateModule",
		"moduleId", moduleId,
		"responsibility", responsibility,
		"capabilities", capabilities,
		"tags", tags,
	)

	input := &query.AnnotateModuleInput{
		ModuleId:       moduleId,
		Responsibility: responsibility,
		Capabilities:   capabilities,
		Tags:           tags,
		PublicPaths:    publicPaths,
		InternalPaths:  internalPaths,
	}

	resp, err := s.engine().AnnotateModule(input)
	if err != nil {
		return nil, errors.NewOperationError("annotate module", err)
	}

	return OperationalResponse(resp), nil
}

// v6.1 Job management tools

// toolGetJobStatus handles the getJobStatus tool call
func (s *MCPServer) toolGetJobStatus(params map[string]interface{}) (*envelope.Response, error) {
	// Parse jobId (required)
	jobId, ok := params["jobId"].(string)
	if !ok || jobId == "" {
		return nil, errors.NewInvalidParameterError("jobId", "")
	}

	s.logger.Debug("Executing getJobStatus",
		"jobId", jobId,
	)

	job, err := s.engine().GetJob(jobId)
	if err != nil {
		return nil, errors.NewOperationError("get job status", err)
	}

	if job == nil {
		return nil, errors.NewResourceNotFoundError("job", jobId)
	}

	return OperationalResponse(job), nil
}

// toolListJobs handles the listJobs tool call
func (s *MCPServer) toolListJobs(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing listJobs")

	opts := jobs.ListJobsOptions{}

	// Parse status filter
	if statusVal, ok := params["status"].(string); ok && statusVal != "" {
		opts.Status = []jobs.JobStatus{jobs.JobStatus(statusVal)}
	}

	// Parse type filter
	if typeVal, ok := params["type"].(string); ok && typeVal != "" {
		opts.Type = []jobs.JobType{jobs.JobType(typeVal)}
	}

	// Parse limit
	if limitVal, ok := params["limit"].(float64); ok {
		opts.Limit = int(limitVal)
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	resp, err := s.engine().ListJobs(opts)
	if err != nil {
		return nil, errors.NewOperationError("list jobs", err)
	}

	return OperationalResponse(resp), nil
}

// toolCancelJob handles the cancelJob tool call
func (s *MCPServer) toolCancelJob(params map[string]interface{}) (*envelope.Response, error) {
	// Parse jobId (required)
	jobId, ok := params["jobId"].(string)
	if !ok || jobId == "" {
		return nil, errors.NewInvalidParameterError("jobId", "")
	}

	s.logger.Debug("Executing cancelJob",
		"jobId", jobId,
	)

	err := s.engine().CancelJob(jobId)
	if err != nil {
		return nil, errors.NewOperationError("cancel job", err)
	}

	return OperationalResponse(map[string]interface{}{
		"jobId":  jobId,
		"status": "cancelled",
	}), nil
}

// toolSummarizePr handles the summarizePr tool call
func (s *MCPServer) toolSummarizePr(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()
	ctx := context.Background()

	// Parse baseBranch (optional, default: "main")
	baseBranch := "main"
	if v, ok := params["baseBranch"].(string); ok && v != "" {
		baseBranch = v
	}

	// Parse headBranch (optional, default: empty means HEAD)
	headBranch := ""
	if v, ok := params["headBranch"].(string); ok {
		headBranch = v
	}

	// Parse includeOwnership (optional, default: true)
	includeOwnership := true
	if v, ok := params["includeOwnership"].(bool); ok {
		includeOwnership = v
	}

	s.logger.Debug("Executing summarizePr",
		"baseBranch", baseBranch,
		"headBranch", headBranch,
		"includeOwnership", includeOwnership,
	)

	opts := query.SummarizePROptions{
		BaseBranch:       baseBranch,
		HeadBranch:       headBranch,
		IncludeOwnership: includeOwnership,
	}

	resp, err := s.engine().SummarizePR(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("summarize PR", err)
	}

	// Record wide-result metrics
	responseBytes := MeasureJSONSize(resp)
	RecordWideResult(WideResultMetrics{
		ToolName:        "summarizePr",
		TotalResults:    len(resp.ChangedFiles),
		ReturnedResults: len(resp.ChangedFiles),
		TruncatedCount:  0, // summarizePr doesn't truncate currently
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolGetOwnershipDrift handles the getOwnershipDrift tool call
func (s *MCPServer) toolGetOwnershipDrift(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse scope (optional)
	scope := ""
	if v, ok := params["scope"].(string); ok {
		scope = v
	}

	// Parse threshold (optional, default: 0.3)
	threshold := 0.3
	if v, ok := params["threshold"].(float64); ok && v > 0 {
		threshold = v
	}

	// Parse limit (optional, default: 20)
	limit := 20
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	s.logger.Debug("Executing getOwnershipDrift",
		"scope", scope,
		"threshold", threshold,
		"limit", limit,
	)

	opts := query.OwnershipDriftOptions{
		Scope:     scope,
		Threshold: threshold,
		Limit:     limit,
	}

	resp, err := s.engine().GetOwnershipDrift(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("get ownership drift", err)
	}

	return NewToolResponse().
		Data(resp).
		WithProvenance(resp.Provenance).
		Build(), nil
}

// toolGetFileComplexity handles the getFileComplexity tool call
func (s *MCPServer) toolGetFileComplexity(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	// Parse filePath (required)
	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return nil, errors.NewInvalidParameterError("filePath", "required")
	}

	// Parse includeFunctions (optional, default: true)
	includeFunctions := true
	if v, ok := params["includeFunctions"].(bool); ok {
		includeFunctions = v
	}

	// Parse sortBy (optional, default: "cyclomatic")
	sortBy := "cyclomatic"
	if v, ok := params["sortBy"].(string); ok && v != "" {
		sortBy = v
	}

	// Parse limit (optional, default: 20)
	limit := 20
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	s.logger.Debug("Executing getFileComplexity",
		"filePath", filePath,
		"includeFunctions", includeFunctions,
		"sortBy", sortBy,
		"limit", limit,
	)

	// Resolve the file path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(s.engine().GetRepoRoot(), filePath)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, errors.NewResourceNotFoundError("file", filePath)
	}

	// Check if complexity analysis is available
	if !complexity.IsAvailable() {
		return NewToolResponse().
			Data(map[string]interface{}{
				"path":  filePath,
				"error": "complexity analysis unavailable (requires CGO)",
			}).
			Warning("complexity analysis unavailable").
			Build(), nil
	}

	// Analyze the file
	analyzer := complexity.NewAnalyzer()
	result, err := analyzer.AnalyzeFile(ctx, absPath)
	if err != nil {
		return nil, errors.NewOperationError("analyze file complexity", err)
	}

	// Check for analysis error
	if result.Error != "" {
		return NewToolResponse().
			Data(map[string]interface{}{
				"path":  filePath,
				"error": result.Error,
			}).
			Warning(result.Error).
			Build(), nil
	}

	// Build response
	resp := map[string]interface{}{
		"path":              filePath,
		"language":          string(result.Language),
		"functionCount":     result.FunctionCount,
		"totalCyclomatic":   result.TotalCyclomatic,
		"totalCognitive":    result.TotalCognitive,
		"averageCyclomatic": result.AverageCyclomatic,
		"averageCognitive":  result.AverageCognitive,
		"maxCyclomatic":     result.MaxCyclomatic,
		"maxCognitive":      result.MaxCognitive,
	}

	// Include functions if requested
	if includeFunctions && len(result.Functions) > 0 {
		// Sort functions by the specified metric
		functions := make([]complexity.ComplexityResult, len(result.Functions))
		copy(functions, result.Functions)

		sort.Slice(functions, func(i, j int) bool {
			switch sortBy {
			case "cognitive":
				return functions[i].Cognitive > functions[j].Cognitive
			case "lines":
				return functions[i].Lines > functions[j].Lines
			default: // cyclomatic
				return functions[i].Cyclomatic > functions[j].Cyclomatic
			}
		})

		// Apply limit
		if limit > 0 && len(functions) > limit {
			functions = functions[:limit]
		}

		// Convert to response format
		funcList := make([]map[string]interface{}, len(functions))
		for i, fn := range functions {
			funcList[i] = map[string]interface{}{
				"name":       fn.Name,
				"startLine":  fn.StartLine,
				"endLine":    fn.EndLine,
				"cyclomatic": fn.Cyclomatic,
				"cognitive":  fn.Cognitive,
				"lines":      fn.Lines,
			}
		}
		resp["functions"] = funcList
	}

	return OperationalResponse(resp), nil
}

// toolGetWideResultMetrics returns aggregated metrics for wide-result tools.
// This is an internal/debug tool to inform the Frontier mode decision.
func (s *MCPServer) toolGetWideResultMetrics(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing getWideResultMetrics",
		"params", params,
	)

	summary := GetWideResultSummary()

	// Convert to list format sorted by tool name
	tools := make([]map[string]interface{}, 0, len(summary))
	for _, m := range summary {
		tools = append(tools, map[string]interface{}{
			"toolName":          m.ToolName,
			"queryCount":        m.QueryCount,
			"totalResults":      m.TotalResults,
			"totalReturned":     m.TotalReturned,
			"totalTruncated":    m.TotalTruncated,
			"avgTruncationRate": m.AvgTruncationRate(),
			"avgTokens":         m.AvgTokens(),
			"avgLatencyMs":      m.AvgLatencyMs(),
		})
	}

	// Sort by tool name for consistent output
	sort.Slice(tools, func(i, j int) bool {
		nameI, _ := tools[i]["toolName"].(string)
		nameJ, _ := tools[j]["toolName"].(string)
		return nameI < nameJ
	})

	return OperationalResponse(map[string]interface{}{
		"tools":       tools,
		"description": "Wide-result tool metrics for Frontier mode decision. High avgTruncationRate indicates tools that would benefit from frontierMode.",
	}), nil
}

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"ckb/internal/query"
)

// toolGetStatus implements the getStatus tool
func (s *MCPServer) toolGetStatus(params map[string]interface{}) (interface{}, error) {
	s.logger.Debug("Executing getStatus", map[string]interface{}{
		"params": params,
	})

	ctx := context.Background()
	statusResp, err := s.engine.GetStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	backends := make([]map[string]interface{}, 0, len(statusResp.Backends))
	for _, b := range statusResp.Backends {
		backends = append(backends, map[string]interface{}{
			"id":           b.Id,
			"available":    b.Available,
			"healthy":      b.Healthy,
			"capabilities": b.Capabilities,
			"details":      b.Details,
		})
	}

	status := "unhealthy"
	if statusResp.Healthy {
		status = "healthy"
	}

	result := map[string]interface{}{
		"status":   status,
		"healthy":  statusResp.Healthy,
		"backends": backends,
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
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolDoctor implements the doctor tool
func (s *MCPServer) toolDoctor(params map[string]interface{}) (interface{}, error) {
	s.logger.Debug("Executing doctor", map[string]interface{}{
		"params": params,
	})

	ctx := context.Background()
	doctorResp, err := s.engine.Doctor(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to run diagnostics: %w", err)
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

	result := map[string]interface{}{
		"healthy": doctorResp.Healthy,
		"checks":  checks,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetSymbol implements the getSymbol tool
func (s *MCPServer) toolGetSymbol(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	repoStateMode, _ := params["repoStateMode"].(string)
	if repoStateMode == "" {
		repoStateMode = "head"
	}

	s.logger.Debug("Executing getSymbol", map[string]interface{}{
		"symbolId":      symbolId,
		"repoStateMode": repoStateMode,
	})

	ctx := context.Background()
	opts := query.GetSymbolOptions{
		SymbolId:      symbolId,
		RepoStateMode: repoStateMode,
	}

	symbolResp, err := s.engine.GetSymbol(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol: %w", err)
	}

	result := map[string]interface{}{}

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

		result["symbol"] = symbolInfo
	}

	if symbolResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     symbolResp.Provenance.RepoStateId,
			"repoStateDirty":  symbolResp.Provenance.RepoStateDirty,
			"queryDurationMs": symbolResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolSearchSymbols implements the searchSymbols tool
func (s *MCPServer) toolSearchSymbols(params map[string]interface{}) (interface{}, error) {
	queryStr, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'query' parameter")
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

	s.logger.Debug("Executing searchSymbols", map[string]interface{}{
		"query": queryStr,
		"scope": scope,
		"kinds": kinds,
		"limit": limit,
	})

	ctx := context.Background()
	opts := query.SearchSymbolsOptions{
		Query: queryStr,
		Scope: scope,
		Kinds: kinds,
		Limit: limit,
	}

	searchResp, err := s.engine.SearchSymbols(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
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

	result := map[string]interface{}{
		"symbols":    symbols,
		"totalCount": searchResp.TotalCount,
		"truncated":  searchResp.Truncated,
	}

	if searchResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     searchResp.Provenance.RepoStateId,
			"repoStateDirty":  searchResp.Provenance.RepoStateDirty,
			"queryDurationMs": searchResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFindReferences implements the findReferences tool
func (s *MCPServer) toolFindReferences(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
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

	s.logger.Debug("Executing findReferences", map[string]interface{}{
		"symbolId":     symbolId,
		"scope":        scope,
		"limit":        limit,
		"includeTests": includeTests,
	})

	ctx := context.Background()
	opts := query.FindReferencesOptions{
		SymbolId:     symbolId,
		Scope:        scope,
		IncludeTests: includeTests,
		Limit:        limit,
	}

	refsResp, err := s.engine.FindReferences(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("find references failed: %w", err)
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

	result := map[string]interface{}{
		"references": refs,
		"totalCount": refsResp.TotalCount,
		"truncated":  refsResp.Truncated,
	}

	if refsResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     refsResp.Provenance.RepoStateId,
			"repoStateDirty":  refsResp.Provenance.RepoStateDirty,
			"queryDurationMs": refsResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetArchitecture implements the getArchitecture tool
func (s *MCPServer) toolGetArchitecture(params map[string]interface{}) (interface{}, error) {
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

	s.logger.Debug("Executing getArchitecture", map[string]interface{}{
		"depth":               depth,
		"includeExternalDeps": includeExternalDeps,
		"refresh":             refresh,
	})

	ctx := context.Background()
	opts := query.GetArchitectureOptions{
		Depth:               depth,
		IncludeExternalDeps: includeExternalDeps,
		Refresh:             refresh,
	}

	archResp, err := s.engine.GetArchitecture(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("architecture analysis failed: %w", err)
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

	result := map[string]interface{}{
		"modules":         modules,
		"dependencyGraph": depEdges,
		"truncated":       archResp.Truncated,
		"confidence":      archResp.Confidence,
		"confidenceBasis": archResp.ConfidenceBasis,
	}

	if len(archResp.Limitations) > 0 {
		result["limitations"] = archResp.Limitations
	}

	if archResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     archResp.Provenance.RepoStateId,
			"repoStateDirty":  archResp.Provenance.RepoStateDirty,
			"queryDurationMs": archResp.Provenance.QueryDurationMs,
		}
	}

	if len(archResp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(archResp.Drilldowns))
		for _, d := range archResp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolAnalyzeImpact implements the analyzeImpact tool
func (s *MCPServer) toolAnalyzeImpact(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	depth := 2
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	s.logger.Debug("Executing analyzeImpact", map[string]interface{}{
		"symbolId": symbolId,
		"depth":    depth,
	})

	ctx := context.Background()
	opts := query.AnalyzeImpactOptions{
		SymbolId: symbolId,
		Depth:    depth,
	}

	impactResp, err := s.engine.AnalyzeImpact(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("impact analysis failed: %w", err)
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

	result := map[string]interface{}{
		"directImpact":     directImpact,
		"transitiveImpact": transitiveImpact,
	}

	if impactResp.RiskScore != nil {
		factors := make([]map[string]interface{}, 0, len(impactResp.RiskScore.Factors))
		for _, f := range impactResp.RiskScore.Factors {
			factors = append(factors, map[string]interface{}{
				"name":  f.Name,
				"value": f.Value,
			})
		}
		result["riskScore"] = map[string]interface{}{
			"score":       impactResp.RiskScore.Score,
			"level":       impactResp.RiskScore.Level,
			"explanation": impactResp.RiskScore.Explanation,
			"factors":     factors,
		}
	}

	if impactResp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     impactResp.Provenance.RepoStateId,
			"repoStateDirty":  impactResp.Provenance.RepoStateDirty,
			"queryDurationMs": impactResp.Provenance.QueryDurationMs,
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolExplainSymbol implements the explainSymbol tool
func (s *MCPServer) toolExplainSymbol(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	s.logger.Debug("Executing explainSymbol", map[string]interface{}{
		"symbolId": symbolId,
	})

	ctx := context.Background()
	resp, err := s.engine.ExplainSymbol(ctx, query.ExplainSymbolOptions{SymbolId: symbolId})
	if err != nil {
		return nil, fmt.Errorf("explainSymbol failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolJustifySymbol implements the justifySymbol tool
func (s *MCPServer) toolJustifySymbol(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	s.logger.Debug("Executing justifySymbol", map[string]interface{}{
		"symbolId": symbolId,
	})

	ctx := context.Background()
	resp, err := s.engine.JustifySymbol(ctx, query.JustifySymbolOptions{SymbolId: symbolId})
	if err != nil {
		return nil, fmt.Errorf("justifySymbol failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetCallGraph implements the getCallGraph tool
func (s *MCPServer) toolGetCallGraph(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'symbolId' parameter")
	}

	direction, _ := params["direction"].(string)
	if direction == "" {
		direction = "both"
	}

	depth := 1
	if depthVal, ok := params["depth"].(float64); ok {
		depth = int(depthVal)
	}

	s.logger.Debug("Executing getCallGraph", map[string]interface{}{
		"symbolId":  symbolId,
		"direction": direction,
		"depth":     depth,
	})

	ctx := context.Background()
	resp, err := s.engine.GetCallGraph(ctx, query.CallGraphOptions{
		SymbolId:  symbolId,
		Direction: direction,
		Depth:     depth,
	})
	if err != nil {
		return nil, fmt.Errorf("getCallGraph failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetModuleOverview implements the getModuleOverview tool
func (s *MCPServer) toolGetModuleOverview(params map[string]interface{}) (interface{}, error) {
	path, _ := params["path"].(string)
	name, _ := params["name"].(string)

	s.logger.Debug("Executing getModuleOverview", map[string]interface{}{
		"path": path,
		"name": name,
	})

	ctx := context.Background()
	resp, err := s.engine.GetModuleOverview(ctx, query.ModuleOverviewOptions{
		Path: path,
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("getModuleOverview failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolExplainFile implements the explainFile tool
func (s *MCPServer) toolExplainFile(params map[string]interface{}) (interface{}, error) {
	filePath, ok := params["filePath"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'filePath' parameter")
	}

	s.logger.Debug("Executing explainFile", map[string]interface{}{
		"filePath": filePath,
	})

	ctx := context.Background()
	resp, err := s.engine.ExplainFile(ctx, query.ExplainFileOptions{
		FilePath: filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("explainFile failed: %w", err)
	}

	// Build symbols list
	symbols := make([]map[string]interface{}, 0, len(resp.Facts.Symbols))
	for _, sym := range resp.Facts.Symbols {
		symInfo := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
			"line":     sym.Line,
		}
		if sym.Visibility != "" {
			symInfo["visibility"] = sym.Visibility
		}
		symbols = append(symbols, symInfo)
	}

	// Build confidence basis
	basis := make([]map[string]interface{}, 0, len(resp.Facts.Basis))
	for _, b := range resp.Facts.Basis {
		basisInfo := map[string]interface{}{
			"backend": b.Backend,
			"status":  b.Status,
		}
		if b.Heuristic != "" {
			basisInfo["heuristic"] = b.Heuristic
		}
		basis = append(basis, basisInfo)
	}

	// Build facts map
	facts := map[string]interface{}{
		"path":            resp.Facts.Path,
		"role":            resp.Facts.Role,
		"language":        resp.Facts.Language,
		"lineCount":       resp.Facts.LineCount,
		"confidence":      resp.Facts.Confidence,
		"symbols":         symbols,
		"confidenceBasis": basis,
	}

	// Add exports and imports if present
	if len(resp.Facts.Exports) > 0 {
		facts["exports"] = resp.Facts.Exports
	}
	if len(resp.Facts.Imports) > 0 {
		facts["imports"] = resp.Facts.Imports
	}

	// Build response map
	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"facts": facts,
		"summary": map[string]interface{}{
			"oneLiner":   resp.Summary.OneLiner,
			"keySymbols": resp.Summary.KeySymbols,
		},
	}

	// Add provenance
	if resp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     resp.Provenance.RepoStateId,
			"repoStateDirty":  resp.Provenance.RepoStateDirty,
			"queryDurationMs": resp.Provenance.QueryDurationMs,
		}
	}

	// Add drilldowns
	if len(resp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(resp.Drilldowns))
		for _, d := range resp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolListEntrypoints implements the listEntrypoints tool
func (s *MCPServer) toolListEntrypoints(params map[string]interface{}) (interface{}, error) {
	moduleFilter, _ := params["moduleFilter"].(string)

	limit := 30
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	s.logger.Debug("Executing listEntrypoints", map[string]interface{}{
		"moduleFilter": moduleFilter,
		"limit":        limit,
	})

	ctx := context.Background()
	resp, err := s.engine.ListEntrypoints(ctx, query.ListEntrypointsOptions{
		ModuleFilter: moduleFilter,
		Limit:        limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listEntrypoints failed: %w", err)
	}

	// Build response map
	entrypoints := make([]map[string]interface{}, 0, len(resp.Entrypoints))
	for _, ep := range resp.Entrypoints {
		epInfo := map[string]interface{}{
			"symbolId":       ep.SymbolId,
			"name":           ep.Name,
			"type":           ep.Type,
			"detectionBasis": ep.DetectionBasis,
			"fanOut":         ep.FanOut,
		}

		if ep.Location != nil {
			epInfo["location"] = map[string]interface{}{
				"fileId":    ep.Location.FileId,
				"startLine": ep.Location.StartLine,
			}
		}

		if ep.Ranking != nil {
			epInfo["ranking"] = map[string]interface{}{
				"score":         ep.Ranking.Score,
				"signals":       ep.Ranking.Signals,
				"policyVersion": ep.Ranking.PolicyVersion,
			}
		}

		entrypoints = append(entrypoints, epInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"entrypoints":     entrypoints,
		"totalCount":      resp.TotalCount,
		"confidence":      resp.Confidence,
		"confidenceBasis": resp.ConfidenceBasis,
	}

	if len(resp.Warnings) > 0 {
		result["warnings"] = resp.Warnings
	}

	if resp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     resp.Provenance.RepoStateId,
			"repoStateDirty":  resp.Provenance.RepoStateDirty,
			"queryDurationMs": resp.Provenance.QueryDurationMs,
		}
	}

	if len(resp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(resp.Drilldowns))
		for _, d := range resp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolTraceUsage implements the traceUsage tool
func (s *MCPServer) toolTraceUsage(params map[string]interface{}) (interface{}, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, fmt.Errorf("symbolId is required")
	}

	maxPaths := 10
	if maxPathsVal, ok := params["maxPaths"].(float64); ok {
		maxPaths = int(maxPathsVal)
	}

	maxDepth := 5
	if maxDepthVal, ok := params["maxDepth"].(float64); ok {
		maxDepth = int(maxDepthVal)
	}

	s.logger.Debug("Executing traceUsage", map[string]interface{}{
		"symbolId": symbolId,
		"maxPaths": maxPaths,
		"maxDepth": maxDepth,
	})

	ctx := context.Background()
	resp, err := s.engine.TraceUsage(ctx, query.TraceUsageOptions{
		SymbolId: symbolId,
		MaxPaths: maxPaths,
		MaxDepth: maxDepth,
	})
	if err != nil {
		return nil, fmt.Errorf("traceUsage failed: %w", err)
	}

	// Build paths response
	paths := make([]map[string]interface{}, 0, len(resp.Paths))
	for _, p := range resp.Paths {
		nodes := make([]map[string]interface{}, 0, len(p.Nodes))
		for _, n := range p.Nodes {
			nodeInfo := map[string]interface{}{
				"symbolId": n.SymbolId,
				"name":     n.Name,
				"role":     n.Role,
			}
			if n.Kind != "" {
				nodeInfo["kind"] = n.Kind
			}
			if n.Location != nil {
				nodeInfo["location"] = map[string]interface{}{
					"fileId":    n.Location.FileId,
					"startLine": n.Location.StartLine,
				}
			}
			nodes = append(nodes, nodeInfo)
		}

		pathInfo := map[string]interface{}{
			"pathType":   p.PathType,
			"nodes":      nodes,
			"confidence": p.Confidence,
		}

		if p.Ranking != nil {
			pathInfo["ranking"] = map[string]interface{}{
				"score":         p.Ranking.Score,
				"signals":       p.Ranking.Signals,
				"policyVersion": p.Ranking.PolicyVersion,
			}
		}

		paths = append(paths, pathInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"targetSymbol":    resp.TargetSymbol,
		"paths":           paths,
		"totalPathsFound": resp.TotalPathsFound,
		"confidence":      resp.Confidence,
		"confidenceBasis": resp.ConfidenceBasis,
	}

	if len(resp.Limitations) > 0 {
		result["limitations"] = resp.Limitations
	}

	if resp.Resolved != nil {
		result["resolved"] = map[string]interface{}{
			"symbolId":     resp.Resolved.SymbolId,
			"resolvedFrom": resp.Resolved.ResolvedFrom,
			"confidence":   resp.Resolved.Confidence,
		}
	}

	if resp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     resp.Provenance.RepoStateId,
			"repoStateDirty":  resp.Provenance.RepoStateDirty,
			"queryDurationMs": resp.Provenance.QueryDurationMs,
		}
	}

	if len(resp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(resp.Drilldowns))
		for _, d := range resp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytesTrace, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesTrace), nil
}

// toolSummarizeDiff handles the summarizeDiff tool call
func (s *MCPServer) toolSummarizeDiff(params map[string]interface{}) (interface{}, error) {
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

	resp, err := s.engine.SummarizeDiff(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("summarizeDiff failed: %w", err)
	}

	// Build changed files response
	changedFiles := make([]map[string]interface{}, 0, len(resp.ChangedFiles))
	for _, f := range resp.ChangedFiles {
		fileInfo := map[string]interface{}{
			"filePath":   f.FilePath,
			"changeType": f.ChangeType,
			"additions":  f.Additions,
			"deletions":  f.Deletions,
			"riskLevel":  f.RiskLevel,
		}
		if f.OldPath != "" {
			fileInfo["oldPath"] = f.OldPath
		}
		if f.Language != "" {
			fileInfo["language"] = f.Language
		}
		if f.Role != "" {
			fileInfo["role"] = f.Role
		}
		changedFiles = append(changedFiles, fileInfo)
	}

	// Build symbols affected response
	symbolsAffected := make([]map[string]interface{}, 0, len(resp.SymbolsAffected))
	for _, sym := range resp.SymbolsAffected {
		symInfo := map[string]interface{}{
			"name":        sym.Name,
			"kind":        sym.Kind,
			"filePath":    sym.FilePath,
			"changeType":  sym.ChangeType,
			"isPublicApi": sym.IsPublicAPI,
		}
		if sym.SymbolId != "" {
			symInfo["symbolId"] = sym.SymbolId
		}
		if sym.IsEntrypoint {
			symInfo["isEntrypoint"] = true
		}
		symbolsAffected = append(symbolsAffected, symInfo)
	}

	// Build risk signals response
	riskSignals := make([]map[string]interface{}, 0, len(resp.RiskSignals))
	for _, r := range resp.RiskSignals {
		riskSignals = append(riskSignals, map[string]interface{}{
			"type":        r.Type,
			"severity":    r.Severity,
			"filePath":    r.FilePath,
			"description": r.Description,
			"confidence":  r.Confidence,
		})
	}

	// Build suggested tests response
	suggestedTests := make([]map[string]interface{}, 0, len(resp.SuggestedTests))
	for _, t := range resp.SuggestedTests {
		suggestedTests = append(suggestedTests, map[string]interface{}{
			"testPath": t.TestPath,
			"reason":   t.Reason,
			"priority": t.Priority,
		})
	}

	// Build commits response
	commits := make([]map[string]interface{}, 0, len(resp.Commits))
	for _, c := range resp.Commits {
		commitInfo := map[string]interface{}{
			"hash": c.Hash,
		}
		if c.Message != "" {
			commitInfo["message"] = c.Message
		}
		if c.Author != "" {
			commitInfo["author"] = c.Author
		}
		if c.Timestamp != "" {
			commitInfo["timestamp"] = c.Timestamp
		}
		commits = append(commits, commitInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"selector": map[string]interface{}{
			"type":  resp.Selector.Type,
			"value": resp.Selector.Value,
		},
		"changedFiles":    changedFiles,
		"symbolsAffected": symbolsAffected,
		"riskSignals":     riskSignals,
		"suggestedTests":  suggestedTests,
		"summary": map[string]interface{}{
			"oneLiner":     resp.Summary.OneLiner,
			"keyChanges":   resp.Summary.KeyChanges,
			"riskOverview": resp.Summary.RiskOverview,
		},
		"commits":         commits,
		"confidence":      resp.Confidence,
		"confidenceBasis": resp.ConfidenceBasis,
	}

	if len(resp.Limitations) > 0 {
		result["limitations"] = resp.Limitations
	}

	if resp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     resp.Provenance.RepoStateId,
			"repoStateDirty":  resp.Provenance.RepoStateDirty,
			"queryDurationMs": resp.Provenance.QueryDurationMs,
		}
	}

	if len(resp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(resp.Drilldowns))
		for _, d := range resp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytesDiff, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesDiff), nil
}

// toolGetHotspots handles the getHotspots tool call
func (s *MCPServer) toolGetHotspots(params map[string]interface{}) (interface{}, error) {
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

	resp, err := s.engine.GetHotspots(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("getHotspots failed: %w", err)
	}

	// Build hotspots response
	hotspots := make([]map[string]interface{}, 0, len(resp.Hotspots))
	for _, h := range resp.Hotspots {
		hotspotInfo := map[string]interface{}{
			"filePath":  h.FilePath,
			"recency":   h.Recency,
			"riskLevel": h.RiskLevel,
			"churn": map[string]interface{}{
				"changeCount":    h.Churn.ChangeCount,
				"authorCount":    h.Churn.AuthorCount,
				"averageChanges": h.Churn.AverageChanges,
				"score":          h.Churn.Score,
			},
		}
		if h.Role != "" {
			hotspotInfo["role"] = h.Role
		}
		if h.Language != "" {
			hotspotInfo["language"] = h.Language
		}
		if h.Coupling != nil {
			hotspotInfo["coupling"] = map[string]interface{}{
				"dependentCount":  h.Coupling.DependentCount,
				"dependencyCount": h.Coupling.DependencyCount,
				"score":           h.Coupling.Score,
			}
		}
		if h.Ranking != nil {
			hotspotInfo["ranking"] = map[string]interface{}{
				"score":         h.Ranking.Score,
				"signals":       h.Ranking.Signals,
				"policyVersion": h.Ranking.PolicyVersion,
			}
		}
		hotspots = append(hotspots, hotspotInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"hotspots":        hotspots,
		"totalCount":      resp.TotalCount,
		"timeWindow":      resp.TimeWindow,
		"confidence":      resp.Confidence,
		"confidenceBasis": resp.ConfidenceBasis,
	}

	if len(resp.Limitations) > 0 {
		result["limitations"] = resp.Limitations
	}

	if resp.Provenance != nil {
		result["provenance"] = map[string]interface{}{
			"repoStateId":     resp.Provenance.RepoStateId,
			"repoStateDirty":  resp.Provenance.RepoStateDirty,
			"queryDurationMs": resp.Provenance.QueryDurationMs,
		}
	}

	if len(resp.Drilldowns) > 0 {
		drilldowns := make([]map[string]interface{}, 0, len(resp.Drilldowns))
		for _, d := range resp.Drilldowns {
			drilldowns = append(drilldowns, map[string]interface{}{
				"label":          d.Label,
				"query":          d.Query,
				"relevanceScore": d.RelevanceScore,
			})
		}
		result["drilldowns"] = drilldowns
	}

	jsonBytesHotspots, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesHotspots), nil
}

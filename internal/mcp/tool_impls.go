package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ckb/internal/complexity"
	"ckb/internal/jobs"
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
		if h.Complexity != nil {
			hotspotInfo["complexity"] = map[string]interface{}{
				"cyclomatic":    h.Complexity.Cyclomatic,
				"cognitive":     h.Complexity.Cognitive,
				"functionCount": h.Complexity.FunctionCount,
				"score":         h.Complexity.Score,
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

// toolExplainPath handles the explainPath tool call
func (s *MCPServer) toolExplainPath(params map[string]interface{}) (interface{}, error) {
	ctx := context.Background()

	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return nil, fmt.Errorf("missing or invalid 'filePath' parameter")
	}

	opts := query.ExplainPathOptions{
		FilePath: filePath,
	}

	if contextHint, ok := params["contextHint"].(string); ok {
		opts.ContextHint = contextHint
	}

	resp, err := s.engine.ExplainPath(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("explainPath failed: %w", err)
	}

	// Build classification basis response
	classificationBasis := make([]map[string]interface{}, 0, len(resp.ClassificationBasis))
	for _, b := range resp.ClassificationBasis {
		classificationBasis = append(classificationBasis, map[string]interface{}{
			"type":       b.Type,
			"signal":     b.Signal,
			"confidence": b.Confidence,
		})
	}

	// Build related paths response
	relatedPaths := make([]map[string]interface{}, 0, len(resp.RelatedPaths))
	for _, r := range resp.RelatedPaths {
		relatedPaths = append(relatedPaths, map[string]interface{}{
			"path":     r.Path,
			"relation": r.Relation,
		})
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"filePath":            resp.FilePath,
		"role":                resp.Role,
		"roleExplanation":     resp.RoleExplanation,
		"classificationBasis": classificationBasis,
		"relatedPaths":        relatedPaths,
		"confidence":          resp.Confidence,
		"confidenceBasis":     resp.ConfidenceBasis,
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

	jsonBytesPath, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesPath), nil
}

// toolListKeyConcepts handles the listKeyConcepts tool call
func (s *MCPServer) toolListKeyConcepts(params map[string]interface{}) (interface{}, error) {
	ctx := context.Background()

	limit := 12
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	resp, err := s.engine.ListKeyConcepts(ctx, query.ListKeyConceptsOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("listKeyConcepts failed: %w", err)
	}

	// Build concepts response
	concepts := make([]map[string]interface{}, 0, len(resp.Concepts))
	for _, c := range resp.Concepts {
		conceptInfo := map[string]interface{}{
			"name":        c.Name,
			"category":    c.Category,
			"occurrences": c.Occurrences,
			"description": c.Description,
		}
		if len(c.Files) > 0 {
			conceptInfo["files"] = c.Files
		}
		if len(c.Symbols) > 0 {
			conceptInfo["symbols"] = c.Symbols
		}
		if c.Ranking != nil {
			conceptInfo["ranking"] = map[string]interface{}{
				"score":         c.Ranking.Score,
				"signals":       c.Ranking.Signals,
				"policyVersion": c.Ranking.PolicyVersion,
			}
		}
		concepts = append(concepts, conceptInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"concepts":        concepts,
		"totalFound":      resp.TotalFound,
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

	jsonBytesConcepts, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesConcepts), nil
}

// toolRecentlyRelevant handles the recentlyRelevant tool call
func (s *MCPServer) toolRecentlyRelevant(params map[string]interface{}) (interface{}, error) {
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

	resp, err := s.engine.RecentlyRelevant(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("recentlyRelevant failed: %w", err)
	}

	// Build items response
	items := make([]map[string]interface{}, 0, len(resp.Items))
	for _, item := range resp.Items {
		itemInfo := map[string]interface{}{
			"type":         item.Type,
			"name":         item.Name,
			"changeCount":  item.ChangeCount,
			"lastModified": item.LastModified,
		}
		if item.Path != "" {
			itemInfo["path"] = item.Path
		}
		if item.SymbolId != "" {
			itemInfo["symbolId"] = item.SymbolId
		}
		if len(item.Authors) > 0 {
			itemInfo["authors"] = item.Authors
		}
		if item.Ranking != nil {
			itemInfo["ranking"] = map[string]interface{}{
				"score":         item.Ranking.Score,
				"signals":       item.Ranking.Signals,
				"policyVersion": item.Ranking.PolicyVersion,
			}
		}
		items = append(items, itemInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"items":           items,
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

	jsonBytesRecent, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesRecent), nil
}

// toolRefreshArchitecture handles the refreshArchitecture tool call (v6.0)
func (s *MCPServer) toolRefreshArchitecture(params map[string]interface{}) (interface{}, error) {
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

	s.logger.Debug("Executing refreshArchitecture", map[string]interface{}{
		"scope":  scope,
		"force":  force,
		"dryRun": dryRun,
		"async":  async,
	})

	opts := query.RefreshArchitectureOptions{
		Scope:  scope,
		Force:  force,
		DryRun: dryRun,
		Async:  async,
	}

	resp, err := s.engine.RefreshArchitecture(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("refreshArchitecture failed: %w", err)
	}

	// Build changes response
	changes := make(map[string]interface{})
	if resp.Changes != nil {
		if resp.Changes.ModulesUpdated > 0 {
			changes["modulesUpdated"] = resp.Changes.ModulesUpdated
		}
		if resp.Changes.ModulesCreated > 0 {
			changes["modulesCreated"] = resp.Changes.ModulesCreated
		}
		if resp.Changes.OwnershipUpdated > 0 {
			changes["ownershipUpdated"] = resp.Changes.OwnershipUpdated
		}
		if resp.Changes.HotspotsUpdated > 0 {
			changes["hotspotsUpdated"] = resp.Changes.HotspotsUpdated
		}
		if resp.Changes.ResponsibilitiesUpdated > 0 {
			changes["responsibilitiesUpdated"] = resp.Changes.ResponsibilitiesUpdated
		}
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"status":     resp.Status,
		"scope":      resp.Scope,
		"changes":    changes,
		"durationMs": resp.DurationMs,
	}

	if resp.DryRun {
		result["dryRun"] = true
	}

	if resp.JobId != "" {
		result["jobId"] = resp.JobId
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

	jsonBytesRefresh, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesRefresh), nil
}

// toolGetOwnership handles the getOwnership tool call (v6.0)
func (s *MCPServer) toolGetOwnership(params map[string]interface{}) (interface{}, error) {
	ctx := context.Background()

	// Parse path (required)
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("missing or invalid 'path' parameter")
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

	s.logger.Debug("Executing getOwnership", map[string]interface{}{
		"path":           path,
		"includeBlame":   includeBlame,
		"includeHistory": includeHistory,
	})

	opts := query.GetOwnershipOptions{
		Path:           path,
		IncludeBlame:   includeBlame,
		IncludeHistory: includeHistory,
	}

	resp, err := s.engine.GetOwnership(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("getOwnership failed: %w", err)
	}

	// Build owners response
	owners := make([]map[string]interface{}, 0, len(resp.Owners))
	for _, o := range resp.Owners {
		ownerInfo := map[string]interface{}{
			"id":         o.ID,
			"type":       o.Type,
			"scope":      o.Scope,
			"source":     o.Source,
			"confidence": o.Confidence,
		}
		owners = append(owners, ownerInfo)
	}

	// Build blame contributors if present
	var blameContributors []map[string]interface{}
	if resp.BlameOwnership != nil {
		blameContributors = make([]map[string]interface{}, 0, len(resp.BlameOwnership.Contributors))
		for _, c := range resp.BlameOwnership.Contributors {
			contribInfo := map[string]interface{}{
				"author":     c.Author,
				"email":      c.Email,
				"lineCount":  c.LineCount,
				"percentage": c.Percentage,
			}
			blameContributors = append(blameContributors, contribInfo)
		}
	}

	// Build history events if present
	var history []map[string]interface{}
	if len(resp.History) > 0 {
		history = make([]map[string]interface{}, 0, len(resp.History))
		for _, h := range resp.History {
			historyInfo := map[string]interface{}{
				"pattern":    h.Pattern,
				"ownerId":    h.OwnerID,
				"event":      h.Event,
				"recordedAt": h.RecordedAt,
			}
			if h.Reason != "" {
				historyInfo["reason"] = h.Reason
			}
			history = append(history, historyInfo)
		}
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"path":            resp.Path,
		"owners":          owners,
		"confidence":      resp.Confidence,
		"confidenceBasis": resp.ConfidenceBasis,
	}

	if blameContributors != nil {
		result["blameOwnership"] = map[string]interface{}{
			"totalLines":   resp.BlameOwnership.TotalLines,
			"contributors": blameContributors,
		}
	}

	if history != nil {
		result["history"] = history
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

	jsonBytesOwnership, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesOwnership), nil
}

// toolGetModuleResponsibilities handles the getModuleResponsibilities tool call (v6.0)
func (s *MCPServer) toolGetModuleResponsibilities(params map[string]interface{}) (interface{}, error) {
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

	s.logger.Debug("Executing getModuleResponsibilities", map[string]interface{}{
		"moduleId":     moduleId,
		"includeFiles": includeFiles,
		"limit":        limit,
	})

	opts := query.GetModuleResponsibilitiesOptions{
		ModuleId:     moduleId,
		IncludeFiles: includeFiles,
		Limit:        limit,
	}

	resp, err := s.engine.GetModuleResponsibilities(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("getModuleResponsibilities failed: %w", err)
	}

	// Build modules response
	modules := make([]map[string]interface{}, 0, len(resp.Modules))
	for _, m := range resp.Modules {
		moduleInfo := map[string]interface{}{
			"moduleId":   m.ModuleId,
			"name":       m.Name,
			"path":       m.Path,
			"summary":    m.Summary,
			"source":     m.Source,
			"confidence": m.Confidence,
		}
		if len(m.Capabilities) > 0 {
			moduleInfo["capabilities"] = m.Capabilities
		}
		if len(m.Files) > 0 {
			files := make([]map[string]interface{}, 0, len(m.Files))
			for _, f := range m.Files {
				files = append(files, map[string]interface{}{
					"path":       f.Path,
					"summary":    f.Summary,
					"confidence": f.Confidence,
				})
			}
			moduleInfo["files"] = files
		}
		modules = append(modules, moduleInfo)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion":    resp.CkbVersion,
			"schemaVersion": resp.SchemaVersion,
			"tool":          resp.Tool,
		},
		"modules":         modules,
		"totalCount":      resp.TotalCount,
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

	jsonBytesResp, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesResp), nil
}

// toolRecordDecision handles the recordDecision tool call (v6.0)
func (s *MCPServer) toolRecordDecision(params map[string]interface{}) (interface{}, error) {
	// Parse required parameters
	title, ok := params["title"].(string)
	if !ok || title == "" {
		return nil, fmt.Errorf("missing or invalid 'title' parameter")
	}

	context_, ok := params["context"].(string)
	if !ok || context_ == "" {
		return nil, fmt.Errorf("missing or invalid 'context' parameter")
	}

	decision, ok := params["decision"].(string)
	if !ok || decision == "" {
		return nil, fmt.Errorf("missing or invalid 'decision' parameter")
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
		return nil, fmt.Errorf("missing or invalid 'consequences' parameter")
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

	s.logger.Debug("Executing recordDecision", map[string]interface{}{
		"title":           title,
		"affectedModules": affectedModules,
		"author":          author,
		"status":          status,
	})

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

	resp, err := s.engine.RecordDecision(input)
	if err != nil {
		return nil, fmt.Errorf("recordDecision failed: %w", err)
	}

	result := map[string]interface{}{
		"id":       resp.Decision.ID,
		"title":    resp.Decision.Title,
		"status":   resp.Decision.Status,
		"filePath": resp.Decision.FilePath,
		"date":     resp.Decision.Date.Format("2006-01-02"),
		"source":   resp.Source,
	}

	if resp.Decision.Author != "" {
		result["author"] = resp.Decision.Author
	}

	if len(resp.Decision.AffectedModules) > 0 {
		result["affectedModules"] = resp.Decision.AffectedModules
	}

	jsonBytesDecision, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesDecision), nil
}

// toolGetDecisions handles the getDecisions tool call (v6.0)
func (s *MCPServer) toolGetDecisions(params map[string]interface{}) (interface{}, error) {
	// Check if specific ID is requested
	if id, ok := params["id"].(string); ok && id != "" {
		s.logger.Debug("Executing getDecisions (single)", map[string]interface{}{
			"id": id,
		})

		resp, err := s.engine.GetDecision(id)
		if err != nil {
			return nil, fmt.Errorf("getDecision failed: %w", err)
		}

		result := map[string]interface{}{
			"id":           resp.Decision.ID,
			"title":        resp.Decision.Title,
			"status":       resp.Decision.Status,
			"context":      resp.Decision.Context,
			"decision":     resp.Decision.Decision,
			"consequences": resp.Decision.Consequences,
			"date":         resp.Decision.Date.Format("2006-01-02"),
			"filePath":     resp.Decision.FilePath,
			"source":       resp.Source,
		}

		if resp.Decision.Author != "" {
			result["author"] = resp.Decision.Author
		}

		if len(resp.Decision.AffectedModules) > 0 {
			result["affectedModules"] = resp.Decision.AffectedModules
		}

		if len(resp.Decision.Alternatives) > 0 {
			result["alternatives"] = resp.Decision.Alternatives
		}

		if resp.Decision.SupersededBy != "" {
			result["supersededBy"] = resp.Decision.SupersededBy
		}

		jsonBytesSingle, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, err
		}

		return string(jsonBytesSingle), nil
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

	s.logger.Debug("Executing getDecisions (list)", map[string]interface{}{
		"status":   queryOpts.Status,
		"moduleId": queryOpts.ModuleID,
		"search":   queryOpts.Search,
		"limit":    queryOpts.Limit,
	})

	resp, err := s.engine.GetDecisions(queryOpts)
	if err != nil {
		return nil, fmt.Errorf("getDecisions failed: %w", err)
	}

	// Build decisions response
	decisions := make([]map[string]interface{}, 0, len(resp.Decisions))
	for _, d := range resp.Decisions {
		decisionInfo := map[string]interface{}{
			"id":       d.ID,
			"title":    d.Title,
			"status":   d.Status,
			"date":     d.Date.Format("2006-01-02"),
			"filePath": d.FilePath,
		}

		if d.Author != "" {
			decisionInfo["author"] = d.Author
		}

		if len(d.AffectedModules) > 0 {
			decisionInfo["affectedModules"] = d.AffectedModules
		}

		decisions = append(decisions, decisionInfo)
	}

	result := map[string]interface{}{
		"decisions":  decisions,
		"totalCount": resp.Total,
	}

	if resp.Query != nil {
		query := map[string]interface{}{}
		if resp.Query.Status != "" {
			query["status"] = resp.Query.Status
		}
		if resp.Query.ModuleID != "" {
			query["moduleId"] = resp.Query.ModuleID
		}
		if resp.Query.Search != "" {
			query["search"] = resp.Query.Search
		}
		if len(query) > 0 {
			result["query"] = query
		}
	}

	jsonBytesDecisions, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesDecisions), nil
}

// toolAnnotateModule handles the annotateModule tool call (v6.0)
func (s *MCPServer) toolAnnotateModule(params map[string]interface{}) (interface{}, error) {
	// Parse required parameters
	moduleId, ok := params["moduleId"].(string)
	if !ok || moduleId == "" {
		return nil, fmt.Errorf("missing or invalid 'moduleId' parameter")
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

	s.logger.Debug("Executing annotateModule", map[string]interface{}{
		"moduleId":       moduleId,
		"responsibility": responsibility,
		"capabilities":   capabilities,
		"tags":           tags,
	})

	input := &query.AnnotateModuleInput{
		ModuleId:       moduleId,
		Responsibility: responsibility,
		Capabilities:   capabilities,
		Tags:           tags,
		PublicPaths:    publicPaths,
		InternalPaths:  internalPaths,
	}

	resp, err := s.engine.AnnotateModule(input)
	if err != nil {
		return nil, fmt.Errorf("annotateModule failed: %w", err)
	}

	result := map[string]interface{}{
		"moduleId": resp.ModuleId,
		"updated":  resp.Updated,
		"created":  resp.Created,
	}

	if resp.Responsibility != "" {
		result["responsibility"] = resp.Responsibility
	}

	if len(resp.Capabilities) > 0 {
		result["capabilities"] = resp.Capabilities
	}

	if len(resp.Tags) > 0 {
		result["tags"] = resp.Tags
	}

	if resp.Boundaries != nil {
		boundaries := map[string]interface{}{}
		if len(resp.Boundaries.Public) > 0 {
			boundaries["public"] = resp.Boundaries.Public
		}
		if len(resp.Boundaries.Internal) > 0 {
			boundaries["internal"] = resp.Boundaries.Internal
		}
		if len(boundaries) > 0 {
			result["boundaries"] = boundaries
		}
	}

	jsonBytesAnnotate, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytesAnnotate), nil
}

// v6.1 Job management tools

// toolGetJobStatus handles the getJobStatus tool call
func (s *MCPServer) toolGetJobStatus(params map[string]interface{}) (interface{}, error) {
	// Parse jobId (required)
	jobId, ok := params["jobId"].(string)
	if !ok || jobId == "" {
		return nil, fmt.Errorf("missing or invalid 'jobId' parameter")
	}

	s.logger.Debug("Executing getJobStatus", map[string]interface{}{
		"jobId": jobId,
	})

	job, err := s.engine.GetJob(jobId)
	if err != nil {
		return nil, fmt.Errorf("getJobStatus failed: %w", err)
	}

	if job == nil {
		return nil, fmt.Errorf("job not found: %s", jobId)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion": "6.1",
			"tool":       "getJobStatus",
		},
		"job": map[string]interface{}{
			"id":        job.ID,
			"type":      job.Type,
			"status":    job.Status,
			"progress":  job.Progress,
			"createdAt": job.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}

	jobMap := result["job"].(map[string]interface{})

	if job.StartedAt != nil {
		jobMap["startedAt"] = job.StartedAt.Format("2006-01-02T15:04:05Z")
	}

	if job.CompletedAt != nil {
		jobMap["completedAt"] = job.CompletedAt.Format("2006-01-02T15:04:05Z")
	}

	if job.Error != "" {
		jobMap["error"] = job.Error
	}

	if job.Result != "" {
		// Parse result JSON if possible
		var resultData interface{}
		if err := json.Unmarshal([]byte(job.Result), &resultData); err == nil {
			jobMap["result"] = resultData
		} else {
			jobMap["result"] = job.Result
		}
	}

	if job.IsTerminal() {
		jobMap["duration"] = job.Duration().String()
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolListJobs handles the listJobs tool call
func (s *MCPServer) toolListJobs(params map[string]interface{}) (interface{}, error) {
	s.logger.Debug("Executing listJobs", params)

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

	resp, err := s.engine.ListJobs(opts)
	if err != nil {
		return nil, fmt.Errorf("listJobs failed: %w", err)
	}

	jobsList := make([]map[string]interface{}, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobMap := map[string]interface{}{
			"id":        j.ID,
			"type":      j.Type,
			"status":    j.Status,
			"progress":  j.Progress,
			"createdAt": j.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}

		if j.CompletedAt != nil {
			jobMap["completedAt"] = j.CompletedAt.Format("2006-01-02T15:04:05Z")
		}

		if j.Error != "" {
			jobMap["error"] = j.Error
		}

		jobsList = append(jobsList, jobMap)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion": "6.1",
			"tool":       "listJobs",
		},
		"jobs":       jobsList,
		"totalCount": resp.TotalCount,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolCancelJob handles the cancelJob tool call
func (s *MCPServer) toolCancelJob(params map[string]interface{}) (interface{}, error) {
	// Parse jobId (required)
	jobId, ok := params["jobId"].(string)
	if !ok || jobId == "" {
		return nil, fmt.Errorf("missing or invalid 'jobId' parameter")
	}

	s.logger.Debug("Executing cancelJob", map[string]interface{}{
		"jobId": jobId,
	})

	err := s.engine.CancelJob(jobId)
	if err != nil {
		return nil, fmt.Errorf("cancelJob failed: %w", err)
	}

	result := map[string]interface{}{
		"meta": map[string]interface{}{
			"ckbVersion": "6.1",
			"tool":       "cancelJob",
		},
		"jobId":  jobId,
		"status": "cancelled",
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolSummarizePr handles the summarizePr tool call
func (s *MCPServer) toolSummarizePr(params map[string]interface{}) (interface{}, error) {
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

	s.logger.Debug("Executing summarizePr", map[string]interface{}{
		"baseBranch":       baseBranch,
		"headBranch":       headBranch,
		"includeOwnership": includeOwnership,
	})

	opts := query.SummarizePROptions{
		BaseBranch:       baseBranch,
		HeadBranch:       headBranch,
		IncludeOwnership: includeOwnership,
	}

	resp, err := s.engine.SummarizePR(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("summarizePr failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetOwnershipDrift handles the getOwnershipDrift tool call
func (s *MCPServer) toolGetOwnershipDrift(params map[string]interface{}) (interface{}, error) {
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

	s.logger.Debug("Executing getOwnershipDrift", map[string]interface{}{
		"scope":     scope,
		"threshold": threshold,
		"limit":     limit,
	})

	opts := query.OwnershipDriftOptions{
		Scope:     scope,
		Threshold: threshold,
		Limit:     limit,
	}

	resp, err := s.engine.GetOwnershipDrift(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("getOwnershipDrift failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetFileComplexity handles the getFileComplexity tool call
func (s *MCPServer) toolGetFileComplexity(params map[string]interface{}) (interface{}, error) {
	ctx := context.Background()

	// Parse filePath (required)
	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return nil, fmt.Errorf("filePath is required")
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

	s.logger.Debug("Executing getFileComplexity", map[string]interface{}{
		"filePath":         filePath,
		"includeFunctions": includeFunctions,
		"sortBy":           sortBy,
		"limit":            limit,
	})

	// Resolve the file path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(s.engine.GetRepoRoot(), filePath)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	// Analyze the file
	analyzer := complexity.NewAnalyzer()
	result, err := analyzer.AnalyzeFile(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}

	// Check for analysis error
	if result.Error != "" {
		return map[string]interface{}{
			"path":  filePath,
			"error": result.Error,
		}, nil
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

	jsonBytes, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

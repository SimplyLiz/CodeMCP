package mcp

import (
	"fmt"
	"time"

	"ckb/internal/docs"
	"ckb/internal/envelope"
)

// v7.3 Doc-Symbol Linking tool implementations

// toolGetDocsForSymbol finds documentation that references a symbol
func (s *MCPServer) toolGetDocsForSymbol(params map[string]interface{}) (*envelope.Response, error) {
	symbol, ok := params["symbol"].(string)
	if !ok || symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	limit := 10
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}

	refs, err := s.engine().GetDocsForSymbol(symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find docs for symbol: %w", err)
	}

	// Convert to response format
	result := make([]map[string]interface{}, 0, len(refs))
	for _, ref := range refs {
		item := map[string]interface{}{
			"docPath":    ref.DocPath,
			"rawText":    ref.RawText,
			"line":       ref.Line,
			"resolution": string(ref.Resolution),
		}
		if ref.Context != "" {
			item["context"] = truncateString(ref.Context, 100)
		}
		if ref.SymbolID != nil {
			item["symbolId"] = *ref.SymbolID
		}
		result = append(result, item)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"symbol":     symbol,
			"references": result,
			"count":      len(result),
		}).
		Build(), nil
}

// toolGetSymbolsInDoc lists all symbol references found in a documentation file
func (s *MCPServer) toolGetSymbolsInDoc(params map[string]interface{}) (*envelope.Response, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	doc, err := s.engine().GetDocumentInfo(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get document info: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s (run 'ckb docs index' first)", path)
	}

	// Convert references to response format
	symbols := make([]map[string]interface{}, 0, len(doc.References))
	for _, ref := range doc.References {
		item := map[string]interface{}{
			"rawText":    ref.RawText,
			"line":       ref.Line,
			"resolution": string(ref.Resolution),
		}
		if ref.SymbolID != nil {
			item["symbolId"] = *ref.SymbolID
		}
		if ref.SymbolName != "" {
			item["symbolName"] = ref.SymbolName
		}
		if len(ref.Candidates) > 0 {
			item["candidates"] = ref.Candidates
		}
		symbols = append(symbols, item)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"path":        doc.Path,
			"type":        string(doc.Type),
			"title":       doc.Title,
			"symbols":     symbols,
			"symbolCount": len(symbols),
			"modules":     doc.Modules,
			"lastIndexed": doc.LastIndexed.Format(time.RFC3339),
		}).
		Build(), nil
}

// toolGetDocsForModule finds documentation explicitly linked to a module
func (s *MCPServer) toolGetDocsForModule(params map[string]interface{}) (*envelope.Response, error) {
	moduleID, ok := params["moduleId"].(string)
	if !ok || moduleID == "" {
		return nil, fmt.Errorf("moduleId is required")
	}

	docsList, err := s.engine().GetDocsForModule(moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to find docs for module: %w", err)
	}

	// Convert to response format
	result := make([]map[string]interface{}, 0, len(docsList))
	for _, doc := range docsList {
		result = append(result, map[string]interface{}{
			"path":  doc.Path,
			"type":  string(doc.Type),
			"title": doc.Title,
		})
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"moduleId": moduleID,
			"docs":     result,
			"count":    len(result),
		}).
		Build(), nil
}

// toolCheckDocStaleness checks documentation for stale symbol references
func (s *MCPServer) toolCheckDocStaleness(params map[string]interface{}) (*envelope.Response, error) {
	path, _ := params["path"].(string)
	checkAll, _ := params["all"].(bool)

	var reports []docs.StalenessReport
	var err error

	if path != "" {
		// Check single document
		report, e := s.engine().CheckDocStaleness(path)
		if e != nil {
			return nil, fmt.Errorf("failed to check staleness: %w", e)
		}
		if report != nil {
			reports = []docs.StalenessReport{*report}
		}
	} else if checkAll {
		// Check all documents
		reports, err = s.engine().CheckAllDocsStaleness()
		if err != nil {
			return nil, fmt.Errorf("failed to check staleness: %w", err)
		}
	} else {
		return nil, fmt.Errorf("specify a path or use all=true")
	}

	// Convert to response format
	result := make([]map[string]interface{}, 0)
	totalStale := 0
	for _, r := range reports {
		if len(r.Stale) == 0 {
			continue
		}
		staleRefs := make([]map[string]interface{}, 0, len(r.Stale))
		for _, staleRef := range r.Stale {
			item := map[string]interface{}{
				"rawText": staleRef.RawText,
				"line":    staleRef.Line,
				"reason":  string(staleRef.Reason),
				"message": staleRef.Message,
			}
			if len(staleRef.Suggestions) > 0 {
				// Limit suggestions
				suggestions := staleRef.Suggestions
				if len(suggestions) > 5 {
					suggestions = suggestions[:5]
				}
				item["suggestions"] = suggestions
			}
			staleRefs = append(staleRefs, item)
		}
		result = append(result, map[string]interface{}{
			"docPath":         r.DocPath,
			"totalReferences": r.TotalReferences,
			"valid":           r.Valid,
			"stale":           staleRefs,
		})
		totalStale += len(r.Stale)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"reports":    result,
			"totalStale": totalStale,
		}).
		Build(), nil
}

// toolIndexDocs scans and indexes documentation for symbol references
func (s *MCPServer) toolIndexDocs(params map[string]interface{}) (*envelope.Response, error) {
	force := false
	if v, ok := params["force"].(bool); ok {
		force = v
	}

	start := time.Now()
	stats, err := s.engine().IndexDocs(force)
	if err != nil {
		return nil, fmt.Errorf("failed to index docs: %w", err)
	}

	return OperationalResponse(map[string]interface{}{
		"docsIndexed":     stats.DocsIndexed,
		"docsSkipped":     stats.DocsSkipped,
		"referencesFound": stats.ReferencesFound,
		"resolved":        stats.Resolved,
		"ambiguous":       stats.Ambiguous,
		"missing":         stats.Missing,
		"durationMs":      time.Since(start).Milliseconds(),
	}), nil
}

// toolGetDocCoverage returns documentation coverage statistics
func (s *MCPServer) toolGetDocCoverage(params map[string]interface{}) (*envelope.Response, error) {
	exportedOnly := false
	if v, ok := params["exportedOnly"].(bool); ok {
		exportedOnly = v
	}

	topN := 10
	if v, ok := params["topN"].(float64); ok {
		topN = int(v)
	}

	report, err := s.engine().GetDocCoverage(exportedOnly, topN)
	if err != nil {
		return nil, fmt.Errorf("failed to get doc coverage: %w", err)
	}

	// Convert top undocumented to response format
	topUndocumented := make([]map[string]interface{}, 0, len(report.TopUndocumented))
	for _, u := range report.TopUndocumented {
		topUndocumented = append(topUndocumented, map[string]interface{}{
			"symbolId":   u.SymbolID,
			"name":       u.Name,
			"centrality": u.Centrality,
		})
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"totalSymbols":    report.TotalSymbols,
			"documented":      report.Documented,
			"undocumented":    report.Undocumented,
			"coveragePercent": report.CoveragePercent,
			"topUndocumented": topUndocumented,
		}).
		Build(), nil
}

// truncateString truncates a string to max length, adding ellipsis if needed
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

package mcp

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// toolListSymbols lists all symbols in a scope with complexity metrics.
func (s *MCPServer) toolListSymbols(params map[string]interface{}) (*envelope.Response, error) {
	timer := NewWideResultTimer()
	ctx := context.Background()

	scope := ""
	if v, ok := params["scope"].(string); ok {
		scope = v
	}

	var kinds []string
	if v, ok := params["kinds"].([]interface{}); ok {
		for _, k := range v {
			if ks, ok := k.(string); ok {
				kinds = append(kinds, ks)
			}
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"function", "method"}
	}

	minLines := 3
	if v, ok := params["minLines"].(float64); ok && v >= 0 {
		minLines = int(v)
	}

	minComplexity := 0
	if v, ok := params["minComplexity"].(float64); ok && v >= 0 {
		minComplexity = int(v)
	}

	sortBy := "complexity"
	if v, ok := params["sortBy"].(string); ok && v != "" {
		sortBy = v
	}

	limit := 50
	if v, ok := params["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 200 {
		limit = 200
	}

	// Use searchSymbols with empty query to list all symbols in scope.
	// Exclude struct fields (#) by default — listSymbols is for behavioral
	// analysis (functions/types), not data shape (use getSymbol for that).
	searchResp, err := s.engine().SearchSymbols(ctx, query.SearchSymbolsOptions{
		Query:           "",
		Scope:           scope,
		Kinds:           kinds,
		Limit:           limit * 5, // Request more to survive filtering
		ExcludePatterns: []string{"#"},
	})
	if err != nil {
		return nil, err
	}

	// Filter by minLines and minComplexity
	var filtered []map[string]interface{}
	for _, sym := range searchResp.Symbols {
		// Skip symbols without body ranges when minLines is set
		if minLines > 0 && sym.Lines > 0 && sym.Lines < minLines {
			continue
		}
		// Skip anonymous/unknown symbols
		if sym.Name == "<anonymous>" || sym.Name == "<unknown>" || sym.Name == "" {
			continue
		}
		if minComplexity > 0 && sym.Cyclomatic < minComplexity {
			continue
		}

		entry := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
		}
		if sym.Location != nil {
			loc := map[string]interface{}{
				"fileId":    sym.Location.FileId,
				"startLine": sym.Location.StartLine,
			}
			if sym.Location.EndLine > 0 {
				loc["endLine"] = sym.Location.EndLine
			}
			entry["location"] = loc
		}
		if sym.Lines > 0 {
			entry["lines"] = sym.Lines
		}
		if sym.Cyclomatic > 0 {
			entry["cyclomatic"] = sym.Cyclomatic
			entry["cognitive"] = sym.Cognitive
		}
		if sym.Visibility != nil {
			entry["visibility"] = sym.Visibility.Visibility
		}
		if sym.ModuleId != "" {
			entry["moduleId"] = sym.ModuleId
		}
		filtered = append(filtered, entry)
	}

	// Sort
	switch sortBy {
	case "complexity":
		sort.Slice(filtered, func(i, j int) bool {
			ci, _ := filtered[i]["cyclomatic"].(int)
			cj, _ := filtered[j]["cyclomatic"].(int)
			return ci > cj
		})
	case "lines":
		sort.Slice(filtered, func(i, j int) bool {
			li, _ := filtered[i]["lines"].(int)
			lj, _ := filtered[j]["lines"].(int)
			return li > lj
		})
	case "name":
		sort.Slice(filtered, func(i, j int) bool {
			ni, _ := filtered[i]["name"].(string)
			nj, _ := filtered[j]["name"].(string)
			return ni < nj
		})
	}

	// Apply limit
	total := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	data := map[string]interface{}{
		"symbols":    filtered,
		"totalCount": total,
		"scope":      scope,
	}

	responseBytes := MeasureJSONSize(data)
	RecordWideResult(WideResultMetrics{
		ToolName:        "listSymbols",
		TotalResults:    total,
		ReturnedResults: len(filtered),
		ResponseBytes:   responseBytes,
		EstimatedTokens: EstimateTokens(responseBytes),
		ExecutionMs:     timer.ElapsedMs(),
	})

	return NewToolResponse().
		Data(data).
		WithProvenance(searchResp.Provenance).
		Build(), nil
}

// toolGetSymbolGraph returns call graph edges for multiple symbols in one call.
func (s *MCPServer) toolGetSymbolGraph(params map[string]interface{}) (*envelope.Response, error) {
	ctx := context.Background()

	var symbolIds []string
	if v, ok := params["symbolIds"].([]interface{}); ok {
		for _, id := range v {
			if idStr, ok := id.(string); ok && idStr != "" {
				symbolIds = append(symbolIds, idStr)
			}
		}
	}
	if len(symbolIds) == 0 {
		return NewToolResponse().Data(map[string]interface{}{
			"nodes": []interface{}{},
			"edges": []interface{}{},
		}).Build(), nil
	}
	if len(symbolIds) > 30 {
		symbolIds = symbolIds[:30]
	}

	depth := 1
	if v, ok := params["depth"].(float64); ok && v >= 1 && v <= 3 {
		depth = int(v)
	}

	direction := "both"
	if v, ok := params["direction"].(string); ok && v != "" {
		direction = v
	}

	// Fetch call graphs in parallel
	type graphResult struct {
		nodes []map[string]interface{}
		edges []map[string]interface{}
		err   error
	}
	results := make([]graphResult, len(symbolIds))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // concurrency limit

	for i, symId := range symbolIds {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cg, err := s.engine().GetCallGraph(ctx, query.CallGraphOptions{
				SymbolId:  id,
				Direction: direction,
				Depth:     depth,
			})
			if err != nil {
				results[idx] = graphResult{err: err}
				return
			}

			var nodes []map[string]interface{}
			var edges []map[string]interface{}
			for _, n := range cg.Nodes {
				node := map[string]interface{}{
					"symbolId": n.SymbolId,
					"name":     n.Name,
					"role":     n.Role,
				}
				if n.Location != nil {
					node["file"] = n.Location.FileId
					node["line"] = n.Location.StartLine
					if n.Location.EndLine > 0 {
						node["endLine"] = n.Location.EndLine
						node["lines"] = n.Location.EndLine - n.Location.StartLine + 1
					}
				}
				nodes = append(nodes, node)
			}
			for _, e := range cg.Edges {
				edges = append(edges, map[string]interface{}{
					"from": e.From,
					"to":   e.To,
				})
			}
			results[idx] = graphResult{nodes: nodes, edges: edges}
		}(i, symId)
	}
	wg.Wait()

	// Merge all results, deduplicating nodes by symbolId
	seenNodes := make(map[string]bool)
	seenEdges := make(map[string]bool)
	var allNodes []map[string]interface{}
	var allEdges []map[string]interface{}
	var errors []string

	for i, r := range results {
		if r.err != nil {
			errors = append(errors, symbolIds[i]+": "+r.err.Error())
			continue
		}
		for _, n := range r.nodes {
			id, _ := n["symbolId"].(string)
			if id != "" && !seenNodes[id] {
				seenNodes[id] = true
				allNodes = append(allNodes, n)
			}
		}
		for _, e := range r.edges {
			from, _ := e["from"].(string)
			to, _ := e["to"].(string)
			key := from + "→" + to
			if !seenEdges[key] {
				seenEdges[key] = true
				allEdges = append(allEdges, e)
			}
		}
	}

	// Enrich nodes with complexity from tree-sitter (reuse single analyzer)
	if complexity.IsAvailable() {
		analyzer := complexity.NewAnalyzer() // Single instance for all files in this call
		// Group nodes by file
		fileNodes := make(map[string][]int)
		for i, n := range allNodes {
			if f, ok := n["file"].(string); ok && f != "" {
				fileNodes[f] = append(fileNodes[f], i)
			}
		}
		repoRoot := s.engine().GetRepoRoot()
		for file, indices := range fileNodes {
			absPath := repoRoot + "/" + file
			fc, err := analyzer.AnalyzeFile(ctx, absPath)
			if err != nil || fc == nil || fc.Error != "" {
				continue
			}
			// Build lookup by (name, startLine)
			type key struct {
				name string
				line int
			}
			cxMap := make(map[key]struct{ cyc, cog int })
			for _, fn := range fc.Functions {
				cxMap[key{fn.Name, fn.StartLine}] = struct{ cyc, cog int }{fn.Cyclomatic, fn.Cognitive}
			}
			for _, idx := range indices {
				n := allNodes[idx]
				name, _ := n["name"].(string)
				line, _ := n["line"].(int)
				// Strip container prefix for matching
				if hashIdx := strings.LastIndex(name, "#"); hashIdx >= 0 {
					name = name[hashIdx+1:]
				}
				if cx, ok := cxMap[key{name, line}]; ok {
					n["cyclomatic"] = cx.cyc
					n["cognitive"] = cx.cog
				}
			}
		}
	}

	data := map[string]interface{}{
		"nodes":       allNodes,
		"edges":       allEdges,
		"symbolCount": len(symbolIds),
	}
	if len(errors) > 0 {
		data["errors"] = errors
	}

	return NewToolResponse().Data(data).Build(), nil
}

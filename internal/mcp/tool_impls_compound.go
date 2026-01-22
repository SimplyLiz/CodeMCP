package mcp

import (
	"context"

	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/query"
)

// v8.0 Compound tool implementations
// These tools aggregate multiple granular queries to reduce AI tool calls by 60-70%

// toolExplore provides comprehensive area exploration
func (s *MCPServer) toolExplore(params map[string]interface{}) (*envelope.Response, error) {
	target, ok := params["target"].(string)
	if !ok || target == "" {
		return nil, errors.NewInvalidParameterError("target", "required")
	}

	depth := query.ExploreStandard
	if v, ok := params["depth"].(string); ok {
		switch v {
		case "shallow":
			depth = query.ExploreShallow
		case "deep":
			depth = query.ExploreDeep
		case "standard":
			depth = query.ExploreStandard
		}
	}

	focus := query.FocusStructure
	if v, ok := params["focus"].(string); ok {
		switch v {
		case "structure":
			focus = query.FocusStructure
		case "dependencies":
			focus = query.FocusDependencies
		case "changes":
			focus = query.FocusChanges
		}
	}

	ctx := context.Background()
	result, err := s.engine().Explore(ctx, query.ExploreOptions{
		Target: target,
		Depth:  depth,
		Focus:  focus,
	})
	if err != nil {
		return nil, err
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}

// toolUnderstand provides comprehensive symbol deep-dive
func (s *MCPServer) toolUnderstand(params map[string]interface{}) (*envelope.Response, error) {
	q, ok := params["query"].(string)
	if !ok || q == "" {
		return nil, errors.NewInvalidParameterError("query", "required")
	}

	includeReferences := true
	if v, ok := params["includeReferences"].(bool); ok {
		includeReferences = v
	}

	includeCallGraph := true
	if v, ok := params["includeCallGraph"].(bool); ok {
		includeCallGraph = v
	}

	maxReferences := 50
	if v, ok := params["maxReferences"].(float64); ok {
		maxReferences = int(v)
	}

	ctx := context.Background()
	result, err := s.engine().Understand(ctx, query.UnderstandOptions{
		Query:             q,
		IncludeReferences: includeReferences,
		IncludeCallGraph:  includeCallGraph,
		MaxReferences:     maxReferences,
	})
	if err != nil {
		return nil, err
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}

// toolPrepareChange provides pre-change analysis
func (s *MCPServer) toolPrepareChange(params map[string]interface{}) (*envelope.Response, error) {
	target, ok := params["target"].(string)
	if !ok || target == "" {
		return nil, errors.NewInvalidParameterError("target", "required")
	}

	changeType := query.ChangeModify
	if v, ok := params["changeType"].(string); ok {
		switch v {
		case "modify":
			changeType = query.ChangeModify
		case "rename":
			changeType = query.ChangeRename
		case "delete":
			changeType = query.ChangeDelete
		case "extract":
			changeType = query.ChangeExtract
		}
	}

	ctx := context.Background()
	result, err := s.engine().PrepareChange(ctx, query.PrepareChangeOptions{
		Target:     target,
		ChangeType: changeType,
	})
	if err != nil {
		return nil, err
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}

// toolBatchGet retrieves multiple symbols by ID
func (s *MCPServer) toolBatchGet(params map[string]interface{}) (*envelope.Response, error) {
	symbolIds, ok := params["symbolIds"].([]interface{})
	if !ok || len(symbolIds) == 0 {
		return nil, errors.NewInvalidParameterError("symbolIds", "required array of symbol IDs")
	}

	ids := make([]string, 0, len(symbolIds))
	for _, id := range symbolIds {
		if s, ok := id.(string); ok {
			ids = append(ids, s)
		}
	}

	if len(ids) == 0 {
		return nil, errors.NewInvalidParameterError("symbolIds", "must contain string values")
	}

	ctx := context.Background()
	result, err := s.engine().BatchGet(ctx, query.BatchGetOptions{
		SymbolIds: ids,
	})
	if err != nil {
		return nil, err
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}

// toolBatchSearch performs multiple symbol searches
func (s *MCPServer) toolBatchSearch(params map[string]interface{}) (*envelope.Response, error) {
	queriesRaw, ok := params["queries"].([]interface{})
	if !ok || len(queriesRaw) == 0 {
		return nil, errors.NewInvalidParameterError("queries", "required array of search queries")
	}

	queries := make([]query.BatchSearchQuery, 0, len(queriesRaw))
	for _, qRaw := range queriesRaw {
		qMap, ok := qRaw.(map[string]interface{})
		if !ok {
			continue
		}

		q := query.BatchSearchQuery{}

		if v, ok := qMap["query"].(string); ok {
			q.Query = v
		}
		if v, ok := qMap["kind"].(string); ok {
			q.Kind = v
		}
		if v, ok := qMap["scope"].(string); ok {
			q.Scope = v
		}
		if v, ok := qMap["limit"].(float64); ok {
			q.Limit = int(v)
		}

		if q.Query != "" {
			queries = append(queries, q)
		}
	}

	if len(queries) == 0 {
		return nil, errors.NewInvalidParameterError("queries", "must contain valid query objects")
	}

	ctx := context.Background()
	result, err := s.engine().BatchSearch(ctx, query.BatchSearchOptions{
		Queries: queries,
	})
	if err != nil {
		return nil, err
	}

	return NewToolResponse().
		Data(result).
		Build(), nil
}

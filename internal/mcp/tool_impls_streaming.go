package mcp

import (
	"context"

	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/query"
	"ckb/internal/streaming"
)

// RegisterStreamableTools registers all tools that support streaming.
func (s *MCPServer) RegisterStreamableTools() {
	s.RegisterStreamableHandler("findReferences", s.streamFindReferences, "references")
	s.RegisterStreamableHandler("searchSymbols", s.streamSearchSymbols, "symbols")
}

// streamFindReferences is the streaming version of findReferences.
func (s *MCPServer) streamFindReferences(params map[string]interface{}, stream *streaming.Stream) (*envelope.Response, error) {
	symbolId, ok := params["symbolId"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("symbolId", "")
	}

	scope, _ := params["scope"].(string)

	limit := 500 // Higher limit for streaming
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	includeTests := false
	if includeVal, ok := params["includeTests"].(bool); ok {
		includeTests = includeVal
	}

	s.logger.Debug("Executing streaming findReferences", map[string]interface{}{
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

	refsResp, err := s.engine().FindReferences(ctx, opts)
	if err != nil {
		return nil, errors.NewOperationError("find references", err)
	}

	// Send metadata
	if err := stream.SendMeta(streaming.MetaData{
		Total:      len(refsResp.References),
		Backends:   []string{"scip"},
		Confidence: 1.0,
	}); err != nil {
		return nil, err
	}

	// Send progress
	if err := stream.SendProgress("processing", 0, len(refsResp.References)); err != nil {
		return nil, err
	}

	// Convert and chunk references
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

	// Chunk and send
	chunks := streaming.ChunkSlice(refs, stream.ChunkSize())
	for i, chunk := range chunks {
		hasMore := i < len(chunks)-1
		data := map[string]interface{}{
			"references": chunk,
		}
		if err := stream.SendChunk(data, len(chunk), hasMore); err != nil {
			return nil, err
		}

		// Progress update every few chunks
		if i%3 == 0 {
			sent := 0
			for j := 0; j <= i; j++ {
				sent += len(chunks[j])
			}
			if err := stream.SendProgress("streaming", sent, len(refs)); err != nil {
				return nil, err
			}
		}
	}

	return nil, stream.SendDone(refsResp.Truncated)
}

// streamSearchSymbols is the streaming version of searchSymbols.
func (s *MCPServer) streamSearchSymbols(params map[string]interface{}, stream *streaming.Stream) (*envelope.Response, error) {
	queryStr, ok := params["query"].(string)
	if !ok {
		return nil, errors.NewInvalidParameterError("query", "")
	}

	scope, _ := params["scope"].(string)

	limit := 200 // Higher limit for streaming
	if limitVal, ok := params["limit"].(float64); ok {
		limit = int(limitVal)
	}

	kinds := []string{}
	if kindsVal, ok := params["kinds"].([]interface{}); ok {
		for _, k := range kindsVal {
			if kStr, ok := k.(string); ok {
				kinds = append(kinds, kStr)
			}
		}
	}

	s.logger.Debug("Executing streaming searchSymbols", map[string]interface{}{
		"query": queryStr,
		"scope": scope,
		"limit": limit,
		"kinds": kinds,
	})

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

	// Send metadata
	if err := stream.SendMeta(streaming.MetaData{
		Total:      len(searchResp.Symbols),
		Confidence: 1.0,
	}); err != nil {
		return nil, err
	}

	// Convert symbols
	symbols := make([]map[string]interface{}, 0, len(searchResp.Symbols))
	for _, sym := range searchResp.Symbols {
		symbol := map[string]interface{}{
			"stableId": sym.StableId,
			"name":     sym.Name,
			"kind":     sym.Kind,
			"moduleId": sym.ModuleId,
			"score":    sym.Score,
		}
		if sym.ModuleName != "" {
			symbol["moduleName"] = sym.ModuleName
		}
		if sym.Location != nil {
			symbol["location"] = map[string]interface{}{
				"fileId":    sym.Location.FileId,
				"startLine": sym.Location.StartLine,
			}
		}
		symbols = append(symbols, symbol)
	}

	// Chunk and send
	chunks := streaming.ChunkSlice(symbols, stream.ChunkSize())
	for i, chunk := range chunks {
		hasMore := i < len(chunks)-1
		data := map[string]interface{}{
			"symbols": chunk,
		}
		if err := stream.SendChunk(data, len(chunk), hasMore); err != nil {
			return nil, err
		}
	}

	return nil, stream.SendDone(searchResp.Truncated)
}

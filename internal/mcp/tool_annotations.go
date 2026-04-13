package mcp

import (
	"fmt"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/storage"
)

func (s *MCPServer) toolAnnotationSet(params map[string]interface{}) (*envelope.Response, error) {
	symbolURI, _ := params["symbol_uri"].(string)
	key, _ := params["key"].(string)
	value, _ := params["value"].(string)
	authorID, _ := params["author_id"].(string)
	if authorID == "" {
		authorID = "agent:ckb"
	}
	confidence := uint8(80)
	if c, ok := params["confidence"].(float64); ok && c >= 0 && c <= 100 {
		confidence = uint8(c)
	}

	if symbolURI == "" || key == "" {
		return nil, fmt.Errorf("symbol_uri and key are required")
	}

	repo := storage.NewLIPAnnotationRepository(s.engine().DB().Conn())
	err := repo.Set(&storage.LIPAnnotation{
		SymbolURI:  symbolURI,
		Key:        key,
		Value:      value,
		AuthorID:   authorID,
		Confidence: confidence,
	})
	if err != nil {
		return nil, err
	}
	return OperationalResponse(map[string]interface{}{"ok": true, "symbol_uri": symbolURI, "key": key}), nil
}

func (s *MCPServer) toolAnnotationGet(params map[string]interface{}) (*envelope.Response, error) {
	symbolURI, _ := params["symbol_uri"].(string)
	key, _ := params["key"].(string)
	if symbolURI == "" || key == "" {
		return nil, fmt.Errorf("symbol_uri and key are required")
	}

	repo := storage.NewLIPAnnotationRepository(s.engine().DB().Conn())
	a, err := repo.Get(symbolURI, key)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return OperationalResponse(map[string]interface{}{"found": false}), nil
	}
	return OperationalResponse(map[string]interface{}{
		"found":        true,
		"symbol_uri":   a.SymbolURI,
		"key":          a.Key,
		"value":        a.Value,
		"author_id":    a.AuthorID,
		"confidence":   a.Confidence,
		"timestamp_ms": a.TimestampMs,
		"expires_ms":   a.ExpiresMs,
	}), nil
}

func (s *MCPServer) toolAnnotationList(params map[string]interface{}) (*envelope.Response, error) {
	symbolURI, _ := params["symbol_uri"].(string)
	if symbolURI == "" {
		return nil, fmt.Errorf("symbol_uri is required")
	}

	repo := storage.NewLIPAnnotationRepository(s.engine().DB().Conn())
	annotations, err := repo.List(symbolURI)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0, len(annotations))
	for _, a := range annotations {
		out = append(out, map[string]interface{}{
			"key":          a.Key,
			"value":        a.Value,
			"author_id":    a.AuthorID,
			"confidence":   a.Confidence,
			"timestamp_ms": a.TimestampMs,
			"expires_ms":   a.ExpiresMs,
		})
	}
	return OperationalResponse(map[string]interface{}{"symbol_uri": symbolURI, "annotations": out}), nil
}

package mcp

import (
	"encoding/json"
	"fmt"

	"ckb/internal/envelope"
)

// handleMessage processes an incoming MCP message and returns a response
func (s *MCPServer) handleMessage(msg *MCPMessage) *MCPMessage {
	// Handle requests
	if msg.IsRequest() {
		return s.handleRequest(msg)
	}

	// Handle notifications (no response needed)
	if msg.IsNotification() {
		s.handleNotification(msg)
		return nil
	}

	// Invalid message
	return NewErrorMessage(msg.Id, InvalidRequest, "Invalid message: not a request or notification", nil)
}

// handleRequest handles a JSON-RPC request
func (s *MCPServer) handleRequest(msg *MCPMessage) *MCPMessage {
	s.logger.Debug("Handling request", map[string]interface{}{
		"method": msg.Method,
		"id":     msg.Id,
	})

	switch msg.Method {
	case "initialize":
		return s.handleInitializeRequest(msg)
	case "tools/list":
		return s.handleListToolsRequest(msg)
	case "tools/call":
		return s.handleCallToolRequest(msg)
	case "resources/list":
		return s.handleListResourcesRequest(msg)
	case "resources/read":
		return s.handleReadResourceRequest(msg)
	default:
		return NewErrorMessage(msg.Id, MethodNotFound, fmt.Sprintf("Method not found: %s", msg.Method), nil)
	}
}

// handleNotification handles a JSON-RPC notification
func (s *MCPServer) handleNotification(msg *MCPMessage) {
	s.logger.Debug("Handling notification", map[string]interface{}{
		"method": msg.Method,
	})

	// Handle specific notifications if needed
	switch msg.Method {
	case "notifications/initialized":
		s.logger.Info("Client initialized", nil)
	default:
		s.logger.Debug("Unknown notification", map[string]interface{}{
			"method": msg.Method,
		})
	}
}

// handleInitializeRequest handles the initialize request
func (s *MCPServer) handleInitializeRequest(msg *MCPMessage) *MCPMessage {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	result, err := s.handleInitialize(params)
	if err != nil {
		return NewErrorMessage(msg.Id, InternalError, err.Error(), nil)
	}

	return NewResultMessage(msg.Id, result)
}

// handleListToolsRequest handles the tools/list request
func (s *MCPServer) handleListToolsRequest(msg *MCPMessage) *MCPMessage {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	result, err := s.handleListTools(params)
	if err != nil {
		return NewErrorMessage(msg.Id, InternalError, err.Error(), nil)
	}

	return NewResultMessage(msg.Id, result)
}

// handleCallToolRequest handles the tools/call request
func (s *MCPServer) handleCallToolRequest(msg *MCPMessage) *MCPMessage {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		return NewErrorMessage(msg.Id, InvalidParams, "Invalid params: expected object", nil)
	}

	result, err := s.handleCallTool(params)
	if err != nil {
		return NewErrorMessage(msg.Id, InternalError, err.Error(), nil)
	}

	return NewResultMessage(msg.Id, result)
}

// handleListResourcesRequest handles the resources/list request
func (s *MCPServer) handleListResourcesRequest(msg *MCPMessage) *MCPMessage {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	result, err := s.handleListResources(params)
	if err != nil {
		return NewErrorMessage(msg.Id, InternalError, err.Error(), nil)
	}

	return NewResultMessage(msg.Id, result)
}

// handleReadResourceRequest handles the resources/read request
func (s *MCPServer) handleReadResourceRequest(msg *MCPMessage) *MCPMessage {
	params, ok := msg.Params.(map[string]interface{})
	if !ok {
		return NewErrorMessage(msg.Id, InvalidParams, "Invalid params: expected object", nil)
	}

	result, err := s.handleReadResource(params)
	if err != nil {
		return NewErrorMessage(msg.Id, InternalError, err.Error(), nil)
	}

	return NewResultMessage(msg.Id, result)
}

// handleListTools returns the list of available tools
func (s *MCPServer) handleListTools(params map[string]interface{}) (interface{}, error) {
	tools := s.GetToolDefinitions()
	return map[string]interface{}{
		"tools": tools,
	}, nil
}

// handleCallTool executes a tool
func (s *MCPServer) handleCallTool(params map[string]interface{}) (interface{}, error) {
	toolName, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'name' parameter")
	}

	toolParams, ok := params["arguments"].(map[string]interface{})
	if !ok {
		toolParams = make(map[string]interface{})
	}

	handler, exists := s.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	s.logger.Info("Calling tool", map[string]interface{}{
		"tool":   toolName,
		"params": toolParams,
	})

	result, err := handler(toolParams)
	if err != nil {
		// Wrap error in envelope format
		errResp := envelope.New().Data(nil).Error(err).Build()
		jsonBytes, _ := json.Marshal(errResp)
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": string(jsonBytes),
				},
			},
		}, nil
	}

	// Marshal the envelope response to JSON
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(jsonBytes),
			},
		},
	}, nil
}

// handleListResources returns the list of available resources
func (s *MCPServer) handleListResources(params map[string]interface{}) (interface{}, error) {
	resources, templates := s.GetResourceDefinitions()
	return map[string]interface{}{
		"resources":         resources,
		"resourceTemplates": templates,
	}, nil
}

// handleReadResource reads a resource by URI
func (s *MCPServer) handleReadResource(params map[string]interface{}) (interface{}, error) {
	uri, ok := params["uri"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'uri' parameter")
	}

	s.logger.Info("Reading resource", map[string]interface{}{
		"uri": uri,
	})

	result, err := s.handleResourceRead(uri)
	if err != nil {
		return nil, fmt.Errorf("resource read failed: %w", err)
	}

	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      uri,
				"mimeType": "application/json",
				"text":     fmt.Sprintf("%v", result),
			},
		},
	}, nil
}

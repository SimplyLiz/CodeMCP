package mcp

import (
	"strings"

	"ckb/internal/errors"
)

// Resource represents a static resource
type Resource struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// ResourceTemplate represents a dynamic resource with URI template
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
}

// ResourceHandler is a function that handles a resource read
type ResourceHandler func(uri string) (interface{}, error)

// GetResourceDefinitions returns static resources and resource templates
func (s *MCPServer) GetResourceDefinitions() ([]Resource, []ResourceTemplate) {
	resources := []Resource{
		{
			URI:  "ckb://status",
			Name: "System Status",
		},
		{
			URI:  "ckb://architecture",
			Name: "Architecture",
		},
	}

	templates := []ResourceTemplate{
		{
			URITemplate: "ckb://module/{moduleId}",
			Name:        "Module",
		},
		{
			URITemplate: "ckb://symbol/{symbolId}",
			Name:        "Symbol",
		},
	}

	return resources, templates
}

// handleResourceRead handles reading a resource by URI
func (s *MCPServer) handleResourceRead(uri string) (interface{}, error) {
	s.logger.Debug("Reading resource", map[string]interface{}{
		"uri": uri,
	})

	// Parse the URI scheme
	if !strings.HasPrefix(uri, "ckb://") {
		return nil, errors.NewInvalidParameterError("uri", "expected ckb:// scheme")
	}

	path := strings.TrimPrefix(uri, "ckb://")
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		return nil, errors.NewInvalidParameterError("uri", "empty path")
	}

	resourceType := parts[0]

	switch resourceType {
	case "status":
		return s.toolGetStatus(map[string]interface{}{})
	case "architecture":
		return s.toolGetArchitecture(map[string]interface{}{})
	case "module":
		if len(parts) < 2 {
			return nil, errors.NewInvalidParameterError("uri", "module URI requires module ID")
		}
		moduleId := parts[1]
		return s.readModule(moduleId)
	case "symbol":
		if len(parts) < 2 {
			return nil, errors.NewInvalidParameterError("uri", "symbol URI requires symbol ID")
		}
		symbolId := parts[1]
		return s.toolGetSymbol(map[string]interface{}{
			"symbolId": symbolId,
		})
	default:
		return nil, errors.NewResourceNotFoundError("resource type", resourceType)
	}
}

// readModule reads a module resource (placeholder)
func (s *MCPServer) readModule(moduleId string) (interface{}, error) {
	// This would call the actual module query implementation
	// For now, return a placeholder
	return map[string]interface{}{
		"moduleId": moduleId,
		"message":  "Module resource not yet implemented",
	}, nil
}

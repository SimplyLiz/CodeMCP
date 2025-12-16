package api

import (
	"net/http"
)

// handleOpenAPISpec returns the OpenAPI specification
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	spec := GenerateOpenAPISpec()
	WriteJSON(w, spec, http.StatusOK)
}

// GenerateOpenAPISpec generates the OpenAPI specification for the API
func GenerateOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "CKB HTTP API",
			"version":     "0.1.0",
			"description": "Code Knowledge Backend HTTP API for codebase comprehension",
		},
		"servers": []map[string]interface{}{
			{
				"url":         "http://localhost:8080",
				"description": "Local development server",
			},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health check",
					"description": "Simple liveness check for load balancers",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Server is healthy",
						},
					},
				},
			},
			"/ready": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Readiness check",
					"description": "Checks if all backends are available and ready",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Server is ready",
						},
						"503": map[string]interface{}{
							"description": "Server is not ready",
						},
					},
				},
			},
			"/status": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "System status",
					"description": "Returns current system status including backends, cache, and repository state",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "System status",
						},
					},
				},
			},
			"/doctor": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Diagnostic checks",
					"description": "Performs comprehensive diagnostic checks on all system components",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Diagnostic results",
						},
					},
				},
			},
			"/symbol/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get symbol by ID",
					"description": "Retrieves detailed information about a symbol",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Symbol identifier",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Symbol information",
						},
						"404": map[string]interface{}{
							"description": "Symbol not found",
						},
					},
				},
			},
			"/search": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Search symbols",
					"description": "Searches for symbols matching the query",
					"parameters": []map[string]interface{}{
						{
							"name":        "q",
							"in":          "query",
							"required":    true,
							"description": "Search query",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "scope",
							"in":          "query",
							"description": "Module scope to search within",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "kinds",
							"in":          "query",
							"description": "Comma-separated list of symbol kinds",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
						{
							"name":        "limit",
							"in":          "query",
							"description": "Maximum number of results",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 50,
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Search results",
						},
					},
				},
			},
			"/refs/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Find references",
					"description": "Finds all references to a symbol",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Symbol identifier",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "References found",
						},
						"404": map[string]interface{}{
							"description": "Symbol not found",
						},
					},
				},
			},
			"/architecture": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Architecture overview",
					"description": "Returns an overview of the codebase architecture",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Architecture information",
						},
					},
				},
			},
			"/impact/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Impact analysis",
					"description": "Analyzes the impact of changing a symbol",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Symbol identifier",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Impact analysis results",
						},
						"404": map[string]interface{}{
							"description": "Symbol not found",
						},
					},
				},
			},
			"/doctor/fix": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Get fix script",
					"description": "Generates a fix script for detected issues",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Fix script generated",
						},
					},
				},
			},
			"/cache/warm": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Warm cache",
					"description": "Initiates cache warming for commonly accessed data",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cache warming initiated",
						},
					},
				},
			},
			"/cache/clear": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Clear cache",
					"description": "Clears all cached data",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Cache cleared",
						},
					},
				},
			},
		},
	}
}

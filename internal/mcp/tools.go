package mcp

// Tool represents a CKB tool exposed via MCP
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolHandler is a function that handles a tool call
type ToolHandler func(params map[string]interface{}) (interface{}, error)

// GetToolDefinitions returns all tool definitions
func (s *MCPServer) GetToolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "getStatus",
			Description: "Get CKB system status including backend health, cache stats, and repository state",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "doctor",
			Description: "Diagnose CKB configuration issues and get suggested fixes",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "getSymbol",
			Description: "Get symbol metadata and location by stable ID",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The stable symbol ID (ckb:<repo>:sym:<fingerprint>)",
					},
					"repoStateMode": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"head", "full"},
						"default":     "head",
						"description": "Whether to use HEAD commit only or full working tree state",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "searchSymbols",
			Description: "Search for symbols by name with optional filtering",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query (substring match, case-insensitive)",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Optional module ID to limit search scope",
					},
					"kinds": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Optional list of symbol kinds to filter (e.g., 'class', 'function')",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"default":     20,
						"description": "Maximum number of results to return",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "findReferences",
			Description: "Find all references to a symbol with completeness information",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The stable symbol ID",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Optional module ID to limit search scope",
					},
					"merge": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"prefer-first", "union"},
						"default":     "prefer-first",
						"description": "Backend merge strategy",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"default":     100,
						"description": "Maximum number of references to return",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "getArchitecture",
			Description: "Get codebase architecture with module dependencies",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"depth": map[string]interface{}{
						"type":        "number",
						"default":     2,
						"description": "Maximum dependency depth to traverse",
					},
					"includeExternalDeps": map[string]interface{}{
						"type":        "boolean",
						"default":     false,
						"description": "Whether to include external dependencies",
					},
					"refresh": map[string]interface{}{
						"type":        "boolean",
						"default":     false,
						"description": "Force refresh of cached architecture",
					},
				},
			},
		},
		{
			Name:        "analyzeImpact",
			Description: "Analyze the impact of changing a symbol",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The stable symbol ID to analyze",
					},
					"depth": map[string]interface{}{
						"type":        "number",
						"default":     2,
						"description": "Maximum depth for transitive impact analysis",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "explainSymbol",
			Description: "Explain a symbol with usage, history, and summary",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The stable symbol ID to explain",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "justifySymbol",
			Description: "Provide a keep/investigate/remove style verdict",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The stable symbol ID to justify",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "getCallGraph",
			Description: "Return a lightweight call graph rooted at a symbol",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "Root symbol ID",
					},
					"direction": map[string]interface{}{
						"type":    "string",
						"enum":    []string{"callers", "callees", "both"},
						"default": "callers",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "getModuleOverview",
			Description: "Basic module overview including size and recent commits",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Module root path",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Friendly module name",
					},
				},
			},
		},
	}
}

// RegisterTools registers all tool handlers
func (s *MCPServer) RegisterTools() {
	s.tools["getStatus"] = s.toolGetStatus
	s.tools["doctor"] = s.toolDoctor
	s.tools["getSymbol"] = s.toolGetSymbol
	s.tools["searchSymbols"] = s.toolSearchSymbols
	s.tools["findReferences"] = s.toolFindReferences
	s.tools["getArchitecture"] = s.toolGetArchitecture
	s.tools["analyzeImpact"] = s.toolAnalyzeImpact
	s.tools["explainSymbol"] = s.toolExplainSymbol
	s.tools["justifySymbol"] = s.toolJustifySymbol
	s.tools["getCallGraph"] = s.toolGetCallGraph
	s.tools["getModuleOverview"] = s.toolGetModuleOverview
}

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
			Description: "Get an AI-friendly explanation of a symbol including usage, history, and summary",
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
			Description: "Get a keep/investigate/remove verdict for a symbol based on usage analysis",
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
			Description: "Get a lightweight call graph showing callers and callees of a symbol",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The root symbol ID for the call graph",
					},
					"direction": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"callers", "callees", "both"},
						"default":     "both",
						"description": "Which direction to traverse",
					},
					"depth": map[string]interface{}{
						"type":        "number",
						"default":     1,
						"description": "Maximum depth to traverse (1-4)",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "getModuleOverview",
			Description: "Get a high-level overview of a module including size and recent activity",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the module directory",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Optional friendly name for the module",
					},
				},
			},
		},
		{
			Name:        "explainFile",
			Description: "Get lightweight orientation for a file including role, symbols, and key relationships",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filePath": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file (relative or absolute)",
					},
				},
				"required": []string{"filePath"},
			},
		},
		{
			Name:        "listEntrypoints",
			Description: "List system entrypoints (API handlers, CLI mains, jobs) with ranking signals",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"moduleFilter": map[string]interface{}{
						"type":        "string",
						"description": "Optional filter to specific module",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"default":     30,
						"description": "Maximum number of entrypoints to return",
					},
				},
			},
		},
		{
			Name:        "traceUsage",
			Description: "Trace how a symbol is reached from system entrypoints. Returns causal paths, not just neighbors.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbolId": map[string]interface{}{
						"type":        "string",
						"description": "The target symbol ID to trace usage for",
					},
					"maxPaths": map[string]interface{}{
						"type":        "number",
						"default":     10,
						"description": "Maximum number of paths to return",
					},
					"maxDepth": map[string]interface{}{
						"type":        "number",
						"default":     5,
						"description": "Maximum path depth to traverse (1-5)",
					},
				},
				"required": []string{"symbolId"},
			},
		},
		{
			Name:        "summarizeDiff",
			Description: "Compress diffs into 'what changed, what might break'. Supports commit ranges, single commits, or time windows. Default: last 30 days.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"commitRange": map[string]interface{}{
						"type":        "object",
						"description": "Commit range selector (base..head)",
						"properties": map[string]interface{}{
							"base": map[string]interface{}{
								"type":        "string",
								"description": "Base commit hash or ref",
							},
							"head": map[string]interface{}{
								"type":        "string",
								"description": "Head commit hash or ref",
							},
						},
						"required": []string{"base", "head"},
					},
					"commit": map[string]interface{}{
						"type":        "string",
						"description": "Single commit hash to analyze",
					},
					"timeWindow": map[string]interface{}{
						"type":        "object",
						"description": "Time window selector",
						"properties": map[string]interface{}{
							"start": map[string]interface{}{
								"type":        "string",
								"description": "Start date (ISO8601)",
							},
							"end": map[string]interface{}{
								"type":        "string",
								"description": "End date (ISO8601)",
							},
						},
						"required": []string{"start"},
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
	s.tools["explainFile"] = s.toolExplainFile
	s.tools["listEntrypoints"] = s.toolListEntrypoints
	s.tools["traceUsage"] = s.toolTraceUsage
	s.tools["summarizeDiff"] = s.toolSummarizeDiff
}

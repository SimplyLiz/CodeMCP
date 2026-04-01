package a2a

import (
	"fmt"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/mcp"
)

// SkillRegistry maps MCP tools to A2A skills.
type SkillRegistry struct {
	skills []Skill
	byID   map[string]Skill
}

// tagMap maps preset names to descriptive tags for A2A skills.
var tagMap = map[string][]string{
	mcp.PresetCore:       {"code-intelligence", "core"},
	mcp.PresetReview:     {"code-review"},
	mcp.PresetRefactor:   {"refactoring"},
	mcp.PresetFederation: {"federation", "cross-repo"},
	mcp.PresetDocs:       {"documentation"},
	mcp.PresetOps:        {"operations"},
}

// NewSkillRegistry creates a skill registry from MCP tool definitions.
func NewSkillRegistry(mcpServer *mcp.MCPServer) *SkillRegistry {
	tools := mcpServer.GetToolDefinitions()
	presetMembership := buildPresetMembership()

	registry := &SkillRegistry{
		byID: make(map[string]Skill, len(tools)),
	}

	for _, tool := range tools {
		skill := Skill{
			ID:          tool.Name,
			Name:        tool.Name,
			Description: tool.Description,
			Tags:        tagsForTool(tool.Name, presetMembership),
			InputModes:  []string{"application/json"},
			OutputModes: []string{"application/json"},
		}
		// Generate usage examples from tool name
		skill.Examples = examplesForTool(tool.Name)

		registry.skills = append(registry.skills, skill)
		registry.byID[skill.ID] = skill
	}

	return registry
}

// AllSkills returns all registered skills.
func (r *SkillRegistry) AllSkills() []Skill {
	return r.skills
}

// GetSkill returns a skill by ID, or nil if not found.
func (r *SkillRegistry) GetSkill(id string) *Skill {
	s, ok := r.byID[id]
	if !ok {
		return nil
	}
	return &s
}

// ExtendedSkills returns skills with full input schemas from MCP tool definitions.
func ExtendedSkills(mcpServer *mcp.MCPServer) []ExtendedSkill {
	tools := mcpServer.GetToolDefinitions()
	var extended []ExtendedSkill
	for _, tool := range tools {
		extended = append(extended, ExtendedSkill{
			Skill: Skill{
				ID:          tool.Name,
				Name:        tool.Name,
				Description: tool.Description,
				InputModes:  []string{"application/json"},
				OutputModes: []string{"application/json"},
			},
			InputSchema: tool.InputSchema,
		})
	}
	return extended
}

// buildPresetMembership returns a map of tool name -> list of presets it belongs to.
func buildPresetMembership() map[string][]string {
	membership := make(map[string][]string)
	for presetName, toolNames := range mcp.Presets {
		for _, name := range toolNames {
			membership[name] = append(membership[name], presetName)
		}
	}
	return membership
}

// tagsForTool derives tags from the tool's preset membership.
func tagsForTool(toolName string, membership map[string][]string) []string {
	presets := membership[toolName]
	seen := make(map[string]bool)
	var tags []string
	for _, preset := range presets {
		for _, tag := range tagMap[preset] {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	// Add category from tool name heuristics
	name := strings.ToLower(toolName)
	switch {
	case strings.HasPrefix(name, "get") || strings.HasPrefix(name, "list") || strings.HasPrefix(name, "search") || strings.HasPrefix(name, "find"):
		if !seen["navigation"] {
			tags = append(tags, "navigation")
		}
	case strings.HasPrefix(name, "explain"):
		if !seen["understanding"] {
			tags = append(tags, "understanding")
		}
	case strings.HasPrefix(name, "analyze") || strings.HasPrefix(name, "audit"):
		if !seen["analysis"] {
			tags = append(tags, "analysis")
		}
	}
	return tags
}

// examplesForTool generates usage examples for a tool.
func examplesForTool(toolName string) []string {
	switch toolName {
	case "searchSymbols":
		return []string{`{"skill": "searchSymbols", "params": {"query": "handleAuth"}}`}
	case "getSymbol":
		return []string{`{"skill": "getSymbol", "params": {"symbolId": "ckb:repo:sym:abc123"}}`}
	case "findReferences":
		return []string{`{"skill": "findReferences", "params": {"symbolId": "ckb:repo:sym:abc123"}}`}
	case "explore":
		return []string{`{"skill": "explore", "params": {"target": "internal/api/", "depth": "standard"}}`}
	case "understand":
		return []string{`{"skill": "understand", "params": {"target": "handleAuth"}}`}
	case "getArchitecture":
		return []string{`{"skill": "getArchitecture", "params": {}}`}
	case "analyzeImpact":
		return []string{`{"skill": "analyzeImpact", "params": {"symbolId": "ckb:repo:sym:abc123"}}`}
	case "getStatus":
		return []string{`{"skill": "getStatus", "params": {}}`}
	default:
		return []string{fmt.Sprintf(`{"skill": "%s", "params": {}}`, toolName)}
	}
}

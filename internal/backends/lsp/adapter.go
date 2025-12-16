package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"ckb/internal/backends"
	"ckb/internal/errors"
	"ckb/internal/logging"
)

// LspAdapter adapts the LSP supervisor to the Backend interface
type LspAdapter struct {
	supervisor *LspSupervisor
	languageId string
	logger     *logging.Logger
}

// NewLspAdapter creates a new LSP adapter for a specific language
func NewLspAdapter(supervisor *LspSupervisor, languageId string, logger *logging.Logger) *LspAdapter {
	return &LspAdapter{
		supervisor: supervisor,
		languageId: languageId,
		logger:     logger,
	}
}

// ID returns the backend identifier
func (l *LspAdapter) ID() backends.BackendID {
	return backends.BackendLSP
}

// IsAvailable checks if the LSP backend is available
func (l *LspAdapter) IsAvailable() bool {
	// Check if server is configured
	if _, ok := l.supervisor.config.Backends.Lsp.Servers[l.languageId]; !ok {
		return false
	}

	// Check if LSP is enabled
	if !l.supervisor.config.Backends.Lsp.Enabled {
		return false
	}

	return true
}

// Capabilities returns the capabilities this backend supports
func (l *LspAdapter) Capabilities() []string {
	if !l.IsAvailable() {
		return []string{}
	}

	// Get process to check server capabilities
	proc := l.supervisor.GetProcess(l.languageId)
	if proc == nil || !proc.IsHealthy() {
		// Return default capabilities if process not running
		return []string{
			"symbol-search",
			"find-references",
			"goto-definition",
		}
	}

	// Check server capabilities and map to CKB capabilities
	caps := proc.GetCapabilities()
	ckbCaps := make([]string, 0)

	// Check for definition support
	if hasCapability(caps, "definitionProvider") {
		ckbCaps = append(ckbCaps, "goto-definition")
	}

	// Check for references support
	if hasCapability(caps, "referencesProvider") {
		ckbCaps = append(ckbCaps, "find-references")
	}

	// Check for symbol support
	if hasCapability(caps, "documentSymbolProvider") || hasCapability(caps, "workspaceSymbolProvider") {
		ckbCaps = append(ckbCaps, "symbol-search")
	}

	// Check for hover support
	if hasCapability(caps, "hoverProvider") {
		ckbCaps = append(ckbCaps, "hover")
	}

	return ckbCaps
}

// Priority returns the backend priority (LSP is fallback, priority 3)
func (l *LspAdapter) Priority() int {
	return 3
}

// GetSymbol retrieves symbol information
func (l *LspAdapter) GetSymbol(ctx context.Context, id string) (*backends.SymbolResult, error) {
	// Parse symbol ID (format: "file://path:line:character")
	uri, line, character, err := parseSymbolID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid symbol ID: %w", err)
	}

	// Get hover information for documentation
	hoverResult, err := l.supervisor.QueryHover(ctx, l.languageId, uri, line, character)
	if err != nil {
		l.logger.Debug("Failed to get hover info", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Get definition location
	defResult, err := l.supervisor.QueryDefinition(ctx, l.languageId, uri, line, character)
	if err != nil {
		return nil, fmt.Errorf("failed to get definition: %w", err)
	}

	// Parse definition result
	location, err := parseLocation(defResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse definition location: %w", err)
	}

	// Build symbol result
	symbol := &backends.SymbolResult{
		StableID: id,
		Name:     extractSymbolName(id),
		Kind:     "symbol", // LSP might provide more specific kind
		Location: *location,
		Completeness: backends.NewCompletenessInfo(
			0.7, // LSP provides best-effort results
			backends.BestEffortLSP,
			"Results from Language Server Protocol",
		),
	}

	// Extract documentation from hover if available
	if hoverResult != nil {
		if doc := extractDocumentation(hoverResult); doc != "" {
			symbol.Documentation = doc
		}
	}

	return symbol, nil
}

// SearchSymbols searches for symbols
func (l *LspAdapter) SearchSymbols(ctx context.Context, query string, opts backends.SearchOptions) (*backends.SearchResult, error) {
	// Use workspace/symbol to search
	result, err := l.supervisor.QueryWorkspaceSymbols(ctx, l.languageId, query)
	if err != nil {
		return nil, fmt.Errorf("workspace symbol search failed: %w", err)
	}

	// Parse symbols from result
	symbols, err := parseSymbols(result, l.supervisor.config.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to parse symbols: %w", err)
	}

	// Apply limits
	if opts.MaxResults > 0 && len(symbols) > opts.MaxResults {
		symbols = symbols[:opts.MaxResults]
	}

	return &backends.SearchResult{
		Symbols:      symbols,
		TotalMatches: len(symbols),
		Completeness: backends.NewCompletenessInfo(
			0.7,
			backends.BestEffortLSP,
			"Results from Language Server Protocol",
		),
	}, nil
}

// FindReferences finds all references to a symbol
func (l *LspAdapter) FindReferences(ctx context.Context, symbolID string, opts backends.RefOptions) (*backends.ReferencesResult, error) {
	// Parse symbol ID
	uri, line, character, err := parseSymbolID(symbolID)
	if err != nil {
		return nil, fmt.Errorf("invalid symbol ID: %w", err)
	}

	// Query references
	result, err := l.supervisor.QueryReferences(ctx, l.languageId, uri, line, character, opts.IncludeDeclaration)
	if err != nil {
		return nil, fmt.Errorf("references query failed: %w", err)
	}

	// Parse references
	refs, err := parseReferences(result, l.supervisor.config.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to parse references: %w", err)
	}

	// Apply limits
	if opts.MaxResults > 0 && len(refs) > opts.MaxResults {
		refs = refs[:opts.MaxResults]
	}

	return &backends.ReferencesResult{
		References:      refs,
		TotalReferences: len(refs),
		Completeness: backends.NewCompletenessInfo(
			0.7,
			backends.BestEffortLSP,
			"Results from Language Server Protocol",
		),
	}, nil
}

// Helper functions

func hasCapability(caps map[string]interface{}, capability string) bool {
	val, ok := caps[capability]
	if !ok {
		return false
	}

	// Check if it's a boolean true
	if b, ok := val.(bool); ok {
		return b
	}

	// If it's an object/map, the capability is supported
	if _, ok := val.(map[string]interface{}); ok {
		return true
	}

	return false
}

func parseSymbolID(id string) (uri string, line int, character int, err error) {
	// Expected format: "file:///path:line:character"
	if !strings.HasPrefix(id, "file://") {
		return "", 0, 0, fmt.Errorf("symbol ID must start with file://")
	}

	// Find the last two colons for line:character
	parts := strings.Split(id, ":")
	if len(parts) < 4 {
		return "", 0, 0, fmt.Errorf("invalid symbol ID format")
	}

	// Reconstruct URI (everything before last two parts)
	uri = strings.Join(parts[:len(parts)-2], ":")

	// Parse line and character
	_, err = fmt.Sscanf(parts[len(parts)-2]+":"+parts[len(parts)-1], "%d:%d", &line, &character)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to parse line:character: %w", err)
	}

	return uri, line, character, nil
}

func extractSymbolName(id string) string {
	// Extract filename from URI
	if strings.HasPrefix(id, "file://") {
		path := strings.TrimPrefix(id, "file://")
		// Remove line:character suffix
		if idx := strings.LastIndex(path, ":"); idx != -1 {
			path = path[:idx]
			if idx := strings.LastIndex(path, ":"); idx != -1 {
				path = path[:idx]
			}
		}
		return filepath.Base(path)
	}
	return "unknown"
}

func parseLocation(result interface{}) (*backends.Location, error) {
	// LSP returns Location or Location[]
	if locMap, ok := result.(map[string]interface{}); ok {
		return parseLocationFromMap(locMap)
	}

	if locArray, ok := result.([]interface{}); ok {
		if len(locArray) == 0 {
			return nil, errors.NewCkbError(
				errors.SymbolNotFound,
				"no location found",
				nil,
				nil,
				nil,
			)
		}
		if locMap, ok := locArray[0].(map[string]interface{}); ok {
			return parseLocationFromMap(locMap)
		}
	}

	return nil, fmt.Errorf("unexpected location format")
}

func parseLocationFromMap(locMap map[string]interface{}) (*backends.Location, error) {
	uri, _ := locMap["uri"].(string)
	rangeMap, _ := locMap["range"].(map[string]interface{})

	if rangeMap == nil {
		return nil, fmt.Errorf("missing range in location")
	}

	startMap, _ := rangeMap["start"].(map[string]interface{})
	endMap, _ := rangeMap["end"].(map[string]interface{})

	if startMap == nil || endMap == nil {
		return nil, fmt.Errorf("missing start/end in range")
	}

	startLine, _ := startMap["line"].(float64)
	startChar, _ := startMap["character"].(float64)
	endLine, _ := endMap["line"].(float64)
	endChar, _ := endMap["character"].(float64)

	// Convert file:// URI to relative path
	path := strings.TrimPrefix(uri, "file://")

	return &backends.Location{
		Path:      path,
		Line:      int(startLine) + 1, // LSP is 0-indexed, CKB is 1-indexed
		Column:    int(startChar) + 1,
		EndLine:   int(endLine) + 1,
		EndColumn: int(endChar) + 1,
	}, nil
}

func parseSymbols(result interface{}, repoRoot string) ([]backends.SymbolResult, error) {
	symbolArray, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array of symbols")
	}

	symbols := make([]backends.SymbolResult, 0, len(symbolArray))

	for _, item := range symbolArray {
		symMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := symMap["name"].(string)
		kind, _ := symMap["kind"].(float64)

		location, err := parseLocation(symMap["location"])
		if err != nil {
			continue
		}

		symbol := backends.SymbolResult{
			StableID:     fmt.Sprintf("file://%s:%d:%d", location.Path, location.Line, location.Column),
			Name:         name,
			Kind:         symbolKindToString(int(kind)),
			Location:     *location,
			Completeness: backends.NewCompletenessInfo(0.7, backends.BestEffortLSP, "LSP symbol"),
		}

		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

func parseReferences(result interface{}, repoRoot string) ([]backends.Reference, error) {
	refArray, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array of references")
	}

	refs := make([]backends.Reference, 0, len(refArray))

	for _, item := range refArray {
		locMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		location, err := parseLocationFromMap(locMap)
		if err != nil {
			continue
		}

		ref := backends.Reference{
			Location: *location,
			Kind:     "reference",
		}

		refs = append(refs, ref)
	}

	return refs, nil
}

func extractDocumentation(hoverResult interface{}) string {
	hoverMap, ok := hoverResult.(map[string]interface{})
	if !ok {
		return ""
	}

	contents, ok := hoverMap["contents"].(map[string]interface{})
	if !ok {
		// Might be a string or array
		if str, ok := hoverMap["contents"].(string); ok {
			return str
		}
		return ""
	}

	if value, ok := contents["value"].(string); ok {
		return value
	}

	return ""
}

func symbolKindToString(kind int) string {
	// LSP SymbolKind enumeration
	switch kind {
	case 1:
		return "file"
	case 2:
		return "module"
	case 3:
		return "namespace"
	case 4:
		return "package"
	case 5:
		return "class"
	case 6:
		return "method"
	case 7:
		return "property"
	case 8:
		return "field"
	case 9:
		return "constructor"
	case 10:
		return "enum"
	case 11:
		return "interface"
	case 12:
		return "function"
	case 13:
		return "variable"
	case 14:
		return "constant"
	case 15:
		return "string"
	case 16:
		return "number"
	case 17:
		return "boolean"
	case 18:
		return "array"
	default:
		return "symbol"
	}
}

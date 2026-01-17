// Package query provides compound tools for v8.0 that aggregate multiple granular queries.
// These reduce AI tool calls by 60-70% for common exploration workflows.
package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ckb/internal/coupling"
	"ckb/internal/errors"
	"ckb/internal/output"
	"ckb/internal/version"
)

// ExploreDepth controls the thoroughness of exploration.
type ExploreDepth string

const (
	ExploreShallow  ExploreDepth = "shallow"
	ExploreStandard ExploreDepth = "standard"
	ExploreDeep     ExploreDepth = "deep"
)

// ExploreFocus controls which aspects to emphasize.
type ExploreFocus string

const (
	FocusStructure    ExploreFocus = "structure"
	FocusDependencies ExploreFocus = "dependencies"
	FocusChanges      ExploreFocus = "changes"
)

// ExploreOptions controls explore behavior.
type ExploreOptions struct {
	Target string       // file, directory, or module path
	Depth  ExploreDepth // shallow, standard, deep
	Focus  ExploreFocus // structure, dependencies, changes
}

// ExploreResponse provides comprehensive area exploration.
type ExploreResponse struct {
	AINavigationMeta
	Overview      *ExploreOverview     `json:"overview"`
	KeySymbols    []ExploreSymbol      `json:"keySymbols"`
	Dependencies  *ExploreDependencies `json:"dependencies,omitempty"`
	RecentChanges []ExploreChange      `json:"recentChanges,omitempty"`
	Hotspots      []ExploreHotspot     `json:"hotspots,omitempty"`
	Suggestions   []string             `json:"suggestions,omitempty"`
	Health        *ExploreHealth       `json:"health"`
}

// ExploreOverview provides high-level information about the target.
type ExploreOverview struct {
	TargetType     string `json:"targetType"` // file, directory, module
	Path           string `json:"path"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	FileCount      int    `json:"fileCount,omitempty"`
	SymbolCount    int    `json:"symbolCount,omitempty"`
	LineCount      int    `json:"lineCount,omitempty"`
	Language       string `json:"language,omitempty"`
	Role           string `json:"role,omitempty"` // core, glue, test, config
	Responsibility string `json:"responsibility,omitempty"`
}

// ExploreSymbol represents a key symbol in the explored area.
type ExploreSymbol struct {
	StableId   string  `json:"stableId"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Line       int     `json:"line,omitempty"`
	File       string  `json:"file,omitempty"`
	Visibility string  `json:"visibility"`
	Importance float64 `json:"importance"`       // ranking score
	Reason     string  `json:"reason,omitempty"` // why it's important
}

// ExploreDependencies describes imports and exports.
type ExploreDependencies struct {
	Imports      []ExploreDependency `json:"imports,omitempty"`
	Exports      []ExploreDependency `json:"exports,omitempty"`
	InternalDeps []string            `json:"internalDeps,omitempty"`
	ExternalDeps []string            `json:"externalDeps,omitempty"`
	Truncated    bool                `json:"truncated,omitempty"` // True if limits were applied
}

// ExploreDependency represents a single dependency.
type ExploreDependency struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Kind     string `json:"kind,omitempty"` // package, module, file
	Internal bool   `json:"internal"`
}

// ExploreChange represents a recent change in the area.
type ExploreChange struct {
	CommitHash   string `json:"commitHash"`
	Message      string `json:"message"`
	Author       string `json:"author"`
	Date         string `json:"date"`
	FilesChanged int    `json:"filesChanged"`
}

// ExploreHotspot represents a volatile area.
type ExploreHotspot struct {
	File      string  `json:"file"`
	Score     float64 `json:"score"`
	Reason    string  `json:"reason"` // high-churn, high-coupling, complex
	ChurnRate float64 `json:"churnRate,omitempty"`
}

// ExploreHealth summarizes backend status for this query.
type ExploreHealth struct {
	ScipAvailable bool     `json:"scipAvailable"`
	GitAvailable  bool     `json:"gitAvailable"`
	OverallStatus string   `json:"overallStatus"` // healthy, degraded, limited
	Warnings      []string `json:"warnings,omitempty"`
}

// Explore provides comprehensive area exploration.
// Replaces: explainFile → searchSymbols → getCallGraph → getHotspots
func (e *Engine) Explore(ctx context.Context, opts ExploreOptions) (*ExploreResponse, error) {
	startTime := time.Now()

	// Set defaults
	if opts.Depth == "" {
		opts.Depth = ExploreStandard
	}
	if opts.Focus == "" {
		opts.Focus = FocusStructure
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Determine target type
	targetType, absPath, err := e.resolveExploreTarget(opts.Target)
	if err != nil {
		return nil, err
	}

	// Collect results from sub-queries in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	var overview *ExploreOverview
	var keySymbols []ExploreSymbol
	var dependencies *ExploreDependencies
	var recentChanges []ExploreChange
	var hotspots []ExploreHotspot
	var warnings []string

	// Health check
	health := &ExploreHealth{
		ScipAvailable: e.scipAdapter != nil && e.scipAdapter.IsAvailable(),
		GitAvailable:  e.gitAdapter != nil && e.gitAdapter.IsAvailable(),
	}
	if health.ScipAvailable && health.GitAvailable {
		health.OverallStatus = "healthy"
	} else if health.ScipAvailable || health.GitAvailable {
		health.OverallStatus = "degraded"
	} else {
		health.OverallStatus = "limited"
	}

	// 1. Get overview based on target type
	wg.Add(1)
	go func() {
		defer wg.Done()
		ov, err := e.buildExploreOverview(ctx, targetType, absPath, opts.Target)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("overview: %s", err.Error()))
			mu.Unlock()
			return
		}
		mu.Lock()
		overview = ov
		mu.Unlock()
	}()

	// 2. Get key symbols
	wg.Add(1)
	go func() {
		defer wg.Done()
		limit := 10
		if opts.Depth == ExploreDeep {
			limit = 20
		} else if opts.Depth == ExploreShallow {
			limit = 5
		}
		syms, err := e.getExploreSymbols(ctx, targetType, absPath, opts.Target, limit)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("symbols: %s", err.Error()))
			mu.Unlock()
			return
		}
		mu.Lock()
		keySymbols = syms
		mu.Unlock()
	}()

	// 3. Get dependencies (if standard or deep, or focus is dependencies)
	if opts.Depth != ExploreShallow || opts.Focus == FocusDependencies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deps := e.getExploreDependencies(ctx, targetType, absPath, opts.Target)
			mu.Lock()
			dependencies = deps
			mu.Unlock()
		}()
	}

	// 4. Get recent changes (if git available and not shallow, or focus is changes)
	if health.GitAvailable && (opts.Depth != ExploreShallow || opts.Focus == FocusChanges) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limit := 5
			if opts.Focus == FocusChanges {
				limit = 10
			}
			changes, err := e.getExploreChanges(ctx, absPath, opts.Target, limit)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("changes: %s", err.Error()))
				mu.Unlock()
				return
			}
			mu.Lock()
			recentChanges = changes
			mu.Unlock()
		}()
	}

	// 5. Get hotspots (if git available and deep or focus is changes)
	if health.GitAvailable && (opts.Depth == ExploreDeep || opts.Focus == FocusChanges) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hs, err := e.getExploreHotspots(ctx, absPath, opts.Target, 5)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("hotspots: %s", err.Error()))
				mu.Unlock()
				return
			}
			mu.Lock()
			hotspots = hs
			mu.Unlock()
		}()
	}

	// Wait for all queries
	wg.Wait()

	// Generate suggestions based on results
	suggestions := e.generateExploreSuggestions(keySymbols, hotspots, opts)

	// Build provenance
	var backendContribs []BackendContribution
	if health.ScipAvailable {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "scip",
			Available: true,
			Used:      true,
		})
	}
	if health.GitAvailable {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "git",
			Available: true,
			Used:      true,
		})
	}

	completeness := CompletenessInfo{
		Score:  0.9,
		Reason: "compound-explore",
	}
	if !health.ScipAvailable {
		completeness.Score = 0.5
		completeness.Reason = "limited-scip-unavailable"
	}

	health.Warnings = warnings

	return &ExploreResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "explore",
			Resolved:      &ResolvedTarget{SymbolId: opts.Target, ResolvedFrom: "path", Confidence: 1.0},
			Provenance:    e.buildProvenance(repoState, "head", startTime, backendContribs, completeness),
		},
		Overview:      overview,
		KeySymbols:    keySymbols,
		Dependencies:  dependencies,
		RecentChanges: recentChanges,
		Hotspots:      hotspots,
		Suggestions:   suggestions,
		Health:        health,
	}, nil
}

// resolveExploreTarget determines the target type and absolute path.
func (e *Engine) resolveExploreTarget(target string) (string, string, error) {
	// Normalize path
	absPath := target
	if !filepath.IsAbs(target) {
		absPath = filepath.Join(e.repoRoot, target)
	}
	absPath = filepath.Clean(absPath)

	// Security: verify path is within repo root
	repoRootClean := filepath.Clean(e.repoRoot)
	if !strings.HasPrefix(absPath, repoRootClean+string(filepath.Separator)) && absPath != repoRootClean {
		return "", "", errors.NewCkbError(errors.InvalidParameter, fmt.Sprintf("path outside repository: %s", target), nil, nil, nil)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return "", "", errors.NewCkbError(errors.ResourceNotFound, fmt.Sprintf("path not found: %s", target), nil, nil, nil)
	}
	if err != nil {
		return "", "", e.wrapError(err, errors.InternalError)
	}

	if info.IsDir() {
		return "directory", absPath, nil
	}
	return "file", absPath, nil
}

// buildExploreOverview creates the overview for the target.
func (e *Engine) buildExploreOverview(ctx context.Context, targetType, absPath, relTarget string) (*ExploreOverview, error) {
	overview := &ExploreOverview{
		TargetType: targetType,
		Path:       relTarget,
		Name:       filepath.Base(relTarget),
	}

	if targetType == "file" {
		// Single file overview
		overview.Language = detectLanguage(relTarget)
		overview.Role = classifyFileRole(relTarget)
		overview.LineCount = countFileLines(absPath)
		overview.SymbolCount = 0 // Will be updated by symbol search
	} else {
		// Directory overview - skip large generated directories
		fileCount := 0
		//nolint:errcheck // intentionally ignore walk errors to count accessible files
		_ = filepath.Walk(absPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // skip inaccessible files, continue walk
			}
			if info.IsDir() {
				if skipExploreDirectory(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			fileCount++
			return nil
		})
		overview.FileCount = fileCount

		// Get module overview if available
		modResp, err := e.GetModuleOverview(ctx, ModuleOverviewOptions{Path: relTarget})
		if err == nil && modResp != nil {
			overview.Responsibility = modResp.Module.Responsibility
			overview.SymbolCount = modResp.Size.SymbolCount
		}
	}

	return overview, nil
}

// getExploreSymbols retrieves key symbols for the target.
func (e *Engine) getExploreSymbols(ctx context.Context, targetType, absPath, relTarget string, limit int) ([]ExploreSymbol, error) {
	if e.scipAdapter == nil || !e.scipAdapter.IsAvailable() {
		return nil, nil
	}

	// Search for symbols in the target scope
	searchResp, err := e.SearchSymbols(ctx, SearchSymbolsOptions{
		Query: "",
		Scope: relTarget,
		Limit: limit * 2, // Request more for ranking
	})
	if err != nil {
		return nil, err
	}

	// Convert and rank symbols
	symbols := make([]ExploreSymbol, 0, len(searchResp.Symbols))
	for _, sym := range searchResp.Symbols {
		importance := calculateSymbolImportance(sym)
		reason := inferImportanceReason(sym, importance)

		file := ""
		line := 0
		if sym.Location != nil {
			file = sym.Location.FileId
			line = sym.Location.StartLine
		}

		visibility := "internal"
		if sym.Visibility != nil {
			visibility = sym.Visibility.Visibility
		}

		symbols = append(symbols, ExploreSymbol{
			StableId:   sym.StableId,
			Name:       sym.Name,
			Kind:       sym.Kind,
			Line:       line,
			File:       file,
			Visibility: visibility,
			Importance: importance,
			Reason:     reason,
		})
	}

	// Sort by importance
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].Importance > symbols[j].Importance
	})

	// Apply limit
	if len(symbols) > limit {
		symbols = symbols[:limit]
	}

	return symbols, nil
}

// calculateSymbolImportance computes importance score for ranking.
func calculateSymbolImportance(sym SearchResultItem) float64 {
	score := 0.0

	// Visibility weight
	if sym.Visibility != nil {
		switch sym.Visibility.Visibility {
		case "public":
			score += 40
		case "internal":
			score += 20
		case "private":
			score += 10
		}
	}

	// Kind weight
	switch sym.Kind {
	case "class", "interface", "struct":
		score += 30
	case "function", "method":
		score += 25
	case "type":
		score += 20
	case "constant", "variable":
		score += 10
	}

	// Add existing ranking score if available
	if sym.Ranking != nil {
		score += sym.Ranking.Score * 0.3
	}

	return score
}

// inferImportanceReason explains why a symbol is important.
func inferImportanceReason(sym SearchResultItem, importance float64) string {
	if sym.Visibility != nil && sym.Visibility.Visibility == "public" {
		return "exported API"
	}
	switch sym.Kind {
	case "class", "interface", "struct":
		return "key type"
	case "function", "method":
		if importance > 50 {
			return "high-visibility function"
		}
		return "function"
	}
	return ""
}

// Budget limits for explore dependencies
const (
	maxExploreFiles   = 100 // Maximum files to scan for imports
	maxExploreImports = 100 // Maximum unique imports to collect
)

// skipDirectories returns true for directories that should be skipped during exploration
func skipExploreDirectory(name string) bool {
	switch name {
	case "node_modules", "vendor", "dist", "build", ".git", ".next", "__pycache__", ".cache":
		return true
	}
	return false
}

// getExploreDependencies extracts import/export information.
func (e *Engine) getExploreDependencies(ctx context.Context, targetType, absPath, relTarget string) *ExploreDependencies {
	deps := &ExploreDependencies{}

	if targetType == "file" {
		// For single file, parse imports
		imports := parseFileImports(absPath)
		for _, imp := range imports {
			isInternal := !strings.Contains(imp, "/") || strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../")
			deps.Imports = append(deps.Imports, ExploreDependency{
				Name:     imp,
				Internal: isInternal,
			})
			if isInternal {
				deps.InternalDeps = append(deps.InternalDeps, imp)
			} else {
				deps.ExternalDeps = append(deps.ExternalDeps, imp)
			}
		}
	} else {
		// For directory, aggregate imports from files with limits
		importSet := make(map[string]bool)
		fileCount := 0
		truncated := false

		//nolint:errcheck // intentionally ignore walk errors to collect imports from accessible files
		_ = filepath.Walk(absPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // skip inaccessible files, continue walk
			}
			if info.IsDir() {
				// Skip common large/generated directories
				if skipExploreDirectory(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			// Limit files scanned
			if fileCount >= maxExploreFiles {
				truncated = true
				return filepath.SkipAll
			}

			// Limit imports collected
			if len(importSet) >= maxExploreImports {
				truncated = true
				return filepath.SkipAll
			}

			fileCount++
			for _, imp := range parseFileImports(path) {
				if len(importSet) < maxExploreImports {
					importSet[imp] = true
				} else {
					truncated = true
					break
				}
			}
			return nil
		})

		deps.Truncated = truncated

		for imp := range importSet {
			isInternal := !strings.Contains(imp, "/") || strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../")
			deps.Imports = append(deps.Imports, ExploreDependency{
				Name:     imp,
				Internal: isInternal,
			})
			if isInternal {
				deps.InternalDeps = append(deps.InternalDeps, imp)
			} else {
				deps.ExternalDeps = append(deps.ExternalDeps, imp)
			}
		}
	}

	// Sort for deterministic output
	sort.Slice(deps.Imports, func(i, j int) bool {
		return deps.Imports[i].Name < deps.Imports[j].Name
	})
	sort.Strings(deps.InternalDeps)
	sort.Strings(deps.ExternalDeps)

	return deps
}

// parseFileImports extracts import statements from a file.
func parseFileImports(filePath string) []string {
	// Only parse source code files, skip JSON, markdown, etc.
	if !isSourceFile(filepath.Base(filePath)) {
		return nil
	}

	// Simple heuristic parsing - could be enhanced with tree-sitter
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var imports []string
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Go imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, `"`) && strings.HasSuffix(line, `"`) {
			if strings.Contains(line, `"`) {
				// Extract quoted import
				start := strings.Index(line, `"`)
				end := strings.LastIndex(line, `"`)
				if start < end {
					imports = append(imports, line[start+1:end])
				}
			}
		}

		// TypeScript/JavaScript imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			if idx := strings.Index(line, "from "); idx >= 0 {
				rest := line[idx+5:]
				rest = strings.Trim(rest, `"';`)
				if rest != "" {
					imports = append(imports, rest)
				}
			} else if strings.Contains(line, `"`) || strings.Contains(line, "'") {
				// Handle: import "module"
				for _, sep := range []string{`"`, `'`} {
					if strings.Contains(line, sep) {
						parts := strings.Split(line, sep)
						if len(parts) >= 2 {
							imports = append(imports, parts[1])
							break
						}
					}
				}
			}
		}

		// Python imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			if strings.HasPrefix(line, "from ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					imports = append(imports, parts[1])
				}
			} else {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					imp := strings.Split(parts[1], ",")[0]
					imports = append(imports, strings.TrimSpace(imp))
				}
			}
		}
	}

	return imports
}

// getExploreChanges retrieves recent changes for the target.
func (e *Engine) getExploreChanges(ctx context.Context, absPath, relTarget string, limit int) ([]ExploreChange, error) {
	if e.gitAdapter == nil || !e.gitAdapter.IsAvailable() {
		return nil, nil
	}

	history, err := e.gitAdapter.GetFileHistory(relTarget, limit)
	if err != nil {
		return nil, err
	}

	changes := make([]ExploreChange, 0, len(history.Commits))
	for _, commit := range history.Commits {
		changes = append(changes, ExploreChange{
			CommitHash:   commit.Hash[:8],
			Message:      commit.Message,
			Author:       commit.Author,
			Date:         commit.Timestamp,
			FilesChanged: 1, // Would need git show to get accurate count
		})
	}

	return changes, nil
}

// getExploreHotspots retrieves hotspots for the target.
func (e *Engine) getExploreHotspots(ctx context.Context, absPath, relTarget string, limit int) ([]ExploreHotspot, error) {
	// Use the existing hotspots API
	hsResp, err := e.GetHotspots(ctx, GetHotspotsOptions{
		Scope: relTarget,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}

	hotspots := make([]ExploreHotspot, 0, len(hsResp.Hotspots))
	for _, hs := range hsResp.Hotspots {
		// Calculate score from ranking if available
		score := 0.5 // default
		if hs.Ranking != nil {
			score = hs.Ranking.Score / 100.0 // Normalize to 0-1
		}
		reason := "high-churn"
		if hs.RiskLevel == "high" {
			reason = "volatile"
		}
		hotspots = append(hotspots, ExploreHotspot{
			File:      hs.FilePath,
			Score:     score,
			Reason:    reason,
			ChurnRate: float64(hs.Churn.ChangeCount) / 30.0, // Changes per day
		})
	}

	return hotspots, nil
}

// generateExploreSuggestions creates actionable suggestions.
func (e *Engine) generateExploreSuggestions(symbols []ExploreSymbol, hotspots []ExploreHotspot, opts ExploreOptions) []string {
	var suggestions []string

	// Suggest drilling into key symbols
	if len(symbols) > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Use 'understand %s' to deep-dive into key symbol", symbols[0].Name))
	}

	// Suggest reviewing hotspots
	if len(hotspots) > 0 && hotspots[0].Score > 0.7 {
		suggestions = append(suggestions, fmt.Sprintf("Review hotspot '%s' (high churn)", filepath.Base(hotspots[0].File)))
	}

	// Suggest deeper exploration if shallow
	if opts.Depth == ExploreShallow {
		suggestions = append(suggestions, "Use depth='standard' or 'deep' for more details")
	}

	return suggestions
}

// =============================================================================
// understand - Symbol Deep-Dive
// =============================================================================

// UnderstandOptions controls understand behavior.
type UnderstandOptions struct {
	Query             string // symbol name or ID
	IncludeReferences bool
	IncludeCallGraph  bool
	MaxReferences     int
}

// UnderstandResponse provides comprehensive symbol understanding.
type UnderstandResponse struct {
	AINavigationMeta
	Symbol       *SymbolInfo           `json:"symbol,omitempty"`
	Explanation  string                `json:"explanation"`
	References   *UnderstandReferences `json:"references,omitempty"`
	Callers      []UnderstandCaller    `json:"callers,omitempty"`
	Callees      []UnderstandCallee    `json:"callees,omitempty"`
	RelatedTests []UnderstandTest      `json:"relatedTests,omitempty"`
	Ambiguity    *UnderstandAmbiguity  `json:"ambiguity,omitempty"`
}

// UnderstandReferences groups references by file.
type UnderstandReferences struct {
	TotalCount int                       `json:"totalCount"`
	ByFile     []UnderstandReferenceFile `json:"byFile"`
	Truncated  bool                      `json:"truncated"`
}

// UnderstandReferenceFile groups references in a single file.
type UnderstandReferenceFile struct {
	File       string              `json:"file"`
	Count      int                 `json:"count"`
	References []UnderstandRefLine `json:"references"`
}

// UnderstandRefLine represents a single reference location.
type UnderstandRefLine struct {
	Line    int    `json:"line"`
	Kind    string `json:"kind"`
	Context string `json:"context,omitempty"`
}

// UnderstandCaller represents a caller of the symbol.
type UnderstandCaller struct {
	SymbolId string `json:"symbolId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// UnderstandCallee represents a callee of the symbol.
type UnderstandCallee struct {
	SymbolId string `json:"symbolId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	File     string `json:"file,omitempty"`
}

// UnderstandTest represents a test related to the symbol.
type UnderstandTest struct {
	File     string `json:"file"`
	Name     string `json:"name,omitempty"`
	Line     int    `json:"line,omitempty"`
	Relation string `json:"relation"` // imports, tests, references
}

// UnderstandAmbiguity describes multi-match scenarios.
type UnderstandAmbiguity struct {
	MatchCount int               `json:"matchCount"`
	TopMatches []UnderstandMatch `json:"topMatches"`
	Hint       string            `json:"hint"`
}

// UnderstandMatch represents a potential match for ambiguous queries.
type UnderstandMatch struct {
	SymbolId   string `json:"symbolId"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	ModuleId   string `json:"moduleId"`
	Visibility string `json:"visibility"`
}

// Understand provides comprehensive symbol deep-dive.
// Replaces: searchSymbols → getSymbol → explainSymbol → findReferences → getCallGraph
func (e *Engine) Understand(ctx context.Context, opts UnderstandOptions) (*UnderstandResponse, error) {
	startTime := time.Now()

	// Set defaults
	if opts.MaxReferences <= 0 {
		opts.MaxReferences = 50
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// First, search for the symbol
	searchResp, err := e.SearchSymbols(ctx, SearchSymbolsOptions{
		Query: opts.Query,
		Limit: 10,
	})
	if err != nil {
		return nil, err
	}

	// Check for ambiguity
	if len(searchResp.Symbols) == 0 {
		return nil, errors.NewCkbError(errors.SymbolNotFound, fmt.Sprintf("No symbols found matching '%s'", opts.Query), nil, nil, nil)
	}

	// If multiple matches, return ambiguity info unless exact match
	var targetSymbol *SearchResultItem
	var ambiguity *UnderstandAmbiguity

	if len(searchResp.Symbols) > 1 {
		// Check for exact match
		for i, sym := range searchResp.Symbols {
			if strings.EqualFold(sym.Name, opts.Query) || sym.StableId == opts.Query {
				targetSymbol = &searchResp.Symbols[i]
				break
			}
		}

		// If no exact match, return ambiguity info
		if targetSymbol == nil {
			topMatches := make([]UnderstandMatch, 0, 5)
			for i, sym := range searchResp.Symbols {
				if i >= 5 {
					break
				}
				visibility := "internal"
				if sym.Visibility != nil {
					visibility = sym.Visibility.Visibility
				}
				topMatches = append(topMatches, UnderstandMatch{
					SymbolId:   sym.StableId,
					Name:       sym.Name,
					Kind:       sym.Kind,
					ModuleId:   sym.ModuleId,
					Visibility: visibility,
				})
			}
			ambiguity = &UnderstandAmbiguity{
				MatchCount: searchResp.TotalCount,
				TopMatches: topMatches,
				Hint:       "Add module scope or use full symbol ID to disambiguate",
			}
			targetSymbol = &searchResp.Symbols[0] // Use best match
		}
	} else {
		targetSymbol = &searchResp.Symbols[0]
	}

	// Get detailed symbol info
	symbolResp, err := e.GetSymbol(ctx, GetSymbolOptions{SymbolId: targetSymbol.StableId})
	if err != nil {
		return nil, err
	}

	// Get explanation
	explainResp, err := e.ExplainSymbol(ctx, ExplainSymbolOptions{SymbolId: targetSymbol.StableId})
	explanation := ""
	if err == nil && explainResp != nil {
		explanation = explainResp.Summary.Tldr
	}

	// Collect results in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	var references *UnderstandReferences
	var callers []UnderstandCaller
	var callees []UnderstandCallee
	var relatedTests []UnderstandTest
	var warnings []string

	// Get references if requested
	if opts.IncludeReferences {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refs, err := e.getUnderstandReferences(ctx, targetSymbol.StableId, opts.MaxReferences)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("references: %s", err.Error()))
				mu.Unlock()
				return
			}
			mu.Lock()
			references = refs
			mu.Unlock()
		}()
	}

	// Get call graph if requested
	if opts.IncludeCallGraph {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, ce, err := e.getUnderstandCallGraph(ctx, targetSymbol.StableId)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("callgraph: %s", err.Error()))
				mu.Unlock()
				return
			}
			mu.Lock()
			callers = c
			callees = ce
			mu.Unlock()
		}()
	}

	// Get related tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		tests := e.getUnderstandTests(ctx, targetSymbol.StableId, targetSymbol.ModuleId)
		mu.Lock()
		relatedTests = tests
		mu.Unlock()
	}()

	wg.Wait()

	// Build provenance
	var backendContribs []BackendContribution
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "scip",
			Available: true,
			Used:      true,
		})
	}

	completeness := CompletenessInfo{
		Score:  0.9,
		Reason: "compound-understand",
	}

	// Build drilldowns
	drilldowns := []output.Drilldown{
		{Label: "Analyze impact", Query: fmt.Sprintf("prepareChange %s", targetSymbol.StableId)},
	}
	if references != nil && references.Truncated {
		drilldowns = append(drilldowns, output.Drilldown{
			Label: "Get all references",
			Query: fmt.Sprintf("findReferences %s --limit 200", targetSymbol.StableId),
		})
	}

	return &UnderstandResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "understand",
			Resolved:      &ResolvedTarget{SymbolId: targetSymbol.StableId, ResolvedFrom: "search", Confidence: 0.95},
			Provenance:    e.buildProvenance(repoState, "full", startTime, backendContribs, completeness),
			Drilldowns:    drilldowns,
		},
		Symbol:       symbolResp.Symbol,
		Explanation:  explanation,
		References:   references,
		Callers:      callers,
		Callees:      callees,
		RelatedTests: relatedTests,
		Ambiguity:    ambiguity,
	}, nil
}

// getUnderstandReferences retrieves and groups references by file.
func (e *Engine) getUnderstandReferences(ctx context.Context, symbolId string, limit int) (*UnderstandReferences, error) {
	refResp, err := e.FindReferences(ctx, FindReferencesOptions{
		SymbolId:     symbolId,
		IncludeTests: true,
		Limit:        limit,
	})
	if err != nil {
		return nil, err
	}

	// Group by file
	byFile := make(map[string][]UnderstandRefLine)
	for _, ref := range refResp.References {
		if ref.Location == nil {
			continue
		}
		file := ref.Location.FileId
		byFile[file] = append(byFile[file], UnderstandRefLine{
			Line:    ref.Location.StartLine,
			Kind:    ref.Kind,
			Context: ref.Context,
		})
	}

	// Convert to sorted slice
	files := make([]UnderstandReferenceFile, 0, len(byFile))
	for file, refs := range byFile {
		// Sort refs by line
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].Line < refs[j].Line
		})
		files = append(files, UnderstandReferenceFile{
			File:       file,
			Count:      len(refs),
			References: refs,
		})
	}

	// Sort files by count (most references first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Count > files[j].Count
	})

	return &UnderstandReferences{
		TotalCount: refResp.TotalCount,
		ByFile:     files,
		Truncated:  refResp.Truncated,
	}, nil
}

// getUnderstandCallGraph retrieves callers and callees.
func (e *Engine) getUnderstandCallGraph(ctx context.Context, symbolId string) ([]UnderstandCaller, []UnderstandCallee, error) {
	callResp, err := e.GetCallGraph(ctx, CallGraphOptions{
		SymbolId:  symbolId,
		Direction: "both",
		Depth:     1,
	})
	if err != nil {
		return nil, nil, err
	}

	var callers []UnderstandCaller
	var callees []UnderstandCallee

	for _, node := range callResp.Nodes {
		if node.Role == "caller" {
			loc := node.Location
			line := 0
			file := ""
			if loc != nil {
				line = loc.StartLine
				file = loc.FileId
			}
			callers = append(callers, UnderstandCaller{
				SymbolId: node.SymbolId,
				Name:     node.Name,
				Kind:     "", // Not in CallGraphNode
				File:     file,
				Line:     line,
			})
		} else if node.Role == "callee" {
			callees = append(callees, UnderstandCallee{
				SymbolId: node.SymbolId,
				Name:     node.Name,
				Kind:     "",
			})
		}
	}

	return callers, callees, nil
}

// getUnderstandTests finds tests related to a symbol.
func (e *Engine) getUnderstandTests(ctx context.Context, symbolId, moduleId string) []UnderstandTest {
	var tests []UnderstandTest

	// Look for test files in the same module
	if moduleId != "" {
		testPattern := filepath.Join(e.repoRoot, moduleId, "*_test.go")
		matches, _ := filepath.Glob(testPattern)
		for _, match := range matches {
			rel, _ := filepath.Rel(e.repoRoot, match)
			tests = append(tests, UnderstandTest{
				File:     rel,
				Relation: "same-module",
			})
		}

		// Also check for TypeScript/JavaScript tests
		for _, pattern := range []string{"*.test.ts", "*.test.js", "*.spec.ts", "*.spec.js"} {
			testPattern = filepath.Join(e.repoRoot, moduleId, pattern)
			matches, _ = filepath.Glob(testPattern)
			for _, match := range matches {
				rel, _ := filepath.Rel(e.repoRoot, match)
				tests = append(tests, UnderstandTest{
					File:     rel,
					Relation: "same-module",
				})
			}
		}
	}

	// Limit to 10 tests
	if len(tests) > 10 {
		tests = tests[:10]
	}

	return tests
}

// =============================================================================
// prepareChange - Pre-Change Analysis
// =============================================================================

// ChangeType describes the type of change being planned.
type ChangeType string

const (
	ChangeModify  ChangeType = "modify"
	ChangeRename  ChangeType = "rename"
	ChangeDelete  ChangeType = "delete"
	ChangeExtract ChangeType = "extract"
)

// PrepareChangeOptions controls prepareChange behavior.
type PrepareChangeOptions struct {
	Target     string     // symbol ID or file path
	ChangeType ChangeType // modify, rename, delete, extract
}

// PrepareChangeResponse provides comprehensive pre-change analysis.
type PrepareChangeResponse struct {
	AINavigationMeta
	Target           *PrepareChangeTarget `json:"target"`
	DirectDependents []PrepareDependent   `json:"directDependents"`
	TransitiveImpact *PrepareTransitive   `json:"transitiveImpact"`
	RelatedTests     []PrepareTest        `json:"relatedTests"`
	CoChangeFiles    []PrepareCoChange    `json:"coChangeFiles,omitempty"`
	RiskAssessment   *PrepareRisk         `json:"riskAssessment"`
}

// PrepareChangeTarget describes what will be changed.
type PrepareChangeTarget struct {
	SymbolId   string `json:"symbolId,omitempty"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // symbol, file, module
	Path       string `json:"path,omitempty"`
	ModuleId   string `json:"moduleId,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// PrepareDependent describes a direct dependent.
type PrepareDependent struct {
	SymbolId string `json:"symbolId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	ModuleId string `json:"moduleId"`
}

// PrepareTransitive summarizes transitive impact.
type PrepareTransitive struct {
	TotalCallers int `json:"totalCallers"`
	ModuleSpread int `json:"moduleSpread"`
	MaxDepth     int `json:"maxDepth"`
}

// PrepareTest describes a test that may need updating.
type PrepareTest struct {
	File     string `json:"file"`
	Name     string `json:"name,omitempty"`
	Relation string `json:"relation"` // direct, transitive, coverage
}

// PrepareCoChange describes a file that historically changes together.
type PrepareCoChange struct {
	File        string  `json:"file"`
	Correlation float64 `json:"correlation"`
	CoChanges   int     `json:"coChanges"`
}

// PrepareRisk assesses the risk of the change.
type PrepareRisk struct {
	Level       string   `json:"level"` // low, medium, high, critical
	Score       float64  `json:"score"`
	Factors     []string `json:"factors"`
	Suggestions []string `json:"suggestions"`
}

// PrepareChange provides comprehensive pre-change analysis.
// Replaces: analyzeImpact + getAffectedTests + analyzeCoupling + risk calculation
func (e *Engine) PrepareChange(ctx context.Context, opts PrepareChangeOptions) (*PrepareChangeResponse, error) {
	startTime := time.Now()

	// Set defaults
	if opts.ChangeType == "" {
		opts.ChangeType = ChangeModify
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Determine target type and resolve
	target, err := e.resolvePrepareTarget(ctx, opts.Target)
	if err != nil {
		return nil, err
	}

	// Collect results in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	var directDependents []PrepareDependent
	var transitiveImpact *PrepareTransitive
	var relatedTests []PrepareTest
	var coChangeFiles []PrepareCoChange
	var riskFactors []string
	var warnings []string

	// Get impact analysis
	wg.Add(1)
	go func() {
		defer wg.Done()
		if target.SymbolId == "" {
			return
		}
		deps, trans, factors, err := e.getPrepareImpact(ctx, target.SymbolId)
		if err != nil {
			mu.Lock()
			warnings = append(warnings, fmt.Sprintf("impact: %s", err.Error()))
			mu.Unlock()
			return
		}
		mu.Lock()
		directDependents = deps
		transitiveImpact = trans
		riskFactors = append(riskFactors, factors...)
		mu.Unlock()
	}()

	// Get affected tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		tests := e.getPrepareTests(ctx, target)
		mu.Lock()
		relatedTests = tests
		mu.Unlock()
	}()

	// Get co-change files if git available
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path := target.Path
			if path == "" && target.ModuleId != "" {
				path = target.ModuleId
			}
			if path == "" {
				return
			}
			coChanges, err := e.getPrepareCoChanges(ctx, path)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("cochange: %s", err.Error()))
				mu.Unlock()
				return
			}
			mu.Lock()
			coChangeFiles = coChanges
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Calculate risk assessment
	risk := e.calculatePrepareRisk(target, directDependents, transitiveImpact, relatedTests, coChangeFiles, riskFactors, opts.ChangeType)

	// Build provenance
	var backendContribs []BackendContribution
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "scip",
			Available: true,
			Used:      true,
		})
	}
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "git",
			Available: true,
			Used:      true,
		})
	}

	completeness := CompletenessInfo{
		Score:  0.9,
		Reason: "compound-preparechange",
	}

	return &PrepareChangeResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "prepareChange",
			Resolved:      &ResolvedTarget{SymbolId: target.SymbolId, ResolvedFrom: "id", Confidence: 1.0},
			Provenance:    e.buildProvenance(repoState, "full", startTime, backendContribs, completeness),
		},
		Target:           target,
		DirectDependents: directDependents,
		TransitiveImpact: transitiveImpact,
		RelatedTests:     relatedTests,
		CoChangeFiles:    coChangeFiles,
		RiskAssessment:   risk,
	}, nil
}

// resolvePrepareTarget resolves the target to a PrepareChangeTarget.
func (e *Engine) resolvePrepareTarget(ctx context.Context, target string) (*PrepareChangeTarget, error) {
	// Check if it's a file path
	absPath := target
	if !filepath.IsAbs(target) {
		absPath = filepath.Join(e.repoRoot, target)
	}
	absPath = filepath.Clean(absPath)

	if info, err := os.Stat(absPath); err == nil {
		// It's a file or directory
		relPath, _ := filepath.Rel(e.repoRoot, absPath)
		if info.IsDir() {
			return &PrepareChangeTarget{
				Name:     filepath.Base(relPath),
				Kind:     "module",
				Path:     relPath,
				ModuleId: relPath,
			}, nil
		}
		return &PrepareChangeTarget{
			Name:     filepath.Base(relPath),
			Kind:     "file",
			Path:     relPath,
			ModuleId: filepath.Dir(relPath),
		}, nil
	}

	// Try to resolve as symbol ID
	symbolResp, err := e.GetSymbol(ctx, GetSymbolOptions{SymbolId: target})
	if err != nil {
		return nil, errors.NewCkbError(errors.ResourceNotFound, fmt.Sprintf("Target not found: %s", target), nil, nil, nil)
	}

	if symbolResp.Symbol == nil {
		return nil, errors.NewCkbError(errors.ResourceNotFound, fmt.Sprintf("Symbol not found: %s", target), nil, nil, nil)
	}

	visibility := "internal"
	if symbolResp.Symbol.Visibility != nil {
		visibility = symbolResp.Symbol.Visibility.Visibility
	}

	path := ""
	if symbolResp.Symbol.Location != nil {
		path = symbolResp.Symbol.Location.FileId
	}

	return &PrepareChangeTarget{
		SymbolId:   symbolResp.Symbol.StableId,
		Name:       symbolResp.Symbol.Name,
		Kind:       symbolResp.Symbol.Kind,
		Path:       path,
		ModuleId:   symbolResp.Symbol.ModuleId,
		Visibility: visibility,
	}, nil
}

// getPrepareImpact retrieves impact analysis data.
func (e *Engine) getPrepareImpact(ctx context.Context, symbolId string) ([]PrepareDependent, *PrepareTransitive, []string, error) {
	impactResp, err := e.AnalyzeImpact(ctx, AnalyzeImpactOptions{
		SymbolId: symbolId,
		Depth:    2,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// Convert direct impacts to dependents
	dependents := make([]PrepareDependent, 0, len(impactResp.DirectImpact))
	for _, imp := range impactResp.DirectImpact {
		line := 0
		file := ""
		if imp.Location != nil {
			line = imp.Location.StartLine
			file = imp.Location.FileId
		}
		dependents = append(dependents, PrepareDependent{
			SymbolId: imp.StableId,
			Name:     imp.Name,
			Kind:     imp.Kind,
			File:     file,
			Line:     line,
			ModuleId: imp.ModuleId,
		})
	}

	// Build transitive summary
	moduleSet := make(map[string]bool)
	maxDepth := 0
	for _, imp := range impactResp.TransitiveImpact {
		moduleSet[imp.ModuleId] = true
		if imp.Distance > maxDepth {
			maxDepth = imp.Distance
		}
	}

	transitive := &PrepareTransitive{
		TotalCallers: len(impactResp.TransitiveImpact) + len(impactResp.DirectImpact),
		ModuleSpread: len(moduleSet),
		MaxDepth:     maxDepth,
	}

	// Extract risk factors
	var factors []string
	if impactResp.RiskScore != nil {
		for _, f := range impactResp.RiskScore.Factors {
			factors = append(factors, f.Name)
		}
	}

	return dependents, transitive, factors, nil
}

// getPrepareTests finds tests affected by the change.
func (e *Engine) getPrepareTests(ctx context.Context, target *PrepareChangeTarget) []PrepareTest {
	var tests []PrepareTest

	// Look for test files in the same module
	if target.ModuleId != "" {
		patterns := []string{
			filepath.Join(e.repoRoot, target.ModuleId, "*_test.go"),
			filepath.Join(e.repoRoot, target.ModuleId, "*.test.ts"),
			filepath.Join(e.repoRoot, target.ModuleId, "*.test.js"),
			filepath.Join(e.repoRoot, target.ModuleId, "*.spec.ts"),
			filepath.Join(e.repoRoot, target.ModuleId, "*.spec.js"),
		}
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				rel, _ := filepath.Rel(e.repoRoot, match)
				tests = append(tests, PrepareTest{
					File:     rel,
					Relation: "same-module",
				})
			}
		}
	}

	// Limit to 20 tests
	if len(tests) > 20 {
		tests = tests[:20]
	}

	return tests
}

// getPrepareCoChanges finds files that historically change together.
func (e *Engine) getPrepareCoChanges(ctx context.Context, path string) ([]PrepareCoChange, error) {
	// Use coupling package directly
	analyzer := coupling.NewAnalyzer(e.repoRoot, e.logger)

	result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
		Target:         path,
		MinCorrelation: 0.3,
		WindowDays:     365,
		Limit:          10,
		RepoRoot:       e.repoRoot,
	})
	if err != nil {
		return nil, err
	}

	coChanges := make([]PrepareCoChange, 0, len(result.Correlations))
	for _, cf := range result.Correlations {
		coChanges = append(coChanges, PrepareCoChange{
			File:        cf.File,
			Correlation: cf.Correlation,
			CoChanges:   cf.CoChangeCount,
		})
	}

	return coChanges, nil
}

// calculatePrepareRisk calculates the risk assessment.
func (e *Engine) calculatePrepareRisk(
	target *PrepareChangeTarget,
	dependents []PrepareDependent,
	transitive *PrepareTransitive,
	tests []PrepareTest,
	coChanges []PrepareCoChange,
	existingFactors []string,
	changeType ChangeType,
) *PrepareRisk {
	score := 0.0
	var factors []string
	var suggestions []string

	// Factor: Number of direct dependents
	if len(dependents) > 10 {
		score += 0.25
		factors = append(factors, fmt.Sprintf("High dependent count (%d direct callers)", len(dependents)))
	} else if len(dependents) > 5 {
		score += 0.15
		factors = append(factors, fmt.Sprintf("Moderate dependent count (%d direct callers)", len(dependents)))
	}

	// Factor: Module spread
	if transitive != nil && transitive.ModuleSpread > 5 {
		score += 0.2
		factors = append(factors, fmt.Sprintf("High module spread (%d modules affected)", transitive.ModuleSpread))
		suggestions = append(suggestions, "Consider splitting into smaller changes")
	}

	// Factor: Public visibility
	if target.Visibility == "public" {
		score += 0.15
		factors = append(factors, "Public API change")
		suggestions = append(suggestions, "Ensure backward compatibility or bump major version")
	}

	// Factor: Test coverage
	if len(tests) == 0 {
		score += 0.2
		factors = append(factors, "No tests found")
		suggestions = append(suggestions, "Add tests before modifying")
	}

	// Factor: Change type
	if changeType == ChangeDelete {
		score += 0.15
		factors = append(factors, "Deletion change type")
	} else if changeType == ChangeRename {
		score += 0.1
		factors = append(factors, "Rename change type")
	}

	// Factor: Co-change files
	if len(coChanges) > 5 {
		score += 0.1
		factors = append(factors, fmt.Sprintf("High coupling (%d co-change files)", len(coChanges)))
	}

	// Add existing factors
	factors = append(factors, existingFactors...)

	// Determine level
	level := "low"
	if score >= 0.7 {
		level = "critical"
	} else if score >= 0.5 {
		level = "high"
	} else if score >= 0.3 {
		level = "medium"
	}

	return &PrepareRisk{
		Level:       level,
		Score:       score,
		Factors:     factors,
		Suggestions: suggestions,
	}
}

// =============================================================================
// Batch Operations
// =============================================================================

// BatchGetOptions controls batchGet behavior.
type BatchGetOptions struct {
	SymbolIds []string // max 50
}

// BatchGetResponse returns multiple symbols by ID.
type BatchGetResponse struct {
	AINavigationMeta
	Results map[string]*SymbolInfo `json:"results"`
	Errors  map[string]string      `json:"errors,omitempty"`
}

// BatchGet retrieves multiple symbols by ID in a single call.
func (e *Engine) BatchGet(ctx context.Context, opts BatchGetOptions) (*BatchGetResponse, error) {
	startTime := time.Now()

	// Limit to 50 symbols
	if len(opts.SymbolIds) > 50 {
		return nil, errors.NewCkbError(errors.InvalidParameter, "Maximum 50 symbols per batch", nil, nil, nil)
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	results := make(map[string]*SymbolInfo)
	errs := make(map[string]string)

	// Fetch symbols in parallel with limited concurrency
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 10) // Limit concurrency

	for _, id := range opts.SymbolIds {
		wg.Add(1)
		go func(symbolId string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := e.GetSymbol(ctx, GetSymbolOptions{SymbolId: symbolId})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[symbolId] = err.Error()
			} else if resp.Symbol != nil {
				results[symbolId] = resp.Symbol
			} else {
				errs[symbolId] = "symbol not found"
			}
		}(id)
	}

	wg.Wait()

	// Build provenance
	var backendContribs []BackendContribution
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId:   "scip",
			Available:   true,
			Used:        true,
			ResultCount: len(results),
		})
	}

	completeness := CompletenessInfo{
		Score:  float64(len(results)) / float64(len(opts.SymbolIds)),
		Reason: "batch-get",
	}

	return &BatchGetResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "batchGet",
			Provenance:    e.buildProvenance(repoState, "head", startTime, backendContribs, completeness),
		},
		Results: results,
		Errors:  errs,
	}, nil
}

// BatchSearchQuery represents a single search in a batch.
type BatchSearchQuery struct {
	Query string `json:"query"`
	Kind  string `json:"kind,omitempty"`
	Scope string `json:"scope,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// BatchSearchOptions controls batchSearch behavior.
type BatchSearchOptions struct {
	Queries []BatchSearchQuery
}

// BatchSearchResult contains results for a single search.
type BatchSearchResult struct {
	Query   string             `json:"query"`
	Symbols []SearchResultItem `json:"symbols"`
	Error   string             `json:"error,omitempty"`
}

// BatchSearchResponse returns multiple search results.
type BatchSearchResponse struct {
	AINavigationMeta
	Results []BatchSearchResult `json:"results"`
}

// BatchSearch performs multiple symbol searches in a single call.
func (e *Engine) BatchSearch(ctx context.Context, opts BatchSearchOptions) (*BatchSearchResponse, error) {
	startTime := time.Now()

	// Limit to 10 queries
	if len(opts.Queries) > 10 {
		return nil, errors.NewCkbError(errors.InvalidParameter, "Maximum 10 queries per batch", nil, nil, nil)
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	results := make([]BatchSearchResult, len(opts.Queries))

	// Search in parallel
	var wg sync.WaitGroup
	for i, q := range opts.Queries {
		wg.Add(1)
		go func(idx int, query BatchSearchQuery) {
			defer wg.Done()

			limit := query.Limit
			if limit <= 0 {
				limit = 10
			}

			var kinds []string
			if query.Kind != "" {
				kinds = []string{query.Kind}
			}

			resp, err := e.SearchSymbols(ctx, SearchSymbolsOptions{
				Query: query.Query,
				Scope: query.Scope,
				Kinds: kinds,
				Limit: limit,
			})

			if err != nil {
				results[idx] = BatchSearchResult{
					Query: query.Query,
					Error: err.Error(),
				}
			} else {
				results[idx] = BatchSearchResult{
					Query:   query.Query,
					Symbols: resp.Symbols,
				}
			}
		}(i, q)
	}

	wg.Wait()

	// Build provenance
	var backendContribs []BackendContribution
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		backendContribs = append(backendContribs, BackendContribution{
			BackendId: "scip",
			Available: true,
			Used:      true,
		})
	}

	completeness := CompletenessInfo{
		Score:  0.9,
		Reason: "batch-search",
	}

	return &BatchSearchResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    version.Version,
			SchemaVersion: 1,
			Tool:          "batchSearch",
			Provenance:    e.buildProvenance(repoState, "head", startTime, backendContribs, completeness),
		},
		Results: results,
	}, nil
}

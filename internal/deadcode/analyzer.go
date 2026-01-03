package deadcode

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"ckb/internal/backends/scip"
)

// Analyzer detects dead code using SCIP index reference analysis.
type Analyzer struct {
	scipAdapter *scip.SCIPAdapter
	exclusions  *ExclusionRules
	logger      *slog.Logger
	repoRoot    string
}

// NewAnalyzer creates a new dead code analyzer.
func NewAnalyzer(scipAdapter *scip.SCIPAdapter, repoRoot string, logger *slog.Logger, excludePatterns []string) *Analyzer {
	return &Analyzer{
		scipAdapter: scipAdapter,
		exclusions:  NewExclusionRules(excludePatterns),
		logger:      logger,
		repoRoot:    repoRoot,
	}
}

// Analyze performs dead code detection with the given options.
func (a *Analyzer) Analyze(ctx context.Context, opts AnalyzerOptions) (*Result, error) {
	if a.scipAdapter == nil || !a.scipAdapter.IsAvailable() {
		return &Result{
			DeadCode: []DeadCodeItem{},
			Summary: DeadCodeSummary{
				ByKind:     make(map[string]int),
				ByCategory: make(map[string]int),
			},
		}, nil
	}

	// Get the underlying index for direct access
	index := a.scipAdapter.GetIndex()
	if index == nil {
		return &Result{
			DeadCode: []DeadCodeItem{},
			Summary: DeadCodeSummary{
				ByKind:     make(map[string]int),
				ByCategory: make(map[string]int),
			},
		}, nil
	}

	// Get all symbols
	allSymbols := a.scipAdapter.AllSymbols()
	if allSymbols == nil {
		a.logger.Debug("AllSymbols returned nil")
		return &Result{
			DeadCode: []DeadCodeItem{},
			Summary: DeadCodeSummary{
				ByKind:     make(map[string]int),
				ByCategory: make(map[string]int),
			},
		}, nil
	}

	a.logger.Debug("Starting dead code analysis",
		"totalSymbols", len(allSymbols),
		"includeExported", opts.IncludeExported,
		"includeUnexported", opts.IncludeUnexported,
		"scope", opts.Scope)

	var deadCode []DeadCodeItem
	totalAnalyzed := 0
	skippedNoId := 0
	skippedScope := 0
	skippedExport := 0
	skippedTest := 0
	skippedExclusion := 0

	for _, sym := range allSymbols {
		// Skip if no symbol ID
		if sym.Symbol == "" {
			skippedNoId++
			continue
		}

		// Parse symbol info
		symInfo := a.parseSymbolInfo(sym)

		// Check if in scope
		if len(opts.Scope) > 0 && !a.isInScope(symInfo.FilePath, opts.Scope) {
			skippedScope++
			continue
		}

		// Check export filter
		if opts.IncludeExported && !symInfo.Exported {
			if !opts.IncludeUnexported {
				skippedExport++
				continue
			}
		}
		if opts.IncludeUnexported && symInfo.Exported {
			if !opts.IncludeExported {
				skippedExport++
				continue
			}
		}

		// Skip test files for dead code analysis (they test other code)
		if IsTestFile(symInfo.FilePath) {
			skippedTest++
			continue
		}

		// Check exclusions
		if reason := a.exclusions.ShouldExclude(symInfo); reason != "" {
			skippedExclusion++
			continue
		}

		totalAnalyzed++

		// Get references
		refs, err := index.FindReferences(sym.Symbol, scip.ReferenceOptions{
			IncludeDefinition: false,
			IncludeTests:      true,
		})
		if err != nil {
			a.logger.Debug("Error finding references",
				"symbol", sym.Symbol,
				"error", err.Error())
			continue
		}

		// Categorize references
		stats := a.categorizeReferences(refs, sym.Symbol, symInfo.FilePath)

		// Classify as dead or not
		item, isDead := a.classifySymbol(symInfo, stats, opts)
		if isDead && item.Confidence >= opts.MinConfidence {
			deadCode = append(deadCode, item)
		}
	}

	// Sort by confidence (highest first), then by file path
	sort.Slice(deadCode, func(i, j int) bool {
		if deadCode[i].Confidence != deadCode[j].Confidence {
			return deadCode[i].Confidence > deadCode[j].Confidence
		}
		return deadCode[i].FilePath < deadCode[j].FilePath
	})

	// Apply limit
	if opts.Limit > 0 && len(deadCode) > opts.Limit {
		deadCode = deadCode[:opts.Limit]
	}

	a.logger.Debug("Dead code analysis completed",
		"totalAnalyzed", totalAnalyzed,
		"deadCodeFound", len(deadCode),
		"skippedNoId", skippedNoId,
		"skippedScope", skippedScope,
		"skippedExport", skippedExport,
		"skippedTest", skippedTest,
		"skippedExclusion", skippedExclusion)

	return &Result{
		DeadCode: deadCode,
		Summary:  a.computeSummary(deadCode, totalAnalyzed),
		Scope:    opts.Scope,
	}, nil
}

// parseSymbolInfo extracts information from a SCIP symbol.
func (a *Analyzer) parseSymbolInfo(sym *scip.SymbolInformation) SymbolInfo {
	name := sym.DisplayName
	// If DisplayName is empty, try to extract from the symbol ID
	if name == "" {
		name = extractNameFromSymbol(sym.Symbol)
	}

	info := SymbolInfo{
		Name:     name,
		Kind:     kindToString(sym.Kind),
		Exported: false,
	}

	// Parse documentation
	if len(sym.Documentation) > 0 {
		info.Documentation = strings.Join(sym.Documentation, "\n")
	}

	// Extract file path from symbol ID
	// SCIP symbol format: scip-go gomod ... pkg/path/file.go/SymbolName
	info.FilePath = extractFilePathFromSymbol(sym.Symbol)

	// Determine if exported based on naming conventions
	info.Exported = isExported(info.Name, info.Kind)

	return info
}

// extractNameFromSymbol extracts the symbol name from a SCIP symbol ID.
func extractNameFromSymbol(symbolID string) string {
	if symbolID == "" {
		return ""
	}

	// SCIP symbol format varies by language, but generally the last part is the name
	// Examples:
	//   scip-go gomod github.com/example/pkg v1.0.0 internal/query/engine.go Engine.
	//   scip-go gomod example.com/pkg v0.0.0 pkg/types/user.go User#Name.
	// The name is typically after the last / or # and before trailing .

	// Find the last component
	lastSlash := strings.LastIndex(symbolID, "/")
	lastHash := strings.LastIndex(symbolID, "#")
	lastParen := strings.LastIndex(symbolID, "(")

	startIdx := 0
	if lastHash > lastSlash {
		startIdx = lastHash + 1
	} else if lastSlash >= 0 {
		startIdx = lastSlash + 1
	}

	name := symbolID[startIdx:]

	// Remove method signature (parentheses and after)
	if lastParen > startIdx {
		name = name[:lastParen-startIdx]
	}

	// Remove trailing . or ` or other markers
	name = strings.TrimSuffix(name, ".")
	name = strings.TrimSuffix(name, "`")
	name = strings.TrimSuffix(name, "()")

	return name
}

// extractFilePathFromSymbol extracts the file path from a SCIP symbol ID.
func extractFilePathFromSymbol(symbolID string) string {
	// SCIP symbol format varies, but we can often find the file path
	// For Go: scip-go gomod example.com/pkg v1.0.0 path/to/file.go/SymbolName
	parts := strings.Split(symbolID, " ")
	for _, part := range parts {
		if strings.HasSuffix(part, ".go") ||
			strings.HasSuffix(part, ".ts") ||
			strings.HasSuffix(part, ".js") ||
			strings.HasSuffix(part, ".py") {
			// Extract up to the file extension
			idx := strings.LastIndex(part, "/")
			if idx > 0 {
				// Check if this looks like a path
				for i := idx + 1; i < len(part); i++ {
					if part[i] == '/' {
						return part[:i]
					}
				}
			}
			return part
		}
	}

	// Fallback: try to find anything that looks like a path
	for _, part := range parts {
		if strings.Contains(part, "/") && !strings.HasPrefix(part, "scip-") {
			return part
		}
	}

	return ""
}

// isExported determines if a symbol is exported based on language conventions.
func isExported(name, kind string) bool {
	if name == "" {
		return false
	}

	// Skip local variables and parameters
	if kind == "variable" || kind == "parameter" {
		return false
	}

	// Go convention: uppercase first letter means exported
	firstChar := rune(name[0])
	return firstChar >= 'A' && firstChar <= 'Z'
}

// kindToString converts SCIP kind int to string.
func kindToString(kind int32) string {
	switch kind {
	case 1:
		return "package"
	case 2:
		return "type"
	case 3:
		return "term" // could be variable, constant, etc.
	case 4:
		return "method"
	case 5:
		return "type_parameter"
	case 6:
		return "parameter"
	case 7:
		return "self_parameter"
	case 8:
		return "attr"
	case 9:
		return "macro"
	default:
		return "unknown"
	}
}

// isInScope checks if a file path is within the given scope.
func (a *Analyzer) isInScope(filePath string, scope []string) bool {
	if len(scope) == 0 {
		return true
	}

	for _, s := range scope {
		if strings.HasPrefix(filePath, s) {
			return true
		}
		// Also check with trailing slash
		if strings.HasPrefix(filePath, s+"/") {
			return true
		}
	}
	return false
}

// categorizeReferences categorizes references to a symbol.
func (a *Analyzer) categorizeReferences(refs []*scip.SCIPReference, symbolID, symbolFilePath string) ReferenceStats {
	stats := ReferenceStats{}

	symbolDir := filepath.Dir(symbolFilePath)

	for _, ref := range refs {
		if ref == nil || ref.Location == nil {
			continue
		}

		stats.Total++
		refPath := ref.Location.FileId

		// Self-reference check
		if ref.SymbolId == symbolID && refPath == symbolFilePath {
			stats.FromSelf++
			continue
		}

		// Test file check
		if IsTestFile(refPath) {
			stats.FromTests++
			continue
		}

		// External vs internal check
		refDir := filepath.Dir(refPath)
		if refDir != symbolDir {
			stats.External++
		} else {
			stats.Internal++
		}
	}

	return stats
}

// classifySymbol determines if a symbol is dead and returns the classification.
func (a *Analyzer) classifySymbol(sym SymbolInfo, stats ReferenceStats, opts AnalyzerOptions) (DeadCodeItem, bool) {
	item := DeadCodeItem{
		SymbolID:       "", // We don't have the ID in SymbolInfo currently
		SymbolName:     sym.Name,
		Kind:           sym.Kind,
		FilePath:       sym.FilePath,
		ReferenceCount: stats.Total,
		TestReferences: stats.FromTests,
		SelfReferences: stats.FromSelf,
		Exported:       sym.Exported,
	}

	// Calculate non-self, non-definition references
	nonSelfRefs := stats.Total - stats.FromSelf

	// Zero references (excluding self) = definitely dead
	if nonSelfRefs == 0 {
		if stats.FromSelf > 0 {
			item.Category = CategorySelfOnly
			item.Reason = "Only referenced by itself (recursive but never called)"
			item.Confidence = 0.95
		} else {
			item.Category = CategoryZeroRefs
			item.Reason = "No references found"
			item.Confidence = 0.99
		}
		return item, true
	}

	// Only test references (unless excluded)
	if !opts.ExcludeTestOnly && stats.FromTests == nonSelfRefs {
		item.Category = CategoryTestOnly
		item.Reason = "Only referenced from test files"
		item.Confidence = 0.75
		return item, true
	}

	// Exported but only used internally
	if sym.Exported && stats.External == 0 && stats.Internal > 0 {
		item.Category = CategoryInternalExport
		item.Reason = "Exported but only used within same package"
		item.Confidence = 0.60
		return item, true
	}

	return item, false
}

// computeSummary calculates aggregate statistics for the results.
func (a *Analyzer) computeSummary(deadCode []DeadCodeItem, totalAnalyzed int) DeadCodeSummary {
	summary := DeadCodeSummary{
		TotalSymbols: totalAnalyzed,
		DeadCount:    0,
		ByKind:       make(map[string]int),
		ByCategory:   make(map[string]int),
	}

	estimatedLines := 0
	for _, item := range deadCode {
		if item.Confidence >= 0.9 {
			summary.DeadCount++
		} else {
			summary.SuspiciousCount++
		}

		summary.ByKind[item.Kind]++
		summary.ByCategory[string(item.Category)]++

		// Rough estimate: functions ~20 lines, types ~30 lines, others ~5 lines
		switch item.Kind {
		case "function", "method":
			estimatedLines += 20
		case "type", "class", "interface":
			estimatedLines += 30
		default:
			estimatedLines += 5
		}
	}

	summary.EstimatedLines = estimatedLines
	return summary
}

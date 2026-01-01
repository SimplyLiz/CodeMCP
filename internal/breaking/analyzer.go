package breaking

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ckb/internal/backends/scip"
	"ckb/internal/logging"
)

// Analyzer compares API surfaces between git refs
type Analyzer struct {
	scipAdapter *scip.SCIPAdapter
	logger      *logging.Logger
	repoRoot    string
}

// NewAnalyzer creates a new breaking change analyzer
func NewAnalyzer(scipAdapter *scip.SCIPAdapter, repoRoot string, logger *logging.Logger) *Analyzer {
	return &Analyzer{
		scipAdapter: scipAdapter,
		logger:      logger,
		repoRoot:    repoRoot,
	}
}

// Compare analyzes API changes between two versions
func (a *Analyzer) Compare(ctx context.Context, opts CompareOptions) (*CompareResult, error) {
	if opts.IgnorePrivate {
		opts.IgnorePrivate = true // Default to true
	}

	a.logger.Debug("Starting breaking change analysis", map[string]interface{}{
		"baseRef":   opts.BaseRef,
		"targetRef": opts.TargetRef,
		"scope":     opts.Scope,
	})

	// For now, we can only analyze the current state (targetRef)
	// Full git ref comparison would require indexing at different commits
	// This simplified version analyzes current exported symbols

	targetSymbols := a.extractAPISymbols(opts)

	// Build the result with current analysis
	// Note: Full implementation would diff two SCIP indexes
	result := &CompareResult{
		BaseRef:            opts.BaseRef,
		TargetRef:          opts.TargetRef,
		Changes:            []APIChange{},
		TotalBaseSymbols:   0, // Would need to index base ref
		TotalTargetSymbols: len(targetSymbols),
	}

	// For now, report all symbols as the API surface
	// A full implementation would compare base vs target
	a.logger.Debug("API analysis completed", map[string]interface{}{
		"targetSymbols": len(targetSymbols),
	})

	result.Summary = a.computeSummary(result.Changes)
	result.SemverAdvice = a.computeSemverAdvice(result.Summary)

	return result, nil
}

// CompareSymbolSets compares two sets of API symbols directly
func (a *Analyzer) CompareSymbolSets(baseSymbols, targetSymbols []APISymbol) *CompareResult {
	changes := []APIChange{}

	// Build lookup maps
	baseMap := make(map[string]APISymbol)
	for _, sym := range baseSymbols {
		key := a.symbolKey(sym)
		baseMap[key] = sym
	}

	targetMap := make(map[string]APISymbol)
	for _, sym := range targetSymbols {
		key := a.symbolKey(sym)
		targetMap[key] = sym
	}

	// Find removed symbols (in base but not in target)
	for key, baseSym := range baseMap {
		if _, exists := targetMap[key]; !exists {
			// Check if renamed (same package, similar name)
			if newName := a.findPotentialRename(baseSym, targetSymbols); newName != "" {
				changes = append(changes, APIChange{
					Kind:         ChangeRenamed,
					Severity:     SeverityBreaking,
					SymbolName:   baseSym.Name,
					SymbolKind:   baseSym.Kind,
					Package:      baseSym.Package,
					FilePath:     baseSym.FilePath,
					LineNumber:   baseSym.LineNumber,
					Description:  fmt.Sprintf("Symbol '%s' was renamed to '%s'", baseSym.Name, newName),
					OldValue:     baseSym.Name,
					NewValue:     newName,
					AffectsUsers: true,
				})
			} else {
				changes = append(changes, APIChange{
					Kind:         ChangeRemoved,
					Severity:     SeverityBreaking,
					SymbolName:   baseSym.Name,
					SymbolKind:   baseSym.Kind,
					Package:      baseSym.Package,
					FilePath:     baseSym.FilePath,
					LineNumber:   baseSym.LineNumber,
					Description:  fmt.Sprintf("Symbol '%s' was removed", baseSym.Name),
					AffectsUsers: baseSym.Exported,
				})
			}
		}
	}

	// Find added and changed symbols
	for key, targetSym := range targetMap {
		if baseSym, exists := baseMap[key]; exists {
			// Check for signature changes
			if baseSym.Signature != targetSym.Signature && baseSym.Signature != "" && targetSym.Signature != "" {
				changes = append(changes, APIChange{
					Kind:         ChangeSignatureChanged,
					Severity:     SeverityBreaking,
					SymbolName:   targetSym.Name,
					SymbolKind:   targetSym.Kind,
					Package:      targetSym.Package,
					FilePath:     targetSym.FilePath,
					LineNumber:   targetSym.LineNumber,
					Description:  fmt.Sprintf("Signature of '%s' changed", targetSym.Name),
					OldValue:     baseSym.Signature,
					NewValue:     targetSym.Signature,
					AffectsUsers: targetSym.Exported,
				})
			}

			// Check for type changes
			if baseSym.TypeSignature != targetSym.TypeSignature && baseSym.TypeSignature != "" && targetSym.TypeSignature != "" {
				changes = append(changes, APIChange{
					Kind:         ChangeTypeChanged,
					Severity:     SeverityBreaking,
					SymbolName:   targetSym.Name,
					SymbolKind:   targetSym.Kind,
					Package:      targetSym.Package,
					FilePath:     targetSym.FilePath,
					LineNumber:   targetSym.LineNumber,
					Description:  fmt.Sprintf("Type of '%s' changed", targetSym.Name),
					OldValue:     baseSym.TypeSignature,
					NewValue:     targetSym.TypeSignature,
					AffectsUsers: targetSym.Exported,
				})
			}

			// Check for visibility changes
			if baseSym.Exported && !targetSym.Exported {
				changes = append(changes, APIChange{
					Kind:         ChangeVisibilityChanged,
					Severity:     SeverityBreaking,
					SymbolName:   targetSym.Name,
					SymbolKind:   targetSym.Kind,
					Package:      targetSym.Package,
					FilePath:     targetSym.FilePath,
					LineNumber:   targetSym.LineNumber,
					Description:  fmt.Sprintf("'%s' is no longer exported", targetSym.Name),
					OldValue:     "exported",
					NewValue:     "unexported",
					AffectsUsers: true,
				})
			}

			// Check for deprecation
			if !baseSym.Deprecated && targetSym.Deprecated {
				changes = append(changes, APIChange{
					Kind:         ChangeDeprecated,
					Severity:     SeverityWarning,
					SymbolName:   targetSym.Name,
					SymbolKind:   targetSym.Kind,
					Package:      targetSym.Package,
					FilePath:     targetSym.FilePath,
					LineNumber:   targetSym.LineNumber,
					Description:  fmt.Sprintf("'%s' has been deprecated", targetSym.Name),
					AffectsUsers: targetSym.Exported,
				})
			}
		} else {
			// New symbol added
			changes = append(changes, APIChange{
				Kind:         ChangeAdded,
				Severity:     SeverityNonBreaking,
				SymbolName:   targetSym.Name,
				SymbolKind:   targetSym.Kind,
				Package:      targetSym.Package,
				FilePath:     targetSym.FilePath,
				LineNumber:   targetSym.LineNumber,
				Description:  fmt.Sprintf("New symbol '%s' added", targetSym.Name),
				AffectsUsers: false,
			})
		}
	}

	// Sort changes by severity, then by symbol name
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Severity != changes[j].Severity {
			return severityOrder(changes[i].Severity) < severityOrder(changes[j].Severity)
		}
		return changes[i].SymbolName < changes[j].SymbolName
	})

	result := &CompareResult{
		Changes:            changes,
		TotalBaseSymbols:   len(baseSymbols),
		TotalTargetSymbols: len(targetSymbols),
	}
	result.Summary = a.computeSummary(changes)
	result.SemverAdvice = a.computeSemverAdvice(result.Summary)

	return result
}

func severityOrder(s Severity) int {
	switch s {
	case SeverityBreaking:
		return 0
	case SeverityWarning:
		return 1
	case SeverityNonBreaking:
		return 2
	default:
		return 3
	}
}

// extractAPISymbols extracts all exported symbols from the current SCIP index
func (a *Analyzer) extractAPISymbols(opts CompareOptions) []APISymbol {
	if a.scipAdapter == nil || !a.scipAdapter.IsAvailable() {
		return nil
	}

	allSymbols := a.scipAdapter.AllSymbols()
	if allSymbols == nil {
		return nil
	}

	var apiSymbols []APISymbol

	for _, sym := range allSymbols {
		if sym.Symbol == "" {
			continue
		}

		// Extract symbol info
		name := extractNameFromSymbol(sym.Symbol)
		if name == "" {
			continue
		}

		// Check if exported (Go: uppercase first letter)
		exported := isExported(name)
		if opts.IgnorePrivate && !exported {
			continue
		}

		filePath := extractFilePathFromSymbol(sym.Symbol)

		// Check scope
		if len(opts.Scope) > 0 && !isInScope(filePath, opts.Scope) {
			continue
		}

		// Skip test files
		if isTestFile(filePath) {
			continue
		}

		kind := kindToString(sym.Kind)
		if kind == "unknown" || kind == "parameter" || kind == "variable" {
			continue
		}

		apiSymbol := APISymbol{
			Name:     name,
			Kind:     kind,
			Package:  extractPackageFromSymbol(sym.Symbol),
			FilePath: filePath,
			Exported: exported,
		}

		// Extract signature from display name or documentation
		if len(sym.Documentation) > 0 {
			apiSymbol.Documentation = strings.Join(sym.Documentation, "\n")
			// Check for deprecation markers
			for _, doc := range sym.Documentation {
				if strings.Contains(strings.ToLower(doc), "deprecated") {
					apiSymbol.Deprecated = true
					break
				}
			}
		}

		if sym.DisplayName != "" {
			if kind == "function" || kind == "method" {
				apiSymbol.Signature = sym.DisplayName
			} else if kind == "type" {
				apiSymbol.TypeSignature = sym.DisplayName
			}
		}

		apiSymbols = append(apiSymbols, apiSymbol)
	}

	return apiSymbols
}

// symbolKey generates a unique key for a symbol
func (a *Analyzer) symbolKey(sym APISymbol) string {
	return fmt.Sprintf("%s.%s.%s", sym.Package, sym.Kind, sym.Name)
}

// findPotentialRename looks for a similar symbol that might be a rename
func (a *Analyzer) findPotentialRename(baseSym APISymbol, targetSymbols []APISymbol) string {
	// Look for same kind, same package, similar structure
	for _, target := range targetSymbols {
		if target.Kind != baseSym.Kind || target.Package != baseSym.Package {
			continue
		}
		// Similar signature suggests a rename
		if baseSym.Signature != "" && baseSym.Signature == target.Signature {
			return target.Name
		}
	}
	return ""
}

// computeSummary calculates summary statistics
func (a *Analyzer) computeSummary(changes []APIChange) *Summary {
	summary := &Summary{
		TotalChanges: len(changes),
		ByKind:       make(map[string]int),
		ByPackage:    make(map[string]int),
	}

	for _, change := range changes {
		summary.ByKind[string(change.Kind)]++
		if change.Package != "" {
			summary.ByPackage[change.Package]++
		}

		switch change.Severity {
		case SeverityBreaking:
			summary.BreakingChanges++
		case SeverityWarning:
			summary.Warnings++
		case SeverityNonBreaking:
			summary.Additions++
		}
	}

	return summary
}

// computeSemverAdvice suggests the appropriate version bump
func (a *Analyzer) computeSemverAdvice(summary *Summary) string {
	if summary.BreakingChanges > 0 {
		return "major"
	}
	if summary.Additions > 0 {
		return "minor"
	}
	return "patch"
}

// Helper functions

func extractNameFromSymbol(symbolID string) string {
	if symbolID == "" {
		return ""
	}
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
	if lastParen > startIdx {
		name = name[:lastParen-startIdx]
	}
	name = strings.TrimSuffix(name, ".")
	name = strings.TrimSuffix(name, "`")
	name = strings.TrimSuffix(name, "()")
	return name
}

func extractFilePathFromSymbol(symbolID string) string {
	parts := strings.Split(symbolID, " ")
	for _, part := range parts {
		if strings.HasSuffix(part, ".go") ||
			strings.HasSuffix(part, ".ts") ||
			strings.HasSuffix(part, ".js") ||
			strings.HasSuffix(part, ".py") {
			idx := strings.LastIndex(part, "/")
			if idx > 0 {
				for i := idx + 1; i < len(part); i++ {
					if part[i] == '/' {
						return part[:i]
					}
				}
			}
			return part
		}
	}
	return ""
}

func extractPackageFromSymbol(symbolID string) string {
	parts := strings.Split(symbolID, " ")
	for i, part := range parts {
		if strings.HasSuffix(part, ".go") {
			// Package is typically before the file
			if i > 0 {
				prev := parts[i-1]
				if strings.Contains(prev, "/") {
					lastSlash := strings.LastIndex(prev, "/")
					return prev[lastSlash+1:]
				}
				return prev
			}
		}
	}
	return ""
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	firstChar := rune(name[0])
	return firstChar >= 'A' && firstChar <= 'Z'
}

func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".spec.ts") ||
		strings.HasSuffix(path, "_test.py") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/__tests__/")
}

func isInScope(filePath string, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, s := range scope {
		if strings.HasPrefix(filePath, s) || strings.HasPrefix(filePath, s+"/") {
			return true
		}
	}
	return false
}

func kindToString(kind int32) string {
	switch kind {
	case 1:
		return "package"
	case 2:
		return "type"
	case 3:
		return "term"
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

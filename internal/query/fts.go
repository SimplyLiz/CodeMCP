package query

import (
	"context"
	"strings"
	"time"

	"ckb/internal/backends/scip"
	"ckb/internal/storage"
)

// PopulateFTSFromSCIP populates the FTS5 symbol index from the loaded SCIP index.
// This should be called after the SCIP adapter loads its index.
func (e *Engine) PopulateFTSFromSCIP(ctx context.Context) error {
	if e.scipAdapter == nil || !e.scipAdapter.IsAvailable() {
		e.logger.Debug("Skipping FTS population - SCIP adapter not available", nil)
		return nil
	}

	start := time.Now()

	// Get the SCIP index
	index := e.scipAdapter.GetIndex()
	if index == nil {
		e.logger.Debug("Skipping FTS population - no SCIP index loaded", nil)
		return nil
	}

	// Convert SCIP symbols to FTS records
	var records []storage.SymbolFTSRecord
	for _, symInfo := range index.Symbols {
		// Convert SymbolInformation to FTS record
		record := convertSymbolToFTSRecord(symInfo, index)
		records = append(records, record)
	}

	if len(records) == 0 {
		e.logger.Debug("No symbols to index for FTS", nil)
		return nil
	}

	// Get FTS manager from DB
	ftsManager := storage.NewFTSManager(e.db.Conn(), storage.DefaultFTSConfig())

	// Initialize schema if needed
	if err := ftsManager.InitSchema(); err != nil {
		e.logger.Warn("Failed to initialize FTS schema", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	// Bulk insert symbols
	if err := ftsManager.BulkInsert(ctx, records); err != nil {
		e.logger.Warn("Failed to populate FTS index", map[string]interface{}{
			"error":        err.Error(),
			"symbol_count": len(records),
		})
		return err
	}

	e.logger.Info("FTS index populated from SCIP", map[string]interface{}{
		"symbol_count": len(records),
		"duration_ms":  time.Since(start).Milliseconds(),
	})

	return nil
}

// convertSymbolToFTSRecord converts a SCIP SymbolInformation to an FTS record
func convertSymbolToFTSRecord(symInfo *scip.SymbolInformation, index *scip.SCIPIndex) storage.SymbolFTSRecord {
	// Parse the SCIP identifier to extract useful info
	scipId, _ := scip.ParseSCIPIdentifier(symInfo.Symbol)

	// Get display name
	name := symInfo.DisplayName
	if name == "" && scipId != nil {
		name = scipId.GetSimpleName()
	}

	// Get kind string
	kind := inferKindString(symInfo.Kind, scipId)

	// Get documentation
	documentation := strings.Join(symInfo.Documentation, "\n")

	// Get file path from definition location
	filePath := ""
	language := ""
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			if occ.Symbol == symInfo.Symbol && occ.SymbolRoles&scip.SymbolRoleDefinition != 0 {
				filePath = doc.RelativePath
				language = doc.Language
				break
			}
		}
		if filePath != "" {
			break
		}
	}

	// Build signature from display name and enclosing symbol
	signature := name
	if symInfo.EnclosingSymbol != "" {
		if enclosingId, err := scip.ParseSCIPIdentifier(symInfo.EnclosingSymbol); err == nil {
			signature = enclosingId.GetSimpleName() + "." + name
		}
	}

	return storage.SymbolFTSRecord{
		ID:            symInfo.Symbol,
		Name:          name,
		Kind:          kind,
		Documentation: documentation,
		Signature:     signature,
		FilePath:      filePath,
		Language:      language,
	}
}

// inferKindString converts the SCIP kind int32 to a string
func inferKindString(kind int32, scipId *scip.SCIPIdentifier) string {
	switch kind {
	case 1:
		return "class"
	case 2:
		return "interface"
	case 3:
		return "enum"
	case 6:
		return "function"
	case 7:
		return "variable"
	case 8:
		return "constant"
	case 9:
		return "method"
	case 10:
		return "property"
	case 11:
		return "field"
	case 12:
		return "parameter"
	case 19:
		return "namespace"
	case 20:
		return "package"
	case 21:
		return "type"
	default:
		// Fall back to descriptor-based inference
		if scipId != nil {
			return string(scipId.ExtractSymbolKind())
		}
		return "unknown"
	}
}

// SearchSymbolsFTS performs FTS5-accelerated symbol search.
// Returns results from FTS if available, falls back to nil if not.
func (e *Engine) SearchSymbolsFTS(ctx context.Context, query string, limit int) ([]storage.FTSSearchResult, error) {
	// Get FTS manager
	ftsManager := storage.NewFTSManager(e.db.Conn(), storage.DefaultFTSConfig())

	// Check if FTS has data
	stats, err := ftsManager.GetStats(ctx)
	if err != nil {
		return nil, nil
	}

	indexedSymbols, ok := stats["indexed_symbols"].(int)
	if !ok || indexedSymbols == 0 {
		// No FTS data available
		return nil, nil
	}

	// Perform FTS search
	return ftsManager.Search(ctx, query, limit)
}

// RefreshFTS rebuilds the FTS index from current SCIP data.
func (e *Engine) RefreshFTS(ctx context.Context) error {
	return e.PopulateFTSFromSCIP(ctx)
}

// GetFTSStats returns statistics about the FTS index
func (e *Engine) GetFTSStats(ctx context.Context) (map[string]interface{}, error) {
	ftsManager := storage.NewFTSManager(e.db.Conn(), storage.DefaultFTSConfig())
	return ftsManager.GetStats(ctx)
}

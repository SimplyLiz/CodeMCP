package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ckb/internal/errors"

	scippb "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// SCIPIndex represents a loaded SCIP index
type SCIPIndex struct {
	// Metadata contains index metadata
	Metadata *Metadata

	// Documents are all indexed documents
	Documents []*Document

	// Symbols maps symbol IDs to symbol information
	Symbols map[string]*SymbolInformation

	// LoadedAt is when the index was loaded
	LoadedAt time.Time

	// IndexedCommit is the git commit the index was built from
	IndexedCommit string

	// raw is the raw protobuf index
	raw *scippb.Index
}

// LoadSCIPIndex loads a SCIP index from the specified path
func LoadSCIPIndex(path string) (*SCIPIndex, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.NewCkbError(
			errors.IndexMissing,
			fmt.Sprintf("SCIP index not found at %s", path),
			err,
			errors.GetSuggestedFixes(errors.IndexMissing),
			nil,
		)
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			fmt.Sprintf("Failed to read SCIP index from %s", path),
			err,
			nil,
			nil,
		)
	}

	// Parse protobuf
	var index scippb.Index
	if err := proto.Unmarshal(data, &index); err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			fmt.Sprintf("Failed to parse SCIP index from %s", path),
			err,
			[]errors.FixAction{
				{
					Type:        errors.RunCommand,
					Command:     "scip print --index=" + path,
					Safe:        true,
					Description: "Verify SCIP index is valid",
				},
			},
			nil,
		)
	}

	// Convert to internal representation
	scipIndex := &SCIPIndex{
		Metadata:  convertMetadata(index.Metadata),
		Documents: convertDocuments(index.Documents),
		Symbols:   make(map[string]*SymbolInformation),
		LoadedAt:  time.Now(),
		raw:       &index,
	}

	// Build symbol map
	for _, doc := range scipIndex.Documents {
		for _, sym := range doc.Symbols {
			scipIndex.Symbols[sym.Symbol] = sym
		}
	}

	// Extract indexed commit from metadata if available
	if scipIndex.Metadata != nil && scipIndex.Metadata.ToolInfo != nil {
		scipIndex.IndexedCommit = extractCommitFromToolInfo(scipIndex.Metadata.ToolInfo)
	}

	return scipIndex, nil
}

// IsStale checks if the index is stale compared to the current HEAD commit
func (i *SCIPIndex) IsStale(headCommit string) bool {
	// If we don't know the indexed commit, assume it's stale
	if i.IndexedCommit == "" {
		return true
	}

	// Compare with HEAD
	return i.IndexedCommit != headCommit
}

// GetDocument retrieves a document by its relative path
func (i *SCIPIndex) GetDocument(relativePath string) *Document {
	for _, doc := range i.Documents {
		if doc.RelativePath == relativePath {
			return doc
		}
	}
	return nil
}

// GetSymbol retrieves symbol information by ID
func (i *SCIPIndex) GetSymbol(symbolId string) *SymbolInformation {
	return i.Symbols[symbolId]
}

// AllSymbols returns all symbols in the index
func (i *SCIPIndex) AllSymbols() []*SymbolInformation {
	symbols := make([]*SymbolInformation, 0, len(i.Symbols))
	for _, sym := range i.Symbols {
		symbols = append(symbols, sym)
	}
	return symbols
}

// convertMetadata converts protobuf metadata to internal representation
func convertMetadata(meta *scippb.Metadata) *Metadata {
	if meta == nil {
		return nil
	}

	var toolInfo *ToolInfo
	if meta.ToolInfo != nil {
		toolInfo = &ToolInfo{
			Name:      meta.ToolInfo.Name,
			Version:   meta.ToolInfo.Version,
			Arguments: meta.ToolInfo.Arguments,
		}
	}

	return &Metadata{
		Version:              fmt.Sprintf("%d", meta.Version),
		ToolInfo:             toolInfo,
		ProjectRoot:          meta.ProjectRoot,
		TextDocumentEncoding: meta.TextDocumentEncoding.String(),
	}
}

// convertDocuments converts protobuf documents to internal representation
func convertDocuments(docs []*scippb.Document) []*Document {
	result := make([]*Document, len(docs))
	for i, doc := range docs {
		result[i] = convertDocument(doc)
	}
	return result
}

// convertDocument converts a single protobuf document
func convertDocument(doc *scippb.Document) *Document {
	occurrences := make([]*Occurrence, len(doc.Occurrences))
	for i, occ := range doc.Occurrences {
		occurrences[i] = convertOccurrence(occ)
	}

	symbols := make([]*SymbolInformation, len(doc.Symbols))
	for i, sym := range doc.Symbols {
		symbols[i] = convertSymbolInformation(sym)
	}

	return &Document{
		RelativePath: doc.RelativePath,
		Language:     doc.Language,
		Occurrences:  occurrences,
		Symbols:      symbols,
	}
}

// convertOccurrence converts a protobuf occurrence
func convertOccurrence(occ *scippb.Occurrence) *Occurrence {
	diagnostics := make([]*Diagnostic, len(occ.Diagnostics))
	for i, diag := range occ.Diagnostics {
		// Convert DiagnosticTag slice to int32 slice
		tags := make([]int32, len(diag.Tags))
		for j, tag := range diag.Tags {
			tags[j] = int32(tag)
		}

		diagnostics[i] = &Diagnostic{
			Severity: int32(diag.Severity),
			Code:     diag.Code,
			Message:  diag.Message,
			Source:   diag.Source,
			Tags:     tags,
		}
	}

	return &Occurrence{
		Range:                 occ.Range,
		Symbol:                occ.Symbol,
		SymbolRoles:           occ.SymbolRoles,
		OverrideDocumentation: occ.OverrideDocumentation,
		SyntaxKind:            int32(occ.SyntaxKind),
		Diagnostics:           diagnostics,
		EnclosingRange:        occ.EnclosingRange,
	}
}

// convertSymbolInformation converts protobuf symbol information
func convertSymbolInformation(sym *scippb.SymbolInformation) *SymbolInformation {
	relationships := make([]*Relationship, len(sym.Relationships))
	for i, rel := range sym.Relationships {
		relationships[i] = &Relationship{
			Symbol:           rel.Symbol,
			IsReference:      rel.IsReference,
			IsImplementation: rel.IsImplementation,
			IsTypeDefinition: rel.IsTypeDefinition,
			IsDefinition:     rel.IsDefinition,
		}
	}

	return &SymbolInformation{
		Symbol:                 sym.Symbol,
		Documentation:          sym.Documentation,
		Relationships:          relationships,
		Kind:                   int32(sym.Kind),
		DisplayName:            sym.DisplayName,
		SignatureDocumentation: nil, // Skip signature documentation for now
		EnclosingSymbol:        sym.EnclosingSymbol,
	}
}

// extractCommitFromToolInfo attempts to extract git commit from tool info
func extractCommitFromToolInfo(toolInfo *ToolInfo) string {
	// Look for commit hash in arguments
	// Common patterns:
	// --commit=<hash>
	// --git-commit=<hash>
	// --module-version=<hash> (scip-go)
	// -c <hash>
	for i, arg := range toolInfo.Arguments {
		if len(arg) > 9 && arg[:9] == "--commit=" {
			return arg[9:]
		}
		if len(arg) > 13 && arg[:13] == "--git-commit=" {
			return arg[13:]
		}
		if len(arg) > 17 && arg[:17] == "--module-version=" {
			return arg[17:]
		}
		if arg == "-c" && i+1 < len(toolInfo.Arguments) {
			return toolInfo.Arguments[i+1]
		}
	}

	// Also check version field which scip-go populates
	if toolInfo.Version != "" && looksLikeCommitHash(toolInfo.Version) {
		return toolInfo.Version
	}

	return ""
}

// looksLikeCommitHash checks if a string looks like a git commit hash
func looksLikeCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetIndexPath returns the index path from config and repo root
func GetIndexPath(repoRoot string, configPath string) string {
	if filepath.IsAbs(configPath) {
		return configPath
	}
	return filepath.Join(repoRoot, configPath)
}

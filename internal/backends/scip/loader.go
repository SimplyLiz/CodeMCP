package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/errors"

	scippb "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// OccurrenceRef is a lightweight reference to an occurrence for the inverted index
type OccurrenceRef struct {
	Doc *Document
	Occ *Occurrence
}

// SCIPIndex represents a loaded SCIP index
type SCIPIndex struct {
	// Metadata contains index metadata
	Metadata *Metadata

	// Documents are all indexed documents
	Documents []*Document

	// DocumentsByPath is an O(1) lookup map from relative path to document
	DocumentsByPath map[string]*Document

	// Symbols maps symbol IDs to symbol information
	Symbols map[string]*SymbolInformation

	// RefIndex is an inverted index for O(1) reference lookups: symbolId -> occurrences
	RefIndex map[string][]*OccurrenceRef

	// ConvertedSymbols caches pre-converted SCIPSymbol objects to avoid repeated conversion
	ConvertedSymbols map[string]*SCIPSymbol

	// ContainerIndex maps occurrence positions to their containing symbol for O(1) lookup
	// Key format: "docPath:line:col" -> containerSymbolId
	ContainerIndex map[string]string

	// LoadedAt is when the index was loaded
	LoadedAt time.Time

	// IndexedCommit is the git commit the index was built from
	IndexedCommit string
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

	// Convert to internal representation using parallel document processing.
	nWorkers := runtime.GOMAXPROCS(0)

	// Phase 1: convert documents and build per-doc indexes in parallel.
	type docResult struct {
		doc              *Document
		symbols          map[string]*SymbolInformation
		refEntries       map[string][]*OccurrenceRef
		containerEntries map[string]string
	}

	results := make([]docResult, len(index.Documents))

	var wg sync.WaitGroup
	sem := make(chan struct{}, nWorkers)

	for i, pbDoc := range index.Documents {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, pbDoc *scippb.Document) {
			defer wg.Done()
			defer func() { <-sem }()

			doc := convertDocument(pbDoc)
			r := docResult{
				doc:              doc,
				symbols:          make(map[string]*SymbolInformation, len(doc.Symbols)),
				refEntries:       make(map[string][]*OccurrenceRef),
				containerEntries: make(map[string]string),
			}

			// Index symbols
			for _, sym := range doc.Symbols {
				r.symbols[sym.Symbol] = sym
			}

			// Build inverted reference index for O(1) lookups
			for _, occ := range doc.Occurrences {
				if occ.Symbol != "" {
					r.refEntries[occ.Symbol] = append(
						r.refEntries[occ.Symbol],
						&OccurrenceRef{Doc: doc, Occ: occ},
					)
				}
			}

			// Build container index.
			// Collect definition occurrences that have enclosing ranges.
			type defScope struct {
				symbol    string
				startLine int32
				endLine   int32
			}
			var defScopes []defScope
			for _, occ := range doc.Occurrences {
				if occ.SymbolRoles&SymbolRoleDefinition != 0 && len(occ.EnclosingRange) >= 3 {
					startLine := occ.EnclosingRange[0]
					var endLine int32
					if len(occ.EnclosingRange) >= 4 {
						endLine = occ.EnclosingRange[2]
					} else {
						endLine = startLine
					}
					defScopes = append(defScopes, defScope{
						symbol:    occ.Symbol,
						startLine: startLine,
						endLine:   endLine,
					})
				}
			}

			if len(defScopes) > 0 {
				// Sort by scope size ascending so the first match is the innermost.
				sort.Slice(defScopes, func(a, b int) bool {
					return (defScopes[a].endLine - defScopes[a].startLine) <
						(defScopes[b].endLine - defScopes[b].startLine)
				})

				for _, occ := range doc.Occurrences {
					if len(occ.Range) < 2 {
						continue
					}
					occLine := occ.Range[0]
					for idx := range defScopes {
						ds := &defScopes[idx]
						if occLine >= ds.startLine && occLine <= ds.endLine {
							key := fmt.Sprintf("%s:%d:%d", doc.RelativePath, occ.Range[0], occ.Range[1])
							r.containerEntries[key] = ds.symbol
							break // first match is innermost (sorted by size asc)
						}
					}
				}
			}

			results[i] = r
		}(i, pbDoc)
	}
	wg.Wait()

	// Merge per-doc results into the main index (serial, fast map assignment).
	// Pre-size maps based on doc count to reduce rehashing.
	totalSyms := 0
	totalRefs := 0
	totalContainer := 0
	docs := make([]*Document, len(results))
	for i, r := range results {
		docs[i] = r.doc
		totalSyms += len(r.symbols)
		totalRefs += len(r.refEntries)
		totalContainer += len(r.containerEntries)
	}

	scipIndex := &SCIPIndex{
		Metadata:        convertMetadata(index.Metadata),
		Documents:       docs,
		DocumentsByPath: make(map[string]*Document, len(docs)),
		Symbols:         make(map[string]*SymbolInformation, totalSyms),
		RefIndex:        make(map[string][]*OccurrenceRef, totalRefs),
		ConvertedSymbols: make(map[string]*SCIPSymbol, totalSyms),
		ContainerIndex:  make(map[string]string, totalContainer),
		LoadedAt:        time.Now(),
	}

	for _, doc := range docs {
		scipIndex.DocumentsByPath[doc.RelativePath] = doc
	}
	for _, r := range results {
		for k, v := range r.symbols {
			scipIndex.Symbols[k] = v
		}
		for k, v := range r.refEntries {
			scipIndex.RefIndex[k] = append(scipIndex.RefIndex[k], v...)
		}
		for k, v := range r.containerEntries {
			scipIndex.ContainerIndex[k] = v
		}
	}

	// Phase 2: pre-convert all symbols in parallel.
	// RefIndex and Symbols are fully built at this point (read-only from here).
	type symResult struct {
		id  string
		sym *SCIPSymbol
	}

	symIDs := make([]string, 0, len(scipIndex.Symbols))
	for id := range scipIndex.Symbols {
		symIDs = append(symIDs, id)
	}

	symCh := make(chan symResult, len(symIDs))
	batchSize := (len(symIDs) + nWorkers - 1) / nWorkers
	if batchSize < 1 {
		batchSize = 1
	}

	var wg2 sync.WaitGroup
	for b := 0; b*batchSize < len(symIDs); b++ {
		start := b * batchSize
		end := start + batchSize
		if end > len(symIDs) {
			end = len(symIDs)
		}
		wg2.Add(1)
		go func(ids []string) {
			defer wg2.Done()
			for _, id := range ids {
				if converted, err := convertToSCIPSymbol(scipIndex.Symbols[id], scipIndex); err == nil {
					symCh <- symResult{id: id, sym: converted}
				}
			}
		}(symIDs[start:end])
	}
	go func() {
		wg2.Wait()
		close(symCh)
	}()
	for r := range symCh {
		scipIndex.ConvertedSymbols[r.id] = r.sym
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
	return i.DocumentsByPath[relativePath]
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

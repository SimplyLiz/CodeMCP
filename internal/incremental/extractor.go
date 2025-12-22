package incremental

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ckb/internal/backends/scip"
	"ckb/internal/logging"
)

// SCIPExtractor extracts per-file data from SCIP index
type SCIPExtractor struct {
	repoRoot  string
	indexPath string
	logger    *logging.Logger
}

// NewSCIPExtractor creates a new SCIP extractor
func NewSCIPExtractor(repoRoot string, logger *logging.Logger) *SCIPExtractor {
	return &SCIPExtractor{
		repoRoot:  repoRoot,
		indexPath: filepath.Join(repoRoot, "index.scip"),
		logger:    logger,
	}
}

// ExtractDeltas runs scip-go and extracts data only for changed files
func (e *SCIPExtractor) ExtractDeltas(changedFiles []ChangedFile) (*SymbolDelta, error) {
	// Build set of paths we care about (for fast lookup during iteration)
	changedPaths := make(map[string]ChangedFile)
	for _, cf := range changedFiles {
		changedPaths[cf.Path] = cf
		// For renames, we need to extract from the new path
		if cf.OldPath != "" {
			changedPaths[cf.Path] = cf // New path maps to the change
		}
	}

	// Load full SCIP index (required - protobuf doesn't stream)
	// This is unavoidable, but we only process changed documents
	index, err := scip.LoadSCIPIndex(e.indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load SCIP index: %w", err)
	}

	delta := &SymbolDelta{}

	// Iterate documents, only process those in changedPaths set
	for _, doc := range index.Documents {
		cf, isChanged := changedPaths[doc.RelativePath]
		if !isChanged {
			// Skip this document - not in our changed set
			continue
		}

		// This is a changed file - extract its data
		fileDelta := e.extractFileDelta(doc, cf)
		delta.FileDeltas = append(delta.FileDeltas, fileDelta)

		// Update stats
		switch cf.ChangeType {
		case ChangeAdded:
			delta.Stats.FilesAdded++
		case ChangeModified, ChangeRenamed:
			delta.Stats.FilesChanged++
		}
		delta.Stats.SymbolsAdded += len(fileDelta.Symbols)
		delta.Stats.RefsAdded += len(fileDelta.Refs)
	}

	// Handle deleted files (they won't be in the new SCIP index)
	for _, cf := range changedFiles {
		if cf.ChangeType == ChangeDeleted {
			delta.FileDeltas = append(delta.FileDeltas, FileDelta{
				Path:       cf.Path,
				OldPath:    cf.Path, // Same as path for deletes
				ChangeType: ChangeDeleted,
				// Symbols and Refs will be empty - updater handles deletion
			})
			delta.Stats.FilesDeleted++
		}
	}

	return delta, nil
}

// RunSCIPGo executes the scip-go indexer
func (e *SCIPExtractor) RunSCIPGo() error {
	cmd := exec.Command("scip-go")
	cmd.Dir = e.repoRoot
	cmd.Stdout = os.Stderr // Show indexer output
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LoadIndex loads the SCIP index from disk
func (e *SCIPExtractor) LoadIndex() (*scip.SCIPIndex, error) {
	return scip.LoadSCIPIndex(e.indexPath)
}

// extractFileDelta extracts symbols and refs from a SCIP document
func (e *SCIPExtractor) extractFileDelta(doc *scip.Document, change ChangedFile) FileDelta {
	delta := FileDelta{
		Path:       change.Path,
		OldPath:    change.OldPath, // Preserve for rename handling
		ChangeType: change.ChangeType,
	}

	// If this is a rename, OldPath is the path to delete from
	// If OldPath is empty, use Path (for add/modify)
	if delta.OldPath == "" {
		delta.OldPath = delta.Path
	}

	// Use file hash from change detection if available
	// Otherwise compute it now
	if change.Hash != "" {
		delta.Hash = change.Hash
	} else {
		// Compute hash from actual file (not SCIP, for true content hash)
		fullPath := filepath.Join(e.repoRoot, change.Path)
		if h, err := hashFile(fullPath); err == nil {
			delta.Hash = h
		}
	}

	// Build symbol info map for enrichment
	symbolInfo := make(map[string]*scip.SymbolInformation)
	for _, sym := range doc.Symbols {
		symbolInfo[sym.Symbol] = sym
	}

	// Extract definitions (symbols)
	// IMPORTANT: Filter out local symbols and ensure consistent FilePath
	for _, occ := range doc.Occurrences {
		if occ.SymbolRoles&scip.SymbolRoleDefinition == 0 {
			continue // Not a definition
		}

		// Skip local symbols (they start with "local ")
		if isLocalSymbol(occ.Symbol) {
			continue
		}

		sym := Symbol{
			ID:       occ.Symbol,
			FilePath: change.Path, // Use canonical path from change, not doc
		}

		// Parse range (SCIP is 0-indexed, we use 1-indexed)
		if len(occ.Range) >= 1 {
			sym.StartLine = int(occ.Range[0]) + 1
		}
		if len(occ.Range) >= 3 {
			sym.EndLine = int(occ.Range[2]) + 1
		} else {
			sym.EndLine = sym.StartLine
		}

		// Enrich with symbol information if available
		if info, ok := symbolInfo[occ.Symbol]; ok {
			sym.Name = extractSymbolName(occ.Symbol, info.DisplayName)
			sym.Kind = mapSymbolKind(info.Kind)
			if len(info.Documentation) > 0 {
				sym.Documentation = info.Documentation[0]
			}
		} else {
			// Minimal info from symbol ID alone
			sym.Name = extractSymbolName(occ.Symbol, "")
			sym.Kind = "unknown"
		}

		delta.Symbols = append(delta.Symbols, sym)
	}

	// Extract references (non-definitions)
	for _, occ := range doc.Occurrences {
		if occ.SymbolRoles&scip.SymbolRoleDefinition != 0 {
			continue // Skip definitions
		}

		// Skip local symbols
		if isLocalSymbol(occ.Symbol) {
			continue
		}

		ref := Reference{
			FromFile:   change.Path,
			ToSymbolID: occ.Symbol,
			Kind:       "reference",
		}

		if len(occ.Range) >= 1 {
			ref.FromLine = int(occ.Range[0]) + 1
		}

		delta.Refs = append(delta.Refs, ref)
	}

	// Compute document hash for change detection optimization
	delta.SCIPDocumentHash = computeDocHash(doc)
	delta.SymbolCount = len(delta.Symbols)

	return delta
}

// extractSymbolName extracts the short name from a SCIP symbol ID
// Example: "scip-go gomod github.com/foo/bar 1.0 pkg.Func()." -> "Func"
func extractSymbolName(symbolID string, displayName string) string {
	if displayName != "" {
		return displayName
	}

	// Parse the symbol ID to extract the name
	// SCIP format: scheme manager name version descriptor.
	parts := strings.Split(symbolID, " ")
	if len(parts) < 5 {
		return symbolID
	}

	descriptor := parts[len(parts)-1]
	// Remove trailing . if present
	descriptor = strings.TrimSuffix(descriptor, ".")

	// Get the last component (the actual name)
	// Handles: package.Type.Method() -> Method
	components := strings.Split(descriptor, ".")
	if len(components) > 0 {
		name := components[len(components)-1]
		// Remove parentheses for functions/methods
		name = strings.TrimSuffix(name, "()")
		return name
	}

	return descriptor
}

// isLocalSymbol checks if a symbol is file-local
func isLocalSymbol(symbolID string) bool {
	return len(symbolID) > 6 && symbolID[:6] == "local "
}

// mapSymbolKind converts SCIP kind int32 to our string kind
func mapSymbolKind(kind int32) string {
	// SCIP kind enum values (from scip.proto)
	switch kind {
	case 0:
		return "unknown"
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
	case 19:
		return "object"
	case 20:
		return "key"
	case 21:
		return "null"
	case 22:
		return "enum_member"
	case 23:
		return "struct"
	case 24:
		return "event"
	case 25:
		return "operator"
	case 26:
		return "type_parameter"
	default:
		return "unknown"
	}
}

// computeDocHash computes a hash of the SCIP document for caching
// This lets us skip updates if the SCIP doc is unchanged
// (e.g., formatting changes that don't affect symbols)
func computeDocHash(doc *scip.Document) string {
	h := sha256.New()

	// Include document path for safety
	h.Write([]byte(doc.RelativePath))

	var buf [4]byte

	// Hash occurrences (order-dependent for stability)
	for _, occ := range doc.Occurrences {
		h.Write([]byte(occ.Symbol))

		// Properly encode int32 range values
		for _, r := range occ.Range {
			binary.LittleEndian.PutUint32(buf[:], uint32(r))
			h.Write(buf[:])
		}

		// Include role bits for stability
		binary.LittleEndian.PutUint32(buf[:], uint32(occ.SymbolRoles))
		h.Write(buf[:])
	}

	// Include symbol information (definitions, docs, kinds)
	for _, sym := range doc.Symbols {
		h.Write([]byte(sym.Symbol))
		binary.LittleEndian.PutUint32(buf[:], uint32(sym.Kind))
		h.Write(buf[:])
		for _, d := range sym.Documentation {
			h.Write([]byte(d))
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Truncate for storage
}

// hashFile computes SHA256 of a file's contents
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

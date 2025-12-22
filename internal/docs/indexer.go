package docs

import (
	"os"
	"path/filepath"
	"time"
)

// Indexer orchestrates document scanning, resolution, and storage.
type Indexer struct {
	scanner     *Scanner
	resolver    *Resolver
	store       *Store
	repoRoot    string
	config      IndexerConfig
}

// IndexerConfig contains configuration for the indexer.
type IndexerConfig struct {
	Paths   []string // Paths to scan for documentation
	Include []string // File patterns to include
	Exclude []string // Patterns to exclude
}

// DefaultIndexerConfig returns the default indexer configuration.
func DefaultIndexerConfig() IndexerConfig {
	return IndexerConfig{
		Paths:   []string{"docs", "README.md", "ARCHITECTURE.md", "DESIGN.md", "CONTRIBUTING.md"},
		Include: []string{"*.md"},
		Exclude: []string{"node_modules", "vendor", ".git"},
	}
}

// NewIndexer creates a new indexer.
func NewIndexer(repoRoot string, symbolIndex SymbolIndex, store *Store, config IndexerConfig) *Indexer {
	return &Indexer{
		scanner:  NewScanner(repoRoot),
		resolver: NewResolver(symbolIndex, store, DefaultResolverConfig()),
		store:    store,
		repoRoot: repoRoot,
		config:   config,
	}
}

// IndexAll scans and indexes all documentation.
func (i *Indexer) IndexAll(force bool) (*IndexStats, error) {
	stats := &IndexStats{}

	// Collect all markdown files from configured paths
	var files []string
	for _, p := range i.config.Paths {
		path := filepath.Join(i.repoRoot, p)

		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		}

		if info.IsDir() {
			dirFiles, err := i.collectFiles(path)
			if err != nil {
				continue
			}
			files = append(files, dirFiles...)
		} else {
			// Single file (e.g., README.md)
			if isMarkdown(path) {
				files = append(files, path)
			}
		}
	}

	// Process each file
	for _, file := range files {
		indexed, err := i.indexFile(file, force)
		if err != nil {
			continue
		}

		if indexed {
			stats.DocsIndexed++
		} else {
			stats.DocsSkipped++
		}
	}

	// Get final stats
	finalStats, err := i.store.GetStats()
	if err == nil {
		stats.ReferencesFound = finalStats.ReferencesFound
		stats.Resolved = finalStats.Resolved
		stats.Ambiguous = finalStats.Ambiguous
		stats.Missing = finalStats.Missing
		stats.Ineligible = finalStats.Ineligible
	}

	return stats, nil
}

// indexFile indexes a single file.
// Returns true if the file was indexed, false if skipped (unchanged).
func (i *Indexer) indexFile(path string, force bool) (bool, error) {
	// Check if file has changed (unless force)
	if !force {
		existingHash, err := i.store.GetDocumentHash(i.relativePath(path))
		if err == nil && existingHash != "" {
			// Compute current hash
			result := i.scanner.ScanFile(path)
			if result.Error != nil {
				return false, result.Error
			}
			if result.Doc.Hash == existingHash {
				return false, nil // Unchanged
			}
		}
	}

	// Scan the file
	result := i.scanner.ScanFile(path)
	if result.Error != nil {
		return false, result.Error
	}

	// Resolve mentions to symbols
	var references []DocReference
	for _, mention := range result.Mentions {
		resolution := i.resolver.Resolve(mention.RawText)

		ref := DocReference{
			DocPath:         result.Doc.Path,
			RawText:         mention.RawText,
			NormalizedText:  Normalize(mention.RawText),
			Line:            mention.Line,
			Column:          mention.Column,
			Context:         mention.Context,
			DetectionMethod: mention.Method,
			Resolution:      resolution.Status,
			Confidence:      resolution.Confidence,
			LastResolved:    time.Now(),
		}

		if resolution.SymbolID != "" {
			ref.SymbolID = &resolution.SymbolID
			ref.SymbolName = resolution.SymbolName
		}

		if len(resolution.Candidates) > 0 {
			ref.Candidates = resolution.Candidates
		}

		references = append(references, ref)
	}

	// Build document
	doc := &Document{
		Path:        result.Doc.Path,
		Type:        result.Doc.Type,
		Title:       result.Doc.Title,
		Hash:        result.Doc.Hash,
		LastIndexed: time.Now(),
		References:  references,
	}

	// Add module links
	for _, mod := range result.Modules {
		doc.Modules = append(doc.Modules, mod.ModuleID)
	}

	// Save to database
	if err := i.store.SaveDocument(doc); err != nil {
		return false, err
	}

	return true, nil
}

// collectFiles collects all markdown files from a directory.
func (i *Indexer) collectFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip excluded directories
		if info.IsDir() {
			name := info.Name()
			for _, ex := range i.config.Exclude {
				if name == ex || (len(name) > 0 && name[0] == '.') {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only process markdown files
		if isMarkdown(path) {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// relativePath converts an absolute path to a repo-relative path.
func (i *Indexer) relativePath(path string) string {
	if rel, err := filepath.Rel(i.repoRoot, path); err == nil {
		return rel
	}
	return path
}

// isMarkdown checks if a file is a markdown file.
func isMarkdown(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md" || ext == ".markdown"
}

// IndexFile indexes a single file by path.
func (i *Indexer) IndexFile(path string) error {
	_, err := i.indexFile(path, true)
	return err
}

// RebuildSuffixIndex rebuilds the suffix index from all SCIP symbols.
func (i *Indexer) RebuildSuffixIndex(symbols []Symbol, version string) error {
	suffixIndex := NewSuffixIndex(i.store)
	return suffixIndex.Build(symbols, version)
}

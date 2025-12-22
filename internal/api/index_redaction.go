package api

import (
	"path/filepath"
	"strings"
)

// Redactor applies privacy settings to API responses
type Redactor struct {
	config *IndexPrivacyConfig
}

// NewRedactor creates a new redactor with the given privacy config
func NewRedactor(config *IndexPrivacyConfig) *Redactor {
	return &Redactor{config: config}
}

// RedactSymbol applies redaction rules to a single symbol
func (r *Redactor) RedactSymbol(s IndexSymbol) IndexSymbol {
	// Apply path redaction
	if !r.config.ExposePaths {
		s.FilePath = ""
	} else if r.config.PathPrefixStrip != "" && s.FilePath != "" {
		s.FilePath = r.stripPrefix(s.FilePath)
	}

	// Keep basename regardless (always safe to expose)
	if s.FilePath != "" && s.FileBasename == "" {
		s.FileBasename = filepath.Base(s.FilePath)
	}

	// Apply docs redaction
	if !r.config.ExposeDocs {
		s.Documentation = ""
	}

	// Apply signature redaction
	if !r.config.ExposeSignatures {
		s.Signature = ""
	}

	return s
}

// RedactSymbols applies redaction rules to a slice of symbols
func (r *Redactor) RedactSymbols(symbols []IndexSymbol) []IndexSymbol {
	result := make([]IndexSymbol, len(symbols))
	for i, s := range symbols {
		result[i] = r.RedactSymbol(s)
	}
	return result
}

// RedactRef applies redaction rules to a single reference
func (r *Redactor) RedactRef(ref IndexRef) IndexRef {
	if !r.config.ExposePaths {
		ref.FromFile = ""
	} else if r.config.PathPrefixStrip != "" && ref.FromFile != "" {
		ref.FromFile = r.stripPrefix(ref.FromFile)
	}
	return ref
}

// RedactRefs applies redaction rules to a slice of references
func (r *Redactor) RedactRefs(refs []IndexRef) []IndexRef {
	result := make([]IndexRef, len(refs))
	for i, ref := range refs {
		result[i] = r.RedactRef(ref)
	}
	return result
}

// RedactCallEdge applies redaction rules to a single call edge
func (r *Redactor) RedactCallEdge(edge IndexCallEdge) IndexCallEdge {
	if !r.config.ExposePaths {
		edge.CallerFile = ""
	} else if r.config.PathPrefixStrip != "" && edge.CallerFile != "" {
		edge.CallerFile = r.stripPrefix(edge.CallerFile)
	}
	return edge
}

// RedactCallEdges applies redaction rules to a slice of call edges
func (r *Redactor) RedactCallEdges(edges []IndexCallEdge) []IndexCallEdge {
	result := make([]IndexCallEdge, len(edges))
	for i, edge := range edges {
		result[i] = r.RedactCallEdge(edge)
	}
	return result
}

// RedactFile applies redaction rules to a single file
func (r *Redactor) RedactFile(f IndexFile) IndexFile {
	if !r.config.ExposePaths {
		f.Path = ""
	} else if r.config.PathPrefixStrip != "" && f.Path != "" {
		f.Path = r.stripPrefix(f.Path)
	}

	// Keep basename regardless (always safe to expose)
	if f.Path != "" && f.Basename == "" {
		f.Basename = filepath.Base(f.Path)
	}

	return f
}

// RedactFiles applies redaction rules to a slice of files
func (r *Redactor) RedactFiles(files []IndexFile) []IndexFile {
	result := make([]IndexFile, len(files))
	for i, f := range files {
		result[i] = r.RedactFile(f)
	}
	return result
}

// stripPrefix removes the configured prefix from a path
func (r *Redactor) stripPrefix(path string) string {
	if r.config.PathPrefixStrip == "" {
		return path
	}

	// Normalize both paths for comparison
	prefix := r.config.PathPrefixStrip
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}

	// Also try without trailing slash
	prefix = strings.TrimSuffix(prefix, "/")
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix+"/")
	}

	return path
}

// NoOpRedactor returns a redactor that doesn't redact anything
func NoOpRedactor() *Redactor {
	return &Redactor{
		config: &IndexPrivacyConfig{
			ExposePaths:      true,
			ExposeDocs:       true,
			ExposeSignatures: true,
		},
	}
}

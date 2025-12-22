package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Hasher computes canonical hashes for delta entities.
// Uses length-prefixed encoding to avoid delimiter ambiguity.
// Format: ${len}:${value}${len}:${value}... where NULL → 0:
// Algorithm: SHA-256, lowercase hex output
type Hasher struct{}

// NewHasher creates a new hasher instance
func NewHasher() *Hasher {
	return &Hasher{}
}

// HashSymbol computes the canonical hash for a symbol record.
// Fields (in order): id, name, kind, file_id, line, language, signature, documentation
func (h *Hasher) HashSymbol(s *SymbolRecord) string {
	parts := []string{
		s.ID,
		s.Name,
		s.Kind,
		s.FileID,
		strconv.Itoa(s.Line),
		s.Language,
		s.Signature,
		s.Documentation,
	}
	return h.hashFields(parts)
}

// HashRef computes the canonical hash for a reference record.
// Fields (in order): from_file_id, line, col, to_symbol_id, kind, language
func (h *Hasher) HashRef(r *RefRecord) string {
	parts := []string{
		r.FromFileID,
		strconv.Itoa(r.Line),
		strconv.Itoa(r.Column),
		r.ToSymbolID,
		r.Kind,
		r.Language,
	}
	return h.hashFields(parts)
}

// HashCallEdge computes the canonical hash for a call graph edge.
// Fields (in order): caller_file_id, call_line, call_col, callee_id, caller_id, language
func (h *Hasher) HashCallEdge(c *CallEdge) string {
	parts := []string{
		c.CallerFileID,
		strconv.Itoa(c.CallLine),
		strconv.Itoa(c.CallColumn),
		c.CalleeID,
		c.CallerID,
		c.Language,
	}
	return h.hashFields(parts)
}

// HashFile computes the canonical hash for a file record.
// Fields (in order): id, path, language
func (h *Hasher) HashFile(f *FileRecord) string {
	parts := []string{
		f.ID,
		f.Path,
		f.Language,
	}
	return h.hashFields(parts)
}

// hashFields computes SHA-256 of length-prefixed fields
func (h *Hasher) hashFields(fields []string) string {
	var builder strings.Builder

	for _, field := range fields {
		// Length prefix: "len:value"
		// Empty/null → "0:"
		builder.WriteString(strconv.Itoa(len(field)))
		builder.WriteByte(':')
		builder.WriteString(field)
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}

// HashDelta computes the overall hash of a delta for snapshot identification.
// This creates the new_snapshot_id.
func (h *Hasher) HashDelta(d *Delta) string {
	var builder strings.Builder

	// Include base snapshot for chaining
	builder.WriteString(d.BaseSnapshotID)
	builder.WriteByte(':')
	builder.WriteString(d.Commit)
	builder.WriteByte(':')

	// Hash all symbols (sorted by ID for determinism)
	for _, s := range d.Deltas.Symbols.Added {
		builder.WriteString("sa:")
		builder.WriteString(h.HashSymbol(&s))
	}
	for _, s := range d.Deltas.Symbols.Modified {
		builder.WriteString("sm:")
		builder.WriteString(h.HashSymbol(&s))
	}
	for _, id := range d.Deltas.Symbols.Deleted {
		builder.WriteString("sd:")
		builder.WriteString(id)
	}

	// Hash all refs
	for _, r := range d.Deltas.Refs.Added {
		builder.WriteString("ra:")
		builder.WriteString(h.HashRef(&r))
	}
	for _, key := range d.Deltas.Refs.Deleted {
		builder.WriteString("rd:")
		builder.WriteString(key)
	}

	// Hash all call edges
	for _, c := range d.Deltas.CallGraph.Added {
		builder.WriteString("ca:")
		builder.WriteString(h.HashCallEdge(&c))
	}
	for _, key := range d.Deltas.CallGraph.Deleted {
		builder.WriteString("cd:")
		builder.WriteString(key)
	}

	// Hash all files
	for _, f := range d.Deltas.Files.Added {
		builder.WriteString("fa:")
		builder.WriteString(h.HashFile(&f))
	}
	for _, f := range d.Deltas.Files.Modified {
		builder.WriteString("fm:")
		builder.WriteString(h.HashFile(&f))
	}
	for _, id := range d.Deltas.Files.Deleted {
		builder.WriteString("fd:")
		builder.WriteString(id)
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ComputeSnapshotID computes a snapshot ID from a database state.
// This is used to verify base_snapshot_id matches the current state.
func (h *Hasher) ComputeSnapshotID(symbols []SymbolRecord, refs []RefRecord, calls []CallEdge, files []FileRecord) string {
	var builder strings.Builder

	// Hash all entities in canonical order
	for _, s := range symbols {
		builder.WriteString("s:")
		builder.WriteString(h.HashSymbol(&s))
	}
	for _, r := range refs {
		builder.WriteString("r:")
		builder.WriteString(h.HashRef(&r))
	}
	for _, c := range calls {
		builder.WriteString("c:")
		builder.WriteString(h.HashCallEdge(&c))
	}
	for _, f := range files {
		builder.WriteString("f:")
		builder.WriteString(h.HashFile(&f))
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// VerifyEntityHash checks if an entity's hash matches its computed hash.
// Returns true if hash is empty (not provided) or matches.
func (h *Hasher) VerifySymbolHash(s *SymbolRecord) bool {
	if s.Hash == "" {
		return true
	}
	return s.Hash == h.HashSymbol(s)
}

// VerifyRefHash checks a reference hash
func (h *Hasher) VerifyRefHash(r *RefRecord) bool {
	if r.Hash == "" {
		return true
	}
	return r.Hash == h.HashRef(r)
}

// VerifyCallEdgeHash checks a call edge hash
func (h *Hasher) VerifyCallEdgeHash(c *CallEdge) bool {
	if c.Hash == "" {
		return true
	}
	return c.Hash == h.HashCallEdge(c)
}

// Package diff provides delta artifact generation and validation for incremental indexing.
// Delta artifacts allow CI to emit pre-computed diffs, reducing server-side ingestion
// from O(N) over all symbols to O(delta).
package diff

import (
	"encoding/json"
	"time"
)

// SchemaVersion is the current delta schema version
const SchemaVersion = 1

// Delta represents a complete delta artifact for incremental indexing.
// CI systems generate this by comparing two SCIP indexes.
type Delta struct {
	// SchemaVersion for forward compatibility
	SchemaVersion int `json:"delta_schema_version"`

	// BaseSnapshotID is the SHA-256 hash of the base state (empty for initial)
	BaseSnapshotID string `json:"base_snapshot_id"`

	// NewSnapshotID is the SHA-256 hash of the new state
	NewSnapshotID string `json:"new_snapshot_id"`

	// Commit is the git commit hash this delta represents
	Commit string `json:"commit"`

	// Timestamp is Unix epoch seconds when delta was generated
	Timestamp int64 `json:"timestamp"`

	// Deltas contains all entity changes
	Deltas EntityDeltas `json:"deltas"`

	// Stats contains summary statistics for validation
	Stats DeltaStats `json:"stats"`
}

// EntityDeltas contains changes for each entity type
type EntityDeltas struct {
	Symbols   SymbolDeltas    `json:"symbols"`
	Refs      RefDeltas       `json:"refs"`
	CallGraph CallGraphDeltas `json:"callgraph"`
	Files     FileDeltas      `json:"files"`
}

// SymbolDeltas contains symbol-level changes
type SymbolDeltas struct {
	Added    []SymbolRecord `json:"added,omitempty"`
	Modified []SymbolRecord `json:"modified,omitempty"`
	Deleted  []string       `json:"deleted,omitempty"` // Symbol IDs only
}

// SymbolRecord represents a symbol for delta transfer
type SymbolRecord struct {
	ID            string `json:"id"`                      // Stable symbol ID (scip-go gomod...)
	Name          string `json:"name"`                    // Simple name
	Kind          string `json:"kind"`                    // function, class, variable, etc.
	FileID        string `json:"file_id"`                 // File path or ID
	Line          int    `json:"line"`                    // 1-indexed line number
	Column        int    `json:"column,omitempty"`        // 1-indexed column
	Language      string `json:"language,omitempty"`      // go, typescript, python, etc.
	Signature     string `json:"signature,omitempty"`     // Function signature
	Documentation string `json:"documentation,omitempty"` // Doc comment
	Hash          string `json:"hash,omitempty"`          // Canonical hash for validation
}

// RefDeltas contains reference-level changes
type RefDeltas struct {
	Added   []RefRecord `json:"added,omitempty"`
	Deleted []string    `json:"deleted,omitempty"` // Composite keys: "file_id:line:col:symbol_id"
}

// RefRecord represents a reference for delta transfer
type RefRecord struct {
	FromFileID string `json:"from_file_id"`   // Source file
	Line       int    `json:"line"`           // 1-indexed
	Column     int    `json:"column"`         // 1-indexed
	ToSymbolID string `json:"to_symbol_id"`   // Target symbol
	Kind       string `json:"kind,omitempty"` // reference, definition, implementation
	Language   string `json:"language,omitempty"`
	Hash       string `json:"hash,omitempty"` // Canonical hash for validation
}

// CompositeKey returns the composite key for a reference
func (r *RefRecord) CompositeKey() string {
	return r.FromFileID + ":" + itoa(r.Line) + ":" + itoa(r.Column) + ":" + r.ToSymbolID
}

// CallGraphDeltas contains call graph edge changes
type CallGraphDeltas struct {
	Added   []CallEdge `json:"added,omitempty"`
	Deleted []string   `json:"deleted,omitempty"` // Composite keys
}

// CallEdge represents a caller-callee relationship
type CallEdge struct {
	CallerFileID string `json:"caller_file_id"`
	CallLine     int    `json:"call_line"`
	CallColumn   int    `json:"call_column"`
	CallerID     string `json:"caller_id"` // Caller symbol ID
	CalleeID     string `json:"callee_id"` // Callee symbol ID
	Language     string `json:"language,omitempty"`
	Hash         string `json:"hash,omitempty"` // Canonical hash for validation
}

// CompositeKey returns the composite key for a call edge
func (c *CallEdge) CompositeKey() string {
	return c.CallerFileID + ":" + itoa(c.CallLine) + ":" + itoa(c.CallColumn) + ":" + c.CalleeID
}

// FileDeltas contains file-level changes
type FileDeltas struct {
	Added    []FileRecord `json:"added,omitempty"`
	Modified []FileRecord `json:"modified,omitempty"`
	Deleted  []string     `json:"deleted,omitempty"` // File paths/IDs
}

// FileRecord represents a file for delta transfer
type FileRecord struct {
	ID       string `json:"id"`                 // Usually the path
	Path     string `json:"path"`               // Relative file path
	Language string `json:"language,omitempty"` // Detected language
	Hash     string `json:"hash,omitempty"`     // Content hash
}

// DeltaStats contains summary statistics for validation
type DeltaStats struct {
	TotalAdded    int `json:"total_added"`
	TotalModified int `json:"total_modified"`
	TotalDeleted  int `json:"total_deleted"`

	// Per-entity counts for detailed validation
	SymbolsAdded    int `json:"symbols_added,omitempty"`
	SymbolsModified int `json:"symbols_modified,omitempty"`
	SymbolsDeleted  int `json:"symbols_deleted,omitempty"`
	RefsAdded       int `json:"refs_added,omitempty"`
	RefsDeleted     int `json:"refs_deleted,omitempty"`
	CallsAdded      int `json:"calls_added,omitempty"`
	CallsDeleted    int `json:"calls_deleted,omitempty"`
	FilesAdded      int `json:"files_added,omitempty"`
	FilesModified   int `json:"files_modified,omitempty"`
	FilesDeleted    int `json:"files_deleted,omitempty"`
}

// ComputeStats calculates statistics from the delta contents
func (d *Delta) ComputeStats() DeltaStats {
	stats := DeltaStats{
		SymbolsAdded:    len(d.Deltas.Symbols.Added),
		SymbolsModified: len(d.Deltas.Symbols.Modified),
		SymbolsDeleted:  len(d.Deltas.Symbols.Deleted),
		RefsAdded:       len(d.Deltas.Refs.Added),
		RefsDeleted:     len(d.Deltas.Refs.Deleted),
		CallsAdded:      len(d.Deltas.CallGraph.Added),
		CallsDeleted:    len(d.Deltas.CallGraph.Deleted),
		FilesAdded:      len(d.Deltas.Files.Added),
		FilesModified:   len(d.Deltas.Files.Modified),
		FilesDeleted:    len(d.Deltas.Files.Deleted),
	}

	stats.TotalAdded = stats.SymbolsAdded + stats.RefsAdded + stats.CallsAdded + stats.FilesAdded
	stats.TotalModified = stats.SymbolsModified + stats.FilesModified
	stats.TotalDeleted = stats.SymbolsDeleted + stats.RefsDeleted + stats.CallsDeleted + stats.FilesDeleted

	return stats
}

// Validate checks that stats match actual counts
func (d *Delta) ValidateStats() bool {
	computed := d.ComputeStats()
	return computed.TotalAdded == d.Stats.TotalAdded &&
		computed.TotalModified == d.Stats.TotalModified &&
		computed.TotalDeleted == d.Stats.TotalDeleted
}

// IsEmpty returns true if the delta contains no changes
func (d *Delta) IsEmpty() bool {
	return len(d.Deltas.Symbols.Added) == 0 &&
		len(d.Deltas.Symbols.Modified) == 0 &&
		len(d.Deltas.Symbols.Deleted) == 0 &&
		len(d.Deltas.Refs.Added) == 0 &&
		len(d.Deltas.Refs.Deleted) == 0 &&
		len(d.Deltas.CallGraph.Added) == 0 &&
		len(d.Deltas.CallGraph.Deleted) == 0 &&
		len(d.Deltas.Files.Added) == 0 &&
		len(d.Deltas.Files.Modified) == 0 &&
		len(d.Deltas.Files.Deleted) == 0
}

// NewDelta creates a new delta with current timestamp
func NewDelta(baseSnapshot, newSnapshot, commit string) *Delta {
	return &Delta{
		SchemaVersion:  SchemaVersion,
		BaseSnapshotID: baseSnapshot,
		NewSnapshotID:  newSnapshot,
		Commit:         commit,
		Timestamp:      time.Now().Unix(),
		Deltas:         EntityDeltas{},
		Stats:          DeltaStats{},
	}
}

// ToJSON serializes the delta to JSON
func (d *Delta) ToJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// ParseDelta deserializes a delta from JSON
func ParseDelta(data []byte) (*Delta, error) {
	var delta Delta
	if err := json.Unmarshal(data, &delta); err != nil {
		return nil, err
	}
	return &delta, nil
}

// Helper to convert int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	neg := i < 0
	if neg {
		i = -i
	}

	var buf [20]byte
	pos := len(buf)

	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

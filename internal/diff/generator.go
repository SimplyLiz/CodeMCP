package diff

import (
	"database/sql"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

// Generator creates delta artifacts by comparing two database states
type Generator struct {
	hasher *Hasher
}

// NewGenerator creates a new delta generator
func NewGenerator() *Generator {
	return &Generator{
		hasher: NewHasher(),
	}
}

// GenerateOptions configures delta generation
type GenerateOptions struct {
	Commit         string // Git commit hash for the new state
	IncludeHashes  bool   // Include entity hashes for validation
	BaseSnapshotID string // Override base snapshot ID (optional)
}

// GenerateFromDBs generates a delta by comparing two SQLite databases
func (g *Generator) GenerateFromDBs(basePath, newPath string, opts GenerateOptions) (*Delta, error) {
	// Open base database (may be empty for initial import)
	var baseDB *sql.DB
	var err error

	if basePath != "" {
		baseDB, err = sql.Open("sqlite", basePath+"?mode=ro")
		if err != nil {
			return nil, fmt.Errorf("failed to open base database: %w", err)
		}
		defer baseDB.Close()
	}

	// Open new database
	newDB, err := sql.Open("sqlite", newPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open new database: %w", err)
	}
	defer newDB.Close()

	// Load symbols from both databases
	baseSymbols, err := g.loadSymbols(baseDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load base symbols: %w", err)
	}

	newSymbols, err := g.loadSymbols(newDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load new symbols: %w", err)
	}

	// Load refs from both databases
	baseRefs, err := g.loadRefs(baseDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load base refs: %w", err)
	}

	newRefs, err := g.loadRefs(newDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load new refs: %w", err)
	}

	// Load call graph from both databases
	baseCalls, err := g.loadCallGraph(baseDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load base call graph: %w", err)
	}

	newCalls, err := g.loadCallGraph(newDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load new call graph: %w", err)
	}

	// Load files from both databases
	baseFiles, err := g.loadFiles(baseDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load base files: %w", err)
	}

	newFiles, err := g.loadFiles(newDB)
	if err != nil {
		return nil, fmt.Errorf("failed to load new files: %w", err)
	}

	// Compute base snapshot ID
	baseSnapshotID := opts.BaseSnapshotID
	if baseSnapshotID == "" && baseDB != nil {
		baseSnapshotID = g.hasher.ComputeSnapshotID(
			mapValues(baseSymbols),
			mapValues(baseRefs),
			mapValues(baseCalls),
			mapValues(baseFiles),
		)
	}

	// Create delta
	delta := NewDelta(baseSnapshotID, "", opts.Commit)

	// Diff symbols
	delta.Deltas.Symbols = g.diffSymbols(baseSymbols, newSymbols, opts.IncludeHashes)

	// Diff refs
	delta.Deltas.Refs = g.diffRefs(baseRefs, newRefs, opts.IncludeHashes)

	// Diff call graph
	delta.Deltas.CallGraph = g.diffCallGraph(baseCalls, newCalls, opts.IncludeHashes)

	// Diff files
	delta.Deltas.Files = g.diffFiles(baseFiles, newFiles, opts.IncludeHashes)

	// Compute stats
	delta.Stats = delta.ComputeStats()

	// Compute new snapshot ID
	delta.NewSnapshotID = g.hasher.HashDelta(delta)

	return delta, nil
}

func (g *Generator) loadSymbols(db *sql.DB) (map[string]SymbolRecord, error) {
	symbols := make(map[string]SymbolRecord)

	if db == nil {
		return symbols, nil
	}

	// Check if symbols table exists
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='symbols'").Scan(&tableName)
	if err == sql.ErrNoRows {
		return symbols, nil // No symbols table
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT
			COALESCE(id, ''),
			COALESCE(name, ''),
			COALESCE(kind, ''),
			COALESCE(file_id, ''),
			COALESCE(line, 0),
			COALESCE(col, 0),
			COALESCE(language, ''),
			COALESCE(signature, ''),
			COALESCE(documentation, '')
		FROM symbols
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s SymbolRecord
		if err := rows.Scan(&s.ID, &s.Name, &s.Kind, &s.FileID, &s.Line, &s.Column, &s.Language, &s.Signature, &s.Documentation); err != nil {
			return nil, err
		}
		symbols[s.ID] = s
	}

	return symbols, rows.Err()
}

func (g *Generator) loadRefs(db *sql.DB) (map[string]RefRecord, error) {
	refs := make(map[string]RefRecord)

	if db == nil {
		return refs, nil
	}

	// Check if refs table exists
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='refs'").Scan(&tableName)
	if err == sql.ErrNoRows {
		return refs, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT
			COALESCE(from_file_id, ''),
			COALESCE(line, 0),
			COALESCE(col, 0),
			COALESCE(to_symbol_id, ''),
			COALESCE(kind, ''),
			COALESCE(language, '')
		FROM refs
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r RefRecord
		if err := rows.Scan(&r.FromFileID, &r.Line, &r.Column, &r.ToSymbolID, &r.Kind, &r.Language); err != nil {
			return nil, err
		}
		refs[r.CompositeKey()] = r
	}

	return refs, rows.Err()
}

func (g *Generator) loadCallGraph(db *sql.DB) (map[string]CallEdge, error) {
	calls := make(map[string]CallEdge)

	if db == nil {
		return calls, nil
	}

	// Check if callgraph table exists
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='callgraph'").Scan(&tableName)
	if err == sql.ErrNoRows {
		return calls, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT
			COALESCE(caller_file_id, ''),
			COALESCE(call_line, 0),
			COALESCE(call_col, 0),
			COALESCE(caller_id, ''),
			COALESCE(callee_id, ''),
			COALESCE(language, '')
		FROM callgraph
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c CallEdge
		if err := rows.Scan(&c.CallerFileID, &c.CallLine, &c.CallColumn, &c.CallerID, &c.CalleeID, &c.Language); err != nil {
			return nil, err
		}
		calls[c.CompositeKey()] = c
	}

	return calls, rows.Err()
}

func (g *Generator) loadFiles(db *sql.DB) (map[string]FileRecord, error) {
	files := make(map[string]FileRecord)

	if db == nil {
		return files, nil
	}

	// Check if files/documents table exists
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name IN ('files', 'documents') LIMIT 1").Scan(&tableName)
	if err == sql.ErrNoRows {
		return files, nil
	}
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT
			COALESCE(id, path, ''),
			COALESCE(path, ''),
			COALESCE(language, '')
		FROM %s
	`, tableName)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var f FileRecord
		if err := rows.Scan(&f.ID, &f.Path, &f.Language); err != nil {
			return nil, err
		}
		if f.ID == "" {
			f.ID = f.Path
		}
		files[f.ID] = f
	}

	return files, rows.Err()
}

func (g *Generator) diffSymbols(base, new map[string]SymbolRecord, includeHashes bool) SymbolDeltas {
	var result SymbolDeltas

	// Find added and modified symbols
	for id, newSym := range new {
		if baseSym, exists := base[id]; !exists {
			// Added
			if includeHashes {
				newSym.Hash = g.hasher.HashSymbol(&newSym)
			}
			result.Added = append(result.Added, newSym)
		} else {
			// Check if modified (compare hashes)
			baseHash := g.hasher.HashSymbol(&baseSym)
			newHash := g.hasher.HashSymbol(&newSym)
			if baseHash != newHash {
				if includeHashes {
					newSym.Hash = newHash
				}
				result.Modified = append(result.Modified, newSym)
			}
		}
	}

	// Find deleted symbols
	for id := range base {
		if _, exists := new[id]; !exists {
			result.Deleted = append(result.Deleted, id)
		}
	}

	// Sort for deterministic output
	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].ID < result.Added[j].ID })
	sort.Slice(result.Modified, func(i, j int) bool { return result.Modified[i].ID < result.Modified[j].ID })
	sort.Strings(result.Deleted)

	return result
}

func (g *Generator) diffRefs(base, new map[string]RefRecord, includeHashes bool) RefDeltas {
	var result RefDeltas

	// Find added refs
	for key, newRef := range new {
		if _, exists := base[key]; !exists {
			if includeHashes {
				newRef.Hash = g.hasher.HashRef(&newRef)
			}
			result.Added = append(result.Added, newRef)
		}
	}

	// Find deleted refs
	for key := range base {
		if _, exists := new[key]; !exists {
			result.Deleted = append(result.Deleted, key)
		}
	}

	// Sort for deterministic output
	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].CompositeKey() < result.Added[j].CompositeKey() })
	sort.Strings(result.Deleted)

	return result
}

func (g *Generator) diffCallGraph(base, new map[string]CallEdge, includeHashes bool) CallGraphDeltas {
	var result CallGraphDeltas

	// Find added edges
	for key, newEdge := range new {
		if _, exists := base[key]; !exists {
			if includeHashes {
				newEdge.Hash = g.hasher.HashCallEdge(&newEdge)
			}
			result.Added = append(result.Added, newEdge)
		}
	}

	// Find deleted edges
	for key := range base {
		if _, exists := new[key]; !exists {
			result.Deleted = append(result.Deleted, key)
		}
	}

	// Sort for deterministic output
	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].CompositeKey() < result.Added[j].CompositeKey() })
	sort.Strings(result.Deleted)

	return result
}

func (g *Generator) diffFiles(base, new map[string]FileRecord, includeHashes bool) FileDeltas {
	var result FileDeltas

	// Find added and modified files
	for id, newFile := range new {
		if baseFile, exists := base[id]; !exists {
			// Added
			if includeHashes {
				newFile.Hash = g.hasher.HashFile(&newFile)
			}
			result.Added = append(result.Added, newFile)
		} else {
			// Check if modified
			baseHash := g.hasher.HashFile(&baseFile)
			newHash := g.hasher.HashFile(&newFile)
			if baseHash != newHash {
				if includeHashes {
					newFile.Hash = newHash
				}
				result.Modified = append(result.Modified, newFile)
			}
		}
	}

	// Find deleted files
	for id := range base {
		if _, exists := new[id]; !exists {
			result.Deleted = append(result.Deleted, id)
		}
	}

	// Sort for deterministic output
	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].ID < result.Added[j].ID })
	sort.Slice(result.Modified, func(i, j int) bool { return result.Modified[i].ID < result.Modified[j].ID })
	sort.Strings(result.Deleted)

	return result
}

// Helper to convert map to slice
func mapValues[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// SymbolMapping represents a symbol mapping record (Section 4.3)
type SymbolMapping struct {
	StableID                  string
	State                     string // 'active' | 'deleted' | 'unknown'
	BackendStableID           *string
	FingerprintJSON           string
	LocationJSON              string
	DefinitionVersionID       *string
	DefinitionVersionSemantics *string
	LastVerifiedAt            time.Time
	LastVerifiedStateID       string
	DeletedAt                 *time.Time
	DeletedInStateID          *string
}

// SymbolAlias represents an alias/redirect record (Section 4.4)
type SymbolAlias struct {
	OldStableID    string
	NewStableID    string
	Reason         string
	Confidence     float64
	CreatedAt      time.Time
	CreatedStateID string
}

// Module represents a module record
type Module struct {
	ModuleID     string
	Name         string
	RootPath     string
	ManifestType *string
	DetectedAt   time.Time
	StateID      string
}

// DependencyEdge represents a dependency relationship between modules
type DependencyEdge struct {
	FromModule string
	ToModule   string
	Kind       string
	Strength   int
}

// SymbolRepository provides CRUD operations for symbol_mappings table
type SymbolRepository struct {
	db *DB
}

// NewSymbolRepository creates a new symbol repository
func NewSymbolRepository(db *DB) *SymbolRepository {
	return &SymbolRepository{db: db}
}

// Create inserts a new symbol mapping
func (r *SymbolRepository) Create(mapping *SymbolMapping) error {
	_, err := r.db.Exec(`
		INSERT INTO symbol_mappings (
			stable_id, state, backend_stable_id, fingerprint_json, location_json,
			definition_version_id, definition_version_semantics,
			last_verified_at, last_verified_state_id,
			deleted_at, deleted_in_state_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		mapping.StableID,
		mapping.State,
		mapping.BackendStableID,
		mapping.FingerprintJSON,
		mapping.LocationJSON,
		mapping.DefinitionVersionID,
		mapping.DefinitionVersionSemantics,
		mapping.LastVerifiedAt.Format(time.RFC3339),
		mapping.LastVerifiedStateID,
		formatTimePtr(mapping.DeletedAt),
		mapping.DeletedInStateID,
	)

	if err != nil {
		return fmt.Errorf("failed to create symbol mapping: %w", err)
	}

	return nil
}

// GetByStableID retrieves a symbol mapping by its stable ID
func (r *SymbolRepository) GetByStableID(stableID string) (*SymbolMapping, error) {
	var mapping SymbolMapping
	var lastVerifiedAt string
	var deletedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT stable_id, state, backend_stable_id, fingerprint_json, location_json,
		       definition_version_id, definition_version_semantics,
		       last_verified_at, last_verified_state_id,
		       deleted_at, deleted_in_state_id
		FROM symbol_mappings
		WHERE stable_id = ?
	`, stableID).Scan(
		&mapping.StableID,
		&mapping.State,
		&mapping.BackendStableID,
		&mapping.FingerprintJSON,
		&mapping.LocationJSON,
		&mapping.DefinitionVersionID,
		&mapping.DefinitionVersionSemantics,
		&lastVerifiedAt,
		&mapping.LastVerifiedStateID,
		&deletedAt,
		&mapping.DeletedInStateID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol mapping: %w", err)
	}

	// Parse timestamps
	mapping.LastVerifiedAt, err = time.Parse(time.RFC3339, lastVerifiedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid last_verified_at format: %w", err)
	}

	if deletedAt.Valid {
		t, err := time.Parse(time.RFC3339, deletedAt.String)
		if err != nil {
			return nil, fmt.Errorf("invalid deleted_at format: %w", err)
		}
		mapping.DeletedAt = &t
	}

	return &mapping, nil
}

// Update updates an existing symbol mapping
func (r *SymbolRepository) Update(mapping *SymbolMapping) error {
	result, err := r.db.Exec(`
		UPDATE symbol_mappings
		SET state = ?,
		    backend_stable_id = ?,
		    fingerprint_json = ?,
		    location_json = ?,
		    definition_version_id = ?,
		    definition_version_semantics = ?,
		    last_verified_at = ?,
		    last_verified_state_id = ?,
		    deleted_at = ?,
		    deleted_in_state_id = ?
		WHERE stable_id = ?
	`,
		mapping.State,
		mapping.BackendStableID,
		mapping.FingerprintJSON,
		mapping.LocationJSON,
		mapping.DefinitionVersionID,
		mapping.DefinitionVersionSemantics,
		mapping.LastVerifiedAt.Format(time.RFC3339),
		mapping.LastVerifiedStateID,
		formatTimePtr(mapping.DeletedAt),
		mapping.DeletedInStateID,
		mapping.StableID,
	)

	if err != nil {
		return fmt.Errorf("failed to update symbol mapping: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("symbol mapping not found: %s", mapping.StableID)
	}

	return nil
}

// MarkAsDeleted marks a symbol as deleted (tombstone)
func (r *SymbolRepository) MarkAsDeleted(stableID string, stateID string) error {
	now := time.Now()

	result, err := r.db.Exec(`
		UPDATE symbol_mappings
		SET state = 'deleted',
		    deleted_at = ?,
		    deleted_in_state_id = ?,
		    last_verified_at = ?,
		    last_verified_state_id = ?
		WHERE stable_id = ?
	`,
		now.Format(time.RFC3339),
		stateID,
		now.Format(time.RFC3339),
		stateID,
		stableID,
	)

	if err != nil {
		return fmt.Errorf("failed to mark symbol as deleted: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("symbol mapping not found: %s", stableID)
	}

	return nil
}

// ListByState returns all symbol mappings with a given state
func (r *SymbolRepository) ListByState(state string, limit int) ([]*SymbolMapping, error) {
	rows, err := r.db.Query(`
		SELECT stable_id, state, backend_stable_id, fingerprint_json, location_json,
		       definition_version_id, definition_version_semantics,
		       last_verified_at, last_verified_state_id,
		       deleted_at, deleted_in_state_id
		FROM symbol_mappings
		WHERE state = ?
		LIMIT ?
	`, state, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to list symbol mappings: %w", err)
	}
	defer rows.Close()

	return r.scanSymbolMappings(rows)
}

// Delete permanently removes a symbol mapping (use with caution)
func (r *SymbolRepository) Delete(stableID string) error {
	_, err := r.db.Exec("DELETE FROM symbol_mappings WHERE stable_id = ?", stableID)
	if err != nil {
		return fmt.Errorf("failed to delete symbol mapping: %w", err)
	}
	return nil
}

// scanSymbolMappings scans rows into SymbolMapping structs
func (r *SymbolRepository) scanSymbolMappings(rows *sql.Rows) ([]*SymbolMapping, error) {
	var mappings []*SymbolMapping

	for rows.Next() {
		var mapping SymbolMapping
		var lastVerifiedAt string
		var deletedAt sql.NullString

		err := rows.Scan(
			&mapping.StableID,
			&mapping.State,
			&mapping.BackendStableID,
			&mapping.FingerprintJSON,
			&mapping.LocationJSON,
			&mapping.DefinitionVersionID,
			&mapping.DefinitionVersionSemantics,
			&lastVerifiedAt,
			&mapping.LastVerifiedStateID,
			&deletedAt,
			&mapping.DeletedInStateID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan symbol mapping: %w", err)
		}

		// Parse timestamps
		mapping.LastVerifiedAt, err = time.Parse(time.RFC3339, lastVerifiedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid last_verified_at format: %w", err)
		}

		if deletedAt.Valid {
			t, err := time.Parse(time.RFC3339, deletedAt.String)
			if err != nil {
				return nil, fmt.Errorf("invalid deleted_at format: %w", err)
			}
			mapping.DeletedAt = &t
		}

		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating symbol mappings: %w", err)
	}

	return mappings, nil
}

// AliasRepository provides CRUD operations for symbol_aliases table
type AliasRepository struct {
	db *DB
}

// NewAliasRepository creates a new alias repository
func NewAliasRepository(db *DB) *AliasRepository {
	return &AliasRepository{db: db}
}

// Create inserts a new symbol alias
func (r *AliasRepository) Create(alias *SymbolAlias) error {
	_, err := r.db.Exec(`
		INSERT INTO symbol_aliases (old_stable_id, new_stable_id, reason, confidence, created_at, created_state_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		alias.OldStableID,
		alias.NewStableID,
		alias.Reason,
		alias.Confidence,
		alias.CreatedAt.Format(time.RFC3339),
		alias.CreatedStateID,
	)

	if err != nil {
		return fmt.Errorf("failed to create symbol alias: %w", err)
	}

	return nil
}

// GetByOldStableID retrieves all aliases for an old stable ID
func (r *AliasRepository) GetByOldStableID(oldStableID string) ([]*SymbolAlias, error) {
	rows, err := r.db.Query(`
		SELECT old_stable_id, new_stable_id, reason, confidence, created_at, created_state_id
		FROM symbol_aliases
		WHERE old_stable_id = ?
	`, oldStableID)

	if err != nil {
		return nil, fmt.Errorf("failed to get symbol aliases: %w", err)
	}
	defer rows.Close()

	return r.scanSymbolAliases(rows)
}

// Delete removes a symbol alias
func (r *AliasRepository) Delete(oldStableID string, newStableID string) error {
	_, err := r.db.Exec("DELETE FROM symbol_aliases WHERE old_stable_id = ? AND new_stable_id = ?", oldStableID, newStableID)
	if err != nil {
		return fmt.Errorf("failed to delete symbol alias: %w", err)
	}
	return nil
}

// scanSymbolAliases scans rows into SymbolAlias structs
func (r *AliasRepository) scanSymbolAliases(rows *sql.Rows) ([]*SymbolAlias, error) {
	var aliases []*SymbolAlias

	for rows.Next() {
		var alias SymbolAlias
		var createdAt string

		err := rows.Scan(
			&alias.OldStableID,
			&alias.NewStableID,
			&alias.Reason,
			&alias.Confidence,
			&createdAt,
			&alias.CreatedStateID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan symbol alias: %w", err)
		}

		// Parse timestamp
		alias.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("invalid created_at format: %w", err)
		}

		aliases = append(aliases, &alias)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating symbol aliases: %w", err)
	}

	return aliases, nil
}

// ModuleRepository provides CRUD operations for modules table
type ModuleRepository struct {
	db *DB
}

// NewModuleRepository creates a new module repository
func NewModuleRepository(db *DB) *ModuleRepository {
	return &ModuleRepository{db: db}
}

// Create inserts a new module
func (r *ModuleRepository) Create(module *Module) error {
	_, err := r.db.Exec(`
		INSERT INTO modules (module_id, name, root_path, manifest_type, detected_at, state_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		module.ModuleID,
		module.Name,
		module.RootPath,
		module.ManifestType,
		module.DetectedAt.Format(time.RFC3339),
		module.StateID,
	)

	if err != nil {
		return fmt.Errorf("failed to create module: %w", err)
	}

	return nil
}

// GetByID retrieves a module by its ID
func (r *ModuleRepository) GetByID(moduleID string) (*Module, error) {
	var module Module
	var detectedAt string

	err := r.db.QueryRow(`
		SELECT module_id, name, root_path, manifest_type, detected_at, state_id
		FROM modules
		WHERE module_id = ?
	`, moduleID).Scan(
		&module.ModuleID,
		&module.Name,
		&module.RootPath,
		&module.ManifestType,
		&detectedAt,
		&module.StateID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get module: %w", err)
	}

	// Parse timestamp
	module.DetectedAt, err = time.Parse(time.RFC3339, detectedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid detected_at format: %w", err)
	}

	return &module, nil
}

// ListAll returns all modules
func (r *ModuleRepository) ListAll() ([]*Module, error) {
	rows, err := r.db.Query(`
		SELECT module_id, name, root_path, manifest_type, detected_at, state_id
		FROM modules
		ORDER BY name
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to list modules: %w", err)
	}
	defer rows.Close()

	return r.scanModules(rows)
}

// Delete removes a module
func (r *ModuleRepository) Delete(moduleID string) error {
	_, err := r.db.Exec("DELETE FROM modules WHERE module_id = ?", moduleID)
	if err != nil {
		return fmt.Errorf("failed to delete module: %w", err)
	}
	return nil
}

// scanModules scans rows into Module structs
func (r *ModuleRepository) scanModules(rows *sql.Rows) ([]*Module, error) {
	var modules []*Module

	for rows.Next() {
		var module Module
		var detectedAt string

		err := rows.Scan(
			&module.ModuleID,
			&module.Name,
			&module.RootPath,
			&module.ManifestType,
			&detectedAt,
			&module.StateID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan module: %w", err)
		}

		// Parse timestamp
		module.DetectedAt, err = time.Parse(time.RFC3339, detectedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid detected_at format: %w", err)
		}

		modules = append(modules, &module)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating modules: %w", err)
	}

	return modules, nil
}

// DependencyRepository provides CRUD operations for dependency_edges table
type DependencyRepository struct {
	db *DB
}

// NewDependencyRepository creates a new dependency repository
func NewDependencyRepository(db *DB) *DependencyRepository {
	return &DependencyRepository{db: db}
}

// Create inserts a new dependency edge
func (r *DependencyRepository) Create(edge *DependencyEdge) error {
	_, err := r.db.Exec(`
		INSERT INTO dependency_edges (from_module, to_module, kind, strength)
		VALUES (?, ?, ?, ?)
	`,
		edge.FromModule,
		edge.ToModule,
		edge.Kind,
		edge.Strength,
	)

	if err != nil {
		return fmt.Errorf("failed to create dependency edge: %w", err)
	}

	return nil
}

// GetByFromModule retrieves all dependencies from a module
func (r *DependencyRepository) GetByFromModule(fromModule string) ([]*DependencyEdge, error) {
	rows, err := r.db.Query(`
		SELECT from_module, to_module, kind, strength
		FROM dependency_edges
		WHERE from_module = ?
	`, fromModule)

	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}
	defer rows.Close()

	return r.scanDependencyEdges(rows)
}

// GetByToModule retrieves all dependencies to a module
func (r *DependencyRepository) GetByToModule(toModule string) ([]*DependencyEdge, error) {
	rows, err := r.db.Query(`
		SELECT from_module, to_module, kind, strength
		FROM dependency_edges
		WHERE to_module = ?
	`, toModule)

	if err != nil {
		return nil, fmt.Errorf("failed to get reverse dependencies: %w", err)
	}
	defer rows.Close()

	return r.scanDependencyEdges(rows)
}

// Delete removes a dependency edge
func (r *DependencyRepository) Delete(fromModule string, toModule string) error {
	_, err := r.db.Exec("DELETE FROM dependency_edges WHERE from_module = ? AND to_module = ?", fromModule, toModule)
	if err != nil {
		return fmt.Errorf("failed to delete dependency edge: %w", err)
	}
	return nil
}

// scanDependencyEdges scans rows into DependencyEdge structs
func (r *DependencyRepository) scanDependencyEdges(rows *sql.Rows) ([]*DependencyEdge, error) {
	var edges []*DependencyEdge

	for rows.Next() {
		var edge DependencyEdge

		err := rows.Scan(
			&edge.FromModule,
			&edge.ToModule,
			&edge.Kind,
			&edge.Strength,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan dependency edge: %w", err)
		}

		edges = append(edges, &edge)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating dependency edges: %w", err)
	}

	return edges, nil
}

// Helper function to format time pointer for SQL
func formatTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

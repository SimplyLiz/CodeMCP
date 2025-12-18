package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// SymbolMapping represents a symbol mapping record (Section 4.3)
type SymbolMapping struct {
	StableID                   string
	State                      string // 'active' | 'deleted' | 'unknown'
	BackendStableID            *string
	FingerprintJSON            string
	LocationJSON               string
	DefinitionVersionID        *string
	DefinitionVersionSemantics *string
	LastVerifiedAt             time.Time
	LastVerifiedStateID        string
	DeletedAt                  *time.Time
	DeletedInStateID           *string
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
	ModuleID       string
	Name           string
	RootPath       string
	ManifestType   *string
	DetectedAt     time.Time
	StateID        string
	// v2 fields for Architectural Memory
	Boundaries     *string // JSON: {public: [], internal: []}
	Responsibility *string // one-sentence description
	OwnerRef       *string // link to ownership
	Tags           *string // JSON array
	Source         string  // "declared" | "inferred"
	Confidence     float64
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

// UpdateAnnotations updates the v2 annotation fields for a module
func (r *ModuleRepository) UpdateAnnotations(moduleID string, boundaries, responsibility, ownerRef, tags *string, source string, confidence float64) error {
	result, err := r.db.Exec(`
		UPDATE modules
		SET boundaries = ?, responsibility = ?, owner_ref = ?, tags = ?, source = ?, confidence = ?
		WHERE module_id = ?
	`, boundaries, responsibility, ownerRef, tags, source, confidence, moduleID)
	if err != nil {
		return fmt.Errorf("failed to update module annotations: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("module not found: %s", moduleID)
	}

	return nil
}

// GetAnnotations retrieves the v2 annotation fields for a module
func (r *ModuleRepository) GetAnnotations(moduleID string) (*Module, error) {
	var module Module
	var detectedAt string

	err := r.db.QueryRow(`
		SELECT module_id, name, root_path, manifest_type, detected_at, state_id,
		       boundaries, responsibility, owner_ref, tags,
		       COALESCE(source, 'inferred') as source, COALESCE(confidence, 0.5) as confidence
		FROM modules WHERE module_id = ?
	`, moduleID).Scan(
		&module.ModuleID, &module.Name, &module.RootPath, &module.ManifestType,
		&detectedAt, &module.StateID,
		&module.Boundaries, &module.Responsibility, &module.OwnerRef, &module.Tags,
		&module.Source, &module.Confidence,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get module annotations: %w", err)
	}

	module.DetectedAt, _ = time.Parse(time.RFC3339, detectedAt)
	return &module, nil
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

// ============================================================================
// v2 Ownership Repository (Architectural Memory)
// ============================================================================

// OwnershipRecord represents an ownership record in the database
type OwnershipRecord struct {
	ID         int64
	Pattern    string  // glob pattern (e.g., "internal/api/**")
	OwnersJSON string  // JSON array of Owner objects
	Scope      string  // "maintainer" | "reviewer" | "contributor"
	Source     string  // "codeowners" | "git-blame" | "declared" | "inferred"
	Confidence float64
	UpdatedAt  time.Time
}

// OwnershipHistoryRecord represents a historical ownership change
type OwnershipHistoryRecord struct {
	ID         int64
	Pattern    string
	OwnerID    string // @username, @org/team, or email
	Event      string // "added" | "removed" | "promoted" | "demoted"
	Reason     string // e.g., "git_blame_shift", "codeowners_update"
	RecordedAt time.Time
}

// OwnershipRepository provides CRUD operations for ownership tables
type OwnershipRepository struct {
	db *DB
}

// NewOwnershipRepository creates a new ownership repository
func NewOwnershipRepository(db *DB) *OwnershipRepository {
	return &OwnershipRepository{db: db}
}

// Create inserts a new ownership record
func (r *OwnershipRepository) Create(record *OwnershipRecord) error {
	result, err := r.db.Exec(`
		INSERT INTO ownership (pattern, owners, scope, source, confidence, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		record.Pattern,
		record.OwnersJSON,
		record.Scope,
		record.Source,
		record.Confidence,
		record.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to create ownership record: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get inserted id: %w", err)
	}
	record.ID = id

	return nil
}

// GetByPattern retrieves ownership records matching a pattern
func (r *OwnershipRepository) GetByPattern(pattern string) ([]*OwnershipRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, pattern, owners, scope, source, confidence, updated_at
		FROM ownership
		WHERE pattern = ?
		ORDER BY confidence DESC
	`, pattern)

	if err != nil {
		return nil, fmt.Errorf("failed to get ownership records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanOwnershipRecords(rows)
}

// GetBySource retrieves all ownership records from a specific source
func (r *OwnershipRepository) GetBySource(source string) ([]*OwnershipRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, pattern, owners, scope, source, confidence, updated_at
		FROM ownership
		WHERE source = ?
		ORDER BY pattern
	`, source)

	if err != nil {
		return nil, fmt.Errorf("failed to get ownership records by source: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanOwnershipRecords(rows)
}

// GetMatchingPattern finds ownership records where the path matches the stored pattern
// Uses GLOB matching similar to CODEOWNERS
func (r *OwnershipRepository) GetMatchingPattern(filePath string) ([]*OwnershipRecord, error) {
	// Get all records and match in Go (SQLite GLOB doesn't support ** patterns)
	rows, err := r.db.Query(`
		SELECT id, pattern, owners, scope, source, confidence, updated_at
		FROM ownership
		ORDER BY LENGTH(pattern) DESC, confidence DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to get ownership records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records, err := r.scanOwnershipRecords(rows)
	if err != nil {
		return nil, err
	}

	// Filter to matching patterns (in a real implementation, we'd do pattern matching)
	// For now, return all records - the caller will handle matching
	return records, nil
}

// Update updates an existing ownership record
func (r *OwnershipRepository) Update(record *OwnershipRecord) error {
	result, err := r.db.Exec(`
		UPDATE ownership
		SET pattern = ?,
		    owners = ?,
		    scope = ?,
		    source = ?,
		    confidence = ?,
		    updated_at = ?
		WHERE id = ?
	`,
		record.Pattern,
		record.OwnersJSON,
		record.Scope,
		record.Source,
		record.Confidence,
		record.UpdatedAt.Format(time.RFC3339),
		record.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update ownership record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("ownership record not found: %d", record.ID)
	}

	return nil
}

// Upsert creates or updates an ownership record based on pattern and source
func (r *OwnershipRepository) Upsert(record *OwnershipRecord) error {
	// First try to find existing record
	var existingID int64
	err := r.db.QueryRow(`
		SELECT id FROM ownership WHERE pattern = ? AND source = ?
	`, record.Pattern, record.Source).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Create new record
		return r.Create(record)
	}
	if err != nil {
		return fmt.Errorf("failed to check existing ownership: %w", err)
	}

	// Update existing record
	record.ID = existingID
	return r.Update(record)
}

// Delete removes an ownership record
func (r *OwnershipRepository) Delete(id int64) error {
	_, err := r.db.Exec("DELETE FROM ownership WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete ownership record: %w", err)
	}
	return nil
}

// DeleteByPatternAndSource removes ownership records matching pattern and source
func (r *OwnershipRepository) DeleteByPatternAndSource(pattern string, source string) error {
	_, err := r.db.Exec("DELETE FROM ownership WHERE pattern = ? AND source = ?", pattern, source)
	if err != nil {
		return fmt.Errorf("failed to delete ownership records: %w", err)
	}
	return nil
}

// DeleteBySource removes all ownership records from a source
func (r *OwnershipRepository) DeleteBySource(source string) error {
	_, err := r.db.Exec("DELETE FROM ownership WHERE source = ?", source)
	if err != nil {
		return fmt.Errorf("failed to delete ownership records by source: %w", err)
	}
	return nil
}

// ListAll returns all ownership records
func (r *OwnershipRepository) ListAll() ([]*OwnershipRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, pattern, owners, scope, source, confidence, updated_at
		FROM ownership
		ORDER BY pattern, confidence DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("failed to list ownership records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanOwnershipRecords(rows)
}

// scanOwnershipRecords scans rows into OwnershipRecord structs
func (r *OwnershipRepository) scanOwnershipRecords(rows *sql.Rows) ([]*OwnershipRecord, error) {
	var records []*OwnershipRecord

	for rows.Next() {
		var record OwnershipRecord
		var updatedAt string

		err := rows.Scan(
			&record.ID,
			&record.Pattern,
			&record.OwnersJSON,
			&record.Scope,
			&record.Source,
			&record.Confidence,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan ownership record: %w", err)
		}

		record.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid updated_at format: %w", err)
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ownership records: %w", err)
	}

	return records, nil
}

// RecordHistoryEvent adds an ownership history event (append-only)
func (r *OwnershipRepository) RecordHistoryEvent(record *OwnershipHistoryRecord) error {
	result, err := r.db.Exec(`
		INSERT INTO ownership_history (pattern, owner_id, event, reason, recorded_at)
		VALUES (?, ?, ?, ?, ?)
	`,
		record.Pattern,
		record.OwnerID,
		record.Event,
		record.Reason,
		record.RecordedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to record ownership history: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get inserted id: %w", err)
	}
	record.ID = id

	return nil
}

// GetHistoryByPattern retrieves ownership history for a pattern
func (r *OwnershipRepository) GetHistoryByPattern(pattern string, limit int) ([]*OwnershipHistoryRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, pattern, owner_id, event, reason, recorded_at
		FROM ownership_history
		WHERE pattern = ?
		ORDER BY recorded_at DESC
		LIMIT ?
	`, pattern, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get ownership history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanHistoryRecords(rows)
}

// GetHistoryByOwner retrieves ownership history for an owner
func (r *OwnershipRepository) GetHistoryByOwner(ownerID string, limit int) ([]*OwnershipHistoryRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, pattern, owner_id, event, reason, recorded_at
		FROM ownership_history
		WHERE owner_id = ?
		ORDER BY recorded_at DESC
		LIMIT ?
	`, ownerID, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get ownership history by owner: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanHistoryRecords(rows)
}

// scanHistoryRecords scans rows into OwnershipHistoryRecord structs
func (r *OwnershipRepository) scanHistoryRecords(rows *sql.Rows) ([]*OwnershipHistoryRecord, error) {
	var records []*OwnershipHistoryRecord

	for rows.Next() {
		var record OwnershipHistoryRecord
		var recordedAt string
		var reason sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.Pattern,
			&record.OwnerID,
			&record.Event,
			&reason,
			&recordedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan ownership history: %w", err)
		}

		record.RecordedAt, err = time.Parse(time.RFC3339, recordedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid recorded_at format: %w", err)
		}

		if reason.Valid {
			record.Reason = reason.String
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ownership history: %w", err)
	}

	return records, nil
}

// ============================================================================
// v2 Hotspot Repository (Architectural Memory)
// ============================================================================

// HotspotSnapshotRecord represents a hotspot snapshot in the database
type HotspotSnapshotRecord struct {
	ID                   int64
	TargetID             string  // file path, module ID, or symbol ID
	TargetType           string  // "file" | "module" | "symbol"
	SnapshotDate         time.Time
	ChurnCommits30d      int
	ChurnCommits90d      int
	ChurnAuthors30d      int
	ComplexityCyclomatic float64
	ComplexityCognitive  float64
	CouplingAfferent     int     // incoming dependencies
	CouplingEfferent     int     // outgoing dependencies
	CouplingInstability  float64 // efferent / (afferent + efferent)
	Score                float64 // composite hotspot score
}

// HotspotRepository provides CRUD operations for hotspot_snapshots table
type HotspotRepository struct {
	db *DB
}

// NewHotspotRepository creates a new hotspot repository
func NewHotspotRepository(db *DB) *HotspotRepository {
	return &HotspotRepository{db: db}
}

// SaveSnapshot inserts a new hotspot snapshot (append-only)
func (r *HotspotRepository) SaveSnapshot(record *HotspotSnapshotRecord) error {
	result, err := r.db.Exec(`
		INSERT INTO hotspot_snapshots (
			target_id, target_type, snapshot_date,
			churn_commits_30d, churn_commits_90d, churn_authors_30d,
			complexity_cyclomatic, complexity_cognitive,
			coupling_afferent, coupling_efferent, coupling_instability,
			score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.TargetID,
		record.TargetType,
		record.SnapshotDate.Format(time.RFC3339),
		record.ChurnCommits30d,
		record.ChurnCommits90d,
		record.ChurnAuthors30d,
		record.ComplexityCyclomatic,
		record.ComplexityCognitive,
		record.CouplingAfferent,
		record.CouplingEfferent,
		record.CouplingInstability,
		record.Score,
	)

	if err != nil {
		return fmt.Errorf("failed to save hotspot snapshot: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get inserted id: %w", err)
	}
	record.ID = id

	return nil
}

// GetSnapshotsByTarget retrieves snapshots for a target, ordered by date descending
func (r *HotspotRepository) GetSnapshotsByTarget(targetID string, limit int) ([]*HotspotSnapshotRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, target_id, target_type, snapshot_date,
		       churn_commits_30d, churn_commits_90d, churn_authors_30d,
		       complexity_cyclomatic, complexity_cognitive,
		       coupling_afferent, coupling_efferent, coupling_instability,
		       score
		FROM hotspot_snapshots
		WHERE target_id = ?
		ORDER BY snapshot_date DESC
		LIMIT ?
	`, targetID, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get hotspot snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanSnapshots(rows)
}

// GetSnapshotsByTargetInRange retrieves snapshots within a date range
func (r *HotspotRepository) GetSnapshotsByTargetInRange(targetID string, startDate, endDate time.Time) ([]*HotspotSnapshotRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, target_id, target_type, snapshot_date,
		       churn_commits_30d, churn_commits_90d, churn_authors_30d,
		       complexity_cyclomatic, complexity_cognitive,
		       coupling_afferent, coupling_efferent, coupling_instability,
		       score
		FROM hotspot_snapshots
		WHERE target_id = ? AND snapshot_date >= ? AND snapshot_date <= ?
		ORDER BY snapshot_date ASC
	`, targetID, startDate.Format(time.RFC3339), endDate.Format(time.RFC3339))

	if err != nil {
		return nil, fmt.Errorf("failed to get hotspot snapshots in range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanSnapshots(rows)
}

// GetTopHotspots retrieves the top hotspots by score for a given date
func (r *HotspotRepository) GetTopHotspots(targetType string, snapshotDate time.Time, limit int) ([]*HotspotSnapshotRecord, error) {
	dateStr := snapshotDate.Format("2006-01-02")
	rows, err := r.db.Query(`
		SELECT id, target_id, target_type, snapshot_date,
		       churn_commits_30d, churn_commits_90d, churn_authors_30d,
		       complexity_cyclomatic, complexity_cognitive,
		       coupling_afferent, coupling_efferent, coupling_instability,
		       score
		FROM hotspot_snapshots
		WHERE target_type = ? AND date(snapshot_date) = ?
		ORDER BY score DESC
		LIMIT ?
	`, targetType, dateStr, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get top hotspots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanSnapshots(rows)
}

// GetLatestSnapshot retrieves the most recent snapshot for a target
func (r *HotspotRepository) GetLatestSnapshot(targetID string) (*HotspotSnapshotRecord, error) {
	var record HotspotSnapshotRecord
	var snapshotDate string

	err := r.db.QueryRow(`
		SELECT id, target_id, target_type, snapshot_date,
		       churn_commits_30d, churn_commits_90d, churn_authors_30d,
		       complexity_cyclomatic, complexity_cognitive,
		       coupling_afferent, coupling_efferent, coupling_instability,
		       score
		FROM hotspot_snapshots
		WHERE target_id = ?
		ORDER BY snapshot_date DESC
		LIMIT 1
	`, targetID).Scan(
		&record.ID,
		&record.TargetID,
		&record.TargetType,
		&snapshotDate,
		&record.ChurnCommits30d,
		&record.ChurnCommits90d,
		&record.ChurnAuthors30d,
		&record.ComplexityCyclomatic,
		&record.ComplexityCognitive,
		&record.CouplingAfferent,
		&record.CouplingEfferent,
		&record.CouplingInstability,
		&record.Score,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	record.SnapshotDate, err = time.Parse(time.RFC3339, snapshotDate)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot_date format: %w", err)
	}

	return &record, nil
}

// DeleteOldSnapshots removes snapshots older than the specified date
func (r *HotspotRepository) DeleteOldSnapshots(before time.Time) (int64, error) {
	result, err := r.db.Exec(`
		DELETE FROM hotspot_snapshots WHERE snapshot_date < ?
	`, before.Format(time.RFC3339))

	if err != nil {
		return 0, fmt.Errorf("failed to delete old snapshots: %w", err)
	}

	return result.RowsAffected()
}

// scanSnapshots scans rows into HotspotSnapshotRecord structs
func (r *HotspotRepository) scanSnapshots(rows *sql.Rows) ([]*HotspotSnapshotRecord, error) {
	var records []*HotspotSnapshotRecord

	for rows.Next() {
		var record HotspotSnapshotRecord
		var snapshotDate string

		err := rows.Scan(
			&record.ID,
			&record.TargetID,
			&record.TargetType,
			&snapshotDate,
			&record.ChurnCommits30d,
			&record.ChurnCommits90d,
			&record.ChurnAuthors30d,
			&record.ComplexityCyclomatic,
			&record.ComplexityCognitive,
			&record.CouplingAfferent,
			&record.CouplingEfferent,
			&record.CouplingInstability,
			&record.Score,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan hotspot snapshot: %w", err)
		}

		record.SnapshotDate, err = time.Parse(time.RFC3339, snapshotDate)
		if err != nil {
			return nil, fmt.Errorf("invalid snapshot_date format: %w", err)
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hotspot snapshots: %w", err)
	}

	return records, nil
}

// ============================================================================
// v2 Responsibilities Repository (Architectural Memory)
// ============================================================================

// ResponsibilityRecord represents a responsibility in the database
type ResponsibilityRecord struct {
	ID           int64
	TargetID     string  // module ID, file path, or symbol ID
	TargetType   string  // "module" | "file" | "symbol"
	Summary      string  // one-sentence description
	Capabilities string  // JSON array of capabilities
	Source       string  // "declared" | "inferred" | "llm-generated"
	Confidence   float64
	UpdatedAt    time.Time
	VerifiedAt   *time.Time // human verification timestamp
}

// ResponsibilityRepository provides CRUD operations for responsibilities table
type ResponsibilityRepository struct {
	db *DB
}

// NewResponsibilityRepository creates a new responsibility repository
func NewResponsibilityRepository(db *DB) *ResponsibilityRepository {
	return &ResponsibilityRepository{db: db}
}

// Create inserts a new responsibility record
func (r *ResponsibilityRepository) Create(record *ResponsibilityRecord) error {
	result, err := r.db.Exec(`
		INSERT INTO responsibilities (target_id, target_type, summary, capabilities, source, confidence, updated_at, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.TargetID,
		record.TargetType,
		record.Summary,
		record.Capabilities,
		record.Source,
		record.Confidence,
		record.UpdatedAt.Format(time.RFC3339),
		formatTimePtr(record.VerifiedAt),
	)

	if err != nil {
		return fmt.Errorf("failed to create responsibility: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get inserted id: %w", err)
	}
	record.ID = id

	return nil
}

// GetByTarget retrieves responsibility for a target
func (r *ResponsibilityRepository) GetByTarget(targetID string) (*ResponsibilityRecord, error) {
	var record ResponsibilityRecord
	var updatedAt string
	var verifiedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, target_id, target_type, summary, capabilities, source, confidence, updated_at, verified_at
		FROM responsibilities
		WHERE target_id = ?
	`, targetID).Scan(
		&record.ID,
		&record.TargetID,
		&record.TargetType,
		&record.Summary,
		&record.Capabilities,
		&record.Source,
		&record.Confidence,
		&updatedAt,
		&verifiedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get responsibility: %w", err)
	}

	record.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid updated_at format: %w", err)
	}

	if verifiedAt.Valid {
		t, err := time.Parse(time.RFC3339, verifiedAt.String)
		if err != nil {
			return nil, fmt.Errorf("invalid verified_at format: %w", err)
		}
		record.VerifiedAt = &t
	}

	return &record, nil
}

// GetByType retrieves all responsibilities for a target type
func (r *ResponsibilityRepository) GetByType(targetType string, limit int) ([]*ResponsibilityRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, target_id, target_type, summary, capabilities, source, confidence, updated_at, verified_at
		FROM responsibilities
		WHERE target_type = ?
		ORDER BY confidence DESC
		LIMIT ?
	`, targetType, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get responsibilities by type: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanResponsibilities(rows)
}

// Upsert creates or updates a responsibility
func (r *ResponsibilityRepository) Upsert(record *ResponsibilityRecord) error {
	var existingID int64
	err := r.db.QueryRow(`
		SELECT id FROM responsibilities WHERE target_id = ?
	`, record.TargetID).Scan(&existingID)

	if err == sql.ErrNoRows {
		return r.Create(record)
	}
	if err != nil {
		return fmt.Errorf("failed to check existing responsibility: %w", err)
	}

	record.ID = existingID
	return r.Update(record)
}

// Update updates an existing responsibility
func (r *ResponsibilityRepository) Update(record *ResponsibilityRecord) error {
	_, err := r.db.Exec(`
		UPDATE responsibilities
		SET target_type = ?, summary = ?, capabilities = ?, source = ?, confidence = ?, updated_at = ?, verified_at = ?
		WHERE id = ?
	`,
		record.TargetType,
		record.Summary,
		record.Capabilities,
		record.Source,
		record.Confidence,
		record.UpdatedAt.Format(time.RFC3339),
		formatTimePtr(record.VerifiedAt),
		record.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update responsibility: %w", err)
	}

	return nil
}

// Delete removes a responsibility
func (r *ResponsibilityRepository) Delete(targetID string) error {
	_, err := r.db.Exec("DELETE FROM responsibilities WHERE target_id = ?", targetID)
	if err != nil {
		return fmt.Errorf("failed to delete responsibility: %w", err)
	}
	return nil
}

// Search performs full-text search on responsibilities
func (r *ResponsibilityRepository) Search(query string, limit int) ([]*ResponsibilityRecord, error) {
	// Use LIKE for simple search (FTS5 would be better for production)
	searchPattern := "%" + query + "%"
	rows, err := r.db.Query(`
		SELECT id, target_id, target_type, summary, capabilities, source, confidence, updated_at, verified_at
		FROM responsibilities
		WHERE summary LIKE ? OR capabilities LIKE ?
		ORDER BY confidence DESC
		LIMIT ?
	`, searchPattern, searchPattern, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to search responsibilities: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanResponsibilities(rows)
}

// scanResponsibilities scans rows into ResponsibilityRecord structs
func (r *ResponsibilityRepository) scanResponsibilities(rows *sql.Rows) ([]*ResponsibilityRecord, error) {
	var records []*ResponsibilityRecord

	for rows.Next() {
		var record ResponsibilityRecord
		var updatedAt string
		var verifiedAt sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.TargetID,
			&record.TargetType,
			&record.Summary,
			&record.Capabilities,
			&record.Source,
			&record.Confidence,
			&updatedAt,
			&verifiedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan responsibility: %w", err)
		}

		record.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("invalid updated_at format: %w", err)
		}

		if verifiedAt.Valid {
			t, err := time.Parse(time.RFC3339, verifiedAt.String)
			if err != nil {
				return nil, fmt.Errorf("invalid verified_at format: %w", err)
			}
			record.VerifiedAt = &t
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating responsibilities: %w", err)
	}

	return records, nil
}

// ============================================================================
// v2 Decisions Repository (Architectural Memory)
// ============================================================================

// DecisionRecord represents an ADR record in the database
type DecisionRecord struct {
	ID              string    // "ADR-001" style
	Title           string
	Status          string    // "proposed" | "accepted" | "deprecated" | "superseded"
	AffectedModules string    // JSON array of module IDs
	FilePath        string    // relative path to .md file
	Author          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DecisionRepository provides CRUD operations for decisions table
type DecisionRepository struct {
	db *DB
}

// NewDecisionRepository creates a new decision repository
func NewDecisionRepository(db *DB) *DecisionRepository {
	return &DecisionRepository{db: db}
}

// Create inserts a new decision record
func (r *DecisionRepository) Create(record *DecisionRecord) error {
	_, err := r.db.Exec(`
		INSERT INTO decisions (id, title, status, affected_modules, file_path, author, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID,
		record.Title,
		record.Status,
		record.AffectedModules,
		record.FilePath,
		record.Author,
		record.CreatedAt.Format(time.RFC3339),
		record.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to create decision: %w", err)
	}

	return nil
}

// GetByID retrieves a decision by its ID
func (r *DecisionRepository) GetByID(id string) (*DecisionRecord, error) {
	var record DecisionRecord
	var createdAt, updatedAt string
	var author sql.NullString
	var affectedModules sql.NullString

	err := r.db.QueryRow(`
		SELECT id, title, status, affected_modules, file_path, author, created_at, updated_at
		FROM decisions
		WHERE id = ?
	`, id).Scan(
		&record.ID,
		&record.Title,
		&record.Status,
		&affectedModules,
		&record.FilePath,
		&author,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get decision: %w", err)
	}

	record.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	record.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if author.Valid {
		record.Author = author.String
	}
	if affectedModules.Valid {
		record.AffectedModules = affectedModules.String
	}

	return &record, nil
}

// GetByStatus retrieves all decisions with a given status
func (r *DecisionRepository) GetByStatus(status string, limit int) ([]*DecisionRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, title, status, affected_modules, file_path, author, created_at, updated_at
		FROM decisions
		WHERE status = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, status, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get decisions by status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanDecisions(rows)
}

// GetByModule retrieves all decisions affecting a module
func (r *DecisionRepository) GetByModule(moduleID string, limit int) ([]*DecisionRecord, error) {
	// Use JSON_EACH to search within the affected_modules array
	searchPattern := "%" + moduleID + "%"
	rows, err := r.db.Query(`
		SELECT id, title, status, affected_modules, file_path, author, created_at, updated_at
		FROM decisions
		WHERE affected_modules LIKE ?
		ORDER BY created_at DESC
		LIMIT ?
	`, searchPattern, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to get decisions by module: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanDecisions(rows)
}

// ListAll returns all decisions
func (r *DecisionRepository) ListAll(limit int) ([]*DecisionRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, title, status, affected_modules, file_path, author, created_at, updated_at
		FROM decisions
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to list decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanDecisions(rows)
}

// Update updates an existing decision record
func (r *DecisionRepository) Update(record *DecisionRecord) error {
	record.UpdatedAt = time.Now()
	result, err := r.db.Exec(`
		UPDATE decisions
		SET title = ?, status = ?, affected_modules = ?, file_path = ?, author = ?, updated_at = ?
		WHERE id = ?
	`,
		record.Title,
		record.Status,
		record.AffectedModules,
		record.FilePath,
		record.Author,
		record.UpdatedAt.Format(time.RFC3339),
		record.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update decision: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("decision not found: %s", record.ID)
	}

	return nil
}

// Upsert creates or updates a decision
func (r *DecisionRepository) Upsert(record *DecisionRecord) error {
	existing, err := r.GetByID(record.ID)
	if err != nil {
		return err
	}

	if existing == nil {
		return r.Create(record)
	}

	return r.Update(record)
}

// Delete removes a decision record
func (r *DecisionRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM decisions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete decision: %w", err)
	}
	return nil
}

// Search performs text search on decisions
func (r *DecisionRepository) Search(query string, limit int) ([]*DecisionRecord, error) {
	searchPattern := "%" + query + "%"
	rows, err := r.db.Query(`
		SELECT id, title, status, affected_modules, file_path, author, created_at, updated_at
		FROM decisions
		WHERE title LIKE ? OR id LIKE ?
		ORDER BY created_at DESC
		LIMIT ?
	`, searchPattern, searchPattern, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to search decisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanDecisions(rows)
}

// scanDecisions scans rows into DecisionRecord structs
func (r *DecisionRepository) scanDecisions(rows *sql.Rows) ([]*DecisionRecord, error) {
	var records []*DecisionRecord

	for rows.Next() {
		var record DecisionRecord
		var createdAt, updatedAt string
		var author sql.NullString
		var affectedModules sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.Title,
			&record.Status,
			&affectedModules,
			&record.FilePath,
			&author,
			&createdAt,
			&updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan decision: %w", err)
		}

		record.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		record.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if author.Valid {
			record.Author = author.String
		}
		if affectedModules.Valid {
			record.AffectedModules = affectedModules.String
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating decisions: %w", err)
	}

	return records, nil
}

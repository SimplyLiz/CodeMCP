package identity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

// SymbolFilter represents filter criteria for listing symbols
type SymbolFilter struct {
	State           *SymbolState // Filter by state (nil = all)
	Kind            *SymbolKind  // Filter by kind (nil = all)
	BackendStableId string       // Filter by backend ID (empty = all)
	Limit           int          // Limit results (0 = no limit)
	Offset          int          // Offset for pagination
}

// SymbolRepository provides CRUD operations for symbol mappings
type SymbolRepository struct {
	db     *storage.DB
	logger *logging.Logger
}

// NewSymbolRepository creates a new symbol repository
func NewSymbolRepository(db *storage.DB, logger *logging.Logger) *SymbolRepository {
	return &SymbolRepository{
		db:     db,
		logger: logger,
	}
}

// Get retrieves a symbol mapping by stable ID
func (r *SymbolRepository) Get(stableId string) (*SymbolMapping, error) {
	query := `
		SELECT
			stable_id, state, backend_stable_id, fingerprint_json,
			location_json, definition_version_id, definition_version_semantics,
			last_verified_at, last_verified_state_id,
			deleted_at, deleted_in_state_id
		FROM symbol_mappings
		WHERE stable_id = ?
	`

	row := r.db.QueryRow(query, stableId)

	var mapping SymbolMapping
	var fingerprintJson, locationJson string
	var backendStableId, definitionVersionId sql.NullString
	var deletedAt, deletedInStateId sql.NullString

	err := row.Scan(
		&mapping.StableId,
		&mapping.State,
		&backendStableId,
		&fingerprintJson,
		&locationJson,
		&definitionVersionId,
		&mapping.DefinitionVersionSemantics,
		&mapping.LastVerifiedAt,
		&mapping.LastVerifiedStateId,
		&deletedAt,
		&deletedInStateId,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(fingerprintJson), &mapping.Fingerprint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fingerprint: %w", err)
	}

	if err := json.Unmarshal([]byte(locationJson), &mapping.Location); err != nil {
		return nil, fmt.Errorf("failed to unmarshal location: %w", err)
	}

	// Handle nullable fields
	if backendStableId.Valid {
		mapping.BackendStableId = backendStableId.String
	}
	if definitionVersionId.Valid {
		mapping.DefinitionVersionId = definitionVersionId.String
	}
	if deletedAt.Valid {
		mapping.DeletedAt = deletedAt.String
	}
	if deletedInStateId.Valid {
		mapping.DeletedInStateId = deletedInStateId.String
	}

	return &mapping, nil
}

// Create inserts a new symbol mapping
func (r *SymbolRepository) Create(mapping *SymbolMapping) error {
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid mapping: %w", err)
	}

	// Marshal JSON fields
	fingerprintJson, err := json.Marshal(mapping.Fingerprint)
	if err != nil {
		return fmt.Errorf("failed to marshal fingerprint: %w", err)
	}

	locationJson, err := json.Marshal(mapping.Location)
	if err != nil {
		return fmt.Errorf("failed to marshal location: %w", err)
	}

	query := `
		INSERT INTO symbol_mappings (
			stable_id, state, backend_stable_id, fingerprint_json,
			location_json, definition_version_id, definition_version_semantics,
			last_verified_at, last_verified_state_id,
			deleted_at, deleted_in_state_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.Exec(
		query,
		mapping.StableId,
		mapping.State,
		nullString(mapping.BackendStableId),
		string(fingerprintJson),
		string(locationJson),
		nullString(mapping.DefinitionVersionId),
		mapping.DefinitionVersionSemantics,
		mapping.LastVerifiedAt,
		mapping.LastVerifiedStateId,
		nullString(mapping.DeletedAt),
		nullString(mapping.DeletedInStateId),
	)

	if err != nil {
		return fmt.Errorf("failed to create symbol: %w", err)
	}

	r.logger.Debug("created symbol mapping", map[string]interface{}{
		"stable_id": mapping.StableId,
		"state":     mapping.State,
	})

	return nil
}

// Update updates an existing symbol mapping
func (r *SymbolRepository) Update(mapping *SymbolMapping) error {
	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid mapping: %w", err)
	}

	// Marshal JSON fields
	fingerprintJson, err := json.Marshal(mapping.Fingerprint)
	if err != nil {
		return fmt.Errorf("failed to marshal fingerprint: %w", err)
	}

	locationJson, err := json.Marshal(mapping.Location)
	if err != nil {
		return fmt.Errorf("failed to marshal location: %w", err)
	}

	query := `
		UPDATE symbol_mappings SET
			state = ?,
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
	`

	result, err := r.db.Exec(
		query,
		mapping.State,
		nullString(mapping.BackendStableId),
		string(fingerprintJson),
		string(locationJson),
		nullString(mapping.DefinitionVersionId),
		mapping.DefinitionVersionSemantics,
		mapping.LastVerifiedAt,
		mapping.LastVerifiedStateId,
		nullString(mapping.DeletedAt),
		nullString(mapping.DeletedInStateId),
		mapping.StableId,
	)

	if err != nil {
		return fmt.Errorf("failed to update symbol: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("symbol not found: %s", mapping.StableId)
	}

	r.logger.Debug("updated symbol mapping", map[string]interface{}{
		"stable_id": mapping.StableId,
		"state":     mapping.State,
	})

	return nil
}

// MarkDeleted marks a symbol as deleted (creates a tombstone)
func (r *SymbolRepository) MarkDeleted(stableId, deletedStateId string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		UPDATE symbol_mappings SET
			state = ?,
			deleted_at = ?,
			deleted_in_state_id = ?
		WHERE stable_id = ?
	`

	result, err := r.db.Exec(query, StateDeleted, now, deletedStateId, stableId)
	if err != nil {
		return fmt.Errorf("failed to mark symbol as deleted: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("symbol not found: %s", stableId)
	}

	r.logger.Info("marked symbol as deleted", map[string]interface{}{
		"stable_id":      stableId,
		"deleted_at":     now,
		"deleted_state": deletedStateId,
	})

	return nil
}

// GetByBackendId retrieves a symbol mapping by backend stable ID
func (r *SymbolRepository) GetByBackendId(backendId string) (*SymbolMapping, error) {
	query := `
		SELECT
			stable_id, state, backend_stable_id, fingerprint_json,
			location_json, definition_version_id, definition_version_semantics,
			last_verified_at, last_verified_state_id,
			deleted_at, deleted_in_state_id
		FROM symbol_mappings
		WHERE backend_stable_id = ?
		AND state = ?
		LIMIT 1
	`

	row := r.db.QueryRow(query, backendId, StateActive)

	var mapping SymbolMapping
	var fingerprintJson, locationJson string
	var backendStableId, definitionVersionId sql.NullString
	var deletedAt, deletedInStateId sql.NullString

	err := row.Scan(
		&mapping.StableId,
		&mapping.State,
		&backendStableId,
		&fingerprintJson,
		&locationJson,
		&definitionVersionId,
		&mapping.DefinitionVersionSemantics,
		&mapping.LastVerifiedAt,
		&mapping.LastVerifiedStateId,
		&deletedAt,
		&deletedInStateId,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol by backend ID: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal([]byte(fingerprintJson), &mapping.Fingerprint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fingerprint: %w", err)
	}

	if err := json.Unmarshal([]byte(locationJson), &mapping.Location); err != nil {
		return nil, fmt.Errorf("failed to unmarshal location: %w", err)
	}

	// Handle nullable fields
	if backendStableId.Valid {
		mapping.BackendStableId = backendStableId.String
	}
	if definitionVersionId.Valid {
		mapping.DefinitionVersionId = definitionVersionId.String
	}
	if deletedAt.Valid {
		mapping.DeletedAt = deletedAt.String
	}
	if deletedInStateId.Valid {
		mapping.DeletedInStateId = deletedInStateId.String
	}

	return &mapping, nil
}

// List retrieves symbol mappings matching the filter criteria
func (r *SymbolRepository) List(filter SymbolFilter) ([]*SymbolMapping, error) {
	query := `
		SELECT
			stable_id, state, backend_stable_id, fingerprint_json,
			location_json, definition_version_id, definition_version_semantics,
			last_verified_at, last_verified_state_id,
			deleted_at, deleted_in_state_id
		FROM symbol_mappings
		WHERE 1=1
	`
	args := []interface{}{}

	// Apply filters
	if filter.State != nil {
		query += " AND state = ?"
		args = append(args, *filter.State)
	}

	if filter.BackendStableId != "" {
		query += " AND backend_stable_id = ?"
		args = append(args, filter.BackendStableId)
	}

	// Add ordering
	query += " ORDER BY stable_id ASC"

	// Add limit and offset
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list symbols: %w", err)
	}
	defer rows.Close()

	var mappings []*SymbolMapping

	for rows.Next() {
		var mapping SymbolMapping
		var fingerprintJson, locationJson string
		var backendStableId, definitionVersionId sql.NullString
		var deletedAt, deletedInStateId sql.NullString

		err := rows.Scan(
			&mapping.StableId,
			&mapping.State,
			&backendStableId,
			&fingerprintJson,
			&locationJson,
			&definitionVersionId,
			&mapping.DefinitionVersionSemantics,
			&mapping.LastVerifiedAt,
			&mapping.LastVerifiedStateId,
			&deletedAt,
			&deletedInStateId,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan symbol: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal([]byte(fingerprintJson), &mapping.Fingerprint); err != nil {
			return nil, fmt.Errorf("failed to unmarshal fingerprint: %w", err)
		}

		if err := json.Unmarshal([]byte(locationJson), &mapping.Location); err != nil {
			return nil, fmt.Errorf("failed to unmarshal location: %w", err)
		}

		// Handle nullable fields
		if backendStableId.Valid {
			mapping.BackendStableId = backendStableId.String
		}
		if definitionVersionId.Valid {
			mapping.DefinitionVersionId = definitionVersionId.String
		}
		if deletedAt.Valid {
			mapping.DeletedAt = deletedAt.String
		}
		if deletedInStateId.Valid {
			mapping.DeletedInStateId = deletedInStateId.String
		}

		mappings = append(mappings, &mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating symbols: %w", err)
	}

	return mappings, nil
}

// nullString converts a string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

package storage

import (
	"database/sql"
	"fmt"
)

// Schema version tracking
const currentSchemaVersion = 1

// initializeSchema creates all tables for a new database
func (db *DB) initializeSchema() error {
	return db.WithTx(func(tx *sql.Tx) error {
		// Create schema_version table first
		if err := createSchemaVersionTable(tx); err != nil {
			return err
		}

		// Create all application tables
		if err := createSymbolMappingsTable(tx); err != nil {
			return err
		}
		if err := createSymbolAliasesTable(tx); err != nil {
			return err
		}
		if err := createModulesTable(tx); err != nil {
			return err
		}
		if err := createDependencyEdgesTable(tx); err != nil {
			return err
		}
		if err := createCacheTablesTable(tx); err != nil {
			return err
		}

		// Set initial schema version
		if err := setSchemaVersion(tx, currentSchemaVersion); err != nil {
			return err
		}

		db.logger.Info("Database schema initialized", map[string]interface{}{
			"version": currentSchemaVersion,
		})

		return nil
	})
}

// runMigrations runs any pending schema migrations
func (db *DB) runMigrations() error {
	// Get current schema version
	version, err := db.getSchemaVersion()
	if err != nil {
		return err
	}

	if version == currentSchemaVersion {
		db.logger.Debug("Database schema is up to date", map[string]interface{}{
			"version": version,
		})
		return nil
	}

	db.logger.Info("Running database migrations", map[string]interface{}{
		"from_version": version,
		"to_version":   currentSchemaVersion,
	})

	// Run migrations sequentially
	// Add migration functions here as schema evolves
	// Example:
	// if version < 2 {
	//     if err := db.migrateToV2(); err != nil {
	//         return err
	//     }
	// }

	return nil
}

// getSchemaVersion gets the current schema version
func (db *DB) getSchemaVersion() (int, error) {
	// Check if schema_version table exists
	var tableName string
	err := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&tableName)

	if err == sql.ErrNoRows {
		// Table doesn't exist, this is a new database
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	// Get version
	var version int
	err = db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return version, nil
}

// setSchemaVersion sets the schema version
func setSchemaVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec("DELETE FROM schema_version")
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", version)
	return err
}

// createSchemaVersionTable creates the schema_version tracking table
func createSchemaVersionTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)
	`)
	return err
}

// createSymbolMappingsTable creates the symbol_mappings table
// Section 4.3: Symbol mappings with tombstones
func createSymbolMappingsTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS symbol_mappings (
			stable_id TEXT PRIMARY KEY,
			state TEXT NOT NULL CHECK(state IN ('active', 'deleted', 'unknown')),
			backend_stable_id TEXT,
			fingerprint_json TEXT NOT NULL,
			location_json TEXT NOT NULL,
			definition_version_id TEXT,
			definition_version_semantics TEXT,
			last_verified_at TEXT NOT NULL,
			last_verified_state_id TEXT NOT NULL,
			deleted_at TEXT,
			deleted_in_state_id TEXT,

			-- Constraints
			CHECK(
				(state = 'deleted' AND deleted_at IS NOT NULL AND deleted_in_state_id IS NOT NULL) OR
				(state != 'deleted' AND deleted_at IS NULL AND deleted_in_state_id IS NULL)
			)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create symbol_mappings table: %w", err)
	}

	// Create indexes for common queries
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_symbol_mappings_state ON symbol_mappings(state)",
		"CREATE INDEX IF NOT EXISTS idx_symbol_mappings_backend_stable_id ON symbol_mappings(backend_stable_id)",
		"CREATE INDEX IF NOT EXISTS idx_symbol_mappings_last_verified_state_id ON symbol_mappings(last_verified_state_id)",
		"CREATE INDEX IF NOT EXISTS idx_symbol_mappings_deleted_in_state_id ON symbol_mappings(deleted_in_state_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// createSymbolAliasesTable creates the symbol_aliases table
// Section 4.4: Alias/redirect table
func createSymbolAliasesTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS symbol_aliases (
			old_stable_id TEXT NOT NULL,
			new_stable_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			confidence REAL NOT NULL CHECK(confidence >= 0.0 AND confidence <= 1.0),
			created_at TEXT NOT NULL,
			created_state_id TEXT NOT NULL,

			PRIMARY KEY (old_stable_id, new_stable_id),
			FOREIGN KEY (old_stable_id) REFERENCES symbol_mappings(stable_id) ON DELETE CASCADE,
			FOREIGN KEY (new_stable_id) REFERENCES symbol_mappings(stable_id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create symbol_aliases table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_symbol_aliases_new_stable_id ON symbol_aliases(new_stable_id)",
		"CREATE INDEX IF NOT EXISTS idx_symbol_aliases_created_state_id ON symbol_aliases(created_state_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// createModulesTable creates the modules table
func createModulesTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS modules (
			module_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			root_path TEXT NOT NULL,
			manifest_type TEXT,
			detected_at TEXT NOT NULL,
			state_id TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create modules table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_modules_name ON modules(name)",
		"CREATE INDEX IF NOT EXISTS idx_modules_root_path ON modules(root_path)",
		"CREATE INDEX IF NOT EXISTS idx_modules_state_id ON modules(state_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// createDependencyEdgesTable creates the dependency_edges table
func createDependencyEdgesTable(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS dependency_edges (
			from_module TEXT NOT NULL,
			to_module TEXT NOT NULL,
			kind TEXT NOT NULL,
			strength INTEGER NOT NULL,

			PRIMARY KEY (from_module, to_module),
			FOREIGN KEY (from_module) REFERENCES modules(module_id) ON DELETE CASCADE,
			FOREIGN KEY (to_module) REFERENCES modules(module_id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create dependency_edges table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_dependency_edges_to_module ON dependency_edges(to_module)",
		"CREATE INDEX IF NOT EXISTS idx_dependency_edges_kind ON dependency_edges(kind)",
	}

	for _, indexSQL := range indexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// createCacheTablesTable creates cache tables for all cache tiers
// Section 9: Cache tiers (query, view, negative)
func createCacheTablesTable(tx *sql.Tx) error {
	// Query cache table (TTL 300s, key includes headCommit)
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS query_cache (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			state_id TEXT NOT NULL,
			head_commit TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create query_cache table: %w", err)
	}

	// View cache table (TTL 3600s, key includes repoStateId)
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS view_cache (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			state_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create view_cache table: %w", err)
	}

	// Negative cache table (TTL 60s, key includes repoStateId)
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS negative_cache (
			key TEXT PRIMARY KEY,
			error_type TEXT NOT NULL,
			error_message TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			state_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create negative_cache table: %w", err)
	}

	// Create indexes for cache cleanup and lookup
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_query_cache_expires_at ON query_cache(expires_at)",
		"CREATE INDEX IF NOT EXISTS idx_query_cache_state_id ON query_cache(state_id)",
		"CREATE INDEX IF NOT EXISTS idx_view_cache_expires_at ON view_cache(expires_at)",
		"CREATE INDEX IF NOT EXISTS idx_view_cache_state_id ON view_cache(state_id)",
		"CREATE INDEX IF NOT EXISTS idx_negative_cache_expires_at ON negative_cache(expires_at)",
		"CREATE INDEX IF NOT EXISTS idx_negative_cache_state_id ON negative_cache(state_id)",
		"CREATE INDEX IF NOT EXISTS idx_negative_cache_error_type ON negative_cache(error_type)",
	}

	for _, indexSQL := range indexes {
		if _, err := tx.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create cache index: %w", err)
		}
	}

	return nil
}

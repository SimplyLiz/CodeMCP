package federation

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"ckb/internal/paths"
)

// Index represents the federation index database
type Index struct {
	db   *sql.DB
	name string
}

// indexSchema is the SQL schema for the federation index database
const indexSchema = `
-- Schema version
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

-- Federation repository metadata
CREATE TABLE IF NOT EXISTS federation_repos (
    repo_uid TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL UNIQUE,
    path TEXT NOT NULL,
    tags TEXT,                        -- JSON array
    schema_version INTEGER,
    last_synced_at TEXT,
    last_state_id TEXT,               -- RepoStateID from last sync
    status TEXT NOT NULL DEFAULT 'active',  -- active, stale, offline
    indexed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_fed_repos_id ON federation_repos(repo_id);
CREATE INDEX IF NOT EXISTS idx_fed_repos_status ON federation_repos(status);

-- Module summaries (materialized from repo DBs)
CREATE TABLE IF NOT EXISTS federated_modules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_uid TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    module_id TEXT NOT NULL,
    name TEXT NOT NULL,
    responsibility TEXT,
    owner_ref TEXT,
    tags TEXT,                        -- JSON array
    symbol_count INTEGER,
    file_count INTEGER,
    source TEXT,                      -- declared or inferred
    confidence REAL,
    synced_at TEXT NOT NULL,
    FOREIGN KEY (repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fed_modules_repo ON federated_modules(repo_uid);
CREATE INDEX IF NOT EXISTS idx_fed_modules_name ON federated_modules(name);

-- FTS for module search
CREATE VIRTUAL TABLE IF NOT EXISTS federated_modules_fts USING fts5(
    name, responsibility, tags,
    content='federated_modules',
    content_rowid='id'
);

-- Ownership summaries
CREATE TABLE IF NOT EXISTS federated_ownership (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_uid TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    pattern TEXT NOT NULL,
    owners TEXT NOT NULL,             -- JSON array
    scope TEXT NOT NULL,              -- maintainer, reviewer, contributor
    source TEXT NOT NULL,             -- codeowners, git-blame, declared
    confidence REAL NOT NULL,
    synced_at TEXT NOT NULL,
    FOREIGN KEY (repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fed_ownership_repo ON federated_ownership(repo_uid);
CREATE INDEX IF NOT EXISTS idx_fed_ownership_pattern ON federated_ownership(pattern);

-- Hotspot top-N per repo
CREATE TABLE IF NOT EXISTS federated_hotspots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_uid TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    target_type TEXT NOT NULL,        -- file, module, symbol
    score REAL NOT NULL,
    churn_commits_30d INTEGER,
    complexity_cyclomatic REAL,
    coupling_instability REAL,
    synced_at TEXT NOT NULL,
    FOREIGN KEY (repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fed_hotspots_repo ON federated_hotspots(repo_uid);
CREATE INDEX IF NOT EXISTS idx_fed_hotspots_score ON federated_hotspots(score DESC);

-- Decision metadata
CREATE TABLE IF NOT EXISTS federated_decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_uid TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    decision_id TEXT NOT NULL,        -- ADR-001 etc.
    title TEXT NOT NULL,
    status TEXT NOT NULL,             -- proposed, accepted, deprecated, superseded
    affected_modules TEXT,            -- JSON array
    author TEXT,
    created_at TEXT,
    synced_at TEXT NOT NULL,
    FOREIGN KEY (repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fed_decisions_repo ON federated_decisions(repo_uid);
CREATE INDEX IF NOT EXISTS idx_fed_decisions_status ON federated_decisions(status);

-- FTS for decision search
CREATE VIRTUAL TABLE IF NOT EXISTS federated_decisions_fts USING fts5(
    title, affected_modules,
    content='federated_decisions',
    content_rowid='id'
);

-- Sync log for tracking sync operations
CREATE TABLE IF NOT EXISTS sync_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_uid TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    status TEXT NOT NULL,             -- running, success, failed, partial
    modules_synced INTEGER DEFAULT 0,
    ownership_synced INTEGER DEFAULT 0,
    hotspots_synced INTEGER DEFAULT 0,
    decisions_synced INTEGER DEFAULT 0,
    error TEXT
);
CREATE INDEX IF NOT EXISTS idx_sync_log_repo ON sync_log(repo_uid);

-- v6.3 Contract tables
-- Contracts table stores detected API contracts (proto, openapi, graphql)
CREATE TABLE IF NOT EXISTS contracts (
    id TEXT PRIMARY KEY,              -- repoUid:path (stable)
    repo_uid TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    path TEXT NOT NULL,

    contract_type TEXT NOT NULL,      -- "proto" | "openapi" | "graphql"

    -- Parsed metadata (JSON)
    metadata TEXT NOT NULL,

    -- Classification
    visibility TEXT NOT NULL,         -- "public" | "internal" | "unknown"
    visibility_basis TEXT,
    confidence REAL NOT NULL,

    -- For import resolution (JSON array)
    import_keys TEXT,

    indexed_at TEXT NOT NULL,

    FOREIGN KEY (repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_contracts_repo ON contracts(repo_uid);
CREATE INDEX IF NOT EXISTS idx_contracts_type ON contracts(contract_type);
CREATE INDEX IF NOT EXISTS idx_contracts_visibility ON contracts(visibility);

-- Import key lookup (for fast resolution)
CREATE TABLE IF NOT EXISTS contract_import_keys (
    import_key TEXT NOT NULL,
    contract_id TEXT NOT NULL,
    PRIMARY KEY (import_key, contract_id),
    FOREIGN KEY (contract_id) REFERENCES contracts(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_import_keys_key ON contract_import_keys(import_key);

-- Cross-repo dependency edges
CREATE TABLE IF NOT EXISTS contract_edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Unique constraint to prevent duplicates
    edge_key TEXT NOT NULL UNIQUE,    -- hash(source + consumer + evidence_type)

    -- The contract being consumed
    contract_id TEXT NOT NULL,

    -- The consumer
    consumer_repo_uid TEXT NOT NULL,
    consumer_repo_id TEXT NOT NULL,
    consumer_paths TEXT NOT NULL,     -- JSON array of files proving consumption

    -- Evidence classification
    tier TEXT NOT NULL,               -- "declared" | "derived" | "heuristic"
    evidence_type TEXT NOT NULL,      -- "proto_import" | "generated_code" | etc.
    evidence_details TEXT,            -- JSON
    confidence REAL NOT NULL,
    confidence_basis TEXT,

    -- Detector metadata
    detector_name TEXT NOT NULL,
    detected_at TEXT NOT NULL,

    -- Manual overrides
    suppressed INTEGER DEFAULT 0,
    suppressed_by TEXT,
    suppressed_at TEXT,
    suppression_reason TEXT,
    verified INTEGER DEFAULT 0,
    verified_by TEXT,
    verified_at TEXT,

    FOREIGN KEY (contract_id) REFERENCES contracts(id) ON DELETE CASCADE,
    FOREIGN KEY (consumer_repo_uid) REFERENCES federation_repos(repo_uid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_edges_contract ON contract_edges(contract_id);
CREATE INDEX IF NOT EXISTS idx_edges_consumer ON contract_edges(consumer_repo_uid);
CREATE INDEX IF NOT EXISTS idx_edges_tier ON contract_edges(tier);
CREATE INDEX IF NOT EXISTS idx_edges_suppressed ON contract_edges(suppressed);

-- Proto import graph (for transitive analysis)
CREATE TABLE IF NOT EXISTS proto_imports (
    importer_contract_id TEXT NOT NULL,
    imported_contract_id TEXT NOT NULL,
    import_path TEXT NOT NULL,        -- the actual import string
    PRIMARY KEY (importer_contract_id, imported_contract_id),
    FOREIGN KEY (importer_contract_id) REFERENCES contracts(id) ON DELETE CASCADE,
    FOREIGN KEY (imported_contract_id) REFERENCES contracts(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_proto_imports_imported ON proto_imports(imported_contract_id);
`

const currentSchemaVersion = 1

// OpenIndex opens or creates the federation index database
func OpenIndex(federationName string) (*Index, error) {
	indexPath, err := paths.GetFederationIndexPath(federationName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index path: %w", err)
	}

	// Ensure the federation directory exists
	if _, err := paths.EnsureFederationDir(federationName); err != nil {
		return nil, fmt.Errorf("failed to create federation directory: %w", err)
	}

	db, err := sql.Open("sqlite", indexPath+"?_journal_mode=WAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	idx := &Index{
		db:   db,
		name: federationName,
	}

	// Initialize schema
	if err := idx.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return idx, nil
}

// initSchema creates the database schema if it doesn't exist
func (idx *Index) initSchema() error {
	// Check schema version
	var version int
	err := idx.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		// New database, create schema
		if _, err := idx.db.Exec(indexSchema); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
		if _, err := idx.db.Exec("INSERT INTO schema_version (version) VALUES (?)", currentSchemaVersion); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	} else if err != nil {
		// Table might not exist, try creating schema
		if _, err := idx.db.Exec(indexSchema); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
		if _, err := idx.db.Exec("INSERT OR REPLACE INTO schema_version (version) VALUES (?)", currentSchemaVersion); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}
	// If version exists and matches, we're good

	return nil
}

// Close closes the index database
func (idx *Index) Close() error {
	return idx.db.Close()
}

// DB returns the underlying database connection
func (idx *Index) DB() *sql.DB {
	return idx.db
}

// FederatedRepo represents a repository in the federation index
type FederatedRepo struct {
	RepoUID       string
	RepoID        string
	Path          string
	Tags          string // JSON array
	SchemaVersion int
	LastSyncedAt  *time.Time
	LastStateID   string
	Status        string
	IndexedAt     time.Time
}

// UpsertRepo inserts or updates a repository in the federation index
func (idx *Index) UpsertRepo(repo *FederatedRepo) error {
	_, err := idx.db.Exec(`
		INSERT INTO federation_repos (repo_uid, repo_id, path, tags, schema_version, last_synced_at, last_state_id, status, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_uid) DO UPDATE SET
			repo_id = excluded.repo_id,
			path = excluded.path,
			tags = excluded.tags,
			schema_version = excluded.schema_version,
			last_synced_at = excluded.last_synced_at,
			last_state_id = excluded.last_state_id,
			status = excluded.status,
			indexed_at = excluded.indexed_at
	`, repo.RepoUID, repo.RepoID, repo.Path, repo.Tags, repo.SchemaVersion,
		formatTime(repo.LastSyncedAt), repo.LastStateID, repo.Status, formatTime(&repo.IndexedAt))
	return err
}

// GetRepo returns a repository by UID
func (idx *Index) GetRepo(repoUID string) (*FederatedRepo, error) {
	repo := &FederatedRepo{}
	var lastSynced, indexed sql.NullString

	err := idx.db.QueryRow(`
		SELECT repo_uid, repo_id, path, tags, schema_version, last_synced_at, last_state_id, status, indexed_at
		FROM federation_repos WHERE repo_uid = ?
	`, repoUID).Scan(
		&repo.RepoUID, &repo.RepoID, &repo.Path, &repo.Tags, &repo.SchemaVersion,
		&lastSynced, &repo.LastStateID, &repo.Status, &indexed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	repo.LastSyncedAt = parseTime(lastSynced.String)
	if indexed.Valid {
		repo.IndexedAt = *parseTime(indexed.String)
	}

	return repo, nil
}

// ListRepos returns all repositories in the federation
func (idx *Index) ListRepos() ([]*FederatedRepo, error) {
	rows, err := idx.db.Query(`
		SELECT repo_uid, repo_id, path, tags, schema_version, last_synced_at, last_state_id, status, indexed_at
		FROM federation_repos ORDER BY repo_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*FederatedRepo
	for rows.Next() {
		repo := &FederatedRepo{}
		var lastSynced, indexed sql.NullString

		if err := rows.Scan(
			&repo.RepoUID, &repo.RepoID, &repo.Path, &repo.Tags, &repo.SchemaVersion,
			&lastSynced, &repo.LastStateID, &repo.Status, &indexed); err != nil {
			return nil, err
		}

		repo.LastSyncedAt = parseTime(lastSynced.String)
		if indexed.Valid {
			repo.IndexedAt = *parseTime(indexed.String)
		}
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// DeleteRepo removes a repository and all its indexed data
func (idx *Index) DeleteRepo(repoUID string) error {
	_, err := idx.db.Exec("DELETE FROM federation_repos WHERE repo_uid = ?", repoUID)
	return err
}

// ClearRepoData clears all indexed data for a repository (but keeps the repo entry)
func (idx *Index) ClearRepoData(repoUID string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{
		"federated_modules",
		"federated_ownership",
		"federated_hotspots",
		"federated_decisions",
		"contracts",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE repo_uid = ?", table), repoUID); err != nil {
			return err
		}
	}

	// Also clear edges where this repo is the consumer
	if _, err := tx.Exec("DELETE FROM contract_edges WHERE consumer_repo_uid = ?", repoUID); err != nil {
		return err
	}

	return tx.Commit()
}

// Helper functions for time formatting
func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

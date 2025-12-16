package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"ckb/internal/logging"
)

// DB represents a database connection with transaction helpers
type DB struct {
	conn   *sql.DB
	logger *logging.Logger
	dbPath string
}

// Open opens or creates a SQLite database at .ckb/ckb.db
// If the database doesn't exist, it will be created along with all necessary tables
func Open(repoRoot string, logger *logging.Logger) (*DB, error) {
	// Ensure .ckb directory exists
	ckbDir := filepath.Join(repoRoot, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .ckb directory: %w", err)
	}

	// Database path
	dbPath := filepath.Join(ckbDir, "ckb.db")

	// Check if database needs to be created
	dbExists := fileExists(dbPath)

	// Open database connection
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set pragmas for performance and reliability
	pragmas := []string{
		"PRAGMA journal_mode=WAL",        // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL",      // Balance between safety and performance
		"PRAGMA foreign_keys=ON",         // Enable foreign key constraints
		"PRAGMA busy_timeout=5000",       // Wait up to 5 seconds on lock
		"PRAGMA cache_size=-64000",       // 64MB cache
		"PRAGMA temp_store=MEMORY",       // Use memory for temp tables
		"PRAGMA mmap_size=268435456",     // 256MB mmap
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	db := &DB{
		conn:   conn,
		logger: logger,
		dbPath: dbPath,
	}

	// Initialize schema if database is new
	if !dbExists {
		logger.Info("Creating new database", map[string]interface{}{
			"path": dbPath,
		})
		if err := db.initializeSchema(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	} else {
		// Run migrations on existing database
		logger.Debug("Running database migrations", map[string]interface{}{
			"path": dbPath,
		})
		if err := db.runMigrations(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn returns the underlying sql.DB connection
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// WithTx executes a function within a transaction
// If the function returns an error, the transaction is rolled back
// Otherwise, the transaction is committed
func (db *DB) WithTx(fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			db.logger.Error("failed to rollback transaction", map[string]interface{}{
				"error":          err.Error(),
				"rollback_error": rbErr.Error(),
			})
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Exec executes a query without returning rows
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}

// Query executes a query that returns rows
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

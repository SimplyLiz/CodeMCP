package storage

// This file provides examples of how to use the storage layer
// It is not meant to be executed, but serves as documentation

import (
	"database/sql"
	"time"

	"ckb/internal/logging"
)

// ExampleBasicSetup demonstrates basic database initialization
func ExampleBasicSetup(repoRoot string) error {
	// Create logger
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	// Open database (creates if doesn't exist)
	db, err := Open(repoRoot, logger)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	// Database is ready to use
	return nil
}

// ExampleSymbolMappingCRUD demonstrates CRUD operations on symbol mappings
func ExampleSymbolMappingCRUD(db *DB) error {
	repo := NewSymbolRepository(db)

	// Create a new symbol mapping
	mapping := &SymbolMapping{
		StableID:            "sym-abc123",
		State:               "active",
		FingerprintJSON:     `{"name":"myFunction","kind":"function","signature":"func myFunction()"}`,
		LocationJSON:        `{"path":"src/main.go","line":42,"column":5}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-xyz789",
	}

	if err := repo.Create(mapping); err != nil {
		return err
	}

	// Retrieve the mapping
	retrieved, err := repo.GetByStableID("sym-abc123")
	if err != nil {
		return err
	}

	// Update the mapping
	retrieved.State = "active"
	if updateErr := repo.Update(retrieved); updateErr != nil {
		return updateErr
	}

	// Mark as deleted (tombstone)
	if deleteErr := repo.MarkAsDeleted("sym-abc123", "state-new456"); deleteErr != nil {
		return deleteErr
	}

	// List all active symbols
	activeSymbols, err := repo.ListByState("active", 100)
	if err != nil {
		return err
	}
	_ = activeSymbols

	return nil
}

// ExampleAliasManagement demonstrates symbol alias operations
func ExampleAliasManagement(db *DB) error {
	// First create the symbol mappings that we'll alias
	symbolRepo := NewSymbolRepository(db)

	oldSymbol := &SymbolMapping{
		StableID:            "sym-old-123",
		State:               "deleted",
		FingerprintJSON:     `{"name":"oldFunction"}`,
		LocationJSON:        `{"path":"old/path.go","line":10}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-1",
		DeletedAt:           timePtrHelper(time.Now()),
		DeletedInStateID:    strPtrHelper("state-1"),
	}

	newSymbol := &SymbolMapping{
		StableID:            "sym-new-456",
		State:               "active",
		FingerprintJSON:     `{"name":"newFunction"}`,
		LocationJSON:        `{"path":"new/path.go","line":20}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-1",
	}

	if err := symbolRepo.Create(oldSymbol); err != nil {
		return err
	}
	if err := symbolRepo.Create(newSymbol); err != nil {
		return err
	}

	// Create an alias
	aliasRepo := NewAliasRepository(db)
	alias := &SymbolAlias{
		OldStableID:    "sym-old-123",
		NewStableID:    "sym-new-456",
		Reason:         "refactored",
		Confidence:     0.95,
		CreatedAt:      time.Now(),
		CreatedStateID: "state-2",
	}

	if err := aliasRepo.Create(alias); err != nil {
		return err
	}

	// Look up aliases
	aliases, err := aliasRepo.GetByOldStableID("sym-old-123")
	if err != nil {
		return err
	}
	_ = aliases

	return nil
}

// ExampleCacheUsage demonstrates cache operations
func ExampleCacheUsage(db *DB) error {
	cache := NewCache(db)

	// Query cache (includes head commit)
	err := cache.SetQueryCache(
		"query:definition:myFunction",
		`{"result":"found","location":"src/main.go:42"}`,
		"commit-abc123",
		"state-xyz789",
		300, // TTL 300 seconds
	)
	if err != nil {
		return err
	}

	// Retrieve from query cache
	value, found, err := cache.GetQueryCache("query:definition:myFunction", "commit-abc123")
	if err != nil {
		return err
	}
	if found {
		// Use cached value
		_ = value
	}

	// View cache (includes state ID)
	err = cache.SetViewCache(
		"view:module-graph",
		`{"modules":[{"id":"mod-1","name":"core"}]}`,
		"state-xyz789",
		3600, // TTL 3600 seconds
	)
	if err != nil {
		return err
	}

	// Negative cache
	err = cache.SetNegativeCache(
		"symbol:notfound:nonExistentFunc",
		"symbol-not-found",
		"Symbol 'nonExistentFunc' not found in codebase",
		"state-xyz789",
		60, // TTL 60 seconds
	)
	if err != nil {
		return err
	}

	// Invalidate cache when state changes
	err = cache.InvalidateByStateID("state-xyz789")
	if err != nil {
		return err
	}

	// Cleanup expired entries
	err = cache.CleanupExpiredEntries()
	if err != nil {
		return err
	}

	return nil
}

// ExampleNegativeCacheWithPolicies demonstrates negative cache with error type policies
func ExampleNegativeCacheWithPolicies(db *DB) error {
	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache different error types (TTL is automatically determined by policy)

	// Symbol not found (TTL: 60s)
	err := manager.CacheError(
		"symbol:foo",
		SymbolNotFound,
		"Symbol 'foo' not found",
		"state-123",
	)
	if err != nil {
		return err
	}

	// Backend unavailable (TTL: 15s)
	err = manager.CacheError(
		"backend:lsp:typescript",
		BackendUnavailable,
		"TypeScript language server is not running",
		"state-123",
	)
	if err != nil {
		return err
	}

	// Workspace not ready (TTL: 10s, triggers warmup)
	err = manager.CacheError(
		"workspace:dart",
		WorkspaceNotReady,
		"Dart workspace is initializing",
		"state-123",
	)
	if err != nil {
		return err
	}

	// Check if error is cached
	entry, err := manager.CheckError("symbol:foo", "state-123")
	if err != nil {
		return err
	}
	if entry != nil {
		// Error is cached, use cached error
		_ = entry.ErrorType
		_ = entry.ErrorMessage
	}

	// Get statistics about cached errors
	stats, err := manager.GetErrorStats()
	if err != nil {
		return err
	}
	_ = stats

	return nil
}

// ExampleModuleAndDependencies demonstrates module and dependency tracking
func ExampleModuleAndDependencies(db *DB) error {
	moduleRepo := NewModuleRepository(db)
	depRepo := NewDependencyRepository(db)

	// Create modules
	coreModule := &Module{
		ModuleID:   "mod-core",
		Name:       "core",
		RootPath:   "src/core",
		DetectedAt: time.Now(),
		StateID:    "state-123",
	}

	utilsModule := &Module{
		ModuleID:   "mod-utils",
		Name:       "utils",
		RootPath:   "src/utils",
		DetectedAt: time.Now(),
		StateID:    "state-123",
	}

	if err := moduleRepo.Create(coreModule); err != nil {
		return err
	}
	if err := moduleRepo.Create(utilsModule); err != nil {
		return err
	}

	// Create dependency edge
	edge := &DependencyEdge{
		FromModule: "mod-core",
		ToModule:   "mod-utils",
		Kind:       "import",
		Strength:   10,
	}

	if err := depRepo.Create(edge); err != nil {
		return err
	}

	// Query dependencies
	deps, err := depRepo.GetByFromModule("mod-core")
	if err != nil {
		return err
	}
	_ = deps

	// Query reverse dependencies
	reverseDeps, err := depRepo.GetByToModule("mod-utils")
	if err != nil {
		return err
	}
	_ = reverseDeps

	// List all modules
	allModules, err := moduleRepo.ListAll()
	if err != nil {
		return err
	}
	_ = allModules

	return nil
}

// ExampleTransactions demonstrates transaction usage
func ExampleTransactions(db *DB) error {
	// Execute operations within a transaction
	err := db.WithTx(func(tx *sql.Tx) error {
		// All operations within this function are part of the same transaction
		// If any operation fails, the entire transaction is rolled back

		_, err := tx.Exec(`
			INSERT INTO symbol_mappings (
				stable_id, state, fingerprint_json, location_json,
				last_verified_at, last_verified_state_id
			) VALUES (?, ?, ?, ?, ?, ?)
		`, "sym-1", "active", "{}", "{}", time.Now().Format(time.RFC3339), "state-1")

		if err != nil {
			return err // Transaction will be rolled back
		}

		_, err = tx.Exec(`
			INSERT INTO symbol_mappings (
				stable_id, state, fingerprint_json, location_json,
				last_verified_at, last_verified_state_id
			) VALUES (?, ?, ?, ?, ?, ?)
		`, "sym-2", "active", "{}", "{}", time.Now().Format(time.RFC3339), "state-1")

		if err != nil {
			return err // Transaction will be rolled back
		}

		// If we reach here, transaction will be committed
		return nil
	})

	return err
}

// Helper functions used in examples
func timePtrHelper(t time.Time) *time.Time {
	return &t
}

func strPtrHelper(s string) *string {
	return &s
}

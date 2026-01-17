package identity

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"ckb/internal/storage"
)

func setupTestDB(t *testing.T) (*storage.DB, string) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ckb-identity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Open database
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	return db, tmpDir
}

func cleanupTestDB(db *storage.DB, tmpDir string) {
	if db != nil {
		_ = db.Close()
	}
	_ = os.RemoveAll(tmpDir)
}

func TestFingerprintComputation(t *testing.T) {
	fp := &SymbolFingerprint{
		QualifiedContainer:  "mypackage.MyClass",
		Name:                "myMethod",
		Kind:                KindMethod,
		Arity:               2,
		SignatureNormalized: "myMethod(string,int)",
	}

	// Compute fingerprint
	hash := ComputeStableFingerprint(fp)
	if hash == "" {
		t.Fatal("expected non-empty fingerprint hash")
	}

	// Should be deterministic
	hash2 := ComputeStableFingerprint(fp)
	if hash != hash2 {
		t.Errorf("fingerprint not deterministic: %s != %s", hash, hash2)
	}

	// Generate stable ID
	stableId := GenerateStableId("test-repo", fp)
	if stableId == "" {
		t.Fatal("expected non-empty stable ID")
	}

	expectedPrefix := "ckb:test-repo:sym:"
	if len(stableId) <= len(expectedPrefix) {
		t.Errorf("stable ID too short: %s", stableId)
	}
}

func TestSymbolRepository(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)

	// Create a test symbol
	fp := &SymbolFingerprint{
		QualifiedContainer:  "pkg.Class",
		Name:                "method",
		Kind:                KindMethod,
		Arity:               1,
		SignatureNormalized: "method(int)",
	}

	stableId := GenerateStableId("test-repo", fp)
	now := time.Now().UTC().Format(time.RFC3339)

	mapping := &SymbolMapping{
		StableId:                   stableId,
		BackendStableId:            "scip:test:123",
		Fingerprint:                fp,
		State:                      StateActive,
		Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
		LocationFreshness:          Fresh,
		DefinitionVersionId:        "v1",
		DefinitionVersionSemantics: BackendDefinitionHash,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-123",
	}

	// Test Create
	err := repo.Create(mapping)
	if err != nil {
		t.Fatalf("failed to create symbol: %v", err)
	}

	// Test Get
	retrieved, err := repo.Get(stableId)
	if err != nil {
		t.Fatalf("failed to get symbol: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected symbol to be found")
	}

	if retrieved.StableId != stableId {
		t.Errorf("stable ID mismatch: got %s, want %s", retrieved.StableId, stableId)
	}

	if retrieved.State != StateActive {
		t.Errorf("state mismatch: got %s, want %s", retrieved.State, StateActive)
	}

	// Test GetByBackendId
	retrieved2, err := repo.GetByBackendId("scip:test:123")
	if err != nil {
		t.Fatalf("failed to get by backend ID: %v", err)
	}

	if retrieved2 == nil {
		t.Fatal("expected symbol to be found by backend ID")
	}

	if retrieved2.StableId != stableId {
		t.Errorf("stable ID mismatch: got %s, want %s", retrieved2.StableId, stableId)
	}

	// Test MarkDeleted
	err = repo.MarkDeleted(stableId, "state-456")
	if err != nil {
		t.Fatalf("failed to mark deleted: %v", err)
	}

	// Verify deletion
	deleted, err := repo.Get(stableId)
	if err != nil {
		t.Fatalf("failed to get deleted symbol: %v", err)
	}

	if !deleted.IsDeleted() {
		t.Error("expected symbol to be deleted")
	}

	if deleted.DeletedAt == "" {
		t.Error("expected deletedAt to be set")
	}
}

func TestAliasResolution(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	resolver := NewIdentityResolver(db, logger)

	// Create two symbols
	fp1 := &SymbolFingerprint{
		QualifiedContainer: "pkg",
		Name:               "oldMethod",
		Kind:               KindMethod,
	}
	fp2 := &SymbolFingerprint{
		QualifiedContainer: "pkg",
		Name:               "newMethod",
		Kind:               KindMethod,
	}

	oldId := GenerateStableId("test-repo", fp1)
	newId := GenerateStableId("test-repo", fp2)
	now := time.Now().UTC().Format(time.RFC3339)

	// Create old symbol as a tombstone (deleted state) - FK constraint requires it to exist
	oldMapping := &SymbolMapping{
		StableId:                   oldId,
		Fingerprint:                fp1,
		State:                      StateDeleted,
		Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
		LocationFreshness:          MayBeStale,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-1",
		DeletedAt:                  now,
		DeletedInStateId:           "state-2",
	}

	// Create new symbol
	newMapping := &SymbolMapping{
		StableId:                   newId,
		Fingerprint:                fp2,
		State:                      StateActive,
		Location:                   &Location{Path: "src/test.go", Line: 15, Column: 5},
		LocationFreshness:          Fresh,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-2",
	}

	// Create old symbol first (FK constraint), then new symbol
	if err := repo.Create(oldMapping); err != nil {
		t.Fatalf("failed to create old mapping: %v", err)
	}
	if err := repo.Create(newMapping); err != nil {
		t.Fatalf("failed to create new mapping: %v", err)
	}

	// Create alias
	alias := &SymbolAlias{
		OldStableId:    oldId,
		NewStableId:    newId,
		Reason:         ReasonRenamed,
		Confidence:     0.95,
		CreatedAt:      now,
		CreatedStateId: "state-2",
	}

	creator := NewAliasCreator(db, logger)
	if err := creator.createAlias(alias); err != nil {
		t.Fatalf("failed to create alias: %v", err)
	}

	// Test resolution - requesting old ID should resolve to new symbol
	resolved, err := resolver.ResolveSymbolId(oldId)
	if err != nil {
		t.Fatalf("failed to resolve symbol: %v", err)
	}

	if resolved.Symbol == nil {
		t.Fatal("expected resolved symbol")
	}

	if resolved.Symbol.StableId != newId {
		t.Errorf("expected resolution to new ID, got %s", resolved.Symbol.StableId)
	}

	if !resolved.Redirected {
		t.Error("expected redirected flag to be true")
	}

	if resolved.RedirectedFrom != oldId {
		t.Errorf("expected redirectedFrom to be %s, got %s", oldId, resolved.RedirectedFrom)
	}

	if resolved.RedirectReason != ReasonRenamed {
		t.Errorf("expected redirect reason %s, got %s", ReasonRenamed, resolved.RedirectReason)
	}
}

func TestAliasCreation(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	creator := NewAliasCreator(db, logger)
	now := time.Now().UTC().Format(time.RFC3339)

	// Create old mappings
	oldMappings := []*SymbolMapping{
		{
			StableId:        "ckb:test:sym:old1",
			BackendStableId: "scip:backend:123",
			Fingerprint: &SymbolFingerprint{
				QualifiedContainer: "pkg",
				Name:               "oldName",
				Kind:               KindFunction,
			},
			State:                      StateActive,
			Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
			LocationFreshness:          Fresh,
			DefinitionVersionSemantics: UnknownSemantics,
			LastVerifiedAt:             now,
			LastVerifiedStateId:        "state-1",
		},
	}

	// Create new mappings (renamed symbol with same backend ID)
	newMappings := []*SymbolMapping{
		{
			StableId:        "ckb:test:sym:new1",
			BackendStableId: "scip:backend:123", // Same backend ID
			Fingerprint: &SymbolFingerprint{
				QualifiedContainer: "pkg",
				Name:               "newName", // Different name
				Kind:               KindFunction,
			},
			State:                      StateActive,
			Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
			LocationFreshness:          Fresh,
			DefinitionVersionSemantics: UnknownSemantics,
			LastVerifiedAt:             now,
			LastVerifiedStateId:        "state-2",
		},
	}

	// Insert both OLD and NEW mappings into DB
	// Old symbols will be marked as deleted by CreateAliasesOnRefresh if an alias is created
	// FK constraint requires both to exist in symbol_mappings table
	for _, m := range oldMappings {
		if err := repo.Create(m); err != nil {
			t.Fatalf("failed to create old mapping: %v", err)
		}
	}
	for _, m := range newMappings {
		if err := repo.Create(m); err != nil {
			t.Fatalf("failed to create new mapping: %v", err)
		}
	}

	// Run alias creation
	err := creator.CreateAliasesOnRefresh(oldMappings, newMappings, "state-2")
	if err != nil {
		t.Fatalf("failed to create aliases: %v", err)
	}

	// Verify alias was created
	resolver := NewIdentityResolver(db, logger)
	resolved, err := resolver.ResolveSymbolId("ckb:test:sym:old1")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}

	if resolved.Symbol == nil {
		t.Fatal("expected resolved symbol")
	}

	if resolved.Symbol.StableId != "ckb:test:sym:new1" {
		t.Errorf("expected resolution to new ID, got %s", resolved.Symbol.StableId)
	}

	if !resolved.Redirected {
		t.Error("expected redirected flag")
	}
}

func TestBackendIdRoles(t *testing.T) {
	tests := []struct {
		backendId    string
		expectedRole BackendIdRole
		canAnchor    bool
	}{
		{"scip:github.com/test/repo:123", RolePrimaryAnchor, true},
		{"glean:some:id:here", RolePrimaryAnchor, true},
		{"file:///path/to/file.ts#L10", RoleResolverOnly, false},
		{"", RoleResolverOnly, false},
		{"unknown-format", RoleResolverOnly, false},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			role := GetBackendIdRole(tt.backendId)
			if role != tt.expectedRole {
				t.Errorf("GetBackendIdRole(%q) = %v, want %v", tt.backendId, role, tt.expectedRole)
			}

			canAnchor := CanBeIdAnchor(tt.backendId)
			if canAnchor != tt.canAnchor {
				t.Errorf("CanBeIdAnchor(%q) = %v, want %v", tt.backendId, canAnchor, tt.canAnchor)
			}
		})
	}
}

func TestTombstones(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	resolver := NewIdentityResolver(db, logger)

	// Create and then delete a symbol
	fp := &SymbolFingerprint{
		QualifiedContainer: "pkg",
		Name:               "deletedMethod",
		Kind:               KindMethod,
	}

	stableId := GenerateStableId("test-repo", fp)
	now := time.Now().UTC().Format(time.RFC3339)

	mapping := &SymbolMapping{
		StableId:                   stableId,
		Fingerprint:                fp,
		State:                      StateActive,
		Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
		LocationFreshness:          Fresh,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-1",
	}

	if err := repo.Create(mapping); err != nil {
		t.Fatalf("failed to create symbol: %v", err)
	}

	// Delete the symbol
	if err := repo.MarkDeleted(stableId, "state-2"); err != nil {
		t.Fatalf("failed to delete symbol: %v", err)
	}

	// Resolve should return deleted status
	resolved, err := resolver.ResolveSymbolId(stableId)
	if err != nil {
		t.Fatalf("failed to resolve deleted symbol: %v", err)
	}

	if !resolved.Deleted {
		t.Error("expected deleted flag to be true")
	}

	if resolved.DeletedAt == "" {
		t.Error("expected deletedAt to be set")
	}

	if resolved.Symbol != nil {
		t.Error("expected symbol to be nil for deleted symbols")
	}
}

func TestAliasCycleDetection(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	resolver := NewIdentityResolver(db, logger)
	now := time.Now().UTC().Format(time.RFC3339)

	// Create two symbols that will form a cycle
	mapping1 := &SymbolMapping{
		StableId:                   "ckb:test:sym:cycle1",
		Fingerprint:                &SymbolFingerprint{Name: "cycle1", Kind: KindFunction},
		State:                      StateDeleted,
		Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
		LocationFreshness:          MayBeStale,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-1",
		DeletedAt:                  now,
		DeletedInStateId:           "state-1",
	}
	mapping2 := &SymbolMapping{
		StableId:                   "ckb:test:sym:cycle2",
		Fingerprint:                &SymbolFingerprint{Name: "cycle2", Kind: KindFunction},
		State:                      StateDeleted,
		Location:                   &Location{Path: "src/test.go", Line: 20, Column: 5},
		LocationFreshness:          MayBeStale,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-1",
		DeletedAt:                  now,
		DeletedInStateId:           "state-1",
	}

	if err := repo.Create(mapping1); err != nil {
		t.Fatalf("failed to create mapping1: %v", err)
	}
	if err := repo.Create(mapping2); err != nil {
		t.Fatalf("failed to create mapping2: %v", err)
	}

	// Create circular alias: cycle1 -> cycle2 -> cycle1
	creator := NewAliasCreator(db, logger)
	alias1 := &SymbolAlias{
		OldStableId:    "ckb:test:sym:cycle1",
		NewStableId:    "ckb:test:sym:cycle2",
		Reason:         ReasonRenamed,
		Confidence:     0.9,
		CreatedAt:      now,
		CreatedStateId: "state-1",
	}
	alias2 := &SymbolAlias{
		OldStableId:    "ckb:test:sym:cycle2",
		NewStableId:    "ckb:test:sym:cycle1",
		Reason:         ReasonRenamed,
		Confidence:     0.9,
		CreatedAt:      now,
		CreatedStateId: "state-1",
	}

	if err := creator.createAlias(alias1); err != nil {
		t.Fatalf("failed to create alias1: %v", err)
	}
	if err := creator.createAlias(alias2); err != nil {
		t.Fatalf("failed to create alias2: %v", err)
	}

	// Resolution should detect the cycle and return an error
	resolved, err := resolver.ResolveSymbolId("ckb:test:sym:cycle1")
	if err == nil {
		t.Error("expected error for alias cycle")
	}
	if resolved == nil {
		t.Fatal("expected resolved struct even on error")
	}
	if resolved.Error == "" {
		t.Error("expected error message in resolved struct")
	}
}

func TestAliasChainTooDeep(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	resolver := NewIdentityResolver(db, logger)
	now := time.Now().UTC().Format(time.RFC3339)
	creator := NewAliasCreator(db, logger)

	// Create a chain of 5 symbols (exceeding AliasChainMaxDepth of 3)
	numSymbols := AliasChainMaxDepth + 3
	symbols := make([]string, numSymbols)
	for i := 0; i < numSymbols; i++ {
		symbols[i] = fmt.Sprintf("ckb:test:sym:deep%d", i)
		state := StateDeleted
		if i == numSymbols-1 {
			state = StateActive // Last one is active
		}
		mapping := &SymbolMapping{
			StableId:                   symbols[i],
			Fingerprint:                &SymbolFingerprint{Name: fmt.Sprintf("deep%d", i), Kind: KindFunction},
			State:                      state,
			Location:                   &Location{Path: "src/test.go", Line: 10 + i, Column: 5},
			LocationFreshness:          Fresh,
			DefinitionVersionSemantics: UnknownSemantics,
			LastVerifiedAt:             now,
			LastVerifiedStateId:        "state-1",
		}
		if state == StateDeleted {
			mapping.DeletedAt = now
			mapping.DeletedInStateId = "state-1"
			mapping.LocationFreshness = MayBeStale
		}
		if err := repo.Create(mapping); err != nil {
			t.Fatalf("failed to create mapping %d: %v", i, err)
		}
	}

	// Create chain of aliases: deep0 -> deep1 -> deep2 -> deep3 -> deep4 -> deep5
	for i := 0; i < numSymbols-1; i++ {
		alias := &SymbolAlias{
			OldStableId:    symbols[i],
			NewStableId:    symbols[i+1],
			Reason:         ReasonRenamed,
			Confidence:     0.9,
			CreatedAt:      now,
			CreatedStateId: "state-1",
		}
		if err := creator.createAlias(alias); err != nil {
			t.Fatalf("failed to create alias %d: %v", i, err)
		}
	}

	// Resolution should fail due to max depth exceeded
	resolved, err := resolver.ResolveSymbolId(symbols[0])
	if err == nil {
		t.Error("expected error for alias chain too deep")
	}
	if resolved == nil {
		t.Fatal("expected resolved struct even on error")
	}
	if resolved.Error == "" {
		t.Error("expected error message in resolved struct")
	}
}

func TestSymbolNotFound(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	resolver := NewIdentityResolver(db, logger)

	// Try to resolve a non-existent symbol with no alias
	resolved, err := resolver.ResolveSymbolId("ckb:test:sym:nonexistent")
	if err == nil {
		t.Error("expected error for non-existent symbol")
	}
	if resolved == nil {
		t.Fatal("expected resolved struct even on error")
	}
	if resolved.Error == "" {
		t.Error("expected error message in resolved struct")
	}
	if resolved.Symbol != nil {
		t.Error("expected symbol to be nil for non-existent symbol")
	}
}

func TestDirectSymbolResolution(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer cleanupTestDB(db, tmpDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repo := NewSymbolRepository(db, logger)
	resolver := NewIdentityResolver(db, logger)
	now := time.Now().UTC().Format(time.RFC3339)

	// Create an active symbol
	mapping := &SymbolMapping{
		StableId:                   "ckb:test:sym:direct",
		Fingerprint:                &SymbolFingerprint{Name: "directFunc", Kind: KindFunction},
		State:                      StateActive,
		Location:                   &Location{Path: "src/test.go", Line: 10, Column: 5},
		LocationFreshness:          Fresh,
		DefinitionVersionSemantics: UnknownSemantics,
		LastVerifiedAt:             now,
		LastVerifiedStateId:        "state-1",
	}

	if err := repo.Create(mapping); err != nil {
		t.Fatalf("failed to create mapping: %v", err)
	}

	// Direct resolution should work without redirect
	resolved, err := resolver.ResolveSymbolId("ckb:test:sym:direct")
	if err != nil {
		t.Fatalf("failed to resolve symbol: %v", err)
	}

	if resolved.Symbol == nil {
		t.Fatal("expected symbol to be found")
	}
	if resolved.Symbol.StableId != "ckb:test:sym:direct" {
		t.Errorf("expected stable ID 'ckb:test:sym:direct', got %s", resolved.Symbol.StableId)
	}
	if resolved.Redirected {
		t.Error("expected no redirect for direct lookup")
	}
	if resolved.Deleted {
		t.Error("expected symbol not to be deleted")
	}
}

func TestAliasChainMaxDepthConstant(t *testing.T) {
	// Verify the constant is set correctly per spec
	if AliasChainMaxDepth != 3 {
		t.Errorf("AliasChainMaxDepth = %d, expected 3 per spec Section 4.5", AliasChainMaxDepth)
	}
}

func TestResolvedSymbolStruct(t *testing.T) {
	// Test that ResolvedSymbol struct has all expected fields
	resolved := &ResolvedSymbol{
		Symbol: &SymbolMapping{
			StableId: "test",
		},
		Redirected:         true,
		RedirectedFrom:     "old-id",
		RedirectReason:     ReasonRenamed,
		RedirectConfidence: 0.95,
		Deleted:            false,
		DeletedAt:          "",
		Error:              "",
	}

	if resolved.Symbol == nil {
		t.Error("expected symbol to be set")
	}
	if !resolved.Redirected {
		t.Error("expected redirected to be true")
	}
	if resolved.RedirectedFrom != "old-id" {
		t.Errorf("expected redirectedFrom 'old-id', got %s", resolved.RedirectedFrom)
	}
	if resolved.RedirectReason != ReasonRenamed {
		t.Errorf("expected reason %s, got %s", ReasonRenamed, resolved.RedirectReason)
	}
	if resolved.RedirectConfidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", resolved.RedirectConfidence)
	}
	if resolved.Deleted {
		t.Error("expected deleted to be false")
	}
	if resolved.DeletedAt != "" {
		t.Errorf("expected empty deletedAt, got %s", resolved.DeletedAt)
	}
	if resolved.Error != "" {
		t.Errorf("expected empty error, got %s", resolved.Error)
	}
}

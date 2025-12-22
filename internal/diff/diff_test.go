package diff

import (
	"encoding/json"
	"testing"
)

func TestDeltaTypes(t *testing.T) {
	// Test NewDelta
	delta := NewDelta("sha256:base123", "sha256:new456", "abc123")

	if delta.SchemaVersion != SchemaVersion {
		t.Errorf("expected schema version %d, got %d", SchemaVersion, delta.SchemaVersion)
	}
	if delta.BaseSnapshotID != "sha256:base123" {
		t.Errorf("expected base snapshot 'sha256:base123', got '%s'", delta.BaseSnapshotID)
	}
	if delta.Commit != "abc123" {
		t.Errorf("expected commit 'abc123', got '%s'", delta.Commit)
	}
	if delta.Timestamp <= 0 {
		t.Error("expected positive timestamp")
	}
}

func TestDeltaIsEmpty(t *testing.T) {
	delta := NewDelta("", "", "abc123")

	if !delta.IsEmpty() {
		t.Error("expected new delta to be empty")
	}

	// Add a symbol
	delta.Deltas.Symbols.Added = append(delta.Deltas.Symbols.Added, SymbolRecord{
		ID:   "test-symbol",
		Name: "Test",
		Kind: "function",
	})

	if delta.IsEmpty() {
		t.Error("expected delta with symbols to not be empty")
	}
}

func TestDeltaComputeStats(t *testing.T) {
	delta := NewDelta("", "", "abc123")

	// Add some entities
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{ID: "s1", Name: "Func1", Kind: "function"},
		{ID: "s2", Name: "Func2", Kind: "function"},
	}
	delta.Deltas.Symbols.Modified = []SymbolRecord{
		{ID: "s3", Name: "Func3", Kind: "function"},
	}
	delta.Deltas.Symbols.Deleted = []string{"s4"}

	delta.Deltas.Refs.Added = []RefRecord{
		{FromFileID: "f1", Line: 10, Column: 5, ToSymbolID: "s1"},
	}
	delta.Deltas.Refs.Deleted = []string{"f2:20:10:s2"}

	stats := delta.ComputeStats()

	if stats.SymbolsAdded != 2 {
		t.Errorf("expected 2 symbols added, got %d", stats.SymbolsAdded)
	}
	if stats.SymbolsModified != 1 {
		t.Errorf("expected 1 symbol modified, got %d", stats.SymbolsModified)
	}
	if stats.SymbolsDeleted != 1 {
		t.Errorf("expected 1 symbol deleted, got %d", stats.SymbolsDeleted)
	}
	if stats.RefsAdded != 1 {
		t.Errorf("expected 1 ref added, got %d", stats.RefsAdded)
	}
	if stats.RefsDeleted != 1 {
		t.Errorf("expected 1 ref deleted, got %d", stats.RefsDeleted)
	}
	if stats.TotalAdded != 3 { // 2 symbols + 1 ref
		t.Errorf("expected 3 total added, got %d", stats.TotalAdded)
	}
	if stats.TotalModified != 1 {
		t.Errorf("expected 1 total modified, got %d", stats.TotalModified)
	}
	if stats.TotalDeleted != 2 { // 1 symbol + 1 ref
		t.Errorf("expected 2 total deleted, got %d", stats.TotalDeleted)
	}
}

func TestDeltaValidateStats(t *testing.T) {
	delta := NewDelta("", "", "abc123")

	// Add entities
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{ID: "s1", Name: "Func1", Kind: "function"},
	}

	// Set correct stats
	delta.Stats = delta.ComputeStats()

	if !delta.ValidateStats() {
		t.Error("expected stats validation to pass")
	}

	// Corrupt stats
	delta.Stats.TotalAdded = 999

	if delta.ValidateStats() {
		t.Error("expected stats validation to fail with incorrect stats")
	}
}

func TestDeltaSerialization(t *testing.T) {
	delta := NewDelta("sha256:base", "sha256:new", "abc123")
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{ID: "s1", Name: "Func1", Kind: "function", FileID: "main.go", Line: 10},
	}
	delta.Stats = delta.ComputeStats()

	// Serialize
	data, err := delta.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize delta: %v", err)
	}

	// Deserialize
	parsed, err := ParseDelta(data)
	if err != nil {
		t.Fatalf("failed to parse delta: %v", err)
	}

	if parsed.SchemaVersion != delta.SchemaVersion {
		t.Error("schema version mismatch after round-trip")
	}
	if parsed.Commit != delta.Commit {
		t.Error("commit mismatch after round-trip")
	}
	if len(parsed.Deltas.Symbols.Added) != 1 {
		t.Error("symbols mismatch after round-trip")
	}
	if parsed.Deltas.Symbols.Added[0].ID != "s1" {
		t.Error("symbol ID mismatch after round-trip")
	}
}

func TestHasher(t *testing.T) {
	hasher := NewHasher()

	// Test symbol hashing
	s1 := &SymbolRecord{ID: "s1", Name: "Func1", Kind: "function", FileID: "main.go", Line: 10}
	s2 := &SymbolRecord{ID: "s1", Name: "Func1", Kind: "function", FileID: "main.go", Line: 10}
	s3 := &SymbolRecord{ID: "s1", Name: "Func1", Kind: "function", FileID: "main.go", Line: 11} // Different line

	hash1 := hasher.HashSymbol(s1)
	hash2 := hasher.HashSymbol(s2)
	hash3 := hasher.HashSymbol(s3)

	if hash1 != hash2 {
		t.Error("expected identical symbols to have same hash")
	}
	if hash1 == hash3 {
		t.Error("expected different symbols to have different hash")
	}

	// Test ref hashing
	r1 := &RefRecord{FromFileID: "f1", Line: 10, Column: 5, ToSymbolID: "s1"}
	r2 := &RefRecord{FromFileID: "f1", Line: 10, Column: 5, ToSymbolID: "s1"}
	r3 := &RefRecord{FromFileID: "f1", Line: 10, Column: 6, ToSymbolID: "s1"} // Different column

	refHash1 := hasher.HashRef(r1)
	refHash2 := hasher.HashRef(r2)
	refHash3 := hasher.HashRef(r3)

	if refHash1 != refHash2 {
		t.Error("expected identical refs to have same hash")
	}
	if refHash1 == refHash3 {
		t.Error("expected different refs to have different hash")
	}
}

func TestHasherVerification(t *testing.T) {
	hasher := NewHasher()

	// Symbol with correct hash
	s := &SymbolRecord{ID: "s1", Name: "Func1", Kind: "function", FileID: "main.go", Line: 10}
	s.Hash = hasher.HashSymbol(s)

	if !hasher.VerifySymbolHash(s) {
		t.Error("expected verification to pass for correct hash")
	}

	// Corrupt the hash
	s.Hash = "invalid"

	if hasher.VerifySymbolHash(s) {
		t.Error("expected verification to fail for incorrect hash")
	}

	// Empty hash should pass (not provided)
	s.Hash = ""

	if !hasher.VerifySymbolHash(s) {
		t.Error("expected verification to pass for empty hash")
	}
}

func TestCompositeKeys(t *testing.T) {
	// Test RefRecord composite key
	ref := &RefRecord{FromFileID: "main.go", Line: 10, Column: 5, ToSymbolID: "sym1"}
	key := ref.CompositeKey()

	if key != "main.go:10:5:sym1" {
		t.Errorf("unexpected ref composite key: %s", key)
	}

	// Test CallEdge composite key
	call := &CallEdge{CallerFileID: "main.go", CallLine: 20, CallColumn: 3, CalleeID: "callee1"}
	callKey := call.CompositeKey()

	if callKey != "main.go:20:3:callee1" {
		t.Errorf("unexpected call composite key: %s", callKey)
	}
}

func TestValidator(t *testing.T) {
	validator := NewValidator(
		WithValidationMode(ValidationStrict),
		WithSpotCheckPercentage(1.0), // Check all entities
	)

	// Valid delta
	delta := NewDelta("sha256:base", "", "abc123")
	delta.NewSnapshotID = "sha256:new"
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{ID: "s1", Name: "Func1", Kind: "function"},
	}
	delta.Stats = delta.ComputeStats()

	result := validator.Validate(delta, "sha256:base")

	if !result.Valid {
		t.Errorf("expected valid delta, got errors: %v", result.Errors)
	}
}

func TestValidatorVersionCheck(t *testing.T) {
	validator := NewValidator()

	// Invalid schema version
	delta := NewDelta("", "", "abc123")
	delta.SchemaVersion = 999 // Unsupported version
	delta.NewSnapshotID = "sha256:new"

	result := validator.Validate(delta, "")

	if result.Valid {
		t.Error("expected validation to fail for unsupported version")
	}

	foundVersionError := false
	for _, err := range result.Errors {
		if err.Code == ErrCodeUnsupportedVersion {
			foundVersionError = true
			break
		}
	}
	if !foundVersionError {
		t.Error("expected UNSUPPORTED_VERSION error")
	}
}

func TestValidatorSnapshotMismatch(t *testing.T) {
	validator := NewValidator()

	delta := NewDelta("sha256:base123", "", "abc123")
	delta.NewSnapshotID = "sha256:new"
	delta.Stats = delta.ComputeStats()

	// Current snapshot doesn't match base
	result := validator.Validate(delta, "sha256:different")

	if result.Valid {
		t.Error("expected validation to fail for snapshot mismatch")
	}

	foundMismatchError := false
	for _, err := range result.Errors {
		if err.Code == ErrCodeSnapshotMismatch {
			foundMismatchError = true
			break
		}
	}
	if !foundMismatchError {
		t.Error("expected SNAPSHOT_MISMATCH error")
	}
}

func TestValidatorStatsMismatch(t *testing.T) {
	validator := NewValidator()

	delta := NewDelta("", "", "abc123")
	delta.NewSnapshotID = "sha256:new"
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{ID: "s1", Name: "Func1", Kind: "function"},
	}
	// Intentionally wrong stats
	delta.Stats.TotalAdded = 999

	result := validator.Validate(delta, "")

	if result.Valid {
		t.Error("expected validation to fail for stats mismatch")
	}

	foundStatsError := false
	for _, err := range result.Errors {
		if err.Code == ErrCodeStatsMismatch {
			foundStatsError = true
			break
		}
	}
	if !foundStatsError {
		t.Error("expected STATS_MISMATCH error")
	}
}

func TestValidatorMissingFields(t *testing.T) {
	validator := NewValidator()

	delta := NewDelta("", "", "")
	delta.NewSnapshotID = "" // Missing required field
	delta.Commit = ""        // Missing required field

	result := validator.Validate(delta, "")

	if result.Valid {
		t.Error("expected validation to fail for missing fields")
	}

	foundMissingError := false
	for _, err := range result.Errors {
		if err.Code == ErrCodeMissingField {
			foundMissingError = true
			break
		}
	}
	if !foundMissingError {
		t.Error("expected MISSING_FIELD error")
	}
}

func TestQuickValidate(t *testing.T) {
	validator := NewValidator()

	// Valid delta
	delta := NewDelta("", "", "abc123")
	delta.NewSnapshotID = "sha256:new"
	delta.Stats = delta.ComputeStats()

	err := validator.QuickValidate(delta)
	if err != nil {
		t.Errorf("expected quick validate to pass, got: %v", err)
	}

	// Invalid schema version
	delta.SchemaVersion = 0
	err = validator.QuickValidate(delta)
	if err == nil {
		t.Error("expected quick validate to fail for invalid version")
	}
}

func TestDeltaJSONSchema(t *testing.T) {
	// Test that the JSON output matches expected schema
	delta := NewDelta("sha256:base", "", "abc123def")
	delta.NewSnapshotID = "sha256:new"
	delta.Deltas.Symbols.Added = []SymbolRecord{
		{
			ID:            "scip-go gomod example `example`/Func().",
			Name:          "Func",
			Kind:          "function",
			FileID:        "main.go",
			Line:          10,
			Column:        6,
			Language:      "go",
			Signature:     "func Func() error",
			Documentation: "Func does something",
		},
	}
	delta.Deltas.Refs.Added = []RefRecord{
		{
			FromFileID: "main.go",
			Line:       20,
			Column:     5,
			ToSymbolID: "scip-go gomod example `example`/Func().",
			Kind:       "reference",
			Language:   "go",
		},
	}
	delta.Stats = delta.ComputeStats()

	data, err := delta.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check required fields exist
	if _, ok := parsed["delta_schema_version"]; !ok {
		t.Error("missing delta_schema_version")
	}
	if _, ok := parsed["deltas"]; !ok {
		t.Error("missing deltas")
	}
	if _, ok := parsed["stats"]; !ok {
		t.Error("missing stats")
	}
}

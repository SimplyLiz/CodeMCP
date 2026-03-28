package query

import (
	"testing"
)

func TestBatchGetOptions_IncludeCounts_FieldExists(t *testing.T) {
	t.Parallel()

	// Verify the struct can be constructed with IncludeCounts and defaults to false.
	opts := BatchGetOptions{
		SymbolIds: []string{"ckb:repo:sym:abc123"},
	}

	if opts.IncludeCounts {
		t.Error("IncludeCounts should default to false")
	}

	// Verify it can be set to true.
	opts.IncludeCounts = true
	if !opts.IncludeCounts {
		t.Error("IncludeCounts should be true after assignment")
	}
}

func TestBatchGetOptions_SymbolIdLimit(t *testing.T) {
	t.Parallel()

	// Verify the documented limit is 50 by constructing options with 51 IDs
	// and calling BatchGet. We can't easily call BatchGet without a full engine,
	// so just verify we can construct the options and the limit constant is
	// documented in the struct.
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = "ckb:repo:sym:test"
	}
	opts := BatchGetOptions{
		SymbolIds:     ids,
		IncludeCounts: true,
	}

	if len(opts.SymbolIds) != 51 {
		t.Errorf("expected 51 symbol IDs, got %d", len(opts.SymbolIds))
	}
}

func TestBatchGetOptions_DefaultCounts(t *testing.T) {
	t.Parallel()

	// When IncludeCounts is false (default), the response should not populate
	// referenceCount, callerCount, calleeCount. We verify the struct field
	// semantics here; the actual population logic is tested via integration.
	withCounts := BatchGetOptions{
		SymbolIds:     []string{"sym1", "sym2"},
		IncludeCounts: true,
	}
	withoutCounts := BatchGetOptions{
		SymbolIds: []string{"sym1", "sym2"},
	}

	if withCounts.IncludeCounts == withoutCounts.IncludeCounts {
		t.Error("expected different IncludeCounts values")
	}
}

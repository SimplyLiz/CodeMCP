package perf

// Portable tests — no build tag, compiles in both CGO and non-CGO environments.
// Only tests types and behaviors that exist in both builds.

import "testing"

func TestStructuralPerfOptions_ZeroValue(t *testing.T) {
	var opts StructuralPerfOptions
	if opts.Limit != 0 || opts.WindowDays != 0 || opts.MinChurnCount != 0 {
		t.Error("StructuralPerfOptions zero value should have all-zero fields")
	}
	if opts.Scope != nil || opts.EntrypointFiles != nil {
		t.Error("StructuralPerfOptions zero value should have nil slices")
	}
}

func TestLoopCallSite_ZeroValue(t *testing.T) {
	var cs LoopCallSite
	if cs.NearEntrypoint {
		t.Error("NearEntrypoint default should be false")
	}
	if cs.Line != 0 {
		t.Error("Line default should be 0")
	}
}

func TestStructuralPerfSummary_ZeroValue(t *testing.T) {
	var s StructuralPerfSummary
	if s.FilesScanned != 0 || s.HotFilesFound != 0 || s.CallSitesFound != 0 {
		t.Error("StructuralPerfSummary zero value should have all-zero counts")
	}
}

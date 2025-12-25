package docs

import (
	"testing"
)

func TestNewCoverageAnalyzer(t *testing.T) {
	// NewCoverageAnalyzer requires a store and symbolIndex which need
	// more complex setup. Test the constructor with nil values to verify
	// it doesn't panic.
	analyzer := NewCoverageAnalyzer(nil, nil)
	if analyzer == nil {
		t.Fatal("NewCoverageAnalyzer returned nil")
	}
}

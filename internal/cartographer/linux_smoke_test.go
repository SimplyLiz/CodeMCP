//go:build cartographer

package cartographer

import (
	"os"
	"testing"
)

// Regression: the tree-sitter walkers recurse by AST depth, so a very large repo
// with deeply nested files (the Linux kernel's macro-generated C) used to overflow
// a rayon worker's stack and SIGABRT the whole process — including this host when
// linked via FFI. build_mapped_files now runs the parse on a large-stack pool.
// This exercises the FFI at real scale; a regression aborts the test binary.
// Skips unless /tmp/linux exists (clone it with: git clone --depth 1 <kernel> /tmp/linux).
func TestFFIDoesNotOverflowOnLinuxKernel(t *testing.T) {
	const root = "/tmp/linux"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("skip: %s not present", root)
	}
	rep, err := Health(root)
	if err != nil {
		t.Fatalf("Health(%s) errored: %v", root, err)
	}
	if rep == nil {
		t.Fatalf("Health(%s) returned nil report", root)
	}
	t.Logf("Linux kernel via FFI: no crash, health report returned")
}

package incremental

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ckb/internal/project"
	"ckb/internal/storage"
)

// getFixtureRoot returns the path to the testdata/incremental directory.
func getFixtureRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "incremental")
}

// isScipGoInstalled checks if scip-go is available.
func isScipGoInstalled() bool {
	// Check standard PATH
	if _, err := exec.LookPath("scip-go"); err == nil {
		return true
	}
	// Check ~/go/bin which is common for go install
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	scipGoPath := filepath.Join(home, "go", "bin", "scip-go")
	_, err = os.Stat(scipGoPath)
	return err == nil
}

// getScipGoPath returns the path to scip-go executable.
func getScipGoPath() string {
	if path, err := exec.LookPath("scip-go"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "scip-go"
	}
	return filepath.Join(home, "go", "bin", "scip-go")
}

// TestExtractDeltas_GoFixture tests ExtractDeltas using the pre-generated SCIP index.
// This test always runs because it uses a committed SCIP index fixture.
func TestExtractDeltas_GoFixture(t *testing.T) {
	fixtureRoot := getFixtureRoot(t)
	goFixture := filepath.Join(fixtureRoot, "go")
	indexPath := filepath.Join(goFixture, ".scip", "index.scip")

	// Verify fixture exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatalf("SCIP index fixture not found at %s - run 'scip-go --output .scip/index.scip' in testdata/incremental/go", indexPath)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	extractor := NewSCIPExtractor(goFixture, indexPath, logger)

	// Simulate changes to both files
	changes := []ChangedFile{
		{Path: "main.go", ChangeType: ChangeModified},
		{Path: "utils.go", ChangeType: ChangeModified},
	}

	delta, err := extractor.ExtractDeltas(changes)
	if err != nil {
		t.Fatalf("ExtractDeltas failed: %v", err)
	}

	// Verify we got file deltas
	if len(delta.FileDeltas) != 2 {
		t.Errorf("expected 2 file deltas, got %d", len(delta.FileDeltas))
	}

	// Check that we extracted symbols
	var mainSymbols, utilsSymbols []Symbol
	for _, fd := range delta.FileDeltas {
		switch fd.Path {
		case "main.go":
			mainSymbols = fd.Symbols
		case "utils.go":
			utilsSymbols = fd.Symbols
		}
	}

	// Verify expected symbols from main.go
	expectedMainSymbols := []string{"Message", "User", "Greet", "NewUser", "main"}
	verifySymbolsExist(t, "main.go", mainSymbols, expectedMainSymbols)

	// Verify expected symbols from utils.go
	expectedUtilsSymbols := []string{"Add", "Subtract", "formatGreeting", "ValidateName"}
	verifySymbolsExist(t, "utils.go", utilsSymbols, expectedUtilsSymbols)

	// Verify we have references (cross-file calls)
	var mainRefs []Reference
	for _, fd := range delta.FileDeltas {
		if fd.Path == "main.go" {
			mainRefs = fd.Refs
			break
		}
	}

	// main.go should have references to Add and formatGreeting from utils.go
	hasAddRef := false
	hasFormatGreetingRef := false
	for _, ref := range mainRefs {
		if strings.Contains(ref.ToSymbolID, "Add") {
			hasAddRef = true
		}
		if strings.Contains(ref.ToSymbolID, "formatGreeting") {
			hasFormatGreetingRef = true
		}
	}

	if !hasAddRef {
		t.Error("expected reference to Add function in main.go")
	}
	if !hasFormatGreetingRef {
		t.Error("expected reference to formatGreeting function in main.go")
	}

	// Verify call edges
	var mainCallEdges []CallEdge
	for _, fd := range delta.FileDeltas {
		if fd.Path == "main.go" {
			mainCallEdges = fd.CallEdges
			break
		}
	}

	if len(mainCallEdges) == 0 {
		t.Error("expected call edges in main.go")
	}

	// Verify stats
	if delta.Stats.FilesChanged != 2 {
		t.Errorf("expected FilesChanged=2, got %d", delta.Stats.FilesChanged)
	}
	if delta.Stats.SymbolsAdded == 0 {
		t.Error("expected SymbolsAdded > 0")
	}
	if delta.Stats.RefsAdded == 0 {
		t.Error("expected RefsAdded > 0")
	}
}

// TestExtractDeltas_DeletedFile tests handling of deleted files.
func TestExtractDeltas_DeletedFile(t *testing.T) {
	fixtureRoot := getFixtureRoot(t)
	goFixture := filepath.Join(fixtureRoot, "go")
	indexPath := filepath.Join(goFixture, ".scip", "index.scip")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Skip("SCIP index fixture not found")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	extractor := NewSCIPExtractor(goFixture, indexPath, logger)

	// Simulate a deleted file
	changes := []ChangedFile{
		{Path: "deleted.go", ChangeType: ChangeDeleted},
	}

	delta, err := extractor.ExtractDeltas(changes)
	if err != nil {
		t.Fatalf("ExtractDeltas failed: %v", err)
	}

	// Should have one file delta for the deleted file
	if len(delta.FileDeltas) != 1 {
		t.Errorf("expected 1 file delta for deleted file, got %d", len(delta.FileDeltas))
	}

	if delta.FileDeltas[0].ChangeType != ChangeDeleted {
		t.Errorf("expected ChangeDeleted, got %v", delta.FileDeltas[0].ChangeType)
	}

	if delta.Stats.FilesDeleted != 1 {
		t.Errorf("expected FilesDeleted=1, got %d", delta.Stats.FilesDeleted)
	}
}

// TestExtractDeltas_AddedFile tests handling of newly added files.
func TestExtractDeltas_AddedFile(t *testing.T) {
	fixtureRoot := getFixtureRoot(t)
	goFixture := filepath.Join(fixtureRoot, "go")
	indexPath := filepath.Join(goFixture, ".scip", "index.scip")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Skip("SCIP index fixture not found")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	extractor := NewSCIPExtractor(goFixture, indexPath, logger)

	// Simulate adding main.go (which exists in the index)
	changes := []ChangedFile{
		{Path: "main.go", ChangeType: ChangeAdded},
	}

	delta, err := extractor.ExtractDeltas(changes)
	if err != nil {
		t.Fatalf("ExtractDeltas failed: %v", err)
	}

	if delta.Stats.FilesAdded != 1 {
		t.Errorf("expected FilesAdded=1, got %d", delta.Stats.FilesAdded)
	}

	// Should extract symbols from the added file
	if len(delta.FileDeltas) != 1 {
		t.Errorf("expected 1 file delta, got %d", len(delta.FileDeltas))
	}

	if len(delta.FileDeltas[0].Symbols) == 0 {
		t.Error("expected symbols in added file")
	}
}

// TestLiveIncrementalIndex_Go tests the full incremental workflow with a real scip-go execution.
// This test is skipped if scip-go is not installed or if running in a temp directory
// (scip-go has issues with git repos in temp directories).
func TestLiveIncrementalIndex_Go(t *testing.T) {
	if !isScipGoInstalled() {
		t.Skip("scip-go not installed, skipping live integration test")
	}
	// Note: scip-go has a known issue where it fails with "project root is outside the repository"
	// when running in temp directories with git initialized. This test uses a workaround by
	// running without git for the initial index, then adding git for change detection.
	// TODO: Fix this when scip-go is updated or find a better workaround.

	// Create a temporary directory for this test
	tmpDir, err := os.MkdirTemp("", "incremental-live-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy the Go fixture to temp dir
	fixtureRoot := getFixtureRoot(t)
	goFixture := filepath.Join(fixtureRoot, "go")

	// Copy files (excluding .scip directory - we want to generate fresh)
	for _, file := range []string{"go.mod", "main.go", "utils.go"} {
		src := filepath.Join(goFixture, file)
		dst := filepath.Join(tmpDir, file)
		content, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("failed to read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, content, 0644); err != nil {
			t.Fatalf("failed to write %s: %v", dst, err)
		}
	}

	// WORKAROUND: Run scip-go BEFORE initializing git to avoid the
	// "project root is outside the repository" error in temp directories
	scipDir := filepath.Join(tmpDir, ".scip")
	if err := os.MkdirAll(scipDir, 0755); err != nil {
		t.Fatalf("failed to create .scip dir: %v", err)
	}

	cmd := exec.Command(getScipGoPath(), "--output", filepath.Join(scipDir, "index.scip"))
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run scip-go: %v\nOutput: %s", err, string(output))
	}

	// Now initialize git repo for change detection
	setupGitRepoForIntegration(t, tmpDir)

	// Create .ckb directory and database
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatalf("failed to create .ckb dir: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Create indexer
	config := DefaultConfig()
	indexer := NewIncrementalIndexer(tmpDir, db, config, logger)

	// Verify we can use incremental for Go
	canUse, reason := indexer.CanUseIncremental(project.LangGo)
	if !canUse {
		t.Fatalf("expected CanUseIncremental=true for Go, got false: %s", reason)
	}

	// Populate tracking from full index
	if err := indexer.PopulateAfterFullIndex(); err != nil {
		t.Fatalf("PopulateAfterFullIndex failed: %v", err)
	}

	// Test that we can query the index state
	state := indexer.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full' after PopulateAfterFullIndex, got %q", state.State)
	}

	// Test NeedsFullReindex returns false after full index
	needs, reason := indexer.NeedsFullReindex()
	if needs {
		t.Errorf("expected NeedsFullReindex=false after full index, got true: %s", reason)
	}

	// Note: We can't test the full incremental workflow with changes here because
	// scip-go fails when git is initialized in temp directories. The NoChanges test
	// verifies the "no changes" path works correctly.
	t.Log("Live test completed - scip-go ran successfully, PopulateAfterFullIndex worked")
}

// TestLiveIncrementalIndex_NoChanges tests incremental with no changes.
func TestLiveIncrementalIndex_NoChanges(t *testing.T) {
	if !isScipGoInstalled() {
		t.Skip("scip-go not installed, skipping live integration test")
	}

	tmpDir, err := os.MkdirTemp("", "incremental-nochange-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple Go file
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// WORKAROUND: Run scip-go BEFORE initializing git to avoid the
	// "project root is outside the repository" error in temp directories
	scipDir := filepath.Join(tmpDir, ".scip")
	if err := os.MkdirAll(scipDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(getScipGoPath(), "--output", filepath.Join(scipDir, "index.scip"))
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scip-go failed: %v\nOutput: %s", err, string(output))
	}

	// Now initialize git and commit
	setupGitRepoForIntegration(t, tmpDir)

	// Create .ckb and database
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create indexer and populate
	config := DefaultConfig()
	indexer := NewIncrementalIndexer(tmpDir, db, config, logger)
	if err := indexer.PopulateAfterFullIndex(); err != nil {
		t.Fatal(err)
	}

	// Run incremental with no changes
	ctx := context.Background()
	stats, err := indexer.IndexIncrementalWithLang(ctx, "", project.LangGo)
	if err != nil {
		t.Fatalf("IndexIncrementalWithLang failed: %v", err)
	}

	// Should report unchanged
	if stats.IndexState != "unchanged" {
		t.Errorf("expected IndexState='unchanged', got %q", stats.IndexState)
	}
}

// Helper functions

func verifySymbolsExist(t *testing.T, file string, symbols []Symbol, expectedNames []string) {
	t.Helper()

	for _, expected := range expectedNames {
		found := false
		for _, sym := range symbols {
			// Check if the symbol name contains the expected name
			// SCIP symbol IDs look like:
			// - com/testfixture`/Message (constant)
			// - com/testfixture`/User# (struct type - ends with #)
			// - com/testfixture`/User#Greet (method)
			// - com/testfixture`/NewUser (function)
			if strings.Contains(sym.Name, "/"+expected) ||
				strings.Contains(sym.Name, "#"+expected) ||
				strings.Contains(sym.Name, "`"+expected) ||
				sym.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: expected symbol containing %q not found. Got: %v", file, expected, getSymbolNames(symbols))
		}
	}
}

func getSymbolNames(symbols []Symbol) []string {
	names := make([]string, len(symbols))
	for i, sym := range symbols {
		names[i] = sym.Name
	}
	return names
}

func setupGitRepoForIntegration(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git command %v failed: %v", args, err)
		}
	}
}

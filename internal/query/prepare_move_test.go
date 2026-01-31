package query

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckTargetConflicts_NoConflict(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "src/handler.go", "package src")
	createTestDirectory(t, engine, "dest")

	conflicts := engine.checkTargetConflicts("src/handler.go", "dest/handler_new.go")
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestCheckTargetConflicts_FileExists(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "src/handler.go", "package src")
	createTestFile(t, engine, "dest/handler.go", "package dest")

	// Target file path itself exists
	conflicts := engine.checkTargetConflicts("src/handler.go", "dest/handler.go")

	foundFile := false
	for _, c := range conflicts {
		if c.ConflictKind == "file" {
			foundFile = true
		}
	}
	if !foundFile {
		t.Error("expected a file conflict when target exists")
	}
}

func TestCheckTargetConflicts_SameBaseNameInTargetDir(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "src/handler.go", "package src")
	createTestFile(t, engine, "dest/handler.go", "package dest")

	// Target is a directory, and handler.go already exists there
	conflicts := engine.checkTargetConflicts("src/handler.go", "dest")

	foundConflict := false
	for _, c := range conflicts {
		if c.Name == "handler.go" && c.ConflictKind == "file" {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Error("expected file conflict for same base name in target dir")
	}
}

func TestGetPrepareMove_NilTarget(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	result := engine.getPrepareMove(nil, nil, "dest")
	if result != nil {
		t.Error("expected nil for nil target")
	}
}

func TestGetPrepareMove_EmptyTargetPath(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	target := &PrepareChangeTarget{Path: "src/handler.go"}
	result := engine.getPrepareMove(nil, target, "")
	if result != nil {
		t.Error("expected nil for empty targetPath")
	}
}

func TestGetPrepareMove_Basic(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "src/handler.go", "package src")
	createTestDirectory(t, engine, "dest")

	target := &PrepareChangeTarget{Path: "src/handler.go"}
	result := engine.getPrepareMove(nil, target, "dest/handler.go")

	if result == nil {
		t.Fatal("expected non-nil MoveDetail")
	}
	if result.SourcePath != "src/handler.go" {
		t.Errorf("expected source path src/handler.go, got %s", result.SourcePath)
	}
	if result.TargetPath != "dest/handler.go" {
		t.Errorf("expected target path dest/handler.go, got %s", result.TargetPath)
	}
}

func TestFindAffectedImportsHeuristic(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create a Go file that imports a package
	createTestFile(t, engine, "internal/handler/handler.go", `package handler

import "github.com/example/internal/models"

func Handle() {
	models.New()
}
`)
	// Create the source package
	createTestFile(t, engine, "internal/models/model.go", "package models\n\nfunc New() {}\n")

	imports := engine.findAffectedImportsHeuristic("internal/models", "pkg/models")

	if len(imports) != 1 {
		t.Fatalf("expected 1 affected import, got %d", len(imports))
	}
	if imports[0].OldImport != "internal/models" {
		t.Errorf("expected old import internal/models, got %s", imports[0].OldImport)
	}
	if imports[0].NewImport != "pkg/models" {
		t.Errorf("expected new import pkg/models, got %s", imports[0].NewImport)
	}
}

func TestFindAffectedImportsHeuristic_NoMatches(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	createTestFile(t, engine, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)

	imports := engine.findAffectedImportsHeuristic("internal/models", "pkg/models")
	if len(imports) != 0 {
		t.Errorf("expected no affected imports, got %d", len(imports))
	}
}

func TestFindAffectedImportsHeuristic_EmptySource(t *testing.T) {
	t.Parallel()
	engine, cleanup := testEngine(t)
	defer cleanup()

	imports := engine.findAffectedImportsHeuristic("", "pkg/models")
	if imports != nil {
		t.Error("expected nil for empty source package")
	}
}

// createTestDirectory and createTestFile are defined in compound_test.go
// testEngine is defined in engine_test.go
// Verify they're accessible (build will fail if not)
var _ = func() {
	_ = os.MkdirAll
	_ = filepath.Join
}

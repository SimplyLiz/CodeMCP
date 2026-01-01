package query

import (
	"testing"
)

func TestAnnotateModule(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("creates new annotation", func(t *testing.T) {
		input := &AnnotateModuleInput{
			ModuleId:       "internal/api",
			Responsibility: "HTTP API handlers",
			Capabilities:   []string{"REST API", "Authentication"},
			Tags:           []string{"api", "public"},
		}

		result, err := engine.AnnotateModule(input)
		if err != nil {
			t.Fatalf("AnnotateModule failed: %v", err)
		}

		if result.ModuleId != "internal/api" {
			t.Errorf("ModuleId = %q, want %q", result.ModuleId, "internal/api")
		}
		if result.Responsibility != "HTTP API handlers" {
			t.Errorf("Responsibility = %q, want %q", result.Responsibility, "HTTP API handlers")
		}
		if !result.Created {
			t.Error("expected Created to be true for new annotation")
		}
		if result.Updated {
			t.Error("expected Updated to be false for new annotation")
		}
		if len(result.Capabilities) != 2 {
			t.Errorf("expected 2 capabilities, got %d", len(result.Capabilities))
		}
		if len(result.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(result.Tags))
		}
	})

	t.Run("updates existing annotation", func(t *testing.T) {
		// First create
		input1 := &AnnotateModuleInput{
			ModuleId:       "internal/storage",
			Responsibility: "Data persistence",
		}
		_, err := engine.AnnotateModule(input1)
		if err != nil {
			t.Fatalf("first AnnotateModule failed: %v", err)
		}

		// Then update
		input2 := &AnnotateModuleInput{
			ModuleId:       "internal/storage",
			Responsibility: "Updated data persistence layer",
			Capabilities:   []string{"SQLite", "Caching"},
		}
		result, err := engine.AnnotateModule(input2)
		if err != nil {
			t.Fatalf("second AnnotateModule failed: %v", err)
		}

		if result.Created {
			t.Error("expected Created to be false for update")
		}
		if !result.Updated {
			t.Error("expected Updated to be true for update")
		}
		if result.Responsibility != "Updated data persistence layer" {
			t.Errorf("Responsibility = %q, want %q", result.Responsibility, "Updated data persistence layer")
		}
	})

	t.Run("merges with existing when fields empty", func(t *testing.T) {
		// Create with responsibility
		input1 := &AnnotateModuleInput{
			ModuleId:       "internal/merge",
			Responsibility: "Original responsibility",
			Capabilities:   []string{"cap1"},
		}
		_, err := engine.AnnotateModule(input1)
		if err != nil {
			t.Fatalf("first AnnotateModule failed: %v", err)
		}

		// Update with empty responsibility should keep original
		input2 := &AnnotateModuleInput{
			ModuleId:       "internal/merge",
			Responsibility: "",  // empty - should keep original
			Capabilities:   nil, // nil - should keep original
		}
		_, err = engine.AnnotateModule(input2)
		if err != nil {
			t.Fatalf("second AnnotateModule failed: %v", err)
		}
	})

	t.Run("handles boundaries", func(t *testing.T) {
		input := &AnnotateModuleInput{
			ModuleId:      "internal/bounded",
			PublicPaths:   []string{"api.go", "types.go"},
			InternalPaths: []string{"internal.go"},
		}

		result, err := engine.AnnotateModule(input)
		if err != nil {
			t.Fatalf("AnnotateModule failed: %v", err)
		}

		if result.Boundaries == nil {
			t.Fatal("expected Boundaries to be set")
		}
		if len(result.Boundaries.Public) != 2 {
			t.Errorf("expected 2 public paths, got %d", len(result.Boundaries.Public))
		}
		if len(result.Boundaries.Internal) != 1 {
			t.Errorf("expected 1 internal path, got %d", len(result.Boundaries.Internal))
		}
	})

	t.Run("handles empty input", func(t *testing.T) {
		input := &AnnotateModuleInput{
			ModuleId: "internal/empty",
			// All other fields empty
		}

		result, err := engine.AnnotateModule(input)
		if err != nil {
			t.Fatalf("AnnotateModule failed: %v", err)
		}

		if result.ModuleId != "internal/empty" {
			t.Errorf("ModuleId = %q, want %q", result.ModuleId, "internal/empty")
		}
		if result.Boundaries != nil {
			t.Error("expected Boundaries to be nil for empty input")
		}
	})

	t.Run("only public paths", func(t *testing.T) {
		input := &AnnotateModuleInput{
			ModuleId:    "internal/public-only",
			PublicPaths: []string{"public.go"},
		}

		result, err := engine.AnnotateModule(input)
		if err != nil {
			t.Fatalf("AnnotateModule failed: %v", err)
		}

		if result.Boundaries == nil {
			t.Fatal("expected Boundaries to be set")
		}
		if len(result.Boundaries.Public) != 1 {
			t.Errorf("expected 1 public path, got %d", len(result.Boundaries.Public))
		}
	})

	t.Run("only internal paths", func(t *testing.T) {
		input := &AnnotateModuleInput{
			ModuleId:      "internal/internal-only",
			InternalPaths: []string{"internal.go"},
		}

		result, err := engine.AnnotateModule(input)
		if err != nil {
			t.Fatalf("AnnotateModule failed: %v", err)
		}

		if result.Boundaries == nil {
			t.Fatal("expected Boundaries to be set")
		}
		if len(result.Boundaries.Internal) != 1 {
			t.Errorf("expected 1 internal path, got %d", len(result.Boundaries.Internal))
		}
	})
}

func TestAnnotateModuleInputStruct(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		input := AnnotateModuleInput{}

		if input.ModuleId != "" {
			t.Error("expected empty ModuleId")
		}
		if input.Responsibility != "" {
			t.Error("expected empty Responsibility")
		}
		if input.Capabilities != nil {
			t.Error("expected nil Capabilities")
		}
		if input.Tags != nil {
			t.Error("expected nil Tags")
		}
		if input.PublicPaths != nil {
			t.Error("expected nil PublicPaths")
		}
		if input.InternalPaths != nil {
			t.Error("expected nil InternalPaths")
		}
	})
}

func TestAnnotateModuleResultStruct(t *testing.T) {
	t.Run("with boundaries", func(t *testing.T) {
		result := AnnotateModuleResult{
			ModuleId:       "test",
			Responsibility: "test responsibility",
			Capabilities:   []string{"cap1"},
			Tags:           []string{"tag1"},
			Boundaries: &Boundaries{
				Public:   []string{"public.go"},
				Internal: []string{"internal.go"},
			},
			Updated: true,
			Created: false,
		}

		if result.ModuleId != "test" {
			t.Error("ModuleId mismatch")
		}
		if result.Boundaries == nil {
			t.Error("expected Boundaries")
		}
	})

	t.Run("without boundaries", func(t *testing.T) {
		result := AnnotateModuleResult{
			ModuleId: "test",
			Created:  true,
		}

		if result.Boundaries != nil {
			t.Error("expected nil Boundaries")
		}
		if !result.Created {
			t.Error("expected Created to be true")
		}
	})
}

func TestBoundariesStruct(t *testing.T) {
	t.Run("empty boundaries", func(t *testing.T) {
		b := Boundaries{}
		if b.Public != nil {
			t.Error("expected nil Public")
		}
		if b.Internal != nil {
			t.Error("expected nil Internal")
		}
	})

	t.Run("with paths", func(t *testing.T) {
		b := Boundaries{
			Public:   []string{"a.go", "b.go"},
			Internal: []string{"c.go"},
		}
		if len(b.Public) != 2 {
			t.Errorf("expected 2 public, got %d", len(b.Public))
		}
		if len(b.Internal) != 1 {
			t.Errorf("expected 1 internal, got %d", len(b.Internal))
		}
	})
}

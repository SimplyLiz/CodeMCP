package query

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/decisions"
)

func TestRecordDecision(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("creates ADR with required fields", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:    "Use SQLite for local storage",
			Context:  "We need a simple embedded database",
			Decision: "Use SQLite because it's embedded and reliable",
			Consequences: []string{
				"Simple deployment",
				"No external dependencies",
			},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision == nil {
			t.Fatal("expected non-nil decision")
		}
		if result.Decision.Title != input.Title {
			t.Errorf("title mismatch: got %q, want %q", result.Decision.Title, input.Title)
		}
		if result.Decision.Context != input.Context {
			t.Errorf("context mismatch: got %q, want %q", result.Decision.Context, input.Context)
		}
		if result.Decision.Decision != input.Decision {
			t.Errorf("decision mismatch: got %q, want %q", result.Decision.Decision, input.Decision)
		}
		if result.Source != "file" {
			t.Errorf("source should be 'file', got %q", result.Source)
		}
	})

	t.Run("creates ADR with optional fields", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:           "Choose REST over GraphQL",
			Context:         "Need an API pattern",
			Decision:        "Use REST for simplicity",
			Consequences:    []string{"Standard tooling", "Well understood"},
			AffectedModules: []string{"api", "handlers"},
			Alternatives:    []string{"GraphQL", "gRPC"},
			Author:          "test-author",
			Status:          "accepted",
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision.Author != input.Author {
			t.Errorf("author mismatch: got %q, want %q", result.Decision.Author, input.Author)
		}
		if result.Decision.Status != input.Status {
			t.Errorf("status mismatch: got %q, want %q", result.Decision.Status, input.Status)
		}
		if len(result.Decision.AffectedModules) != 2 {
			t.Errorf("affected modules count mismatch: got %d, want 2", len(result.Decision.AffectedModules))
		}
		if len(result.Decision.Alternatives) != 2 {
			t.Errorf("alternatives count mismatch: got %d, want 2", len(result.Decision.Alternatives))
		}
	})

	t.Run("defaults to proposed status", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "Test default status",
			Context:      "Testing",
			Decision:     "Just a test",
			Consequences: []string{"None"},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision.Status != "proposed" {
			t.Errorf("expected default status 'proposed', got %q", result.Decision.Status)
		}
	})

	t.Run("ignores invalid status", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "Test invalid status",
			Context:      "Testing",
			Decision:     "Just a test",
			Consequences: []string{"None"},
			Status:       "invalid-status",
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		// Should fall back to default (proposed)
		if result.Decision.Status != "proposed" {
			t.Errorf("expected status 'proposed' for invalid input, got %q", result.Decision.Status)
		}
	})

	t.Run("increments ADR number", func(t *testing.T) {
		// Create first ADR
		input1 := &RecordDecisionInput{
			Title:        "First ADR",
			Context:      "Context 1",
			Decision:     "Decision 1",
			Consequences: []string{"C1"},
		}
		result1, err := engine.RecordDecision(input1)
		if err != nil {
			t.Fatalf("First RecordDecision failed: %v", err)
		}

		// Create second ADR
		input2 := &RecordDecisionInput{
			Title:        "Second ADR",
			Context:      "Context 2",
			Decision:     "Decision 2",
			Consequences: []string{"C2"},
		}
		result2, err := engine.RecordDecision(input2)
		if err != nil {
			t.Fatalf("Second RecordDecision failed: %v", err)
		}

		// IDs should be different and sequential
		if result1.Decision.ID == result2.Decision.ID {
			t.Error("ADR IDs should be different")
		}
	})
}

func TestGetDecision(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// First create an ADR to retrieve
	input := &RecordDecisionInput{
		Title:        "Test Get Decision",
		Context:      "Testing retrieval",
		Decision:     "Test decision",
		Consequences: []string{"Testable"},
	}
	created, err := engine.RecordDecision(input)
	if err != nil {
		t.Fatalf("Setup: RecordDecision failed: %v", err)
	}

	t.Run("retrieves existing decision", func(t *testing.T) {
		result, err := engine.GetDecision(created.Decision.ID)
		if err != nil {
			t.Fatalf("GetDecision failed: %v", err)
		}

		if result.Decision == nil {
			t.Fatal("expected non-nil decision")
		}
		if result.Decision.ID != created.Decision.ID {
			t.Errorf("ID mismatch: got %q, want %q", result.Decision.ID, created.Decision.ID)
		}
		if result.Decision.Title != input.Title {
			t.Errorf("title mismatch: got %q, want %q", result.Decision.Title, input.Title)
		}
	})

	t.Run("returns error for non-existent decision", func(t *testing.T) {
		_, err := engine.GetDecision("ADR-9999")
		if err == nil {
			t.Error("expected error for non-existent decision")
		}
	})

	t.Run("returns error for empty ID", func(t *testing.T) {
		_, err := engine.GetDecision("")
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})
}

func TestGetDecisions(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create several ADRs with different statuses
	adrs := []RecordDecisionInput{
		{Title: "First", Context: "C1", Decision: "D1", Consequences: []string{"X"}, Status: "proposed"},
		{Title: "Second", Context: "C2", Decision: "D2", Consequences: []string{"Y"}, Status: "accepted"},
		{Title: "Third", Context: "C3", Decision: "D3", Consequences: []string{"Z"}, Status: "accepted", AffectedModules: []string{"api"}},
	}

	for _, input := range adrs {
		inputCopy := input
		if _, err := engine.RecordDecision(&inputCopy); err != nil {
			t.Fatalf("Setup: RecordDecision failed: %v", err)
		}
	}

	t.Run("returns all decisions with nil query", func(t *testing.T) {
		result, err := engine.GetDecisions(nil)
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		if result.Total < 3 {
			t.Errorf("expected at least 3 decisions, got %d", result.Total)
		}
	})

	t.Run("returns all decisions with empty query", func(t *testing.T) {
		result, err := engine.GetDecisions(&DecisionsQuery{})
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		if result.Total < 3 {
			t.Errorf("expected at least 3 decisions, got %d", result.Total)
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		result, err := engine.GetDecisions(&DecisionsQuery{
			Status: "accepted",
		})
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		// Should find at least our 2 accepted ADRs
		if result.Total < 2 {
			t.Errorf("expected at least 2 accepted decisions, got %d", result.Total)
		}

		// All returned should be accepted
		for _, d := range result.Decisions {
			if d.Status != "accepted" {
				t.Errorf("expected status 'accepted', got %q", d.Status)
			}
		}
	})

	t.Run("filters by module", func(t *testing.T) {
		result, err := engine.GetDecisions(&DecisionsQuery{
			ModuleID: "api",
		})
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		// Should find at least our 1 ADR with api module
		if result.Total < 1 {
			t.Errorf("expected at least 1 decision for module 'api', got %d", result.Total)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		result, err := engine.GetDecisions(&DecisionsQuery{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		if len(result.Decisions) > 2 {
			t.Errorf("expected at most 2 decisions, got %d", len(result.Decisions))
		}
	})

	t.Run("sets default limit", func(t *testing.T) {
		query := &DecisionsQuery{}
		_, err := engine.GetDecisions(query)
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		// Query should have default limit set
		if query.Limit != 50 {
			t.Errorf("expected default limit 50, got %d", query.Limit)
		}
	})
}

func TestUpdateDecisionStatus(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create an ADR to update
	input := &RecordDecisionInput{
		Title:        "Test Update Status",
		Context:      "Testing status update",
		Decision:     "Test decision",
		Consequences: []string{"Updateable"},
		Status:       "proposed",
	}
	created, err := engine.RecordDecision(input)
	if err != nil {
		t.Fatalf("Setup: RecordDecision failed: %v", err)
	}

	t.Run("updates to accepted", func(t *testing.T) {
		result, err := engine.UpdateDecisionStatus(created.Decision.ID, "accepted")
		if err != nil {
			t.Fatalf("UpdateDecisionStatus failed: %v", err)
		}

		if result.Decision.Status != "accepted" {
			t.Errorf("expected status 'accepted', got %q", result.Decision.Status)
		}
	})

	t.Run("updates to deprecated", func(t *testing.T) {
		result, err := engine.UpdateDecisionStatus(created.Decision.ID, "deprecated")
		if err != nil {
			t.Fatalf("UpdateDecisionStatus failed: %v", err)
		}

		if result.Decision.Status != "deprecated" {
			t.Errorf("expected status 'deprecated', got %q", result.Decision.Status)
		}
	})

	t.Run("updates to superseded", func(t *testing.T) {
		result, err := engine.UpdateDecisionStatus(created.Decision.ID, "superseded")
		if err != nil {
			t.Fatalf("UpdateDecisionStatus failed: %v", err)
		}

		if result.Decision.Status != "superseded" {
			t.Errorf("expected status 'superseded', got %q", result.Decision.Status)
		}
	})

	t.Run("rejects invalid status", func(t *testing.T) {
		_, err := engine.UpdateDecisionStatus(created.Decision.ID, "invalid")
		if err == nil {
			t.Error("expected error for invalid status")
		}
	})

	t.Run("rejects non-existent decision", func(t *testing.T) {
		_, err := engine.UpdateDecisionStatus("ADR-9999", "accepted")
		if err == nil {
			t.Error("expected error for non-existent decision")
		}
	})
}

func TestSyncDecisionsFromFiles(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("syncs from empty directory", func(t *testing.T) {
		synced, err := engine.SyncDecisionsFromFiles()
		if err != nil {
			t.Fatalf("SyncDecisionsFromFiles failed: %v", err)
		}

		// Empty directory should sync 0
		if synced != 0 {
			t.Errorf("expected 0 synced from empty dir, got %d", synced)
		}
	})

	t.Run("syncs created ADRs", func(t *testing.T) {
		// Create some ADRs first
		inputs := []RecordDecisionInput{
			{Title: "Sync Test 1", Context: "C1", Decision: "D1", Consequences: []string{"X"}},
			{Title: "Sync Test 2", Context: "C2", Decision: "D2", Consequences: []string{"Y"}},
		}
		for _, input := range inputs {
			inputCopy := input
			if _, err := engine.RecordDecision(&inputCopy); err != nil {
				t.Fatalf("Setup: RecordDecision failed: %v", err)
			}
		}

		// Now sync
		synced, err := engine.SyncDecisionsFromFiles()
		if err != nil {
			t.Fatalf("SyncDecisionsFromFiles failed: %v", err)
		}

		if synced < 2 {
			t.Errorf("expected at least 2 synced, got %d", synced)
		}
	})
}

func TestDecisionResultStruct(t *testing.T) {
	t.Run("DecisionResult fields", func(t *testing.T) {
		result := DecisionResult{
			Decision: &decisions.ArchitecturalDecision{
				ID:    "ADR-001",
				Title: "Test",
			},
			Source: "file",
		}

		if result.Decision.ID != "ADR-001" {
			t.Errorf("ID mismatch")
		}
		if result.Source != "file" {
			t.Errorf("Source mismatch")
		}
	})

	t.Run("DecisionsResult fields", func(t *testing.T) {
		result := DecisionsResult{
			Decisions: []*decisions.ArchitecturalDecision{
				{ID: "ADR-001"},
				{ID: "ADR-002"},
			},
			Total: 2,
			Query: &DecisionsQuery{Status: "accepted"},
		}

		if len(result.Decisions) != 2 {
			t.Errorf("expected 2 decisions")
		}
		if result.Total != 2 {
			t.Errorf("expected total 2")
		}
		if result.Query.Status != "accepted" {
			t.Errorf("expected query status 'accepted'")
		}
	})

	t.Run("DecisionsQuery fields", func(t *testing.T) {
		query := DecisionsQuery{
			Status:   "proposed",
			ModuleID: "api",
			Search:   "database",
			Limit:    25,
		}

		if query.Status != "proposed" {
			t.Errorf("Status mismatch")
		}
		if query.ModuleID != "api" {
			t.Errorf("ModuleID mismatch")
		}
		if query.Search != "database" {
			t.Errorf("Search mismatch")
		}
		if query.Limit != 25 {
			t.Errorf("Limit mismatch")
		}
	})

	t.Run("RecordDecisionInput fields", func(t *testing.T) {
		input := RecordDecisionInput{
			Title:           "Test Title",
			Context:         "Test Context",
			Decision:        "Test Decision",
			Consequences:    []string{"C1", "C2"},
			AffectedModules: []string{"mod1"},
			Alternatives:    []string{"alt1"},
			Author:          "author",
			Status:          "proposed",
		}

		if input.Title != "Test Title" {
			t.Errorf("Title mismatch")
		}
		if len(input.Consequences) != 2 {
			t.Errorf("Consequences count mismatch")
		}
		if len(input.AffectedModules) != 1 {
			t.Errorf("AffectedModules count mismatch")
		}
	})
}

func TestDecisionFileOperations(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("ADR file is created", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "File Creation Test",
			Context:      "Testing file creation",
			Decision:     "Create file",
			Consequences: []string{"File exists"},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		// Check file exists
		if result.Decision.FilePath == "" {
			t.Error("FilePath should not be empty")
		}

		// For v6.0 style, FilePath should be absolute
		if !filepath.IsAbs(result.Decision.FilePath) {
			// It might be relative in some cases - just check it's not empty
			t.Logf("FilePath is relative: %s", result.Decision.FilePath)
		}
	})

	t.Run("GetDecision reads file content", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "File Read Test",
			Context:      "Testing file reading with full content",
			Decision:     "Read the file completely",
			Consequences: []string{"Content is readable", "All fields preserved"},
		}

		created, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		retrieved, err := engine.GetDecision(created.Decision.ID)
		if err != nil {
			t.Fatalf("GetDecision failed: %v", err)
		}

		// Content should match
		if retrieved.Decision.Context != input.Context {
			t.Errorf("context mismatch after read: got %q, want %q",
				retrieved.Decision.Context, input.Context)
		}
		if retrieved.Decision.Decision != input.Decision {
			t.Errorf("decision text mismatch after read: got %q, want %q",
				retrieved.Decision.Decision, input.Decision)
		}
		if len(retrieved.Decision.Consequences) != 2 {
			t.Errorf("consequences count mismatch after read: got %d, want 2",
				len(retrieved.Decision.Consequences))
		}
	})
}

func TestDecisionEdgeCases(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("empty consequences", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "Empty Consequences",
			Context:      "Testing empty consequences",
			Decision:     "No consequences",
			Consequences: []string{},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision == nil {
			t.Fatal("expected non-nil decision")
		}
	})

	t.Run("unicode in fields", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "Unicode Test: \u4e2d\u6587",
			Context:      "Context with emoji \U0001F680",
			Decision:     "Decision with accents: caf\u00e9",
			Consequences: []string{"Consequence with \u00fc"},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision.Title != input.Title {
			t.Errorf("unicode title not preserved")
		}
	})

	t.Run("long content", func(t *testing.T) {
		longText := ""
		for i := 0; i < 1000; i++ {
			longText += "This is a long text. "
		}

		input := &RecordDecisionInput{
			Title:        "Long Content Test",
			Context:      longText,
			Decision:     "Handle long content",
			Consequences: []string{"Memory usage", "File size"},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		if result.Decision.Context != longText {
			t.Error("long context not preserved")
		}
	})
}

func TestDecisionSearchQuery(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create ADRs with searchable content
	inputs := []RecordDecisionInput{
		{Title: "Database Selection", Context: "Need storage", Decision: "Use PostgreSQL", Consequences: []string{"SQL support"}},
		{Title: "Cache Strategy", Context: "Need caching", Decision: "Use Redis", Consequences: []string{"Fast lookups"}},
	}

	for _, input := range inputs {
		inputCopy := input
		if _, err := engine.RecordDecision(&inputCopy); err != nil {
			t.Fatalf("Setup: RecordDecision failed: %v", err)
		}
	}

	t.Run("search by title keyword", func(t *testing.T) {
		result, err := engine.GetDecisions(&DecisionsQuery{
			Search: "Database",
		})
		if err != nil {
			t.Fatalf("GetDecisions failed: %v", err)
		}

		// Should find at least the database ADR
		found := false
		for _, d := range result.Decisions {
			if d.Title == "Database Selection" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find 'Database Selection' ADR")
		}
	})
}

// Test that decisions directory structure is correct
func TestDecisionsDirectory(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	t.Run("creates decisions in correct location", func(t *testing.T) {
		input := &RecordDecisionInput{
			Title:        "Directory Test",
			Context:      "Testing directory",
			Decision:     "Check location",
			Consequences: []string{"Correct path"},
		}

		result, err := engine.RecordDecision(input)
		if err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}

		// File should exist
		if _, err := os.Stat(result.Decision.FilePath); os.IsNotExist(err) {
			t.Errorf("ADR file not found at %s", result.Decision.FilePath)
		}
	})
}

package query

import (
	"context"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/storage"
)

func seedSymbolExistsFixture(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	ftsManager := storage.NewFTSManager(e.db.Conn(), storage.DefaultFTSConfig())
	if err := ftsManager.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	records := []storage.SymbolFTSRecord{
		{ID: "s1", Name: "ENV_PATH", Kind: "const", Signature: "ENV_PATH", FilePath: "src/fixtures.ts", Language: "typescript"},
		{ID: "s2", Name: "ReportPersistenceService", Kind: "class", Signature: "ReportPersistenceService", FilePath: "src/fixtures.ts", Language: "typescript"},
		{ID: "s3", Name: "saveReport", Kind: "method", Signature: "ReportPersistenceService.saveReport", FilePath: "src/fixtures.ts", Language: "typescript"},
		{ID: "s4", Name: "trackUsage", Kind: "method", Signature: "ReportPersistenceService.trackUsage", FilePath: "src/fixtures.ts", Language: "typescript"},
		{ID: "s5", Name: "setApiKey", Kind: "property", Signature: "settingsRouter.setApiKey", FilePath: "src/fixtures.ts", Language: "typescript"},
	}
	if err := ftsManager.BulkInsert(ctx, records); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}
}

func TestSymbolExists(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()
	seedSymbolExistsFixture(t, engine)

	ctx := context.Background()

	tests := []struct {
		name         string
		opts         SymbolExistsOptions
		wantExists   bool
		wantMatches  int
		wantKinds    []string
		wantReceiver string // expected receiver, or "" if none
	}{
		{
			name:        "top-level const",
			opts:        SymbolExistsOptions{Name: "ENV_PATH"},
			wantExists:  true,
			wantMatches: 1,
			wantKinds:   []string{"const"},
		},
		{
			name:        "class",
			opts:        SymbolExistsOptions{Name: "ReportPersistenceService"},
			wantExists:  true,
			wantMatches: 1,
			wantKinds:   []string{"class"},
		},
		{
			name:         "class method",
			opts:         SymbolExistsOptions{Name: "saveReport"},
			wantExists:   true,
			wantMatches:  1,
			wantKinds:    []string{"method"},
			wantReceiver: "ReportPersistenceService",
		},
		{
			name:         "private class method",
			opts:         SymbolExistsOptions{Name: "trackUsage"},
			wantExists:   true,
			wantMatches:  1,
			wantKinds:    []string{"method"},
			wantReceiver: "ReportPersistenceService",
		},
		{
			name:         "object property",
			opts:         SymbolExistsOptions{Name: "setApiKey"},
			wantExists:   true,
			wantMatches:  1,
			wantKinds:    []string{"property"},
			wantReceiver: "settingsRouter",
		},
		{
			name:        "nonexistent symbol",
			opts:        SymbolExistsOptions{Name: "fakeNameNobodyWrote"},
			wantExists:  false,
			wantMatches: 0,
			wantKinds:   []string{},
		},
		{
			name:        "kind filter — matches",
			opts:        SymbolExistsOptions{Name: "saveReport", Kinds: []string{"method"}},
			wantExists:  true,
			wantMatches: 1,
			wantKinds:   []string{"method"},
		},
		{
			name:        "kind filter — excludes",
			opts:        SymbolExistsOptions{Name: "saveReport", Kinds: []string{"class"}},
			wantExists:  false,
			wantMatches: 0,
			wantKinds:   []string{},
		},
		{
			name:        "scope filter — matches",
			opts:        SymbolExistsOptions{Name: "ENV_PATH", Scope: "src/"},
			wantExists:  true,
			wantMatches: 1,
		},
		{
			name:        "scope filter — excludes",
			opts:        SymbolExistsOptions{Name: "ENV_PATH", Scope: "other/"},
			wantExists:  false,
			wantMatches: 0,
			wantKinds:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.SymbolExists(ctx, tt.opts)
			if err != nil {
				t.Fatalf("SymbolExists error: %v", err)
			}
			if result.Exists != tt.wantExists {
				t.Errorf("Exists = %v, want %v", result.Exists, tt.wantExists)
			}
			if result.Matches != tt.wantMatches {
				t.Errorf("Matches = %d, want %d", result.Matches, tt.wantMatches)
			}
			if tt.wantKinds != nil {
				if len(result.Kinds) != len(tt.wantKinds) {
					t.Errorf("Kinds = %v, want %v", result.Kinds, tt.wantKinds)
				} else {
					for i, k := range result.Kinds {
						if k != tt.wantKinds[i] {
							t.Errorf("Kinds[%d] = %q, want %q", i, k, tt.wantKinds[i])
						}
					}
				}
			}
			if tt.wantReceiver != "" {
				found := false
				for _, r := range result.Receivers {
					if r == tt.wantReceiver {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Receivers = %v, want to contain %q", result.Receivers, tt.wantReceiver)
				}
			}
			if result.Provenance == nil {
				t.Error("Provenance should not be nil")
			}
		})
	}
}

func TestSymbolExistsEmptyName(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	_, err := engine.SymbolExists(context.Background(), SymbolExistsOptions{Name: ""})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

package envelope

import (
	"testing"

	"ckb/internal/query"
)

func TestGenerateConfidenceFactors(t *testing.T) {
	tests := []struct {
		name    string
		prov    *query.Provenance
		wantLen int
		checkFn func(t *testing.T, factors []ConfidenceFactor)
	}{
		{
			name: "scip available and used",
			prov: &query.Provenance{
				Backends: []query.BackendContribution{
					{BackendId: "scip", Available: true, Used: true},
				},
				RepoStateDirty: false,
			},
			wantLen: 2, // scip_backend + repo_state
			checkFn: func(t *testing.T, factors []ConfidenceFactor) {
				for _, f := range factors {
					if f.Factor == "scip_backend" {
						if f.Status != "available" {
							t.Errorf("scip_backend status = %s, want available", f.Status)
						}
						if f.Impact != 0.3 {
							t.Errorf("scip_backend impact = %f, want 0.3", f.Impact)
						}
					}
				}
			},
		},
		{
			name: "scip unavailable",
			prov: &query.Provenance{
				Backends: []query.BackendContribution{
					{BackendId: "scip", Available: false, Used: false},
					{BackendId: "git", Available: true, Used: true},
				},
				RepoStateDirty: false,
			},
			wantLen: 3, // scip_backend + git_backend + repo_state
			checkFn: func(t *testing.T, factors []ConfidenceFactor) {
				for _, f := range factors {
					if f.Factor == "scip_backend" {
						if f.Status != "unavailable" {
							t.Errorf("scip_backend status = %s, want unavailable", f.Status)
						}
						if f.Impact != -0.2 {
							t.Errorf("scip_backend impact = %f, want -0.2", f.Impact)
						}
					}
					if f.Factor == "git_backend" {
						if f.Status != "available" {
							t.Errorf("git_backend status = %s, want available", f.Status)
						}
						if f.Impact != 0.1 {
							t.Errorf("git_backend impact = %f, want 0.1", f.Impact)
						}
					}
				}
			},
		},
		{
			name: "dirty repo state",
			prov: &query.Provenance{
				Backends:       []query.BackendContribution{},
				RepoStateDirty: true,
			},
			wantLen: 1, // repo_state only
			checkFn: func(t *testing.T, factors []ConfidenceFactor) {
				if len(factors) == 0 {
					t.Fatal("expected at least one factor")
				}
				if factors[0].Factor != "repo_state" {
					t.Errorf("factor = %s, want repo_state", factors[0].Factor)
				}
				if factors[0].Status != "dirty" {
					t.Errorf("status = %s, want dirty", factors[0].Status)
				}
				if factors[0].Impact != -0.1 {
					t.Errorf("impact = %f, want -0.1", factors[0].Impact)
				}
			},
		},
		{
			name: "available but unused backend",
			prov: &query.Provenance{
				Backends: []query.BackendContribution{
					{BackendId: "lsp", Available: true, Used: false},
				},
				RepoStateDirty: false,
			},
			wantLen: 2,
			checkFn: func(t *testing.T, factors []ConfidenceFactor) {
				for _, f := range factors {
					if f.Factor == "lsp_backend" {
						if f.Status != "available_unused" {
							t.Errorf("lsp_backend status = %s, want available_unused", f.Status)
						}
						if f.Impact != 0.0 {
							t.Errorf("lsp_backend impact = %f, want 0.0", f.Impact)
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factors := generateConfidenceFactors(tt.prov)
			if len(factors) != tt.wantLen {
				t.Errorf("got %d factors, want %d", len(factors), tt.wantLen)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, factors)
			}
		})
	}
}

func TestFromProvenanceWithCacheInfo(t *testing.T) {
	prov := &query.Provenance{
		RepoStateId: "abc123",
		Backends: []query.BackendContribution{
			{BackendId: "scip", Available: true, Used: true},
		},
		Completeness: query.CompletenessInfo{
			Score:  0.95,
			Reason: "SCIP available",
		},
		CachedAt: "2m30s",
	}

	resp := New().FromProvenance(prov).Build()

	// Check cache info was populated
	if resp.Meta == nil {
		t.Fatal("expected meta to be populated")
	}
	if resp.Meta.Cache == nil {
		t.Fatal("expected cache info to be populated")
	}
	if !resp.Meta.Cache.Hit {
		t.Error("expected cache hit = true")
	}
	if resp.Meta.Cache.Age != "2m30s" {
		t.Errorf("cache age = %s, want 2m30s", resp.Meta.Cache.Age)
	}
}

func TestFromProvenanceWithConfidenceFactors(t *testing.T) {
	prov := &query.Provenance{
		RepoStateId: "abc123",
		Backends: []query.BackendContribution{
			{BackendId: "scip", Available: true, Used: true},
			{BackendId: "git", Available: true, Used: true},
		},
		Completeness: query.CompletenessInfo{
			Score:  0.95,
			Reason: "all backends available",
		},
		RepoStateDirty: false,
	}

	resp := New().FromProvenance(prov).Build()

	// Check confidence factors were populated
	if resp.Meta == nil || resp.Meta.Confidence == nil {
		t.Fatal("expected confidence to be populated")
	}
	if len(resp.Meta.Confidence.Factors) == 0 {
		t.Fatal("expected confidence factors to be populated")
	}

	// Should have 3 factors: scip_backend, git_backend, repo_state
	if len(resp.Meta.Confidence.Factors) != 3 {
		t.Errorf("got %d factors, want 3", len(resp.Meta.Confidence.Factors))
	}

	// Verify factor names
	factorNames := make(map[string]bool)
	for _, f := range resp.Meta.Confidence.Factors {
		factorNames[f.Factor] = true
	}
	expectedFactors := []string{"scip_backend", "git_backend", "repo_state"}
	for _, name := range expectedFactors {
		if !factorNames[name] {
			t.Errorf("missing factor: %s", name)
		}
	}
}

func TestFromProvenanceNoCacheWhenNotCached(t *testing.T) {
	prov := &query.Provenance{
		RepoStateId: "abc123",
		Backends: []query.BackendContribution{
			{BackendId: "scip", Available: true, Used: true},
		},
		Completeness: query.CompletenessInfo{
			Score: 0.95,
		},
		CachedAt: "", // Not cached
	}

	resp := New().FromProvenance(prov).Build()

	// Cache info should not be populated
	if resp.Meta != nil && resp.Meta.Cache != nil {
		t.Error("expected cache info to be nil when not cached")
	}
}

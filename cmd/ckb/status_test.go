package main

import (
	"testing"
	"time"

	"ckb/internal/query"
	"ckb/internal/tier"
)

func TestStatusResponseCLI(t *testing.T) {
	resp := &StatusResponseCLI{
		CkbVersion: "7.5.0",
		Tier: &tier.TierInfo{
			Current:     tier.TierEnhanced,
			CurrentName: "enhanced",
			Mode:        "auto",
		},
		RepoState: &query.RepoState{
			RepoStateId: "abc123",
			HeadCommit:  "def456",
			Dirty:       false,
		},
		Backends: []BackendStatusCLI{
			{
				ID:           "scip",
				Available:    true,
				Healthy:      true,
				Capabilities: []string{"navigation", "references"},
			},
		},
		Cache: CacheStatusCLI{
			QueryCount: 100,
			ViewCount:  50,
			HitRate:    0.85,
			SizeBytes:  1024,
		},
		Healthy: true,
	}

	if resp.CkbVersion != "7.5.0" {
		t.Errorf("Expected CkbVersion='7.5.0', got %q", resp.CkbVersion)
	}
	if !resp.Healthy {
		t.Error("Expected Healthy=true")
	}
	if len(resp.Backends) != 1 {
		t.Errorf("Expected 1 backend, got %d", len(resp.Backends))
	}
	if resp.Cache.HitRate != 0.85 {
		t.Errorf("Expected HitRate=0.85, got %f", resp.Cache.HitRate)
	}
}

func TestIndexStatusCLI(t *testing.T) {
	now := time.Now()

	status := &IndexStatusCLI{
		Exists:         true,
		Fresh:          false,
		Reason:         "5 commit(s) behind HEAD",
		CreatedAt:      now,
		IndexAge:       "2 hours",
		CommitHash:     "abc123",
		FileCount:      100,
		CommitsBehind:  5,
		HasUncommitted: true,
	}

	if !status.Exists {
		t.Error("Expected Exists=true")
	}
	if status.Fresh {
		t.Error("Expected Fresh=false")
	}
	if status.CommitsBehind != 5 {
		t.Errorf("Expected CommitsBehind=5, got %d", status.CommitsBehind)
	}
	if !status.HasUncommitted {
		t.Error("Expected HasUncommitted=true")
	}
	if status.FileCount != 100 {
		t.Errorf("Expected FileCount=100, got %d", status.FileCount)
	}
}

func TestBackendStatusCLI(t *testing.T) {
	status := BackendStatusCLI{
		ID:           "scip",
		Available:    true,
		Healthy:      true,
		Capabilities: []string{"navigation", "references", "hover"},
		Details:      "SCIP backend with 1000 symbols",
	}

	if status.ID != "scip" {
		t.Errorf("Expected ID='scip', got %q", status.ID)
	}
	if !status.Available {
		t.Error("Expected Available=true")
	}
	if len(status.Capabilities) != 3 {
		t.Errorf("Expected 3 capabilities, got %d", len(status.Capabilities))
	}
}

func TestCacheStatusCLI(t *testing.T) {
	status := CacheStatusCLI{
		QueryCount: 500,
		ViewCount:  200,
		HitRate:    0.75,
		SizeBytes:  10240,
	}

	if status.QueryCount != 500 {
		t.Errorf("Expected QueryCount=500, got %d", status.QueryCount)
	}
	if status.ViewCount != 200 {
		t.Errorf("Expected ViewCount=200, got %d", status.ViewCount)
	}
	if status.HitRate != 0.75 {
		t.Errorf("Expected HitRate=0.75, got %f", status.HitRate)
	}
	if status.SizeBytes != 10240 {
		t.Errorf("Expected SizeBytes=10240, got %d", status.SizeBytes)
	}
}

func TestConvertStatusResponse(t *testing.T) {
	// Create a query.StatusResponse
	queryResp := &query.StatusResponse{
		CkbVersion: "7.5.0",
		Tier: &tier.TierInfo{
			Current:     tier.TierEnhanced,
			CurrentName: "enhanced",
			Mode:        "auto",
		},
		RepoState: &query.RepoState{
			RepoStateId: "abc123",
			HeadCommit:  "def456",
			Dirty:       false,
		},
		Backends: []query.BackendStatus{
			{
				Id:           "scip",
				Available:    true,
				Healthy:      true,
				Capabilities: []string{"navigation"},
				Details:      "test",
			},
		},
		Cache: &query.CacheStatus{
			QueriesCached: 100,
			ViewsCached:   50,
			HitRate:       0.9,
			SizeBytes:     2048,
		},
		Healthy: true,
	}

	cliResp := convertStatusResponse(queryResp)

	if cliResp.CkbVersion != "7.5.0" {
		t.Errorf("Expected CkbVersion='7.5.0', got %q", cliResp.CkbVersion)
	}
	if !cliResp.Healthy {
		t.Error("Expected Healthy=true")
	}
	if len(cliResp.Backends) != 1 {
		t.Errorf("Expected 1 backend, got %d", len(cliResp.Backends))
	}
	if cliResp.Backends[0].ID != "scip" {
		t.Errorf("Expected backend ID='scip', got %q", cliResp.Backends[0].ID)
	}
	if cliResp.Cache.QueryCount != 100 {
		t.Errorf("Expected QueryCount=100, got %d", cliResp.Cache.QueryCount)
	}
	if cliResp.Cache.HitRate != 0.9 {
		t.Errorf("Expected HitRate=0.9, got %f", cliResp.Cache.HitRate)
	}
}

func TestConvertStatusResponse_NilCache(t *testing.T) {
	queryResp := &query.StatusResponse{
		CkbVersion: "7.5.0",
		Backends:   []query.BackendStatus{},
		Cache:      nil,
		Healthy:    true,
	}

	cliResp := convertStatusResponse(queryResp)

	// Should have zero-value cache, not panic
	if cliResp.Cache.QueryCount != 0 {
		t.Errorf("Expected QueryCount=0 for nil cache, got %d", cliResp.Cache.QueryCount)
	}
}

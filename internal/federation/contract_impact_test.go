package federation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMatchOwnershipPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		// Wildcard patterns
		{"star matches all", "*", "any/path/file.go", true},
		{"double star matches all", "**", "any/path/file.go", true},

		// Exact matches
		{"exact match", "internal/api/handler.go", "internal/api/handler.go", true},
		{"exact mismatch", "internal/api/handler.go", "internal/api/other.go", false},

		// Directory patterns (trailing slash)
		{"dir pattern matches file in dir", "internal/api/", "internal/api/handler.go", true},
		{"dir pattern matches subdir", "internal/api/", "internal/api/v2/handler.go", true},
		{"dir pattern no match", "internal/api/", "internal/query/engine.go", false},
		{"dir pattern exact match", "internal/api/", "internal/api", true},

		// Prefix patterns (no trailing slash but matches as dir)
		{"prefix matches subpath", "internal/api", "internal/api/handler.go", true},
		{"prefix no match", "internal/api", "internal/query/engine.go", false},

		// Double star patterns
		{"double star prefix", "internal/**/*.go", "internal/api/handler.go", true},
		{"double star deep", "internal/**/*.go", "internal/api/v2/types/model.go", true},
		{"double star no go extension", "internal/**/*.go", "internal/api/config.json", false},
		{"double star prefix only", "internal/**", "internal/api/handler.go", true},

		// Single star glob patterns
		{"star glob", "*.go", "handler.go", true},
		{"star glob mismatch", "*.go", "handler.ts", false},
		{"star glob in path", "internal/*.go", "internal/types.go", true},
		{"star glob in path subdir no match", "internal/*.go", "internal/api/types.go", false},

		// File patterns (match by basename)
		{"basename glob", "*.proto", "api/v1/service.proto", true},
		{"basename glob no match", "*.proto", "api/v1/service.go", false},

		// Leading slash normalization
		{"leading slash pattern", "/internal/api", "internal/api/handler.go", true},
		{"leading slash path", "internal/api", "/internal/api/handler.go", true},
		{"both leading slashes", "/internal/api", "/internal/api/handler.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchOwnershipPattern(tt.pattern, tt.path)
			if result != tt.expected {
				t.Errorf("matchOwnershipPattern(%q, %q) = %v, want %v",
					tt.pattern, tt.path, result, tt.expected)
			}
		})
	}
}

func TestPatternSpecificity(t *testing.T) {
	tests := []struct {
		pattern  string
		minScore float64
		maxScore float64
	}{
		// Wildcards have low specificity
		{"*", 0.0, 0.2},
		{"**", 0.0, 0.2},

		// Double star patterns
		{"internal/**/*.go", 0.4, 0.6},
		{"**/*.proto", 0.4, 0.6},

		// Single star patterns
		{"*.go", 0.6, 0.8},
		{"internal/*.go", 0.6, 0.8},

		// Exact paths have high specificity
		{"internal/api/handler.go", 1.0, 2.0},
		{"cmd/ckb/main.go", 1.0, 2.0},
		{"a", 1.0, 1.1}, // short path
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			result := patternSpecificity(tt.pattern)
			if result < tt.minScore || result > tt.maxScore {
				t.Errorf("patternSpecificity(%q) = %f, want between %f and %f",
					tt.pattern, result, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestDeduplicateOwners(t *testing.T) {
	t.Run("merges duplicate owners", func(t *testing.T) {
		owners := []Owner{
			{Type: "user", ID: "@alice", Weight: 0.5},
			{Type: "user", ID: "@bob", Weight: 0.3},
			{Type: "user", ID: "@alice", Weight: 0.3}, // duplicate
		}

		result := deduplicateOwners(owners)

		// Should have 2 unique owners
		if len(result) != 2 {
			t.Errorf("expected 2 owners, got %d", len(result))
		}

		// Find alice and check merged weight
		var aliceWeight float64
		for _, o := range result {
			if o.ID == "@alice" {
				aliceWeight = o.Weight
			}
		}
		if aliceWeight != 0.8 { // 0.5 + 0.3
			t.Errorf("expected alice weight 0.8, got %f", aliceWeight)
		}
	})

	t.Run("handles empty list", func(t *testing.T) {
		result := deduplicateOwners(nil)
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d owners", len(result))
		}
	})

	t.Run("preserves single owner", func(t *testing.T) {
		owners := []Owner{
			{Type: "team", ID: "@org/backend", Weight: 1.0},
		}

		result := deduplicateOwners(owners)
		if len(result) != 1 {
			t.Errorf("expected 1 owner, got %d", len(result))
		}
		if result[0].Weight != 1.0 {
			t.Errorf("expected weight 1.0, got %f", result[0].Weight)
		}
	})

	t.Run("merges different types separately", func(t *testing.T) {
		owners := []Owner{
			{Type: "user", ID: "@alice", Weight: 0.5},
			{Type: "team", ID: "@alice", Weight: 0.3}, // same ID, different type
		}

		result := deduplicateOwners(owners)
		if len(result) != 2 {
			t.Errorf("expected 2 owners (different types), got %d", len(result))
		}
	})
}

func TestComputeRisk(t *testing.T) {
	t.Run("high risk - many repos", func(t *testing.T) {
		contract := &Contract{
			ContractType: ContractTypeOpenAPI,
			Visibility:   VisibilityPublic,
		}
		consumers := []Consumer{
			{RepoID: "repo1", Tier: TierDeclared},
			{RepoID: "repo2", Tier: TierDeclared},
			{RepoID: "repo3", Tier: TierDeclared},
		}

		level, factors := computeRisk(contract, consumers, 5)

		if level != "high" {
			t.Errorf("expected 'high' risk, got %q", level)
		}
		if len(factors) == 0 {
			t.Error("expected risk factors")
		}
	})

	t.Run("medium risk - few repos", func(t *testing.T) {
		contract := &Contract{
			ContractType: ContractTypeOpenAPI,
			Visibility:   VisibilityInternal,
		}
		consumers := []Consumer{
			{RepoID: "repo1", Tier: TierDerived},
		}

		level, _ := computeRisk(contract, consumers, 2)

		if level != "medium" {
			t.Errorf("expected 'medium' risk, got %q", level)
		}
	})

	t.Run("low risk - single repo", func(t *testing.T) {
		contract := &Contract{
			ContractType: ContractTypeOpenAPI,
			Visibility:   VisibilityInternal,
		}
		consumers := []Consumer{}

		level, _ := computeRisk(contract, consumers, 1)

		if level != "low" {
			t.Errorf("expected 'low' risk, got %q", level)
		}
	})

	t.Run("proto with services increases risk", func(t *testing.T) {
		metadata, _ := json.Marshal(ProtoMetadata{
			Services:    []string{"UserService", "AuthService"},
			PackageName: "api.user",
		})
		contract := &Contract{
			ContractType: ContractTypeProto,
			Visibility:   VisibilityPublic,
			Metadata:     metadata,
		}
		consumers := []Consumer{}

		level, factors := computeRisk(contract, consumers, 1)

		// Should have factor about services
		hasServiceFactor := false
		for _, f := range factors {
			if strings.Contains(f, "gRPC services") {
				hasServiceFactor = true
				break
			}
		}
		if !hasServiceFactor {
			t.Error("expected risk factor about gRPC services")
		}

		// Should also have factor about not versioned
		hasVersionFactor := false
		for _, f := range factors {
			if strings.Contains(f, "not versioned") {
				hasVersionFactor = true
				break
			}
		}
		if !hasVersionFactor {
			t.Error("expected risk factor about versioning")
		}
		_ = level // we're testing factors, not level here
	})

	t.Run("versioned proto package reduces risk", func(t *testing.T) {
		metadata, _ := json.Marshal(ProtoMetadata{
			Services:    []string{"UserService"},
			PackageName: "api.user.v1", // versioned
		})
		contract := &Contract{
			ContractType: ContractTypeProto,
			Visibility:   VisibilityInternal,
			Metadata:     metadata,
		}
		consumers := []Consumer{}

		_, factors := computeRisk(contract, consumers, 1)

		// Should NOT have factor about not versioned
		for _, f := range factors {
			if strings.Contains(f, "not versioned") {
				t.Error("versioned package should not have 'not versioned' factor")
			}
		}
	})

	t.Run("many declared consumers increases risk", func(t *testing.T) {
		contract := &Contract{
			ContractType: ContractTypeOpenAPI,
			Visibility:   VisibilityInternal,
		}
		consumers := []Consumer{
			{RepoID: "repo1", Tier: TierDeclared},
			{RepoID: "repo2", Tier: TierDeclared},
			{RepoID: "repo3", Tier: TierDeclared},
			{RepoID: "repo4", Tier: TierDerived}, // not declared
		}

		_, factors := computeRisk(contract, consumers, 4)

		hasDeclaredFactor := false
		for _, f := range factors {
			if strings.Contains(f, "declared dependencies") {
				hasDeclaredFactor = true
				break
			}
		}
		if !hasDeclaredFactor {
			t.Error("expected risk factor about declared dependencies")
		}
	})
}

func TestEdgesToConsumers(t *testing.T) {
	t.Run("converts edges to consumers", func(t *testing.T) {
		edges := []ContractEdge{
			{
				ConsumerRepoUID: "uid1",
				ConsumerRepoID:  "repo1",
				ConsumerPaths:   []string{"src/client.go"},
				Tier:            TierDeclared,
				EvidenceType:    "import",
				Confidence:      0.95,
			},
			{
				ConsumerRepoUID: "uid2",
				ConsumerRepoID:  "repo2",
				ConsumerPaths:   []string{"lib/api.go", "lib/types.go"},
				Tier:            TierDerived,
				EvidenceType:    "generated",
				Confidence:      0.8,
			},
		}

		consumers := edgesToConsumers(edges)

		if len(consumers) != 2 {
			t.Fatalf("expected 2 consumers, got %d", len(consumers))
		}

		// Check first consumer
		if consumers[0].RepoID != "repo1" {
			t.Errorf("expected repo1, got %s", consumers[0].RepoID)
		}
		if consumers[0].Tier != TierDeclared {
			t.Errorf("expected tier declared, got %s", consumers[0].Tier)
		}
		if len(consumers[0].ConsumerPaths) != 1 {
			t.Errorf("expected 1 path, got %d", len(consumers[0].ConsumerPaths))
		}

		// Check second consumer
		if consumers[1].RepoID != "repo2" {
			t.Errorf("expected repo2, got %s", consumers[1].RepoID)
		}
		if len(consumers[1].ConsumerPaths) != 2 {
			t.Errorf("expected 2 paths, got %d", len(consumers[1].ConsumerPaths))
		}
	})

	t.Run("handles empty edges", func(t *testing.T) {
		consumers := edgesToConsumers(nil)
		if len(consumers) != 0 {
			t.Errorf("expected empty consumers, got %d", len(consumers))
		}
	})
}

// Test type structures
func TestContractTypes(t *testing.T) {
	t.Run("ContractType constants", func(t *testing.T) {
		if ContractTypeProto != "proto" {
			t.Errorf("unexpected proto type: %s", ContractTypeProto)
		}
		if ContractTypeOpenAPI != "openapi" {
			t.Errorf("unexpected openapi type: %s", ContractTypeOpenAPI)
		}
		if ContractTypeGraphQL != "graphql" {
			t.Errorf("unexpected graphql type: %s", ContractTypeGraphQL)
		}
	})

	t.Run("Visibility constants", func(t *testing.T) {
		if VisibilityPublic != "public" {
			t.Errorf("unexpected public visibility: %s", VisibilityPublic)
		}
		if VisibilityInternal != "internal" {
			t.Errorf("unexpected internal visibility: %s", VisibilityInternal)
		}
		if VisibilityUnknown != "unknown" {
			t.Errorf("unexpected unknown visibility: %s", VisibilityUnknown)
		}
	})

	t.Run("EvidenceTier constants", func(t *testing.T) {
		if TierDeclared != "declared" {
			t.Errorf("unexpected declared tier: %s", TierDeclared)
		}
		if TierDerived != "derived" {
			t.Errorf("unexpected derived tier: %s", TierDerived)
		}
		if TierHeuristic != "heuristic" {
			t.Errorf("unexpected heuristic tier: %s", TierHeuristic)
		}
	})
}

func TestContractStructures(t *testing.T) {
	t.Run("Contract JSON serialization", func(t *testing.T) {
		metadata, _ := json.Marshal(ProtoMetadata{
			PackageName: "api.user",
			Services:    []string{"UserService"},
		})
		contract := Contract{
			ID:           "repo1:api/user.proto",
			RepoUID:      "uid1",
			RepoID:       "repo1",
			Path:         "api/user.proto",
			ContractType: ContractTypeProto,
			Metadata:     metadata,
			Visibility:   VisibilityPublic,
			Confidence:   0.95,
		}

		data, err := json.Marshal(contract)
		if err != nil {
			t.Fatalf("failed to marshal contract: %v", err)
		}

		var decoded Contract
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal contract: %v", err)
		}

		if decoded.ID != contract.ID {
			t.Errorf("ID mismatch: %s vs %s", decoded.ID, contract.ID)
		}
		if decoded.ContractType != contract.ContractType {
			t.Errorf("ContractType mismatch: %s vs %s", decoded.ContractType, contract.ContractType)
		}
	})

	t.Run("Consumer structure", func(t *testing.T) {
		consumer := Consumer{
			RepoID:        "my-service",
			RepoUID:       "uid123",
			ConsumerPaths: []string{"src/client.go", "src/types.go"},
			Tier:          TierDeclared,
			EvidenceType:  "import",
			Confidence:    0.95,
		}

		if consumer.RepoID != "my-service" {
			t.Errorf("RepoID mismatch")
		}
		if len(consumer.ConsumerPaths) != 2 {
			t.Errorf("expected 2 paths")
		}
	})

	t.Run("ContractEdge structure", func(t *testing.T) {
		edge := ContractEdge{
			EdgeKey:         "key1",
			ContractID:      "contract1",
			ConsumerRepoUID: "uid1",
			ConsumerRepoID:  "repo1",
			ConsumerPaths:   []string{"src/client.go"},
			Tier:            TierDeclared,
			EvidenceType:    "import",
			Confidence:      0.9,
			DetectorName:    "proto-detector",
		}

		if edge.EdgeKey != "key1" {
			t.Errorf("EdgeKey mismatch")
		}
		if edge.Tier != TierDeclared {
			t.Errorf("Tier mismatch")
		}
	})
}

func TestImpactStructures(t *testing.T) {
	t.Run("ImpactSummary", func(t *testing.T) {
		summary := ImpactSummary{
			DirectRepoCount:     3,
			TransitiveRepoCount: 5,
			TotalRepoCount:      8,
			RiskLevel:           "high",
			RiskFactors:         []string{"Many consumers", "Public API"},
		}

		if summary.TotalRepoCount != 8 {
			t.Errorf("TotalRepoCount mismatch")
		}
		if len(summary.RiskFactors) != 2 {
			t.Errorf("expected 2 risk factors")
		}
	})

	t.Run("ImpactOwnership", func(t *testing.T) {
		ownership := ImpactOwnership{
			DefinitionOwners: []Owner{{Type: "user", ID: "@alice"}},
			ConsumerOwners:   []Owner{{Type: "team", ID: "@backend"}},
			ApprovalRequired: []Owner{{Type: "user", ID: "@alice"}},
		}

		if len(ownership.DefinitionOwners) != 1 {
			t.Errorf("expected 1 definition owner")
		}
	})
}

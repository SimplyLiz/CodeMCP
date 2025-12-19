package federation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// AnalyzeContractImpactOptions contains options for impact analysis
type AnalyzeContractImpactOptions struct {
	// Federation is the federation name
	Federation string `json:"federation"`

	// RepoID is the repo containing the contract
	RepoID string `json:"repoId"`

	// Path is the file path to analyze
	Path string `json:"path"`

	// IncludeHeuristic includes tier 3 edges
	IncludeHeuristic bool `json:"includeHeuristic,omitempty"`

	// IncludeTransitive includes transitive consumers
	IncludeTransitive bool `json:"includeTransitive,omitempty"`

	// MaxDepth is the transitive depth limit
	MaxDepth int `json:"maxDepth,omitempty"`
}

// ContractImpactResponse contains the results of impact analysis
type ContractImpactResponse struct {
	// Contract is the contract being analyzed (nil if path isn't a contract)
	Contract *ContractSummary `json:"contract,omitempty"`

	// DirectConsumers are repos that directly consume this contract
	DirectConsumers []Consumer `json:"directConsumers"`

	// TransitiveConsumers are repos that consume this contract transitively
	TransitiveConsumers []TransitiveConsumer `json:"transitiveConsumers"`

	// Summary contains aggregated stats
	Summary ImpactSummary `json:"summary"`

	// Ownership contains ownership info
	Ownership ImpactOwnership `json:"ownership"`

	// Staleness contains freshness info
	Staleness FederationStaleness `json:"staleness"`

	// Limitations lists any limitations of the analysis
	Limitations []Limitation `json:"limitations,omitempty"`
}

// ContractSummary is a summary of a contract
type ContractSummary struct {
	ContractID   string       `json:"contractId"`
	ContractType ContractType `json:"contractType"`
	Visibility   Visibility   `json:"visibility"`
	Path         string       `json:"path"`
	RepoID       string       `json:"repoId"`
}

// Consumer represents a consuming repository
type Consumer struct {
	RepoID        string       `json:"repoId"`
	RepoUID       string       `json:"repoUid"`
	ConsumerPaths []string     `json:"consumerPaths"`
	Tier          EvidenceTier `json:"tier"`
	EvidenceType  string       `json:"evidenceType"`
	Confidence    float64      `json:"confidence"`
}

// TransitiveConsumer is a consumer via a chain of imports
type TransitiveConsumer struct {
	Consumer
	ViaContract string `json:"viaContract"`
	Depth       int    `json:"depth"`
}

// ImpactSummary summarizes the impact
type ImpactSummary struct {
	DirectRepoCount     int      `json:"directRepoCount"`
	TransitiveRepoCount int      `json:"transitiveRepoCount"`
	TotalRepoCount      int      `json:"totalRepoCount"`
	RiskLevel           string   `json:"riskLevel"` // low, medium, high
	RiskFactors         []string `json:"riskFactors"`
}

// ImpactOwnership contains ownership info for impact
type ImpactOwnership struct {
	DefinitionOwners []Owner `json:"definitionOwners"`
	ConsumerOwners   []Owner `json:"consumerOwners"`
	ApprovalRequired []Owner `json:"approvalRequired"`
}

// Limitation describes a limitation of the analysis
type Limitation struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Path    string `json:"path,omitempty"`
}

// AnalyzeContractImpact analyzes the impact of changing a contract
func (f *Federation) AnalyzeContractImpact(opts AnalyzeContractImpactOptions) (*ContractImpactResponse, error) {
	// Set defaults
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}
	// Note: IncludeTransitive defaults to false (zero value), callers must opt-in

	// Find the repo
	repo := f.config.GetRepo(opts.RepoID)
	if repo == nil {
		return nil, fmt.Errorf("repository %q not found in federation", opts.RepoID)
	}

	// Construct the contract ID
	contractID := fmt.Sprintf("%s:%s", repo.RepoUID, opts.Path)

	// Get the contract
	contract, err := f.index.GetContract(contractID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract: %w", err)
	}

	// Get staleness info
	staleness := f.computeStaleness()

	// If not a contract, return empty result
	if contract == nil {
		return &ContractImpactResponse{
			Contract:            nil,
			DirectConsumers:     []Consumer{},
			TransitiveConsumers: []TransitiveConsumer{},
			Summary: ImpactSummary{
				DirectRepoCount:     0,
				TransitiveRepoCount: 0,
				TotalRepoCount:      0,
				RiskLevel:           "low",
				RiskFactors:         []string{"Path is not a recognized contract"},
			},
			Ownership: ImpactOwnership{
				DefinitionOwners: []Owner{},
				ConsumerOwners:   []Owner{},
				ApprovalRequired: []Owner{},
			},
			Staleness: staleness,
			Limitations: []Limitation{
				{Type: "not_a_contract", Path: opts.Path},
			},
		}, nil
	}

	// Find direct consumers
	minTier := TierDerived
	if opts.IncludeHeuristic {
		minTier = TierHeuristic
	}

	edges, err := f.index.FindDirectConsumers(contractID, minTier)
	if err != nil {
		return nil, fmt.Errorf("failed to find consumers: %w", err)
	}

	directConsumers := edgesToConsumers(edges)

	// Find transitive consumers
	var transitiveConsumers []TransitiveConsumer
	if opts.IncludeTransitive && contract.ContractType == ContractTypeProto {
		transitiveConsumers, err = f.findTransitiveConsumers(contractID, minTier, opts.MaxDepth)
		if err != nil && f.logger != nil {
			f.logger.Warn("Failed to find transitive consumers", map[string]interface{}{
				"contract": contractID,
				"error":    err.Error(),
			})
		}
	}

	// Compute unique repos
	allRepos := make(map[string]bool)
	directRepos := make(map[string]bool)
	for _, c := range directConsumers {
		directRepos[c.RepoID] = true
		allRepos[c.RepoID] = true
	}

	transitiveRepos := make(map[string]bool)
	for _, c := range transitiveConsumers {
		if !directRepos[c.RepoID] {
			transitiveRepos[c.RepoID] = true
		}
		allRepos[c.RepoID] = true
	}

	// Compute risk level
	riskLevel, riskFactors := computeRisk(contract, directConsumers, len(allRepos))

	// Gather ownership
	ownership := f.gatherOwnership(contract, directConsumers)

	return &ContractImpactResponse{
		Contract: &ContractSummary{
			ContractID:   contract.ID,
			ContractType: contract.ContractType,
			Visibility:   contract.Visibility,
			Path:         contract.Path,
			RepoID:       opts.RepoID,
		},
		DirectConsumers:     directConsumers,
		TransitiveConsumers: transitiveConsumers,
		Summary: ImpactSummary{
			DirectRepoCount:     len(directRepos),
			TransitiveRepoCount: len(transitiveRepos),
			TotalRepoCount:      len(allRepos),
			RiskLevel:           riskLevel,
			RiskFactors:         riskFactors,
		},
		Ownership: ownership,
		Staleness: staleness,
	}, nil
}

// findTransitiveConsumers finds consumers via proto imports
func (f *Federation) findTransitiveConsumers(contractID string, minTier EvidenceTier, maxDepth int) ([]TransitiveConsumer, error) {
	// Find reverse import closure
	dependentContracts, err := f.index.FindReverseImportClosure(contractID, maxDepth)
	if err != nil {
		return nil, err
	}

	var transitiveConsumers []TransitiveConsumer

	for _, dep := range dependentContracts {
		// Find consumers of this dependent contract
		edges, err := f.index.FindDirectConsumers(dep.Contract.ID, minTier)
		if err != nil {
			continue
		}

		for _, edge := range edges {
			transitiveConsumers = append(transitiveConsumers, TransitiveConsumer{
				Consumer: Consumer{
					RepoID:        edge.ConsumerRepoID,
					RepoUID:       edge.ConsumerRepoUID,
					ConsumerPaths: edge.ConsumerPaths,
					Tier:          edge.Tier,
					EvidenceType:  edge.EvidenceType,
					Confidence:    edge.Confidence,
				},
				ViaContract: dep.Contract.ID,
				Depth:       dep.Depth,
			})
		}
	}

	return transitiveConsumers, nil
}

// edgesToConsumers converts edges to consumers
func edgesToConsumers(edges []ContractEdge) []Consumer {
	consumers := make([]Consumer, len(edges))
	for i, e := range edges {
		consumers[i] = Consumer{
			RepoID:        e.ConsumerRepoID,
			RepoUID:       e.ConsumerRepoUID,
			ConsumerPaths: e.ConsumerPaths,
			Tier:          e.Tier,
			EvidenceType:  e.EvidenceType,
			Confidence:    e.Confidence,
		}
	}
	return consumers
}

// computeRisk computes risk level and factors
func computeRisk(contract *Contract, consumers []Consumer, totalRepoCount int) (string, []string) {
	var factors []string
	score := 0

	// Factor: Number of consuming repos
	if totalRepoCount >= 5 {
		score += 3
		factors = append(factors, fmt.Sprintf("%d repos depend on this contract", totalRepoCount))
	} else if totalRepoCount >= 2 {
		score += 2
		factors = append(factors, fmt.Sprintf("%d repos depend on this contract", totalRepoCount))
	}

	// Factor: Public visibility
	if contract.Visibility == VisibilityPublic {
		score += 1
		factors = append(factors, "Contract is marked as public API")
	}

	// Factor: Has service definitions (proto)
	if contract.ContractType == ContractTypeProto {
		var metadata ProtoMetadata
		if err := json.Unmarshal(contract.Metadata, &metadata); err == nil {
			if len(metadata.Services) > 0 {
				score += 1
				factors = append(factors, "Contract defines gRPC services")
			}

			// Factor: Not versioned
			if metadata.PackageName != "" && !versionedPkgRegex.MatchString(metadata.PackageName) {
				score += 1
				factors = append(factors, "Contract is not versioned (e.g., no .v1 suffix)")
			}
		}
	}

	// Factor: High-confidence consumers
	declaredCount := 0
	for _, c := range consumers {
		if c.Tier == TierDeclared {
			declaredCount++
		}
	}
	if declaredCount >= 3 {
		score += 1
		factors = append(factors, fmt.Sprintf("%d consumers with declared dependencies", declaredCount))
	}

	// Compute level
	var level string
	if score >= 4 {
		level = "high"
	} else if score >= 2 {
		level = "medium"
	} else {
		level = "low"
	}

	return level, factors
}

// gatherOwnership gathers ownership info from federated_ownership table
func (f *Federation) gatherOwnership(contract *Contract, consumers []Consumer) ImpactOwnership {
	ownership := ImpactOwnership{
		DefinitionOwners: []Owner{},
		ConsumerOwners:   []Owner{},
		ApprovalRequired: []Owner{},
	}

	// Query ownership for the contract definition
	defOwners := f.queryOwnership(contract.RepoUID, contract.Path)
	ownership.DefinitionOwners = defOwners

	// Query ownership for consumer paths
	seenOwners := make(map[string]bool)
	for _, c := range consumers {
		for _, path := range c.ConsumerPaths {
			owners := f.queryOwnership(c.RepoUID, path)
			for _, o := range owners {
				key := o.Type + ":" + o.ID
				if !seenOwners[key] {
					seenOwners[key] = true
					ownership.ConsumerOwners = append(ownership.ConsumerOwners, o)
				}
			}
		}
	}

	// Approval required: definition owners + top consumer owners
	ownership.ApprovalRequired = append(ownership.ApprovalRequired, ownership.DefinitionOwners...)

	// Add top N consumer owners (limit to 5)
	topN := 5
	if len(ownership.ConsumerOwners) < topN {
		topN = len(ownership.ConsumerOwners)
	}
	for i := 0; i < topN; i++ {
		// Avoid duplicates in approval list
		key := ownership.ConsumerOwners[i].Type + ":" + ownership.ConsumerOwners[i].ID
		alreadyInApproval := false
		for _, a := range ownership.ApprovalRequired {
			if a.Type+":"+a.ID == key {
				alreadyInApproval = true
				break
			}
		}
		if !alreadyInApproval {
			ownership.ApprovalRequired = append(ownership.ApprovalRequired, ownership.ConsumerOwners[i])
		}
	}

	return ownership
}

// queryOwnership queries federated_ownership for owners matching a path
func (f *Federation) queryOwnership(repoUID, path string) []Owner {
	var owners []Owner

	// Query ownership entries for this repo that could match the path
	// We fetch patterns and filter in Go for proper glob matching
	rows, err := f.index.db.Query(`
		SELECT pattern, owners, scope, source, confidence
		FROM federated_ownership
		WHERE repo_uid = ?
		ORDER BY confidence DESC
	`, repoUID)
	if err != nil {
		return owners
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var pattern, ownersJSON, scope, source string
		var confidence float64
		if err := rows.Scan(&pattern, &ownersJSON, &scope, &source, &confidence); err != nil {
			continue
		}

		// Check if pattern matches the path
		if !matchOwnershipPattern(pattern, path) {
			continue
		}

		// Parse owners JSON
		var ownerList []Owner
		if err := json.Unmarshal([]byte(ownersJSON), &ownerList); err != nil {
			continue
		}

		// Add owners with their weight adjusted by confidence
		// More specific patterns (longer, no wildcards) get higher weight
		specificity := patternSpecificity(pattern)
		for _, o := range ownerList {
			o.Weight = o.Weight * confidence * specificity
			owners = append(owners, o)
		}
	}

	// Deduplicate and sort by weight
	deduped := deduplicateOwners(owners)
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Weight > deduped[j].Weight
	})

	// Return top 5
	if len(deduped) > 5 {
		return deduped[:5]
	}
	return deduped
}

// matchOwnershipPattern matches a path against a CODEOWNERS-style pattern
func matchOwnershipPattern(pattern, path string) bool {
	// Handle special cases
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Normalize paths
	pattern = strings.TrimPrefix(pattern, "/")
	path = strings.TrimPrefix(path, "/")

	// If pattern ends with /, it matches directories
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(path, dirPattern+"/") || path == dirPattern
	}

	// If pattern contains **, use path.Match with recursive matching
	if strings.Contains(pattern, "**") {
		// Convert ** to match any path segment
		// e.g., "internal/**/*.go" should match "internal/foo/bar/baz.go"
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")

			// Check if path starts with prefix
			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}

			// Check if path ends with suffix pattern
			if suffix != "" {
				remainder := path
				if prefix != "" {
					remainder = strings.TrimPrefix(path, prefix)
					remainder = strings.TrimPrefix(remainder, "/")
				}
				// Simple glob match for suffix
				matched, _ := filepath.Match(suffix, filepath.Base(remainder))
				if !matched && !strings.HasSuffix(remainder, suffix) {
					return false
				}
			}
			return true
		}
	}

	// Try exact match first
	if path == pattern {
		return true
	}

	// Try as a prefix (directory pattern without trailing /)
	if strings.HasPrefix(path, pattern+"/") {
		return true
	}

	// Try glob match
	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Try matching just the filename for patterns like "*.go"
	if strings.Contains(pattern, "*") && !strings.Contains(pattern, "/") {
		matched, _ = filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
	}

	return false
}

// patternSpecificity returns a weight based on how specific a pattern is
// More specific patterns should have higher weight
func patternSpecificity(pattern string) float64 {
	// Wildcards reduce specificity
	if pattern == "*" || pattern == "**" {
		return 0.1
	}
	if strings.Contains(pattern, "**") {
		return 0.5
	}
	if strings.Contains(pattern, "*") {
		return 0.7
	}
	// Longer patterns are more specific
	specificity := 1.0 + float64(len(pattern))/100.0
	if specificity > 2.0 {
		specificity = 2.0
	}
	return specificity
}

// deduplicateOwners merges duplicate owners by summing weights
func deduplicateOwners(owners []Owner) []Owner {
	merged := make(map[string]*Owner)
	for _, o := range owners {
		key := o.Type + ":" + o.ID
		if existing, ok := merged[key]; ok {
			existing.Weight += o.Weight
		} else {
			copy := o
			merged[key] = &copy
		}
	}

	result := make([]Owner, 0, len(merged))
	for _, o := range merged {
		result = append(result, *o)
	}
	return result
}

// ListContractsResult contains the results of listing contracts
type ListContractsResult struct {
	Contracts  []ContractSummary   `json:"contracts"`
	TotalCount int                 `json:"totalCount"`
	Staleness  FederationStaleness `json:"staleness"`
}

// ListContracts lists contracts in the federation
func (f *Federation) ListContracts(opts ListContractsOptions) (*ListContractsResult, error) {
	contracts, err := f.index.ListContracts(opts)
	if err != nil {
		return nil, err
	}

	summaries := make([]ContractSummary, len(contracts))
	for i, c := range contracts {
		summaries[i] = ContractSummary{
			ContractID:   c.ID,
			ContractType: c.ContractType,
			Visibility:   c.Visibility,
			Path:         c.Path,
			RepoID:       c.RepoID,
		}
	}

	staleness := f.computeStaleness()

	return &ListContractsResult{
		Contracts:  summaries,
		TotalCount: len(summaries),
		Staleness:  staleness,
	}, nil
}

// GetDependenciesOptions contains options for dependency query
type GetDependenciesOptions struct {
	// Federation is the federation name
	Federation string `json:"federation"`

	// RepoID is the repo to analyze
	RepoID string `json:"repoId"`

	// ModuleID optionally filters to a module
	ModuleID string `json:"moduleId,omitempty"`

	// Direction specifies which direction to query
	Direction string `json:"direction"` // consumers, dependencies, both

	// IncludeHeuristic includes tier 3 edges
	IncludeHeuristic bool `json:"includeHeuristic,omitempty"`
}

// GetDependenciesResponse contains dependency results
type GetDependenciesResponse struct {
	// Dependencies are contracts this repo depends on
	Dependencies []DependencyEntry `json:"dependencies"`

	// Consumers are repos that consume contracts from this repo
	Consumers []ConsumerEntry `json:"consumers"`

	// Staleness info
	Staleness FederationStaleness `json:"staleness"`
}

// DependencyEntry is a contract this repo depends on
type DependencyEntry struct {
	Contract     ContractSummary `json:"contract"`
	Tier         EvidenceTier    `json:"tier"`
	EvidenceType string          `json:"evidenceType"`
	Confidence   float64         `json:"confidence"`
}

// ConsumerEntry is a consumer of this repo's contracts
type ConsumerEntry struct {
	Contract     ContractSummary `json:"contract"`
	ConsumerRepo struct {
		RepoID     string  `json:"repoId"`
		Tier       string  `json:"tier"`
		Confidence float64 `json:"confidence"`
	} `json:"consumerRepo"`
}

// GetDependencies gets dependencies for a repo
func (f *Federation) GetDependencies(opts GetDependenciesOptions) (*GetDependenciesResponse, error) {
	repo := f.config.GetRepo(opts.RepoID)
	if repo == nil {
		return nil, fmt.Errorf("repository %q not found", opts.RepoID)
	}

	response := &GetDependenciesResponse{
		Dependencies: []DependencyEntry{},
		Consumers:    []ConsumerEntry{},
		Staleness:    f.computeStaleness(),
	}

	// Query edges where this repo is the consumer (dependencies)
	if opts.Direction == "dependencies" || opts.Direction == "both" {
		rows, err := f.index.db.Query(`
			SELECT e.contract_id, e.tier, e.evidence_type, e.confidence,
			       c.contract_type, c.visibility, c.path, c.repo_id
			FROM contract_edges e
			JOIN contracts c ON e.contract_id = c.id
			WHERE e.consumer_repo_uid = ? AND e.suppressed = 0
			ORDER BY e.confidence DESC
		`, repo.RepoUID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var dep DependencyEntry
			var contractID string
			if err := rows.Scan(&contractID, &dep.Tier, &dep.EvidenceType, &dep.Confidence,
				&dep.Contract.ContractType, &dep.Contract.Visibility, &dep.Contract.Path, &dep.Contract.RepoID); err != nil {
				continue
			}
			dep.Contract.ContractID = contractID
			response.Dependencies = append(response.Dependencies, dep)
		}
	}

	// Query contracts this repo provides and their consumers
	if opts.Direction == "consumers" || opts.Direction == "both" {
		// First get contracts for this repo
		contracts, err := f.index.ListContracts(ListContractsOptions{
			RepoID: opts.RepoID,
		})
		if err != nil {
			return nil, err
		}

		for _, c := range contracts {
			// Find consumers of each contract
			edges, err := f.index.FindDirectConsumers(c.ID, TierDerived)
			if err != nil {
				continue
			}

			for _, edge := range edges {
				entry := ConsumerEntry{
					Contract: ContractSummary{
						ContractID:   c.ID,
						ContractType: c.ContractType,
						Visibility:   c.Visibility,
						Path:         c.Path,
						RepoID:       c.RepoID,
					},
				}
				entry.ConsumerRepo.RepoID = edge.ConsumerRepoID
				entry.ConsumerRepo.Tier = string(edge.Tier)
				entry.ConsumerRepo.Confidence = edge.Confidence
				response.Consumers = append(response.Consumers, entry)
			}
		}
	}

	return response, nil
}

// SuppressContractEdge suppresses a contract edge
func (f *Federation) SuppressContractEdge(edgeID int64, suppressedBy string, reason string) error {
	return f.index.SuppressEdge(edgeID, suppressedBy, reason)
}

// VerifyContractEdge verifies a contract edge
func (f *Federation) VerifyContractEdge(edgeID int64, verifiedBy string) error {
	return f.index.VerifyEdge(edgeID, verifiedBy)
}

// GetContractEdge gets a contract edge by ID
func (f *Federation) GetContractEdge(edgeID int64) (*ContractEdge, error) {
	rows, err := f.index.db.Query(`
		SELECT id, edge_key, contract_id, consumer_repo_uid, consumer_repo_id, consumer_paths,
			tier, evidence_type, evidence_details, confidence, confidence_basis, detector_name, detected_at,
			suppressed, suppressed_by, suppressed_at, suppression_reason, verified, verified_by, verified_at
		FROM contract_edges
		WHERE id = ?
	`, edgeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	edges, err := scanContractEdges(rows)
	if err != nil {
		return nil, err
	}
	if len(edges) == 0 {
		return nil, nil
	}
	return &edges[0], nil
}

// GetContractEdgeByKey gets a contract edge by edge key
func (f *Federation) GetContractEdgeByKey(edgeKey string) (*ContractEdge, error) {
	rows, err := f.index.db.Query(`
		SELECT id, edge_key, contract_id, consumer_repo_uid, consumer_repo_id, consumer_paths,
			tier, evidence_type, evidence_details, confidence, confidence_basis, detector_name, detected_at,
			suppressed, suppressed_by, suppressed_at, suppression_reason, verified, verified_by, verified_at
		FROM contract_edges
		WHERE edge_key = ?
	`, edgeKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	edges, err := scanContractEdges(rows)
	if err != nil {
		return nil, err
	}
	if len(edges) == 0 {
		return nil, nil
	}
	return &edges[0], nil
}

// ContractStats provides summary stats for contracts
type ContractStats struct {
	TotalContracts    int            `json:"totalContracts"`
	PublicContracts   int            `json:"publicContracts"`
	InternalContracts int            `json:"internalContracts"`
	ByType            map[string]int `json:"byType"`
	TotalEdges        int            `json:"totalEdges"`
	DeclaredEdges     int            `json:"declaredEdges"`
	DerivedEdges      int            `json:"derivedEdges"`
}

// GetContractStats returns summary statistics
func (f *Federation) GetContractStats() (*ContractStats, error) {
	stats := &ContractStats{
		ByType: make(map[string]int),
	}

	// Count contracts
	rows, err := f.index.db.Query(`
		SELECT contract_type, visibility, COUNT(*) as cnt
		FROM contracts
		GROUP BY contract_type, visibility
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var contractType, visibility string
		var count int
		if scanErr := rows.Scan(&contractType, &visibility, &count); scanErr != nil {
			continue
		}

		stats.TotalContracts += count
		stats.ByType[contractType] += count

		if visibility == string(VisibilityPublic) {
			stats.PublicContracts += count
		} else if visibility == string(VisibilityInternal) {
			stats.InternalContracts += count
		}
	}

	// Count edges
	rows2, err := f.index.db.Query(`
		SELECT tier, COUNT(*) as cnt
		FROM contract_edges
		WHERE suppressed = 0
		GROUP BY tier
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows2.Close() }()

	for rows2.Next() {
		var tier string
		var count int
		if err := rows2.Scan(&tier, &count); err != nil {
			continue
		}

		stats.TotalEdges += count
		if tier == string(TierDeclared) {
			stats.DeclaredEdges += count
		} else if tier == string(TierDerived) {
			stats.DerivedEdges += count
		}
	}

	return stats, nil
}

// ContractWithConsumers combines a contract with its consumers
type ContractWithConsumers struct {
	Contract      ContractSummary `json:"contract"`
	ConsumerCount int             `json:"consumerCount"`
	Consumers     []Consumer      `json:"consumers,omitempty"`
}

// ListContractsWithConsumers lists contracts with their consumer counts
func (f *Federation) ListContractsWithConsumers(opts ListContractsOptions, includeConsumerDetails bool) ([]ContractWithConsumers, error) {
	contracts, err := f.index.ListContracts(opts)
	if err != nil {
		return nil, err
	}

	result := make([]ContractWithConsumers, len(contracts))
	for i, c := range contracts {
		result[i] = ContractWithConsumers{
			Contract: ContractSummary{
				ContractID:   c.ID,
				ContractType: c.ContractType,
				Visibility:   c.Visibility,
				Path:         c.Path,
				RepoID:       c.RepoID,
			},
		}

		// Get consumer count
		edges, err := f.index.FindDirectConsumers(c.ID, TierDerived)
		if err != nil {
			continue
		}
		result[i].ConsumerCount = len(edges)

		if includeConsumerDetails {
			result[i].Consumers = edgesToConsumers(edges)
		}
	}

	// Sort by consumer count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].ConsumerCount > result[j].ConsumerCount
	})

	return result, nil
}

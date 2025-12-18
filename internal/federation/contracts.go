package federation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ContractType represents the type of API contract
type ContractType string

const (
	ContractTypeProto   ContractType = "proto"
	ContractTypeOpenAPI ContractType = "openapi"
	ContractTypeGraphQL ContractType = "graphql"
)

// Visibility represents the contract visibility classification
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityInternal Visibility = "internal"
	VisibilityUnknown  Visibility = "unknown"
)

// EvidenceTier represents the quality tier of evidence
type EvidenceTier string

const (
	TierDeclared  EvidenceTier = "declared"  // Tier 1: Import statements, deps
	TierDerived   EvidenceTier = "derived"   // Tier 2: Generated code, configs
	TierHeuristic EvidenceTier = "heuristic" // Tier 3: Co-change patterns (hidden by default)
)

// Contract represents a detected API contract
type Contract struct {
	// ID is the stable contract ID: repoUID:path
	ID string `json:"id"`

	// Repository info
	RepoUID string `json:"repoUid"`
	RepoID  string `json:"repoId"`
	Path    string `json:"path"`

	// Contract type
	ContractType ContractType `json:"contractType"`

	// Parsed metadata (type-specific)
	Metadata json.RawMessage `json:"metadata"`

	// Classification
	Visibility      Visibility `json:"visibility"`
	VisibilityBasis string     `json:"visibilityBasis,omitempty"`
	Confidence      float64    `json:"confidence"`

	// Import keys for resolution
	ImportKeys []string `json:"importKeys,omitempty"`

	// Timestamps
	IndexedAt time.Time `json:"indexedAt"`
}

// ProtoMetadata contains parsed proto file metadata
type ProtoMetadata struct {
	PackageName string   `json:"packageName,omitempty"`
	Services    []string `json:"services,omitempty"`
	Imports     []string `json:"imports,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// OpenAPIMetadata contains parsed OpenAPI file metadata
type OpenAPIMetadata struct {
	Version    string   `json:"version"` // 2.0, 3.0, 3.1
	Title      string   `json:"title"`
	APIVersion string   `json:"apiVersion"`
	Servers    []string `json:"servers,omitempty"`
}

// ContractEdge represents a dependency edge between a contract and a consumer
type ContractEdge struct {
	// ID is the edge ID
	ID int64 `json:"id,omitempty"`

	// EdgeKey is the unique key for deduplication
	EdgeKey string `json:"edgeKey"`

	// The contract being consumed
	ContractID string `json:"contractId"`

	// The consumer
	ConsumerRepoUID string   `json:"consumerRepoUid"`
	ConsumerRepoID  string   `json:"consumerRepoId"`
	ConsumerPaths   []string `json:"consumerPaths"`

	// Evidence classification
	Tier            EvidenceTier    `json:"tier"`
	EvidenceType    string          `json:"evidenceType"`
	EvidenceDetails json.RawMessage `json:"evidenceDetails,omitempty"`
	Confidence      float64         `json:"confidence"`
	ConfidenceBasis string          `json:"confidenceBasis,omitempty"`

	// Detector metadata
	DetectorName string    `json:"detectorName"`
	DetectedAt   time.Time `json:"detectedAt"`

	// Manual overrides
	Suppressed         bool       `json:"suppressed,omitempty"`
	SuppressedBy       string     `json:"suppressedBy,omitempty"`
	SuppressedAt       *time.Time `json:"suppressedAt,omitempty"`
	SuppressionReason  string     `json:"suppressionReason,omitempty"`
	Verified           bool       `json:"verified,omitempty"`
	VerifiedBy         string     `json:"verifiedBy,omitempty"`
	VerifiedAt         *time.Time `json:"verifiedAt,omitempty"`
}

// ProtoImport represents an import relationship between proto contracts
type ProtoImport struct {
	ImporterContractID string `json:"importerContractId"`
	ImportedContractID string `json:"importedContractId"`
	ImportPath         string `json:"importPath"`
}

// OutgoingReference represents a detected reference from a repo to a contract
type OutgoingReference struct {
	// The consumer
	ConsumerRepoUID string `json:"consumerRepoUid"`
	ConsumerRepoID  string `json:"consumerRepoId"`
	ConsumerPath    string `json:"consumerPath"`

	// What's being referenced (will be resolved to contract ID)
	ImportKey string `json:"importKey"`

	// Evidence classification
	Tier            EvidenceTier    `json:"tier"`
	EvidenceType    string          `json:"evidenceType"`
	EvidenceDetails json.RawMessage `json:"evidenceDetails,omitempty"`
	Confidence      float64         `json:"confidence"`
	ConfidenceBasis string          `json:"confidenceBasis,omitempty"`

	// Detector metadata
	DetectorName string `json:"detectorName"`
}

// DetectorResult contains the results from a contract detector
type DetectorResult struct {
	Contracts    []Contract          `json:"contracts"`
	References   []OutgoingReference `json:"references"`
	ProtoImports []ProtoImport       `json:"protoImports,omitempty"`
}

// Detector interface for contract detection
type Detector interface {
	Name() string
	Detect(repoPath string, repoUID string, repoID string) (*DetectorResult, error)
}

// ComputeEdgeKey computes a unique key for a contract edge
func ComputeEdgeKey(contractID, consumerRepoUID, evidenceType string, consumerPaths []string) string {
	// Sort paths for determinism
	sorted := make([]string, len(consumerPaths))
	copy(sorted, consumerPaths)
	sort.Strings(sorted)

	input := strings.Join([]string{
		contractID,
		consumerRepoUID,
		evidenceType,
		strings.Join(sorted, ","),
	}, "|")

	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:16])
}

// UpsertContract inserts or updates a contract in the index
func (idx *Index) UpsertContract(c *Contract) error {
	metadataJSON, err := json.Marshal(c.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	importKeysJSON, err := json.Marshal(c.ImportKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal import keys: %w", err)
	}

	now := time.Now().Format(time.RFC3339)

	_, err = idx.db.Exec(`
		INSERT INTO contracts (id, repo_uid, repo_id, path, contract_type, metadata, visibility, visibility_basis, confidence, import_keys, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			repo_id = excluded.repo_id,
			contract_type = excluded.contract_type,
			metadata = excluded.metadata,
			visibility = excluded.visibility,
			visibility_basis = excluded.visibility_basis,
			confidence = excluded.confidence,
			import_keys = excluded.import_keys,
			indexed_at = excluded.indexed_at
	`, c.ID, c.RepoUID, c.RepoID, c.Path, string(c.ContractType),
		string(metadataJSON), string(c.Visibility), c.VisibilityBasis, c.Confidence,
		string(importKeysJSON), now)

	if err != nil {
		return fmt.Errorf("failed to upsert contract: %w", err)
	}

	// Update import keys lookup table
	_, err = idx.db.Exec("DELETE FROM contract_import_keys WHERE contract_id = ?", c.ID)
	if err != nil {
		return fmt.Errorf("failed to clear import keys: %w", err)
	}

	for _, key := range c.ImportKeys {
		_, err = idx.db.Exec(`
			INSERT OR IGNORE INTO contract_import_keys (import_key, contract_id)
			VALUES (?, ?)
		`, key, c.ID)
		if err != nil {
			// Log but continue
			continue
		}
	}

	return nil
}

// GetContract retrieves a contract by ID
func (idx *Index) GetContract(id string) (*Contract, error) {
	var c Contract
	var metadata, importKeys, visibilityBasis sql.NullString
	var indexedAt string

	err := idx.db.QueryRow(`
		SELECT id, repo_uid, repo_id, path, contract_type, metadata, visibility, visibility_basis, confidence, import_keys, indexed_at
		FROM contracts WHERE id = ?
	`, id).Scan(&c.ID, &c.RepoUID, &c.RepoID, &c.Path, &c.ContractType,
		&metadata, &c.Visibility, &visibilityBasis, &c.Confidence, &importKeys, &indexedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if metadata.Valid {
		c.Metadata = json.RawMessage(metadata.String)
	}
	c.VisibilityBasis = visibilityBasis.String
	if importKeys.Valid && importKeys.String != "" {
		json.Unmarshal([]byte(importKeys.String), &c.ImportKeys)
	}
	if t, err := time.Parse(time.RFC3339, indexedAt); err == nil {
		c.IndexedAt = t
	}

	return &c, nil
}

// ResolveImportKey resolves an import key to contract IDs
func (idx *Index) ResolveImportKey(importKey string) ([]string, error) {
	rows, err := idx.db.Query(`
		SELECT contract_id FROM contract_import_keys WHERE import_key = ?
	`, importKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contractIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		contractIDs = append(contractIDs, id)
	}

	return contractIDs, rows.Err()
}

// UpsertContractEdge inserts or updates a contract edge
func (idx *Index) UpsertContractEdge(e *ContractEdge) error {
	consumerPathsJSON, err := json.Marshal(e.ConsumerPaths)
	if err != nil {
		return fmt.Errorf("failed to marshal consumer paths: %w", err)
	}

	var evidenceDetailsJSON string
	if e.EvidenceDetails != nil {
		evidenceDetailsJSON = string(e.EvidenceDetails)
	}

	now := time.Now().Format(time.RFC3339)

	_, err = idx.db.Exec(`
		INSERT INTO contract_edges (edge_key, contract_id, consumer_repo_uid, consumer_repo_id, consumer_paths,
			tier, evidence_type, evidence_details, confidence, confidence_basis, detector_name, detected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(edge_key) DO UPDATE SET
			consumer_paths = excluded.consumer_paths,
			confidence = excluded.confidence,
			confidence_basis = excluded.confidence_basis,
			detected_at = excluded.detected_at
	`, e.EdgeKey, e.ContractID, e.ConsumerRepoUID, e.ConsumerRepoID, string(consumerPathsJSON),
		string(e.Tier), e.EvidenceType, evidenceDetailsJSON, e.Confidence, e.ConfidenceBasis,
		e.DetectorName, now)

	return err
}

// UpsertProtoImport inserts or updates a proto import relationship
func (idx *Index) UpsertProtoImport(pi *ProtoImport) error {
	_, err := idx.db.Exec(`
		INSERT OR REPLACE INTO proto_imports (importer_contract_id, imported_contract_id, import_path)
		VALUES (?, ?, ?)
	`, pi.ImporterContractID, pi.ImportedContractID, pi.ImportPath)

	return err
}

// FindDirectConsumers finds consumers of a contract
func (idx *Index) FindDirectConsumers(contractID string, includeTier EvidenceTier) ([]ContractEdge, error) {
	tierFilter := "tier IN ('declared', 'derived')"
	if includeTier == TierHeuristic {
		tierFilter = "1=1" // Include all
	}

	query := fmt.Sprintf(`
		SELECT id, edge_key, contract_id, consumer_repo_uid, consumer_repo_id, consumer_paths,
			tier, evidence_type, evidence_details, confidence, confidence_basis, detector_name, detected_at,
			suppressed, suppressed_by, suppressed_at, suppression_reason, verified, verified_by, verified_at
		FROM contract_edges
		WHERE contract_id = ? AND suppressed = 0 AND %s
		ORDER BY confidence DESC
	`, tierFilter)

	rows, err := idx.db.Query(query, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanContractEdges(rows)
}

// FindReverseImportClosure finds all contracts that import the given contract (transitively)
func (idx *Index) FindReverseImportClosure(contractID string, maxDepth int) ([]struct {
	Contract Contract
	Depth    int
}, error) {
	var result []struct {
		Contract Contract
		Depth    int
	}

	visited := map[string]bool{contractID: true}
	frontier := []string{contractID}
	depth := 1

	for len(frontier) > 0 && depth <= maxDepth {
		// Find all contracts that import any contract in frontier
		placeholders := make([]string, len(frontier))
		args := make([]interface{}, len(frontier))
		for i, id := range frontier {
			placeholders[i] = "?"
			args[i] = id
		}

		query := fmt.Sprintf(`
			SELECT DISTINCT c.*
			FROM contracts c
			JOIN proto_imports pi ON pi.importer_contract_id = c.id
			WHERE pi.imported_contract_id IN (%s)
		`, strings.Join(placeholders, ","))

		rows, err := idx.db.Query(query, args...)
		if err != nil {
			return nil, err
		}

		var newFrontier []string
		for rows.Next() {
			c, err := scanContract(rows)
			if err != nil {
				rows.Close()
				continue
			}

			if !visited[c.ID] {
				visited[c.ID] = true
				newFrontier = append(newFrontier, c.ID)
				result = append(result, struct {
					Contract Contract
					Depth    int
				}{Contract: *c, Depth: depth})
			}
		}
		rows.Close()

		frontier = newFrontier
		depth++
	}

	return result, nil
}

// ListContracts lists contracts with optional filtering
func (idx *Index) ListContracts(opts ListContractsOptions) ([]Contract, error) {
	var args []interface{}
	var whereClauses []string

	if opts.RepoID != "" {
		whereClauses = append(whereClauses, "repo_id = ?")
		args = append(args, opts.RepoID)
	}

	if opts.ContractType != "" {
		whereClauses = append(whereClauses, "contract_type = ?")
		args = append(args, opts.ContractType)
	}

	if opts.Visibility != "" {
		whereClauses = append(whereClauses, "visibility = ?")
		args = append(args, opts.Visibility)
	}

	query := `
		SELECT id, repo_uid, repo_id, path, contract_type, metadata, visibility, visibility_basis, confidence, import_keys, indexed_at
		FROM contracts
	`

	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query += " ORDER BY repo_id, path"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []Contract
	for rows.Next() {
		c, err := scanContract(rows)
		if err != nil {
			continue
		}
		contracts = append(contracts, *c)
	}

	return contracts, rows.Err()
}

// ListContractsOptions contains options for listing contracts
type ListContractsOptions struct {
	RepoID       string
	ContractType string
	Visibility   string
	Limit        int
}

// ClearContractsForRepo removes all contracts for a repo
func (idx *Index) ClearContractsForRepo(repoUID string) error {
	_, err := idx.db.Exec("DELETE FROM contracts WHERE repo_uid = ?", repoUID)
	return err
}

// ClearEdgesForConsumer removes all edges where this repo is the consumer
func (idx *Index) ClearEdgesForConsumer(repoUID string) error {
	_, err := idx.db.Exec("DELETE FROM contract_edges WHERE consumer_repo_uid = ?", repoUID)
	return err
}

// Helper to scan contract edges from rows
func scanContractEdges(rows *sql.Rows) ([]ContractEdge, error) {
	var edges []ContractEdge

	for rows.Next() {
		var e ContractEdge
		var consumerPaths, evidenceDetails, confidenceBasis sql.NullString
		var detectedAt string
		var suppressedAt, verifiedAt sql.NullString
		var suppressedBy, verifiedBy, suppressionReason sql.NullString

		if err := rows.Scan(&e.ID, &e.EdgeKey, &e.ContractID, &e.ConsumerRepoUID, &e.ConsumerRepoID,
			&consumerPaths, &e.Tier, &e.EvidenceType, &evidenceDetails, &e.Confidence, &confidenceBasis,
			&e.DetectorName, &detectedAt, &e.Suppressed, &suppressedBy, &suppressedAt, &suppressionReason,
			&e.Verified, &verifiedBy, &verifiedAt); err != nil {
			continue
		}

		if consumerPaths.Valid {
			json.Unmarshal([]byte(consumerPaths.String), &e.ConsumerPaths)
		}
		if evidenceDetails.Valid {
			e.EvidenceDetails = json.RawMessage(evidenceDetails.String)
		}
		e.ConfidenceBasis = confidenceBasis.String
		if t, err := time.Parse(time.RFC3339, detectedAt); err == nil {
			e.DetectedAt = t
		}
		e.SuppressedBy = suppressedBy.String
		e.SuppressionReason = suppressionReason.String
		e.VerifiedBy = verifiedBy.String
		if suppressedAt.Valid {
			if t, err := time.Parse(time.RFC3339, suppressedAt.String); err == nil {
				e.SuppressedAt = &t
			}
		}
		if verifiedAt.Valid {
			if t, err := time.Parse(time.RFC3339, verifiedAt.String); err == nil {
				e.VerifiedAt = &t
			}
		}

		edges = append(edges, e)
	}

	return edges, rows.Err()
}

// Helper to scan a contract from rows
func scanContract(rows *sql.Rows) (*Contract, error) {
	var c Contract
	var metadata, importKeys, visibilityBasis sql.NullString
	var indexedAt string

	if err := rows.Scan(&c.ID, &c.RepoUID, &c.RepoID, &c.Path, &c.ContractType,
		&metadata, &c.Visibility, &visibilityBasis, &c.Confidence, &importKeys, &indexedAt); err != nil {
		return nil, err
	}

	if metadata.Valid {
		c.Metadata = json.RawMessage(metadata.String)
	}
	c.VisibilityBasis = visibilityBasis.String
	if importKeys.Valid && importKeys.String != "" {
		json.Unmarshal([]byte(importKeys.String), &c.ImportKeys)
	}
	if t, err := time.Parse(time.RFC3339, indexedAt); err == nil {
		c.IndexedAt = t
	}

	return &c, nil
}

// SuppressEdge suppresses a contract edge
func (idx *Index) SuppressEdge(edgeID int64, suppressedBy, reason string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := idx.db.Exec(`
		UPDATE contract_edges
		SET suppressed = 1, suppressed_by = ?, suppressed_at = ?, suppression_reason = ?
		WHERE id = ?
	`, suppressedBy, now, reason, edgeID)
	return err
}

// VerifyEdge marks an edge as verified
func (idx *Index) VerifyEdge(edgeID int64, verifiedBy string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := idx.db.Exec(`
		UPDATE contract_edges
		SET verified = 1, verified_by = ?, verified_at = ?
		WHERE id = ?
	`, verifiedBy, now, edgeID)
	return err
}

package federation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"ckb/internal/logging"
)

// SyncResult contains the results of a sync operation
type SyncResult struct {
	// RepoID is the repository that was synced
	RepoID string `json:"repoId"`

	// RepoUID is the repository UUID
	RepoUID string `json:"repoUid"`

	// Status is the sync status
	Status string `json:"status"` // success, failed, skipped

	// Duration is how long the sync took
	Duration time.Duration `json:"duration"`

	// ModulesSynced is the number of modules synced
	ModulesSynced int `json:"modulesSynced"`

	// OwnershipSynced is the number of ownership entries synced
	OwnershipSynced int `json:"ownershipSynced"`

	// HotspotsSynced is the number of hotspots synced
	HotspotsSynced int `json:"hotspotsSynced"`

	// DecisionsSynced is the number of decisions synced
	DecisionsSynced int `json:"decisionsSynced"`

	// ContractsSynced is the number of contracts synced
	ContractsSynced int `json:"contractsSynced"`

	// ReferencesSynced is the number of contract references synced
	ReferencesSynced int `json:"referencesSynced"`

	// Error is the error message if sync failed
	Error string `json:"error,omitempty"`
}

// SyncOptions contains options for sync operations
type SyncOptions struct {
	// Force forces sync even if data is fresh
	Force bool

	// DryRun reports what would be synced without actually syncing
	DryRun bool

	// RepoIDs limits sync to specific repos (empty = all)
	RepoIDs []string
}

// Sync syncs repository data to the federation index
func (f *Federation) Sync(opts SyncOptions) ([]SyncResult, error) {
	var results []SyncResult

	repos := f.config.Repos
	if len(opts.RepoIDs) > 0 {
		// Filter to specified repos
		repos = nil
		for _, id := range opts.RepoIDs {
			if repo := f.config.GetRepo(id); repo != nil {
				repos = append(repos, *repo)
			}
		}
	}

	for _, repo := range repos {
		result := f.syncRepo(repo, opts)
		results = append(results, result)
	}

	return results, nil
}

// syncRepo syncs a single repository
func (f *Federation) syncRepo(repo RepoConfig, opts SyncOptions) SyncResult {
	startTime := time.Now()
	result := SyncResult{
		RepoID:  repo.RepoID,
		RepoUID: repo.RepoUID,
	}

	// Check schema compatibility first
	check, err := CheckSchemaCompatibility(repo.RepoID, repo.Path)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("compatibility check failed: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	if check.Status != CompatibilityOK {
		result.Status = "skipped"
		result.Error = check.Message
		result.Duration = time.Since(startTime)
		return result
	}

	if opts.DryRun {
		result.Status = "dry_run"
		result.Duration = time.Since(startTime)
		return result
	}

	// Open the repository database
	dbPath := filepath.Join(repo.Path, ".ckb", "ckb.db")
	repoDb, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to open repo database: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer repoDb.Close()

	// Clear existing data for this repo
	if err := f.index.ClearRepoData(repo.RepoUID); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to clear repo data: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Sync each data type
	var syncErr error

	result.ModulesSynced, syncErr = f.syncModules(repoDb, repo)
	if syncErr != nil && f.logger != nil {
		f.logger.Warn("Failed to sync modules", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": syncErr.Error(),
		})
	}

	result.OwnershipSynced, syncErr = f.syncOwnership(repoDb, repo)
	if syncErr != nil && f.logger != nil {
		f.logger.Warn("Failed to sync ownership", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": syncErr.Error(),
		})
	}

	result.HotspotsSynced, syncErr = f.syncHotspots(repoDb, repo)
	if syncErr != nil && f.logger != nil {
		f.logger.Warn("Failed to sync hotspots", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": syncErr.Error(),
		})
	}

	result.DecisionsSynced, syncErr = f.syncDecisions(repoDb, repo)
	if syncErr != nil && f.logger != nil {
		f.logger.Warn("Failed to sync decisions", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": syncErr.Error(),
		})
	}

	// Sync contracts (v6.3)
	result.ContractsSynced, result.ReferencesSynced, syncErr = f.syncContracts(repo)
	if syncErr != nil && f.logger != nil {
		f.logger.Warn("Failed to sync contracts", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": syncErr.Error(),
		})
	}

	// Update repo entry in index
	now := time.Now()
	tagsJSON, _ := json.Marshal(repo.Tags)
	fedRepo := &FederatedRepo{
		RepoUID:       repo.RepoUID,
		RepoID:        repo.RepoID,
		Path:          repo.Path,
		Tags:          string(tagsJSON),
		SchemaVersion: check.SchemaVersion,
		LastSyncedAt:  &now,
		Status:        "active",
		IndexedAt:     now,
	}
	if err := f.index.UpsertRepo(fedRepo); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to update repo entry: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	result.Status = "success"
	result.Duration = time.Since(startTime)

	if f.logger != nil {
		f.logger.Info("Synced repository", map[string]interface{}{
			"repo":       repo.RepoID,
			"modules":    result.ModulesSynced,
			"ownership":  result.OwnershipSynced,
			"hotspots":   result.HotspotsSynced,
			"decisions":  result.DecisionsSynced,
			"contracts":  result.ContractsSynced,
			"references": result.ReferencesSynced,
			"durationMs": result.Duration.Milliseconds(),
		})
	}

	return result
}

// syncContracts detects and syncs contracts and their references
func (f *Federation) syncContracts(repo RepoConfig) (int, int, error) {
	contractsSynced := 0
	referencesSynced := 0

	// Run proto detector
	protoDetector := NewProtoDetector()
	protoResult, err := protoDetector.Detect(repo.Path, repo.RepoUID, repo.RepoID)
	if err != nil && f.logger != nil {
		f.logger.Warn("Proto detection failed", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": err.Error(),
		})
	}
	if protoResult != nil {
		for _, contract := range protoResult.Contracts {
			if err := f.index.UpsertContract(&contract); err == nil {
				contractsSynced++
			}
		}
		referencesSynced += len(protoResult.References)

		// Store proto imports for transitivity
		for _, pi := range protoResult.ProtoImports {
			f.index.UpsertProtoImport(&pi)
		}
	}

	// Run OpenAPI detector
	openapiDetector := NewOpenAPIDetector()
	openapiResult, err := openapiDetector.Detect(repo.Path, repo.RepoUID, repo.RepoID)
	if err != nil && f.logger != nil {
		f.logger.Warn("OpenAPI detection failed", map[string]interface{}{
			"repo":  repo.RepoID,
			"error": err.Error(),
		})
	}
	if openapiResult != nil {
		for _, contract := range openapiResult.Contracts {
			if err := f.index.UpsertContract(&contract); err == nil {
				contractsSynced++
			}
		}
		referencesSynced += len(openapiResult.References)
	}

	// Resolve references and create edges
	allRefs := []OutgoingReference{}
	if protoResult != nil {
		allRefs = append(allRefs, protoResult.References...)
	}
	if openapiResult != nil {
		allRefs = append(allRefs, openapiResult.References...)
	}

	for _, ref := range allRefs {
		// Try to resolve the import key to contract IDs
		contractIDs, err := f.index.ResolveImportKey(ref.ImportKey)
		if err != nil || len(contractIDs) == 0 {
			continue
		}

		// Create edges to each resolved contract
		for _, contractID := range contractIDs {
			edge := &ContractEdge{
				EdgeKey:         ComputeEdgeKey(contractID, ref.ConsumerRepoUID, ref.EvidenceType, []string{ref.ConsumerPath}),
				ContractID:      contractID,
				ConsumerRepoUID: ref.ConsumerRepoUID,
				ConsumerRepoID:  ref.ConsumerRepoID,
				ConsumerPaths:   []string{ref.ConsumerPath},
				Tier:            ref.Tier,
				EvidenceType:    ref.EvidenceType,
				EvidenceDetails: ref.EvidenceDetails,
				Confidence:      ref.Confidence,
				ConfidenceBasis: ref.ConfidenceBasis,
				DetectorName:    ref.DetectorName,
			}
			f.index.UpsertContractEdge(edge)
		}
	}

	// Resolve proto imports to contract IDs
	if protoResult != nil {
		for _, contract := range protoResult.Contracts {
			var metadata ProtoMetadata
			if err := json.Unmarshal(contract.Metadata, &metadata); err != nil {
				continue
			}

			for _, imp := range metadata.Imports {
				importedIDs, err := f.index.ResolveImportKey(imp)
				if err != nil || len(importedIDs) == 0 {
					continue
				}

				for _, importedID := range importedIDs {
					pi := &ProtoImport{
						ImporterContractID: contract.ID,
						ImportedContractID: importedID,
						ImportPath:         imp,
					}
					f.index.UpsertProtoImport(pi)
				}
			}
		}
	}

	return contractsSynced, referencesSynced, nil
}

// syncModules syncs modules from the repo to the federation index
func (f *Federation) syncModules(repoDb *sql.DB, repo RepoConfig) (int, error) {
	// Check if modules table exists
	var count int
	err := repoDb.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='modules'").Scan(&count)
	if err != nil || count == 0 {
		return 0, nil // Table doesn't exist, skip
	}

	rows, err := repoDb.Query(`
		SELECT id, name, responsibility, owner_ref, tags, source, confidence
		FROM modules
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	now := time.Now().Format(time.RFC3339)

	for rows.Next() {
		var id, name string
		var responsibility, ownerRef, tags, source sql.NullString
		var confidence sql.NullFloat64

		if err := rows.Scan(&id, &name, &responsibility, &ownerRef, &tags, &source, &confidence); err != nil {
			continue
		}

		_, err := f.index.DB().Exec(`
			INSERT INTO federated_modules (repo_uid, repo_id, module_id, name, responsibility, owner_ref, tags, source, confidence, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.RepoUID, repo.RepoID, id, name,
			nullString(responsibility), nullString(ownerRef), nullString(tags),
			nullString(source), nullFloat(confidence), now)

		if err == nil {
			synced++
		}
	}

	// Rebuild FTS index
	f.index.DB().Exec("INSERT INTO federated_modules_fts(federated_modules_fts) VALUES('rebuild')")

	return synced, rows.Err()
}

// syncOwnership syncs ownership from the repo to the federation index
func (f *Federation) syncOwnership(repoDb *sql.DB, repo RepoConfig) (int, error) {
	// Check if ownership table exists
	var count int
	err := repoDb.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ownership'").Scan(&count)
	if err != nil || count == 0 {
		return 0, nil
	}

	rows, err := repoDb.Query(`
		SELECT pattern, owners, scope, source, confidence
		FROM ownership
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	now := time.Now().Format(time.RFC3339)

	for rows.Next() {
		var pattern, owners, scope, source string
		var confidence float64

		if err := rows.Scan(&pattern, &owners, &scope, &source, &confidence); err != nil {
			continue
		}

		_, err := f.index.DB().Exec(`
			INSERT INTO federated_ownership (repo_uid, repo_id, pattern, owners, scope, source, confidence, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.RepoUID, repo.RepoID, pattern, owners, scope, source, confidence, now)

		if err == nil {
			synced++
		}
	}

	return synced, rows.Err()
}

// syncHotspots syncs top hotspots from the repo to the federation index
func (f *Federation) syncHotspots(repoDb *sql.DB, repo RepoConfig) (int, error) {
	// Check if hotspot_snapshots table exists
	var count int
	err := repoDb.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='hotspot_snapshots'").Scan(&count)
	if err != nil || count == 0 {
		return 0, nil
	}

	// Get top 20 hotspots by score (most recent snapshot per target)
	rows, err := repoDb.Query(`
		SELECT target_id, target_type, score, churn_commits_30d, complexity_cyclomatic, coupling_instability
		FROM hotspot_snapshots
		WHERE id IN (
			SELECT MAX(id) FROM hotspot_snapshots GROUP BY target_id
		)
		ORDER BY score DESC
		LIMIT 20
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	now := time.Now().Format(time.RFC3339)

	for rows.Next() {
		var targetID, targetType string
		var score float64
		var churn sql.NullInt64
		var complexity, coupling sql.NullFloat64

		if err := rows.Scan(&targetID, &targetType, &score, &churn, &complexity, &coupling); err != nil {
			continue
		}

		_, err := f.index.DB().Exec(`
			INSERT INTO federated_hotspots (repo_uid, repo_id, target_id, target_type, score, churn_commits_30d, complexity_cyclomatic, coupling_instability, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.RepoUID, repo.RepoID, targetID, targetType, score,
			nullInt(churn), nullFloat64(complexity), nullFloat64(coupling), now)

		if err == nil {
			synced++
		}
	}

	return synced, rows.Err()
}

// syncDecisions syncs decisions from the repo to the federation index
func (f *Federation) syncDecisions(repoDb *sql.DB, repo RepoConfig) (int, error) {
	// Check if decisions table exists
	var count int
	err := repoDb.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='decisions'").Scan(&count)
	if err != nil || count == 0 {
		return 0, nil
	}

	rows, err := repoDb.Query(`
		SELECT id, title, status, affected_modules, author, created_at
		FROM decisions
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	now := time.Now().Format(time.RFC3339)

	for rows.Next() {
		var id, title, status string
		var affectedModules, author, createdAt sql.NullString

		if err := rows.Scan(&id, &title, &status, &affectedModules, &author, &createdAt); err != nil {
			continue
		}

		_, err := f.index.DB().Exec(`
			INSERT INTO federated_decisions (repo_uid, repo_id, decision_id, title, status, affected_modules, author, created_at, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, repo.RepoUID, repo.RepoID, id, title, status,
			nullString(affectedModules), nullString(author), nullString(createdAt), now)

		if err == nil {
			synced++
		}
	}

	// Rebuild FTS index
	f.index.DB().Exec("INSERT INTO federated_decisions_fts(federated_decisions_fts) VALUES('rebuild')")

	return synced, rows.Err()
}

// Helper functions for nullable values
func nullString(s sql.NullString) interface{} {
	if s.Valid {
		return s.String
	}
	return nil
}

func nullFloat(f sql.NullFloat64) interface{} {
	if f.Valid {
		return f.Float64
	}
	return nil
}

func nullFloat64(f sql.NullFloat64) interface{} {
	if f.Valid {
		return f.Float64
	}
	return nil
}

func nullInt(i sql.NullInt64) interface{} {
	if i.Valid {
		return i.Int64
	}
	return nil
}

// LogSyncResult logs a sync result
func LogSyncResult(logger *logging.Logger, result SyncResult) {
	if logger == nil {
		return
	}

	fields := map[string]interface{}{
		"repo":   result.RepoID,
		"status": result.Status,
	}

	if result.Status == "success" {
		fields["modules"] = result.ModulesSynced
		fields["ownership"] = result.OwnershipSynced
		fields["hotspots"] = result.HotspotsSynced
		fields["decisions"] = result.DecisionsSynced
		fields["durationMs"] = result.Duration.Milliseconds()
		logger.Info("Sync completed", fields)
	} else if result.Status == "failed" {
		fields["error"] = result.Error
		logger.Error("Sync failed", fields)
	} else {
		fields["reason"] = result.Error
		logger.Info("Sync skipped", fields)
	}
}

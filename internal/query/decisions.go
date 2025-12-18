package query

import (
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/decisions"
	"ckb/internal/storage"
)

// DecisionResult represents the result of a decision query
type DecisionResult struct {
	Decision *decisions.ArchitecturalDecision `json:"decision"`
	Source   string                           `json:"source"` // "file" | "database" | "both"
}

// DecisionsResult represents multiple decisions
type DecisionsResult struct {
	Decisions []*decisions.ArchitecturalDecision `json:"decisions"`
	Total     int                                `json:"total"`
	Query     *DecisionsQuery                    `json:"query,omitempty"`
}

// DecisionsQuery represents query parameters for decisions
type DecisionsQuery struct {
	Status   string `json:"status,omitempty"`
	ModuleID string `json:"moduleId,omitempty"`
	Search   string `json:"search,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// RecordDecisionInput represents input for recording a new decision
type RecordDecisionInput struct {
	Title           string   `json:"title"`
	Context         string   `json:"context"`
	Decision        string   `json:"decision"`
	Consequences    []string `json:"consequences"`
	AffectedModules []string `json:"affectedModules,omitempty"`
	Alternatives    []string `json:"alternatives,omitempty"`
	Author          string   `json:"author,omitempty"`
	Status          string   `json:"status,omitempty"` // defaults to "proposed"
}

// RecordDecision creates a new ADR and stores it in both file system and database
func (e *Engine) RecordDecision(input *RecordDecisionInput) (*DecisionResult, error) {
	// Get repository for decisions
	decisionRepo := storage.NewDecisionRepository(e.db)

	// Create parser to find next ADR number
	parser := decisions.NewParser(e.repoRoot)
	nextNum, err := parser.GetNextADRNumber()
	if err != nil {
		nextNum = 1 // fallback if no existing ADRs
	}

	// Create the ADR
	adr := decisions.NewADR(nextNum, input.Title)
	adr.Context = input.Context
	adr.Decision = input.Decision
	adr.Consequences = input.Consequences
	adr.AffectedModules = input.AffectedModules
	adr.Alternatives = input.Alternatives
	adr.Author = input.Author

	if input.Status != "" && decisions.IsValidStatus(input.Status) {
		adr.Status = input.Status
	}

	// Determine output directory
	outputDir := decisions.GetDefaultOutputDir(e.repoRoot)

	// Create writer and write the ADR file
	writer := decisions.NewWriter(e.repoRoot, outputDir)
	filePath, err := writer.CreateADR(adr)
	if err != nil {
		return nil, fmt.Errorf("failed to create ADR file: %w", err)
	}

	// Convert affected modules to JSON for storage
	affectedModulesJSON := "[]"
	if len(adr.AffectedModules) > 0 {
		bytes, err := json.Marshal(adr.AffectedModules)
		if err == nil {
			affectedModulesJSON = string(bytes)
		}
	}

	// Store in database
	record := &storage.DecisionRecord{
		ID:              adr.ID,
		Title:           adr.Title,
		Status:          adr.Status,
		AffectedModules: affectedModulesJSON,
		FilePath:        filePath,
		Author:          adr.Author,
		CreatedAt:       adr.Date,
		UpdatedAt:       adr.Date,
	}

	if err := decisionRepo.Create(record); err != nil {
		// File was created, but DB failed - log warning but don't fail
		e.logger.Warn("ADR file created but database storage failed", map[string]interface{}{
			"id":    adr.ID,
			"error": err.Error(),
		})
	}

	return &DecisionResult{
		Decision: adr,
		Source:   "file",
	}, nil
}

// GetDecision retrieves a single decision by ID
func (e *Engine) GetDecision(id string) (*DecisionResult, error) {
	decisionRepo := storage.NewDecisionRepository(e.db)

	// Try database first
	record, err := decisionRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to query decision: %w", err)
	}

	if record != nil {
		// Parse the file to get full content
		parser := decisions.NewParser(e.repoRoot)
		adr, err := parser.ParseFile(record.FilePath)
		if err == nil {
			return &DecisionResult{
				Decision: adr,
				Source:   "both",
			}, nil
		}

		// Fall back to database-only record
		adr = &decisions.ArchitecturalDecision{
			ID:       record.ID,
			Title:    record.Title,
			Status:   record.Status,
			FilePath: record.FilePath,
			Author:   record.Author,
			Date:     record.CreatedAt,
		}

		if record.AffectedModules != "" {
			_ = json.Unmarshal([]byte(record.AffectedModules), &adr.AffectedModules)
		}

		return &DecisionResult{
			Decision: adr,
			Source:   "database",
		}, nil
	}

	// Not in database - try scanning file system
	parser := decisions.NewParser(e.repoRoot)
	dirs := parser.FindADRDirectories()

	for _, dir := range dirs {
		adrs, err := parser.ParseDirectory(dir)
		if err != nil {
			continue
		}
		for _, adr := range adrs {
			if adr.ID == id {
				return &DecisionResult{
					Decision: adr,
					Source:   "file",
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("decision not found: %s", id)
}

// GetDecisions retrieves decisions matching the query
func (e *Engine) GetDecisions(query *DecisionsQuery) (*DecisionsResult, error) {
	if query == nil {
		query = &DecisionsQuery{}
	}
	if query.Limit == 0 {
		query.Limit = 50
	}

	decisionRepo := storage.NewDecisionRepository(e.db)
	var records []*storage.DecisionRecord
	var err error

	// Query based on filters
	switch {
	case query.Status != "":
		records, err = decisionRepo.GetByStatus(query.Status, query.Limit)
	case query.ModuleID != "":
		records, err = decisionRepo.GetByModule(query.ModuleID, query.Limit)
	case query.Search != "":
		records, err = decisionRepo.Search(query.Search, query.Limit)
	default:
		records, err = decisionRepo.ListAll(query.Limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}

	// If we have database results, use them
	if len(records) > 0 {
		adrs := make([]*decisions.ArchitecturalDecision, 0, len(records))
		for _, record := range records {
			adr := &decisions.ArchitecturalDecision{
				ID:       record.ID,
				Title:    record.Title,
				Status:   record.Status,
				FilePath: record.FilePath,
				Author:   record.Author,
				Date:     record.CreatedAt,
			}
			if record.AffectedModules != "" {
				_ = json.Unmarshal([]byte(record.AffectedModules), &adr.AffectedModules)
			}
			adrs = append(adrs, adr)
		}

		return &DecisionsResult{
			Decisions: adrs,
			Total:     len(adrs),
			Query:     query,
		}, nil
	}

	// Fall back to file system scan
	parser := decisions.NewParser(e.repoRoot)
	dirs := parser.FindADRDirectories()

	var allADRs []*decisions.ArchitecturalDecision
	for _, dir := range dirs {
		adrs, err := parser.ParseDirectory(dir)
		if err != nil {
			continue
		}
		allADRs = append(allADRs, adrs...)
	}

	// Apply filters
	filtered := make([]*decisions.ArchitecturalDecision, 0)
	for _, adr := range allADRs {
		if query.Status != "" && adr.Status != query.Status {
			continue
		}
		if query.ModuleID != "" {
			found := false
			for _, mod := range adr.AffectedModules {
				if mod == query.ModuleID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, adr)
		if len(filtered) >= query.Limit {
			break
		}
	}

	return &DecisionsResult{
		Decisions: filtered,
		Total:     len(filtered),
		Query:     query,
	}, nil
}

// UpdateDecisionStatus updates the status of an existing decision
func (e *Engine) UpdateDecisionStatus(id string, newStatus string) (*DecisionResult, error) {
	if !decisions.IsValidStatus(newStatus) {
		return nil, fmt.Errorf("invalid status: %s (valid: proposed, accepted, deprecated, superseded)", newStatus)
	}

	decisionRepo := storage.NewDecisionRepository(e.db)

	// Get existing record
	record, err := decisionRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get decision: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("decision not found: %s", id)
	}

	// Parse the file to get full ADR
	parser := decisions.NewParser(e.repoRoot)
	adr, err := parser.ParseFile(record.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ADR file: %w", err)
	}

	// Update status
	adr.Status = newStatus

	// Write back to file
	outputDir := decisions.GetDefaultOutputDir(e.repoRoot)
	writer := decisions.NewWriter(e.repoRoot, outputDir)
	if err := writer.UpdateADR(adr); err != nil {
		return nil, fmt.Errorf("failed to update ADR file: %w", err)
	}

	// Update database
	record.Status = newStatus
	record.UpdatedAt = time.Now()
	if err := decisionRepo.Update(record); err != nil {
		e.logger.Warn("ADR file updated but database update failed", map[string]interface{}{
			"id":    id,
			"error": err.Error(),
		})
	}

	return &DecisionResult{
		Decision: adr,
		Source:   "both",
	}, nil
}

// SyncDecisionsFromFiles scans the file system and syncs ADRs to the database
func (e *Engine) SyncDecisionsFromFiles() (int, error) {
	parser := decisions.NewParser(e.repoRoot)
	decisionRepo := storage.NewDecisionRepository(e.db)

	dirs := parser.FindADRDirectories()
	synced := 0

	for _, dir := range dirs {
		adrs, err := parser.ParseDirectory(dir)
		if err != nil {
			continue
		}

		for _, adr := range adrs {
			affectedModulesJSON := "[]"
			if len(adr.AffectedModules) > 0 {
				bytes, err := json.Marshal(adr.AffectedModules)
				if err == nil {
					affectedModulesJSON = string(bytes)
				}
			}

			record := &storage.DecisionRecord{
				ID:              adr.ID,
				Title:           adr.Title,
				Status:          adr.Status,
				AffectedModules: affectedModulesJSON,
				FilePath:        adr.FilePath,
				Author:          adr.Author,
				CreatedAt:       adr.Date,
				UpdatedAt:       time.Now(),
			}

			if err := decisionRepo.Upsert(record); err != nil {
				e.logger.Warn("Failed to sync ADR to database", map[string]interface{}{
					"id":    adr.ID,
					"error": err.Error(),
				})
				continue
			}
			synced++
		}
	}

	return synced, nil
}

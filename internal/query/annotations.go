package query

import (
	"encoding/json"
	"time"

	"ckb/internal/storage"
)

// AnnotateModuleInput represents input for annotating a module
type AnnotateModuleInput struct {
	ModuleId       string   `json:"moduleId"`
	Responsibility string   `json:"responsibility,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	PublicPaths    []string `json:"publicPaths,omitempty"`
	InternalPaths  []string `json:"internalPaths,omitempty"`
}

// AnnotateModuleResult represents the result of annotating a module
type AnnotateModuleResult struct {
	ModuleId       string      `json:"moduleId"`
	Responsibility string      `json:"responsibility,omitempty"`
	Capabilities   []string    `json:"capabilities,omitempty"`
	Tags           []string    `json:"tags,omitempty"`
	Boundaries     *Boundaries `json:"boundaries,omitempty"`
	Updated        bool        `json:"updated"`
	Created        bool        `json:"created"`
}

// Boundaries represents API boundary definitions
type Boundaries struct {
	Public   []string `json:"public,omitempty"`
	Internal []string `json:"internal,omitempty"`
}

// AnnotateModule adds or updates module metadata
func (e *Engine) AnnotateModule(input *AnnotateModuleInput) (*AnnotateModuleResult, error) {
	respRepo := storage.NewResponsibilityRepository(e.db)
	moduleRepo := storage.NewModuleRepository(e.db)

	// Check if module already has annotations
	existing, err := respRepo.GetByTarget(input.ModuleId)
	if err != nil {
		return nil, err
	}

	// Build capabilities JSON
	capabilitiesJSON := "[]"
	if len(input.Capabilities) > 0 {
		bytes, err := json.Marshal(input.Capabilities)
		if err == nil {
			capabilitiesJSON = string(bytes)
		}
	}

	now := time.Now()
	record := &storage.ResponsibilityRecord{
		TargetID:     input.ModuleId,
		TargetType:   "module",
		Summary:      input.Responsibility,
		Capabilities: capabilitiesJSON,
		Source:       "declared",
		Confidence:   1.0, // User-declared is high confidence
		UpdatedAt:    now,
	}

	created := false
	updated := false

	if existing == nil {
		// Create new record
		if err := respRepo.Create(record); err != nil {
			return nil, err
		}
		created = true
	} else {
		// Update existing record (merge values)
		record.ID = existing.ID

		// Merge responsibility
		if input.Responsibility == "" && existing.Summary != "" {
			record.Summary = existing.Summary
		}

		// Merge capabilities
		if len(input.Capabilities) == 0 && existing.Capabilities != "" {
			record.Capabilities = existing.Capabilities
		}

		if err := respRepo.Update(record); err != nil {
			return nil, err
		}
		updated = true
	}

	// Also update the modules table v2 columns (boundaries, responsibility, tags, owner_ref)
	var boundariesJSON *string
	if len(input.PublicPaths) > 0 || len(input.InternalPaths) > 0 {
		boundaries := map[string][]string{
			"public":   input.PublicPaths,
			"internal": input.InternalPaths,
		}
		if bytes, err := json.Marshal(boundaries); err == nil {
			s := string(bytes)
			boundariesJSON = &s
		}
	}

	var tagsJSON *string
	if len(input.Tags) > 0 {
		if bytes, err := json.Marshal(input.Tags); err == nil {
			s := string(bytes)
			tagsJSON = &s
		}
	}

	var responsibilityPtr *string
	if input.Responsibility != "" {
		responsibilityPtr = &input.Responsibility
	}

	// Update or create module with annotations using UpsertAnnotations
	// This ensures the module record exists and has the annotation data
	if err := moduleRepo.UpsertAnnotations(
		input.ModuleId,
		boundariesJSON,
		responsibilityPtr,
		nil, // ownerRef - not provided in input
		tagsJSON,
		"declared",
		1.0, // User-declared is high confidence
	); err != nil {
		// Log warning but don't fail - the responsibility record was already saved
		e.logger.Warn("Failed to update module annotations",
			"moduleId", input.ModuleId,
			"error", err.Error(),
		)
	}

	// Build result
	result := &AnnotateModuleResult{
		ModuleId:       input.ModuleId,
		Responsibility: input.Responsibility,
		Capabilities:   input.Capabilities,
		Tags:           input.Tags,
		Updated:        updated,
		Created:        created,
	}

	if len(input.PublicPaths) > 0 || len(input.InternalPaths) > 0 {
		result.Boundaries = &Boundaries{
			Public:   input.PublicPaths,
			Internal: input.InternalPaths,
		}
	}

	return result, nil
}

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
	ModuleId       string       `json:"moduleId"`
	Responsibility string       `json:"responsibility,omitempty"`
	Capabilities   []string     `json:"capabilities,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
	Boundaries     *Boundaries  `json:"boundaries,omitempty"`
	Updated        bool         `json:"updated"`
	Created        bool         `json:"created"`
}

// Boundaries represents API boundary definitions
type Boundaries struct {
	Public   []string `json:"public,omitempty"`
	Internal []string `json:"internal,omitempty"`
}

// AnnotateModule adds or updates module metadata
func (e *Engine) AnnotateModule(input *AnnotateModuleInput) (*AnnotateModuleResult, error) {
	respRepo := storage.NewResponsibilityRepository(e.db)

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

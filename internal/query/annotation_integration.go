package query

import (
	"encoding/json"
	"strings"

	"ckb/internal/decisions"
	"ckb/internal/storage"
)

// RelatedDecision is a lightweight representation of an ADR for embedding in responses
type RelatedDecision struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	AffectedModules []string `json:"affectedModules,omitempty"`
	FilePath        string   `json:"filePath,omitempty"`
}

// ModuleAnnotations contains declared metadata for a module
type ModuleAnnotations struct {
	Responsibility string   `json:"responsibility,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Boundaries     *struct {
		Public   []string `json:"public,omitempty"`
		Internal []string `json:"internal,omitempty"`
	} `json:"boundaries,omitempty"`
	Source     string  `json:"source"` // "declared" or "inferred"
	Confidence float64 `json:"confidence"`
}

// AnnotationContext provides annotation data for enriching tool responses
type AnnotationContext struct {
	RelatedDecisions  []RelatedDecision  `json:"relatedDecisions,omitempty"`
	ModuleAnnotations *ModuleAnnotations `json:"moduleAnnotations,omitempty"`
}

// getRelatedDecisions finds ADRs that affect the given module
func (e *Engine) getRelatedDecisions(moduleID string) []RelatedDecision {
	if e.db == nil || moduleID == "" {
		return nil
	}

	decisionRepo := storage.NewDecisionRepository(e.db)

	// Try exact match first, then prefix match for nested modules
	records, err := decisionRepo.GetByModule(moduleID, 10)
	if err != nil || len(records) == 0 {
		// Try with parent module path
		parts := strings.Split(moduleID, "/")
		for i := len(parts) - 1; i > 0; i-- {
			parentPath := strings.Join(parts[:i], "/")
			records, err = decisionRepo.GetByModule(parentPath, 10)
			if err == nil && len(records) > 0 {
				break
			}
		}
	}

	if len(records) == 0 {
		return nil
	}

	result := make([]RelatedDecision, 0, len(records))
	for _, rec := range records {
		rd := RelatedDecision{
			ID:       rec.ID,
			Title:    rec.Title,
			Status:   rec.Status,
			FilePath: rec.FilePath,
		}

		// Parse affected modules from JSON
		if rec.AffectedModules != "" {
			var modules []string
			if err := json.Unmarshal([]byte(rec.AffectedModules), &modules); err == nil {
				rd.AffectedModules = modules
			}
		}

		result = append(result, rd)
	}

	return result
}

// getModuleAnnotations retrieves declared annotations for a module
func (e *Engine) getModuleAnnotations(moduleID string) *ModuleAnnotations {
	if e.db == nil || moduleID == "" {
		return nil
	}

	moduleRepo := storage.NewModuleRepository(e.db)
	respRepo := storage.NewResponsibilityRepository(e.db)

	// Check module table for annotations (v2 columns)
	module, err := moduleRepo.GetAnnotations(moduleID)
	if err != nil {
		return nil
	}

	// Also check responsibility table for declared responsibilities
	resp, _ := respRepo.GetByTarget(moduleID)

	// No annotations found
	if module == nil && resp == nil {
		return nil
	}

	annotations := &ModuleAnnotations{
		Source:     "inferred",
		Confidence: 0.5,
	}

	// Prefer declared responsibility from responsibilities table
	if resp != nil && resp.Source == "declared" {
		annotations.Responsibility = resp.Summary
		annotations.Source = "declared"
		annotations.Confidence = resp.Confidence

		// Parse capabilities
		if resp.Capabilities != "" && resp.Capabilities != "[]" {
			var caps []string
			if err := json.Unmarshal([]byte(resp.Capabilities), &caps); err == nil {
				annotations.Capabilities = caps
			}
		}
	}

	// Get additional annotations from module table
	if module != nil {
		// Responsibility from module table (if not already set from responsibilities table)
		if annotations.Responsibility == "" && module.Responsibility != nil && *module.Responsibility != "" {
			annotations.Responsibility = *module.Responsibility
			if module.Source != "" {
				annotations.Source = module.Source
			}
			annotations.Confidence = module.Confidence
		}

		// Tags
		if module.Tags != nil && *module.Tags != "" && *module.Tags != "[]" {
			var tags []string
			if err := json.Unmarshal([]byte(*module.Tags), &tags); err == nil {
				annotations.Tags = tags
			}
		}

		// Boundaries
		if module.Boundaries != nil && *module.Boundaries != "" && *module.Boundaries != "{}" {
			var boundaries struct {
				Public   []string `json:"public"`
				Internal []string `json:"internal"`
			}
			if err := json.Unmarshal([]byte(*module.Boundaries), &boundaries); err == nil {
				if len(boundaries.Public) > 0 || len(boundaries.Internal) > 0 {
					annotations.Boundaries = &struct {
						Public   []string `json:"public,omitempty"`
						Internal []string `json:"internal,omitempty"`
					}{
						Public:   boundaries.Public,
						Internal: boundaries.Internal,
					}
				}
			}
		}
	}

	// Return nil if no meaningful annotations
	if annotations.Responsibility == "" && len(annotations.Capabilities) == 0 &&
		len(annotations.Tags) == 0 && annotations.Boundaries == nil {
		return nil
	}

	return annotations
}

// getAnnotationContext retrieves both decisions and module annotations for a module
func (e *Engine) getAnnotationContext(moduleID string) *AnnotationContext {
	decisions := e.getRelatedDecisions(moduleID)
	annotations := e.getModuleAnnotations(moduleID)

	if len(decisions) == 0 && annotations == nil {
		return nil
	}

	return &AnnotationContext{
		RelatedDecisions:  decisions,
		ModuleAnnotations: annotations,
	}
}

// getDecisionForSymbol checks if there's an ADR that mentions this symbol should be kept
// Returns (hasDecision, decisionID, decisionTitle)
//
//nolint:unused // reserved for future enhancement
func (e *Engine) getDecisionForSymbol(moduleID string, symbolName string) (bool, string, string) {
	relatedDecisions := e.getRelatedDecisions(moduleID)
	if len(relatedDecisions) == 0 {
		return false, "", ""
	}

	// For now, any decision affecting the module counts
	// In the future, we could parse decision content to look for symbol mentions
	for _, d := range relatedDecisions {
		if d.Status == "accepted" || d.Status == "proposed" {
			return true, d.ID, d.Title
		}
	}

	return false, "", ""
}

// parseDecisionFromFile reads full decision content from file
//
//nolint:unused // reserved for future enhancement
func (e *Engine) parseDecisionFromFile(filePath string) *decisions.ArchitecturalDecision {
	if filePath == "" {
		return nil
	}

	// Try parsing absolute path first (v6.0 style)
	if strings.HasPrefix(filePath, "/") {
		adr, err := decisions.ParseFileAbsolute(filePath)
		if err == nil {
			return adr
		}
	}

	// Try relative path
	parser := decisions.NewParser(e.repoRoot)
	adr, err := parser.ParseFile(filePath)
	if err == nil {
		return adr
	}

	return nil
}

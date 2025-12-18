package query

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"ckb/internal/output"
	"ckb/internal/responsibilities"
)

// GetModuleResponsibilitiesOptions controls getModuleResponsibilities behavior.
type GetModuleResponsibilitiesOptions struct {
	ModuleId     string // Specific module, or empty for all
	IncludeFiles bool   // Include file-level responsibilities
	Limit        int    // Max modules to return
}

// ModuleResponsibility represents a module's responsibility info.
type ModuleResponsibility struct {
	ModuleId     string               `json:"moduleId"`
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Summary      string               `json:"summary"`
	Capabilities []string             `json:"capabilities,omitempty"`
	Source       string               `json:"source"` // "declared" | "inferred"
	Confidence   float64              `json:"confidence"`
	Files        []FileResponsibility `json:"files,omitempty"`
}

// FileResponsibility represents a file's responsibility info.
type FileResponsibility struct {
	Path       string  `json:"path"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

// GetModuleResponsibilitiesResponse is the response for getModuleResponsibilities.
type GetModuleResponsibilitiesResponse struct {
	CkbVersion      string                 `json:"ckbVersion"`
	SchemaVersion   string                 `json:"schemaVersion"`
	Tool            string                 `json:"tool"`
	Modules         []ModuleResponsibility `json:"modules"`
	TotalCount      int                    `json:"totalCount"`
	Confidence      float64                `json:"confidence"`
	ConfidenceBasis []ConfidenceBasisItem  `json:"confidenceBasis"`
	Limitations     []string               `json:"limitations,omitempty"`
	Provenance      *Provenance            `json:"provenance,omitempty"`
	Drilldowns      []output.Drilldown     `json:"drilldowns,omitempty"`
}

// GetModuleResponsibilities returns responsibilities for modules.
func (e *Engine) GetModuleResponsibilities(ctx context.Context, opts GetModuleResponsibilitiesOptions) (*GetModuleResponsibilitiesResponse, error) {
	startTime := time.Now()

	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	var confidenceBasis []ConfidenceBasisItem
	var limitations []string
	var moduleResponsibilities []ModuleResponsibility

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, err
	}

	// Create responsibility extractor
	extractor := responsibilities.NewExtractor(e.repoRoot)

	// Get modules from architecture
	archOpts := GetArchitectureOptions{Depth: 2}
	archResp, err := e.GetArchitecture(ctx, archOpts)
	if err != nil {
		limitations = append(limitations, "Could not get architecture: "+err.Error())
	}

	confidenceBasis = append(confidenceBasis, ConfidenceBasisItem{
		Backend: "responsibility-extractor",
		Status:  "available",
	})

	// If specific module requested, filter
	if opts.ModuleId != "" && archResp != nil {
		for _, m := range archResp.Modules {
			if m.ModuleId == opts.ModuleId || m.Path == opts.ModuleId {
				resp, respErr := extractor.ExtractFromModule(m.Path)
				if respErr != nil {
					limitations = append(limitations, "Failed to extract from "+m.Path+": "+respErr.Error())
					continue
				}

				modResp := ModuleResponsibility{
					ModuleId:     m.ModuleId,
					Name:         m.Name,
					Path:         m.Path,
					Summary:      resp.Summary,
					Capabilities: resp.Capabilities,
					Source:       resp.Source,
					Confidence:   resp.Confidence,
				}

				// Include file responsibilities if requested
				if opts.IncludeFiles {
					modResp.Files = e.extractFileResponsibilities(extractor, m.Path)
				}

				moduleResponsibilities = append(moduleResponsibilities, modResp)
				break
			}
		}
	} else if archResp != nil {
		// Get all modules
		for i, m := range archResp.Modules {
			if i >= opts.Limit {
				break
			}

			resp, respErr := extractor.ExtractFromModule(m.Path)
			if respErr != nil {
				limitations = append(limitations, "Failed to extract from "+m.Path+": "+respErr.Error())
				continue
			}

			modResp := ModuleResponsibility{
				ModuleId:     m.ModuleId,
				Name:         m.Name,
				Path:         m.Path,
				Summary:      resp.Summary,
				Capabilities: resp.Capabilities,
				Source:       resp.Source,
				Confidence:   resp.Confidence,
			}

			// Include file responsibilities if requested
			if opts.IncludeFiles {
				modResp.Files = e.extractFileResponsibilities(extractor, m.Path)
			}

			moduleResponsibilities = append(moduleResponsibilities, modResp)
		}
	}

	// Compute overall confidence
	confidence := 0.69 // Base confidence for inferred
	if len(moduleResponsibilities) > 0 {
		var totalConf float64
		for _, m := range moduleResponsibilities {
			totalConf += m.Confidence
		}
		confidence = totalConf / float64(len(moduleResponsibilities))
	}

	// Build completeness
	completeness := CompletenessInfo{
		Score:  1.0,
		Reason: "responsibilities-extracted",
	}
	if len(limitations) > 0 {
		completeness.Score = 0.7
		completeness.Reason = "partial-extraction"
	}

	// Build provenance
	provenance := e.buildProvenance(repoState, "head", startTime, nil, completeness)

	// Generate drilldowns
	var drilldowns []output.Drilldown
	if len(moduleResponsibilities) > 0 {
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Get architecture",
			Query:          "getArchitecture",
			RelevanceScore: 0.8,
		})
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Get ownership",
			Query:          "getOwnership for " + moduleResponsibilities[0].Path,
			RelevanceScore: 0.7,
		})
	}

	return &GetModuleResponsibilitiesResponse{
		CkbVersion:      "6.0",
		SchemaVersion:   "6.0",
		Tool:            "getModuleResponsibilities",
		Modules:         moduleResponsibilities,
		TotalCount:      len(moduleResponsibilities),
		Confidence:      confidence,
		ConfidenceBasis: confidenceBasis,
		Limitations:     limitations,
		Provenance:      provenance,
		Drilldowns:      drilldowns,
	}, nil
}

// extractFileResponsibilities extracts responsibilities for files in a module
func (e *Engine) extractFileResponsibilities(extractor *responsibilities.Extractor, modulePath string) []FileResponsibility {
	var files []FileResponsibility

	fullPath := filepath.Join(e.repoRoot, modulePath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process source files
		name := entry.Name()
		if !isSourceFile(name) {
			continue
		}

		filePath := filepath.Join(modulePath, name)
		resp, err := extractor.ExtractFromFile(filePath)
		if err != nil {
			continue
		}

		files = append(files, FileResponsibility{
			Path:       filePath,
			Summary:    resp.Summary,
			Confidence: resp.Confidence,
		})

		// Limit files per module
		if len(files) >= 10 {
			break
		}
	}

	return files
}

// isSourceFile checks if a file is a source code file
func isSourceFile(name string) bool {
	sourceExtensions := map[string]bool{
		".go":   true,
		".ts":   true,
		".tsx":  true,
		".js":   true,
		".jsx":  true,
		".py":   true,
		".rs":   true,
		".java": true,
		".kt":   true,
		".dart": true,
	}
	ext := filepath.Ext(name)
	return sourceExtensions[ext]
}

// Package api provides language quality handlers.
package api

import (
	"net/http"
	"os"
	"time"

	"ckb/internal/project"
)

// LanguageQualityResponse represents the language quality API response.
type LanguageQualityResponse struct {
	Timestamp       time.Time                       `json:"timestamp"`
	PrimaryLanguage string                          `json:"primaryLanguage"`
	OverallQuality  string                          `json:"overallQuality"`
	TotalSymbols    int                             `json:"totalSymbols"`
	TotalRefs       int                             `json:"totalRefs"`
	TotalCallEdges  int                             `json:"totalCallEdges"`
	Summary         string                          `json:"summary"`
	Languages       map[string]*LanguageQualityInfo `json:"languages"`
}

// LanguageQualityInfo represents quality info for a single language.
type LanguageQualityInfo struct {
	DisplayName      string   `json:"displayName"`
	Tier             int      `json:"tier"`
	TierName         string   `json:"tierName"`
	Quality          string   `json:"quality"`
	SymbolCount      int      `json:"symbolCount"`
	RefCount         int      `json:"refCount"`
	CallEdgeCount    int      `json:"callEdgeCount"`
	FileCount        int      `json:"fileCount"`
	RefAccuracy      float64  `json:"refAccuracy"`
	CallGraphQuality string   `json:"callGraphQuality"`
	KnownIssues      []string `json:"knownIssues,omitempty"`
	Recommendations  []string `json:"recommendations,omitempty"`
}

// handleLanguageQuality handles GET /meta/languages - returns language quality dashboard
func (s *Server) handleLanguageQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get current working directory as project root
	root, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	assessor := project.NewQualityAssessor(root)
	report, err := assessor.GenerateReport(ctx)
	if err != nil {
		InternalError(w, "Failed to generate quality report", err)
		return
	}

	// Convert to API response
	languages := make(map[string]*LanguageQualityInfo)
	for lang, lq := range report.Languages {
		languages[string(lang)] = &LanguageQualityInfo{
			DisplayName:      lq.DisplayName,
			Tier:             int(lq.Tier),
			TierName:         lq.TierName,
			Quality:          string(lq.Quality),
			SymbolCount:      lq.SymbolCount,
			RefCount:         lq.RefCount,
			CallEdgeCount:    lq.CallEdgeCount,
			FileCount:        lq.FileCount,
			RefAccuracy:      lq.RefAccuracy,
			CallGraphQuality: string(lq.CallGraphQuality),
			KnownIssues:      lq.KnownIssues,
			Recommendations:  lq.Recommendations,
		}
	}

	response := LanguageQualityResponse{
		Timestamp:       time.Now().UTC(),
		PrimaryLanguage: string(report.PrimaryLanguage),
		OverallQuality:  string(report.OverallQuality),
		TotalSymbols:    report.TotalSymbols,
		TotalRefs:       report.TotalRefs,
		TotalCallEdges:  report.TotalCallEdges,
		Summary:         report.Summary,
		Languages:       languages,
	}

	WriteJSON(w, response, http.StatusOK)
}

// PythonEnvResponse represents Python environment detection response.
type PythonEnvResponse struct {
	Timestamp       time.Time `json:"timestamp"`
	ActiveVenv      string    `json:"activeVenv,omitempty"`
	IsActive        bool      `json:"isActive"`
	DetectedVenvs   []string  `json:"detectedVenvs,omitempty"`
	HasPyproject    bool      `json:"hasPyproject"`
	HasRequirements bool      `json:"hasRequirements"`
	HasPipfile      bool      `json:"hasPipfile"`
	Recommendation  string    `json:"recommendation,omitempty"`
}

// handlePythonEnv handles GET /meta/python-env - returns Python environment info
func (s *Server) handlePythonEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	root, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	info := project.DetectPythonEnvironment(root)

	response := PythonEnvResponse{
		Timestamp:       time.Now().UTC(),
		ActiveVenv:      info.ActiveVenv,
		IsActive:        info.IsActive,
		DetectedVenvs:   info.DetectedVenvs,
		HasPyproject:    info.HasPyproject,
		HasRequirements: info.HasRequirements,
		HasPipfile:      info.HasPipfile,
	}

	// Generate recommendation
	if !info.IsActive && len(info.DetectedVenvs) > 0 {
		response.Recommendation = "Activate virtual environment before running 'ckb index' for best results: source " + info.DetectedVenvs[0] + "/bin/activate"
	} else if !info.IsActive && len(info.DetectedVenvs) == 0 {
		if info.HasPyproject {
			response.Recommendation = "Create virtual environment with: python -m venv .venv && source .venv/bin/activate && pip install ."
		} else if info.HasRequirements {
			response.Recommendation = "Create virtual environment with: python -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"
		}
	}

	WriteJSON(w, response, http.StatusOK)
}

// TSMonorepoResponse represents TypeScript monorepo detection response.
type TSMonorepoResponse struct {
	Timestamp            time.Time `json:"timestamp"`
	IsMonorepo           bool      `json:"isMonorepo"`
	WorkspaceType        string    `json:"workspaceType,omitempty"`
	HasRootTsconfig      bool      `json:"hasRootTsconfig"`
	Packages             []string  `json:"packages,omitempty"`
	PackagesWithTsconfig []string  `json:"packagesWithTsconfig,omitempty"`
	Recommendation       string    `json:"recommendation,omitempty"`
}

// handleTSMonorepo handles GET /meta/typescript-monorepo - returns TS monorepo info
func (s *Server) handleTSMonorepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	root, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	info := project.DetectTypeScriptMonorepo(root)

	response := TSMonorepoResponse{
		Timestamp:            time.Now().UTC(),
		IsMonorepo:           info.IsMonorepo,
		WorkspaceType:        info.WorkspaceType,
		HasRootTsconfig:      info.HasRootTsconfig,
		Packages:             info.Packages,
		PackagesWithTsconfig: info.PackagesWithTsconfig,
	}

	// Generate recommendation
	if info.IsMonorepo {
		missingTsconfig := len(info.Packages) - len(info.PackagesWithTsconfig)
		if missingTsconfig > 0 {
			response.Recommendation = "Add tsconfig.json to packages for better type resolution. Use scip-typescript --infer-tsconfig for monorepos."
		} else if !info.HasRootTsconfig {
			response.Recommendation = "Consider adding root tsconfig.json with project references"
		}
	}

	WriteJSON(w, response, http.StatusOK)
}

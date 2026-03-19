package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// handleReviewPR handles GET/POST /review/pr - unified PR review with quality gates.
func (s *Server) handleReviewPR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()

	policy := query.DefaultReviewPolicy()
	opts := query.ReviewPROptions{
		BaseBranch: "main",
		Policy:     policy,
	}

	if r.Method == http.MethodGet {
		if base := r.URL.Query().Get("baseBranch"); base != "" {
			opts.BaseBranch = base
		}
		if head := r.URL.Query().Get("headBranch"); head != "" {
			opts.HeadBranch = head
		}
		if failOn := r.URL.Query().Get("failOnLevel"); failOn != "" {
			opts.Policy.FailOnLevel = failOn
		}
		// checks as comma-separated
		if checks := r.URL.Query().Get("checks"); checks != "" {
			for _, c := range parseCommaSeparated(checks) {
				if c != "" {
					opts.Checks = append(opts.Checks, c)
				}
			}
		}
		// criticalPaths as comma-separated
		if paths := r.URL.Query().Get("criticalPaths"); paths != "" {
			for _, p := range parseCommaSeparated(paths) {
				if p != "" {
					opts.Policy.CriticalPaths = append(opts.Policy.CriticalPaths, p)
				}
			}
		}
	} else {
		var req struct {
			BaseBranch    string   `json:"baseBranch"`
			HeadBranch    string   `json:"headBranch"`
			Checks        []string `json:"checks"`
			FailOnLevel   string   `json:"failOnLevel"`
			CriticalPaths []string `json:"criticalPaths"`
			// Policy overrides
			BlockBreakingChanges  *bool    `json:"blockBreakingChanges"`
			BlockSecrets          *bool    `json:"blockSecrets"`
			RequireTests       *bool    `json:"requireTests"`
			MaxRiskScore       *float64 `json:"maxRiskScore"`
			MaxComplexityDelta *int     `json:"maxComplexityDelta"`
			MaxFiles           *int     `json:"maxFiles"`
		}
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				WriteError(w, err, http.StatusBadRequest)
				return
			}
		}
		if req.BaseBranch != "" {
			opts.BaseBranch = req.BaseBranch
		}
		if req.HeadBranch != "" {
			opts.HeadBranch = req.HeadBranch
		}
		if len(req.Checks) > 0 {
			opts.Checks = req.Checks
		}
		if req.FailOnLevel != "" {
			opts.Policy.FailOnLevel = req.FailOnLevel
		}
		if len(req.CriticalPaths) > 0 {
			opts.Policy.CriticalPaths = req.CriticalPaths
		}
		if req.BlockBreakingChanges != nil {
			opts.Policy.BlockBreakingChanges = *req.BlockBreakingChanges
		}
		if req.BlockSecrets != nil {
			opts.Policy.BlockSecrets = *req.BlockSecrets
		}
		if req.RequireTests != nil {
			opts.Policy.RequireTests = *req.RequireTests
		}
		if req.MaxRiskScore != nil {
			opts.Policy.MaxRiskScore = *req.MaxRiskScore
		}
		if req.MaxComplexityDelta != nil {
			opts.Policy.MaxComplexityDelta = *req.MaxComplexityDelta
		}
		if req.MaxFiles != nil {
			opts.Policy.MaxFiles = *req.MaxFiles
		}
	}

	resp, err := s.engine.ReviewPR(ctx, opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, resp, http.StatusOK)
}

// parseCommaSeparated splits a comma-separated string and trims whitespace.
func parseCommaSeparated(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

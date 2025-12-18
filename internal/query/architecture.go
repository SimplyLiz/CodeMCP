package query

import (
	"context"
	"fmt"
	"sort"
	"time"

	"ckb/internal/architecture"
	"ckb/internal/compression"
	"ckb/internal/errors"
	"ckb/internal/jobs"
	"ckb/internal/modules"
	"ckb/internal/output"
)

// GetArchitectureOptions contains options for getArchitecture.
type GetArchitectureOptions struct {
	Depth               int
	IncludeExternalDeps bool
	Refresh             bool
}

// GetArchitectureResponse is the response for getArchitecture.
type GetArchitectureResponse struct {
	Modules         []ModuleSummary       `json:"modules"`
	DependencyGraph []DependencyEdge      `json:"dependencyGraph"`
	Entrypoints     []Entrypoint          `json:"entrypoints"`
	Truncated       bool                  `json:"truncated,omitempty"`
	TruncationInfo  *TruncationInfo       `json:"truncationInfo,omitempty"`
	Provenance      *Provenance           `json:"provenance"`
	Drilldowns      []output.Drilldown    `json:"drilldowns,omitempty"`
	Confidence      float64               `json:"confidence"`
	ConfidenceBasis []ConfidenceBasisItem `json:"confidenceBasis"`
	Limitations     []string              `json:"limitations,omitempty"`
}

// ModuleSummary describes a module in the architecture.
type ModuleSummary struct {
	ModuleId      string `json:"moduleId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Language      string `json:"language,omitempty"`
	SymbolCount   int    `json:"symbolCount"`
	FileCount     int    `json:"fileCount"`
	ExportedCount int    `json:"exportedCount,omitempty"`
	IncomingEdges int    `json:"incomingEdges"`
	OutgoingEdges int    `json:"outgoingEdges"`
	IsEntrypoint  bool   `json:"isEntrypoint,omitempty"`
}

// DependencyEdge represents a dependency between modules.
type DependencyEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"` // local-file, local-module, workspace-package, external-dependency, stdlib
	Strength int    `json:"strength"`
}

// Entrypoint represents an entry point in the codebase.
type Entrypoint struct {
	ModuleId string `json:"moduleId"`
	FileId   string `json:"fileId"`
	Kind     string `json:"kind"` // main, test, script, api
	Name     string `json:"name,omitempty"`
}

// GetArchitecture returns the codebase architecture.
// v5.2 compliant with hard caps: max 20 modules, 50 edges
func (e *Engine) GetArchitecture(ctx context.Context, opts GetArchitectureOptions) (*GetArchitectureResponse, error) {
	startTime := time.Now()

	// v5.2 hard caps
	const maxModules = 20
	const maxEdges = 50
	const minEdgeStrength = 1 // Minimum strength to keep an edge

	// Default options
	if opts.Depth <= 0 {
		opts.Depth = 2
	}

	var confidenceBasis []ConfidenceBasisItem
	var limitations []string

	// Get repo state (full mode for architecture)
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Create import scanner for the architecture generator
	importScanner := modules.NewImportScanner(&e.config.ImportScan, e.logger)

	// Create architecture generator
	generator := architecture.NewArchitectureGenerator(e.repoRoot, e.config, importScanner, e.logger)

	// Build generator options
	genOpts := &architecture.GeneratorOptions{
		Depth:               opts.Depth,
		IncludeExternalDeps: opts.IncludeExternalDeps,
		Refresh:             opts.Refresh,
	}

	// Generate architecture
	arch, err := generator.Generate(ctx, repoState.RepoStateId, genOpts)
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	confidenceBasis = append(confidenceBasis, ConfidenceBasisItem{
		Backend: "scip",
		Status:  "available",
	})

	// Convert to response format
	moduleSummaries := convertModuleSummaries(arch.Modules)
	edges := convertArchEdges(arch.DependencyGraph, opts.IncludeExternalDeps)
	entrypoints := convertArchEntrypoints(arch.Entrypoints)

	// Enrich module summaries with symbol counts from SCIP
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		for i := range moduleSummaries {
			// Count symbols for this module's path prefix
			symbolCount := e.scipAdapter.CountSymbolsByPath(moduleSummaries[i].Path)
			moduleSummaries[i].SymbolCount = symbolCount
		}
	}

	// Compute edge counts for modules
	computeEdgeCounts(moduleSummaries, edges)

	// Sort modules by impact (incoming edges DESC) with deterministic tie-breaker
	sort.Slice(moduleSummaries, func(i, j int) bool {
		if moduleSummaries[i].IncomingEdges != moduleSummaries[j].IncomingEdges {
			return moduleSummaries[i].IncomingEdges > moduleSummaries[j].IncomingEdges
		}
		if moduleSummaries[i].SymbolCount != moduleSummaries[j].SymbolCount {
			return moduleSummaries[i].SymbolCount > moduleSummaries[j].SymbolCount
		}
		return moduleSummaries[i].ModuleId < moduleSummaries[j].ModuleId
	})

	// v5.2: Prune edges - keep only those with strength >= minEdgeStrength
	originalEdgeCount := len(edges)
	prunedEdges := make([]DependencyEdge, 0, len(edges))
	for _, edge := range edges {
		if edge.Strength >= minEdgeStrength {
			prunedEdges = append(prunedEdges, edge)
		}
	}
	edges = prunedEdges

	// v5.2: Sort edges by strength DESC, then lexical tie-breaker
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Strength != edges[j].Strength {
			return edges[i].Strength > edges[j].Strength
		}
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// v5.2: Apply edge cap
	var truncationInfo *TruncationInfo
	if len(edges) > maxEdges {
		limitations = append(limitations, "Edge count exceeded; showing top 50 by strength")
		edges = edges[:maxEdges]
	}

	// v5.2: Apply module cap
	if len(moduleSummaries) > maxModules {
		truncationInfo = &TruncationInfo{
			Reason:        "max-modules",
			OriginalCount: len(moduleSummaries),
			ReturnedCount: maxModules,
		}
		limitations = append(limitations, "Module count exceeded; showing top 20 by impact")
		moduleSummaries = moduleSummaries[:maxModules]
	}

	// Track if we pruned edges
	if originalEdgeCount > len(edges) && len(limitations) == 0 {
		limitations = append(limitations, "Some weak edges pruned")
	}

	// Compute confidence
	confidence := 0.89 // Partial static analysis (SCIP available)
	if len(limitations) > 0 {
		confidence = 0.79 // With limitations
	}

	// Build completeness
	completeness := CompletenessInfo{
		Score:  1.0,
		Reason: "full-backend",
	}

	// Build provenance
	provenance := e.buildProvenance(repoState, "full", startTime, nil, completeness)

	// Generate drilldowns
	var compTrunc *compression.TruncationInfo
	if truncationInfo != nil {
		compTrunc = &compression.TruncationInfo{
			Reason:        compression.TruncMaxModules,
			OriginalCount: truncationInfo.OriginalCount,
			ReturnedCount: truncationInfo.ReturnedCount,
		}
	}

	var topModule *output.Module
	if len(moduleSummaries) > 0 {
		topModule = &output.Module{
			ModuleId: moduleSummaries[0].ModuleId,
			Name:     moduleSummaries[0].Name,
		}
	}

	drilldowns := e.generateDrilldowns(compTrunc, completeness, "", topModule)

	return &GetArchitectureResponse{
		Modules:         moduleSummaries,
		DependencyGraph: edges,
		Entrypoints:     entrypoints,
		Truncated:       truncationInfo != nil,
		TruncationInfo:  truncationInfo,
		Provenance:      provenance,
		Drilldowns:      drilldowns,
		Confidence:      confidence,
		ConfidenceBasis: confidenceBasis,
		Limitations:     limitations,
	}, nil
}

// convertModuleSummaries converts architecture module summaries to response format.
func convertModuleSummaries(archModules []architecture.ModuleSummary) []ModuleSummary {
	result := make([]ModuleSummary, 0, len(archModules))

	for _, m := range archModules {
		result = append(result, ModuleSummary{
			ModuleId:    m.ModuleId,
			Name:        m.Name,
			Path:        m.RootPath,
			Language:    m.Language,
			SymbolCount: m.SymbolCount,
			FileCount:   m.FileCount,
		})
	}

	return result
}

// convertArchEdges converts architecture dependency edges to response format.
func convertArchEdges(archEdges []architecture.DependencyEdge, includeExternal bool) []DependencyEdge {
	edges := make([]DependencyEdge, 0, len(archEdges))

	for _, edge := range archEdges {
		// Filter external dependencies if not requested
		kindStr := string(edge.Kind)
		if !includeExternal && kindStr == "external-dependency" {
			continue
		}

		edges = append(edges, DependencyEdge{
			From:     edge.From,
			To:       edge.To,
			Kind:     kindStr,
			Strength: edge.Strength,
		})
	}

	return edges
}

// convertArchEntrypoints converts architecture entrypoints to response format.
func convertArchEntrypoints(archEntrypoints []architecture.Entrypoint) []Entrypoint {
	entrypoints := make([]Entrypoint, 0, len(archEntrypoints))

	for _, ep := range archEntrypoints {
		entrypoints = append(entrypoints, Entrypoint{
			ModuleId: ep.ModuleId,
			FileId:   ep.FileId,
			Kind:     ep.Kind,
			Name:     ep.Name,
		})
	}

	return entrypoints
}

// computeEdgeCounts updates modules with edge counts.
func computeEdgeCounts(modules []ModuleSummary, edges []DependencyEdge) {
	incoming := make(map[string]int)
	outgoing := make(map[string]int)

	for _, edge := range edges {
		outgoing[edge.From]++
		incoming[edge.To]++
	}

	for i := range modules {
		modules[i].IncomingEdges = incoming[modules[i].ModuleId]
		modules[i].OutgoingEdges = outgoing[modules[i].ModuleId]
	}
}

// v6.0 Architectural Memory - RefreshArchitecture

// RefreshArchitectureOptions contains options for refreshArchitecture.
type RefreshArchitectureOptions struct {
	// Scope determines what to refresh: "all", "modules", "ownership", "hotspots", "responsibilities"
	Scope string

	// Force refresh even if data is fresh
	Force bool

	// DryRun previews changes without making them
	DryRun bool

	// Async runs the refresh in the background and returns immediately with a job ID
	Async bool
}

// RefreshArchitectureChanges tracks what was changed during refresh.
type RefreshArchitectureChanges struct {
	ModulesUpdated          int `json:"modulesUpdated,omitempty"`
	ModulesCreated          int `json:"modulesCreated,omitempty"`
	OwnershipUpdated        int `json:"ownershipUpdated,omitempty"`
	HotspotsUpdated         int `json:"hotspotsUpdated,omitempty"`
	ResponsibilitiesUpdated int `json:"responsibilitiesUpdated,omitempty"`
}

// RefreshArchitectureResponse is the response for refreshArchitecture.
type RefreshArchitectureResponse struct {
	CkbVersion    string                      `json:"ckbVersion"`
	SchemaVersion string                      `json:"schemaVersion"`
	Tool          string                      `json:"tool"`
	Status        string                      `json:"status"` // "completed", "skipped", "queued"
	Scope         string                      `json:"scope"`
	Changes       *RefreshArchitectureChanges `json:"changes,omitempty"`
	DurationMs    int64                       `json:"durationMs,omitempty"`
	DryRun        bool                        `json:"dryRun,omitempty"`
	JobId         string                      `json:"jobId,omitempty"` // Set when Async=true
	Warnings      []string                    `json:"warnings,omitempty"`
	Provenance    *Provenance                 `json:"provenance,omitempty"`
}

// RefreshArchitecture rebuilds the architectural model from sources.
// This is a v6.0 heavy operation (up to 30s) that refreshes modules, ownership,
// hotspots, and/or responsibilities based on the specified scope.
func (e *Engine) RefreshArchitecture(ctx context.Context, opts RefreshArchitectureOptions) (*RefreshArchitectureResponse, error) {
	startTime := time.Now()

	// Default scope
	if opts.Scope == "" {
		opts.Scope = "all"
	}

	// Validate scope
	validScopes := map[string]bool{
		"all":              true,
		"modules":          true,
		"ownership":        true,
		"hotspots":         true,
		"responsibilities": true,
	}
	if !validScopes[opts.Scope] {
		return nil, e.wrapError(nil, errors.ScopeInvalid)
	}

	// Handle async mode - queue job and return immediately
	if opts.Async {
		return e.queueRefreshJob(opts)
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	changes := &RefreshArchitectureChanges{}
	var warnings []string

	// If dry run, just return what would be refreshed
	if opts.DryRun {
		return &RefreshArchitectureResponse{
			CkbVersion:    "6.0",
			SchemaVersion: "6.0",
			Tool:          "refreshArchitecture",
			Status:        "skipped",
			Scope:         opts.Scope,
			Changes:       changes,
			DurationMs:    time.Since(startTime).Milliseconds(),
			DryRun:        true,
			Warnings:      []string{"Dry run - no changes made"},
			Provenance: &Provenance{
				RepoStateId:     repoState.RepoStateId,
				RepoStateDirty:  repoState.Dirty,
				QueryDurationMs: time.Since(startTime).Milliseconds(),
			},
		}, nil
	}

	// Refresh modules if requested
	if opts.Scope == "all" || opts.Scope == "modules" {
		// Re-detect modules
		importScanner := modules.NewImportScanner(&e.config.ImportScan, e.logger)
		generator := architecture.NewArchitectureGenerator(e.repoRoot, e.config, importScanner, e.logger)

		genOpts := &architecture.GeneratorOptions{
			Refresh: true,
		}

		_, genErr := generator.Generate(ctx, repoState.RepoStateId, genOpts)
		if genErr != nil {
			warnings = append(warnings, "Module refresh had errors: "+genErr.Error())
		} else {
			changes.ModulesUpdated = 1 // Placeholder - would count actual changes
		}
	}

	// Refresh ownership if requested
	if opts.Scope == "all" || opts.Scope == "ownership" {
		// TODO: Implement CODEOWNERS parsing and git-blame ownership
		// For now, just mark as placeholder
		warnings = append(warnings, "Ownership refresh not yet implemented")
	}

	// Refresh hotspots if requested
	if opts.Scope == "all" || opts.Scope == "hotspots" {
		// TODO: Implement hotspot snapshot persistence
		// For now, just mark as placeholder
		warnings = append(warnings, "Hotspot persistence not yet implemented")
	}

	// Refresh responsibilities if requested
	if opts.Scope == "all" || opts.Scope == "responsibilities" {
		// TODO: Implement responsibility extraction
		// For now, just mark as placeholder
		warnings = append(warnings, "Responsibility extraction not yet implemented")
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &RefreshArchitectureResponse{
		CkbVersion:    "6.0",
		SchemaVersion: "6.0",
		Tool:          "refreshArchitecture",
		Status:        "completed",
		Scope:         opts.Scope,
		Changes:       changes,
		DurationMs:    durationMs,
		Warnings:      warnings,
		Provenance: &Provenance{
			RepoStateId:     repoState.RepoStateId,
			RepoStateDirty:  repoState.Dirty,
			QueryDurationMs: durationMs,
		},
	}, nil
}

// queueRefreshJob creates a job for async refresh and returns immediately.
func (e *Engine) queueRefreshJob(opts RefreshArchitectureOptions) (*RefreshArchitectureResponse, error) {
	if e.jobRunner == nil {
		return nil, e.wrapError(
			fmt.Errorf("job runner not available"),
			errors.BackendUnavailable,
		)
	}

	// Create job with scope
	scope := &jobs.RefreshScope{
		Scope: opts.Scope,
		Force: opts.Force,
	}

	job, err := jobs.NewJob(jobs.JobTypeRefreshArchitecture, scope)
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Submit to runner
	if err := e.jobRunner.Submit(job); err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	return &RefreshArchitectureResponse{
		CkbVersion:    "6.0",
		SchemaVersion: "6.0",
		Tool:          "refreshArchitecture",
		Status:        "queued",
		Scope:         opts.Scope,
		JobId:         job.ID,
		Warnings:      []string{"Job queued for async processing"},
	}, nil
}

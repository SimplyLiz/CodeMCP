package query

import (
	"context"
	"sort"
	"time"

	"ckb/internal/architecture"
	"ckb/internal/compression"
	"ckb/internal/errors"
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

package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ckb/internal/backends/git"
	"ckb/internal/output"
)

// AINavigationMeta captures common response metadata aligned with the navigation spec.
type AINavigationMeta struct {
	CkbVersion    string             `json:"ckbVersion"`
	SchemaVersion int                `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Resolved      *ResolvedTarget    `json:"resolved,omitempty"`
	Truncation    *TruncationInfo    `json:"truncation,omitempty"`
	Drilldowns    []output.Drilldown `json:"drilldowns,omitempty"`
	Provenance    *Provenance        `json:"provenance,omitempty"`
}

// ResolvedTarget mirrors the resolution summary in the navigation spec.
type ResolvedTarget struct {
	SymbolId     string  `json:"symbolId,omitempty"`
	ResolvedFrom string  `json:"resolvedFrom,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
}

// ExplainSymbolOptions controls explainSymbol behavior.
type ExplainSymbolOptions struct {
	SymbolId string
}

// ExplainSymbolResponse provides an AI-navigation friendly symbol overview.
type ExplainSymbolResponse struct {
	AINavigationMeta
	Facts   ExplainSymbolFacts   `json:"facts"`
	Summary ExplainSymbolSummary `json:"summary"`
}

// ExplainSymbolFacts mirrors the CKB navigation contract at a simplified level.
type ExplainSymbolFacts struct {
	Symbol   *SymbolInfo         `json:"symbol,omitempty"`
	Usage    *ExplainUsage       `json:"usage,omitempty"`
	History  *ExplainHistory     `json:"history,omitempty"`
	Flags    *ExplainSymbolFlags `json:"flags,omitempty"`
	Callers  []ExplainCaller     `json:"callers,omitempty"`
	Callees  []ExplainCallee     `json:"callees,omitempty"`
	Module   string              `json:"module,omitempty"`
	Warnings []string            `json:"warnings,omitempty"`
}

// ExplainUsage captures high level usage stats.
type ExplainUsage struct {
	CallerCount    int `json:"callerCount"`
	CalleeCount    int `json:"calleeCount"`
	ReferenceCount int `json:"referenceCount"`
	ModuleCount    int `json:"moduleCount"`
}

// ExplainHistory captures git derived history.
type ExplainHistory struct {
	CreatedAt       string `json:"createdAt,omitempty"`
	LastModifiedAt  string `json:"lastModifiedAt,omitempty"`
	CommitCount     int    `json:"commitCount,omitempty"`
	CommitFrequency string `json:"commitFrequency,omitempty"`
}

// ExplainSymbolFlags encodes quick status bits.
type ExplainSymbolFlags struct {
	IsPublicApi  bool `json:"isPublicApi"`
	IsExported   bool `json:"isExported"`
	IsEntrypoint bool `json:"isEntrypoint"`
	HasTests     bool `json:"hasTests"`
}

// ExplainCaller represents a caller evidence item.
type ExplainCaller struct {
	FileId  string `json:"fileId"`
	Line    int    `json:"line"`
	Kind    string `json:"kind"`
	IsTest  bool   `json:"isTest"`
	Context string `json:"context,omitempty"`
}

// ExplainCallee is a placeholder for future expansion.
type ExplainCallee struct {
	SymbolId string `json:"symbolId"`
}

// ExplainSymbolSummary provides condensed text.
type ExplainSymbolSummary struct {
	Tldr     string `json:"tldr"`
	Identity string `json:"identity"`
	Usage    string `json:"usage"`
	History  string `json:"history"`
}

// JustifySymbolOptions controls justification logic.
type JustifySymbolOptions struct {
	SymbolId string
}

// JustifySymbolResponse returns a verdict-like assessment.
type JustifySymbolResponse struct {
	AINavigationMeta
	Facts      *ExplainSymbolFacts `json:"facts"`
	Verdict    string              `json:"verdict"`
	Confidence float64             `json:"confidence"`
	Reasoning  string              `json:"reasoning"`
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	SymbolId  string
	Direction string // "callers", "callees", or "both"
	Depth     int
}

// CallGraphResponse contains a lightweight call graph.
type CallGraphResponse struct {
	AINavigationMeta
	Root  string          `json:"root"`
	Nodes []CallGraphNode `json:"nodes"`
	Edges []CallGraphEdge `json:"edges"`
}

// CallGraphNode captures a node in the call graph.
type CallGraphNode struct {
	ID       string        `json:"id"`
	SymbolId string        `json:"symbolId,omitempty"`
	Name     string        `json:"name"`
	Location *LocationInfo `json:"location,omitempty"`
	Depth    int           `json:"depth"`
	Role     string        `json:"role"` // "root", "caller", "callee"
	Score    float64       `json:"score"`
}

// CallGraphEdge encodes a caller->callee relationship.
type CallGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ModuleOverviewOptions controls module overview behavior.
type ModuleOverviewOptions struct {
	Path string
	Name string
}

// ModuleOverviewResponse returns coarse module facts.
type ModuleOverviewResponse struct {
	AINavigationMeta
	Module        ModuleOverviewModule `json:"module"`
	Size          ModuleSize           `json:"size"`
	RecentCommits []string             `json:"recentCommits,omitempty"`
}

// ModuleOverviewModule contains module identity.
type ModuleOverviewModule struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ModuleSize contains basic size stats.
type ModuleSize struct {
	FileCount   int `json:"fileCount"`
	SymbolCount int `json:"symbolCount"`
}

// ExplainSymbol provides an opinionated overview of a symbol using available backends.
func (e *Engine) ExplainSymbol(ctx context.Context, opts ExplainSymbolOptions) (*ExplainSymbolResponse, error) {
	startTime := time.Now()

	// Reuse GetSymbol for symbol identity and provenance
	symbolResp, err := e.GetSymbol(ctx, GetSymbolOptions{SymbolId: opts.SymbolId, RepoStateMode: "full"})
	if err != nil {
		return nil, err
	}

	facts := ExplainSymbolFacts{}
	if symbolResp.Symbol != nil {
		facts.Symbol = symbolResp.Symbol
		facts.Flags = &ExplainSymbolFlags{
			IsPublicApi: strings.EqualFold(symbolResp.Symbol.Visibility.Visibility, "public"),
			IsExported:  strings.EqualFold(symbolResp.Symbol.Visibility.Visibility, "public"),
		}
		facts.Module = symbolResp.Symbol.ModuleId
	}

	// Collect reference data for usage/callers
	refResp, err := e.FindReferences(ctx, FindReferencesOptions{SymbolId: opts.SymbolId, IncludeTests: true, Limit: 200})
	if err == nil && refResp != nil {
		callers := make([]ExplainCaller, 0, len(refResp.References))
		moduleSet := map[string]struct{}{}
		hasTests := false

		for _, ref := range refResp.References {
			moduleName := topLevelModule(ref.Location.FileId)
			moduleSet[moduleName] = struct{}{}
			isCall := strings.Contains(strings.ToLower(ref.Kind), "call")
			if isCall {
				callers = append(callers, ExplainCaller{
					FileId:  ref.Location.FileId,
					Line:    ref.Location.StartLine,
					Kind:    ref.Kind,
					IsTest:  ref.IsTest,
					Context: ref.Context,
				})
			}
			if ref.IsTest {
				hasTests = true
			}
		}

		facts.Callers = callers
		facts.Usage = &ExplainUsage{
			CallerCount:    len(callers),
			CalleeCount:    len(facts.Callees),
			ReferenceCount: len(refResp.References),
			ModuleCount:    len(moduleSet),
		}

		if facts.Flags != nil {
			facts.Flags.HasTests = hasTests
		}

		if len(facts.Callees) == 0 {
			facts.Warnings = append(facts.Warnings, "callee analysis not yet implemented")
		}
	}

	// Compute history from git using definition path when available
	if facts.Symbol != nil && facts.Symbol.Location != nil && e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		history, err := e.gitAdapter.GetFileHistory(facts.Symbol.Location.FileId, 20)
		if err == nil {
			facts.History = &ExplainHistory{
				CreatedAt:       tailTimestamp(history.Commits),
				LastModifiedAt:  history.LastModified,
				CommitCount:     history.CommitCount,
				CommitFrequency: classifyCommitFrequency(history.CommitCount),
			}
		}
	}

	summary := buildExplainSummary(facts)

	prov := symbolResp.Provenance
	if prov != nil {
		prov.QueryDurationMs = time.Since(startTime).Milliseconds()
	}

	resolved := &ResolvedTarget{}
	if facts.Symbol != nil {
		resolved.SymbolId = facts.Symbol.StableId
		resolved.ResolvedFrom = "id"
		resolved.Confidence = 1.0
	}

	var truncation *TruncationInfo
	if refResp != nil && refResp.Truncated {
		truncation = refResp.TruncationInfo
		if truncation == nil {
			truncation = &TruncationInfo{
				Reason:        "limit",
				OriginalCount: refResp.TotalCount,
				ReturnedCount: len(refResp.References),
			}
		}
	}

	return &ExplainSymbolResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    "0.1.0",
			SchemaVersion: 1,
			Tool:          "explainSymbol",
			Resolved:      resolved,
			Truncation:    truncation,
			Drilldowns:    symbolResp.Drilldowns,
			Provenance:    prov,
		},
		Facts:   facts,
		Summary: summary,
	}, nil
}

// buildExplainSummary constructs summary text from facts.
func buildExplainSummary(facts ExplainSymbolFacts) ExplainSymbolSummary {
	summary := ExplainSymbolSummary{}

	if facts.Symbol != nil {
		summary.Identity = fmt.Sprintf("%s %s in module %s", facts.Symbol.Kind, facts.Symbol.Name, facts.Symbol.ModuleId)
	}
	if facts.Usage != nil {
		summary.Usage = fmt.Sprintf("%d callers, %d references across %d modules", facts.Usage.CallerCount, facts.Usage.ReferenceCount, facts.Usage.ModuleCount)
	}
	if facts.History != nil {
		summary.History = fmt.Sprintf("%d commits, last modified %s", facts.History.CommitCount, facts.History.LastModifiedAt)
	}

	parts := []string{}
	if summary.Identity != "" {
		parts = append(parts, summary.Identity)
	}
	if summary.Usage != "" {
		parts = append(parts, summary.Usage)
	}
	summary.Tldr = strings.TrimSpace(strings.Join(parts, " – "))

	return summary
}

// JustifySymbol applies simple heuristics using explainSymbol facts.
func (e *Engine) JustifySymbol(ctx context.Context, opts JustifySymbolOptions) (*JustifySymbolResponse, error) {
	explain, err := e.ExplainSymbol(ctx, ExplainSymbolOptions{SymbolId: opts.SymbolId})
	if err != nil {
		return nil, err
	}

	verdict, confidence, reasoning := computeJustifyVerdict(explain.Facts)

	return &JustifySymbolResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    "0.1.0",
			SchemaVersion: 1,
			Tool:          "justifySymbol",
			Resolved:      explain.Resolved,
			Truncation:    explain.Truncation,
			Drilldowns:    explain.Drilldowns,
			Provenance:    explain.Provenance,
		},
		Facts:      &explain.Facts,
		Verdict:    verdict,
		Confidence: confidence,
		Reasoning:  reasoning,
	}, nil
}

// computeJustifyVerdict encapsulates verdict selection logic for unit testing.
func computeJustifyVerdict(facts ExplainSymbolFacts) (verdict string, confidence float64, reasoning string) {
	// Has active callers -> keep
	if facts.Usage != nil && facts.Usage.CallerCount > 0 {
		return "keep", 0.9, fmt.Sprintf("Active callers detected (%d)", facts.Usage.CallerCount)
	}

	// Public API with no callers -> investigate
	if facts.Flags != nil && facts.Flags.IsPublicApi {
		return "investigate", 0.6, "Public API but no callers found"
	}

	// No callers, not public -> remove candidate
	return "remove-candidate", 0.7, "No callers found"
}

// GetCallGraph builds a shallow caller graph using reference data.
func (e *Engine) GetCallGraph(ctx context.Context, opts CallGraphOptions) (*CallGraphResponse, error) {
	startTime := time.Now()

	if opts.Depth == 0 {
		opts.Depth = 1
	}
	if opts.Direction == "" {
		opts.Direction = "both"
	}

	symbolResp, err := e.GetSymbol(ctx, GetSymbolOptions{SymbolId: opts.SymbolId, RepoStateMode: "full"})
	if err != nil {
		return nil, err
	}

	refResp, err := e.FindReferences(ctx, FindReferencesOptions{SymbolId: opts.SymbolId, IncludeTests: true, Limit: 200})
	if err != nil {
		return nil, err
	}

	nodes := []CallGraphNode{}
	edges := []CallGraphEdge{}

	// Add root node
	rootId := opts.SymbolId
	if symbolResp.Symbol != nil {
		rootId = symbolResp.Symbol.StableId
		nodes = append(nodes, CallGraphNode{
			ID:       rootId,
			SymbolId: rootId,
			Name:     symbolResp.Symbol.Name,
			Depth:    0,
			Role:     "root",
			Score:    1.0,
		})
	}

	// Collect callers if requested
	if opts.Direction == "both" || opts.Direction == "callers" {
		callerCounts := map[string]int{}
		callerLocations := map[string]*LocationInfo{}

		for _, ref := range refResp.References {
			if !strings.Contains(strings.ToLower(ref.Kind), "call") {
				continue
			}
			key := fmt.Sprintf("%s:%d:%d", ref.Location.FileId, ref.Location.StartLine, ref.Location.StartColumn)
			callerCounts[key]++
			callerLocations[key] = &LocationInfo{
				FileId:      ref.Location.FileId,
				StartLine:   ref.Location.StartLine,
				StartColumn: ref.Location.StartColumn,
			}
		}

		for callerKey := range callerCounts {
			parts := strings.SplitN(callerKey, ":", 3)
			nodes = append(nodes, CallGraphNode{
				ID:       callerKey,
				Name:     parts[0],
				Location: callerLocations[callerKey],
				Depth:    1,
				Role:     "caller",
				Score:    float64(callerCounts[callerKey]),
			})
			edges = append(edges, CallGraphEdge{From: callerKey, To: rootId})
		}
	}

	// Note: callees direction not yet implemented - would require analyzing
	// what symbols the target symbol references, which needs different backend queries

	// Sort nodes by score for deterministic output
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Score > nodes[j].Score })

	prov := symbolResp.Provenance
	if prov != nil {
		prov.QueryDurationMs = time.Since(startTime).Milliseconds()
	}

	var truncation *TruncationInfo
	if refResp.TruncationInfo != nil {
		truncation = refResp.TruncationInfo
	} else if opts.Depth > 1 {
		// Note limitation for deeper traversal
		truncation = &TruncationInfo{
			Reason:        "depth",
			OriginalCount: refResp.TotalCount,
			ReturnedCount: len(refResp.References),
		}
	}

	return &CallGraphResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    "0.1.0",
			SchemaVersion: 1,
			Tool:          "getCallGraph",
			Resolved:      &ResolvedTarget{SymbolId: rootId, ResolvedFrom: "id", Confidence: 1.0},
			Truncation:    truncation,
			Provenance:    prov,
		},
		Root:  rootId,
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// GetModuleOverview returns coarse module level information.
func (e *Engine) GetModuleOverview(ctx context.Context, opts ModuleOverviewOptions) (*ModuleOverviewResponse, error) {
	startTime := time.Now()

	modulePath := opts.Path
	if modulePath == "" {
		modulePath = "."
	}

	fileCount := 0
	_ = filepath.Walk(modulePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			base := filepath.Base(path)
			// Skip hidden directories and common non-source directories
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Mode().IsRegular() {
			fileCount++
		}
		return nil
	})

	var recentCommits []string
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		commits, err := e.gitAdapter.GetRecentCommits(5)
		if err == nil {
			for _, c := range commits {
				recentCommits = append(recentCommits, fmt.Sprintf("%s %s", c.Hash, c.Message))
			}
		}
	}

	prov := &Provenance{QueryDurationMs: time.Since(startTime).Milliseconds()}

	moduleName := opts.Name
	if moduleName == "" {
		moduleName = filepath.Base(modulePath)
		if moduleName == "." {
			if cwd, err := os.Getwd(); err == nil {
				moduleName = filepath.Base(cwd)
			}
		}
	}

	return &ModuleOverviewResponse{
		AINavigationMeta: AINavigationMeta{
			CkbVersion:    "0.1.0",
			SchemaVersion: 1,
			Tool:          "getModuleOverview",
			Resolved:      &ResolvedTarget{SymbolId: modulePath, ResolvedFrom: "path", Confidence: 1.0},
			Provenance:    prov,
		},
		Module: ModuleOverviewModule{
			Name: moduleName,
			Path: modulePath,
		},
		Size: ModuleSize{
			FileCount:   fileCount,
			SymbolCount: 0, // Not yet implemented
		},
		RecentCommits: recentCommits,
	}, nil
}

// topLevelModule extracts the top-level directory from a path.
func topLevelModule(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "./"), string(filepath.Separator))
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// tailTimestamp returns the timestamp of the oldest commit in a list.
func tailTimestamp(commits []git.CommitInfo) string {
	if len(commits) == 0 {
		return ""
	}
	return commits[len(commits)-1].Timestamp
}

// classifyCommitFrequency categorizes commit frequency.
func classifyCommitFrequency(count int) string {
	switch {
	case count > 50:
		return "volatile"
	case count > 10:
		return "moderate"
	case count > 0:
		return "stable"
	default:
		return "unknown"
	}
}

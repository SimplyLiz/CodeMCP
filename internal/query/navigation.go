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
)

// ExplainSymbolOptions controls explainSymbol behavior.
type ExplainSymbolOptions struct {
	SymbolId string
}

// ExplainSymbolResponse provides an AI-navigation friendly symbol overview.
type ExplainSymbolResponse struct {
	Facts      ExplainSymbolFacts   `json:"facts"`
	Summary    ExplainSymbolSummary `json:"summary"`
	Provenance *Provenance          `json:"provenance"`
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
	Facts      *ExplainSymbolFacts `json:"facts"`
	Verdict    string              `json:"verdict"`
	Confidence float64             `json:"confidence"`
	Reasoning  string              `json:"reasoning"`
	Provenance *Provenance         `json:"provenance"`
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	SymbolId  string
	Direction string
	Depth     int
}

// CallGraphResponse contains a lightweight call graph.
type CallGraphResponse struct {
	Root       string          `json:"root"`
	Nodes      []CallGraphNode `json:"nodes"`
	Edges      []CallGraphEdge `json:"edges"`
	Provenance *Provenance     `json:"provenance"`
}

// CallGraphNode captures a node in the call graph.
type CallGraphNode struct {
	SymbolId string  `json:"symbolId"`
	Name     string  `json:"name"`
	Depth    int     `json:"depth"`
	Role     string  `json:"role"`
	Score    float64 `json:"score"`
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
	Module        ModuleOverviewModule `json:"module"`
	Size          ModuleSize           `json:"size"`
	RecentCommits []string             `json:"recentCommits,omitempty"`
	Provenance    *Provenance          `json:"provenance"`
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
			IsPublicApi: strings.ToLower(symbolResp.Symbol.Visibility.Visibility) == "public",
			IsExported:  strings.ToLower(symbolResp.Symbol.Visibility.Visibility) == "public",
		}
		facts.Module = symbolResp.Symbol.ModuleId
	}

	// Collect reference data for usage/callers
	refResp, err := e.FindReferences(ctx, FindReferencesOptions{SymbolId: opts.SymbolId, IncludeTests: true, Limit: 200})
	if err == nil && refResp != nil {
		callers := make([]ExplainCaller, 0, len(refResp.References))
		moduleSet := map[string]struct{}{}
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
			if ref.IsTest && facts.Flags != nil {
				facts.Flags.HasTests = true
			}
		}

		facts.Callers = callers
		facts.Usage = &ExplainUsage{
			CallerCount:    len(callers),
			CalleeCount:    len(facts.Callees),
			ReferenceCount: len(refResp.References),
			ModuleCount:    len(moduleSet),
		}

		if facts.Flags != nil && facts.Usage != nil {
			facts.Flags.HasTests = facts.Usage.CallerCount != len(callers) && len(callers) > 0
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
	summary.Tldr = strings.TrimSpace(strings.Join([]string{summary.Identity, summary.Usage}, " – "))

	prov := symbolResp.Provenance
	if prov != nil {
		prov.QueryDurationMs = time.Since(startTime).Milliseconds()
	}

	return &ExplainSymbolResponse{
		Facts:      facts,
		Summary:    summary,
		Provenance: prov,
	}, nil
}

// JustifySymbol applies simple heuristics using explainSymbol facts.
func (e *Engine) JustifySymbol(ctx context.Context, opts JustifySymbolOptions) (*JustifySymbolResponse, error) {
	explain, err := e.ExplainSymbol(ctx, ExplainSymbolOptions{SymbolId: opts.SymbolId})
	if err != nil {
		return nil, err
	}

	verdict := "investigate"
	confidence := 0.5
	reasoning := "Usage unclear"

	if explain.Facts.Usage != nil {
		if explain.Facts.Usage.CallerCount > 0 {
			verdict = "keep"
			confidence = 0.9
			reasoning = fmt.Sprintf("Active callers detected (%d)", explain.Facts.Usage.CallerCount)
		} else if explain.Facts.Flags != nil && explain.Facts.Flags.IsPublicApi {
			verdict = "investigate"
			confidence = 0.6
			reasoning = "Public API but no callers found"
		} else {
			verdict = "remove-candidate"
			confidence = 0.7
			reasoning = "No callers found"
		}
	}

	return &JustifySymbolResponse{
		Facts:      &explain.Facts,
		Verdict:    verdict,
		Confidence: confidence,
		Reasoning:  reasoning,
		Provenance: explain.Provenance,
	}, nil
}

// GetCallGraph builds a shallow caller graph using reference data.
func (e *Engine) GetCallGraph(ctx context.Context, opts CallGraphOptions) (*CallGraphResponse, error) {
	startTime := time.Now()
	if opts.Depth == 0 {
		opts.Depth = 1
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

	if symbolResp.Symbol != nil {
		nodes = append(nodes, CallGraphNode{SymbolId: symbolResp.Symbol.StableId, Name: symbolResp.Symbol.Name, Depth: 0, Role: "root", Score: 1.0})
	}

	callerCounts := map[string]int{}
	for _, ref := range refResp.References {
		if !strings.Contains(strings.ToLower(ref.Kind), "call") {
			continue
		}
		callerKey := fmt.Sprintf("%s:%d", ref.Location.FileId, ref.Location.StartLine)
		callerCounts[callerKey]++
	}

	for callerKey := range callerCounts {
		parts := strings.SplitN(callerKey, ":", 2)
		depth := 1
		nodes = append(nodes, CallGraphNode{SymbolId: callerKey, Name: parts[0], Depth: depth, Role: "caller", Score: float64(callerCounts[callerKey])})
		if len(nodes) > 0 {
			edges = append(edges, CallGraphEdge{From: callerKey, To: opts.SymbolId})
		}
	}

	// Sort nodes for deterministic output
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Score > nodes[j].Score })

	prov := symbolResp.Provenance
	if prov != nil {
		prov.QueryDurationMs = time.Since(startTime).Milliseconds()
	}

	return &CallGraphResponse{
		Root:       opts.SymbolId,
		Nodes:      nodes,
		Edges:      edges,
		Provenance: prov,
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
		if info.Mode().IsRegular() {
			fileCount++
		}
		return nil
	})

	var recent []string
	if e.gitAdapter != nil && e.gitAdapter.IsAvailable() {
		commits, err := e.gitAdapter.GetRecentCommits(5)
		if err == nil {
			for _, c := range commits {
				recent = append(recent, fmt.Sprintf("%s %s", c.Hash, c.Message))
			}
		}
	}

	prov := &Provenance{QueryDurationMs: time.Since(startTime).Milliseconds()}

	return &ModuleOverviewResponse{
		Module: ModuleOverviewModule{
			Name: opts.Name,
			Path: modulePath,
		},
		Size: ModuleSize{
			FileCount:   fileCount,
			SymbolCount: 0,
		},
		RecentCommits: recent,
		Provenance:    prov,
	}, nil
}

func topLevelModule(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "./"), string(filepath.Separator))
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func tailTimestamp(commits []git.CommitInfo) string {
	if len(commits) == 0 {
		return ""
	}
	return commits[len(commits)-1].Timestamp
}

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

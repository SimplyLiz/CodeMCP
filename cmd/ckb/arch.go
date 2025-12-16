package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	archDepth           int
	archIncludeExternal bool
	archRefresh         bool
	archFormat          string
)

var archCmd = &cobra.Command{
	Use:   "arch",
	Short: "Get architecture overview",
	Long: `Get a high-level architecture view of the repository.

Shows:
  - Module structure and dependencies
  - Dependency graph
  - Entry points
  - External dependencies (optional)

Examples:
  ckb arch
  ckb arch --depth=3
  ckb arch --include-external-deps
  ckb arch --refresh`,
	Run: runArch,
}

func init() {
	archCmd.Flags().IntVar(&archDepth, "depth", 2, "Maximum dependency depth")
	archCmd.Flags().BoolVar(&archIncludeExternal, "include-external-deps", false, "Include external dependencies")
	archCmd.Flags().BoolVar(&archRefresh, "refresh", false, "Bypass cache and recompute")
	archCmd.Flags().StringVar(&archFormat, "format", "json", "Output format (json, human)")
	rootCmd.AddCommand(archCmd)
}

func runArch(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(archFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Get architecture from Query Engine
	opts := query.GetArchitectureOptions{
		Depth:               archDepth,
		IncludeExternalDeps: archIncludeExternal,
		Refresh:             archRefresh,
	}
	response, err := engine.GetArchitecture(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting architecture: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertArchResponse(response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(archFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Architecture query completed", map[string]interface{}{
		"modules":  len(response.Modules),
		"duration": time.Since(start).Milliseconds(),
	})
}

// ArchitectureResponseCLI contains architecture overview for CLI output
type ArchitectureResponseCLI struct {
	Modules         []ModuleSummaryCLI  `json:"modules"`
	DependencyGraph []DependencyEdgeCLI `json:"dependencyGraph"`
	Entrypoints     []EntrypointCLI     `json:"entrypoints"`
	Provenance      *ProvenanceCLI      `json:"provenance,omitempty"`
}

// ModuleSummaryCLI provides module statistics
type ModuleSummaryCLI struct {
	ModuleID      string `json:"moduleId"`
	Name          string `json:"name"`
	RootPath      string `json:"rootPath"`
	Language      string `json:"language,omitempty"`
	FileCount     int    `json:"fileCount"`
	SymbolCount   int    `json:"symbolCount"`
	IncomingEdges int    `json:"incomingEdges"`
	OutgoingEdges int    `json:"outgoingEdges"`
}

// DependencyEdgeCLI represents a module dependency
type DependencyEdgeCLI struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Strength int    `json:"strength"`
}

// EntrypointCLI represents an entry point file
type EntrypointCLI struct {
	FileID   string `json:"fileId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	ModuleID string `json:"moduleId"`
}

func convertArchResponse(resp *query.GetArchitectureResponse) *ArchitectureResponseCLI {
	modules := make([]ModuleSummaryCLI, 0, len(resp.Modules))
	for _, m := range resp.Modules {
		modules = append(modules, ModuleSummaryCLI{
			ModuleID:      m.ModuleId,
			Name:          m.Name,
			RootPath:      m.Path,
			Language:      m.Language,
			FileCount:     m.FileCount,
			SymbolCount:   m.SymbolCount,
			IncomingEdges: m.IncomingEdges,
			OutgoingEdges: m.OutgoingEdges,
		})
	}

	edges := make([]DependencyEdgeCLI, 0, len(resp.DependencyGraph))
	for _, e := range resp.DependencyGraph {
		edges = append(edges, DependencyEdgeCLI{
			From:     e.From,
			To:       e.To,
			Kind:     e.Kind,
			Strength: e.Strength,
		})
	}

	entrypoints := make([]EntrypointCLI, 0, len(resp.Entrypoints))
	for _, ep := range resp.Entrypoints {
		entrypoints = append(entrypoints, EntrypointCLI{
			FileID:   ep.FileId,
			Name:     ep.Name,
			Kind:     ep.Kind,
			ModuleID: ep.ModuleId,
		})
	}

	result := &ArchitectureResponseCLI{
		Modules:         modules,
		DependencyGraph: edges,
		Entrypoints:     entrypoints,
	}

	if resp.Provenance != nil {
		result.Provenance = &ProvenanceCLI{
			RepoStateId:     resp.Provenance.RepoStateId,
			RepoStateDirty:  resp.Provenance.RepoStateDirty,
			QueryDurationMs: resp.Provenance.QueryDurationMs,
		}
	}

	return result
}

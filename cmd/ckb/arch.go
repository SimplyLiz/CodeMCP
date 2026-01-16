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
	archGranularity     string
	archInferModules    bool
	archTargetPath      string
)

var archCmd = &cobra.Command{
	Use:   "arch",
	Short: "Get architecture overview",
	Long: `Get a high-level architecture view of the repository.

Shows:
  - Module structure and dependencies (granularity=module, default)
  - Directory-level structure (granularity=directory)
  - File-level dependencies (granularity=file)
  - Dependency graph
  - Entry points
  - External dependencies (optional)

Examples:
  ckb arch
  ckb arch --depth=3
  ckb arch --include-external-deps
  ckb arch --granularity=directory
  ckb arch --granularity=file --target-path=src/components
  ckb arch --refresh`,
	Run: runArch,
}

func init() {
	archCmd.Flags().IntVar(&archDepth, "depth", 2, "Maximum dependency depth")
	archCmd.Flags().BoolVar(&archIncludeExternal, "include-external-deps", false, "Include external dependencies")
	archCmd.Flags().BoolVar(&archRefresh, "refresh", false, "Bypass cache and recompute")
	archCmd.Flags().StringVar(&archFormat, "format", "json", "Output format (json, human)")
	archCmd.Flags().StringVar(&archGranularity, "granularity", "module", "Level of detail: module, directory, file")
	archCmd.Flags().BoolVar(&archInferModules, "infer-modules", true, "Infer modules from directory structure")
	archCmd.Flags().StringVar(&archTargetPath, "target-path", "", "Focus on specific path (relative to repo root)")
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
		Granularity:         archGranularity,
		InferModules:        archInferModules,
		TargetPath:          archTargetPath,
	}
	response, err := engine.GetArchitecture(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting architecture: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format based on granularity
	cliResponse := convertArchResponse(response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(archFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Architecture query completed",
		"granularity", response.Granularity,
		"modules", len(response.Modules),
		"directories", len(response.Directories),
		"files", len(response.Files),
		"duration", time.Since(start).Milliseconds(),
	)
}

// ArchitectureResponseCLI contains architecture overview for CLI output
type ArchitectureResponseCLI struct {
	// Metadata
	Granularity     string `json:"granularity"`
	DetectionMethod string `json:"detectionMethod,omitempty"`

	// Module-level fields (granularity=module)
	Modules         []ModuleSummaryCLI  `json:"modules,omitempty"`
	DependencyGraph []DependencyEdgeCLI `json:"dependencyGraph,omitempty"`
	Entrypoints     []EntrypointCLI     `json:"entrypoints,omitempty"`

	// Directory-level fields (granularity=directory)
	Directories           []DirectorySummaryCLI `json:"directories,omitempty"`
	DirectoryDependencies []DependencyEdgeCLI   `json:"directoryDependencies,omitempty"`

	// File-level fields (granularity=file)
	Files            []FileSummaryCLI        `json:"files,omitempty"`
	FileDependencies []FileDependencyEdgeCLI `json:"fileDependencies,omitempty"`

	Provenance *ProvenanceCLI `json:"provenance,omitempty"`
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

// DirectorySummaryCLI represents a directory in directory-level views
type DirectorySummaryCLI struct {
	Path           string `json:"path"`
	FileCount      int    `json:"fileCount"`
	Language       string `json:"language,omitempty"`
	LOC            int    `json:"loc,omitempty"`
	HasIndexFile   bool   `json:"hasIndexFile"`
	IncomingEdges  int    `json:"incomingEdges"`
	OutgoingEdges  int    `json:"outgoingEdges"`
	IsIntermediate bool   `json:"isIntermediate,omitempty"`
}

// FileSummaryCLI represents a file in file-level views
type FileSummaryCLI struct {
	Path          string `json:"path"`
	Language      string `json:"language,omitempty"`
	LOC           int    `json:"loc,omitempty"`
	IncomingEdges int    `json:"incomingEdges"`
	OutgoingEdges int    `json:"outgoingEdges"`
}

// FileDependencyEdgeCLI represents a file-level dependency
type FileDependencyEdgeCLI struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Line     int    `json:"line,omitempty"`
	Resolved bool   `json:"resolved"`
}

func convertArchResponse(resp *query.GetArchitectureResponse) *ArchitectureResponseCLI {
	result := &ArchitectureResponseCLI{
		Granularity:     resp.Granularity,
		DetectionMethod: resp.DetectionMethod,
	}

	switch resp.Granularity {
	case "directory":
		// Directory-level response
		directories := make([]DirectorySummaryCLI, 0, len(resp.Directories))
		for _, d := range resp.Directories {
			directories = append(directories, DirectorySummaryCLI{
				Path:           d.Path,
				FileCount:      d.FileCount,
				Language:       d.Language,
				LOC:            d.LOC,
				HasIndexFile:   d.HasIndexFile,
				IncomingEdges:  d.IncomingEdges,
				OutgoingEdges:  d.OutgoingEdges,
				IsIntermediate: d.IsIntermediate,
			})
		}
		result.Directories = directories

		dirEdges := make([]DependencyEdgeCLI, 0, len(resp.DirectoryDependencies))
		for _, e := range resp.DirectoryDependencies {
			dirEdges = append(dirEdges, DependencyEdgeCLI{
				From:     e.From,
				To:       e.To,
				Kind:     e.Kind,
				Strength: e.Strength,
			})
		}
		result.DirectoryDependencies = dirEdges

	case "file":
		// File-level response
		files := make([]FileSummaryCLI, 0, len(resp.Files))
		for _, f := range resp.Files {
			files = append(files, FileSummaryCLI{
				Path:          f.Path,
				Language:      f.Language,
				LOC:           f.LOC,
				IncomingEdges: f.IncomingEdges,
				OutgoingEdges: f.OutgoingEdges,
			})
		}
		result.Files = files

		fileEdges := make([]FileDependencyEdgeCLI, 0, len(resp.FileDependencies))
		for _, e := range resp.FileDependencies {
			fileEdges = append(fileEdges, FileDependencyEdgeCLI{
				From:     e.From,
				To:       e.To,
				Kind:     e.Kind,
				Line:     e.Line,
				Resolved: e.Resolved,
			})
		}
		result.FileDependencies = fileEdges

	default:
		// Module-level response (default)
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
		result.Modules = modules

		edges := make([]DependencyEdgeCLI, 0, len(resp.DependencyGraph))
		for _, e := range resp.DependencyGraph {
			edges = append(edges, DependencyEdgeCLI{
				From:     e.From,
				To:       e.To,
				Kind:     e.Kind,
				Strength: e.Strength,
			})
		}
		result.DependencyGraph = edges

		entrypoints := make([]EntrypointCLI, 0, len(resp.Entrypoints))
		for _, ep := range resp.Entrypoints {
			entrypoints = append(entrypoints, EntrypointCLI{
				FileID:   ep.FileId,
				Name:     ep.Name,
				Kind:     ep.Kind,
				ModuleID: ep.ModuleId,
			})
		}
		result.Entrypoints = entrypoints
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

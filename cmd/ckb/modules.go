package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	modulesFormat          string
	modulesPath            string //nolint:unused // reserved for future use
	modulesName            string
	annotateResponsibility string
	annotateCapabilities   string
	annotateTags           string
	annotatePublicPaths    string
	annotateInternalPaths  string
	// Responsibilities subcommand flags
	respModuleId     string //nolint:unused // reserved for future use
	respIncludeFiles bool
	respLimit        int
	respFormat       string
)

var modulesCmd = &cobra.Command{
	Use:   "modules [path]",
	Short: "Get module overview and manage annotations",
	Long: `Get a high-level overview of a module including size and recent activity.

Without arguments, shows overview of the current directory.
With a path, shows overview of that specific module.

Examples:
  ckb modules
  ckb modules internal/api
  ckb modules --name="API Layer" internal/api
  ckb modules --format=human`,
	Args: cobra.MaximumNArgs(1),
	Run:  runModulesOverview,
}

var modulesAnnotateCmd = &cobra.Command{
	Use:   "annotate <module-id>",
	Short: "Add or update module metadata",
	Long: `Annotate a module with responsibilities, capabilities, tags, and API boundaries.

Examples:
  ckb modules annotate internal/api --responsibility="HTTP API handlers"
  ckb modules annotate internal/api --capabilities="REST,WebSocket"
  ckb modules annotate internal/api --tags="core,infrastructure"
  ckb modules annotate internal/api --public-paths="handler.go,routes.go"`,
	Args: cobra.ExactArgs(1),
	Run:  runModulesAnnotate,
}

var modulesResponsibilitiesCmd = &cobra.Command{
	Use:   "responsibilities [module-id]",
	Short: "Get responsibilities for modules",
	Long: `Get responsibilities for modules including what each module does,
its capabilities, and confidence in the assessment.

Responsibilities are extracted from README files, doc comments, and code analysis.

Examples:
  ckb modules responsibilities
  ckb modules responsibilities internal/api
  ckb modules responsibilities --include-files
  ckb modules responsibilities --limit=50`,
	Args: cobra.MaximumNArgs(1),
	Run:  runModulesResponsibilities,
}

func init() {
	// Overview flags
	modulesCmd.Flags().StringVar(&modulesFormat, "format", "json", "Output format (json, human)")
	modulesCmd.Flags().StringVar(&modulesName, "name", "", "Optional friendly name for the module")

	// Annotate flags
	modulesAnnotateCmd.Flags().StringVar(&modulesFormat, "format", "json", "Output format (json, human)")
	modulesAnnotateCmd.Flags().StringVar(&annotateResponsibility, "responsibility", "", "One-sentence description of what this module does")
	modulesAnnotateCmd.Flags().StringVar(&annotateCapabilities, "capabilities", "", "Comma-separated list of capabilities")
	modulesAnnotateCmd.Flags().StringVar(&annotateTags, "tags", "", "Comma-separated list of tags")
	modulesAnnotateCmd.Flags().StringVar(&annotatePublicPaths, "public-paths", "", "Comma-separated list of public API paths")
	modulesAnnotateCmd.Flags().StringVar(&annotateInternalPaths, "internal-paths", "", "Comma-separated list of internal paths")

	// Responsibilities subcommand flags
	modulesResponsibilitiesCmd.Flags().StringVar(&respFormat, "format", "json", "Output format (json, human)")
	modulesResponsibilitiesCmd.Flags().BoolVar(&respIncludeFiles, "include-files", false, "Include file-level responsibilities")
	modulesResponsibilitiesCmd.Flags().IntVar(&respLimit, "limit", 20, "Maximum modules to return")

	modulesCmd.AddCommand(modulesAnnotateCmd)
	modulesCmd.AddCommand(modulesResponsibilitiesCmd)
	rootCmd.AddCommand(modulesCmd)
}

func runModulesOverview(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(modulesFormat)

	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.ModuleOverviewOptions{
		Path: path,
		Name: modulesName,
	}
	response, err := engine.GetModuleOverview(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting module overview: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertModuleOverviewResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(modulesFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Module overview completed",
		"path", path,
		"duration", time.Since(start).Milliseconds(),
	)
}

func runModulesAnnotate(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(modulesFormat)
	moduleId := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	input := &query.AnnotateModuleInput{
		ModuleId:       moduleId,
		Responsibility: annotateResponsibility,
	}

	if annotateCapabilities != "" {
		input.Capabilities = splitAndTrim(annotateCapabilities)
	}
	if annotateTags != "" {
		input.Tags = splitAndTrim(annotateTags)
	}
	if annotatePublicPaths != "" {
		input.PublicPaths = splitAndTrim(annotatePublicPaths)
	}
	if annotateInternalPaths != "" {
		input.InternalPaths = splitAndTrim(annotateInternalPaths)
	}

	result, err := engine.AnnotateModule(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error annotating module: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertAnnotateModuleResponse(result)

	output, err := FormatResponse(cliResponse, OutputFormat(modulesFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Module annotation completed",
		"moduleId", moduleId,
		"created", result.Created,
		"updated", result.Updated,
		"duration", time.Since(start).Milliseconds(),
	)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ModuleOverviewResponseCLI contains module overview for CLI output
type ModuleOverviewResponseCLI struct {
	Module        ModuleOverviewModuleCLI `json:"module"`
	Size          ModuleOverviewSizeCLI   `json:"size"`
	RecentCommits []string                `json:"recentCommits,omitempty"`
	Provenance    *ProvenanceCLI          `json:"provenance,omitempty"`
}

type ModuleOverviewModuleCLI struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ModuleOverviewSizeCLI struct {
	FileCount   int `json:"fileCount"`
	SymbolCount int `json:"symbolCount"`
}

func convertModuleOverviewResponse(resp *query.ModuleOverviewResponse) *ModuleOverviewResponseCLI {
	result := &ModuleOverviewResponseCLI{
		Module: ModuleOverviewModuleCLI{
			Name: resp.Module.Name,
			Path: resp.Module.Path,
		},
		Size: ModuleOverviewSizeCLI{
			FileCount:   resp.Size.FileCount,
			SymbolCount: resp.Size.SymbolCount,
		},
		RecentCommits: resp.RecentCommits,
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

// AnnotateModuleResponseCLI contains annotate result for CLI output
type AnnotateModuleResponseCLI struct {
	ModuleId       string                 `json:"moduleId"`
	Responsibility string                 `json:"responsibility,omitempty"`
	Capabilities   []string               `json:"capabilities,omitempty"`
	Tags           []string               `json:"tags,omitempty"`
	Boundaries     *AnnotateBoundariesCLI `json:"boundaries,omitempty"`
	Updated        bool                   `json:"updated"`
	Created        bool                   `json:"created"`
}

type AnnotateBoundariesCLI struct {
	Public   []string `json:"public,omitempty"`
	Internal []string `json:"internal,omitempty"`
}

func convertAnnotateModuleResponse(resp *query.AnnotateModuleResult) *AnnotateModuleResponseCLI {
	result := &AnnotateModuleResponseCLI{
		ModuleId:       resp.ModuleId,
		Responsibility: resp.Responsibility,
		Capabilities:   resp.Capabilities,
		Tags:           resp.Tags,
		Updated:        resp.Updated,
		Created:        resp.Created,
	}

	if resp.Boundaries != nil {
		result.Boundaries = &AnnotateBoundariesCLI{
			Public:   resp.Boundaries.Public,
			Internal: resp.Boundaries.Internal,
		}
	}

	return result
}

func runModulesResponsibilities(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(respFormat)

	moduleId := ""
	if len(args) > 0 {
		moduleId = args[0]
	}

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.GetModuleResponsibilitiesOptions{
		ModuleId:     moduleId,
		IncludeFiles: respIncludeFiles,
		Limit:        respLimit,
	}
	response, err := engine.GetModuleResponsibilities(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting module responsibilities: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertModuleResponsibilitiesResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(respFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Module responsibilities completed",
		"moduleId", moduleId,
		"count", len(response.Modules),
		"duration", time.Since(start).Milliseconds(),
	)
}

// ModuleResponsibilitiesResponseCLI contains responsibilities for CLI output
type ModuleResponsibilitiesResponseCLI struct {
	Modules     []ModuleResponsibilityCLI `json:"modules"`
	TotalCount  int                       `json:"totalCount"`
	Confidence  float64                   `json:"confidence"`
	Limitations []string                  `json:"limitations,omitempty"`
	Provenance  *ProvenanceCLI            `json:"provenance,omitempty"`
}

type ModuleResponsibilityCLI struct {
	ModuleId     string                  `json:"moduleId"`
	Name         string                  `json:"name"`
	Path         string                  `json:"path"`
	Summary      string                  `json:"summary"`
	Capabilities []string                `json:"capabilities,omitempty"`
	Source       string                  `json:"source"`
	Confidence   float64                 `json:"confidence"`
	Files        []FileResponsibilityCLI `json:"files,omitempty"`
}

type FileResponsibilityCLI struct {
	Path       string  `json:"path"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
}

func convertModuleResponsibilitiesResponse(resp *query.GetModuleResponsibilitiesResponse) *ModuleResponsibilitiesResponseCLI {
	modules := make([]ModuleResponsibilityCLI, 0, len(resp.Modules))
	for _, m := range resp.Modules {
		module := ModuleResponsibilityCLI{
			ModuleId:     m.ModuleId,
			Name:         m.Name,
			Path:         m.Path,
			Summary:      m.Summary,
			Capabilities: m.Capabilities,
			Source:       m.Source,
			Confidence:   m.Confidence,
		}

		if len(m.Files) > 0 {
			module.Files = make([]FileResponsibilityCLI, 0, len(m.Files))
			for _, f := range m.Files {
				module.Files = append(module.Files, FileResponsibilityCLI{
					Path:       f.Path,
					Summary:    f.Summary,
					Confidence: f.Confidence,
				})
			}
		}

		modules = append(modules, module)
	}

	result := &ModuleResponsibilitiesResponseCLI{
		Modules:     modules,
		TotalCount:  resp.TotalCount,
		Confidence:  resp.Confidence,
		Limitations: resp.Limitations,
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

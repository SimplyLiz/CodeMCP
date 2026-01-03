package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"ckb/internal/federation"
)

var contractsCmd = &cobra.Command{
	Use:   "contracts",
	Short: "Manage and analyze API contracts",
	Long: `Commands for working with API contracts (protobuf, OpenAPI) in federations.

CKB v6.3 provides contract-aware impact analysis - understanding how changes
to shared APIs affect consumers across repositories.`,
}

// Flags
var (
	contractsRepoID         string
	contractsContractType   string
	contractsVisibility     string
	contractsIncludeHeur    bool
	contractsIncludeTrans   bool
	contractsMaxDepth       int
	contractsDirection      string
	contractsSuppressReason string
)

var contractsListCmd = &cobra.Command{
	Use:   "list <federation>",
	Short: "List API contracts in a federation",
	Long: `List all detected API contracts (protobuf, OpenAPI) in a federation.

Contracts are classified by visibility:
  public   - Under api/, proto/, versioned package, has service definitions
  internal - Under internal/, testdata/, private package naming
  unknown  - Doesn't match clear patterns (treated conservatively as public)`,
	Args: cobra.ExactArgs(1),
	RunE: runContractsList,
}

var contractsImpactCmd = &cobra.Command{
	Use:   "impact <federation> --repo=<repo-id> --path=<path>",
	Short: "Analyze impact of changing a contract",
	Long: `Analyze the impact of changing an API contract.

Returns:
  - Direct consumers: repos that import/use this contract
  - Transitive consumers: repos reached through contract imports
  - Risk assessment: low/medium/high based on consumer count, visibility, etc.
  - Ownership: who to contact for approval

Examples:
  ckb contracts impact platform --repo=api --path=proto/api/v1/user.proto
  ckb contracts impact platform --repo=gateway --path=openapi.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runContractsImpact,
}

var contractsDepsCmd = &cobra.Command{
	Use:   "deps <federation> --repo=<repo-id>",
	Short: "Get contract dependencies for a repository",
	Long: `Show contract dependencies for a repository.

With --direction=dependencies: contracts this repo depends on
With --direction=consumers: repos that consume contracts from this repo
With --direction=both: both directions (default)`,
	Args: cobra.ExactArgs(1),
	RunE: runContractsDeps,
}

var contractsSuppressCmd = &cobra.Command{
	Use:   "suppress <federation> --edge=<edge-id>",
	Short: "Suppress a false positive contract edge",
	Long:  `Suppress a contract dependency edge that is a false positive. The edge will be hidden from analysis results.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runContractsSuppress,
}

var contractsVerifyCmd = &cobra.Command{
	Use:   "verify <federation> --edge=<edge-id>",
	Short: "Verify a contract edge",
	Long:  `Mark a contract dependency edge as verified, increasing confidence in the edge.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runContractsVerify,
}

var contractsStatsCmd = &cobra.Command{
	Use:   "stats <federation>",
	Short: "Show contract statistics",
	Long:  `Show summary statistics for contracts in a federation.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runContractsStats,
}

var contractsEdgeID int64

func init() {
	rootCmd.AddCommand(contractsCmd)

	// List command
	contractsListCmd.Flags().StringVar(&contractsRepoID, "repo", "", "Filter to contracts from this repo")
	contractsListCmd.Flags().StringVar(&contractsContractType, "type", "", "Filter by contract type (proto, openapi)")
	contractsListCmd.Flags().StringVar(&contractsVisibility, "visibility", "", "Filter by visibility (public, internal, unknown)")
	contractsListCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	contractsCmd.AddCommand(contractsListCmd)

	// Impact command
	contractsImpactCmd.Flags().StringVar(&contractsRepoID, "repo", "", "Repository containing the contract (required)")
	contractsImpactCmd.Flags().StringVar(&fedRepoPath, "path", "", "Path to the contract file (required)")
	contractsImpactCmd.Flags().BoolVar(&contractsIncludeHeur, "include-heuristic", false, "Include tier 3 (heuristic) edges")
	contractsImpactCmd.Flags().BoolVar(&contractsIncludeTrans, "include-transitive", true, "Include transitive consumers")
	contractsImpactCmd.Flags().IntVar(&contractsMaxDepth, "max-depth", 3, "Maximum depth for transitive analysis")
	contractsImpactCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	contractsImpactCmd.MarkFlagRequired("repo")
	contractsImpactCmd.MarkFlagRequired("path")
	contractsCmd.AddCommand(contractsImpactCmd)

	// Deps command
	contractsDepsCmd.Flags().StringVar(&contractsRepoID, "repo", "", "Repository to analyze (required)")
	contractsDepsCmd.Flags().StringVar(&contractsDirection, "direction", "both", "Direction: consumers, dependencies, or both")
	contractsDepsCmd.Flags().BoolVar(&contractsIncludeHeur, "include-heuristic", false, "Include tier 3 (heuristic) edges")
	contractsDepsCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	contractsDepsCmd.MarkFlagRequired("repo")
	contractsCmd.AddCommand(contractsDepsCmd)

	// Suppress command
	contractsSuppressCmd.Flags().Int64Var(&contractsEdgeID, "edge", 0, "Edge ID to suppress (required)")
	contractsSuppressCmd.Flags().StringVar(&contractsSuppressReason, "reason", "", "Reason for suppression")
	contractsSuppressCmd.MarkFlagRequired("edge")
	contractsCmd.AddCommand(contractsSuppressCmd)

	// Verify command
	contractsVerifyCmd.Flags().Int64Var(&contractsEdgeID, "edge", 0, "Edge ID to verify (required)")
	contractsVerifyCmd.MarkFlagRequired("edge")
	contractsCmd.AddCommand(contractsVerifyCmd)

	// Stats command
	contractsStatsCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	contractsCmd.AddCommand(contractsStatsCmd)
}

func runContractsList(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.ListContractsOptions{
		RepoID:       contractsRepoID,
		ContractType: contractsContractType,
		Visibility:   contractsVisibility,
	}

	result, err := fed.ListContracts(opts)
	if err != nil {
		return fmt.Errorf("failed to list contracts: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(result.Contracts) == 0 {
		fmt.Println("No contracts found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tTYPE\tVISIBILITY\tPATH")
	for _, c := range result.Contracts {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.RepoID, c.ContractType, c.Visibility, c.Path)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d contracts\n", result.TotalCount)
	return nil
}

func runContractsImpact(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.AnalyzeContractImpactOptions{
		Federation:        fedName,
		RepoID:            contractsRepoID,
		Path:              fedRepoPath,
		IncludeHeuristic:  contractsIncludeHeur,
		IncludeTransitive: contractsIncludeTrans,
		MaxDepth:          contractsMaxDepth,
	}

	result, err := fed.AnalyzeContractImpact(opts)
	if err != nil {
		return fmt.Errorf("failed to analyze impact: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Print results
	if result.Contract == nil {
		fmt.Println("Path is not a recognized contract")
		return nil
	}

	fmt.Printf("Contract: %s:%s (%s, %s)\n\n", result.Contract.RepoID, result.Contract.Path,
		result.Contract.ContractType, result.Contract.Visibility)

	// Direct consumers
	if len(result.DirectConsumers) > 0 {
		fmt.Printf("Direct Consumers (%d repos):\n", result.Summary.DirectRepoCount)
		for _, c := range result.DirectConsumers {
			fmt.Printf("  %-12s [%s: %s]\t%s\n", c.RepoID, c.Tier, c.EvidenceType, strings.Join(c.ConsumerPaths, ", "))
		}
		fmt.Println()
	} else {
		fmt.Println("No direct consumers found")
		fmt.Println()
	}

	// Transitive consumers
	if len(result.TransitiveConsumers) > 0 {
		fmt.Printf("Transitive Consumers (%d repos):\n", result.Summary.TransitiveRepoCount)
		for _, c := range result.TransitiveConsumers {
			fmt.Printf("  %-12s via %s (depth %d)\n", c.RepoID, c.ViaContract, c.Depth)
		}
		fmt.Println()
	}

	// Risk assessment
	fmt.Printf("Risk: %s\n", strings.ToUpper(result.Summary.RiskLevel))
	for _, factor := range result.Summary.RiskFactors {
		fmt.Printf("  - %s\n", factor)
	}
	fmt.Println()

	// Ownership
	if len(result.Ownership.ApprovalRequired) > 0 {
		fmt.Println("Approval Required:")
		for _, owner := range result.Ownership.ApprovalRequired {
			fmt.Printf("  %s: %s\n", owner.Type, owner.ID)
		}
	}

	return nil
}

func runContractsDeps(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.GetDependenciesOptions{
		Federation:       fedName,
		RepoID:           contractsRepoID,
		Direction:        contractsDirection,
		IncludeHeuristic: contractsIncludeHeur,
	}

	result, err := fed.GetDependencies(opts)
	if err != nil {
		return fmt.Errorf("failed to get dependencies: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Print dependencies
	if contractsDirection == "dependencies" || contractsDirection == "both" {
		fmt.Printf("Dependencies (%d):\n", len(result.Dependencies))
		if len(result.Dependencies) > 0 {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  REPO\tTYPE\tPATH\tTIER\tCONFIDENCE")
			for _, d := range result.Dependencies {
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%.2f\n",
					d.Contract.RepoID, d.Contract.ContractType, d.Contract.Path, d.Tier, d.Confidence)
			}
			w.Flush()
		}
		fmt.Println()
	}

	// Print consumers
	if contractsDirection == "consumers" || contractsDirection == "both" {
		fmt.Printf("Consumers (%d):\n", len(result.Consumers))
		if len(result.Consumers) > 0 {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  CONTRACT\tCONSUMER\tTIER\tCONFIDENCE")
			for _, c := range result.Consumers {
				fmt.Fprintf(w, "  %s\t%s\t%s\t%.2f\n",
					c.Contract.Path, c.ConsumerRepo.RepoID, c.ConsumerRepo.Tier, c.ConsumerRepo.Confidence)
			}
			w.Flush()
		}
	}

	return nil
}

func runContractsSuppress(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	if err := fed.SuppressContractEdge(contractsEdgeID, "cli", contractsSuppressReason); err != nil {
		return fmt.Errorf("failed to suppress edge: %w", err)
	}

	fmt.Printf("Suppressed edge %d\n", contractsEdgeID)
	return nil
}

func runContractsVerify(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	if err := fed.VerifyContractEdge(contractsEdgeID, "cli"); err != nil {
		return fmt.Errorf("failed to verify edge: %w", err)
	}

	fmt.Printf("Verified edge %d\n", contractsEdgeID)
	return nil
}

func runContractsStats(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	stats, err := fed.GetContractStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}

	fmt.Println("Contract Statistics:")
	fmt.Printf("  Total contracts:    %d\n", stats.TotalContracts)
	fmt.Printf("    Public:           %d\n", stats.PublicContracts)
	fmt.Printf("    Internal:         %d\n", stats.InternalContracts)
	fmt.Println()

	fmt.Println("By type:")
	for typ, count := range stats.ByType {
		fmt.Printf("  %-12s %d\n", typ, count)
	}
	fmt.Println()

	fmt.Println("Dependency edges:")
	fmt.Printf("  Total:              %d\n", stats.TotalEdges)
	fmt.Printf("    Declared (tier 1): %d\n", stats.DeclaredEdges)
	fmt.Printf("    Derived (tier 2):  %d\n", stats.DerivedEdges)

	return nil
}

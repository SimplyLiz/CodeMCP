package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	decisionsStatus      string
	decisionsModule      string
	decisionsSearch      string
	decisionsLimit       int
	decisionsFormat      string
	decisionsInteractive bool
)

var decisionsCmd = &cobra.Command{
	Use:   "decisions",
	Short: "Manage architectural decisions (ADRs)",
	Long: `Query and create Architectural Decision Records (ADRs).

Without subcommands, lists existing decisions. Use 'create' to record a new decision.

Examples:
  ckb decisions
  ckb decisions --status=accepted
  ckb decisions --module=internal/api
  ckb decisions --search="authentication"
  ckb decisions create --interactive`,
	Run: runDecisionsList,
}

var decisionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Record a new architectural decision",
	Long: `Create a new Architectural Decision Record (ADR).

Use --interactive for a guided creation process, or provide all fields via flags.

Examples:
  ckb decisions create --interactive
  ckb decisions create --title="Use PostgreSQL" --context="Need persistent storage" --decision="Use PostgreSQL for data persistence"`,
	Run: runDecisionsCreate,
}

// Flags for create subcommand
var (
	createTitle        string
	createContext      string
	createDecision     string
	createConsequences []string
	createModules      []string
	createAlternatives []string
	createAuthor       string
	createStatus       string
)

func init() {
	// List flags
	decisionsCmd.Flags().StringVar(&decisionsStatus, "status", "", "Filter by status (proposed, accepted, deprecated, superseded)")
	decisionsCmd.Flags().StringVar(&decisionsModule, "module", "", "Filter by affected module")
	decisionsCmd.Flags().StringVar(&decisionsSearch, "search", "", "Search in title and content")
	decisionsCmd.Flags().IntVar(&decisionsLimit, "limit", 50, "Maximum decisions to return")
	decisionsCmd.Flags().StringVar(&decisionsFormat, "format", "human", "Output format (json, human)")

	// Create flags
	decisionsCreateCmd.Flags().BoolVar(&decisionsInteractive, "interactive", false, "Interactive creation mode")
	decisionsCreateCmd.Flags().StringVar(&createTitle, "title", "", "Decision title")
	decisionsCreateCmd.Flags().StringVar(&createContext, "context", "", "Background and forces driving the decision")
	decisionsCreateCmd.Flags().StringVar(&createDecision, "decision", "", "What was decided and why")
	decisionsCreateCmd.Flags().StringSliceVar(&createConsequences, "consequence", nil, "Consequences (can be specified multiple times)")
	decisionsCreateCmd.Flags().StringSliceVar(&createModules, "module", nil, "Affected modules (can be specified multiple times)")
	decisionsCreateCmd.Flags().StringSliceVar(&createAlternatives, "alternative", nil, "Alternatives considered (can be specified multiple times)")
	decisionsCreateCmd.Flags().StringVar(&createAuthor, "author", "", "Author of the decision")
	decisionsCreateCmd.Flags().StringVar(&createStatus, "status", "proposed", "Initial status (proposed, accepted)")
	decisionsCreateCmd.Flags().StringVar(&decisionsFormat, "format", "human", "Output format (json, human)")

	decisionsCmd.AddCommand(decisionsCreateCmd)
	rootCmd.AddCommand(decisionsCmd)
}

func runDecisionsList(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(decisionsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	queryOpts := &query.DecisionsQuery{
		Status:   decisionsStatus,
		ModuleID: decisionsModule,
		Search:   decisionsSearch,
		Limit:    decisionsLimit,
	}

	response, err := engine.GetDecisions(queryOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting decisions: %v\n", err)
		os.Exit(1)
	}

	// Format and output
	output, err := FormatResponse(response, OutputFormat(decisionsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Decisions query completed",
		"total", response.Total,
		"returned", len(response.Decisions),
		"duration", time.Since(start).Milliseconds(),
	)
}

func runDecisionsCreate(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(decisionsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	var input query.RecordDecisionInput

	if decisionsInteractive {
		// Interactive mode
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Creating new Architectural Decision Record (ADR)")
		fmt.Println("================================================")
		fmt.Println()

		input.Title = promptRequired(reader, "Title (short description)")
		input.Context = promptRequired(reader, "Context (background and forces)")
		input.Decision = promptRequired(reader, "Decision (what and why)")

		fmt.Println("\nConsequences (one per line, empty line to finish):")
		input.Consequences = promptMultiline(reader)

		fmt.Println("\nAffected modules (one per line, empty line to finish):")
		input.AffectedModules = promptMultiline(reader)

		fmt.Println("\nAlternatives considered (one per line, empty line to finish):")
		input.Alternatives = promptMultiline(reader)

		input.Author = promptOptional(reader, "Author")

		statusInput := promptOptional(reader, "Status (proposed/accepted, default: proposed)")
		if statusInput != "" {
			input.Status = statusInput
		} else {
			input.Status = "proposed"
		}
	} else {
		// Flag-based creation
		if createTitle == "" {
			fmt.Fprintf(os.Stderr, "Error: --title is required (or use --interactive)\n")
			os.Exit(1)
		}
		if createContext == "" {
			fmt.Fprintf(os.Stderr, "Error: --context is required (or use --interactive)\n")
			os.Exit(1)
		}
		if createDecision == "" {
			fmt.Fprintf(os.Stderr, "Error: --decision is required (or use --interactive)\n")
			os.Exit(1)
		}

		input = query.RecordDecisionInput{
			Title:           createTitle,
			Context:         createContext,
			Decision:        createDecision,
			Consequences:    createConsequences,
			AffectedModules: createModules,
			Alternatives:    createAlternatives,
			Author:          createAuthor,
			Status:          createStatus,
		}
	}

	response, err := engine.RecordDecision(&input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recording decision: %v\n", err)
		os.Exit(1)
	}

	// Format and output
	output, err := FormatResponse(response, OutputFormat(decisionsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	if decisionsFormat == "human" {
		fmt.Printf("\nDecision %s created successfully!\n", response.Decision.ID)
	}

	logger.Debug("Decision created",
		"id", response.Decision.ID,
		"title", response.Decision.Title,
		"duration", time.Since(start).Milliseconds(),
	)
}

func promptRequired(reader *bufio.Reader, prompt string) string {
	for {
		fmt.Printf("%s: ", prompt)
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
		fmt.Println("  (required, please enter a value)")
	}
}

func promptOptional(reader *bufio.Reader, prompt string) string {
	fmt.Printf("%s: ", prompt)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func promptMultiline(reader *bufio.Reader) []string {
	var lines []string
	for {
		fmt.Print("  > ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" {
			break
		}
		lines = append(lines, text)
	}
	return lines
}

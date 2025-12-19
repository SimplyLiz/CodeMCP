package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	conceptsFormat string
	conceptsLimit  int
)

var conceptsCmd = &cobra.Command{
	Use:   "concepts",
	Short: "Discover main ideas/concepts in the codebase",
	Long: `Discover key concepts through semantic clustering.

Helps understand domain vocabulary and main ideas in the codebase.

Examples:
  ckb concepts
  ckb concepts --limit=8
  ckb concepts --format=human`,
	Run: runConcepts,
}

func init() {
	conceptsCmd.Flags().StringVar(&conceptsFormat, "format", "json", "Output format (json, human)")
	conceptsCmd.Flags().IntVar(&conceptsLimit, "limit", 12, "Maximum concepts to return (max 12)")
	rootCmd.AddCommand(conceptsCmd)
}

func runConcepts(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(conceptsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.ListKeyConceptsOptions{
		Limit: conceptsLimit,
	}
	response, err := engine.ListKeyConcepts(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing concepts: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertConceptsResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(conceptsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Concepts query completed", map[string]interface{}{
		"count":    len(response.Concepts),
		"duration": time.Since(start).Milliseconds(),
	})
}

// ConceptsResponseCLI contains concepts list for CLI output
type ConceptsResponseCLI struct {
	Concepts    []ConceptCLI   `json:"concepts"`
	TotalFound  int            `json:"totalFound"`
	Confidence  float64        `json:"confidence"`
	Limitations []string       `json:"limitations,omitempty"`
	Provenance  *ProvenanceCLI `json:"provenance,omitempty"`
}

type ConceptCLI struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Occurrences int      `json:"occurrences"`
	Files       []string `json:"files,omitempty"`
	Symbols     []string `json:"symbols,omitempty"`
	Description string   `json:"description,omitempty"`
	Score       float64  `json:"score"`
}

func convertConceptsResponse(resp *query.ListKeyConceptsResponse) *ConceptsResponseCLI {
	concepts := make([]ConceptCLI, 0, len(resp.Concepts))
	for _, c := range resp.Concepts {
		concept := ConceptCLI{
			Name:        c.Name,
			Category:    c.Category,
			Occurrences: c.Occurrences,
			Files:       c.Files,
			Symbols:     c.Symbols,
			Description: c.Description,
		}
		if c.Ranking != nil {
			concept.Score = c.Ranking.Score
		}
		concepts = append(concepts, concept)
	}

	result := &ConceptsResponseCLI{
		Concepts:    concepts,
		TotalFound:  resp.TotalFound,
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

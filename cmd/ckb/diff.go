package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/diff"
	"ckb/internal/logging"
)

var (
	diffBasePath     string
	diffNewPath      string
	diffOutputPath   string
	diffCommit       string
	diffIncludeHash  bool
	diffFormat       string
	diffValidateOnly bool
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Generate delta artifacts for incremental indexing",
	Long: `Generate delta artifacts by comparing two database snapshots.

Delta artifacts allow CI systems to emit pre-computed diffs, reducing
server-side ingestion from O(N) over all symbols to O(delta).

Examples:
  # Generate delta between two snapshots
  ckb diff --base /path/to/old.db --new /path/to/new.db --output delta.json

  # Generate delta for new commit (no base = initial import)
  ckb diff --new /path/to/new.db --output delta.json --commit abc123

  # Validate an existing delta file
  ckb diff --validate delta.json

  # Include entity hashes for validation
  ckb diff --base old.db --new new.db --output delta.json --include-hashes`,
	Run: runDiff,
}

func init() {
	diffCmd.Flags().StringVar(&diffBasePath, "base", "", "Path to base (old) database snapshot")
	diffCmd.Flags().StringVar(&diffNewPath, "new", "", "Path to new database snapshot")
	diffCmd.Flags().StringVar(&diffOutputPath, "output", "", "Output path for delta JSON (default: stdout)")
	diffCmd.Flags().StringVar(&diffCommit, "commit", "", "Git commit hash for the new state")
	diffCmd.Flags().BoolVar(&diffIncludeHash, "include-hashes", false, "Include entity hashes for validation")
	diffCmd.Flags().StringVar(&diffFormat, "format", "json", "Output format: json or human")
	diffCmd.Flags().BoolVar(&diffValidateOnly, "validate", false, "Validate an existing delta file instead of generating")

	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(diffFormat)

	// Validate mode
	if diffValidateOnly {
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: delta file path required for validation")
			os.Exit(1)
		}
		runDiffValidate(args[0], logger)
		return
	}

	// Generation mode
	if diffNewPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --new path is required")
		os.Exit(1)
	}

	// Check new path exists
	if _, err := os.Stat(diffNewPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: new database not found: %s\n", diffNewPath)
		os.Exit(1)
	}

	// Check base path if provided
	if diffBasePath != "" {
		if _, err := os.Stat(diffBasePath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: base database not found: %s\n", diffBasePath)
			os.Exit(1)
		}
	}

	// Generate delta
	generator := diff.NewGenerator()
	opts := diff.GenerateOptions{
		Commit:        diffCommit,
		IncludeHashes: diffIncludeHash,
	}

	delta, err := generator.GenerateFromDBs(diffBasePath, diffNewPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating delta: %v\n", err)
		os.Exit(1)
	}

	// Output
	if diffFormat == "human" {
		printDeltaHuman(delta)
	} else {
		data, err := delta.ToJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error serializing delta: %v\n", err)
			os.Exit(1)
		}

		if diffOutputPath != "" {
			if err := os.WriteFile(diffOutputPath, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
				os.Exit(1)
			}
			logger.Debug("Delta written", map[string]interface{}{
				"path":     diffOutputPath,
				"size":     len(data),
				"duration": time.Since(start).Milliseconds(),
			})
		} else {
			fmt.Println(string(data))
		}
	}

	logger.Debug("Delta generation completed", map[string]interface{}{
		"symbols_added":    delta.Stats.SymbolsAdded,
		"symbols_modified": delta.Stats.SymbolsModified,
		"symbols_deleted":  delta.Stats.SymbolsDeleted,
		"refs_added":       delta.Stats.RefsAdded,
		"refs_deleted":     delta.Stats.RefsDeleted,
		"duration":         time.Since(start).Milliseconds(),
	})
}

func runDiffValidate(path string, logger *logging.Logger) {
	// Read delta file
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading delta file: %v\n", err)
		os.Exit(1)
	}

	// Parse delta
	delta, err := diff.ParseDelta(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing delta: %v\n", err)
		os.Exit(1)
	}

	// Validate
	validator := diff.NewValidator(
		diff.WithValidationMode(diff.ValidationStrict),
		diff.WithSpotCheckPercentage(0.2), // Check 20% of entities
	)

	// For validation-only mode, we don't have the current snapshot ID
	// So we skip snapshot matching
	result := validator.Validate(delta, delta.BaseSnapshotID)

	// Output results
	if diffFormat == "human" {
		printValidationHuman(result)
	} else {
		fmt.Printf("{\n")
		fmt.Printf("  \"valid\": %v,\n", result.Valid)
		fmt.Printf("  \"spotChecked\": %d,\n", result.SpotChecked)
		fmt.Printf("  \"spotCheckPassed\": %d,\n", result.SpotCheckPassed)
		if len(result.Errors) > 0 {
			fmt.Printf("  \"errors\": [\n")
			for i, e := range result.Errors {
				comma := ","
				if i == len(result.Errors)-1 {
					comma = ""
				}
				fmt.Printf("    {\"code\": \"%s\", \"message\": \"%s\"}%s\n", e.Code, e.Message, comma)
			}
			fmt.Printf("  ],\n")
		}
		if len(result.Warnings) > 0 {
			fmt.Printf("  \"warnings\": [\n")
			for i, w := range result.Warnings {
				comma := ","
				if i == len(result.Warnings)-1 {
					comma = ""
				}
				fmt.Printf("    {\"code\": \"%s\", \"message\": \"%s\"}%s\n", w.Code, w.Message, comma)
			}
			fmt.Printf("  ]\n")
		}
		fmt.Printf("}\n")
	}

	if !result.Valid {
		os.Exit(1)
	}
}

func printDeltaHuman(delta *diff.Delta) {
	fmt.Printf("Delta Summary\n")
	fmt.Printf("=============\n")
	fmt.Printf("Schema Version: %d\n", delta.SchemaVersion)
	fmt.Printf("Commit:         %s\n", delta.Commit)
	fmt.Printf("Base Snapshot:  %s\n", truncateHash(delta.BaseSnapshotID))
	fmt.Printf("New Snapshot:   %s\n", truncateHash(delta.NewSnapshotID))
	fmt.Printf("\n")

	fmt.Printf("Changes:\n")
	fmt.Printf("  Symbols:   +%d  ~%d  -%d\n",
		delta.Stats.SymbolsAdded, delta.Stats.SymbolsModified, delta.Stats.SymbolsDeleted)
	fmt.Printf("  Refs:      +%d       -%d\n",
		delta.Stats.RefsAdded, delta.Stats.RefsDeleted)
	fmt.Printf("  Calls:     +%d       -%d\n",
		delta.Stats.CallsAdded, delta.Stats.CallsDeleted)
	fmt.Printf("  Files:     +%d  ~%d  -%d\n",
		delta.Stats.FilesAdded, delta.Stats.FilesModified, delta.Stats.FilesDeleted)
	fmt.Printf("\n")

	fmt.Printf("Totals:      +%d  ~%d  -%d\n",
		delta.Stats.TotalAdded, delta.Stats.TotalModified, delta.Stats.TotalDeleted)

	if delta.IsEmpty() {
		fmt.Printf("\nNo changes detected.\n")
	}
}

func printValidationHuman(result *diff.ValidationResult) {
	if result.Valid {
		fmt.Printf("Validation: PASSED\n")
	} else {
		fmt.Printf("Validation: FAILED\n")
	}

	fmt.Printf("Spot Checks: %d/%d passed\n", result.SpotCheckPassed, result.SpotChecked)

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Printf("  - [%s] %s\n", e.Code, e.Message)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Printf("  - [%s] %s\n", w.Code, w.Message)
		}
	}
}

func truncateHash(hash string) string {
	if len(hash) > 20 {
		return hash[:20] + "..."
	}
	return hash
}

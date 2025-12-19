package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	indexDryRun bool
	indexLang   string
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Generate SCIP index for the repository",
	Long: `Automatically detects the project language and runs the appropriate SCIP indexer.

Supported languages:
  - Go (scip-go)
  - TypeScript/JavaScript (scip-typescript)
  - Python (scip-python)
  - Rust (rust-analyzer)
  - Java (scip-java)

The generated index enables precise cross-references, call graphs, and impact analysis.`,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().BoolVar(&indexDryRun, "dry-run", false, "Show what would be run without executing")
	indexCmd.Flags().StringVar(&indexLang, "lang", "", "Force specific language (go, typescript, python, rust, java)")
	rootCmd.AddCommand(indexCmd)
}

// languageInfo describes a language and its SCIP indexer.
type languageInfo struct {
	name        string
	markers     []string // Files that indicate this language
	indexerCmd  string   // Command to check if indexer is installed
	indexCmd    []string // Full command to run indexer
	installHint string   // How to install the indexer
	outputFile  string   // Default output file name
}

var supportedLanguages = []languageInfo{
	{
		name:        "go",
		markers:     []string{"go.mod", "go.sum"},
		indexerCmd:  "scip-go",
		indexCmd:    []string{"scip-go"},
		installHint: "go install github.com/sourcegraph/scip-go/cmd/scip-go@latest",
		outputFile:  "index.scip",
	},
	{
		name:        "typescript",
		markers:     []string{"tsconfig.json", "package.json"},
		indexerCmd:  "scip-typescript",
		indexCmd:    []string{"scip-typescript", "index"},
		installHint: "npm install -g @sourcegraph/scip-typescript",
		outputFile:  "index.scip",
	},
	{
		name:        "python",
		markers:     []string{"pyproject.toml", "setup.py", "requirements.txt"},
		indexerCmd:  "scip-python",
		indexCmd:    []string{"scip-python", "index", "."},
		installHint: "pip install scip-python",
		outputFile:  "index.scip",
	},
	{
		name:        "rust",
		markers:     []string{"Cargo.toml"},
		indexerCmd:  "rust-analyzer",
		indexCmd:    []string{"rust-analyzer", "scip", "."},
		installHint: "See https://rust-analyzer.github.io/manual.html#installation",
		outputFile:  "index.scip",
	},
	{
		name:        "java",
		markers:     []string{"pom.xml", "build.gradle", "build.gradle.kts"},
		indexerCmd:  "scip-java",
		indexCmd:    []string{"scip-java", "index"},
		installHint: "See https://sourcegraph.github.io/scip-java/",
		outputFile:  "index.scip",
	},
}

func runIndex(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Detect or use specified language
	var lang *languageInfo
	if indexLang != "" {
		lang = findLanguageByName(indexLang)
		if lang == nil {
			return fmt.Errorf("unsupported language: %s\nSupported: go, typescript, python, rust, java", indexLang)
		}
	} else {
		lang = detectLanguage(cwd)
		if lang == nil {
			fmt.Println("Could not auto-detect project language.")
			fmt.Println("Supported languages: go, typescript, python, rust, java")
			fmt.Println("\nUse --lang to specify manually:")
			fmt.Println("  ckb index --lang go")
			return nil
		}
	}

	fmt.Printf("Detected language: %s\n", lang.name)

	// Check if indexer is installed
	if !isCommandAvailable(lang.indexerCmd) {
		fmt.Printf("\nIndexer '%s' not found.\n", lang.indexerCmd)
		fmt.Println("Install it with:")
		fmt.Printf("  %s\n", lang.installHint)
		return nil
	}

	fmt.Printf("Indexer: %s\n", lang.indexerCmd)
	fmt.Printf("Command: %s\n", strings.Join(lang.indexCmd, " "))

	if indexDryRun {
		fmt.Println("\n[dry-run] Would execute the above command")
		return nil
	}

	fmt.Println("\nGenerating SCIP index...")

	// Run the indexer
	execCmd := exec.Command(lang.indexCmd[0], lang.indexCmd[1:]...)
	execCmd.Dir = cwd
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("indexer failed: %w", err)
	}

	// Check if output file was created
	outputPath := filepath.Join(cwd, lang.outputFile)
	if info, statErr := os.Stat(outputPath); statErr == nil {
		fmt.Printf("\nIndex created: %s (%.2f MB)\n", outputPath, float64(info.Size())/1024/1024)
	}

	fmt.Println("\nDone! CKB now has access to precise cross-references.")
	fmt.Println("Run 'ckb status' to verify the index is loaded.")

	return nil
}

func findLanguageByName(name string) *languageInfo {
	name = strings.ToLower(name)
	// Handle common aliases
	switch name {
	case "ts", "js", "javascript":
		name = "typescript"
	case "py":
		name = "python"
	case "rs":
		name = "rust"
	}

	for i := range supportedLanguages {
		if supportedLanguages[i].name == name {
			return &supportedLanguages[i]
		}
	}
	return nil
}

func detectLanguage(root string) *languageInfo {
	for i := range supportedLanguages {
		lang := &supportedLanguages[i]
		for _, marker := range lang.markers {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				return lang
			}
		}
	}
	return nil
}

func isCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/project"
)

var (
	indexForce  bool
	indexDryRun bool
	indexLang   string
	indexCompdb string // Path to compile_commands.json for C/C++
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build SCIP index for full code intelligence",
	Long: `Auto-detects project language and runs the appropriate SCIP indexer.

This command enables enhanced code intelligence features like findReferences,
getCallGraph, and analyzeImpact.

Supported languages:
  - Go (scip-go)
  - TypeScript/JavaScript (scip-typescript)
  - Python (scip-python)
  - Rust (rust-analyzer)
  - Java (scip-java)
  - C/C++ (scip-clang) - requires compile_commands.json
  - Dart (scip_dart)
  - Ruby (scip-ruby)
  - C# (scip-dotnet) - requires .NET 8+
  - PHP (scip-php) - requires PHP 8.2+, composer install

For Kotlin: use scip-java with Gradle plugin integration.
  - Gradle Kotlin: supported
  - Maven Kotlin: auto-config NOT supported
  - Gradle Android: NOT supported yet
See: https://sourcegraph.github.io/scip-java/

Examples:
  ckb index              # Auto-detect language and index
  ckb index --dry-run    # Show what would be run without executing
  ckb index --force      # Re-index even if index.scip exists
  ckb index --lang go    # Force specific language
  ckb index --lang cpp --compdb build/compile_commands.json`,
	Run: runIndex,
}

func init() {
	indexCmd.Flags().BoolVar(&indexForce, "force", false, "Re-index even if index.scip exists")
	indexCmd.Flags().BoolVar(&indexDryRun, "dry-run", false, "Show what would be run without executing")
	indexCmd.Flags().StringVar(&indexLang, "lang", "", "Force specific language (go, ts, py, rs, java, cpp, dart, rb, cs, php)")
	indexCmd.Flags().StringVar(&indexCompdb, "compdb", "", "Path to compile_commands.json (C/C++ only)")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, args []string) {
	repoRoot := mustGetRepoRoot()

	// Check if .ckb directory exists, auto-init if not
	ckbDir := filepath.Join(repoRoot, ".ckb")
	if _, err := os.Stat(ckbDir); os.IsNotExist(err) {
		fmt.Println("No .ckb directory found. Initializing...")
		if err := os.MkdirAll(ckbDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating .ckb directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("  Created .ckb/")
		fmt.Println()
	}

	// Check if index already exists
	indexPath := filepath.Join(repoRoot, "index.scip")
	if !indexForce {
		if info, err := os.Stat(indexPath); err == nil {
			fmt.Printf("SCIP index already exists: %s (%.2f MB)\n", indexPath, float64(info.Size())/1024/1024)
			fmt.Println("Run 'ckb index --force' to re-index")
			os.Exit(0)
		}
	}

	// Detect or use specified language
	var lang project.Language
	var manifest string

	if indexLang != "" {
		lang = parseLanguageFlag(indexLang)
		if lang == project.LangUnknown {
			fmt.Fprintf(os.Stderr, "Unsupported language: %s\n", indexLang)
			fmt.Fprintln(os.Stderr, "Supported: go, ts, py, rs, java, cpp, dart, rb, cs, php")
			os.Exit(1)
		}
		manifest = "(specified via --lang)"
	} else {
		var allLangs []project.Language
		lang, manifest, allLangs = project.DetectAllLanguages(repoRoot)

		if lang == project.LangUnknown {
			fmt.Fprintln(os.Stderr, "Could not detect project language.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Supported manifest files:")
			fmt.Fprintln(os.Stderr, "  Go:         go.mod")
			fmt.Fprintln(os.Stderr, "  TypeScript: package.json + tsconfig.json")
			fmt.Fprintln(os.Stderr, "  Python:     pyproject.toml, requirements.txt, setup.py")
			fmt.Fprintln(os.Stderr, "  Rust:       Cargo.toml")
			fmt.Fprintln(os.Stderr, "  Java:       pom.xml, build.gradle")
			fmt.Fprintln(os.Stderr, "  Kotlin:     build.gradle.kts")
			fmt.Fprintln(os.Stderr, "  C/C++:      compile_commands.json")
			fmt.Fprintln(os.Stderr, "  Dart:       pubspec.yaml")
			fmt.Fprintln(os.Stderr, "  Ruby:       Gemfile, *.gemspec")
			fmt.Fprintln(os.Stderr, "  C#:         *.csproj, *.sln")
			fmt.Fprintln(os.Stderr, "  PHP:        composer.json")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Or specify manually: ckb index --lang go")
			os.Exit(1)
		}

		// Error if multiple languages detected - don't silently default
		if len(allLangs) > 1 {
			fmt.Fprintln(os.Stderr, "Multiple languages detected:")
			for _, l := range allLangs {
				fmt.Fprintf(os.Stderr, "  - %s\n", project.LanguageDisplayName(l))
			}
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Use --lang to specify which language to index:")
			fmt.Fprintf(os.Stderr, "  ckb index --lang %s\n", allLangs[0])
			os.Exit(1)
		}
	}

	fmt.Printf("Detected %s project (from %s)\n", project.LanguageDisplayName(lang), manifest)

	// Get indexer info
	indexer := project.GetIndexerInfo(lang)
	if indexer == nil {
		fmt.Fprintf(os.Stderr, "No SCIP indexer available for %s\n", project.LanguageDisplayName(lang))
		os.Exit(1)
	}

	// Build command - some languages need special handling
	command := indexer.Command
	switch lang {
	case project.LangCpp:
		cppCmd, err := project.BuildCppCommand(repoRoot, indexCompdb)
		if err != nil || cppCmd == "" {
			fmt.Fprintln(os.Stderr, "compile_commands.json not found.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Generate it with CMake:")
			fmt.Fprintln(os.Stderr, "  cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -B build")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Or specify path:")
			fmt.Fprintln(os.Stderr, "  ckb index --lang cpp --compdb build/compile_commands.json")
			os.Exit(1)
		}
		command = cppCmd

	case project.LangRuby:
		rubyCmd, err := project.BuildRubyCommand(repoRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bundle not found. Install Bundler:")
			fmt.Fprintln(os.Stderr, "  gem install bundler")
			os.Exit(1)
		}
		command = rubyCmd

	case project.LangPHP:
		warning, err := project.ValidatePHPSetup(repoRoot)
		if warning != "" {
			fmt.Printf("Warning: %s\n", warning)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "scip-php not installed.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Install with:")
			fmt.Fprintln(os.Stderr, "  composer require --dev davidrjenni/scip-php")
			fmt.Fprintln(os.Stderr, "  composer install")
			os.Exit(1)
		}
	}

	// Check if indexer is installed
	if !isIndexerInstalled(indexer.CheckCommand) {
		fmt.Println()
		fmt.Printf("Indexer not found: %s\n", indexer.CheckCommand)
		fmt.Println()
		fmt.Println("Install with:")
		fmt.Printf("  %s\n", indexer.InstallCommand)
		os.Exit(1)
	}

	fmt.Printf("Indexer: %s\n", indexer.CheckCommand)
	fmt.Printf("Command: %s\n", command)

	// Dry run - show command without executing
	if indexDryRun {
		fmt.Println()
		fmt.Println("[dry-run] Would execute the above command")
		os.Exit(0)
	}

	// Run the indexer
	fmt.Println()
	fmt.Println("Generating SCIP index...")
	fmt.Println()

	start := time.Now()
	err := runIndexerCommand(repoRoot, command)
	duration := time.Since(start)

	if err != nil {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Indexing failed.")
		fmt.Fprintln(os.Stderr, "")
		showTroubleshooting(lang)
		os.Exit(1)
	}

	// Verify index was created
	info, err := os.Stat(indexPath)
	if os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Warning: Indexer completed but index.scip was not created.")
		fmt.Fprintln(os.Stderr, "Check the indexer output above for errors.")
		os.Exit(1)
	}

	// Save project config
	config := &project.ProjectConfig{
		Language:     lang,
		Indexer:      indexer.CheckCommand,
		ManifestPath: manifest,
		DetectedAt:   time.Now(),
	}
	if saveErr := project.SaveConfig(repoRoot, config); saveErr != nil {
		// Non-fatal, just warn
		fmt.Fprintf(os.Stderr, "Warning: Could not save project config: %v\n", saveErr)
	}

	// Success message
	fmt.Println()
	fmt.Printf("Done! Indexed in %.1fs\n", duration.Seconds())
	fmt.Printf("Index: %s (%.2f MB)\n", indexPath, float64(info.Size())/1024/1024)
	fmt.Println()
	fmt.Println("Full code intelligence now available:")
	fmt.Println("  findReferences - Find all usages of a symbol")
	fmt.Println("  getCallGraph   - Trace caller/callee relationships")
	fmt.Println("  analyzeImpact  - Assess change impact")
	fmt.Println()
	fmt.Println("Run 'ckb status' to verify.")
}

// parseLanguageFlag converts a language flag to a Language type.
func parseLanguageFlag(flag string) project.Language {
	flag = strings.ToLower(flag)
	switch flag {
	case "go", "golang":
		return project.LangGo
	case "ts", "typescript":
		return project.LangTypeScript
	case "js", "javascript":
		return project.LangJavaScript
	case "py", "python":
		return project.LangPython
	case "rs", "rust":
		return project.LangRust
	case "java":
		return project.LangJava
	case "kt", "kotlin":
		return project.LangKotlin
	case "c", "c++", "cc", "cxx", "cpp":
		return project.LangCpp
	case "dart":
		return project.LangDart
	case "rb", "ruby":
		return project.LangRuby
	case "cs", "c#", "csharp", "dotnet":
		return project.LangCSharp
	case "php":
		return project.LangPHP
	default:
		return project.LangUnknown
	}
}

// isIndexerInstalled checks if the indexer command is available.
func isIndexerInstalled(command string) bool {
	// Extract just the binary name (first word)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	binary := parts[0]

	// Try to find in PATH
	_, err := exec.LookPath(binary)
	return err == nil
}

// runIndexerCommand runs the indexer and streams output.
func runIndexerCommand(dir, command string) error {
	// Split command into parts
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = dir

	// Capture stderr for error messages, stream both
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Print captured stderr
		if stderr.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Indexer output:")
			fmt.Fprintln(os.Stderr, stderr.String())
		}
		return fmt.Errorf("indexer failed: %w", err)
	}

	return nil
}

// showTroubleshooting shows language-specific troubleshooting tips.
func showTroubleshooting(lang project.Language) {
	switch lang {
	case project.LangGo:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Go:")
		fmt.Fprintln(os.Stderr, "  1. Ensure your code compiles: go build ./...")
		fmt.Fprintln(os.Stderr, "  2. Check for missing dependencies: go mod tidy")
		fmt.Fprintln(os.Stderr, "  3. Try updating scip-go: go install github.com/sourcegraph/scip-go@latest")

	case project.LangTypeScript, project.LangJavaScript:
		fmt.Fprintln(os.Stderr, "Troubleshooting for TypeScript/JavaScript:")
		fmt.Fprintln(os.Stderr, "  1. Ensure dependencies are installed: npm install")
		fmt.Fprintln(os.Stderr, "  2. Check for TypeScript errors: npx tsc --noEmit")
		fmt.Fprintln(os.Stderr, "  3. Ensure tsconfig.json exists for TypeScript projects")

	case project.LangPython:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Python:")
		fmt.Fprintln(os.Stderr, "  1. Ensure dependencies are installed: pip install -r requirements.txt")
		fmt.Fprintln(os.Stderr, "  2. Check for syntax errors: python -m py_compile *.py")

	case project.LangRust:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Rust:")
		fmt.Fprintln(os.Stderr, "  1. Ensure your code compiles: cargo build")
		fmt.Fprintln(os.Stderr, "  2. Check rust-analyzer is installed: rustup component add rust-analyzer")

	case project.LangJava, project.LangKotlin:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Java/Kotlin:")
		fmt.Fprintln(os.Stderr, "  1. Ensure your code compiles with your build tool")
		fmt.Fprintln(os.Stderr, "  2. Check scip-java is properly installed via Coursier")

	case project.LangCpp:
		fmt.Fprintln(os.Stderr, "Troubleshooting for C/C++:")
		fmt.Fprintln(os.Stderr, "  1. Ensure compile_commands.json exists and is valid")
		fmt.Fprintln(os.Stderr, "  2. Run from project root, even if compdb is in build/")
		fmt.Fprintln(os.Stderr, "  3. Generate with: cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -B build")

	case project.LangDart:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Dart:")
		fmt.Fprintln(os.Stderr, "  1. Ensure dependencies are fetched: dart pub get")
		fmt.Fprintln(os.Stderr, "  2. Check scip_dart is activated: dart pub global activate scip_dart")

	case project.LangRuby:
		fmt.Fprintln(os.Stderr, "Troubleshooting for Ruby:")
		fmt.Fprintln(os.Stderr, "  1. Ensure dependencies are installed: bundle install")
		fmt.Fprintln(os.Stderr, "  2. If using Sorbet, check sorbet/config exists")
		fmt.Fprintln(os.Stderr, "  3. Check scip-ruby is installed from releases")

	case project.LangCSharp:
		fmt.Fprintln(os.Stderr, "Troubleshooting for C#:")
		fmt.Fprintln(os.Stderr, "  1. Ensure .NET 8+ is installed: dotnet --version")
		fmt.Fprintln(os.Stderr, "  2. Check scip-dotnet is on PATH: $HOME/.dotnet/tools")
		fmt.Fprintln(os.Stderr, "  3. Ensure project builds: dotnet build")

	case project.LangPHP:
		fmt.Fprintln(os.Stderr, "Troubleshooting for PHP:")
		fmt.Fprintln(os.Stderr, "  1. Ensure PHP 8.2+ is installed: php --version")
		fmt.Fprintln(os.Stderr, "  2. Run: composer install")
		fmt.Fprintln(os.Stderr, "  3. Check vendor/bin/scip-php exists")

	default:
		fmt.Fprintln(os.Stderr, "Check that the project compiles without errors first.")
	}
}

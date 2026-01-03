package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/config"
	"ckb/internal/incremental"
	"ckb/internal/index"
	"ckb/internal/logging"
	"ckb/internal/project"
	"ckb/internal/repostate"
	"ckb/internal/storage"
	"ckb/internal/tier"
)

var (
	indexForce         bool
	indexDryRun        bool
	indexLang          string
	indexCompdb        string        // Path to compile_commands.json for C/C++
	indexTier          string        // Tier to validate (enhanced, full)
	indexAllowFb       bool          // Allow fallback if tier not satisfied
	indexShowTier      bool          // Show tier summary after indexing
	indexWatch         bool          // Watch for changes and auto-reindex
	indexWatchInterval time.Duration // Watch mode polling interval
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
	indexCmd.Flags().StringVar(&indexTier, "tier", "", "Validate tier requirements before indexing (enhanced, full)")
	indexCmd.Flags().BoolVar(&indexAllowFb, "allow-fallback", true, "Continue if tier requirements not met (default: true)")
	indexCmd.Flags().BoolVar(&indexShowTier, "show-tier", true, "Show tier summary after indexing (default: true)")
	indexCmd.Flags().BoolVar(&indexWatch, "watch", false, "Watch for changes and auto-reindex")
	indexCmd.Flags().DurationVar(&indexWatchInterval, "watch-interval", 30*time.Second,
		"Watch mode polling interval (min 5s, max 5m)")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, args []string) {
	repoRoot := mustGetRepoRoot()

	// Check if this is an initialized CKB project
	ckbDir := filepath.Join(repoRoot, ".ckb")
	if _, err := os.Stat(ckbDir); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Error: Not a CKB project.")
		fmt.Fprintln(os.Stderr, "Run 'ckb init' first to initialize this directory.")
		os.Exit(1)
	}

	// Get SCIP index path from config (default: .scip/index.scip)
	indexPath := ".scip/index.scip"
	if cfg, loadErr := config.LoadConfig(repoRoot); loadErr == nil && cfg.Backends.Scip.IndexPath != "" {
		indexPath = cfg.Backends.Scip.IndexPath
	}
	// Make absolute if relative
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repoRoot, indexPath)
	}

	// Check index freshness (unless --force)
	if !indexForce {
		meta, err := index.LoadMeta(ckbDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not load index metadata: %v\n", err)
		}

		if meta != nil {
			freshness := meta.CheckFreshness(repoRoot)
			if freshness.Fresh {
				// Show brief info about current index
				if info, err := os.Stat(indexPath); err == nil {
					commitInfo := ""
					if freshness.IndexedCommit != "" {
						commitInfo = fmt.Sprintf(" (HEAD = %s)", shortHash(freshness.IndexedCommit))
					}
					fmt.Printf("Index is current%s\n", commitInfo)
					fmt.Printf("  %d files, %.2f MB\n", meta.FileCount, float64(info.Size())/1024/1024)
					fmt.Println("Nothing to do. Use --force to re-index.")
					os.Exit(0)
				}
			} else {
				// Show why index is stale
				fmt.Printf("Index is stale: %s\n", freshness.Reason)
			}
		} else {
			// No metadata but index.scip exists - legacy index
			if info, err := os.Stat(indexPath); err == nil {
				fmt.Printf("Found legacy index: %s (%.2f MB)\n", indexPath, float64(info.Size())/1024/1024)
				fmt.Println("Re-indexing to enable freshness tracking...")
			}
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

	case project.LangGo:
		// Ensure output directory exists
		outputDir := filepath.Dir(indexPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating index directory %s: %v\n", outputDir, err)
			os.Exit(1)
		}
		// Add --output flag to use configured path
		command = fmt.Sprintf("%s --output %s", command, indexPath)
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

	// Try incremental indexing for supported languages (unless --force)
	if !indexForce && project.SupportsIncrementalIndexing(lang) {
		if tryIncrementalIndex(repoRoot, ckbDir, lang) {
			// Incremental succeeded, we're done
			return
		}
		// Fall through to full index
	}

	// Acquire lock to prevent concurrent indexing
	lock, err := index.AcquireLock(ckbDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer lock.Release()

	// Run the indexer
	fmt.Println()
	fmt.Println("Generating SCIP index...")
	fmt.Println()

	start := time.Now()
	err = runIndexerCommand(repoRoot, command)
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

	// Save index metadata for freshness tracking
	meta := &index.IndexMeta{
		CreatedAt:   time.Now(),
		FileCount:   countSourceFiles(repoRoot, lang),
		Duration:    duration.Round(time.Millisecond * 100).String(),
		Indexer:     indexer.CheckCommand,
		IndexerArgs: strings.Fields(command),
	}

	// Capture git state if available
	if rs, err := repostate.ComputeRepoState(repoRoot); err == nil {
		meta.CommitHash = rs.HeadCommit
		meta.RepoStateID = rs.RepoStateID
	}

	if saveErr := meta.Save(ckbDir); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not save index metadata: %v\n", saveErr)
	}

	// Populate incremental tracking tables for supported languages
	if project.SupportsIncrementalIndexing(lang) {
		populateIncrementalTracking(repoRoot, lang)
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

	// Show tier summary if enabled
	if indexShowTier {
		showTierSummary(repoRoot, lang)
	}

	fmt.Println("Run 'ckb status' to verify.")

	// Start watch mode if enabled
	if indexWatch {
		fmt.Println()
		runIndexWatchLoop(repoRoot, ckbDir, lang)
	}
}

// showTierSummary displays the current tier status after indexing.
func showTierSummary(repoRoot string, lang project.Language) {
	// Convert project.Language to tier.Language
	tierLang, ok := tier.ParseLanguage(string(lang))
	if !ok {
		return
	}

	ctx := context.Background()
	runner := tier.NewCachingRunner(tier.NewRealRunner(5 * time.Second))
	detector := tier.NewToolDetector(runner, 5*time.Second)

	status := detector.DetectLanguageTier(ctx, tierLang)

	fmt.Println("Tier Status:")
	fmt.Printf("  %s: %s tier\n", status.DisplayName, tierDisplayNameShort(status.ToolTier))

	// Show available capabilities
	if len(status.Capabilities) > 0 {
		fmt.Print("  Capabilities: ")
		caps := []string{}
		for cap, enabled := range status.Capabilities {
			if enabled {
				caps = append(caps, cap)
			}
		}
		fmt.Println(strings.Join(caps, ", "))
	}

	// Show upgrade hint if not at full tier
	if status.ToolTier < tier.TierFull {
		switch status.ToolTier {
		case tier.TierBasic:
			fmt.Println("  Tip: Run 'ckb doctor --tier enhanced' to see what's needed for more features.")
		case tier.TierEnhanced:
			fmt.Println("  Tip: Run 'ckb doctor --tier full' to see what's needed for LSP features.")
		}
	}
	fmt.Println()
}

// tierDisplayNameShort returns a short tier name.
func tierDisplayNameShort(t tier.AnalysisTier) string {
	switch t {
	case tier.TierBasic:
		return "basic"
	case tier.TierEnhanced:
		return "enhanced"
	case tier.TierFull:
		return "full"
	default:
		return "unknown"
	}
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

// shortHash returns the first 7 characters of a git hash.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// countSourceFiles counts source files in the repository for the given language.
func countSourceFiles(root string, lang project.Language) int {
	extensions := getSourceExtensions(lang)
	if len(extensions) == 0 {
		return 0
	}

	extSet := make(map[string]bool)
	for _, ext := range extensions {
		extSet[ext] = true
	}

	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // Skip errors, continue walking
		}
		if d.IsDir() {
			// Skip common non-source directories
			switch d.Name() {
			case ".git", ".ckb", "node_modules", "vendor", ".venv", "__pycache__", "target", "build", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if extSet[ext] {
			count++
		}
		return nil
	})
	return count
}

// getSourceExtensions returns file extensions for the given language.
func getSourceExtensions(lang project.Language) []string {
	switch lang {
	case project.LangGo:
		return []string{".go"}
	case project.LangTypeScript:
		return []string{".ts", ".tsx"}
	case project.LangJavaScript:
		return []string{".js", ".jsx"}
	case project.LangPython:
		return []string{".py"}
	case project.LangRust:
		return []string{".rs"}
	case project.LangJava:
		return []string{".java"}
	case project.LangKotlin:
		return []string{".kt", ".kts"}
	case project.LangCpp:
		return []string{".cpp", ".cc", ".cxx", ".c", ".h", ".hpp"}
	case project.LangDart:
		return []string{".dart"}
	case project.LangRuby:
		return []string{".rb"}
	case project.LangCSharp:
		return []string{".cs"}
	case project.LangPHP:
		return []string{".php"}
	default:
		return nil
	}
}

// tryIncrementalIndex attempts incremental indexing for supported languages.
// Returns true if incremental succeeded (caller should return early).
// Returns false if full reindex is needed.
func tryIncrementalIndex(repoRoot, ckbDir string, lang project.Language) bool {
	dbPath := filepath.Join(ckbDir, "ckb.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// No database = no previous index
		return false
	}

	// Create logger (quiet for CLI - only errors)
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	// Open database
	db, err := storage.Open(repoRoot, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open database for incremental: %v\n", err)
		return false
	}
	defer func() { _ = db.Close() }()

	// Get SCIP index path from config (default: .scip/index.scip)
	indexPath := ".scip/index.scip"
	if cfg, loadErr := config.LoadConfig(repoRoot); loadErr == nil && cfg.Backends.Scip.IndexPath != "" {
		indexPath = cfg.Backends.Scip.IndexPath
	}

	// Create incremental config with the configured index path
	incConfig := &incremental.Config{
		IndexPath:            indexPath,
		IncrementalThreshold: 50,
		IndexTests:           false,
	}

	// Create incremental indexer
	indexer := incremental.NewIncrementalIndexer(repoRoot, db, incConfig, logger)

	// Check if we need full reindex
	needsFull, reason := indexer.NeedsFullReindex()
	if needsFull {
		fmt.Printf("Full reindex required: %s\n", reason)
		return false
	}

	// Acquire lock to prevent concurrent indexing
	lock, err := index.AcquireLock(ckbDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return false
	}
	defer lock.Release()

	// Check if incremental is available for this language
	canUse, reason := indexer.CanUseIncremental(lang)
	if !canUse {
		fmt.Printf("Incremental not available: %s\n", reason)
		return false
	}

	// Try incremental update
	ctx := context.Background()
	stats, err := indexer.IndexIncrementalWithLang(ctx, "", lang)
	if err != nil {
		// Check for specific errors that should fall back to full reindex
		if strings.Contains(err.Error(), "not supported") ||
			strings.Contains(err.Error(), "not installed") {
			fmt.Printf("Incremental not available: %v\n", err)
			return false
		}
		fmt.Printf("Incremental failed: %v\n", err)
		fmt.Println("Falling back to full reindex...")
		return false
	}

	// Get current state for display
	state := indexer.GetIndexState()

	// Format and display results
	fmt.Println(incremental.FormatStats(stats, state))

	return true
}

// populateIncrementalTracking sets up tracking tables after a full index.
// This enables subsequent incremental updates.
func populateIncrementalTracking(repoRoot string, lang project.Language) {
	// Create logger (quiet for CLI - only errors)
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	// Open database
	db, err := storage.Open(repoRoot, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open database for incremental tracking: %v\n", err)
		return
	}
	defer func() { _ = db.Close() }()

	// Get SCIP index path from config (default: .scip/index.scip)
	indexPath := ".scip/index.scip"
	if cfg, loadErr := config.LoadConfig(repoRoot); loadErr == nil && cfg.Backends.Scip.IndexPath != "" {
		indexPath = cfg.Backends.Scip.IndexPath
	}

	// Create incremental config with the configured index path
	incConfig := &incremental.Config{
		IndexPath:            indexPath,
		IncrementalThreshold: 50,
		IndexTests:           false,
	}

	// Create incremental indexer
	indexer := incremental.NewIncrementalIndexer(repoRoot, db, incConfig, logger)

	// Populate tracking tables from the full index
	if err := indexer.PopulateAfterFullIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not populate incremental tracking: %v\n", err)
		return
	}

	fmt.Println("  Incremental tracking enabled for future updates")
}

// runIndexWatchLoop watches for changes and runs incremental updates.
func runIndexWatchLoop(repoRoot, ckbDir string, lang project.Language) {
	// Validate and clamp watch interval
	interval := indexWatchInterval
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if interval > 5*time.Minute {
		interval = 5 * time.Minute
	}

	fmt.Printf("Watching for changes... (polling every %s, Ctrl+C to stop)\n", interval)

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track last known state for change detection
	lastCommit := ""
	if meta, err := index.LoadMeta(ckbDir); err == nil && meta != nil {
		lastCommit = meta.CommitHash
	}

	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopping watch...")
			return

		case <-ticker.C:
			// Check if there are new commits
			currentCommit := getCurrentCommit(repoRoot)
			if currentCommit == "" || currentCommit == lastCommit {
				continue
			}

			fmt.Printf("\nChanges detected (commit %s -> %s)\n", shortHash(lastCommit), shortHash(currentCommit))

			// Try incremental update for supported languages
			if project.SupportsIncrementalIndexing(lang) {
				if tryIncrementalIndex(repoRoot, ckbDir, lang) {
					lastCommit = currentCommit
					fmt.Println("Watching for changes...")
					continue
				}
			}

			// Fall back to checking freshness and full reindex if needed
			meta, err := index.LoadMeta(ckbDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Could not load index metadata: %v\n", err)
				continue
			}

			if meta != nil {
				freshness := meta.CheckFreshness(repoRoot)
				if !freshness.Fresh {
					fmt.Printf("Index stale: %s\n", freshness.Reason)
					fmt.Println("Run 'ckb index --force' to rebuild.")
				}
			}

			lastCommit = currentCommit
			fmt.Println("Watching for changes...")
		}
	}
}

// getCurrentCommit returns the current HEAD commit hash.
func getCurrentCommit(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

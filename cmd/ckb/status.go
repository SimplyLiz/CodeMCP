package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/index"
	"ckb/internal/project"
	"ckb/internal/query"
	"ckb/internal/tier"
)

var (
	statusFormat string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CKB system status",
	Long:  "Display the current status of CKB backends, cache, and repository state",
	Run:   runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", "human", "Output format (json, human)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(statusFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Get status from Query Engine
	response, err := engine.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting status: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertStatusResponse(response)

	// Add index freshness status
	ckbDir := filepath.Join(repoRoot, ".ckb")
	cliResponse.IndexStatus = getIndexStatus(ckbDir, repoRoot)

	// Add change impact analysis status
	cliResponse.ChangeImpactStatus = getChangeImpactStatus(repoRoot)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(statusFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	duration := time.Since(start).Milliseconds()
	if statusFormat == "human" {
		fmt.Printf("\n(Query took %dms)\n", duration)
	}
}

// StatusResponseCLI contains the complete system status for CLI output
type StatusResponseCLI struct {
	CkbVersion         string                  `json:"ckbVersion"`
	Tier               *tier.TierInfo          `json:"tier"`
	RepoState          *query.RepoState        `json:"repoState"`
	IndexStatus        *IndexStatusCLI         `json:"indexStatus,omitempty"`
	ChangeImpactStatus *ChangeImpactStatusCLI  `json:"changeImpactStatus,omitempty"`
	Backends           []BackendStatusCLI      `json:"backends"`
	Cache              CacheStatusCLI          `json:"cache"`
	Healthy            bool                    `json:"healthy"`
}

// ChangeImpactStatusCLI describes the availability of change impact analysis features
type ChangeImpactStatusCLI struct {
	Coverage   *CoverageStatusCLI   `json:"coverage,omitempty"`
	Codeowners *CodeownersStatusCLI `json:"codeowners,omitempty"`
	Language   string               `json:"language,omitempty"`
}

// CoverageStatusCLI describes coverage file status
type CoverageStatusCLI struct {
	Found      bool      `json:"found"`
	Path       string    `json:"path,omitempty"`
	Age        string    `json:"age,omitempty"`
	ModTime    time.Time `json:"modTime,omitempty"`
	Stale      bool      `json:"stale,omitempty"`
	GenerateCmd string   `json:"generateCmd,omitempty"`
}

// CodeownersStatusCLI describes CODEOWNERS file status
type CodeownersStatusCLI struct {
	Found        bool   `json:"found"`
	Path         string `json:"path,omitempty"`
	TeamCount    int    `json:"teamCount,omitempty"`
	PatternCount int    `json:"patternCount,omitempty"`
}

// IndexStatusCLI describes the state of the SCIP index
type IndexStatusCLI struct {
	Exists         bool      `json:"exists"`
	Fresh          bool      `json:"fresh"`
	Reason         string    `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"createdAt,omitempty"`
	IndexAge       string    `json:"indexAge,omitempty"`
	CommitHash     string    `json:"commitHash,omitempty"`
	FileCount      int       `json:"fileCount,omitempty"`
	CommitsBehind  int       `json:"commitsBehind,omitempty"`
	HasUncommitted bool      `json:"hasUncommitted,omitempty"`
}

// BackendStatusCLI describes the status of a backend
type BackendStatusCLI struct {
	ID           string   `json:"id"`
	Available    bool     `json:"available"`
	Healthy      bool     `json:"healthy"`
	Capabilities []string `json:"capabilities"`
	Details      string   `json:"details,omitempty"`
}

// CacheStatusCLI describes the cache state
type CacheStatusCLI struct {
	QueryCount int     `json:"queryCount"`
	ViewCount  int     `json:"viewCount"`
	HitRate    float64 `json:"hitRate"`
	SizeBytes  int64   `json:"sizeBytes"`
}

func convertStatusResponse(resp *query.StatusResponse) *StatusResponseCLI {
	backends := make([]BackendStatusCLI, 0, len(resp.Backends))
	for _, b := range resp.Backends {
		backends = append(backends, BackendStatusCLI{
			ID:           b.Id,
			Available:    b.Available,
			Healthy:      b.Healthy,
			Capabilities: b.Capabilities,
			Details:      b.Details,
		})
	}

	var cache CacheStatusCLI
	if resp.Cache != nil {
		cache = CacheStatusCLI{
			QueryCount: resp.Cache.QueriesCached,
			ViewCount:  resp.Cache.ViewsCached,
			HitRate:    resp.Cache.HitRate,
			SizeBytes:  resp.Cache.SizeBytes,
		}
	}

	return &StatusResponseCLI{
		CkbVersion: resp.CkbVersion,
		Tier:       resp.Tier,
		RepoState:  resp.RepoState,
		Backends:   backends,
		Cache:      cache,
		Healthy:    resp.Healthy,
	}
}

// getIndexStatus retrieves index freshness information
func getIndexStatus(ckbDir, repoRoot string) *IndexStatusCLI {
	indexPath := filepath.Join(repoRoot, "index.scip")

	// Check if index file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return &IndexStatusCLI{
			Exists: false,
			Fresh:  false,
			Reason: "no index found - run 'ckb index' to create one",
		}
	}

	// Load index metadata
	meta, err := index.LoadMeta(ckbDir)
	if err != nil {
		return &IndexStatusCLI{
			Exists: true,
			Fresh:  false,
			Reason: "could not read index metadata",
		}
	}

	if meta == nil {
		return &IndexStatusCLI{
			Exists: true,
			Fresh:  false,
			Reason: "legacy index - run 'ckb index' to enable freshness tracking",
		}
	}

	// Check freshness and staleness
	freshness := meta.CheckFreshness(repoRoot)
	staleness := meta.GetStaleness(repoRoot)

	return &IndexStatusCLI{
		Exists:         true,
		Fresh:          freshness.Fresh,
		Reason:         freshness.Reason,
		CreatedAt:      meta.CreatedAt,
		IndexAge:       staleness.IndexAge,
		CommitHash:     meta.CommitHash,
		FileCount:      meta.FileCount,
		CommitsBehind:  freshness.CommitsBehind,
		HasUncommitted: freshness.HasUncommitted,
	}
}

// getChangeImpactStatus detects coverage files and CODEOWNERS for change impact analysis
func getChangeImpactStatus(repoRoot string) *ChangeImpactStatusCLI {
	status := &ChangeImpactStatusCLI{}

	// Detect language
	lang, _, ok := project.DetectLanguage(repoRoot)
	if ok {
		status.Language = string(lang)
	}

	// Detect coverage file
	status.Coverage = detectCoverage(repoRoot, lang)

	// Detect CODEOWNERS
	status.Codeowners = detectCodeowners(repoRoot)

	return status
}

// coverageLocation describes a coverage file location by language
type coverageLocation struct {
	paths       []string
	generateCmd string
}

// coverageByLang maps languages to their coverage file locations and generation commands
var coverageByLang = map[project.Language]coverageLocation{
	project.LangGo: {
		paths:       []string{"coverage.out", "coverage.txt", "cover.out"},
		generateCmd: "go test -coverprofile=coverage.out ./...",
	},
	project.LangDart: {
		paths:       []string{"coverage/lcov.info"},
		generateCmd: "flutter test --coverage",
	},
	project.LangTypeScript: {
		paths:       []string{"coverage/lcov.info", "coverage/coverage-final.json"},
		generateCmd: "npm test -- --coverage",
	},
	project.LangJavaScript: {
		paths:       []string{"coverage/lcov.info", "coverage/coverage-final.json"},
		generateCmd: "npm test -- --coverage",
	},
	project.LangPython: {
		paths:       []string{".coverage", "coverage.xml", "htmlcov/index.html"},
		generateCmd: "pytest --cov=. --cov-report=xml",
	},
	project.LangRust: {
		paths:       []string{"target/coverage/lcov.info", "tarpaulin-report.json"},
		generateCmd: "cargo tarpaulin --out Lcov",
	},
	project.LangJava: {
		paths:       []string{"target/site/jacoco/jacoco.xml", "build/reports/jacoco/test/jacocoTestReport.xml"},
		generateCmd: "mvn test jacoco:report",
	},
}

// genericCoveragePaths are checked for any language
var genericCoveragePaths = []string{
	"lcov.info",
	"coverage/lcov.info",
	".coverage",
}

// detectCoverage looks for coverage files in the repository
func detectCoverage(repoRoot string, lang project.Language) *CoverageStatusCLI {
	status := &CoverageStatusCLI{Found: false}

	// Get language-specific paths
	var paths []string
	var generateCmd string

	if loc, ok := coverageByLang[lang]; ok {
		paths = loc.paths
		generateCmd = loc.generateCmd
	}

	// Add generic paths
	paths = append(paths, genericCoveragePaths...)

	// Check each path
	for _, relPath := range paths {
		fullPath := filepath.Join(repoRoot, relPath)
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			status.Found = true
			status.Path = relPath
			status.ModTime = info.ModTime()
			status.Age = formatDuration(time.Since(info.ModTime()))

			// Check if stale (> 7 days old)
			if time.Since(info.ModTime()) > 7*24*time.Hour {
				status.Stale = true
			}
			break
		}
	}

	// Set generation command if not found
	if !status.Found && generateCmd != "" {
		status.GenerateCmd = generateCmd
	}

	return status
}

// codeownersLocations are the standard CODEOWNERS file locations
var codeownersLocations = []string{
	".github/CODEOWNERS",
	"CODEOWNERS",
	"docs/CODEOWNERS",
}

// detectCodeowners looks for CODEOWNERS file and parses basic stats
func detectCodeowners(repoRoot string) *CodeownersStatusCLI {
	status := &CodeownersStatusCLI{Found: false}

	for _, relPath := range codeownersLocations {
		fullPath := filepath.Join(repoRoot, relPath)
		content, err := os.ReadFile(fullPath)
		if err == nil {
			status.Found = true
			status.Path = relPath

			// Parse basic stats
			lines := strings.Split(string(content), "\n")
			teams := make(map[string]bool)

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				status.PatternCount++

				// Extract team mentions (@org/team or @username)
				parts := strings.Fields(line)
				for _, part := range parts[1:] { // Skip the pattern
					if strings.HasPrefix(part, "@") {
						teams[part] = true
					}
				}
			}
			status.TeamCount = len(teams)
			break
		}
	}

	return status
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

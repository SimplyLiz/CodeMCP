// Package project provides language quality assessment for CKB.
package project

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
)

// LanguageTier represents the support tier for a language.
type LanguageTier int

const (
	// Tier1 - Full support, all features, stable
	Tier1 LanguageTier = 1
	// Tier2 - Full support, known edge cases
	Tier2 LanguageTier = 2
	// Tier3 - Basic support, callgraph may be incomplete
	Tier3 LanguageTier = 3
	// Tier4 - Experimental
	Tier4 LanguageTier = 4
)

// String returns the tier name.
func (t LanguageTier) String() string {
	switch t {
	case Tier1:
		return "Tier 1 (Full)"
	case Tier2:
		return "Tier 2 (Standard)"
	case Tier3:
		return "Tier 3 (Basic)"
	case Tier4:
		return "Tier 4 (Experimental)"
	default:
		return "Unknown"
	}
}

// QualityLevel represents the quality assessment result.
type QualityLevel string

const (
	QualityOK       QualityLevel = "ok"
	QualityDegraded QualityLevel = "degraded"
	QualityPartial  QualityLevel = "partial"
	QualityUnknown  QualityLevel = "unknown"
)

// LanguageQuality contains quality metrics for a language in a project.
type LanguageQuality struct {
	Language         Language     `json:"language"`
	DisplayName      string       `json:"displayName"`
	Tier             LanguageTier `json:"tier"`
	TierName         string       `json:"tierName"`
	Quality          QualityLevel `json:"quality"`
	SymbolCount      int          `json:"symbolCount"`
	RefCount         int          `json:"refCount"`
	CallEdgeCount    int          `json:"callEdgeCount"`
	FileCount        int          `json:"fileCount"`
	RefAccuracy      float64      `json:"refAccuracy"` // Estimated 0-1
	CallGraphQuality QualityLevel `json:"callGraphQuality"`
	KnownIssues      []string     `json:"knownIssues,omitempty"`
	Recommendations  []string     `json:"recommendations,omitempty"`
}

// LanguageTierInfo contains static tier information for a language.
type LanguageTierInfo struct {
	Language    Language
	Tier        LanguageTier
	KnownIssues []string
	Notes       string
}

// languageTiers defines the support tier for each language.
var languageTiers = map[Language]LanguageTierInfo{
	LangGo: {
		Language: LangGo,
		Tier:     Tier1,
		Notes:    "Full support with scip-go",
	},
	LangTypeScript: {
		Language: LangTypeScript,
		Tier:     Tier2,
		KnownIssues: []string{
			"Monorepo with multiple tsconfig may need --infer-tsconfig",
			"Path aliases require tsconfig.json",
		},
		Notes: "Full support with scip-typescript",
	},
	LangJavaScript: {
		Language: LangJavaScript,
		Tier:     Tier2,
		KnownIssues: []string{
			"Dynamic imports may not be tracked",
			"CommonJS require() patterns may have incomplete resolution",
		},
		Notes: "Full support with scip-typescript",
	},
	LangPython: {
		Language: LangPython,
		Tier:     Tier2,
		KnownIssues: []string{
			"Virtual environments must be activated or detected",
			"Dynamic imports (importlib) not tracked",
			"Type stubs may not be fully resolved",
		},
		Notes: "Requires virtual environment for best results",
	},
	LangRust: {
		Language: LangRust,
		Tier:     Tier3,
		KnownIssues: []string{
			"Macro expansions may have incomplete references",
			"Proc macros may not be fully analyzed",
		},
		Notes: "Uses rust-analyzer SCIP output",
	},
	LangJava: {
		Language: LangJava,
		Tier:     Tier3,
		KnownIssues: []string{
			"Gradle projects may need build task first",
			"Annotation processors may have incomplete resolution",
		},
		Notes: "Requires scip-java with Maven/Gradle",
	},
	LangKotlin: {
		Language: LangKotlin,
		Tier:     Tier3,
		KnownIssues: []string{
			"Requires Gradle plugin setup",
			"Extension functions may have incomplete resolution",
		},
		Notes: "Uses scip-java with Kotlin support",
	},
	LangCpp: {
		Language: LangCpp,
		Tier:     Tier3,
		KnownIssues: []string{
			"Requires compile_commands.json",
			"Templates may have incomplete resolution",
			"Macros can affect accuracy",
		},
		Notes: "Requires scip-clang with compile_commands.json",
	},
	LangRuby: {
		Language: LangRuby,
		Tier:     Tier3,
		KnownIssues: []string{
			"Sorbet types improve accuracy",
			"Metaprogramming may not be tracked",
		},
		Notes: "Better results with Sorbet type annotations",
	},
	LangDart: {
		Language: LangDart,
		Tier:     Tier3,
		KnownIssues: []string{
			"Flutter projects need 'flutter pub get' first",
		},
		Notes: "Uses scip_dart",
	},
	LangCSharp: {
		Language: LangCSharp,
		Tier:     Tier4,
		KnownIssues: []string{
			"Requires .NET 8+ SDK",
			"Source generators may have incomplete resolution",
		},
		Notes: "Experimental - uses scip-dotnet",
	},
	LangPHP: {
		Language: LangPHP,
		Tier:     Tier4,
		KnownIssues: []string{
			"Requires PHP 8.2+",
			"Dynamic calls may not be tracked",
		},
		Notes: "Experimental - uses scip-php",
	},
}

// GetLanguageTierInfo returns tier information for a language.
func GetLanguageTierInfo(lang Language) LanguageTierInfo {
	if info, ok := languageTiers[lang]; ok {
		return info
	}
	return LanguageTierInfo{
		Language: lang,
		Tier:     Tier4,
		Notes:    "Unknown language",
	}
}

// QualityAssessor assesses language quality for a project.
type QualityAssessor struct {
	root   string
	dbPath string
}

// NewQualityAssessor creates a new quality assessor.
func NewQualityAssessor(root string) *QualityAssessor {
	return &QualityAssessor{
		root:   root,
		dbPath: filepath.Join(root, ".ckb", "ckb.db"),
	}
}

// AssessLanguage assesses the quality of a specific language in the project.
func (qa *QualityAssessor) AssessLanguage(ctx context.Context, lang Language) (*LanguageQuality, error) {
	tierInfo := GetLanguageTierInfo(lang)

	quality := &LanguageQuality{
		Language:         lang,
		DisplayName:      LanguageDisplayName(lang),
		Tier:             tierInfo.Tier,
		TierName:         tierInfo.Tier.String(),
		Quality:          QualityUnknown,
		KnownIssues:      tierInfo.KnownIssues,
		CallGraphQuality: QualityUnknown,
	}

	// Try to get metrics from database
	if _, err := os.Stat(qa.dbPath); err == nil {
		if err := qa.loadMetrics(ctx, quality); err != nil {
			// Log but continue - we can still return tier info
			_ = err
		}
	}

	// Assess overall quality based on metrics
	quality.Quality = qa.assessOverallQuality(quality)
	quality.CallGraphQuality = qa.assessCallGraphQuality(quality)

	// Generate recommendations
	quality.Recommendations = qa.generateRecommendations(quality)

	return quality, nil
}

// AssessAllLanguages assesses quality for all detected languages.
func (qa *QualityAssessor) AssessAllLanguages(ctx context.Context) (map[Language]*LanguageQuality, error) {
	_, _, langs := DetectAllLanguages(qa.root)

	results := make(map[Language]*LanguageQuality)

	for _, lang := range langs {
		quality, err := qa.AssessLanguage(ctx, lang)
		if err != nil {
			continue
		}
		results[lang] = quality
	}

	return results, nil
}

// loadMetrics loads metrics from the database.
func (qa *QualityAssessor) loadMetrics(ctx context.Context, quality *LanguageQuality) error {
	db, err := sql.Open("sqlite", qa.dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	langStr := string(quality.Language)

	// Count symbols for this language
	var symbolCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM symbols_fts_content WHERE language = ?
	`, langStr).Scan(&symbolCount)
	if err == nil {
		quality.SymbolCount = symbolCount
	}

	// Count references for this language
	var refCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM refs WHERE language = ?
	`, langStr).Scan(&refCount)
	if err == nil {
		quality.RefCount = refCount
	}

	// Count call edges for this language
	var callCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM call_edges WHERE
		caller_file_id IN (SELECT path FROM files WHERE language = ?)
	`, langStr).Scan(&callCount)
	if err == nil {
		quality.CallEdgeCount = callCount
	}

	// Count files for this language
	var fileCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM files WHERE language = ?
	`, langStr).Scan(&fileCount)
	if err == nil {
		quality.FileCount = fileCount
	}

	// Estimate reference accuracy based on symbol/ref ratio
	if quality.SymbolCount > 0 {
		// A healthy codebase typically has 3-10 refs per symbol
		ratio := float64(quality.RefCount) / float64(quality.SymbolCount)
		if ratio >= 3 && ratio <= 15 {
			quality.RefAccuracy = 0.9 + (0.1 * (1 - (ratio-3)/12))
		} else if ratio >= 1 {
			quality.RefAccuracy = 0.7 + (0.2 * ratio / 3)
		} else {
			quality.RefAccuracy = 0.5 // Low ref count suggests incomplete indexing
		}
		if quality.RefAccuracy > 1 {
			quality.RefAccuracy = 1
		}
	}

	return nil
}

// assessOverallQuality assesses overall quality based on metrics.
func (qa *QualityAssessor) assessOverallQuality(quality *LanguageQuality) QualityLevel {
	if quality.SymbolCount == 0 {
		return QualityUnknown
	}

	// Check thresholds based on tier
	switch quality.Tier {
	case Tier1, Tier2:
		if quality.RefAccuracy >= 0.9 {
			return QualityOK
		} else if quality.RefAccuracy >= 0.7 {
			return QualityDegraded
		}
		return QualityPartial
	case Tier3:
		if quality.RefAccuracy >= 0.8 {
			return QualityOK
		} else if quality.RefAccuracy >= 0.5 {
			return QualityDegraded
		}
		return QualityPartial
	default:
		if quality.SymbolCount > 0 && quality.RefCount > 0 {
			return QualityPartial
		}
		return QualityUnknown
	}
}

// assessCallGraphQuality assesses call graph quality.
func (qa *QualityAssessor) assessCallGraphQuality(quality *LanguageQuality) QualityLevel {
	if quality.SymbolCount == 0 {
		return QualityUnknown
	}

	// Estimate based on call edges per symbol (functions/methods)
	// Typical codebases have 0.5-2 call edges per symbol
	if quality.CallEdgeCount == 0 {
		return QualityUnknown
	}

	ratio := float64(quality.CallEdgeCount) / float64(quality.SymbolCount)

	switch quality.Tier {
	case Tier1:
		if ratio >= 0.5 {
			return QualityOK
		}
		return QualityDegraded
	case Tier2:
		if ratio >= 0.3 {
			return QualityOK
		}
		return QualityDegraded
	default:
		if ratio >= 0.1 {
			return QualityPartial
		}
		return QualityUnknown
	}
}

// generateRecommendations generates recommendations for improving quality.
func (qa *QualityAssessor) generateRecommendations(quality *LanguageQuality) []string {
	var recs []string

	if quality.SymbolCount == 0 {
		recs = append(recs, "Run 'ckb index' to generate SCIP index")
		return recs
	}

	if quality.RefAccuracy < 0.7 {
		recs = append(recs, "Reference accuracy is low - check indexer output for errors")
	}

	if quality.CallEdgeCount == 0 {
		recs = append(recs, "No call graph data - re-run indexer or check language support")
	}

	// Language-specific recommendations
	switch quality.Language {
	case LangPython:
		if detected := qa.detectPythonVenv(); detected == "" {
			recs = append(recs, "Activate virtual environment before indexing for better results")
		}
	case LangTypeScript:
		if !qa.hasTSConfig() {
			recs = append(recs, "Add tsconfig.json for better path resolution")
		}
	case LangJava, LangKotlin:
		if quality.CallEdgeCount == 0 {
			recs = append(recs, "Ensure project is built before indexing (mvn compile or gradle build)")
		}
	case LangCpp:
		if FindCompileCommands(qa.root) == "" {
			recs = append(recs, "Generate compile_commands.json for accurate indexing")
		}
	}

	return recs
}

// detectPythonVenv checks for active or available Python virtual environments.
func (qa *QualityAssessor) detectPythonVenv() string {
	// Check for active venv via environment
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		return venv
	}

	// Check common venv locations
	venvPaths := []string{
		".venv",
		"venv",
		"env",
		".env",
	}

	for _, vp := range venvPaths {
		activatePath := filepath.Join(qa.root, vp, "bin", "activate")
		if _, err := os.Stat(activatePath); err == nil {
			return filepath.Join(qa.root, vp)
		}
		// Windows
		activatePath = filepath.Join(qa.root, vp, "Scripts", "activate")
		if _, err := os.Stat(activatePath); err == nil {
			return filepath.Join(qa.root, vp)
		}
	}

	return ""
}

// hasTSConfig checks if tsconfig.json exists.
func (qa *QualityAssessor) hasTSConfig() bool {
	_, err := os.Stat(filepath.Join(qa.root, "tsconfig.json"))
	return err == nil
}

// ProjectQualityReport contains a full quality report for a project.
type ProjectQualityReport struct {
	PrimaryLanguage Language                      `json:"primaryLanguage"`
	Languages       map[Language]*LanguageQuality `json:"languages"`
	OverallQuality  QualityLevel                  `json:"overallQuality"`
	IndexedAt       string                        `json:"indexedAt,omitempty"`
	TotalSymbols    int                           `json:"totalSymbols"`
	TotalRefs       int                           `json:"totalRefs"`
	TotalCallEdges  int                           `json:"totalCallEdges"`
	Summary         string                        `json:"summary"`
}

// GenerateReport generates a full quality report for the project.
func (qa *QualityAssessor) GenerateReport(ctx context.Context) (*ProjectQualityReport, error) {
	primary, _, _ := DetectAllLanguages(qa.root)
	languages, err := qa.AssessAllLanguages(ctx)
	if err != nil {
		return nil, err
	}

	report := &ProjectQualityReport{
		PrimaryLanguage: primary,
		Languages:       languages,
		OverallQuality:  QualityUnknown,
	}

	// Aggregate metrics
	var okCount, totalCount int
	for _, lq := range languages {
		report.TotalSymbols += lq.SymbolCount
		report.TotalRefs += lq.RefCount
		report.TotalCallEdges += lq.CallEdgeCount
		totalCount++
		if lq.Quality == QualityOK {
			okCount++
		}
	}

	// Determine overall quality
	if totalCount == 0 {
		report.OverallQuality = QualityUnknown
		report.Summary = "No languages detected or indexed"
	} else if okCount == totalCount {
		report.OverallQuality = QualityOK
		report.Summary = "All languages indexed with good quality"
	} else if okCount > 0 {
		report.OverallQuality = QualityDegraded
		report.Summary = "Some languages have reduced quality"
	} else {
		report.OverallQuality = QualityPartial
		report.Summary = "Index quality is limited - check recommendations"
	}

	return report, nil
}

// DetectPythonEnvironment returns information about Python environment.
func DetectPythonEnvironment(root string) *PythonEnvInfo {
	info := &PythonEnvInfo{}

	// Check for active venv
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		info.ActiveVenv = venv
		info.IsActive = true
	}

	// Check for venv directories
	venvPaths := []string{".venv", "venv", "env", ".env"}
	for _, vp := range venvPaths {
		fullPath := filepath.Join(root, vp)
		if _, err := os.Stat(filepath.Join(fullPath, "bin", "python")); err == nil {
			info.DetectedVenvs = append(info.DetectedVenvs, fullPath)
		} else if _, err := os.Stat(filepath.Join(fullPath, "Scripts", "python.exe")); err == nil {
			info.DetectedVenvs = append(info.DetectedVenvs, fullPath)
		}
	}

	// Check for pyproject.toml (Poetry, PDM, etc.)
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
		info.HasPyproject = true
	}

	// Check for requirements.txt
	if _, err := os.Stat(filepath.Join(root, "requirements.txt")); err == nil {
		info.HasRequirements = true
	}

	// Check for Pipfile (Pipenv)
	if _, err := os.Stat(filepath.Join(root, "Pipfile")); err == nil {
		info.HasPipfile = true
	}

	return info
}

// PythonEnvInfo contains Python environment information.
type PythonEnvInfo struct {
	ActiveVenv      string   `json:"activeVenv,omitempty"`
	IsActive        bool     `json:"isActive"`
	DetectedVenvs   []string `json:"detectedVenvs,omitempty"`
	HasPyproject    bool     `json:"hasPyproject"`
	HasRequirements bool     `json:"hasRequirements"`
	HasPipfile      bool     `json:"hasPipfile"`
}

// DetectTypeScriptMonorepo returns information about TypeScript monorepo setup.
func DetectTypeScriptMonorepo(root string) *TSMonorepoInfo {
	info := &TSMonorepoInfo{}

	// Check for root tsconfig.json
	if _, err := os.Stat(filepath.Join(root, "tsconfig.json")); err == nil {
		info.HasRootTsconfig = true
	}

	// Check for packages directory (common in monorepos)
	packagesDir := filepath.Join(root, "packages")
	if entries, err := os.ReadDir(packagesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				pkgPath := filepath.Join(packagesDir, entry.Name())
				if _, err := os.Stat(filepath.Join(pkgPath, "package.json")); err == nil {
					info.Packages = append(info.Packages, entry.Name())
					// Check for per-package tsconfig
					if _, err := os.Stat(filepath.Join(pkgPath, "tsconfig.json")); err == nil {
						info.PackagesWithTsconfig = append(info.PackagesWithTsconfig, entry.Name())
					}
				}
			}
		}
	}

	// Check for workspace configuration
	if _, err := os.Stat(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		info.WorkspaceType = "pnpm"
		info.IsMonorepo = true
	} else if _, err := os.Stat(filepath.Join(root, "lerna.json")); err == nil {
		info.WorkspaceType = "lerna"
		info.IsMonorepo = true
	} else if _, err := os.Stat(filepath.Join(root, "nx.json")); err == nil {
		info.WorkspaceType = "nx"
		info.IsMonorepo = true
	} else if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		if strings.Contains(string(data), "\"workspaces\"") {
			info.WorkspaceType = "yarn/npm"
			info.IsMonorepo = true
		}
	}

	if len(info.Packages) > 1 {
		info.IsMonorepo = true
	}

	return info
}

// TSMonorepoInfo contains TypeScript monorepo information.
type TSMonorepoInfo struct {
	IsMonorepo           bool     `json:"isMonorepo"`
	WorkspaceType        string   `json:"workspaceType,omitempty"` // pnpm, lerna, yarn, nx
	HasRootTsconfig      bool     `json:"hasRootTsconfig"`
	Packages             []string `json:"packages,omitempty"`
	PackagesWithTsconfig []string `json:"packagesWithTsconfig,omitempty"`
}

package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageTier_String(t *testing.T) {
	tests := []struct {
		tier     LanguageTier
		expected string
	}{
		{Tier1, "Tier 1 (Full)"},
		{Tier2, "Tier 2 (Standard)"},
		{Tier3, "Tier 3 (Basic)"},
		{Tier4, "Tier 4 (Experimental)"},
		{LanguageTier(99), "Unknown"},
	}

	for _, tt := range tests {
		result := tt.tier.String()
		if result != tt.expected {
			t.Errorf("Tier %d: expected %q, got %q", tt.tier, tt.expected, result)
		}
	}
}

func TestGetLanguageTierInfo(t *testing.T) {
	// Test known languages
	tests := []struct {
		lang         Language
		expectedTier LanguageTier
	}{
		{LangGo, Tier1},
		{LangTypeScript, Tier2},
		{LangPython, Tier2},
		{LangRust, Tier3},
		{LangJava, Tier3},
		{LangCSharp, Tier4},
	}

	for _, tt := range tests {
		info := GetLanguageTierInfo(tt.lang)
		if info.Tier != tt.expectedTier {
			t.Errorf("Language %s: expected tier %d, got %d", tt.lang, tt.expectedTier, info.Tier)
		}
		if info.Language != tt.lang {
			t.Errorf("Language %s: expected language to match", tt.lang)
		}
	}

	// Test unknown language
	unknownInfo := GetLanguageTierInfo(Language("unknown"))
	if unknownInfo.Tier != Tier4 {
		t.Errorf("Unknown language: expected tier 4, got %d", unknownInfo.Tier)
	}
}

func TestGetLanguageTierInfo_KnownIssues(t *testing.T) {
	// Languages with known issues should have them populated
	tsInfo := GetLanguageTierInfo(LangTypeScript)
	if len(tsInfo.KnownIssues) == 0 {
		t.Error("TypeScript should have known issues")
	}

	pythonInfo := GetLanguageTierInfo(LangPython)
	if len(pythonInfo.KnownIssues) == 0 {
		t.Error("Python should have known issues")
	}

	// Go has no known issues (Tier 1)
	goInfo := GetLanguageTierInfo(LangGo)
	if len(goInfo.KnownIssues) != 0 {
		t.Error("Go should have no known issues")
	}
}

func TestQualityAssessor_NewQualityAssessor(t *testing.T) {
	root := "/test/project"
	assessor := NewQualityAssessor(root)

	if assessor.root != root {
		t.Errorf("Expected root %s, got %s", root, assessor.root)
	}

	expectedDBPath := filepath.Join(root, ".ckb", "ckb.db")
	if assessor.dbPath != expectedDBPath {
		t.Errorf("Expected dbPath %s, got %s", expectedDBPath, assessor.dbPath)
	}
}

func TestQualityAssessor_AssessLanguage_NoDatabase(t *testing.T) {
	// Use temp dir with no database
	tmpDir := t.TempDir()
	assessor := NewQualityAssessor(tmpDir)

	ctx := context.Background()
	quality, err := assessor.AssessLanguage(ctx, LangGo)

	if err != nil {
		t.Fatalf("AssessLanguage failed: %v", err)
	}

	if quality.Language != LangGo {
		t.Errorf("Expected language Go, got %s", quality.Language)
	}

	if quality.Tier != Tier1 {
		t.Errorf("Expected tier 1, got %d", quality.Tier)
	}

	if quality.Quality != QualityUnknown {
		t.Errorf("Expected unknown quality (no DB), got %s", quality.Quality)
	}
}

func TestDetectPythonEnvironment_NoVenv(t *testing.T) {
	tmpDir := t.TempDir()
	info := DetectPythonEnvironment(tmpDir)

	if info.IsActive {
		t.Error("Should not have active venv")
	}

	if len(info.DetectedVenvs) > 0 {
		t.Errorf("Should not detect any venvs, got %v", info.DetectedVenvs)
	}

	if info.HasPyproject {
		t.Error("Should not have pyproject.toml")
	}

	if info.HasRequirements {
		t.Error("Should not have requirements.txt")
	}

	if info.HasPipfile {
		t.Error("Should not have Pipfile")
	}
}

func TestDetectPythonEnvironment_WithPyproject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create pyproject.toml
	pyprojectPath := filepath.Join(tmpDir, "pyproject.toml")
	if err := os.WriteFile(pyprojectPath, []byte("[project]\nname = \"test\""), 0644); err != nil {
		t.Fatalf("Failed to create pyproject.toml: %v", err)
	}

	info := DetectPythonEnvironment(tmpDir)

	if !info.HasPyproject {
		t.Error("Should detect pyproject.toml")
	}
}

func TestDetectPythonEnvironment_WithRequirements(t *testing.T) {
	tmpDir := t.TempDir()

	// Create requirements.txt
	reqPath := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte("flask==2.0\n"), 0644); err != nil {
		t.Fatalf("Failed to create requirements.txt: %v", err)
	}

	info := DetectPythonEnvironment(tmpDir)

	if !info.HasRequirements {
		t.Error("Should detect requirements.txt")
	}
}

func TestDetectPythonEnvironment_WithPipfile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Pipfile
	pipfilePath := filepath.Join(tmpDir, "Pipfile")
	if err := os.WriteFile(pipfilePath, []byte("[packages]\n"), 0644); err != nil {
		t.Fatalf("Failed to create Pipfile: %v", err)
	}

	info := DetectPythonEnvironment(tmpDir)

	if !info.HasPipfile {
		t.Error("Should detect Pipfile")
	}
}

func TestDetectPythonEnvironment_WithVenv(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .venv directory with bin/python
	venvDir := filepath.Join(tmpDir, ".venv", "bin")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatalf("Failed to create venv dir: %v", err)
	}
	pythonPath := filepath.Join(venvDir, "python")
	if err := os.WriteFile(pythonPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("Failed to create python: %v", err)
	}

	info := DetectPythonEnvironment(tmpDir)

	if len(info.DetectedVenvs) != 1 {
		t.Errorf("Should detect 1 venv, got %d", len(info.DetectedVenvs))
	}

	if len(info.DetectedVenvs) > 0 {
		expectedPath := filepath.Join(tmpDir, ".venv")
		if info.DetectedVenvs[0] != expectedPath {
			t.Errorf("Expected venv path %s, got %s", expectedPath, info.DetectedVenvs[0])
		}
	}
}

func TestDetectTypeScriptMonorepo_NotMonorepo(t *testing.T) {
	tmpDir := t.TempDir()
	info := DetectTypeScriptMonorepo(tmpDir)

	if info.IsMonorepo {
		t.Error("Should not be detected as monorepo")
	}

	if info.WorkspaceType != "" {
		t.Errorf("Should not have workspace type, got %s", info.WorkspaceType)
	}

	if info.HasRootTsconfig {
		t.Error("Should not have root tsconfig")
	}
}

func TestDetectTypeScriptMonorepo_WithTsconfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create tsconfig.json
	tsconfigPath := filepath.Join(tmpDir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte("{\"compilerOptions\": {}}"), 0644); err != nil {
		t.Fatalf("Failed to create tsconfig.json: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if !info.HasRootTsconfig {
		t.Error("Should detect root tsconfig.json")
	}
}

func TestDetectTypeScriptMonorepo_PnpmWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create pnpm-workspace.yaml
	workspacePath := filepath.Join(tmpDir, "pnpm-workspace.yaml")
	if err := os.WriteFile(workspacePath, []byte("packages:\n  - packages/*\n"), 0644); err != nil {
		t.Fatalf("Failed to create pnpm-workspace.yaml: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if !info.IsMonorepo {
		t.Error("Should be detected as monorepo")
	}

	if info.WorkspaceType != "pnpm" {
		t.Errorf("Expected workspace type 'pnpm', got %s", info.WorkspaceType)
	}
}

func TestDetectTypeScriptMonorepo_LernaWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create lerna.json
	lernaPath := filepath.Join(tmpDir, "lerna.json")
	if err := os.WriteFile(lernaPath, []byte("{\"version\": \"0.0.0\"}"), 0644); err != nil {
		t.Fatalf("Failed to create lerna.json: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if !info.IsMonorepo {
		t.Error("Should be detected as monorepo")
	}

	if info.WorkspaceType != "lerna" {
		t.Errorf("Expected workspace type 'lerna', got %s", info.WorkspaceType)
	}
}

func TestDetectTypeScriptMonorepo_NxWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nx.json
	nxPath := filepath.Join(tmpDir, "nx.json")
	if err := os.WriteFile(nxPath, []byte("{\"tasksRunnerOptions\": {}}"), 0644); err != nil {
		t.Fatalf("Failed to create nx.json: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if !info.IsMonorepo {
		t.Error("Should be detected as monorepo")
	}

	if info.WorkspaceType != "nx" {
		t.Errorf("Expected workspace type 'nx', got %s", info.WorkspaceType)
	}
}

func TestDetectTypeScriptMonorepo_YarnWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package.json with workspaces
	pkgPath := filepath.Join(tmpDir, "package.json")
	pkgContent := `{"name": "root", "workspaces": ["packages/*"]}`
	if err := os.WriteFile(pkgPath, []byte(pkgContent), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if !info.IsMonorepo {
		t.Error("Should be detected as monorepo")
	}

	if info.WorkspaceType != "yarn/npm" {
		t.Errorf("Expected workspace type 'yarn/npm', got %s", info.WorkspaceType)
	}
}

func TestDetectTypeScriptMonorepo_WithPackages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create pnpm-workspace.yaml
	workspacePath := filepath.Join(tmpDir, "pnpm-workspace.yaml")
	if err := os.WriteFile(workspacePath, []byte("packages:\n  - packages/*\n"), 0644); err != nil {
		t.Fatalf("Failed to create pnpm-workspace.yaml: %v", err)
	}

	// Create packages directory with two packages
	packagesDir := filepath.Join(tmpDir, "packages")
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		t.Fatalf("Failed to create packages dir: %v", err)
	}

	for _, pkg := range []string{"pkg-a", "pkg-b"} {
		pkgDir := filepath.Join(packagesDir, pkg)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatalf("Failed to create package dir: %v", err)
		}
		pkgJSON := filepath.Join(pkgDir, "package.json")
		if err := os.WriteFile(pkgJSON, []byte("{\"name\": \""+pkg+"\"}"), 0644); err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}
	}

	// Add tsconfig to one package
	tsconfigPath := filepath.Join(packagesDir, "pkg-a", "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create tsconfig.json: %v", err)
	}

	info := DetectTypeScriptMonorepo(tmpDir)

	if len(info.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(info.Packages))
	}

	if len(info.PackagesWithTsconfig) != 1 {
		t.Errorf("Expected 1 package with tsconfig, got %d", len(info.PackagesWithTsconfig))
	}
}

func TestAssessOverallQuality(t *testing.T) {
	assessor := NewQualityAssessor("/test")

	tests := []struct {
		name     string
		quality  *LanguageQuality
		expected QualityLevel
	}{
		{
			name: "no symbols",
			quality: &LanguageQuality{
				SymbolCount: 0,
			},
			expected: QualityUnknown,
		},
		{
			name: "tier 1 high accuracy",
			quality: &LanguageQuality{
				Tier:        Tier1,
				SymbolCount: 100,
				RefAccuracy: 0.95,
			},
			expected: QualityOK,
		},
		{
			name: "tier 1 degraded accuracy",
			quality: &LanguageQuality{
				Tier:        Tier1,
				SymbolCount: 100,
				RefAccuracy: 0.75,
			},
			expected: QualityDegraded,
		},
		{
			name: "tier 1 partial accuracy",
			quality: &LanguageQuality{
				Tier:        Tier1,
				SymbolCount: 100,
				RefAccuracy: 0.5,
			},
			expected: QualityPartial,
		},
		{
			name: "tier 3 ok accuracy",
			quality: &LanguageQuality{
				Tier:        Tier3,
				SymbolCount: 100,
				RefAccuracy: 0.85,
			},
			expected: QualityOK,
		},
		{
			name: "tier 4 with data",
			quality: &LanguageQuality{
				Tier:        Tier4,
				SymbolCount: 100,
				RefCount:    200,
			},
			expected: QualityPartial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assessor.assessOverallQuality(tt.quality)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAssessCallGraphQuality(t *testing.T) {
	assessor := NewQualityAssessor("/test")

	tests := []struct {
		name     string
		quality  *LanguageQuality
		expected QualityLevel
	}{
		{
			name: "no symbols",
			quality: &LanguageQuality{
				SymbolCount: 0,
			},
			expected: QualityUnknown,
		},
		{
			name: "no call edges",
			quality: &LanguageQuality{
				SymbolCount:   100,
				CallEdgeCount: 0,
			},
			expected: QualityUnknown,
		},
		{
			name: "tier 1 good ratio",
			quality: &LanguageQuality{
				Tier:          Tier1,
				SymbolCount:   100,
				CallEdgeCount: 60, // 0.6 ratio
			},
			expected: QualityOK,
		},
		{
			name: "tier 1 low ratio",
			quality: &LanguageQuality{
				Tier:          Tier1,
				SymbolCount:   100,
				CallEdgeCount: 20, // 0.2 ratio
			},
			expected: QualityDegraded,
		},
		{
			name: "tier 2 ok ratio",
			quality: &LanguageQuality{
				Tier:          Tier2,
				SymbolCount:   100,
				CallEdgeCount: 35, // 0.35 ratio
			},
			expected: QualityOK,
		},
		{
			name: "tier 3 partial ratio",
			quality: &LanguageQuality{
				Tier:          Tier3,
				SymbolCount:   100,
				CallEdgeCount: 15, // 0.15 ratio
			},
			expected: QualityPartial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assessor.assessCallGraphQuality(tt.quality)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGenerateRecommendations(t *testing.T) {
	assessor := NewQualityAssessor("/test")

	// No symbols - should recommend indexing
	quality := &LanguageQuality{
		SymbolCount: 0,
	}
	recs := assessor.generateRecommendations(quality)
	if len(recs) == 0 {
		t.Error("Expected recommendation for no symbols")
	}
	if recs[0] != "Run 'ckb index' to generate SCIP index" {
		t.Errorf("Expected index recommendation, got: %s", recs[0])
	}

	// Low ref accuracy
	quality2 := &LanguageQuality{
		SymbolCount: 100,
		RefAccuracy: 0.5,
	}
	recs2 := assessor.generateRecommendations(quality2)
	found := false
	for _, r := range recs2 {
		if r == "Reference accuracy is low - check indexer output for errors" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected low accuracy recommendation")
	}

	// No call edges
	quality3 := &LanguageQuality{
		SymbolCount:   100,
		CallEdgeCount: 0,
	}
	recs3 := assessor.generateRecommendations(quality3)
	found = false
	for _, r := range recs3 {
		if r == "No call graph data - re-run indexer or check language support" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected no call graph recommendation")
	}
}

func TestProjectQualityReport(t *testing.T) {
	tmpDir := t.TempDir()
	assessor := NewQualityAssessor(tmpDir)

	ctx := context.Background()
	report, err := assessor.GenerateReport(ctx)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	// Empty project should have unknown quality
	if report.OverallQuality != QualityUnknown {
		t.Errorf("Expected unknown quality for empty project, got %s", report.OverallQuality)
	}
}
